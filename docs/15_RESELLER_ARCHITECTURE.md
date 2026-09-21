# Reseller & Packages Architecture (Phase 15)

This document explains how GohaHost manages hosting limits and the Reseller hierarchy.

## 1. Overview
GohaHost enforces limits (disk space, max domains, max databases) via **Hosting Packages**. A Hosting User is always assigned to a Package. 

## 2. Stateless Agent Architecture
To maintain a purely stateless and secure Agent design, **no package limits are enforced at the Agent level** (with the exception of native Linux disk quotas configured in Phase 4).

If a Reseller attempts to create a 6th database for a user whose package only allows 5:
1. The Control Plane checks the user's `package_id`.
2. The Control Plane counts the current number of databases assigned to that user.
3. The Control Plane rejects the API request with a `403 Forbidden` or `400 Bad Request`.
4. The Agent is never contacted.

## 3. Reseller Role
The `RoleReseller` is a standard JWT RBAC role. When a user has this role, they can:
- Create `HostingUser` entities mapped to their own `CustomerID`.
- Manage resources for those specific users.
- They **cannot** view, modify, or delete users belonging to other Resellers or Admins.
- They **cannot** create custom global packages (only SuperAdmins/Admins can).

## 4. Disk Quota Enforcement
When a Reseller provisions a user, the Control Plane pulls the `disk_quota_mb` from the selected Package and sends it as part of the `CreateHostingUser` payload to the Agent.
The Agent then executes the native Linux `setquota` command (as built in Phase 4) to enforce this quota at the filesystem level. This completely prevents the Reseller from bypassing storage limits via FTP or the File Manager.
