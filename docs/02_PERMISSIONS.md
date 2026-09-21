# Permissions Model

> **Phase:** 1 | **Last Updated:** 2026-09-21

---

## Roles Overview

| Role | Description | Managed by |
|------|-------------|-----------|
| **SuperAdmin** | Full platform control | System owner only |
| **Admin** | Manage platform resources, resellers, packages | SuperAdmin |
| **Reseller** | Manage their own customers and packages | Admin / SuperAdmin |
| **Customer** | Manage their own hosting account | Reseller / Admin |
| **APIIntegration** | Machine-to-machine access (WHMCS, automation) | Admin |

---

## Role Hierarchy

```
SuperAdmin
    └── Admin
            └── Reseller
                    └── Customer
                            └── (Customer subusers — future)

APIIntegration (flat — scoped to specific operations)
```

**Key rules:**
- A role can only grant permissions it possesses itself
- A Reseller cannot create another Reseller at the same level
- A Reseller's resource quota is bounded by what Admin delegated
- Ownership is always verified server-side — client claims are ignored

---

## Permissions Matrix

### Platform Administration

| Action | SuperAdmin | Admin | Reseller | Customer | APIIntegration |
|--------|:---------:|:-----:|:--------:|:--------:|:--------------:|
| View all accounts | ✅ | ✅ | Own only | Own only | ❌ |
| Create Admin user | ✅ | ❌ | ❌ | ❌ | ❌ |
| Create Reseller | ✅ | ✅ | ❌ | ❌ | ❌ |
| Create Customer | ✅ | ✅ | ✅ (own) | ❌ | ✅ (provisioning) |
| Delete any account | ✅ | ✅ | Own reseller tree | ❌ | ❌ |
| Manage packages | ✅ | ✅ | Own only | ❌ | ❌ |
| View audit logs | ✅ | ✅ | Own activity | ❌ | ❌ |
| Manage server nodes | ✅ | ✅ | ❌ | ❌ | ❌ |
| View system metrics | ✅ | ✅ | ❌ | ❌ | ❌ |
| Manage firewall rules | ✅ | ✅ | ❌ | ❌ | ❌ |
| Manage backup destinations | ✅ | ✅ | ❌ | ❌ | ❌ |
| Rotate WHMCS token | ✅ | ✅ | ❌ | ❌ | ❌ |
| View security center | ✅ | ✅ | ❌ | ❌ | ❌ |

---

### Hosting Account Management

| Action | SuperAdmin | Admin | Reseller | Customer | APIIntegration |
|--------|:---------:|:-----:|:--------:|:--------:|:--------------:|
| Create domain/subdomain | ✅ | ✅ | ✅ (own) | ✅ (own) | ✅ (provisioning) |
| Delete domain | ✅ | ✅ | ✅ (own) | ✅ (own) | ✅ (provisioning) |
| Manage SSL certificates | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Set PHP version | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Manage PHP settings | ✅ | ✅ | ✅ (own) | Allowlist only | ❌ |
| Manage Node.js apps | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| View Node.js env vars | ✅ | ✅ | ✅ (own) | Own (masked) | ❌ |
| Suspend account | ✅ | ✅ | ✅ (own) | ❌ | ✅ (provisioning) |
| Terminate account | ✅ | ✅ | ✅ (own) | ❌ | ✅ (provisioning) |
| Change account password | ✅ | ✅ | ✅ (own) | Own only | ✅ (provisioning) |
| Change account package | ✅ | ✅ | ✅ (own) | ❌ | ✅ (provisioning) |

---

### Database Management

| Action | SuperAdmin | Admin | Reseller | Customer | APIIntegration |
|--------|:---------:|:-----:|:--------:|:--------:|:--------------:|
| Create database | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Delete database | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Rotate DB password | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| View DB credentials | ✅ | ✅ | ✅ (own) | Own only | ❌ |
| DB backup/restore | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Grant DB privileges | ✅ | ✅ | ✅ (own) | Own scope | ❌ |

---

### File Manager

| Action | SuperAdmin | Admin | Reseller | Customer | APIIntegration |
|--------|:---------:|:-----:|:--------:|:--------:|:--------------:|
| Browse files | ✅ | ✅ (any) | Own tree | Own home | ❌ |
| Upload files | ✅ | ✅ (any) | Own tree | Own home | ❌ |
| Download files | ✅ | ✅ (any) | Own tree | Own home | ❌ |
| Delete files | ✅ | ✅ (any) | Own tree | Own home | ❌ |
| Create/extract archives | ✅ | ✅ (any) | Own tree | Own home | ❌ |
| Edit files | ✅ | ✅ (any) | Own tree | Own home | ❌ |
| Access system paths | ✅ | ❌ | ❌ | ❌ | ❌ |

---

### DNS Management

| Action | SuperAdmin | Admin | Reseller | Customer | APIIntegration |
|--------|:---------:|:-----:|:--------:|:--------:|:--------------:|
| Create DNS zone | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Delete DNS zone | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Add/edit A, AAAA, CNAME records | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Add/edit MX records | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Add/edit TXT records | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Edit raw zone file | ✅ | ✅ | ❌ | ❌ | ❌ |
| Manage DNSSEC | ✅ | ✅ | ❌ | ❌ | ❌ |

---

### Mail Management

| Action | SuperAdmin | Admin | Reseller | Customer | APIIntegration |
|--------|:---------:|:-----:|:--------:|:--------:|:--------------:|
| Create mailbox | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Delete mailbox | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Reset mailbox password | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Manage aliases/forwarders | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| View DKIM keys | ✅ | ✅ | ✅ (own) | Own (public) | ❌ |
| Manage spam filters | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |

---

### Backup Management

| Action | SuperAdmin | Admin | Reseller | Customer | APIIntegration |
|--------|:---------:|:-----:|:--------:|:--------:|:--------------:|
| Configure backup destinations | ✅ | ✅ | ❌ | ❌ | ❌ |
| View backup destination credentials | ✅ | ✅ | ❌ | ❌ | ❌ |
| Trigger backup (own account) | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Trigger backup (any account) | ✅ | ✅ | ❌ | ❌ | ❌ |
| Restore backup (own account) | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Restore backup (any account) | ✅ | ✅ | ❌ | ❌ | ❌ |
| View backup history | ✅ | ✅ | Own tree | Own | ❌ |
| Delete old backups | ✅ | ✅ | ❌ | ❌ | ❌ |

---

### API Token Management

| Action | SuperAdmin | Admin | Reseller | Customer | APIIntegration |
|--------|:---------:|:-----:|:--------:|:--------:|:--------------:|
| Create API token | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Revoke any token | ✅ | ✅ | Own only | Own only | ❌ |
| View token scopes | ✅ | ✅ | Own only | Own only | Self |
| Set IP restriction on token | ✅ | ✅ | ✅ (own) | ✅ (own) | ❌ |
| Create WHMCS integration token | ✅ | ✅ | ❌ | ❌ | ❌ |

---

## RBAC Enforcement Rules

### Rule 1: Backend Enforcement Only
Frontend permission checks are **UI hints only** — never security controls.
Every API endpoint enforces RBAC independently.

### Rule 2: Ownership Verification
For any operation on a resource, the backend must verify:
1. Is the actor authenticated?
2. Does the actor's role allow this action?
3. Does the actor own (or have delegated access to) this specific resource?

Never trust client-supplied `owner_id`, `account_id`, or `user_id` parameters.

### Rule 3: Reseller Boundary
A Reseller can only see and manage resources in their own customer tree.
Cross-reseller access is forbidden, even for Admins of a reseller.

### Rule 4: Scope Containment
A principal can never grant more permissions than they possess:
```
Reseller cannot create another Reseller
Admin cannot create SuperAdmin
APIIntegration token cannot be scoped beyond the creating user's permissions
```

### Rule 5: Audit Everything
Every permission check failure (403) must be logged with:
- Actor identity
- Resource attempted
- Operation attempted
- Timestamp and request ID

---

## APIIntegration Token Scopes

API tokens are fine-grained and must be explicitly scoped:

| Scope | Description |
|-------|-------------|
| `domains:read` | List and view domains |
| `domains:write` | Create and update domains |
| `domains:delete` | Delete domains |
| `databases:read` | List databases |
| `databases:write` | Create databases |
| `ssl:read` | View certificate status |
| `ssl:write` | Issue/renew certificates |
| `backups:read` | View backup history |
| `backups:trigger` | Trigger backups |
| `whmcs:provisioning` | WHMCS-specific provisioning operations |
| `monitoring:read` | Read-only metrics |

WHMCS integration tokens use only the `whmcs:provisioning` scope.
