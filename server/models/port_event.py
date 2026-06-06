# server/models/port_event.py
import sqlite3
import os
from datetime import datetime

DB_PATH = os.path.join(os.path.dirname(__file__), '../../database/c2.db')

class PortEventManager:
    def __init__(self):
        self._init_db()
    
    def _init_db(self):
        os.makedirs(os.path.dirname(DB_PATH), exist_ok=True)
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute('''CREATE TABLE IF NOT EXISTS port_events
                     (id INTEGER PRIMARY KEY AUTOINCREMENT,
                      listening_port INTEGER,
                      connected_ip TEXT,
                      timestamp TEXT)''')
        conn.commit()
        conn.close()
    
    def add_event(self, listening_port, connected_ip):
        timestamp = datetime.utcnow().isoformat()
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("INSERT INTO port_events (listening_port, connected_ip, timestamp) VALUES (?,?,?)",
                  (listening_port, connected_ip, timestamp))
        conn.commit()
        conn.close()
        return {'port': listening_port, 'ip': connected_ip, 'timestamp': timestamp}
    
    def get_recent_events(self, limit=50):
        conn = sqlite3.connect(DB_PATH)
        c = conn.cursor()
        c.execute("SELECT listening_port, connected_ip, timestamp FROM port_events ORDER BY id DESC LIMIT ?", (limit,))
        rows = c.fetchall()
        conn.close()
        return [{'port': row[0], 'ip': row[1], 'timestamp': row[2]} for row in rows]