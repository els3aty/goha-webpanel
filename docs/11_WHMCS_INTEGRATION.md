# WHMCS Integration API (Phase 11)

## Architecture Overview
GohaHost allows billing software like WHMCS to provision and manage hosting accounts automatically without sacrificing the strict security boundaries of the Control Plane.

## Authentication Model (Scoped Tokens)
Standard JWTs used by human administrators provide unrestricted access across the panel. If WHMCS were compromised, a stolen global JWT would grant full control over all servers.

To prevent this, GohaHost implements **WHMCS Scoped Tokens**:
- Stored as `SHA-256` hashes in the database (preventing theft if the DB is dumped).
- Can only hit endpoints under the `/api/whmcs/*` group.
- Can optionally enforce strict **IP Allowlisting**, so the token will *only* work if the HTTP request originates from the WHMCS server's fixed IP address.

## PHP Module
A dedicated PHP module is deployed on the WHMCS server (`modules/servers/gohahost`). It uses native PHP `cURL` (zero `exec` or `shell_exec`) to dispatch REST calls securely to the GohaHost API.
