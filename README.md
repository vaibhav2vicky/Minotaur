# Minotaur C2 Framework

A Python-based command and control (C2) framework for managing reverse TCP shells and HTTP beaconing agents. The project includes a Flask web dashboard, reverse shell listener management, HTTP agent registration/beaconing, command dispatch, logging, and binary build support for Go-based agents.

> Warning: This repository is not production hardened. It currently exposes APIs without authentication, uses a hardcoded Flask secret key, and enables debug behavior. Use it only in trusted test environments.

## Features

- Web dashboard for managing reverse shells and agents
- TCP listener management for reverse shell connections
- Agent registration, beacon, command dispatch, result reporting, and file exfiltration
- Activity logging for shell commands and agent actions
- Dynamic Go agent build endpoint with versioned binary storage
- Export shell and agent logs as text files

## Repository Structure

- `server/`
  - `app.py` - Flask application and API entrypoint
  - `agent_template.go` - Go agent source template used for dynamic builds
  - `handlers/` - business logic for commands, agents, activity logs, and victim state
  - `listeners/` - TCP listener implementation for reverse shells
  - `models/` - SQLite-backed data models for victims and port events
  - `utils/` - logger helper and shared utilities
- `web/`
  - `templates/` - Flask HTML templates for the dashboard
  - `static/` - dashboard assets and generated agent binaries
- `database/` - SQLite database storage
- `logs/` - application logs produced by `server/utils/logger.py`
- `requirements.txt` - Python dependency list
- `run.sh` - bootstrap script for virtual environment setup and server startup

## Prerequisites

- Linux environment (the project was developed on Linux)
- Python 3
- `go` toolchain (required only for building Go agents via the build endpoint)

## Setup

```bash
cd /home/orca/project/c2_framework
./run.sh
```

Alternatively, manually:

```bash
cd /home/orca/project/c2_framework
python3 -m venv .venv
source .venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt
python server/app.py
```

The server listens on `http://0.0.0.0:5000` by default.

## Usage

Open the dashboard in a browser at:

```text
http://127.0.0.1:5000
```

### TCP Shells

- Open a listener port from the dashboard
- Connect a reverse shell from a target host to the open port
- The dashboard stores victim metadata and shows shell output
- Commands can be sent to individual victims or all victims by OS type

### HTTP Agents

- Agents register over HTTP and periodically beacon for commands
- The UI shows active agents, status, and last beacon time
- Commands can be queued to agents using the dashboard
- Agents support actions like shell execution, file exfiltration, lateral movement, persistence, and more

### Activity Logs

- Shell command and output logs are persisted in SQLite
- Agent command and result logs are persisted separately
- Logs can be filtered and exported via the dashboard

### Build & Update

- Use `Build & Update` to compile a Go agent from `server/agent_template.go`
- Built binaries are stored under `web/static/agents/versions/<platform>/`
- The dashboard exposes download URLs for generated agent binaries

## Go Agent Template

The Go agent source template is located at:

- `server/agent_template.go`

It includes configuration placeholders like:

- `{{ .C2URL }}`
- `{{ .BeaconDelay }}`
- `{{ .Jitter }}`
- `{{ .UserAgent }}`
- `{{ .InsecureTLS }}`
- `{{ .AutoPersistence }}`
- `{{ .DebugMode }}`
- `{{ .AgentVersion }}`

The generated agent registers itself, beacons for commands, executes received commands, and reports results back to the C2 server.

## Important Notes

- `config/settings.py` exists but is currently unused and empty.
- `app.py` currently uses `app.config['SECRET_KEY'] = 'change-this-in-production'`
- Flask is run with `debug=True` when executed directly, which should be disabled in any real deployment.
- `SocketIO` is configured with `cors_allowed_origins="*"`, which is insecure for exposed services.
- Victim state is partially stored in memory (`VictimManager.victims`), so active session state may be lost on restart.
- SQLite usage is not centralized, and concurrent writes may risk locking issues under load.

## Recommended Improvements

If you continue developing this project, consider:

- Adding authentication and authorization for dashboard/API access
- Moving configuration into environment variables or a dedicated settings module
- Refactoring routes into Flask blueprints and using an app factory
- Centralizing SQLite access in a database helper or using SQLAlchemy
- Adding proper error handling and structured logging
- Avoiding storing large exfiltrated files directly in SQLite
- Implementing secure header/CORS policies and HTTPS support

## Useful Commands

Start the server:

```bash
./run.sh
```

Manually rebuild agent binaries:

```bash
go version
# Build is performed via the dashboard; go is required for that endpoint
```

Inspect the SQLite database:

```bash
sqlite3 database/c2.db
```

## Security Disclaimer

This project is intended for research and testing only. Do not deploy it on public networks without proper hardening. The current implementation is not secure for production use and may leak sensitive command or agent data.
