# Production Release Gate Checklist (Phase 20)

Before GohaHost is deployed in a live, customer-facing production environment, the following criteria **MUST** be verified by the system administrator.

## 1. Secrets & Cryptography
- [ ] **AES Master Key:** Ensure the `.env` file contains a strong, randomly generated `AES_MASTER_KEY` that is securely backed up. (If lost, Agent communication breaks forever).
- [ ] **mTLS Certificates:** Ensure the Root CA and Node Certificates have not expired and are strictly scoped to the internal cluster network.

## 2. Network & Firewall (Least Privilege)
- [ ] **Control Plane Network:** Port 8080 (Agent Tasks API) should **only** be accessible to the IP addresses of the Compute Nodes (Agents). It MUST NOT be exposed to the public internet.
- [ ] **Agent Network:** Agents do not listen on any open ports for Control Plane commands (they use polling/long-polling). Only Ports 80 and 443 (HTTP/HTTPS) and 25/587/143/993 (Mail) should be open to the public.

## 3. Backups & Resilience
- [ ] **Database Backups:** A cron job is active that runs `pg_dump` daily and ships the dump to an off-site S3 bucket.
- [ ] **Filesystem Backups:** A script (e.g. Restic or Borg) is active on the Agent nodes to back up `/var/www/` to an off-site location.
- [ ] **Restore Drill:** The procedures in `DISASTER_RECOVERY.md` have been manually tested at least once.

## 4. Quotas & Limits
- [ ] **Kernel Limits:** PHP/Node processes are verified to respect the `systemd` CPU/Memory slicing configured in Phase 15.
- [ ] **Disk Quotas:** Linux user filesystem quotas (`edquota`) are enabled and enforced on the Agent partitions containing `/var/www`.

## 5. Security Posture
- [ ] **Zero-Shell Compliance:** Automated scanners confirm no usage of `sh -c` inside the compiled Go binaries.
- [ ] **Dependencies:** Run `govulncheck` on both the Control Plane and Agent before compiling the release binaries.

---
*If any of the above items are unchecked, the software is NOT cleared for production use.*
