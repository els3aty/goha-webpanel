#!/usr/bin/env bash
# GohaHost Production Installer (Full Stack)
# Supports: Ubuntu, Debian, AlmaLinux, Rocky Linux, RHEL

set -euo pipefail

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=====================================================${NC}"
echo -e "${GREEN}        GohaHost Control Panel Installer             ${NC}"
echo -e "${GREEN}=====================================================${NC}"

if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}Error: Please run as root.${NC}"
  exit 1
fi

# 1. Detect OS
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
            echo -e "${RED}Unsupported OS: $ID${NC}"
            exit 1
            ;;
    esac
else
    echo -e "${RED}Cannot determine OS. /etc/os-release not found.${NC}"
    exit 1
fi

echo -e "${YELLOW}Detected OS: $PRETTY_NAME ($OS_FAMILY family)${NC}"

# 2. Install Dependencies
echo -e "${YELLOW}--> Step 1: Installing System Dependencies...${NC}"
if [ "$OS_FAMILY" = "debian" ]; then
    apt-get update -y
    apt-get install -y wget curl git build-essential nginx postgresql redis-server certbot tar \
        php-fpm php-cli php-mysql php-curl php-gd php-mbstring php-xml php-zip \
        postfix dovecot-core dovecot-imapd dovecot-pop3d jq
    
    # Enable services
    systemctl enable --now postgresql redis-server nginx php8.1-fpm || true
elif [ "$OS_FAMILY" = "rhel" ]; then
    dnf install -y epel-release
    dnf install -y dnf-plugins-core
    dnf config-manager --set-enabled crb || true # For Alma 9
    
    # Install PHP via Remi
    dnf install -y https://rpms.remirepo.net/enterprise/remi-release-9.rpm || true
    dnf module reset php -y || true
    dnf module enable php:remi-8.2 -y || true

    dnf install -y wget curl git gcc nginx postgresql-server redis certbot tar \
        php-fpm php-cli php-mysqlnd php-curl php-gd php-mbstring php-xml php-zip \
        postfix dovecot jq
    
    # Initialize Postgres on RHEL
    if [ ! -f /var/lib/pgsql/data/PG_VERSION ]; then
        echo "Initializing PostgreSQL..."
        postgresql-setup --initdb || true
    fi
    
    # Enable services
    systemctl enable --now postgresql redis nginx php-fpm postfix dovecot
fi

# 3. Install Golang
echo -e "${YELLOW}--> Step 2: Installing Golang 1.22...${NC}"
if ! command -v go &> /dev/null; then
    wget -q https://go.dev/dl/go1.22.1.linux-amd64.tar.gz -O /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    ln -sf /usr/local/go/bin/go /usr/bin/go
fi
go version

# 4. Clone Source Code
echo -e "${YELLOW}--> Step 3: Fetching GohaHost Source Code...${NC}"
INSTALL_DIR="/opt/gohahost"
if [ -d "$INSTALL_DIR" ]; then
    echo "Directory $INSTALL_DIR exists. Updating repository..."
    cd "$INSTALL_DIR"
    git pull origin main
else
    git clone https://github.com/els3aty/goha-webpanel.git "$INSTALL_DIR"
    cd "$INSTALL_DIR"
fi

# 5. Setup PostgreSQL Database
echo -e "${YELLOW}--> Step 4: Configuring Database...${NC}"
DB_PASSWORD=$(head -c 12 /dev/urandom | base64 | tr -dc 'a-zA-Z0-9' | head -c 16)
if [ "$OS_FAMILY" = "debian" ]; then
    sudo -u postgres psql -c "CREATE USER gohahost WITH PASSWORD '$DB_PASSWORD';" || true
    sudo -u postgres psql -c "CREATE DATABASE gohahost OWNER gohahost;" || true
elif [ "$OS_FAMILY" = "rhel" ]; then
    sudo -u postgres psql -c "CREATE USER gohahost WITH PASSWORD '$DB_PASSWORD';" || true
    sudo -u postgres psql -c "CREATE DATABASE gohahost OWNER gohahost;" || true
    # Fix RHEL ident auth issue
    sed -i 's/ident/md5/g' /var/lib/pgsql/data/pg_hba.conf || true
    systemctl restart postgresql
fi

# 6. Generate Configuration
echo -e "${YELLOW}--> Step 5: Generating Configurations...${NC}"
mkdir -p /etc/gohahost
PUBLIC_IP=$(curl -s ifconfig.me)
cat > /etc/gohahost/.env <<EOF
APP_ENV=production
APP_PORT=8080
APP_URL=http://${PUBLIC_IP}:8080
APP_SECRET_KEY=$(head -c 32 /dev/urandom | base64)
SESSION_SECRET=$(head -c 32 /dev/urandom | base64)

DB_HOST=127.0.0.1
DB_PORT=5432
DB_NAME=gohahost
DB_USER=gohahost
DB_PASSWORD=${DB_PASSWORD}
DB_SSLMODE=disable

REDIS_HOST=127.0.0.1
REDIS_PORT=6379
EOF

# 7. Compile Binaries
echo -e "${YELLOW}--> Step 6: Compiling Control Plane and Agent...${NC}"
cd "$INSTALL_DIR/control-plane"
go mod tidy
go build -o /usr/local/bin/gohahost-control-plane ./cmd/server

cd "$INSTALL_DIR/agent"
go mod tidy
go build -o /usr/local/bin/gohahost-agent ./cmd/agent

# 8. Setup Systemd Services
echo -e "${YELLOW}--> Step 7: Creating Systemd Services...${NC}"

# Control Plane Service
cat > /etc/systemd/system/gohahost-control-plane.service <<EOF
[Unit]
Description=GohaHost Control Plane
After=network.target postgresql.service redis.service

[Service]
Type=simple
User=root
WorkingDirectory=/opt/gohahost/control-plane
EnvironmentFile=/etc/gohahost/.env
ExecStart=/usr/local/bin/gohahost-control-plane
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Agent Service
cat > /etc/systemd/system/gohahost-agent.service <<EOF
[Unit]
Description=GohaHost Agent
After=network.target nginx.service

[Service]
Type=simple
User=root
WorkingDirectory=/opt/gohahost/agent
EnvironmentFile=/etc/gohahost/.env
ExecStart=/usr/local/bin/gohahost-agent
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now gohahost-control-plane
systemctl enable --now gohahost-agent

# Open port 8080 in firewalld if it's active
if systemctl is-active --quiet firewalld; then
    echo "--> Opening port 8080 in firewall..."
    firewall-cmd --permanent --add-port=8080/tcp >/dev/null 2>&1
    firewall-cmd --reload >/dev/null 2>&1
fi

# Give it a moment to start and run migrations
sleep 3

echo -e "${GREEN}=====================================================${NC}"
echo -e "${GREEN}        GohaHost Installed Successfully!             ${NC}"
echo -e "${GREEN}=====================================================${NC}"
echo -e "Access the Control Plane API: ${YELLOW}http://${PUBLIC_IP}:8080${NC}"
echo -e "Database Password: ${YELLOW}${DB_PASSWORD}${NC}"
echo -e "Configuration File: /etc/gohahost/.env"
echo -e "Logs: journalctl -u gohahost-control-plane -f"
echo -e "====================================================="
