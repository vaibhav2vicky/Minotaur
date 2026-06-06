# server/models/victim.py
import sqlite3
from datetime import datetime
import os

DB_PATH = os.path.join(os.path.dirname(__file__), '../../database/c2.db')

class VictimManager:
    def __init__(self, socketio):
        self.socketio = socketio
        self.victims = {}
        self._init_db()

    def _init_db(self):
        os.makedirs(os.path.dirname(DB_PATH), exist_ok=True)
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute('''CREATE TABLE IF NOT EXISTS victims
                     (id TEXT PRIMARY KEY,
                      ip TEXT,
                      port INTEGER,
                      os_type TEXT,
                      hostname TEXT,
                      first_seen TEXT,
                      last_seen TEXT,
                      status TEXT)''')
        conn.commit()
        conn.close()

    def register_raw_victim(self, victim_id, ip, port, hostname):
        now = datetime.utcnow().isoformat()
        victim = {
            'id': victim_id,
            'ip': ip,
            'port': port,
            'os_type': 'unknown',
            'hostname': hostname,
            'first_seen': now,
            'last_seen': now,
            'status': 'active',
            'socket': None
        }
        self.victims[victim_id] = victim

        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute('''INSERT OR REPLACE INTO victims
                     (id, ip, port, os_type, hostname, first_seen, last_seen, status)
                     VALUES (?,?,?,?,?,?,?,?)''',
                  (victim_id, ip, port, 'unknown', hostname, now, now, 'active'))
        conn.commit()
        conn.close()

        self.socketio.emit('new_victim', self._serialize_victim(victim))
        return victim

    def update_victim_os(self, victim_id, os_type):
        if victim_id in self.victims:
            self.victims[victim_id]['os_type'] = os_type
            conn = sqlite3.connect(DB_PATH)
            c = conn.cursor()
            c.execute("UPDATE victims SET os_type=? WHERE id=?", (os_type, victim_id))
            conn.commit()
            conn.close()
            self.socketio.emit('victim_updated', self._serialize_victim(self.victims[victim_id]))
            return True
        return False

    def set_socket(self, victim_id, sock):
        if victim_id in self.victims:
            self.victims[victim_id]['socket'] = sock

    def get_socket(self, victim_id):
        victim = self.victims.get(victim_id)
        return victim['socket'] if victim else None

    def get_all_victims(self):
        # Return victims without the socket object for JSON serialization
        return [self._serialize_victim(v) for v in self.victims.values()]

    def get_by_os(self, os_type):
        # Return victims of given OS without socket
        return [self._serialize_victim(v) for v in self.victims.values() if v['os_type'].lower() == os_type.lower()]

    def remove_victim(self, victim_id):
        if victim_id in self.victims:
            del self.victims[victim_id]
            conn = sqlite3.connect(DB_PATH)
            c = conn.cursor()
            c.execute("UPDATE victims SET status='inactive' WHERE id=?", (victim_id,))
            conn.commit()
            conn.close()

    def _serialize_victim(self, victim):
        # Return a copy without the 'socket' field
        serialized = victim.copy()
        serialized.pop('socket', None)
        return serialized