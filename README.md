# Minotaur C2 Framework

A Command & Control (C2) framework for managing and communicating with deployed agents across multiple target systems. Minotaur provides a centralized server for agent deployment, command execution, listener management, and activity logging.

## Overview

Minotaur is a Flask-based C2 framework designed for authorized security testing and red team exercises. It includes:

- **Agent API**: agent registration, beacon polling, command results, file exfiltration, and shell instructions
- **Dashboard**: Web UI for managing agents, building binaries, and monitoring activity
- **Agent Builder**: Builds Go-based agent binaries for selected platforms
- **Database**: SQLite persistence for victims, port events, agent versions, and activity logs

## Features

- **Multi-platform agent build**: Compile Go agent binaries for configurable OS/ARCH pairs
- **Dual-server architecture**: agent API on `5000` and dashboard on `5001`
- **Real-time dashboard updates**: WebSocket-driven dashboard refreshes
- **Remote command execution**: send commands to individual agents or OS groups
- **Shell listener management**: open, stop, and inspect TCP listener ports
- **Agent versioning**: store compiled versions and select current platform versions
- **Optional RSA authentication**: generate RSA keypairs on build
- **Log export**: export shell or agent activity as text files

## Project Structure

```
c2_framework/
├── agents/
│   └── go/
│       ├── agent.go
│       ├── agent_unix.go
│       └── agent_windows.go
├── database/
│   └── c2.db
├── logs/
├── server/
│   ├── app.py
│   ├── handlers/
│   │   ├── activity_logger.py
│   │   ├── agent_handler.py
│   │   └── command_handler.py
│   ├── listeners/
│   │   ├── port_manager.py
│   │   └── tcp_listener.py
│   ├── models/
│   │   ├── port_event.py
│   │   └── victim.py
│   └── utils/
│       └── logger.py
├── web/
│   ├── static/
│   │   ├── agents/
│   │   │   └── versions/
│   │   ├── css/
│   │   │   └── dashboard.css
│   │   └── js/
│   │       └── dashboard.js
│   └── templates/
│       └── dashboard.html
├── requirements.txt
├── run.sh
├── LICENSE
└── README.md
```

## Requirements

### System Requirements

- Python 3.8 or higher
- Go 1.16 or higher
- Bash-compatible shell on Linux or macOS

### Python Dependencies

- `flask>=3.0.0`
- `werkzeug>=3.0.0`
- `flask-socketio>=5.3.6`
- `python-socketio>=5.11.0`
- `cryptography>=42.0.0`

See `requirements.txt` for the full list.

## Installation

### Quick Start

1. Clone or download the project:

   ```bash
   cd c2_framework
   ```

2. Run the startup script:

   ```bash
   chmod +x run.sh
   ./run.sh
   ```

The script will:

- verify Python 3 and Go are installed
- create a virtual environment if needed
- install required Python dependencies
- start the backend services

### Manual Installation

1. Create and activate a virtual environment:

   ```bash
   python3 -m venv .venv
   source .venv/bin/activate
   ```

2. Install dependencies:

   ```bash
   pip install -r requirements.txt
   ```

3. Start the server:

   ```bash
   python3 server/app.py
   ```

## Usage

### Starting the Server

```bash
./run.sh
```

The application launches two services:

- Agent API: `http://0.0.0.0:5000`
- Dashboard UI: `http://127.0.0.1:5001`

### Accessing the Dashboard

Open your browser to:

```text
http://127.0.0.1:5001
```

### Building Custom Agents

Use the dashboard to build agent binaries for your chosen platform and settings. Options include:

- C2 URL
- beacon delay (minimum 5 seconds)
- jitter
- user agent string
- target platform and architecture
- enable auth
- auto persistence
- debug mode

Built binaries are saved under `web/static/agents/versions/`.

### Command Execution

Once agents register and beacon in:

- view connected agents through the dashboard
- execute commands against a single agent or OS group
- issue shell reverse connections to listeners
- monitor command results and activity logs

## Configuration

### Server Settings

Edit `server/app.py` to adjust:

- `DB_PATH` for SQLite storage
- `AGENT_STORAGE` for compiled binaries
- Flask `SECRET_KEY`
- Socket.IO settings and CORS

### Agent Defaults

Modify `agents/go/agent.go` to update default agent settings such as:

- `ServerURL`
- `BeaconDelay`
- `Jitter`
- `UserAgent`
- `InsecureTLS`
- `AutoPersistence`
- `DebugMode`

## API Endpoints

### Agent API (`port 5000`)

- `POST /api/agent/authenticate`
- `POST /api/agent/register`
- `POST /api/agent/beacon`
- `POST /api/agent/result`
- `POST /api/agent/exfil`
- `POST /api/agent/shell`
- `POST /api/build_agent`
- `GET /api/agent/versions`
- `POST /api/agent/set_current_version`
- `POST /api/agent/delete_version`
- `GET /api/agents/list`
- `GET /static/agents/<path:filename>`

### Dashboard API (`port 5001`)

- `GET /`
- `GET /api/victims`
- `GET /api/agents`
- `POST /api/execute`
- `POST /api/set_os`
- `POST /api/agent/send_command`
- `POST /api/agent/delete`
- `POST /api/agents/clear_all`
- `POST /api/build_agent`
- `GET /api/agent/versions`
- `POST /api/agent/set_current_version`
- `POST /api/agent/delete_version`
- `GET /api/agents/list`
- `POST /api/open_port`
- `POST /api/stop_port`
- `POST /api/open_ports`
- `GET /api/port_events`
- `GET /api/active_ports`
- `GET /api/activity/shell`
- `GET /api/activity/agent`
- `GET /api/logs/victims`
- `GET /api/logs/agents`
- `GET /api/export/logs`

## Database

The app uses SQLite at `database/c2.db` and stores:

- victim records
- port event history
- activity logs
- compiled agent versions
- current platform version selections

## License

See [LICENSE](LICENSE) for license information.

## Disclaimer

This framework is provided for authorized security testing, red team exercises, and defensive research only. Unauthorized use against systems you do not own or do not have explicit permission to test is illegal. The authors assume no liability for misuse or damage caused by this software.
