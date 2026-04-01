#!/bin/bash
# =============================================================================
# Update APT Repository
# =============================================================================
# This script updates the APT repository in docs/apt/ with a new .deb package.
# It generates the necessary metadata files and signs them with GPG.
#
# Usage: ./scripts/update-apt-repo.sh <path-to-deb>
#
# Requirements:
# - dpkg-scanpackages (from dpkg-dev)
# - gpg with signing key imported
# - gzip
# =============================================================================

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Configuration
REPO_DIR="docs/apt"
DIST="stable"
COMPONENT="main"
ARCH="amd64"
GPG_KEY_ID="${GPG_KEY_ID:-VPN Manager APT Repository}"

# Validate arguments
if [[ $# -lt 1 ]]; then
    log_error "Usage: $0 <path-to-deb>"
    exit 1
fi

DEB_FILE="$1"

if [[ ! -f "$DEB_FILE" ]]; then
    log_error "DEB file not found: $DEB_FILE"
    exit 1
fi

# Extract package info from .deb
log_info "Extracting package info from $DEB_FILE..."
PKG_NAME=$(dpkg-deb --field "$DEB_FILE" Package)
PKG_VERSION=$(dpkg-deb --field "$DEB_FILE" Version)
DEB_FILENAME="${PKG_NAME}_${PKG_VERSION}_${ARCH}.deb"

log_info "Package: $PKG_NAME v$PKG_VERSION"

# Create directory structure
log_info "Creating APT repository structure..."
mkdir -p "$REPO_DIR/dists/$DIST/$COMPONENT/binary-$ARCH"
mkdir -p "$REPO_DIR/pool/$COMPONENT/${PKG_NAME:0:1}/$PKG_NAME"

# Copy .deb to pool
POOL_PATH="pool/$COMPONENT/${PKG_NAME:0:1}/$PKG_NAME/$DEB_FILENAME"
cp "$DEB_FILE" "$REPO_DIR/$POOL_PATH"
log_info "Copied $DEB_FILENAME to $POOL_PATH"

# Generate Packages file
log_info "Generating Packages index..."
cd "$REPO_DIR"

# Use dpkg-scanpackages to generate Packages file
dpkg-scanpackages --arch "$ARCH" pool/ > "dists/$DIST/$COMPONENT/binary-$ARCH/Packages"

# Create compressed version
gzip -9 -k -f "dists/$DIST/$COMPONENT/binary-$ARCH/Packages"

# Generate Release file for the component
log_info "Generating component Release file..."
cat > "dists/$DIST/$COMPONENT/binary-$ARCH/Release" << EOF
Archive: $DIST
Component: $COMPONENT
Architecture: $ARCH
EOF

# Generate main Release file
log_info "Generating main Release file..."
PACKAGES_SIZE=$(wc -c < "dists/$DIST/$COMPONENT/binary-$ARCH/Packages")
PACKAGES_GZ_SIZE=$(wc -c < "dists/$DIST/$COMPONENT/binary-$ARCH/Packages.gz")
PACKAGES_MD5=$(md5sum "dists/$DIST/$COMPONENT/binary-$ARCH/Packages" | cut -d' ' -f1)
PACKAGES_GZ_MD5=$(md5sum "dists/$DIST/$COMPONENT/binary-$ARCH/Packages.gz" | cut -d' ' -f1)
PACKAGES_SHA256=$(sha256sum "dists/$DIST/$COMPONENT/binary-$ARCH/Packages" | cut -d' ' -f1)
PACKAGES_GZ_SHA256=$(sha256sum "dists/$DIST/$COMPONENT/binary-$ARCH/Packages.gz" | cut -d' ' -f1)

cat > "dists/$DIST/Release" << EOF
Origin: VPN Manager
Label: VPN Manager
Suite: $DIST
Codename: $DIST
Architectures: $ARCH
Components: $COMPONENT
Description: VPN Manager APT Repository
Date: $(date -Ru)
MD5Sum:
 $PACKAGES_MD5 $PACKAGES_SIZE $COMPONENT/binary-$ARCH/Packages
 $PACKAGES_GZ_MD5 $PACKAGES_GZ_SIZE $COMPONENT/binary-$ARCH/Packages.gz
SHA256:
 $PACKAGES_SHA256 $PACKAGES_SIZE $COMPONENT/binary-$ARCH/Packages
 $PACKAGES_GZ_SHA256 $PACKAGES_GZ_SIZE $COMPONENT/binary-$ARCH/Packages.gz
EOF

# Sign the Release file
log_info "Signing Release file with GPG..."

# Create detached signature (Release.gpg)
gpg --batch --yes --armor --default-key "$GPG_KEY_ID" \
    --output "dists/$DIST/Release.gpg" \
    --detach-sign "dists/$DIST/Release"

# Create inline signature (InRelease)
gpg --batch --yes --armor --default-key "$GPG_KEY_ID" \
    --output "dists/$DIST/InRelease" \
    --clearsign "dists/$DIST/Release"

cd - > /dev/null

log_info "APT repository updated successfully!"
log_info ""
log_info "Repository contents:"
find "$REPO_DIR" -type f -name "*.deb" -o -name "Packages*" -o -name "Release*" -o -name "InRelease" | sort

log_info ""
log_info "Users can add this repository with:"
log_info "  curl -fsSL https://yllada.github.io/vpn-manager/apt/gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/vpn-manager.gpg"
log_info "  echo \"deb [signed-by=/usr/share/keyrings/vpn-manager.gpg] https://yllada.github.io/vpn-manager/apt stable main\" | sudo tee /etc/apt/sources.list.d/vpn-manager.list"
log_info "  sudo apt update && sudo apt install vpn-manager"
