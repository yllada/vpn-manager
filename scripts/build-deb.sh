#!/bin/bash
# =============================================================================
# Script to package VPN Manager as .deb
# =============================================================================
# Usage: 
#   ./scripts/build-deb.sh [version]                      # Build from source
#   ./scripts/build-deb.sh [version] [binary] [daemon]    # Use pre-built binaries
#
# Examples:
#   ./scripts/build-deb.sh 1.0.2                          # Compile and package
#   ./scripts/build-deb.sh 1.0.2 ./vpn-manager ./vpn-managerd  # Package existing binaries
# =============================================================================

set -euo pipefail

VERSION="${1:-1.0.0}"
PREBUILT_BINARY="${2:-}"
PREBUILT_DAEMON="${3:-}"
PKG_NAME="vpn-manager"
DAEMON_NAME="vpn-managerd"
ARCH="amd64"
PKG_DIR="${PKG_NAME}_${VERSION}_${ARCH}"

echo "🔨 Packaging VPN Manager v${VERSION}..."

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="${PROJECT_DIR}/build-pkg"

# Clean and create directories
rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}/${PKG_DIR}"
cd "${BUILD_DIR}/${PKG_DIR}"

# .deb package structure
mkdir -p DEBIAN
mkdir -p usr/bin
mkdir -p usr/share/applications
mkdir -p usr/share/icons/hicolor/scalable/apps
mkdir -p usr/share/icons/hicolor/256x256/apps
mkdir -p usr/share/doc/${PKG_NAME}
mkdir -p lib/systemd/system

# Get or build the binaries
cd "${PROJECT_DIR}"

# Detect Go location (needed if building from source)
find_go() {
    local GO_BIN=$(command -v go 2>/dev/null || echo "")
    if [[ -z "$GO_BIN" ]]; then
        for path in /usr/local/go/bin/go /usr/bin/go /snap/bin/go ~/go/bin/go; do
            if [[ -x "$path" ]]; then
                GO_BIN="$path"
                break
            fi
        done
    fi
    echo "$GO_BIN"
}

# Build or copy main binary
if [[ -n "$PREBUILT_BINARY" && -f "$PREBUILT_BINARY" ]]; then
    echo "📦 Using pre-built binary: $PREBUILT_BINARY"
    cp "$PREBUILT_BINARY" "${BUILD_DIR}/${PKG_DIR}/usr/bin/${PKG_NAME}"
else
    echo "📦 Compiling main binary from source..."
    
    GO_BIN=$(find_go)
    if [[ -z "$GO_BIN" || ! -x "$GO_BIN" ]]; then
        echo "❌ Error: Go not found. Install Go or provide pre-built binary."
        echo "   Usage: $0 $VERSION /path/to/vpn-manager /path/to/vpn-managerd"
        exit 1
    fi
    
    echo "   Using Go: $GO_BIN"
    
    # Preserve Go module cache from the original user
    if [[ -n "${SUDO_USER:-}" ]]; then
        export HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)
        export GOPATH="${HOME}/go"
        export GOCACHE="${HOME}/.cache/go-build"
    fi
    
    CGO_ENABLED=1 "$GO_BIN" build \
        -trimpath \
        -ldflags="-s -w -X main.appVersion=${VERSION}" \
        -o "${BUILD_DIR}/${PKG_DIR}/usr/bin/${PKG_NAME}" \
        ./cmd/vpn-manager
fi

# Build or copy daemon binary
if [[ -n "$PREBUILT_DAEMON" && -f "$PREBUILT_DAEMON" ]]; then
    echo "📦 Using pre-built daemon: $PREBUILT_DAEMON"
    cp "$PREBUILT_DAEMON" "${BUILD_DIR}/${PKG_DIR}/usr/bin/${DAEMON_NAME}"
else
    echo "📦 Compiling daemon from source..."
    
    GO_BIN=$(find_go)
    if [[ -z "$GO_BIN" || ! -x "$GO_BIN" ]]; then
        echo "❌ Error: Go not found. Install Go or provide pre-built daemon."
        exit 1
    fi
    
    # Preserve Go module cache from the original user
    if [[ -n "${SUDO_USER:-}" ]]; then
        export HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)
        export GOPATH="${HOME}/go"
        export GOCACHE="${HOME}/.cache/go-build"
    fi
    
    CGO_ENABLED=1 "$GO_BIN" build \
        -trimpath \
        -ldflags="-s -w" \
        -o "${BUILD_DIR}/${PKG_DIR}/usr/bin/${DAEMON_NAME}" \
        ./cmd/vpn-managerd
fi

# Copy files
echo "📄 Copying files..."
cp "${PROJECT_DIR}/assets/vpn-manager.desktop" "${BUILD_DIR}/${PKG_DIR}/usr/share/applications/"
cp "${PROJECT_DIR}/assets/icons/vpn-manager.svg" "${BUILD_DIR}/${PKG_DIR}/usr/share/icons/hicolor/scalable/apps/"

# Copy hicolor icons if exist
if [ -d "${PROJECT_DIR}/assets/icons/hicolor" ]; then
    cp -r "${PROJECT_DIR}/assets/icons/hicolor/"* "${BUILD_DIR}/${PKG_DIR}/usr/share/icons/hicolor/" 2>/dev/null || true
fi

# Copy systemd service file
cp "${PROJECT_DIR}/build/systemd/vpn-managerd.service" "${BUILD_DIR}/${PKG_DIR}/lib/systemd/system/"

# Documentation
cp "${PROJECT_DIR}/README.md" "${BUILD_DIR}/${PKG_DIR}/usr/share/doc/${PKG_NAME}/"
cp "${PROJECT_DIR}/LICENSE" "${BUILD_DIR}/${PKG_DIR}/usr/share/doc/${PKG_NAME}/copyright" 2>/dev/null || \
    echo "Copyright (c) 2026 VPN Manager Team. MIT License." > "${BUILD_DIR}/${PKG_DIR}/usr/share/doc/${PKG_NAME}/copyright"

# Create control file
cat > "${BUILD_DIR}/${PKG_DIR}/DEBIAN/control" << EOF
Package: ${PKG_NAME}
Version: ${VERSION}
Section: net
Priority: optional
Architecture: ${ARCH}
Depends: libgtk-4-1, libadwaita-1-0
Recommends: openvpn | openvpn3, wireguard-tools, tailscale
Installed-Size: $(du -sk "${BUILD_DIR}/${PKG_DIR}/usr" | cut -f1)
Maintainer: VPN Manager Team <yadian.llada@gmail.com>
Homepage: https://github.com/yllada/vpn-manager
Description: Modern GTK4 VPN Manager for Linux
 VPN Manager is a modern VPN client with GTK4 interface supporting
 OpenVPN, WireGuard, and Tailscale. Features include profile management,
 secure credential storage, system tray integration, traffic statistics,
 kill switch, DNS/IPv6 leak protection, and split tunneling.
 .
 Includes vpn-managerd daemon for privileged operations.
EOF

# Post-installation script
cat > "${BUILD_DIR}/${PKG_DIR}/DEBIAN/postinst" << 'EOF'
#!/bin/bash
set -e

# Update icon cache
if command -v gtk-update-icon-cache &> /dev/null; then
    gtk-update-icon-cache -f -t /usr/share/icons/hicolor 2>/dev/null || true
fi

# Update applications database
if command -v update-desktop-database &> /dev/null; then
    update-desktop-database /usr/share/applications 2>/dev/null || true
fi

# Reload systemd
systemctl daemon-reload 2>/dev/null || true

# Enable and start the daemon
if systemctl is-system-running --quiet 2>/dev/null; then
    systemctl enable vpn-managerd 2>/dev/null || true
    systemctl start vpn-managerd 2>/dev/null || true
fi

echo "✅ VPN Manager installed successfully"
echo "   The vpn-managerd daemon has been enabled and started."
echo "   Run 'vpn-manager' or find it in the applications menu."
EOF
chmod 755 "${BUILD_DIR}/${PKG_DIR}/DEBIAN/postinst"

# Pre-removal script (stop daemon before removal)
cat > "${BUILD_DIR}/${PKG_DIR}/DEBIAN/prerm" << 'EOF'
#!/bin/bash
set -e

if [ "$1" = "remove" ] || [ "$1" = "upgrade" ]; then
    # Stop the daemon before removal
    if systemctl is-active --quiet vpn-managerd 2>/dev/null; then
        systemctl stop vpn-managerd 2>/dev/null || true
    fi
    
    if [ "$1" = "remove" ]; then
        systemctl disable vpn-managerd 2>/dev/null || true
    fi
fi
EOF
chmod 755 "${BUILD_DIR}/${PKG_DIR}/DEBIAN/prerm"

# Post-removal script
cat > "${BUILD_DIR}/${PKG_DIR}/DEBIAN/postrm" << 'EOF'
#!/bin/bash
set -e

if [ "$1" = "remove" ] || [ "$1" = "purge" ]; then
    # Reload systemd to forget the service
    systemctl daemon-reload 2>/dev/null || true
    
    # Clean configuration on purge
    if [ "$1" = "purge" ]; then
        rm -rf /home/*/.config/vpn-manager 2>/dev/null || true
        rm -rf /var/run/vpn-manager 2>/dev/null || true
    fi
    
    # Update icon cache
    if command -v gtk-update-icon-cache &> /dev/null; then
        gtk-update-icon-cache -f -t /usr/share/icons/hicolor 2>/dev/null || true
    fi
fi
EOF
chmod 755 "${BUILD_DIR}/${PKG_DIR}/DEBIAN/postrm"

# Set correct permissions
chmod 755 "${BUILD_DIR}/${PKG_DIR}/usr/bin/${PKG_NAME}"
chmod 755 "${BUILD_DIR}/${PKG_DIR}/usr/bin/${DAEMON_NAME}"
chmod 644 "${BUILD_DIR}/${PKG_DIR}/lib/systemd/system/vpn-managerd.service"
find "${BUILD_DIR}/${PKG_DIR}" -type d -exec chmod 755 {} \;
find "${BUILD_DIR}/${PKG_DIR}/usr/share" -type f -exec chmod 644 {} \;

# Build the .deb package
echo "📦 Creating .deb package..."
cd "${BUILD_DIR}"
dpkg-deb --build --root-owner-group "${PKG_DIR}"

# Move to project root
mv "${PKG_DIR}.deb" "${PROJECT_DIR}/"

# Cleanup
rm -rf "${BUILD_DIR}"

echo ""
echo "✅ Package created: ${PROJECT_DIR}/${PKG_DIR}.deb"
echo ""
echo "Contents:"
echo "  - /usr/bin/vpn-manager (GUI/CLI application)"
echo "  - /usr/bin/vpn-managerd (privileged operations daemon)"
echo "  - /lib/systemd/system/vpn-managerd.service"
echo ""
echo "Para instalar:"
echo "  sudo dpkg -i ${PKG_DIR}.deb"
echo "  sudo apt-get install -f  # Si hay dependencias faltantes"
echo ""
echo "O directamente:"
echo "  sudo apt install ./${PKG_DIR}.deb"
