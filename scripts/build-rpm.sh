#!/bin/bash
# =============================================================================
# Script to package VPN Manager as .rpm (Fedora/RHEL/CentOS)
# =============================================================================
# Usage: 
#   ./scripts/build-rpm.sh [version]                      # Build from source
#   ./scripts/build-rpm.sh [version] [binary] [daemon]    # Use pre-built binaries
#
# Examples:
#   ./scripts/build-rpm.sh 1.0.2                          # Compile and package
#   ./scripts/build-rpm.sh 1.0.2 ./vpn-manager ./vpn-managerd  # Package existing binaries
#
# Requirements:
#   - rpm-build package (dnf install rpm-build)
# =============================================================================

set -euo pipefail

VERSION="${1:-1.0.0}"
PREBUILT_BINARY="${2:-}"
PREBUILT_DAEMON="${3:-}"
PKG_NAME="vpn-manager"
DAEMON_NAME="vpn-managerd"
ARCH="x86_64"
RELEASE="1"

echo "🔨 Packaging VPN Manager v${VERSION} as RPM..."

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="${PROJECT_DIR}/rpmbuild"

# Clean and create RPM build structure
rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

# Create source tarball structure
SOURCE_DIR="${BUILD_DIR}/SOURCES/${PKG_NAME}-${VERSION}"
mkdir -p "${SOURCE_DIR}"

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

# Get or build the binaries
cd "${PROJECT_DIR}"

# Build or copy main binary
if [[ -n "$PREBUILT_BINARY" && -f "$PREBUILT_BINARY" ]]; then
    echo "📦 Using pre-built binary: $PREBUILT_BINARY"
    cp "$PREBUILT_BINARY" "${SOURCE_DIR}/${PKG_NAME}"
else
    echo "📦 Compiling main binary from source..."
    
    GO_BIN=$(find_go)
    if [[ -z "$GO_BIN" || ! -x "$GO_BIN" ]]; then
        echo "❌ Error: Go not found. Install Go or provide pre-built binary."
        echo "   Usage: $0 $VERSION /path/to/vpn-manager /path/to/vpn-managerd"
        exit 1
    fi
    
    echo "   Using Go: $GO_BIN"
    
    CGO_ENABLED=1 "$GO_BIN" build \
        -trimpath \
        -ldflags="-s -w -X main.appVersion=${VERSION}" \
        -o "${SOURCE_DIR}/${PKG_NAME}" \
        ./cmd/vpn-manager
fi

# Build or copy daemon binary
if [[ -n "$PREBUILT_DAEMON" && -f "$PREBUILT_DAEMON" ]]; then
    echo "📦 Using pre-built daemon: $PREBUILT_DAEMON"
    cp "$PREBUILT_DAEMON" "${SOURCE_DIR}/${DAEMON_NAME}"
else
    echo "📦 Compiling daemon from source..."
    
    GO_BIN=$(find_go)
    if [[ -z "$GO_BIN" || ! -x "$GO_BIN" ]]; then
        echo "❌ Error: Go not found. Install Go or provide pre-built daemon."
        exit 1
    fi
    
    CGO_ENABLED=1 "$GO_BIN" build \
        -trimpath \
        -ldflags="-s -w" \
        -o "${SOURCE_DIR}/${DAEMON_NAME}" \
        ./cmd/vpn-managerd
fi

# Copy assets to source directory
echo "📄 Copying files..."
cp "${PROJECT_DIR}/assets/vpn-manager.desktop" "${SOURCE_DIR}/"
cp "${PROJECT_DIR}/assets/icons/vpn-manager.svg" "${SOURCE_DIR}/"
cp "${PROJECT_DIR}/README.md" "${SOURCE_DIR}/"
cp "${PROJECT_DIR}/LICENSE" "${SOURCE_DIR}/" 2>/dev/null || \
    echo "MIT License" > "${SOURCE_DIR}/LICENSE"

# Copy systemd service file
cp "${PROJECT_DIR}/build/systemd/vpn-managerd.service" "${SOURCE_DIR}/"

# Copy hicolor icons if they exist
if [ -d "${PROJECT_DIR}/assets/icons/hicolor" ]; then
    cp -r "${PROJECT_DIR}/assets/icons/hicolor" "${SOURCE_DIR}/"
fi

# Create source tarball
cd "${BUILD_DIR}/SOURCES"
tar -czvf "${PKG_NAME}-${VERSION}.tar.gz" "${PKG_NAME}-${VERSION}"
rm -rf "${PKG_NAME}-${VERSION}"

# Create RPM spec file
cat > "${BUILD_DIR}/SPECS/${PKG_NAME}.spec" << EOF
Name:           ${PKG_NAME}
Version:        ${VERSION}
Release:        ${RELEASE}%{?dist}
Summary:        Modern GTK4 VPN Manager for Linux

License:        MIT
URL:            https://github.com/yllada/vpn-manager
Source0:        %{name}-%{version}.tar.gz

# Disable debug package generation (binary is pre-built)
%global debug_package %{nil}

# Dependencies
Requires:       gtk4 >= 4.10
Requires:       libadwaita >= 1.3
Requires:       systemd
Recommends:     (openvpn or openvpn3)
Recommends:     wireguard-tools
Recommends:     tailscale

# Build requirements (for rpmbuild itself)
BuildRequires:  desktop-file-utils
BuildRequires:  systemd-rpm-macros

%description
VPN Manager is a modern VPN client with GTK4 interface supporting
OpenVPN, WireGuard, and Tailscale. Features include profile management,
secure credential storage, system tray integration, traffic statistics,
kill switch, DNS/IPv6 leak protection, and split tunneling.

Includes vpn-managerd daemon for privileged operations.

%prep
%setup -q

%install
# Create directories
mkdir -p %{buildroot}%{_bindir}
mkdir -p %{buildroot}%{_unitdir}
mkdir -p %{buildroot}%{_datadir}/applications
mkdir -p %{buildroot}%{_datadir}/icons/hicolor/scalable/apps
mkdir -p %{buildroot}%{_docdir}/%{name}

# Main binary
install -Dm755 %{name} %{buildroot}%{_bindir}/%{name}

# Daemon binary
install -Dm755 ${DAEMON_NAME} %{buildroot}%{_bindir}/${DAEMON_NAME}

# Systemd service (use explicit path, not macro in install target)
install -Dm644 ${DAEMON_NAME}.service %{buildroot}/usr/lib/systemd/system/${DAEMON_NAME}.service

# Desktop file
install -Dm644 %{name}.desktop %{buildroot}%{_datadir}/applications/%{name}.desktop

# Icons
install -Dm644 %{name}.svg %{buildroot}%{_datadir}/icons/hicolor/scalable/apps/%{name}.svg

# Copy hicolor icons if they exist in source
if [ -d hicolor ]; then
    cp -r hicolor/* %{buildroot}%{_datadir}/icons/hicolor/
fi

# Documentation
install -Dm644 README.md %{buildroot}%{_docdir}/%{name}/README.md
install -Dm644 LICENSE %{buildroot}%{_docdir}/%{name}/LICENSE

%post
# Update icon cache
if [ -x /usr/bin/gtk-update-icon-cache ]; then
    /usr/bin/gtk-update-icon-cache -f -t %{_datadir}/icons/hicolor &>/dev/null || :
fi

# Update desktop database
if [ -x /usr/bin/update-desktop-database ]; then
    /usr/bin/update-desktop-database %{_datadir}/applications &>/dev/null || :
fi

# Enable and start daemon
%systemd_post ${DAEMON_NAME}.service

%preun
%systemd_preun ${DAEMON_NAME}.service

%postun
# Update icon cache on uninstall
if [ \$1 -eq 0 ]; then
    if [ -x /usr/bin/gtk-update-icon-cache ]; then
        /usr/bin/gtk-update-icon-cache -f -t %{_datadir}/icons/hicolor &>/dev/null || :
    fi
fi

%systemd_postun_with_restart ${DAEMON_NAME}.service

%files
%license LICENSE
%doc README.md
%{_bindir}/%{name}
%{_bindir}/${DAEMON_NAME}
/usr/lib/systemd/system/${DAEMON_NAME}.service
%{_datadir}/applications/%{name}.desktop
%{_datadir}/icons/hicolor/scalable/apps/%{name}.svg
%{_docdir}/%{name}/

%changelog
* $(date '+%a %b %d %Y') VPN Manager Team <yadian.llada@gmail.com> - ${VERSION}-${RELEASE}
- Release ${VERSION}
- Includes vpn-managerd daemon for privileged operations
EOF

# Build the RPM
echo "📦 Building RPM package..."
cd "${BUILD_DIR}"

# Use --nodeps to skip BuildRequires check (we're cross-building on Ubuntu, not Fedora)
rpmbuild --define "_topdir ${BUILD_DIR}" \
         --define "_rpmdir ${BUILD_DIR}/RPMS" \
         --nodeps \
         -bb "SPECS/${PKG_NAME}.spec"

# Find and move the built RPM
RPM_FILE=$(find "${BUILD_DIR}/RPMS" -name "*.rpm" -type f | head -1)

if [[ -n "$RPM_FILE" && -f "$RPM_FILE" ]]; then
    # Rename to standard format
    FINAL_NAME="${PKG_NAME}-${VERSION}-${RELEASE}.${ARCH}.rpm"
    mv "$RPM_FILE" "${PROJECT_DIR}/${FINAL_NAME}"
    
    # Cleanup
    rm -rf "${BUILD_DIR}"
    
    echo ""
    echo "✅ Package created: ${PROJECT_DIR}/${FINAL_NAME}"
    echo ""
    echo "Contents:"
    echo "  - /usr/bin/vpn-manager (GUI/CLI application)"
    echo "  - /usr/bin/vpn-managerd (privileged operations daemon)"
    echo "  - /usr/lib/systemd/system/vpn-managerd.service"
    echo ""
    echo "Para instalar:"
    echo "  sudo dnf install ./${FINAL_NAME}"
    echo ""
    echo "O con rpm:"
    echo "  sudo rpm -ivh ${FINAL_NAME}"
else
    echo "❌ Error: RPM file not found after build"
    exit 1
fi
