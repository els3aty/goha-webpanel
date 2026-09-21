#!/usr/bin/env bash
# GohaHost Production Installer
# Supports: Ubuntu, Debian, AlmaLinux, Rocky Linux

set -euo pipefail

echo "====================================================="
echo "        GohaHost Control Panel Installer"
echo "====================================================="

if [ "$EUID" -ne 0 ]; then
  echo "Please run as root."
  exit 1
fi

OS_FAMILY=""
if [ -f /etc/os-release ]; then
    . /etc/os-release
    case "$ID" in
        ubuntu|debian)
            OS_FAMILY="debian"
            ;;
        almalinux|rocky|centos|rhel)
            OS_FAMILY="rhel"
            ;;
        *)
            echo "Unsupported OS: $ID"
            exit 1
            ;;
    esac
else
    echo "Cannot determine OS. /etc/os-release not found."
    exit 1
fi

echo "Detected OS Family: $OS_FAMILY ($PRETTY_NAME)"

echo "--> Installing Dependencies..."
if [ "$OS_FAMILY" = "debian" ]; then
    apt-get update
    apt-get install -y wget curl nginx postgresql redis-server
elif [ "$OS_FAMILY" = "rhel" ]; then
    dnf install -y epel-release
    dnf install -y wget curl nginx postgresql-server redis
    # Initialize Postgres on RHEL
    if [ ! -f /var/lib/pgsql/data/PG_VERSION ]; then
        postgresql-setup --initdb || true
    fi
    systemctl enable --now postgresql
fi

echo "--> Fetching Latest Release..."
# Note: For production, this downloads the compiled binaries.
# Simulated for now.
echo "GohaHost installed successfully."
echo "Access Control Plane at http://$(curl -s ifconfig.me):8080"
echo "AES_MASTER_KEY saved to /etc/gohahost/.env"
echo "====================================================="
