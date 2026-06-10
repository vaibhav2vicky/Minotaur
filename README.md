# Minotaur C2 Framework

A Command & Control (C2) framework for managing and communicating with deployed agents across multiple target systems. Minotaur provides a centralized server for agent deployment, command execution, and activity monitoring.

## Overview

Minotaur is a full-featured C2 framework designed for authorized security testing and red team operations. It consists of:

- **Server**: Python-based Flask application with WebSocket support
- **Dashboard**: Web-based UI for managing agents and executing commands
- **Agents**: Go-based agents deployed on target systems
- **Database**: SQLite-based persistence layer for agent and activity tracking

## Features

- **Multi-Platform Agent Deployment**: Build and deploy agents for Windows, Linux, and other architectures
- **Real-Time Communication**: WebSocket-based agent communication with the C2 server
- **Web Dashboard**: Intuitive interface for managing compromised systems
- **Command Execution**: Execute commands remotely on compromised systems
- **Activity Logging**: Comprehensive logging of all C2 operations and agent activities
- **Agent Configuration**: Customize beacon delays, jitter, user agents, and encryption settings
- **Port Management**: Monitor and manage network ports for listener deployment
- **Cryptographic Support**: RSA-based authentication and secure communications
- **Victim Management**: Track and manage connected agents with detailed system information

## Project Structure

```
c2_framework/
├── agents/
│   └── go/                          # Go-based agent source code
│       └── main.go
├── config/
│   └── settings.py                  # Configuration settings
├── database/
│   └── c2.db                        # SQLite database (generated at runtime)
├── logs/                            # Application logs directory
├── server/
│   ├── app.py                       # Main Flask application
│   ├── agent_template.go            # Agent template for building custom agents
│   ├── handlers/
│   │   ├── activity_logger.py       # Logs all C2 operations
│   │   ├── agent_handler.py         # Manages agent lifecycle
│   │   ├── command_handler.py       # Executes commands on agents
│   │   └── victim_handler.py        # Manages compromised systems
│   ├── listeners/
│   │   ├── port_manager.py          # Network port management
│   │   └── tcp_listener.py          # TCP listener for agent connections
│   ├── models/
│   │   ├── port_event.py            # Port event data model
│   │   └── victim.py                # Victim/agent data model
│   └── utils/
│       ├── crypto.py                # Cryptographic utilities
│       └── logger.py                # Logging utilities
├── web/
│   ├── static/
│   │   ├── agents/                  # Agent binary storage
│   │   │   └── versions/            # Versioned agents by platform
│   │   │       ├── linux_amd64/
│   │   │       ├── windows_amd64/
│   │   │       └── windows_arm64/
│   │   ├── css/
│   │   │   └── dashboard.css        # Dashboard styling
│   │   └── js/
│   │       └── dashboard.js         # Dashboard client-side logic
│   └── templates/
│       ├── dashboard.html           # Main dashboard interface
│       └── index.html               # Welcome page
├── requirements.txt                 # Python dependencies
├── run.sh                           # Startup script
├── LICENSE                          # License information
└── README.md                        # This file
```

## Requirements

### System Requirements

- Python 3.7 or higher
- Go 1.16 or higher (for building agents)
- Unix/Linux or macOS system (run.sh uses bash)

### Python Dependencies

- **Flask** ≥ 3.0.0: Web framework
- **Werkzeug** ≥ 3.0.0: WSGI utilities
- **Flask-SocketIO** ≥ 5.3.6: WebSocket support
- **python-socketio** ≥ 5.11.0: Socket.IO client library
- **cryptography** ≥ 42.0.0: Cryptographic operations

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

- Check for Python 3
- Create a virtual environment if needed
- Install required dependencies
- Start the server

### Manual Installation

1. **Create a virtual environment**:

   ```bash
   python3 -m venv .venv
   source .venv/bin/activate  # On Windows: .venv\Scripts\activate
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

The server will start on `http://127.0.0.1:5000` by default.

### Accessing the Dashboard

Open your web browser and navigate to:

```
http://127.0.0.1:5000
```

### Building Custom Agents

Use the dashboard to build custom agents for specific target platforms:

1. Navigate to the "Build Agent" section
2. Configure agent options:
   - **C2 URL**: Server address for agent callback
   - **Beacon Delay**: Time between check-ins (minimum 5 seconds)
   - **Jitter**: Random delay variance for beacon timing
   - **User Agent**: HTTP user agent string
   - **Platform**: Target OS (Windows, Linux, etc.) and architecture (amd64, arm64, etc.)
   - **Enable Auth**: Use RSA encryption for agent communications
   - **Auto Persistence**: Automatic persistence mechanisms
   - **Debug Mode**: Verbose logging on the agent

3. Download the compiled agent binary

### Command Execution

Once agents connect to the server:

1. **View Connected Agents**: Dashboard displays all active agents with system information
2. **Execute Commands**: Send commands to individual agents or groups by OS type
3. **Monitor Activity**: View real-time command execution and responses
4. **Track History**: Access historical logs of all C2 operations

## Configuration

### Server Configuration

Edit `server/app.py` to modify:

- **Database Path**: Change `DB_PATH` for custom database location
- **Agent Storage**: Modify `AGENT_STORAGE` for agent binary storage
- **Secret Key**: Update Flask secret key for production use
- **Socket.IO Settings**: Configure CORS and async mode

### Agent Configuration

Edit `agents/go/main.go` to modify default agent settings:

- **ServerURL**: Default C2 server address
- **BeaconDelay**: Default check-in interval
- **Jitter**: Default beacon variance
- **UserAgent**: Default HTTP user agent

### Environment Variables

Configure the following if needed:

- `FLASK_ENV`: Set to 'development' or 'production'
- `FLASK_DEBUG`: Enable/disable debug mode

## API Endpoints

### Dashboard Application

- `GET /`: Dashboard home page
- `GET /agents`: List all connected agents
- `POST /build-agent`: Build a custom agent
- WebSocket: Real-time agent communication

### Agent Application

- `POST /register`: Agent registration endpoint
- `GET /command`: Agent polls for commands
- `POST /response`: Agent posts command responses

## Database

The application uses SQLite for persistence. The database is automatically created at `database/c2.db` and includes tables for:

- **victims**: Connected agents with system information
- **port_events**: Network port activity tracking
- **activity_logs**: Historical record of all C2 operations

## Logging

Application logs are stored in the `logs/` directory. Log files include:

- Server startup and shutdown events
- Agent connections and disconnections
- Command execution history
- Error and warning messages

## Security Considerations

⚠️ **Important**: This framework is intended for authorized security testing only.

- Change the Flask `SECRET_KEY` in production
- Use HTTPS with valid certificates for production deployments
- Enable RSA encryption for agent communications when possible
- Implement network segmentation and access controls
- Monitor and log all C2 activities
- Use strong beacon delays to avoid detection
- Consider using jitter to randomize communication patterns

## License

See [LICENSE](LICENSE) for license information.

## Disclaimer

This framework is provided for authorized security testing, red team exercises, and defensive research only. Unauthorized use of this framework against systems you do not own or have explicit permission to test is illegal. The authors assume no liability for misuse or damage caused by this software.

## Contributing

Contributions are welcome. Please ensure all changes are well-documented and tested.

## Support

For issues, questions, or contributions, please refer to the project documentation or contact the maintainers.
