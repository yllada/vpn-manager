#!/bin/bash
# =============================================================================
# Script to build VPN Manager Snap package
# =============================================================================
# Usage:
#   ./scripts/build-snap.sh              # Build snap locally
#   ./scripts/build-snap.sh --clean      # Clean and rebuild
#   ./scripts/build-snap.sh --shell      # Enter snapcraft shell for debugging
#
# Requirements:
#   - snapcraft: sudo snap install snapcraft --classic
#   - LXD (recommended): sudo snap install lxd && sudo lxd init --auto
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
SNAP_DIR="${PROJECT_DIR}/snap"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Check snapcraft is installed
check_snapcraft() {
    if ! command -v snapcraft &> /dev/null; then
        log_error "snapcraft not found. Install with: sudo snap install snapcraft --classic"
        exit 1
    fi
    log_info "snapcraft version: $(snapcraft --version)"
}

# Check LXD is available (recommended for clean builds)
check_lxd() {
    if command -v lxd &> /dev/null; then
        log_info "LXD available - will use containerized builds"
        return 0
    else
        log_warn "LXD not found - will use host build (may be slower/dirtier)"
        log_warn "Install LXD for cleaner builds: sudo snap install lxd && sudo lxd init --auto"
        return 1
    fi
}

# Clean previous builds
clean_build() {
    log_info "Cleaning previous builds..."
    cd "$PROJECT_DIR"
    
    # Remove snap build artifacts
    rm -rf parts/ prime/ stage/ *.snap 2>/dev/null || true
    
    # Clean snapcraft state
    snapcraft clean 2>/dev/null || true
    
    log_success "Build cleaned"
}

# Build the snap
build_snap() {
    cd "$PROJECT_DIR"
    log_info "Building snap package..."
    
    # Determine build mode
    if check_lxd; then
        log_info "Using LXD for build isolation"
        snapcraft
    else
        log_warn "Building on host (use --destructive-mode if needed)"
        snapcraft --destructive-mode
    fi
    
    # Find the built snap
    SNAP_FILE=$(ls -1 *.snap 2>/dev/null | head -1)
    if [ -n "$SNAP_FILE" ]; then
        log_success "Snap built: ${SNAP_FILE}"
        log_info "File size: $(du -h "$SNAP_FILE" | cut -f1)"
        echo ""
        echo "To install locally:"
        echo "  sudo snap install --dangerous ./${SNAP_FILE}"
        echo ""
        echo "To connect required interfaces:"
        echo "  sudo snap connect vpn-manager:firewall-control"
        echo "  sudo snap connect vpn-manager:network-control"
        echo "  sudo snap connect vpn-manager:network-manager"
        echo "  sudo snap connect vpn-manager:system-files"
    else
        log_error "Snap file not found after build"
        exit 1
    fi
}

# Enter snapcraft shell for debugging
debug_shell() {
    cd "$PROJECT_DIR"
    log_info "Entering snapcraft shell..."
    snapcraft --shell
}

# Main
main() {
    case "${1:-}" in
        --clean)
            check_snapcraft
            clean_build
            build_snap
            ;;
        --shell)
            check_snapcraft
            debug_shell
            ;;
        --help|-h)
            echo "Usage: $0 [--clean|--shell|--help]"
            echo ""
            echo "Options:"
            echo "  --clean    Clean and rebuild"
            echo "  --shell    Enter snapcraft shell for debugging"
            echo "  --help     Show this help"
            ;;
        *)
            check_snapcraft
            build_snap
            ;;
    esac
}

main "$@"
