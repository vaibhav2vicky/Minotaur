# Minotaur C2 Framework Developer Guide

This guide helps developers working on the Minotaur C2 Framework codebase.

## Project Goals

- Maintain a dual-server architecture separating the agent API from the dashboard.
- Provide a browser-based dashboard for managing agents, commands, and listener ports.
- Build Go-based agent binaries dynamically for configured platforms.
- Persist operational state in a SQLite database.

## Repository Layout

- `agents/go/agent.go`
  - Primary Go agent template used by the build process.
- `agents/go/agent_unix.go`
  - Unix-specific Go helper code for the generated agent.
- `agents/go/agent_windows.go`
  - Windows-specific Go helper code for the generated agent.
- `database/`
  - SQLite database file created at runtime.
- `logs/`
  - Application log directory.
- `server/app.py`
  - Main application entrypoint.
  - Runs agent API on port `5000` and dashboard on port `5001`.
- `server/handlers/`
  - Business logic for agent lifecycle, commands, and activity logging.
- `server/listeners/`
  - TCP listener and port management logic.
- `server/models/`
  - Victim and port event persistence helpers.
- `server/utils/logger.py`
  - Logging setup for the application.
- `web/`
  - Dashboard templates and static assets.
- `requirements.txt`
  - Python dependency list.
- `run.sh`
  - Startup script for environment validation and server launch.

## Development Setup

1. Clone the repository and change into the project root:

   ```bash
   git clone <repo-url>
   cd c2_framework
   ```

2. Ensure Python 3 and Go are installed.

3. Create and activate a virtual environment:

   ```bash
   python3 -m venv .venv
   source .venv/bin/activate
   ```

4. Install Python dependencies:

   ```bash
   pip install -r requirements.txt
   ```

5. Run the server locally:

   ```bash
   python3 server/app.py
   ```

   Or use the helper script:

   ```bash
   chmod +x run.sh
   ./run.sh
   ```

## Application Behavior

- `server/app.py` initializes two Flask apps and a Socket.IO dashboard.
- `agent_app` listens on `0.0.0.0:5000` for agent traffic.
- `dashboard_app` listens on `127.0.0.1:5001` for the dashboard UI.
- The dashboard uses `web/templates/dashboard.html` and `web/static/js/dashboard.js`.
- Compiled agent binaries are stored in `web/static/agents/versions/`.
- Agent version metadata is recorded in `database/c2.db`.

## Key Components

### `server/app.py`

- Contains the main request routing for both agent and dashboard APIs.
- Implements `build_agent_impl(data)` to generate Go sources, compile the agent, and store binaries.
- Uses `SocketIO` for dashboard client connections.
- Runs both services concurrently using `threading.Thread`.

### `server/handlers/agent_handler.py`

- Manages agent registration and state.
- Tracks pending commands and stores command results.
- Coordinates agent-specific activity updates.

### `server/handlers/command_handler.py`

- Sends commands to a single agent or all agents of a given OS type.
- Supports command dispatch through the dashboard execute flow.

### `server/handlers/activity_logger.py`

- Records shell activity and agent event history.
- Provides query APIs for logs, including export support.

### `server/listeners/port_manager.py`

- Starts and stops TCP listeners.
- Tracks active listeners and forwards new connections to `TCPListener`.

### `server/listeners/tcp_listener.py`

- Waits for inbound connections on managed ports.
- Emits dashboard socket events for new connections and shell output.

### `agents/go/agent.go`

- The Go template for generated agents.
- Contains placeholders injected by the build process.
- Supports optional auth, beaconing, command polling, and exfil.

## Extending the Codebase

### Add a new API route

- Add the new route to `server/app.py` under either `agent_app` or `dashboard_app`.
- Update the related handler or model if the route requires persistence or business logic.
- Add dashboard integration in `web/templates/dashboard.html` and `web/static/js/dashboard.js` as needed.

### Add a new dashboard feature

- Update the dashboard template and JavaScript to call the new endpoint.
- Leverage existing Socket.IO updates for real-time UI state changes.
- Keep dashboard behavior in `web/static/js/dashboard.js`.

### Add support for a new agent platform

- Verify the platform and architecture values are valid for Go cross-compilation.
- Add the new option to the dashboard UI if necessary.
- The existing builder will compile with `GOOS` and `GOARCH`.

## Database Notes

- The SQLite database is created at `database/c2.db` if it does not exist.
- `agent_versions` stores metadata for compiled binaries.
- `agent_current_version` stores the current version selection for each platform.
- Victim and port event tables are managed by the model and listener code.

## Testing and Debugging

- Use `debug_mode` when building an agent to disable release linker flags.
- Add logging in `server/utils/logger.py` and call it from `server/app.py` or handlers.
- Validate endpoint behavior using the dashboard or direct API requests.

## Conventions

- Keep shared configuration constants in `server/app.py`.
- Use Flask JSON responses for API routes.
- Keep business logic within handlers and models rather than inside route functions.
- Keep frontend code isolated in `web/static/js/dashboard.js`.

## Future Improvements

- Add automated tests for the API layer and handler logic.
- Add dashboard authentication and role-based access control.
- Improve configuration handling with environment variables or a settings file.
- Add file upload/download support to the agent template.
