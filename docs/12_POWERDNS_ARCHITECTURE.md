# PowerDNS Architecture (Phase 12)

This document describes the architectural implementation of the PowerDNS integration in the GohaHost Control Panel.

## 1. Overview
GohaHost uses PowerDNS (PDNS) as its primary authoritative DNS server. PowerDNS replaces our earlier, file-based Bind9 implementation. 

## 2. API-First Integration
Instead of writing zone files to disk and invoking `rndc reload` (which requires shelling out), the Server Agent communicates with PowerDNS strictly via its built-in **REST API** (`http://127.0.0.1:8081`).

**Advantages:**
- **Zero-Shell Execution:** The agent makes standard HTTP POST/PATCH/DELETE requests. There are no shell commands executed, adhering perfectly to the Zero-Shell policy.
- **Immediate Propagation:** Changes are live immediately in the PowerDNS memory/backend without needing a service reload.
- **Automatic SOA Serial Management:** The PowerDNS API automatically increments the SOA serial number when records are modified, eliminating a massive source of human error and complex Go logic.
- **Strict Validation:** The API strictly validates RRsets, rejecting malformed DNS data before it reaches the backend.

## 3. Security Boundary
- The PowerDNS REST API binds **only** to `127.0.0.1`. It is not accessible from the public internet or from customer processes.
- The API is secured with a static `X-API-Key` configured in both `pdns.conf` and the Agent's environment variables.
- Because the Agent only connects to `localhost:8081`, there is no TLS overhead required for this local loopback connection.
