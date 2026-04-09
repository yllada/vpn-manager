# VPN Manager Daemon

The VPN Manager Daemon (`vpn-managerd`) handles all privileged operations that require root access, such as:

- **Kill Switch** - Firewall rules via iptables/nftables
- **DNS Protection** - DNS firewall and DoT blocking
- **IPv6 Protection** - Disable IPv6 via sysctl and firewall
- **LAN Gateway** - IP forwarding and NAT rules

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        User Space                                │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  VPN Manager GUI/CLI (runs as regular user)             │    │
│  │  - All UI/UX logic                                      │    │
│  │  - VPN connections (OpenVPN, WireGuard, Tailscale)      │    │
│  │  - Configuration management                             │    │
│  └─────────────────┬───────────────────────────────────────┘    │
│                    │ Unix Socket (JSON-RPC 2.0)                  │
│                    ▼                                             │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  vpn-managerd (runs as root via systemd)                │    │
│  │  - Kill Switch (iptables/nftables)                      │    │
│  │  - DNS Protection (firewall rules)                      │    │
│  │  - IPv6 Protection (sysctl + firewall)                  │    │
│  │  - LAN Gateway (IP forwarding + NAT)                    │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

## Installation

### Prerequisites

- Linux with systemd
- Go 1.21+ (for building)
- Root access (sudo)

### Install

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

## Fallback Mode

If the daemon is not running, VPN Manager automatically falls back to using `pkexec` for privileged operations. This ensures the application works even without the daemon installed.

## Protocol

The daemon uses JSON-RPC 2.0 over a Unix socket:

- **Socket Path**: `/var/run/vpn-manager/vpn-managerd.sock`
- **Protocol**: JSON-RPC 2.0 with newline-delimited messages
- **Authentication**: SO_PEERCRED (verifies client identity via Unix socket)

### Available Methods

| Method | Description |
|--------|-------------|
| `ping` | Health check |
| `status` | Get daemon status and feature states |
| `killswitch.enable` | Enable kill switch |
| `killswitch.disable` | Disable kill switch |
| `dns.enable` | Enable DNS protection |
| `dns.disable` | Disable DNS protection |
| `ipv6.enable` | Enable IPv6 protection |
| `ipv6.disable` | Disable IPv6 protection |
| `gateway.enable` | Enable LAN gateway |
| `gateway.disable` | Disable LAN gateway |

### Example Request

```json
{"jsonrpc":"2.0","method":"killswitch.enable","params":{"interface":"tun0"},"id":1}
```

### Example Response

```json
{"jsonrpc":"2.0","result":{"success":true},"id":1}
```

## Security

### Daemon Hardening

The systemd service applies these security restrictions:

- `PrivateTmp=true` - Isolated /tmp
- `ProtectHome=read-only` - Cannot write to home directories
- `ProtectSystem=full` - Read-only /usr, /boot, /etc

These are intentionally NOT restricted (required for firewall operations):
- `ProtectKernelTunables=false` - Needs sysctl access
- `ProtectKernelModules=false` - May need to load modules
- `ProtectControlGroups=false` - May need cgroup access

### Socket Permissions

The socket directory `/var/run/vpn-manager/` has mode `0755`, allowing any user to connect. The daemon validates operations based on the calling user's UID via `SO_PEERCRED`.

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

### Operations fail with "daemon not available"

The client will fall back to `pkexec`. To use the daemon:

```bash
# Ensure daemon is running
sudo systemctl start vpn-managerd

# Check if socket exists
test -S /var/run/vpn-manager/vpn-managerd.sock && echo "Socket OK" || echo "Socket missing"
```

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
