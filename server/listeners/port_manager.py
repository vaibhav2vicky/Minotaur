# server/listeners/port_manager.py
import threading
from .tcp_listener import TCPListener

class PortManager:
    def __init__(self, victim_manager, port_event_manager, socketio):
        self.victim_mgr = victim_manager
        self.port_event_mgr = port_event_manager
        self.socketio = socketio
        self.listeners = {}  # port -> TCPListener thread
    
    def start_listener(self, port):
        if port in self.listeners and self.listeners[port].is_alive():
            return False
        listener = TCPListener(port, self.victim_mgr, self.port_event_mgr, self.socketio)
        listener.start()
        self.listeners[port] = listener
        return True
    
    def stop_listener(self, port):
        if port in self.listeners:
            self.listeners[port].stop()
            del self.listeners[port]
            return True
        return False
    
    def get_active_ports(self):
        return list(self.listeners.keys())