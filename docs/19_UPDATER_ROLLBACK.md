# Updater & Rollback System (Phase 19)

This document describes how GohaHost manages its own lifecycle and updates.

## 1. Goal
Provide a reliable, automated way to update the Control Plane and Agents without causing downtime, data loss, or unrecoverable system states.

## 2. Architecture
Instead of using a self-updating Go binary (which can lead to corrupted binaries if interrupted), we use a standard bash script wrapper: `scripts/updater.sh`.

### The Update Lifecycle:
1. **Snapshots**: 
   - Takes a `pg_dump` of the PostgreSQL database.
   - Takes a copy of the existing executable binary.
2. **Download**:
   - Fetches the new binary from the release channel.
3. **Pre-Flight Health Check**:
   - Executes the new binary with `--version`.
   - If the binary crashes (e.g. built for wrong OS/Arch, or missing dynamic libraries), the update aborts immediately. The running system is untouched.
4. **Apply & Migrate**:
   - `systemctl stop` -> replace binary -> `systemctl start`.
   - The Control Plane automatically runs database migrations upon startup.
5. **Post-Flight & Rollback**:
   - Waits 3 seconds and checks `systemctl is-active`.
   - If the service is crashing (e.g., bad migration, bad config), the script triggers a **Rollback**.
   - **Rollback Process**: Stops the service, restores the backed-up binary, restores the `pg_dump` database, and restarts the service.

## 3. Security
- The updater script must be run as `root`.
- In a production environment, the download URL must be HTTPS, and ideally, the script should verify a GPG signature of the binary before executing the pre-flight check.
