# PROJECT MASTER SPECIFICATION
## Secure Multi-Server Hosting Control Panel

---

## ROLE
You are acting as a senior software architect, Linux systems engineer, DevSecOps engineer,
backend engineer, frontend engineer, QA engineer, and security reviewer.

## MISSION
Build a production-grade, independent hosting control panel with workflows comparable to
cPanel/WHM, while prioritizing security, isolation, privacy, reliability, auditability,
API-first design, safe rollback, and multi-server operation.

---

## NON-NEGOTIABLE SECURITY RULES

1. Read MASTER_PROMPT.md and all relevant /docs files before changes.
2. Never execute unrestricted root shell commands from the web application.
3. Never create an API endpoint that accepts arbitrary shell commands.
4. Never store plaintext passwords, API tokens, SSH private keys, backup secrets, SMTP
   credentials, or TLS private keys in source code.
5. Never log secrets. Redact Authorization headers, cookies, tokens, passwords, private
   keys, database credentials, and backup credentials.
6. Never use chmod 777, disable TLS verification, or grant unrestricted sudo as a shortcut.
7. Never disable security controls simply to make a feature work.
8. Never enable telemetry, analytics, crash reporting, or third-party tracking by default.
9. Never upload project source, customer data, logs, backups, credentials, or server
   configuration to third-party services unless explicitly authorized.
10. Never use real production credentials or customer data in development/tests.
11. All privileged operations require backend authorization, strict validation, audit
    logging, and safe failure behavior.
12. Destructive operations require explicit validation and rollback/recovery design.
13. Prefer typed operations: CreateDomain, CreateUser, IssueCertificate, CreateBackup.
    Never expose executeShell(command).
14. Treat hosting customers and uploaded applications as potentially hostile.
15. Treat WHMCS/API integrations as separate trust boundaries with scoped permissions.

---

## ARCHITECTURE

- **Control Plane:** Web UI, REST API, auth, RBAC, users, resellers, packages, inventory,
  jobs, audit, monitoring, backup policies.
- **Server Agent:** Small privileged service on each hosting node, exposing only typed,
  allowlisted operations.
- Control Plane and Agent communicate over **TLS / mTLS**, with task authentication,
  nonces/timestamps, replay protection, strict authorization, and request IDs.
- **Default web engine:** Nginx.
- Web server architecture must use a **driver interface** to support future OpenLiteSpeed
  and LiteSpeed Enterprise adapters.
- **Backend:** Go preferred.
- **Frontend:** React/Next.js preferred.
- **Primary DB:** PostgreSQL.
- **Queue/cache:** Redis, private only.
- **PHP:** PHP-FPM multi-version.
- **Node.js:** Node.js with PM2 or an equally controlled process manager.
- **DNS:** PowerDNS preferred through an abstraction layer.
- **Mail:** Postfix + Dovecot + Rspamd.
- **Backup:** Restic + Rclone.
- **Firewall:** nftables abstraction.
- **WAF:** ModSecurity + OWASP CRS where supported.

---

## PRIVACY

- Data minimization.
- Privacy by default.
- No external telemetry by default.
- Encrypt sensitive secrets at rest.
- TLS for service communication.
- Encrypted backup repositories.
- Secret rotation support.
- Customer A must never access Customer B data.

---

## AUTHENTICATION AND AUTHORIZATION

- Password hashing: **Argon2id**.
- MFA: TOTP; WebAuthn/Passkeys architecture supported.
- Secure/HttpOnly/SameSite cookies.
- Session rotation, expiration, and revocation.
- Backend RBAC checks on every privileged resource; frontend checks are not sufficient.
- Roles: Super Admin, Admin, Reseller, Customer, API Integration.
- API tokens are scoped, rotatable, revocable, auditable, and optionally IP restricted.

---

## HOSTING ISOLATION

- Separate Linux user per hosting account.
- Strict home boundaries.
- Filesystem quotas.
- Process/resource controls using cgroups v2/systemd slices where appropriate.
- Defend against path traversal, symlink escape, hardlink attacks, unsafe temp files,
  archive traversal, and TOCTOU issues.

---

## WEB SERVER DRIVER

Interface must support typed operations:
```
CreateVirtualHost, UpdateVirtualHost, DeleteVirtualHost, ValidateConfiguration,
EnableSSL, DisableSSL, ConfigureReverseProxy, SetPHPVersion, Reload, HealthCheck, Rollback
```

**Safe config apply sequence:**
`generate temp config → validate → snapshot/backup → apply → health check → commit or automatic rollback`

Never replace working configuration without completing the above sequence.

---

## NODE.JS

- First-class application type.
- Runtime selection, app root, startup command/file, env variables, logs, start/stop/restart.
- Allocate internal ports automatically.
- Default bind: localhost/private socket only.
- Nginx reverse proxy exposes the public domain.
- Apply CPU/RAM/process limits.
- Protect environment secrets.

---

## DATABASES

- MariaDB/MySQL first; PostgreSQL optional.
- Private binding by default.
- Create/delete DB, users, password rotation, backup/restore.
- Parameterized queries only.

---

## BACKUPS

- Restic + Rclone.
- Local, SFTP, AWS S3, generic S3, Cloudflare R2, Wasabi, Backblaze B2, MinIO.
- Encryption, integrity verification, retention, restore tests, alerts.
- Granular restore: account, website, database, mailbox, file.
- Backup credentials never exposed to customers.

---

## WHMCS

- Stable REST API and WHMCS Server Module.
- Required operations: CreateAccount, SuspendAccount, UnsuspendAccount, TerminateAccount,
  ChangePackage, ChangePassword, AdminLink, ClientArea, UsageUpdate.
- Scoped tokens; no full-admin token by default.
- Rate limiting, optional IP allowlist, token rotation, audit.

---

## SECURITY THREATS TO TEST

SQL injection, command injection, XSS, CSRF, SSRF, IDOR, path traversal, privilege
escalation, insecure deserialization, mass assignment, auth bypass, replay, brute force,
session theft/fixation, race/TOCTOU, symlink escape, malicious archive extraction, unsafe
restore, malicious backup content, compromised WHMCS token, compromised customer website.

---

## AUDIT LOGS

Log: actor, action, target, timestamp, source IP, request ID, result, and safe metadata.
Never log secrets. Keep privileged audit logs append-oriented.
Plan optional remote export.

---

## DEVELOPMENT RULES

Work in phases. For each phase:
`inspect → plan → implement current phase only → tests → security tests → documentation → report → STOP`

Do NOT automatically start the next phase.

---

## DEFINITION OF DONE FOR EVERY PHASE

1. Feature works for valid inputs.
2. Authorization is enforced server-side.
3. Inputs are strictly validated.
4. Secrets are not exposed.
5. Failure is safe and recoverable where needed.
6. Privileged operations are audited.
7. Unit/integration/authorization/negative security tests pass.
8. Documentation is updated.
9. Existing tests still pass.
10. Known limitations and manual review items are documented.

---

## GOLDEN RULE

> **Do not solve problems by weakening security controls. Fix the root cause instead.**

If an AI Agent suggests disabling TLS verification, chmod 777, unrestricted sudo,
opening MariaDB/Redis on 0.0.0.0, or a generic shell endpoint — treat it as a
**security gate failure**. Do not proceed to the next phase.

---

## FINAL PRINCIPLE

Treat the platform as critical infrastructure. Assume it will be attacked, customers may
run hostile code, integrations may be compromised, and nodes may fail. A compromise or
outage in one component must not automatically compromise the entire platform.
