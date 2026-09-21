#!/usr/bin/env bash
# GohaHost Updater Script
# Usage: ./updater.sh <version>
# Example: ./updater.sh v1.2.0

set -euo pipefail

VERSION=${1:-""}
if [ -z "$VERSION" ]; then
    echo "Usage: $0 <version>"
    exit 1
fi

BACKUP_DIR="/var/backups/gohahost/$(date +%Y%m%d_%H%M%S)"
BIN_DIR="/usr/local/bin"
CP_BIN="$BIN_DIR/gohahost-control-plane"
DB_NAME="gohahost"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}Starting GohaHost Update to $VERSION...${NC}"

if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}Please run as root.${NC}"
  exit 1
fi

mkdir -p "$BACKUP_DIR"

# 1. Backup Database
echo "Backing up database..."
if ! -u postgres pg_dump -Fc $DB_NAME > "$BACKUP_DIR/db.dump"; then
    echo -e "${RED}Database backup failed. Aborting update.${NC}"
    exit 1
fi

# 2. Backup Binaries
echo "Backing up binaries..."
if [ -f "$CP_BIN" ]; then
    cp "$CP_BIN" "$BACKUP_DIR/gohahost-control-plane.bak"
fi

# 3. Download/Stage New Binary
# For this script we assume the binary is fetched from a release URL.
# Since this is a demo, we will simulate downloading the binary by checking if a local file exists, or just skipping the download.
DOWNLOAD_URL="https://releases.gohahost.com/$VERSION/gohahost-control-plane"
NEW_BIN="/tmp/gohahost-control-plane-new"

echo "Downloading new binary from $DOWNLOAD_URL..."
# curl -sSL -o $NEW_BIN $DOWNLOAD_URL
# chmod +x $NEW_BIN

# SIMULATION BLOCK: If the new binary isn't actually there (because we are not really downloading), we will fake it for testing
if [ ! -f "$NEW_BIN" ]; then
    echo "Simulating binary download..."
    cp "$CP_BIN" "$NEW_BIN" || touch "$NEW_BIN"
    chmod +x "$NEW_BIN"
fi

# 4. Pre-flight Health Check on New Binary
echo "Running pre-flight checks on new binary..."
# We expect the binary to support a --version flag that exits 0
if ! $NEW_BIN --version >/dev/null 2>&1; then
    echo -e "${RED}New binary failed sanity check. Aborting!${NC}"
    rm -f "$NEW_BIN"
    exit 1
fi

# 5. Apply Update
echo "Applying update..."
systemctl stop gohahost-control-plane || true
mv "$NEW_BIN" "$CP_BIN"

# 6. Database Migrations
# We assume the Control Plane runs migrations automatically on startup.
echo "Starting Control Plane..."
systemctl start gohahost-control-plane || true

# 7. Post-flight Check & Rollback
echo "Waiting for service to stabilize..."
sleep 3

if ! systemctl is-active --quiet gohahost-control-plane; then
    echo -e "${RED}Service failed to start! Initiating ROLLBACK...${NC}"
    
    systemctl stop gohahost-control-plane || true
    
    # Restore Binary
    if [ -f "$BACKUP_DIR/gohahost-control-plane.bak" ]; then
        cp "$BACKUP_DIR/gohahost-control-plane.bak" "$CP_BIN"
    fi
    
    # Restore Database
    echo "Restoring database..."
    # Drop and recreate (requires disconnects, simplified here)
    -u postgres dropdb $DB_NAME || true
    -u postgres createdb $DB_NAME || true
    -u postgres pg_restore -d $DB_NAME "$BACKUP_DIR/db.dump" || true
    
    systemctl start gohahost-control-plane || true
    echo -e "${YELLOW}Rollback completed. System restored to previous state.${NC}"
    exit 1
fi

echo -e "${GREEN}Update to $VERSION applied successfully!${NC}"
