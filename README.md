# Minotaur C2 Framework

A Command & Control (C2) framework for managing and communicating with deployed agents across multiple target systems. Minotaur provides a centralized server for agent deployment, command execution, and activity monitoring.

## Overview

Minotaur is a Flask-based C2 framework designed for authorized security testing and red team operations. It includes:

- **Agent API**: Handles agent registration, beaconing, command results, and file exfiltration
- **Dashboard**: Web-based UI for managing agents, commands, and port listeners
- **Agent Builder**: Generates Go-based agent binaries for target platforms
- **Database**: SQLite persistence layer for agents, logs, and version metadata

## Features

- **Multi-platform agent build**: Compile agent binaries for Windows and Linux targets
- **Dual-server architecture**: Separate agent API and dashboard services
- **Real-time dashboard**: Interactive interface with WebSocket updates
- **Remote command execution**: Send commands to individual agents or OS groups
- **Activity logging**: Track shell and agent activity
- **Port management**: Start and stop listener ports from the dashboard
- **Agent versioning**: Manage compiled agent binaries and current versions
- **Optional RSA authentication**: Secure agent communications with RSA keys

## Project Structure

```
c2_framework/
├── agents/
│   └── go/
│       └── agent.go
├── config/
│   └── settings.py
├── database/
│   └── c2.db
├── logs/
├── server/
│   ├── app.py
│   ├── handlers/
│   │   ├── activity_logger.py
│   │   ├── agent_handler.py
│   │   ├── command_handler.py
│   │   └── victim_handler.py
│   ├── listeners/
│   │   ├── port_manager.py
│   │   └── tcp_listener.py
│   ├── models/
│   │   ├── port_event.py
│   │   └── victim.py
│   └── utils/
│       ├── crypto.py
│       └── logger.py
├── web/
│   ├── static/
│   │   ├── agents/
│   │   │   └── versions/
│   │   │       ├── linux_amd64/
│   │   │       ├── windows_amd64/
│   │   │       └── windows_arm64/
│   │   ├── css/
│   │   │   └── dashboard.css
│   │   └── js/
│   │       └── dashboard.js
│   └── templates/
│       ├── dashboard.html
│       └── index.html
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

See [requirements.txt](requirements.txt) for the complete list.

## Installation

### Quick Start

1. **Clone or download the project**:

   ```bash
   cd c2_framework
   ```

2. **Run the startup script**:

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

1. **Create and activate a virtual environment**:

   ```bash
   python3 -m venv .venv
   source .venv/bin/activate
   ```

2. **Install dependencies**:

   ```bash
   pip install -r requirements.txt
   ```

3. **Start the server**:

   ```bash
   python3 server/app.py
   ```

## Usage

### Starting the Server

```bash
./run.sh
```

The application starts two services:

- Agent API: `http://0.0.0.0:5000`
- Dashboard UI: `http://127.0.0.1:5001`

### Accessing the Dashboard

Open your browser and navigate to:

```
http://127.0.0.1:5001
```

### Building Custom Agents

Use the dashboard to build agent binaries for selected target platforms. Configure options such as:

- C2 URL
- beacon delay (minimum 5 seconds)
- jitter
- user agent
- platform and architecture
- enable auth
- auto persistence
- debug mode

Then download the compiled binary from the dashboard.

### Command Execution

Once agents connect:

- view connected agents in the dashboard
- execute commands against single agents or OS groups
- monitor responses and activity
- review historical logs

## Configuration

### Server Settings

Edit `server/app.py` to adjust:

- `DB_PATH` for database storage
- `AGENT_STORAGE` for compiled binaries
- Flask `SECRET_KEY`
- Socket.IO settings and CORS

### Agent Defaults

Edit `agents/go/agent.go` to change default agent values such as:

- `ServerURL`
- `BeaconDelay`
- `Jitter`
- `UserAgent`
- `InsecureTLS`
- `AutoPersistence`
- `DebugMode`

### Environment Variables

Optional environment variables:

- `FLASK_ENV`
- `FLASK_DEBUG`

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

The app uses SQLite at `database/c2.db` and includes tables for:

- `victims`
- `port_events`
- `activity_logs`
- `agent_versions`
- `agent_current_version`

## Logging

Logs are stored in the `logs/` directory and include:

- startup and shutdown events
- agent registration and beacon activity
- command execution history
- errors and warnings

## Security Considerations

⚠️ **Important**: Use this framework only for authorized testing and Education purpose.

- change the Flask `SECRET_KEY` before production
- use HTTPS in production deployments
- enable RSA authentication for agent communications
- implement access controls and network segmentation
- monitor activity and log operations
- use beacon delay and jitter to reduce detection risk

## License

See [LICENSE](LICENSE) for license information.

## Disclaimer

This framework is provided for authorized security testing, red team exercises, and defensive research only. Unauthorized use against systems you do not own or do not have explicit permission to test is illegal. The authors assume no liability for misuse or damage caused by this software.
