<p align="center">
  <img src="assets/icons/hicolor/scalable/apps/vpn-manager.svg" alt="VPN Manager Logo" width="128" height="128">
</p>

<h1 align="center">VPN Manager</h1>

<p align="center">
  <strong>Modern VPN client for Linux with GUI — OpenVPN, WireGuard & Tailscale</strong>
</p>

<p align="center">
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#usage">Usage</a> •
  <a href="#building">Building</a> •
  <a href="#contributing">Contributing</a> •
  <a href="CHANGELOG.md">Changelog</a>
</p>

<p align="center">
  <a href="https://github.com/yllada/vpn-manager/actions/workflows/ci.yml"><img src="https://github.com/yllada/vpn-manager/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/github/v/release/yllada/vpn-manager?label=version&color=blue" alt="Version">
  <img src="https://img.shields.io/badge/license-MIT-green.svg" alt="License">
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8.svg" alt="Go Version">
  <img src="https://img.shields.io/badge/GTK-4.0-4A86CF.svg" alt="GTK Version">
  <img src="https://img.shields.io/badge/platform-Linux-orange.svg" alt="Platform">
  <a href="https://github.com/yllada/vpn-manager/blob/main/CODE_OF_CONDUCT.md"><img src="https://img.shields.io/badge/Contributor%20Covenant-2.1-4baaaa.svg" alt="Contributor Covenant"></a>
[![Hypercommit](https://img.shields.io/badge/Hypercommit-DB2475)](https://hypercommit.com/vpn-manager)
</p>

---

## Why This Project?

When I started using Linux and needed to connect to a VPN, I spent hours searching for a simple graphical client. Everything I found required command-line knowledge — OpenVPN configs, WireGuard's `wg-quick`, Tailscale CLI... For a newcomer, it was overwhelming.

**VPN Manager was born to solve this.** A simple, beautiful GUI where you can manage all your VPN connections without touching the terminal. This is my gift to the Linux community — so others don't have to go through what I did.

## About

**VPN Manager** is a modern, user-friendly desktop application for managing VPN connections on Linux. It supports **OpenVPN**, **WireGuard**, and **Tailscale** — all in one unified interface. Built with GTK4/libadwaita, it integrates seamlessly with GNOME and other modern desktop environments.

<p align="center">
  <img src="assets/screen/cap.png" alt="VPN Manager Screenshot" width="600">
</p>

## Features

### 🔐 Profile Management
- **Multiple VPN profiles** - Organize and manage multiple VPN configurations
- **Import .ovpn files** - Easily import existing OpenVPN configuration files
- **Secure credentials** - Encrypted password storage using the system keyring

### 🌐 Advanced Connectivity
- **Split Tunneling** - Configure which traffic goes through the VPN
  - Include mode: only specified traffic uses the VPN
  - Exclude mode: all traffic except specified uses the VPN
- **OTP Support** - Two-factor authentication with one-time passwords
- **Auto-reconnect** - Automatically restores connection if lost
- **Health Monitoring** - Continuous connectivity checks with automatic recovery

### 💻 Command Line Interface
- **`--list`** - List all configured VPN profiles
- **`--connect NAME`** - Connect to a profile by name
- **`--disconnect`** - Disconnect active connections
- **`--status`** - Show current connection status

### 🎨 Modern Interface
- **GTK4 + libadwaita** - Native interface following GNOME design guidelines
- **Light/Dark themes** - Automatically follows system theme
- **System tray** - Quick access from the system indicator
- **Native notifications** - Connection/disconnection alerts

### 📊 Real-Time Monitoring
- **Connection status** - View current state of each profile
- **Live statistics** - Uptime and latency displayed in real-time
- **Health indicators** - Visual feedback on connection quality
- **Assigned IP address** - Shows current VPN IP

### 📝 Logging & Debugging
- **Structured logging** - Detailed logs with timestamps and severity levels
- **Automatic rotation** - Log files are compressed and rotated (5MB max, 5 backups)
- **Log location** - `~/.config/vpn-manager/logs/`

### ⚙️ Flexible Configuration
- **Auto-start** - Option to start with the system
- **Minimize to tray** - Keep the app accessible without taking space
- **YAML configuration** - Human-readable and editable config files

## System Requirements

### Dependencies

| Component | Minimum Version | Description |
|-----------|-----------------|-------------|
| **Operating System** | Ubuntu 22.04+ / Fedora 38+ | Or any distribution with GTK4 |
| **OpenVPN** | 2.5+ or OpenVPN3 | Underlying VPN client |
| **GTK4** | 4.0+ | GUI framework |
| **libadwaita** | 1.0+ | GNOME styling library |
| **libsecret** | 0.20+ | Secure credential storage (recommended) |


## Building

### Build Requirements

- **Go** 1.24 or higher
- **GCC** (C compiler)
- **GTK4 development libraries**

```bash
# Ubuntu/Debian
sudo apt install golang gcc libgtk-4-dev libadwaita-1-dev

# Fedora
sudo dnf install golang gcc gtk4-devel libadwaita-devel

# Arch Linux
sudo pacman -S go gcc gtk4 libadwaita
```

### Build from Source

```bash
# Clone the repository
git clone https://github.com/yllada/vpn-manager.git
cd vpn-manager

# Download dependencies
go mod download

# Build
go build -o vpn-manager .

# Run
./vpn-manager
```

### Build DEB Package

```bash
# Run the packaging script
./scripts/build-deb.sh 1.0.0

# The package will be in build/
```

## Contributing

Contributions are welcome! Please read our **[Contributing Guide](CONTRIBUTING.md)** to get started.

Quick start:
1. **Fork** the repository
2. **Create** a branch for your feature (`git checkout -b feature/new-feature`)
3. **Commit** your changes using [Conventional Commits](https://www.conventionalcommits.org/)
4. **Push** to the branch (`git push origin feature/new-feature`)
5. Open a **Pull Request**

Please note that this project is released with a **[Code of Conduct](CODE_OF_CONDUCT.md)**. By participating in this project you agree to abide by its terms.

### Security

For security vulnerabilities, please see our **[Security Policy](SECURITY.md)** for responsible disclosure guidelines.

## Roadmap

### Completed ✅
- [x] **WireGuard support** ✅ v1.0.0
- [x] **Tailscale support** ✅ v1.0.0
- [x] **Kill switch** (block traffic on VPN disconnect) ✅ v1.0.0
- [x] **NetworkManager integration** ✅ v1.0.0
- [x] **Command line interface** ✅ v1.0.2
- [x] **Health monitoring & auto-reconnect** ✅ v1.0.2
- [x] **Live connection statistics** ✅ v1.0.2

### Planned
- [ ] Bulk profile import
- [ ] Historical connection statistics
- [ ] Multi-language support (i18n)
- [ ] Configuration export/import

## License

This project is licensed under the **MIT License** - see the [LICENSE](LICENSE) file for details.

```
MIT License

Copyright (c) 2026 Yadian Llada Lopez

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction...
```

## Author

**Yadian Llada Lopez**

- GitHub: [@yllada](https://github.com/yllada)

---

<p align="center">
  Made with ❤️ for the Linux community
</p>
