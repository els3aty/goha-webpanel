# Analytics & Monitoring Architecture (Phase 16)

This document explains the Zero-Overhead metrics collection strategy used in GohaHost.

## 1. Overview
GohaHost requires real-time insights into the CPU, RAM, Disk, and Service health of all Server Nodes. However, installing heavy third-party daemons (like Prometheus, Netdata, or Zabbix) on every node introduces unwanted bloat, security risks, and consumes the very resources we are trying to monitor.

## 2. Zero-Overhead Kernel Metrics
The GohaHost Agent is designed to collect these metrics directly from the Linux Kernel with absolute minimal overhead:

| Metric | Source / Method | Why this is superior |
|--------|-----------------|----------------------|
| **CPU Load** | `/proc/loadavg` | Doesn't require spawning `top` or calculating deltas over time. The kernel already calculates the 1, 5, and 15-minute load averages perfectly. |
| **Memory** | `/proc/meminfo` | Doesn't spawn `free -m`. Directly reads the kernel's memory allocation counters in memory, costing essentially zero CPU cycles. |
| **Disk Space**| `syscall.Statfs`| Doesn't spawn `df -h`. Uses the native POSIX system call to ask the filesystem for free block counts instantly. |

## 3. Service Status & Zero-Shell
To check if services like Nginx, MariaDB, and Postfix are running, the Agent uses `systemctl is-active <service>`.
Instead of doing `sh -c "systemctl is-active nginx"`, which allows command injection, the Agent executes the binary directly:
`exec.CommandContext("/usr/bin/systemctl", "is-active", "nginx")`
This guarantees that even if a malicious payload was injected into the service name, it could not compromise the shell.

## 4. Polling Strategy
The Control Plane triggers the metric collection. Because the Agent's metric collection is instantaneous and purely native Go / Kernel reads, the Control Plane can poll this endpoint frequently (e.g., every 5 seconds) without causing any noticeable load on the Server Node.
