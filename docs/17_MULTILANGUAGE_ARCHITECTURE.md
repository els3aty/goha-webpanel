# Multi-Language Architecture (Phase 17)

This document details how GohaHost provisions Nginx for multiple application runtimes natively.

## 1. Context-Aware Templating
Instead of executing brittle `sed` or `echo` commands in bash to generate Nginx configurations, the Server Agent uses Go's `text/template` standard library. 

This provides a powerful, memory-safe way to dynamically generate Virtual Hosts.

## 2. Supported Runtimes
When the Control Plane creates a `VirtualHost`, it passes a `runtime_type` and optionally a `runtime_port` to the Agent.

The Agent's Go template renders the Nginx configuration differently based on the runtime:

### PHP-FPM (`runtime_type: php`)
- Uses `fastcgi_pass` to route traffic to a Unix socket.
- The socket is uniquely named (e.g., `php8.2-fpm-username.sock`) to prevent privilege escalation between users.
- A dedicated FPM pool is generated for the user.

### Node.js & Python (`runtime_type: nodejs` or `python`)
- Uses `proxy_pass http://127.0.0.1:<port>;`.
- Adds specific HTTP headers (`Upgrade`, `Connection 'upgrade'`) to support WebSockets, which are extremely common in modern Node and Python (ASGI) applications.
- No FPM pool is generated.

## 3. Security (Zero-Shell)
Because the templating logic is executed inside the compiled Go binary:
- No shell (`sh -c`) is ever invoked to parse the config.
- A malicious domain name like `test.com; rm -rf /` would simply result in an invalid Nginx config file being safely written to disk (and then rejected by Nginx during reload), rather than executing the payload on the host OS.
- File writing is handled natively via `os.WriteFile()`.
