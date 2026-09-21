# Auto-SSL Architecture (Phase 20)

This document explains the Zero-Shell Certbot integration for GohaHost.

## 1. Goal
Provide users with an automated, 1-click Let's Encrypt SSL provisioning system that natively configures Nginx.

## 2. The Problem with Certbot `--nginx`
Certbot has an `--nginx` plugin that automatically reads, parses, and edits Nginx configuration files.
However, allowing a Python script (Certbot) to blindly modify our Go-generated Nginx templates is extremely risky. It can lead to:
- Broken configuration blocks if Certbot misinterprets our templating structure.
- Duplicate SSL directives during updates.
- Loss of Control Plane idempotency (the Control Plane assumes the file matches the Go template).

## 3. The Webroot Pattern
To maintain absolute control, the Agent uses Certbot purely for validation and certificate generation, but **not** for Nginx configuration:
1. **Certbot Execution**: 
   The Agent securely executes `certbot certonly --webroot -w <docroot> -d <domain> ...`
   This uses the existing port 80 HTTP server to validate the ACME challenge.
2. **Success Validation**:
   If Certbot succeeds, the certificates are saved to `/etc/letsencrypt/live/<domain>/`.
3. **Native Templating**:
   The Agent then re-runs its internal `CreateVirtualHost` templating logic with a flag `SSLEnabled = true`.
   The Go `text/template` injects the `listen 443 ssl;` block and the exact certificate paths.
4. **Reload**:
   The Agent natively writes the file and reloads Nginx.

## 4. Security (Zero-Shell)
- Certbot is executed via `exec.CommandContext`, passing variables strictly as arguments. No `sh -c` is used, so domain names like `test.com; rm -rf /` cannot cause shell injection.
- The resulting Nginx config is written directly via Go's `os.WriteFile()`.
