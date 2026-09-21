# Contributing Guide

Thank you for contributing to Hosting Panel. Please read this guide carefully before
making any changes.

---

## Golden Rule

> **Do not solve problems by weakening security controls. Fix the root cause instead.**

If you encounter an issue that seems to require disabling TLS verification, using
`chmod 777`, granting unrestricted sudo, opening database ports publicly, or adding a
generic shell execution endpoint — **stop and fix the root cause.**

---

## Development Environment Setup

### Requirements
- Linux (Ubuntu 22.04+ or Debian 12+ recommended for development)
- Go 1.22+
- Node.js 20+ with npm/pnpm
- PostgreSQL 15+
- Redis 7+
- Git

### Initial Setup
```bash
cp .env.example .env
# Edit .env with fake local-only values — NEVER use real credentials
./scripts/dev-setup.sh
```

---

## Environment & Secrets Policy

### CRITICAL — Never do this:
```bash
# ❌ FORBIDDEN - Real credentials
DB_PASSWORD=my_real_production_password
API_TOKEN=sk-live-abc123realtoken
```

### Always do this:
```bash
# ✅ CORRECT - Fake development values
DB_PASSWORD=development_only_fake_password
API_TOKEN=dev_token_only_not_real
```

### Rules:
1. **`.env` files with real values are NEVER committed to Git**
2. **Only `.env.example` exists in the repository (with fake placeholder values)**
3. **Production credentials stay in a secret store — never in source code**
4. **SSH private keys and TLS private keys are never added to the repository**
5. **Real customer data is never used in development — use fixtures**

---

## Development Workflow

### Phase-Based Development

This project follows a strict phase-based development model:

1. **Read first:** Always read `MASTER_PROMPT.md` and relevant `/docs` files before coding
2. **Work in scope:** Only implement the current phase — do not start future phases
3. **Test as you go:** Unit, integration, authorization, and security tests for each feature
4. **Gate before moving:** Each phase has an acceptance gate — verify all items before proceeding

### For Each Feature / Change:

1. Inspect the relevant code and docs
2. Identify trust boundaries and permissions affected
3. Write a brief implementation plan
4. Implement with:
   - Strict input validation
   - Least privilege
   - Backend authorization (never trust frontend-only checks)
   - Parameterized DB queries
   - Safe process execution (argument arrays, never shell concatenation)
   - Secret redaction in logs
   - Structured audit events for privileged actions
   - Safe failure handling and rollback for partial failures
5. Write tests (including negative/security tests, not just happy path)
6. Update documentation

---

## Code Standards

### Security Requirements (All Code)

- **No shell string concatenation:** Use exec with fixed binaries and argument arrays
  ```go
  // ❌ FORBIDDEN
  exec.Command("sh", "-c", "nginx -t " + userInput)

  // ✅ CORRECT
  exec.Command("/usr/sbin/nginx", "-t", "-c", configPath)
  ```

- **No secret logging:**
  ```go
  // ❌ FORBIDDEN
  log.Printf("Connecting with token: %s", token)

  // ✅ CORRECT
  log.Printf("Connecting to database host: %s", host)
  ```

- **No chmod 777:**
  ```go
  // ❌ FORBIDDEN
  os.Chmod(path, 0777)

  // ✅ CORRECT - use minimum required permissions
  os.Chmod(path, 0750)
  ```

- **Parameterized queries only:**
  ```go
  // ❌ FORBIDDEN
  db.Query("SELECT * FROM users WHERE name = '" + name + "'")

  // ✅ CORRECT
  db.Query("SELECT * FROM users WHERE name = $1", name)
  ```

### Go Standards
- Use Go modules with `go.sum` committed
- Format with `gofmt` / `goimports`
- Lint with `golangci-lint`
- Test with `go test ./...`

### Frontend Standards
- No secrets in frontend code or bundled assets
- CSRF protection on all state-changing requests
- Content-Security-Policy headers

---

## Testing Requirements

Every phase must include:

| Test Type | Required |
|-----------|----------|
| Unit tests | ✅ Yes |
| Integration tests | ✅ Yes |
| Authorization tests | ✅ Yes |
| Negative security tests | ✅ Yes |
| Happy path only | ❌ Not sufficient |

**Examples of required negative tests:**
- Customer trying to access another customer's resources (IDOR)
- Reseller trying to become SuperAdmin
- Malicious filename with `../` path traversal
- SQL injection in all user inputs
- Expired/forged authentication tokens

---

## Commit Guidelines

```
type(scope): short description

Longer description if needed.

Security: note any security implications
Tests: list test coverage added
Breaking: note any breaking changes
```

**Types:** `feat`, `fix`, `security`, `docs`, `test`, `refactor`, `chore`

**Examples:**
```
feat(auth): add Argon2id password hashing

security(agent): enforce mTLS certificate validation

fix(filemanager): prevent path traversal via symlink
Tests: added TestSymlinkEscape, TestPathTraversal
```

---

## Log Sanitization

Before sharing any logs or output with others (including AI Agents), sanitize:
- `Authorization:` headers
- `Cookie:` and `Set-Cookie:` headers
- Tokens, passwords, private keys
- Database connection strings
- Backup credentials

Replace real domains with `example.com` and real IPs with `192.0.2.1`.

---

## Reporting Security Issues

See [SECURITY.md](SECURITY.md) for the full vulnerability reporting process.
Do NOT open public issues for security vulnerabilities.
