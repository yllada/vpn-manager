#!/bin/bash
# =============================================================================
# VPN Manager Daemon Installation Script
# =============================================================================
# This script installs the vpn-managerd daemon for privileged operations.
# Run with sudo: sudo ./install-daemon.sh
# =============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
DAEMON_NAME="vpn-managerd"
INSTALL_DIR="/usr/bin"
SERVICE_DIR="/etc/systemd/system"
SOCKET_DIR="/var/run/vpn-manager"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# =============================================================================
# Helper Functions
# =============================================================================

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

check_dependencies() {
    log_info "Checking dependencies..."
    
    # Check for Go
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed. Please install Go 1.21+ first."
        exit 1
    fi
    
    # Check Go version
    GO_VERSION=$(go version | grep -oP 'go\d+\.\d+' | grep -oP '\d+\.\d+')
    GO_MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
    GO_MINOR=$(echo "$GO_VERSION" | cut -d. -f2)
    
    if [[ "$GO_MAJOR" -lt 1 ]] || [[ "$GO_MAJOR" -eq 1 && "$GO_MINOR" -lt 21 ]]; then
        log_error "Go 1.21+ is required. Found: go$GO_VERSION"
        exit 1
    fi
    
    log_success "Go $GO_VERSION detected"
    
    # Check for systemd
    if ! command -v systemctl &> /dev/null; then
        log_error "systemd is not available on this system"
        exit 1
    fi
    
    log_success "systemd detected"
}

# =============================================================================
# Installation Steps
# =============================================================================

build_daemon() {
    log_info "Building daemon..."
    
    cd "$PROJECT_ROOT"
    
    # Build the daemon binary
    if go build -o "$DAEMON_NAME" ./cmd/vpn-managerd; then
        log_success "Daemon built successfully"
    else
        log_error "Failed to build daemon"
        exit 1
    fi
}

install_binary() {
    log_info "Installing daemon binary to $INSTALL_DIR..."
    
    # Stop service if running
    if systemctl is-active --quiet "$DAEMON_NAME" 2>/dev/null; then
        log_info "Stopping running daemon..."
        systemctl stop "$DAEMON_NAME"
    fi
    
    # Install binary
    install -m 755 "$PROJECT_ROOT/$DAEMON_NAME" "$INSTALL_DIR/$DAEMON_NAME"
    
    # Cleanup build artifact
    rm -f "$PROJECT_ROOT/$DAEMON_NAME"
    
    log_success "Binary installed to $INSTALL_DIR/$DAEMON_NAME"
}

install_service() {
    log_info "Installing systemd service..."
    
    # Copy service file
    install -m 644 "$SCRIPT_DIR/systemd/vpn-managerd.service" "$SERVICE_DIR/$DAEMON_NAME.service"
    
    # Reload systemd
    systemctl daemon-reload
    
    log_success "Service file installed"
}

create_directories() {
    log_info "Creating required directories..."
    
    # Socket directory (created by systemd via RuntimeDirectory, but ensure it exists)
    mkdir -p "$SOCKET_DIR"
    chmod 755 "$SOCKET_DIR"
    
    log_success "Directories created"
}

enable_service() {
    log_info "Enabling and starting service..."
    
    # Enable service to start on boot
    systemctl enable "$DAEMON_NAME"
    
    # Start service
    systemctl start "$DAEMON_NAME"
    
    # Wait a moment and check status
    sleep 2
    
    if systemctl is-active --quiet "$DAEMON_NAME"; then
        log_success "Service is running"
    else
        log_error "Service failed to start. Check: journalctl -u $DAEMON_NAME"
        exit 1
    fi
}

verify_installation() {
    log_info "Verifying installation..."
    
    # Check binary exists
    if [[ ! -x "$INSTALL_DIR/$DAEMON_NAME" ]]; then
        log_error "Binary not found at $INSTALL_DIR/$DAEMON_NAME"
        exit 1
    fi
    
    # Check service is enabled
    if ! systemctl is-enabled --quiet "$DAEMON_NAME"; then
        log_warn "Service is not enabled for boot"
    fi
    
    # Check socket exists
    SOCKET_PATH="$SOCKET_DIR/vpn-managerd.sock"
    if [[ -S "$SOCKET_PATH" ]]; then
        log_success "Socket created at $SOCKET_PATH"
    else
        log_warn "Socket not found (daemon may still be starting)"
    fi
    
    log_success "Installation verified"
}

print_status() {
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN} VPN Manager Daemon Installed!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "Binary:  $INSTALL_DIR/$DAEMON_NAME"
    echo "Service: $SERVICE_DIR/$DAEMON_NAME.service"
    echo "Socket:  $SOCKET_DIR/vpn-managerd.sock"
    echo ""
    echo "Useful commands:"
    echo "  sudo systemctl status $DAEMON_NAME  # Check status"
    echo "  sudo journalctl -u $DAEMON_NAME     # View logs"
    echo "  sudo systemctl restart $DAEMON_NAME # Restart"
    echo ""
}

# =============================================================================
# Main
# =============================================================================

main() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE} VPN Manager Daemon Installer${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
    
    check_root
    check_dependencies
    build_daemon
    install_binary
    install_service
    create_directories
    enable_service
    verify_installation
    print_status
}

main "$@"
