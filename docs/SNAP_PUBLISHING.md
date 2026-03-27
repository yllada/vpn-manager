# Snap Store Publishing Guide for VPN Manager

This guide covers building, testing, and publishing VPN Manager to the Snap Store.

## Prerequisites

### Install Required Tools

```bash
# Install snapcraft
sudo snap install snapcraft --classic

# Install LXD for clean containerized builds (recommended)
sudo snap install lxd
sudo lxd init --auto

# Add yourself to lxd group
sudo usermod -aG lxd $USER
newgrp lxd
```

## Building the Snap

### Quick Build

```bash
# From project root
./scripts/build-snap.sh
```

### Clean Rebuild

```bash
./scripts/build-snap.sh --clean
```

### Manual Build

```bash
cd /path/to/vpn-manager
snapcraft
```

### Debug Build Issues

```bash
# Enter build shell for debugging
./scripts/build-snap.sh --shell

# Or manually
snapcraft --shell
```

## Testing Locally

### Install the Snap

```bash
# Install with --dangerous for local unsigned snaps
sudo snap install --dangerous ./vpn-manager_1.9.0_amd64.snap
```

### Connect Required Interfaces

The snap needs several privileged interfaces for full functionality:

```bash
# REQUIRED: Network control for VPN connections
sudo snap connect vpn-manager:network-control

# REQUIRED: Firewall control for kill switch (iptables/nftables)
sudo snap connect vpn-manager:firewall-control

# REQUIRED: NetworkManager integration
sudo snap connect vpn-manager:network-manager

# REQUIRED: DNS configuration access
sudo snap connect vpn-manager:system-files

# OPTIONAL: Password manager for keyring
sudo snap connect vpn-manager:password-manager-service
```

### Verify Connections

```bash
snap connections vpn-manager
```

Expected output:
```
Interface                  Plug                              Slot                           Notes
content[gnome-42-2204]     vpn-manager:gnome-42-2204         gnome-42-2204:gnome-42-2204    -
content[gtk-3-themes]      vpn-manager:gtk-3-themes          gtk-common-themes:gtk-3-themes -
desktop                    vpn-manager:desktop               :desktop                       -
desktop-legacy             vpn-manager:desktop-legacy        :desktop-legacy                -
firewall-control           vpn-manager:firewall-control      :firewall-control              manual
home                       vpn-manager:home                  :home                          -
network                    vpn-manager:network               :network                       -
network-bind               vpn-manager:network-bind          :network-bind                  -
network-control            vpn-manager:network-control       :network-control               manual
network-manager            vpn-manager:network-manager       :network-manager               manual
system-files               vpn-manager:system-files          :system-files                  manual
wayland                    vpn-manager:wayland               :wayland                       -
x11                        vpn-manager:x11                   :x11                           -
```

### Test the Application

```bash
# Run GUI
vpn-manager

# Run CLI
vpn-manager --help
vpn-manager --status
vpn-manager --list

# Check logs
journalctl --user -u snap.vpn-manager.vpn-manager -f
```

### Uninstall Test Snap

```bash
sudo snap remove vpn-manager
```

## Publishing to Snap Store

### 1. Create Snapcraft Account

1. Go to https://snapcraft.io/account
2. Sign in with Ubuntu One account (or create one)
3. Accept the developer agreement

### 2. Register the Snap Name

```bash
# Login to snapcraft
snapcraft login

# Register the name (do this ONCE, name is globally unique)
snapcraft register vpn-manager
```

If the name is taken, you'll need to choose a different name or request a dispute.

### 3. Upload to Edge Channel (Testing)

```bash
# Build the snap
snapcraft

# Upload to edge channel
snapcraft upload --release=edge vpn-manager_1.9.0_amd64.snap
```

### 4. Test from Edge Channel

```bash
# Install from edge
sudo snap install vpn-manager --edge

# Test thoroughly
vpn-manager --version
```

### 5. Promote to Beta/Stable

```bash
# Once tested, promote to beta
snapcraft release vpn-manager <revision> beta

# After more testing, promote to stable
snapcraft release vpn-manager <revision> stable
```

Or use the web interface at https://snapcraft.io/vpn-manager/releases

### 6. Request Store Permissions

Some interfaces require store approval:

1. Go to https://snapcraft.io/vpn-manager/settings
2. Request permissions for:
   - `firewall-control` - Needed for kill switch
   - `network-control` - Needed for VPN management
   - `system-files` - Needed for DNS configuration

Provide justification explaining why these are needed for VPN functionality.

## Snap Interfaces Reference

| Interface | Purpose | Auto-connect |
|-----------|---------|--------------|
| `network` | Basic network access | Yes |
| `network-bind` | Listen on network ports | Yes |
| `network-control` | VPN connection management | **No** - requires manual connect |
| `network-manager` | NetworkManager integration | **No** - requires manual connect |
| `network-observe` | Read network state | Yes |
| `firewall-control` | iptables/nftables for kill switch | **No** - requires manual connect or store approval |
| `system-files` | DNS config (/etc/resolv.conf) | **No** - requires manual connect |
| `desktop` | Desktop integration | Yes |
| `wayland` | Wayland display | Yes |
| `x11` | X11 display | Yes |
| `home` | User home directory | Yes |
| `password-manager-service` | Keyring access | **No** - requires manual connect |

## Continuous Integration

### GitHub Actions Workflow

Create `.github/workflows/snap.yml`:

```yaml
name: Build Snap

on:
  push:
    tags:
      - 'v*'
  pull_request:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - uses: snapcore/action-build@v1
        id: build
        
      - uses: actions/upload-artifact@v4
        with:
          name: snap
          path: ${{ steps.build.outputs.snap }}
          
      # Publish to edge on main branch
      - uses: snapcore/action-publish@v1
        if: github.ref == 'refs/heads/main'
        env:
          SNAPCRAFT_STORE_CREDENTIALS: ${{ secrets.SNAPCRAFT_TOKEN }}
        with:
          snap: ${{ steps.build.outputs.snap }}
          release: edge
```

### Generate Snapcraft Token

```bash
# Generate token for CI
snapcraft export-login --snaps=vpn-manager --acls=package_upload snapcraft.login

# Add content to GitHub secrets as SNAPCRAFT_TOKEN
```

## Troubleshooting

### Build Fails with GTK4 Errors

Ensure core24 base is used and gnome extension is enabled:

```yaml
base: core24
apps:
  vpn-manager:
    extensions: [gnome]
```

### App Can't Access Network

Connect the network interfaces:

```bash
sudo snap connect vpn-manager:network-control
sudo snap connect vpn-manager:network-manager
```

### Kill Switch Doesn't Work

The `firewall-control` interface is required:

```bash
sudo snap connect vpn-manager:firewall-control
```

### Keyring/Credentials Not Working

Connect password manager interface:

```bash
sudo snap connect vpn-manager:password-manager-service
```

### DNS Settings Not Applied

Connect system-files interface:

```bash
sudo snap connect vpn-manager:system-files
```

### App Looks Wrong (Theme Issues)

Ensure gtk-common-themes is installed:

```bash
sudo snap install gtk-common-themes
```

### Debug Logs

```bash
# View snap logs
snap logs vpn-manager

# View journal logs
journalctl --user -u snap.vpn-manager.vpn-manager

# Run with debug output
vpn-manager --verbose
```

## Version Updates

1. Update version in `snap/snapcraft.yaml`
2. Update `assets/com.vpnmanager.app.metainfo.xml` with release notes
3. Build and upload:

```bash
snapcraft
snapcraft upload --release=edge vpn-manager_X.Y.Z_amd64.snap
```

## File Structure

```
vpn-manager/
├── snap/
│   ├── snapcraft.yaml          # Main snap configuration
│   └── gui/
│       ├── vpn-manager.desktop # Desktop entry for snap
│       └── vpn-manager.svg     # Icon for snap
├── assets/
│   └── com.vpnmanager.app.metainfo.xml  # AppStream metadata
└── scripts/
    └── build-snap.sh           # Build helper script
```

## Resources

- [Snapcraft Documentation](https://snapcraft.io/docs)
- [Snap Interfaces](https://snapcraft.io/docs/supported-interfaces)
- [GNOME Extension](https://snapcraft.io/docs/gnome-extension)
- [Publishing to Snap Store](https://snapcraft.io/docs/releasing-your-app)
- [Store Permissions](https://snapcraft.io/docs/permission-requests)
