# Minotaur C2 Framework Developer Guide

This guide is intended for developers working on the Minotaur C2 Framework codebase.

## Project Goals

- Maintain a dual-server architecture that separates the agent API from the dashboard.
- Provide a web dashboard for managing agents, commands, and listener ports.
- Build Go-based agent binaries dynamically for target platforms.
- Store operational data in a SQLite database.

## Repository Layout

- `agents/go/agent.go`
  - Agent template source code used by the build endpoint.
- `config/settings.py`
  - Shared configuration settings.
- `database/`
  - SQLite database file created at runtime.
- `logs/`
  - Application logs directory.
- `server/app.py`
  - Main application entrypoint.
  - Launches two services: agent API on port `5000` and dashboard on port `5001`.
- `server/handlers/`
  - Logic for agent lifecycle, commands, activity logging, and victims.
- `server/listeners/`
  - Port listener management and TCP listener implementation.
- `server/models/`
  - Data model helpers for victims and port events.
- `server/utils/`
  - Cryptography helpers and logging setup.
- `web/`
  - Dashboard templates and static assets.
- `requirements.txt`
  - Python dependency list.
- `run.sh`
  - Startup script for environment setup and launching the server.

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

- `server/app.py` starts two services:
  - `agent_app` on `0.0.0.0:5000`
  - `dashboard_app` on `127.0.0.1:5001`
- The dashboard is served from `web/templates/dashboard.html`.
- Agent binaries are stored under `web/static/agents/versions/`.
- Agent metadata and version info are persisted in `database/c2.db`.

## Useful Files and Components

### `server/app.py`

- `build_agent_impl(data)` compiles agents from `agents/go/agent.go`.
- Uses environment variables `GOOS`, `GOARCH`, and `CGO_ENABLED=0` for cross-compilation.
- Stores compiled binaries and maintains agent version records in SQLite.
- Exposes both agent and dashboard endpoints.

### `server/handlers/agent_handler.py`

- Tracks agent registrations and state.
- Handles incoming beacons, pending commands, and command results.

### `server/handlers/command_handler.py`

- Dispatches commands to individual agents or entire OS groups.
- Creates command objects for agents to consume on next beacon.

### `server/handlers/activity_logger.py`

- Stores shell and agent activity logs.
- Supports exporting logs via dashboard endpoints.

### `server/listeners/port_manager.py`

- Opens and stops listener ports.
- Manages active listening sockets and port event history.

### `agents/go/agent.go`

- Template source code for agent binaries.
- Contains placeholder tokens replaced by the build process.
- Supports optional RSA auth, beaconing, command polling, and file exfiltration.

## Extending the Codebase

### Add a new API route

- Determine whether the route belongs to the agent API or the dashboard.
- Add the route to `server/app.py` under the appropriate Flask app.
- If the route affects data, update handlers or models as needed.
- Add frontend support in `web/templates/dashboard.html` and `web/static/js/dashboard.js` if needed.

### Add a new dashboard feature

- Modify the template in `web/templates/dashboard.html`.
- Update `web/static/js/dashboard.js` to make API calls and handle UI events.
- Use the existing `SocketIO` live update pattern for real-time state changes.

### Add support for a new agent platform

- Confirm the platform name and architecture values are valid for Go compilation.
- Update the dashboard UI to present the new option.
- The build flow will automatically compile with `GOOS` and `GOARCH`.

## Database Notes

- The SQLite database is created at `database/c2.db` if it does not exist.
- `agent_versions` stores compiled binary metadata.
- `agent_current_version` tracks the active version per platform.
- Existing victim and port event tables are managed by the handlers and models.

## Testing and Debugging

- For quick debugging, use `debug_mode` when building an agent.
- Add log statements using the logger helpers in `server/utils/logger.py`.
- Confirm agent build output and endpoint behavior by testing the dashboard UI.

## Conventions

- Keep configuration values in `server/app.py` and `config/settings.py`.
- Use Flask JSON responses for API routes.
- Prefer small, isolated changes when modifying agent build or endpoint logic.
- Keep frontend state management inside `web/static/js/dashboard.js`.

## Future Improvements

- Add automated tests for API endpoints and handler logic.
- Add a CLI wrapper or management commands for common tasks.
- Introduce stricter configuration handling and environment variable support.
- Add authentication for dashboard access.
- Extend agent template with file upload/download support.
