# Mail Stack Architecture (Phase 13)

This document details how GohaHost provisions and manages email services (Postfix and Dovecot).

## 1. Overview
GohaHost supports creating Mailboxes and Aliases/Forwarders for virtual domains. To guarantee node isolation and avoid database single-points-of-failure for email delivery, the Agent manages local virtual map files instead of connecting the mail daemon directly to the Control Plane PostgreSQL database.

## 2. File-Based Map Management
The Agent manages three critical files using native Go string manipulation:
1. `/etc/dovecot/users`: Stores `email:password_hash::::::`.
2. `/etc/postfix/vmailbox`: Stores `email domain/mailbox/`.
3. `/etc/postfix/virtual`: Stores `alias destination`.

After modifying Postfix maps, the Agent securely invokes `/usr/sbin/postmap` using `exec.CommandContext` (Zero Shell) to rebuild the indexed `.db` files required by Postfix.

## 3. Password Security & Hashing
A common vulnerability in control panels is executing `doveadm pw -s SHA512 -p <password>` via shell. If the user provides a password containing shell characters (e.g., `pass; rm -rf /`), it can lead to command injection.

**GohaHost Mitigation:**
The Server Agent **never** sees or handles the plaintext password. 
1. The Control Plane receives the plaintext password.
2. The Control Plane hashes the password natively in Go using `bcrypt`.
3. The Control Plane prepends the Dovecot scheme identifier (`{BLF-CRYPT}`).
4. The Control Plane sends *only the hash* to the Agent via the mTLS task payload.
5. The Agent appends the hash directly to `/etc/dovecot/users`.

This ensures 100% Zero-Shell compliance and eliminates any possibility of password-based command injection.
