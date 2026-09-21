# Security Policy

## Supported Versions

This project is currently in active development. Security fixes are applied to the
latest version only.

---

## Reporting a Vulnerability

**Do NOT open a public GitHub issue for security vulnerabilities.**

If you discover a security vulnerability, please report it privately:

1. **Email:** security@hosting-panel.local *(replace with your actual address)*
2. **Include in your report:**
   - Description of the vulnerability
   - Steps to reproduce
   - Affected component(s)
   - Potential impact
   - Suggested remediation (if any)

We will acknowledge receipt within 48 hours and provide an estimated resolution timeline.

---

## Security Design Principles

This project is built on the following non-negotiable security principles:

### No Generic Shell Execution
The web application never executes arbitrary shell commands. All privileged operations
use typed, explicitly defined operations with strict input validation.

### Least Privilege
Every component runs with the minimum permissions required. No component has
unrestricted root/sudo access from the web application.

### Defense in Depth
Security is layered: OS isolation, network isolation, application-level RBAC,
audit logging, and monitoring.

### Secrets Management
- Secrets are never stored in source code or logs
- `.env` files with real credentials are never committed to version control
- Only `.env.example` with fake placeholder values exists in the repository
- All credentials are stored in a secret store and rotated regularly

### Customer Isolation
- Separate Linux user per hosting account
- Filesystem quotas enforced at OS level
- Process/resource limits via cgroups v2
- Customer A cannot access Customer B's data under any circumstances

### Audit Trail
Every privileged operation logs: actor, action, target, timestamp, source IP,
request ID, and result. Secrets are never logged.

---

## Development Security Policy

### Forbidden in Development
- Real production credentials or API keys
- Real customer data (use fixtures with fake data only)
- Real SSH private keys or TLS private keys
- Real WHMCS database exports
- Real backup files from production

### Required in Development
- Use `.env.example` as a template; create `.env` with fake local-only values
- Replace real domains with `example.com` or `*.test`
- Replace real IPs with `192.0.2.0/24` or `2001:db8::/32` (documentation ranges)
- Sanitize logs: remove Authorization, Cookie, Set-Cookie, tokens, passwords

### Dependency Policy
- All dependencies must be pinned to specific versions
- Lockfiles (go.sum, package-lock.json, etc.) must be committed
- New dependencies require documented justification and license review
- No install-time scripts from untrusted sources (no curl | bash)

---

## Known Security Boundaries

| Boundary | Trust Level | Notes |
|----------|------------|-------|
| Customer web application | **Hostile** | May run arbitrary code |
| WHMCS integration | **Limited** | Scoped token, separate trust boundary |
| Server Agent | **High** | mTLS authenticated, typed operations only |
| Control Plane API | **Medium** | RBAC + rate limiting + audit |
| Admin session | **High** | MFA required, session revocation supported |

---

## Security Checklist (Pre-Production)

See [docs/SECURITY.md](docs/SECURITY.md) and the Phase 20 release gate in
[MASTER_PROMPT.md](MASTER_PROMPT.md) for the complete pre-production security checklist.
