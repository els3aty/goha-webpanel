#!/usr/bin/env bash
# ============================================================
# Hosting Panel — Development Environment Setup Script
# ============================================================
# Usage: ./scripts/dev-setup.sh
#
# IMPORTANT: Run this on a development machine only.
# Never run on production servers.
# ============================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info()    { echo -e "${BLUE}[INFO]${NC}  $*"; }
log_success() { echo -e "${GREEN}[OK]${NC}    $*"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

echo ""
echo "========================================================"
echo "  Hosting Panel — Development Setup"
echo "========================================================"
echo ""

# ─── Safety Check ────────────────────────────────────────────
if [[ "${APP_ENV:-}" == "production" ]]; then
    log_error "APP_ENV is set to 'production'. This script is for development only."
fi

# ─── Check .env ───────────────────────────────────────────────
if [[ ! -f ".env" ]]; then
    log_warn ".env file not found. Copying from .env.example..."
    cp .env.example .env
    log_warn "Please review and update .env with your LOCAL development values."
    log_warn "NEVER use real production credentials."
fi

# ─── Check Requirements ───────────────────────────────────────
log_info "Checking prerequisites..."

check_cmd() {
    if command -v "$1" &>/dev/null; then
        log_success "$1 found: $(command -v $1)"
    else
        log_warn "$1 not found — please install it"
    fi
}

check_cmd go
check_cmd node
check_cmd npm
check_cmd psql
check_cmd redis-cli
check_cmd git

# ─── Dev TLS Certificates ─────────────────────────────────────
if [[ ! -d "dev-certs" ]]; then
    log_info "Generating self-signed dev certificates for local mTLS testing..."
    mkdir -p dev-certs

    # CA
    openssl genrsa -out dev-certs/ca.key 4096 2>/dev/null
    openssl req -new -x509 -days 365 -key dev-certs/ca.key \
        -out dev-certs/ca.crt \
        -subj "/CN=DevCA/O=HostingPanel Dev/C=US" 2>/dev/null

    # Control Plane cert
    openssl genrsa -out dev-certs/control-plane.key 2048 2>/dev/null
    openssl req -new -key dev-certs/control-plane.key \
        -out dev-certs/control-plane.csr \
        -subj "/CN=control-plane.dev/O=HostingPanel Dev/C=US" 2>/dev/null
    openssl x509 -req -days 365 -in dev-certs/control-plane.csr \
        -CA dev-certs/ca.crt -CAkey dev-certs/ca.key -CAcreateserial \
        -out dev-certs/control-plane.crt 2>/dev/null

    # Agent cert
    openssl genrsa -out dev-certs/agent.key 2048 2>/dev/null
    openssl req -new -key dev-certs/agent.key \
        -out dev-certs/agent.csr \
        -subj "/CN=agent-dev-01/O=HostingPanel Dev/C=US" 2>/dev/null
    openssl x509 -req -days 365 -in dev-certs/agent.csr \
        -CA dev-certs/ca.crt -CAkey dev-certs/ca.key -CAcreateserial \
        -out dev-certs/agent.crt 2>/dev/null

    # Secure key files
    chmod 600 dev-certs/*.key
    chmod 644 dev-certs/*.crt

    log_success "Dev certificates generated in ./dev-certs/"
    log_warn "Dev certs are for local testing only — never use in production."
else
    log_success "Dev certificates already exist."
fi

# ─── Dev Data Directories ─────────────────────────────────────
mkdir -p dev-data/backups dev-data/acme dev-data/uploads
log_success "Dev data directories ready."

# ─── Done ─────────────────────────────────────────────────────
echo ""
echo "========================================================"
log_success "Development environment setup complete!"
echo ""
echo "  Next steps:"
echo "  1. Review and update .env with your local values"
echo "  2. Ensure PostgreSQL and Redis are running"
echo "  3. Run database migrations (Phase 2+)"
echo "  4. See README.md for more details"
echo "========================================================"
echo ""
