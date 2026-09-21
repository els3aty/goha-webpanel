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

## Installation (Production)

To install GohaHost on a fresh Ubuntu 22.04 or Debian 12 server:

```bash
# 1. Download the installation script
curl -O https://raw.githubusercontent.com/els3aty/goha-webpanel/main/installers/install.sh

# 2. Make it executable
chmod +x install.sh

# 3. Run the installer (Must be root)
sudo ./install.sh
```

### Post-Installation
After the installation completes:
1. Access the Control Plane at `http://<your-server-ip>:8080`
2. Login with the credentials provided in your terminal output.
3. Keep your `AES_MASTER_KEY` safe! (Stored in `/etc/gohahost/.env`).

> ⚠️ **Important:** Do not run this installer on a server that already hosts active websites. GohaHost requires a fresh OS installation to configure OS-level quotas and web servers securely.

---

## Development Setup

```bash
git clone https://github.com/els3aty/goha-webpanel.git
cd goha-webpanel
cp .env.example .env
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
