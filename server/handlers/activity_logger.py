# server/handlers/activity_logger.py
import sqlite3
import json
from datetime import datetime
import os

DB_PATH = os.path.join(os.path.dirname(__file__), '../../database/c2.db')

class ActivityLogger:
    def __init__(self):
        self._init_db()

    def _init_db(self):
        os.makedirs(os.path.dirname(DB_PATH), exist_ok=True)
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute('''CREATE TABLE IF NOT EXISTS shell_activity (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            victim_id TEXT,
            command TEXT,
            output TEXT,
            timestamp TEXT,
            direction TEXT
        )''')
        c.execute('''CREATE TABLE IF NOT EXISTS agent_activity (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            agent_id TEXT,
            command_id TEXT,
            command_type TEXT,
            command_payload TEXT,
            result_output TEXT,
            result_error TEXT,
            sent_at TEXT,
            completed_at TEXT
        )''')
        conn.commit()
        conn.close()

    def log_shell_command(self, victim_id, command):
        now = datetime.utcnow().isoformat()
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("INSERT INTO shell_activity (victim_id, command, output, timestamp, direction) VALUES (?,?,?,?,?)",
                  (victim_id, command, '', now, 'sent'))
        conn.commit()
        conn.close()

    def log_shell_output(self, victim_id, line):
        now = datetime.utcnow().isoformat()
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("INSERT INTO shell_activity (victim_id, command, output, timestamp, direction) VALUES (?,?,?,?,?)",
                  (victim_id, '', line, now, 'received'))
        conn.commit()
        conn.close()

    def log_agent_command(self, agent_id, command_id, cmd_type, payload, sent_at):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("INSERT INTO agent_activity (agent_id, command_id, command_type, command_payload, sent_at) VALUES (?,?,?,?,?)",
                  (agent_id, command_id, cmd_type, json.dumps(payload), sent_at))
        conn.commit()
        conn.close()

    def log_agent_result(self, command_id, output, error, completed_at):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("UPDATE agent_activity SET result_output=?, result_error=?, completed_at=? WHERE command_id=?",
                  (output, error, completed_at, command_id))
        conn.commit()
        conn.close()

    def get_shell_logs(self, victim_id=None, limit=100):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        if victim_id:
            c.execute("SELECT victim_id, command, output, timestamp, direction FROM shell_activity WHERE victim_id=? ORDER BY timestamp DESC LIMIT ?", (victim_id, limit))
        else:
            c.execute("SELECT victim_id, command, output, timestamp, direction FROM shell_activity ORDER BY timestamp DESC LIMIT ?", (limit,))
        rows = c.fetchall()
        conn.close()
        return [{'victim_id': r[0], 'command': r[1], 'output': r[2], 'timestamp': r[3], 'direction': r[4]} for r in rows]

    def get_agent_logs(self, agent_id=None, limit=100):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        if agent_id:
            c.execute("SELECT agent_id, command_type, command_payload, result_output, result_error, sent_at, completed_at FROM agent_activity WHERE agent_id=? ORDER BY sent_at DESC LIMIT ?", (agent_id, limit))
        else:
            c.execute("SELECT agent_id, command_type, command_payload, result_output, result_error, sent_at, completed_at FROM agent_activity ORDER BY sent_at DESC LIMIT ?", (limit,))
        rows = c.fetchall()
        conn.close()
        return [{'agent_id': r[0], 'type': r[1], 'payload': r[2], 'output': r[3], 'error': r[4], 'sent_at': r[5], 'completed_at': r[6]} for r in rows]

    def get_distinct_victim_ids(self):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("SELECT DISTINCT victim_id FROM shell_activity ORDER BY victim_id")
        rows = c.fetchall()
        conn.close()
        return [row[0] for row in rows]

    def get_distinct_agent_ids(self):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("SELECT DISTINCT agent_id FROM agent_activity ORDER BY agent_id")
        rows = c.fetchall()
        conn.close()
        return [row[0] for row in rows]