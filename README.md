# Hosting Panel

A production-grade, secure, multi-server hosting control panel — a modern alternative to
cPanel/WHM built with security, isolation, and reliability as first-class requirements.

> **Status:** Phase 0 — Repository Foundation ✅

---

## Overview

This project builds an independent hosting control panel supporting:

- Multi-server architecture (Control Plane + Agents)
- PHP (multi-version, PHP-FPM)
- Node.js with PM2
- MariaDB/MySQL provisioning
- DNS (PowerDNS)
- Mail (Postfix + Dovecot + Rspamd)
- SSL/TLS (ACME / Let's Encrypt)
- Backup (Restic + Rclone → S3/SFTP)
- WHMCS integration
- File Manager with security sandbox
- Reseller/Package management
- Security Center + Monitoring

---

## Architecture Summary

```
┌─────────────────────────────────┐
│         Control Plane           │
│  Web UI + REST API + Auth/RBAC  │
│  PostgreSQL + Redis + Audit Log │
└────────────────┬────────────────┘
                 │ mTLS
    ┌────────────┴────────────┐
    ▼                         ▼
┌──────────┐           ┌──────────┐
│  Agent   │           │  Agent   │
│ Server 1 │           │ Server 2 │
└──────────┘           └──────────┘
```

- **Backend:** Go
- **Frontend:** React / Next.js
- **Primary DB:** PostgreSQL
- **Cache/Queue:** Redis (private)
- **Web Server:** Nginx (driver-based, extensible)

---

## Development Phases

| Phase | Name | Status |
|-------|------|--------|
| 0 | Repository Foundation | ✅ Done |
| 1 | Architecture + Threat Model | ⏳ Next |
| 2 | Control Plane + Auth + RBAC | 🔒 Locked |
| 3 | Server Agent + mTLS | 🔒 Locked |
| 4 | Linux Users + Isolation | 🔒 Locked |
| 5 | Web Server Driver + Nginx | 🔒 Locked |
| 6 | Multi-PHP + PHP-FPM | 🔒 Locked |
| 7 | SSL/ACME | 🔒 Locked |
| 8 | MariaDB/MySQL | 🔒 Locked |
| 9 | Node.js + PM2 | 🔒 Locked |
| 10 | Backup Engine | 🔒 Locked |
| 11 | WHMCS Integration | 🔒 Locked |
| 12 | DNS + PowerDNS | 🔒 Locked |
| 13 | Mail Stack | 🔒 Locked |
| 14 | File Manager | 🔒 Locked |
| 15 | Reseller + Packages | 🔒 Locked |
| 16 | Security Center + Monitoring | 🔒 Locked |
| 17 | OpenLiteSpeed / LiteSpeed | 🔒 Locked |
| 18 | Security Review + Pen-Test | 🔒 Locked |
| 19 | Installer + Updater + Rollback | 🔒 Locked |
| 20 | Production Hardening + DR | 🔒 Locked |

> **Rule:** No phase starts until the previous phase passes its acceptance gate.

---

## Security Philosophy

- **Security-first, not security-last.**
- Typed operations only — no generic shell execution from web.
- Every privileged operation is audited.
- Secrets are never logged or stored in source code.
- Customer isolation is enforced at the OS level.
- See [SECURITY.md](SECURITY.md) and [MASTER_PROMPT.md](MASTER_PROMPT.md).

---

## Getting Started (Development)

### Prerequisites
- Linux (Ubuntu 22.04+ / Debian 12+)
- Go 1.22+
- Node.js 20+
- Docker (for local services)
- PostgreSQL 15+
- Redis 7+

### Setup
```bash
# Clone the repository
git clone <repo-url> hosting-panel
cd hosting-panel

# Copy environment example (never use real credentials in dev)
cp .env.example .env

# Run development setup script
./scripts/dev-setup.sh
```

> ⚠️ **Never use production credentials or customer data in development.**
> See [CONTRIBUTING.md](CONTRIBUTING.md) for full development policy.

---

## Repository Structure

```
hosting-panel/
├── MASTER_PROMPT.md       ← AI Agent constitution
├── README.md
├── SECURITY.md            ← Vulnerability reporting
├── CONTRIBUTING.md        ← Dev guidelines
├── .gitignore
├── .env.example           ← Fake values only
├── docs/                  ← Architecture & security docs
├── control-plane/         ← Web UI + REST API + Auth
├── agent/                 ← Privileged server agent
├── frontend/              ← React/Next.js UI
├── integrations/whmcs/    ← WHMCS module
├── installers/            ← Installation scripts
├── migrations/            ← Database migrations
├── tests/                 ← All test suites
└── scripts/               ← Dev & ops utilities
```

---

## License

Private — all rights reserved.
