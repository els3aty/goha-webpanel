# Threat Model

> **Phase:** 1 | **Methodology:** STRIDE | **Last Updated:** 2026-09-21

---

## Scope

This threat model covers the Hosting Panel system including:
- Control Plane (Web UI, REST API, Auth, Job Queue)
- Server Agents (on hosting nodes)
- Hosting accounts (customer websites, apps, databases)
- External integrations (WHMCS, backup destinations)
- Administrative access (SSH, panel admin)

---

## Assets to Protect

| Asset | Sensitivity | Impact if Compromised |
|-------|------------|----------------------|
| Control Plane database | Critical | Full platform compromise |
| Agent mTLS private keys | Critical | Node takeover |
| Customer SSH keys / credentials | High | Customer account takeover |
| Database credentials | High | Data breach |
| Backup encryption keys | High | Backup data exposure |
| WHMCS API token | High | Unauthorized provisioning |
| TLS certificates / private keys | High | MITM attacks |
| Session tokens | High | Account takeover |
| Customer website files | Medium | Customer data exposure |
| Audit logs | Medium | Cover tracks for attackers |
| System configuration | Medium | Service disruption |

---

## Threat Actors

| Actor | Capability | Motivation |
|-------|-----------|-----------|
| **Malicious Customer** | Runs code on hosting node | Escape isolation, access other customers |
| **Compromised Customer Site** | Arbitrary code execution in web context | Lateral movement, data theft |
| **External Attacker (unauthenticated)** | Network access to exposed services | Gain initial foothold |
| **Authenticated Attacker (low-privilege)** | Valid customer/reseller session | Privilege escalation, IDOR |
| **Compromised WHMCS** | Valid WHMCS API token | Unauthorized account operations |
| **Compromised Agent Node** | Access to one node | Lateral movement to other nodes |
| **Malicious Backup Content** | Crafted backup archive | Path traversal, code execution during restore |
| **Supply Chain Attacker** | Malicious dependency or update | Code execution in panel |
| **Insider / Admin** | Full admin access | Data theft, sabotage |

---

## STRIDE Threat Matrix

### S — Spoofing

| ID | Threat | Component | Impact | Mitigation | Residual Risk |
|----|--------|-----------|--------|-----------|---------------|
| S-01 | Agent impersonation — attacker runs a fake agent | Agent endpoint | Control Plane sends tasks to malicious node | mTLS mutual authentication; agent must present a CA-signed certificate | Low — cert must be signed by internal CA |
| S-02 | Control Plane impersonation — malicious service contacts agent | Agent | Agent executes unauthorized tasks | Agent verifies Control Plane certificate against CA | Low |
| S-03 | Session token theft and replay | Auth | Account takeover | HttpOnly+Secure+SameSite cookies; session rotation on privilege change; revocation | Low-Medium |
| S-04 | WHMCS token spoofing — attacker uses stolen token | WHMCS API | Unauthorized provisioning | Scoped token; IP allowlist; rate limiting; short-lived rotation | Medium — token theft possible if network unprotected |
| S-05 | API token theft | REST API | Unauthorized API access | Scoped tokens; IP restriction optional; revocation; audit | Medium |
| S-06 | Forged task payload sent to agent | Agent | Unauthorized operation execution | Task payload signed by Control Plane; agent verifies signature | Low |

---

### T — Tampering

| ID | Threat | Component | Impact | Mitigation | Residual Risk |
|----|--------|-----------|--------|-----------|---------------|
| T-01 | Nginx config injection via domain name | Web Server Driver | Arbitrary nginx directives; service disruption | Strict domain validation; no raw user directives; config validated before apply | Low |
| T-02 | Database query injection | All DB operations | Data breach; privilege escalation | Parameterized queries everywhere; no string concatenation in SQL | Low |
| T-03 | Shell command injection via filename/domain | Agent operations | Arbitrary command execution | Exec with argument arrays only; no shell concatenation; strict input validation | Low |
| T-04 | PHP-FPM config tampering via unsafe settings | PHP management | Security directive bypass | Allowlist of permitted PHP settings; admin-only override; config validation | Low-Medium |
| T-05 | Backup archive tampering (zip-slip, path traversal) | Backup/Restore | Write files outside account root | Treat restore content as untrusted; canonical path validation; chroot if possible | Medium |
| T-06 | DNS record injection | DNS driver | Hijack domains | Strict record type + value validation; ownership check before any zone operation | Low |
| T-07 | Config file modification via file manager | File Manager | Service config tampering | File manager restricted to account home; sensitive system paths blocklisted | Low-Medium |
| T-08 | Audit log tampering | Audit system | Cover attacker tracks | Append-only audit log table; optional remote export; integrity checks | Medium |
| T-09 | Task replay attack — resend old valid task | Agent | Repeat destructive operation | Nonce + timestamp; agent rejects duplicate nonces within TTL window | Low |

---

### R — Repudiation

| ID | Threat | Component | Impact | Mitigation | Residual Risk |
|----|--------|-----------|--------|-----------|---------------|
| R-01 | Admin denies performing destructive action | Audit log | No accountability | Immutable audit log: actor, action, target, IP, request ID, timestamp | Low-Medium |
| R-02 | Customer denies file deletion | File Manager audit | Disputes | Audit every file operation with actor + timestamp | Low-Medium |
| R-03 | WHMCS denies provisioning request | WHMCS audit | Billing disputes | Log every WHMCS request: service ID, action, actor, result | Low |

---

### I — Information Disclosure

| ID | Threat | Component | Impact | Mitigation | Residual Risk |
|----|--------|-----------|--------|-----------|---------------|
| I-01 | Secret leakage in logs | Logging | Credential exposure | Structured logging with allowlist fields; redact Authorization, Cookie, passwords, tokens | Low |
| I-02 | Customer A accesses Customer B data (IDOR) | REST API | Data breach | Backend ownership checks on every resource access; never trust client-supplied owner ID | Low |
| I-03 | Database credentials exposed in API response | DB management | Credential theft | Credentials returned only through secure workflow; never in list/status responses | Low |
| I-04 | Private key returned in API response | SSL/Keys | Key compromise | Private keys stored encrypted; never returned in API except initial provisioning with one-time display | Low-Medium |
| I-05 | Backup credentials exposed to customer | Backup | Backup destination access | Backup credentials stored in control plane secret store; never sent to customer UI | Low |
| I-06 | Env variables leaked in Node.js app logs | Node.js mgmt | Secret exposure | Env variable display masked in UI; logs filtered for known secret patterns | Medium |
| I-07 | Verbose error messages expose internals | API | Attack surface mapping | Production errors return generic messages; details in server-side logs only | Low |
| I-08 | MariaDB/Redis accessible from public network | DB/Cache | Direct DB attacks | Bind to 127.0.0.1 / private network only; nftables default-deny | Low |
| I-09 | SSRF via webhook or URL feature | Future features | Internal service access | Validate and restrict URLs; blocklist internal IP ranges; no SSRF-prone features without careful review | Medium |

---

### D — Denial of Service

| ID | Threat | Component | Impact | Mitigation | Residual Risk |
|----|--------|-----------|--------|-----------|---------------|
| D-01 | Brute force login | Auth | Account lockout / credential theft | Rate limiting per IP + per account; TOTP as second factor; lockout policy | Low-Medium |
| D-02 | Resource exhaustion via API spam | REST API | Service unavailability | Rate limiting per token + per IP; job queue throttling | Medium |
| D-03 | Disk exhaustion via file upload | File Manager | Node outage | Per-account disk quota enforced at OS level; upload size limits | Low |
| D-04 | Decompression bomb in archive upload | File Manager / Backup | Disk/CPU exhaustion | Size limit before extraction; streaming extraction with quota check | Low-Medium |
| D-05 | CPU/RAM exhaustion by customer app | Node.js / PHP | Node degradation | cgroups v2 resource limits per account; PM2 process limits | Low-Medium |
| D-06 | Agent disconnection (node unreachable) | Multi-node | Tasks fail for that node | Job queue with retry; health monitoring; alert on agent unreachable | Low |

---

### E — Elevation of Privilege

| ID | Threat | Component | Impact | Mitigation | Residual Risk |
|----|--------|-----------|--------|-----------|---------------|
| E-01 | Customer escalates to Admin via IDOR | RBAC | Full platform access | Backend RBAC on every endpoint; never trust role from client | Low |
| E-02 | Reseller manages another reseller's customers | RBAC/Reseller | Tenant violation | Ownership tree validation; reseller can only see their own subtree | Low |
| E-03 | Customer escapes home directory | File Manager | Read/write other accounts | Canonical path validation with chdir guard; symlink protection; no `..` traversal | Low-Medium |
| E-04 | PHP script escapes FPM pool | PHP isolation | Cross-account access | Per-account FPM pool with separate Linux user; open_basedir; chroot where supported | Medium |
| E-05 | Customer app binds to privileged/public port | Node.js | Service hijacking | System-allocated ports only; binding to 0.0.0.0 or port <1024 blocked | Low |
| E-06 | Mass assignment in API | REST API | Unauthorized field modification | Explicit input binding; never bind all request fields to model | Low |
| E-07 | Privilege escalation via WHMCS token | WHMCS | Admin-level operations | WHMCS token scoped to provisioning operations only; no admin operations allowed | Low |
| E-08 | Symlink attack to escalate file access | File Manager / Backup | Read arbitrary files | Detect and reject symlinks pointing outside account root; use O_NOFOLLOW | Low-Medium |
| E-09 | Compromised node agent pivots to other nodes | Multi-node | Platform-wide compromise | Agents cannot communicate with each other; agent only communicates with Control Plane | Low-Medium |

---

## High-Priority Threat Scenarios

### Scenario 1: Malicious Customer Website
**Description:** Customer uploads a PHP/Node.js application containing malicious code.

**Attack Chain:**
1. Customer uploads web shell or exploit code via file manager or FTP
2. Malicious code executes in web server context
3. Attempts to read other customers' files, escape to system, or pivot

**Mitigations:**
- Separate Linux user per account (OS-level isolation)
- `open_basedir` in PHP-FPM
- Per-account cgroups v2 resource limits
- No PHP execution outside account webroot
- File manager with path canonicalization
- ModSecurity + OWASP CRS (WAF layer)

**Residual Risk:** Medium — determined attacker with kernel exploit could escape; local privilege escalation vulnerabilities in Linux kernel remain a risk.

---

### Scenario 2: Compromised WHMCS Integration
**Description:** WHMCS instance is compromised; attacker has WHMCS API token.

**Attack Chain:**
1. Attacker uses stolen WHMCS token
2. Sends CreateAccount, TerminateAccount, or ChangePackage requests
3. Attempts to delete accounts, over-provision, or access admin operations

**Mitigations:**
- WHMCS token scoped to provisioning operations only (no admin/config access)
- IP allowlist for WHMCS API endpoint
- Rate limiting on WHMCS endpoints
- All WHMCS operations idempotent (duplicate CreateAccount safe)
- Termination requires backup policy before deletion
- Full audit trail of all WHMCS-originated operations

**Residual Risk:** Low-Medium — attacker can create/suspend/terminate accounts but cannot access admin functions or other customers' data.

---

### Scenario 3: Malicious Backup Restore
**Description:** Attacker crafts a malicious backup archive and triggers a restore.

**Attack Chain:**
1. Backup archive contains: path traversal entries (../../../etc/passwd), symlinks to system files, or malicious DB dumps
2. Restore process extracts to wrong location or overwrites system files

**Mitigations:**
- Treat all restore content as untrusted
- Validate all archive entries: canonical path, no `..`, no absolute paths
- Symlink detection and rejection
- Restore to staging directory first, validate, then move to account root
- DB restore uses controlled import, not arbitrary SQL execution
- Restore bounded to account's home directory

**Residual Risk:** Low — comprehensive input validation on restore pipeline.

---

### Scenario 4: Agent Node Compromise
**Description:** An attacker gains root access to a hosting node.

**Attack Chain:**
1. Attacker compromises one node (e.g., via customer exploit + local privilege escalation)
2. Has access to Agent private key and local data on that node
3. Attempts to pivot to Control Plane or other nodes

**Mitigations:**
- Agent's mTLS cert is node-specific — compromising one node does not give access to others
- Agents cannot communicate with each other directly
- Control Plane verifies node identity on every task
- Compromised node can be revoked (certificate revocation) from Control Plane
- Blast radius limited to data on that specific node

**Residual Risk:** Medium — customer data on the compromised node is at risk; cross-node impact is limited.

---

## Items Requiring Independent Penetration Testing

- [ ] Authentication bypass and session management
- [ ] IDOR across all resource types (accounts, domains, databases, mailboxes, backups)
- [ ] Path traversal in file manager and backup restore
- [ ] Agent protocol: forged certificates, replayed tasks, expired tasks
- [ ] SQL injection in all input paths
- [ ] Command injection in all typed agent operations
- [ ] PHP/Node.js isolation escape
- [ ] SSRF in any URL-accepting feature
- [ ] Race conditions in quota allocation
- [ ] Symlink attacks in file manager and restore

---

## Threat Model Review Schedule

This threat model should be reviewed:
- Before starting Phase 18 (Security Review)
- After any significant architecture change
- After any confirmed security incident
