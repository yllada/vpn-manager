# VPN Manager Daemon

The VPN Manager Daemon (`vpn-managerd`) handles **all** privileged operations that require root access. The daemon is **required** for VPN Manager to function.

## Supported Operations

| Category | Operations |
|----------|------------|
| **Kill Switch** | iptables/nftables firewall rules for traffic blocking |
| **DNS Protection** | DNS firewall rules and DoT blocking |
| **IPv6 Protection** | Disable IPv6 via sysctl and firewall |
| **LAN Gateway** | IP forwarding and NAT rules for Tailscale exit node sharing |
| **OpenVPN** | Process lifecycle management (start/stop/status) |
| **WireGuard** | Interface management via `wg-quick` |
| **Tailscale CLI** | Full CLI wrapper (up/down/set/login/logout/set_operator) |
| **App Tunnel** | Per-app VPN routing via cgroups and policy routing |

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        User Space                                │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  VPN Manager GUI/CLI (runs as regular user)             │    │
│  │  - All UI/UX logic                                      │    │
│  │  - Configuration management                             │    │
│  │  - Statistics and monitoring                            │    │
│  └─────────────────┬───────────────────────────────────────┘    │
│                    │ Unix Socket (JSON-RPC 2.0)                  │
│                    ▼                                             │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  vpn-managerd (runs as root via systemd)                │    │
│  │  - Kill Switch (iptables/nftables)                      │    │
│  │  - DNS Protection (firewall rules)                      │    │
│  │  - IPv6 Protection (sysctl + firewall)                  │    │
│  │  - LAN Gateway (IP forwarding + NAT)                    │    │
│  │  - OpenVPN process management                           │    │
│  │  - WireGuard interface management                       │    │
│  │  - Tailscale CLI operations                             │    │
│  │  - App Tunnel (cgroups + policy routing)                │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

## Installation

### From Packages

The daemon is automatically installed and enabled when you install via `.deb` or `.rpm` packages.

### Manual Installation

```bash
cd build
sudo ./install-daemon.sh
```

This will:
1. Build the daemon binary
2. Install it to `/usr/bin/vpn-managerd`
3. Install systemd service file
4. Enable and start the service

### Verify Installation

```bash
# Check service status
sudo systemctl status vpn-managerd

# View logs
sudo journalctl -u vpn-managerd -f

# Check socket exists
ls -la /var/run/vpn-manager/vpn-managerd.sock
```

## Uninstallation

```bash
cd build
sudo ./uninstall-daemon.sh
```

Or with auto-confirm:

```bash
sudo ./uninstall-daemon.sh -y
```

## Protocol

The daemon uses JSON-RPC 2.0 over a Unix socket:

- **Socket Path**: `/var/run/vpn-manager/vpn-managerd.sock`
- **Protocol**: JSON-RPC 2.0 with newline-delimited messages
- **Authentication**: SO_PEERCRED (verifies client identity via Unix socket)
- **Permissions**: Socket has mode 0666, allowing any local user to connect

### Available Methods

#### Core Methods

| Method | Description |
|--------|-------------|
| `ping` | Health check |
| `status` | Get daemon status and feature states |

#### Kill Switch & Protection

| Method | Description |
|--------|-------------|
| `killswitch.enable` | Enable kill switch (params: `interface`, `allowLan`) |
| `killswitch.disable` | Disable kill switch |
| `dns.enable` | Enable DNS protection (params: `interface`) |
| `dns.disable` | Disable DNS protection |
| `ipv6.enable` | Enable IPv6 protection |
| `ipv6.disable` | Disable IPv6 protection |
| `gateway.enable` | Enable LAN gateway (params: `interface`, `lanInterface`) |
| `gateway.disable` | Disable LAN gateway |

#### VPN Process Management

| Method | Description |
|--------|-------------|
| `openvpn.start` | Start OpenVPN process (params: `configPath`, `credentials`) |
| `openvpn.stop` | Stop OpenVPN process (params: `profileId`) |
| `openvpn.status` | Get OpenVPN process status (params: `profileId`) |
| `wireguard.up` | Bring up WireGuard interface (params: `configPath`) |
| `wireguard.down` | Bring down WireGuard interface (params: `interface`) |

#### Tailscale CLI

| Method | Description |
|--------|-------------|
| `tailscale.up` | Connect Tailscale (params: `exitNode`, `exitNodeAllowLAN`, `acceptRoutes`, etc.) |
| `tailscale.down` | Disconnect Tailscale |
| `tailscale.set` | Configure Tailscale settings (params: same as `up`) |
| `tailscale.login` | Start login flow (params: `reauthenticate`) |
| `tailscale.logout` | Logout from Tailscale |
| `tailscale.set_operator` | Set Tailscale operator (params: `username`) |

#### App Tunnel (Split Tunneling)

| Method | Description |
|--------|-------------|
| `apptunnel.setup` | Setup app tunnel infrastructure (cgroups, iptables, routes) |
| `apptunnel.cleanup` | Remove app tunnel infrastructure |
| `apptunnel.run` | Run a command inside the tunnel (params: `command`, `args`, `uid`, `gid`) |

### Example Request

```json
{"jsonrpc":"2.0","method":"killswitch.enable","params":{"interface":"tun0","allowLan":true},"id":1}
```

### Example Response

```json
{"jsonrpc":"2.0","result":{"success":true},"id":1}
```

### Error Response

```json
{"jsonrpc":"2.0","error":{"code":-32000,"message":"failed to enable kill switch: iptables not found"},"id":1}
```

## Security

### Daemon Hardening

The systemd service applies these security restrictions:

- `PrivateTmp=true` - Isolated /tmp
- `ProtectHome=read-only` - Cannot write to home directories
- `ProtectSystem=full` - Read-only /usr, /boot, /etc

These are intentionally NOT restricted (required for operations):
- `ProtectKernelTunables=false` - Needs sysctl access
- `ProtectKernelModules=false` - May need to load modules
- `ProtectControlGroups=false` - Required for app tunnel cgroups

### Socket Permissions

The socket `/var/run/vpn-manager/vpn-managerd.sock` has mode `0666`, allowing any local user to connect. This is intentional:

- **Why**: VPN Manager GUI runs as regular user, needs to communicate with daemon
- **Security**: The daemon validates operations via `SO_PEERCRED`, which provides the UID/GID of the connecting process
- **Risk mitigation**: Only local processes can connect (Unix socket, not network)

### Privilege Separation

The architecture follows the principle of least privilege:

1. **GUI/CLI** runs as unprivileged user - handles UI, config, statistics
2. **Daemon** runs as root - handles only privileged operations
3. **Communication** is strictly typed via JSON-RPC - no shell injection possible

## Troubleshooting

### Daemon won't start

```bash
# Check logs for errors
sudo journalctl -u vpn-managerd --no-pager -n 50

# Try running manually
sudo /usr/bin/vpn-managerd
```

### Socket permission denied

```bash
# Check socket permissions
ls -la /var/run/vpn-manager/

# Restart daemon
sudo systemctl restart vpn-managerd
```

### VPN Manager shows "daemon not available"

```bash
# Ensure daemon is running
sudo systemctl start vpn-managerd

# Check if socket exists
test -S /var/run/vpn-manager/vpn-managerd.sock && echo "Socket OK" || echo "Socket missing"
```

### Operations fail

1. Check daemon logs: `sudo journalctl -u vpn-managerd -f`
2. Verify required tools are installed:
   - `iptables` or `nft` for firewall operations
   - `openvpn` for OpenVPN
   - `wg-quick` for WireGuard
   - `tailscale` for Tailscale operations

## Development

### Running Tests

```bash
go test ./daemon/... ./protocol/...
```

### Building Manually

```bash
go build -o vpn-managerd ./cmd/vpn-managerd
```

### Running in Debug Mode

```bash
sudo ./vpn-managerd  # Logs to stdout
```

### Protocol Testing

You can test the daemon directly using `socat`:

```bash
# Ping test
echo '{"jsonrpc":"2.0","method":"ping","id":1}' | sudo socat - UNIX-CONNECT:/var/run/vpn-manager/vpn-managerd.sock

# Get status
echo '{"jsonrpc":"2.0","method":"status","id":2}' | sudo socat - UNIX-CONNECT:/var/run/vpn-manager/vpn-managerd.sock
```
