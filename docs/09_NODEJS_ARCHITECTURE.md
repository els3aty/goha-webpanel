# Node.js & PM2 Integration Architecture

This document describes the architectural implementation of Node.js support in the GohaHost Control Panel.

## 1. Overview
GohaHost provides first-class support for Node.js applications. Instead of running a single global PM2 instance as `root`, which poses a massive security risk, GohaHost dynamically spawns isolated PM2 daemons for each hosting user.

## 2. Component Flow

### Control Plane
- **Data Model (`models/nodejs.go`)**: Tracks `app_name`, `domain`, `app_path`, `startup_file`, `node_version`, and the assigned internal `port`.
- **API (`api/nodejs_handler.go`)**: 
  - Validates user ownership.
  - Automatically provisions a random internal port (e.g., 3000-4000) to avoid port collisions between tenants.
  - Dispatches an asynchronous `CreateNodeApp` task to the Server Agent via mTLS.

### Server Agent
- **Operations (`agent/internal/operations/nodejs.go`)**:
  - Validates paths to prevent traversal out of the user's home directory.
  - Generates the start command using `sudo -u <username> /usr/bin/env PORT=<port> pm2 start <startup_file> --name <app_name>`.
  - Executes `pm2 save` under the user's context.
- **Nginx Driver (`agent/internal/operations/vhost.go`)**:
  - Dynamically renders the Nginx configuration.
  - For Node.js apps, it uses `proxy_pass http://127.0.0.1:<port>` instead of `fastcgi_pass`.
  - Injects headers necessary for WebSocket support (`Upgrade` and `Connection`).

## 3. Security Boundary (Zero-Shell Policy)
The implementation strictly adheres to the Zero-Shell policy:
- **No `su -c`**: Using `su -c "command"` implicitly spawns a Bash shell which processes shell meta-characters, opening the door to Command Injection.
- **Direct Execution (`sudo -u`)**: We execute `sudo` directly using Go's `exec.CommandContext` with explicit array arguments. The arguments are passed to `/usr/bin/env` which sets the port, and then natively executes `pm2`.
- **User Isolation**: Because PM2 runs exclusively under the UID/GID of the specific hosting user, any arbitrary code executed by the Node.js application is strictly jailed to that user's `/home/<username>` directory.
