#!/bin/bash
# Script to package VPN Manager as .deb
# Usage: ./scripts/build-deb.sh [version]

set -e

VERSION="${1:-1.0.0}"
PKG_NAME="vpn-manager"
ARCH="amd64"
PKG_DIR="${PKG_NAME}_${VERSION}_${ARCH}"

# Detect Go location
GO_BIN=$(command -v go 2>/dev/null || echo "/usr/local/go/bin/go")
if [ ! -x "$GO_BIN" ]; then
    # Try common locations
    for path in /usr/local/go/bin/go /usr/bin/go /snap/bin/go ~/go/bin/go; do
        if [ -x "$path" ]; then
            GO_BIN="$path"
            break
        fi
    done
fi

if [ ! -x "$GO_BIN" ]; then
    echo "❌ Error: Go not found. Install Go first."
    exit 1
fi

echo "🔨 Building VPN Manager v${VERSION}..."
echo "   Using Go: $GO_BIN"

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="${PROJECT_DIR}/build"

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

# Compile the binary
echo "📦 Compiling binary..."
cd "${PROJECT_DIR}"

# Preserve Go module cache from the original user
if [ -n "$SUDO_USER" ]; then
    export HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)
    export GOPATH="${HOME}/go"
    export GOCACHE="${HOME}/.cache/go-build"
fi

CGO_ENABLED=1 "$GO_BIN" build -ldflags="-s -w -X main.appVersion=${VERSION}" \
    -o "${BUILD_DIR}/${PKG_DIR}/usr/bin/${PKG_NAME}" \
    .

# Copy files
echo "📄 Copying files..."
cp "${PROJECT_DIR}/assets/vpn-manager.desktop" "${BUILD_DIR}/${PKG_DIR}/usr/share/applications/"
cp "${PROJECT_DIR}/assets/icons/vpn-manager.svg" "${BUILD_DIR}/${PKG_DIR}/usr/share/icons/hicolor/scalable/apps/"

# Copiar icono hicolor si existe
if [ -d "${PROJECT_DIR}/assets/icons/hicolor" ]; then
    cp -r "${PROJECT_DIR}/assets/icons/hicolor/"* "${BUILD_DIR}/${PKG_DIR}/usr/share/icons/hicolor/" 2>/dev/null || true
fi

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
Depends: openvpn | openvpn3, libgtk-4-1, libadwaita-1-0
Recommends: polkit-1
Installed-Size: $(du -sk "${BUILD_DIR}/${PKG_DIR}/usr" | cut -f1)
Maintainer: VPN Manager Team <vpn-manager@example.com>
Homepage: https://github.com/vpn-manager/vpn-manager
Description: Modern GTK4 VPN Manager for Linux
 VPN Manager is a modern OpenVPN client with GTK4 interface.
 Features profile management, secure credential storage,
 system tray integration, and split tunneling support.
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

echo "✅ VPN Manager installed successfully"
echo "   Run 'vpn-manager' or find it in the applications menu"
EOF
chmod 755 "${BUILD_DIR}/${PKG_DIR}/DEBIAN/postinst"

# Uninstall script
cat > "${BUILD_DIR}/${PKG_DIR}/DEBIAN/postrm" << 'EOF'
#!/bin/bash
set -e

if [ "$1" = "remove" ] || [ "$1" = "purge" ]; then
    # Clean configuration on purge
    if [ "$1" = "purge" ]; then
        rm -rf /home/*/.config/vpn-manager 2>/dev/null || true
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
find "${BUILD_DIR}/${PKG_DIR}" -type d -exec chmod 755 {} \;
find "${BUILD_DIR}/${PKG_DIR}/usr" -type f -exec chmod 644 {} \;
chmod 755 "${BUILD_DIR}/${PKG_DIR}/usr/bin/${PKG_NAME}"

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
echo "Para instalar:"
echo "  sudo dpkg -i ${PKG_DIR}.deb"
echo "  sudo apt-get install -f  # Si hay dependencias faltantes"
echo ""
echo "O directamente:"
echo "  sudo apt install ./${PKG_DIR}.deb"
