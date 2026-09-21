# Disaster Recovery (DR) Runbook

This document outlines the standard operating procedures for recovering GohaHost infrastructure after a catastrophic failure.

## Scenario A: Total Loss of the Control Plane
*The server hosting the PostgreSQL database and the `gohahost-control-plane` binary is destroyed.*

**Recovery Steps:**
1. **Provision a New Server**: Deploy a fresh Ubuntu/Debian server.
2. **Restore PostgreSQL**:
   - Install PostgreSQL.
   - Run `createdb gohahost`.
   - Restore the latest off-site backup: `pg_restore -d gohahost db_backup.dump`.
3. **Restore Cryptographic Keys**:
   - Restore the `.env` file containing the `AES_MASTER_KEY`. (Without this, all Agent tokens in the database are permanently unreadable).
4. **Deploy Binary**:
   - Download the exact same version of `gohahost-control-plane` that was running.
   - Start the service.
5. **DNS Update**:
   - Update the A record for the Control Plane so Agents can reconnect. Since Agents initiate the mTLS connection, they will automatically sync once DNS propagates.

## Scenario B: Total Loss of a Compute Node (Agent)
*A server hosting customer websites (PHP/Nginx) is destroyed.*

**Recovery Steps:**
1. **Provision a New Server**: Deploy a fresh node and install the `gohahost-agent`.
2. **Enroll Node**:
   - Generate a new enrollment token from the Control Plane.
   - Enroll the new Agent.
3. **Update Database Mapping**:
   - In the Control Plane database, update the lost node's ID to point to the new Agent's ID, or manually reassign the Users/VirtualHosts to the new Node.
4. **Configuration Push (Idempotency)**:
   - Run the Control Plane's `SyncNode` utility (or re-trigger the VirtualHost creation API).
   - The Agent will idempotently re-create the `nginx/lsws` configurations, PHP-FPM pools, and directory structures based on the Control Plane's state.
5. **Restore Customer Data**:
   - Restore `/var/www/` from the off-site backup to the new Node.
   - Run the Control Plane `IssueSSL` utility to re-fetch certificates.

## Break-Glass Procedure
If the UI is completely inaccessible but the database is up, SuperAdmins can inject emergency operations by inserting tasks directly into the `tasks` table with the desired JSON payloads. The Control Plane worker will pick them up and dispatch them to the Agents.
