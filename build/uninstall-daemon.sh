#!/bin/bash
# =============================================================================
# VPN Manager Daemon Uninstallation Script
# =============================================================================
# This script removes the vpn-managerd daemon completely.
# Run with sudo: sudo ./uninstall-daemon.sh
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

# =============================================================================
# Uninstallation Steps
# =============================================================================

stop_service() {
    log_info "Stopping service..."
    
    if systemctl is-active --quiet "$DAEMON_NAME" 2>/dev/null; then
        systemctl stop "$DAEMON_NAME"
        log_success "Service stopped"
    else
        log_info "Service was not running"
    fi
}

disable_service() {
    log_info "Disabling service..."
    
    if systemctl is-enabled --quiet "$DAEMON_NAME" 2>/dev/null; then
        systemctl disable "$DAEMON_NAME"
        log_success "Service disabled"
    else
        log_info "Service was not enabled"
    fi
}

remove_service_file() {
    log_info "Removing service file..."
    
    SERVICE_FILE="$SERVICE_DIR/$DAEMON_NAME.service"
    
    if [[ -f "$SERVICE_FILE" ]]; then
        rm -f "$SERVICE_FILE"
        systemctl daemon-reload
        log_success "Service file removed"
    else
        log_info "Service file not found"
    fi
}

remove_binary() {
    log_info "Removing daemon binary..."
    
    BINARY_PATH="$INSTALL_DIR/$DAEMON_NAME"
    
    if [[ -f "$BINARY_PATH" ]]; then
        rm -f "$BINARY_PATH"
        log_success "Binary removed"
    else
        log_info "Binary not found"
    fi
}

cleanup_socket() {
    log_info "Cleaning up socket directory..."
    
    SOCKET_PATH="$SOCKET_DIR/vpn-managerd.sock"
    
    # Remove socket file if exists
    if [[ -S "$SOCKET_PATH" ]]; then
        rm -f "$SOCKET_PATH"
        log_success "Socket file removed"
    fi
    
    # Remove directory if empty
    if [[ -d "$SOCKET_DIR" ]]; then
        rmdir "$SOCKET_DIR" 2>/dev/null && log_success "Socket directory removed" || log_info "Socket directory not empty, kept"
    fi
}

reset_systemd() {
    log_info "Resetting systemd state..."
    
    systemctl daemon-reload
    systemctl reset-failed "$DAEMON_NAME" 2>/dev/null || true
    
    log_success "Systemd state reset"
}

print_status() {
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN} VPN Manager Daemon Uninstalled!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "The following were removed:"
    echo "  - Binary:  $INSTALL_DIR/$DAEMON_NAME"
    echo "  - Service: $SERVICE_DIR/$DAEMON_NAME.service"
    echo "  - Socket:  $SOCKET_DIR/vpn-managerd.sock"
    echo ""
    echo "Note: The VPN Manager GUI/CLI will now use pkexec"
    echo "      fallback for privileged operations."
    echo ""
}

# =============================================================================
# Main
# =============================================================================

main() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE} VPN Manager Daemon Uninstaller${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
    
    check_root
    
    # Confirm uninstallation
    if [[ "$1" != "-y" && "$1" != "--yes" ]]; then
        echo -e "${YELLOW}This will remove the vpn-managerd daemon.${NC}"
        echo -n "Continue? [y/N] "
        read -r response
        if [[ ! "$response" =~ ^[Yy]$ ]]; then
            log_info "Uninstallation cancelled"
            exit 0
        fi
    fi
    
    stop_service
    disable_service
    remove_service_file
    remove_binary
    cleanup_socket
    reset_systemd
    print_status
}

main "$@"
