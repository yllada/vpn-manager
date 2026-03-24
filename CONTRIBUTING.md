# Contributing to VPN Manager

First off, thank you for considering contributing to VPN Manager! It's people like you that make VPN Manager such a great tool for the Linux community.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [How Can I Contribute?](#how-can-i-contribute)
- [Development Setup](#development-setup)
- [Pull Request Process](#pull-request-process)
- [Style Guidelines](#style-guidelines)
- [Community](#community)

## Code of Conduct

This project and everyone participating in it is governed by our [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to vpn-manager@example.com.

## Getting Started

VPN Manager is a GTK4-based VPN client for Linux that supports OpenVPN, WireGuard, and Tailscale. Before you begin:

1. Make sure you have a [GitHub account](https://github.com/signup)
2. Familiarize yourself with [Git](https://git-scm.com/doc)
3. Read the [README.md](README.md) to understand the project

### Good First Issues

Looking for something to work on? Check out issues labeled [`good first issue`](https://github.com/yllada/vpn-manager/labels/good%20first%20issue) – these are great for newcomers!

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check the [existing issues](https://github.com/yllada/vpn-manager/issues) to avoid duplicates.

When creating a bug report, please use our [bug report template](.github/ISSUE_TEMPLATE/bug_report.yml) and include:

- **VPN Manager version** (`vpn-manager --version`)
- **Linux distribution and version**
- **Desktop environment** (GNOME, KDE, etc.)
- **Steps to reproduce** the issue
- **Expected vs actual behavior**
- **Relevant logs** from `~/.config/vpn-manager/logs/`

### Suggesting Features

Feature requests are welcome! Please use our [feature request template](.github/ISSUE_TEMPLATE/feature_request.yml) and describe:

- **The problem** you're trying to solve
- **Your proposed solution**
- **Alternatives** you've considered

### Code Contributions

1. **Fork** the repository
2. **Clone** your fork locally
3. **Create a branch** for your changes (`git checkout -b feature/amazing-feature`)
4. **Make your changes** following our style guidelines
5. **Test** your changes thoroughly
6. **Commit** using conventional commits
7. **Push** to your fork
8. **Open a Pull Request**

## Development Setup

### Prerequisites

```bash
# Ubuntu/Debian
sudo apt install golang gcc libgtk-4-dev libadwaita-1-dev openvpn

# Fedora
sudo dnf install golang gcc gtk4-devel libadwaita-devel openvpn

# Arch Linux
sudo pacman -S go gcc gtk4 libadwaita openvpn
```

### Building

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/vpn-manager.git
cd vpn-manager

# Download dependencies
go mod download

# Build
go build -o vpn-manager .

# Run
./vpn-manager
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./vpn/...
```

### Project Structure

```
vpn-manager/
├── main.go              # Application entry point
├── app/                 # Core application logic
│   ├── config.go        # Configuration management
│   ├── eventbus.go      # Event system
│   ├── logger.go        # Structured logging
│   ├── resilience.go    # Circuit breaker, retry logic
│   └── security.go      # Encryption, secure storage
├── cli/                 # Command-line interface
├── keyring/             # System keyring integration
├── ui/                  # GTK4/libadwaita UI components
│   ├── app.go           # Main application window
│   ├── openvpn_panel.go # OpenVPN management
│   ├── wireguard_panel.go
│   ├── tailscale_panel.go
│   └── tray.go          # System tray
└── vpn/                 # VPN providers
    ├── manager.go       # Connection management
    ├── openvpn/         # OpenVPN provider
    ├── wireguard/       # WireGuard provider
    └── tailscale/       # Tailscale provider
```

## Pull Request Process

1. **Update documentation** if you change functionality
2. **Add tests** for new features
3. **Follow the PR template** when submitting
4. **Wait for review** – maintainers will review your PR
5. **Address feedback** if changes are requested
6. **Celebrate** when it's merged! 🎉

### Commit Message Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, semicolons, etc.)
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Examples:**
```
feat(wireguard): add split tunnel support
fix(tailscale): resolve exit node selection bug
docs: update installation instructions
```

## Style Guidelines

### Go Code Style

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use `gofmt` for formatting
- Run `golint` and `go vet` before committing
- Keep functions focused and small
- Add comments for exported functions and types
- Handle errors explicitly – don't ignore them

### UI Guidelines

- Follow [GNOME Human Interface Guidelines](https://developer.gnome.org/hig/)
- Use libadwaita widgets when available
- Support both light and dark themes
- Ensure accessibility (proper labels, keyboard navigation)

### Testing Guidelines

- Write tests for new functionality
- Use table-driven tests when appropriate
- Mock external dependencies
- Test error cases, not just happy paths

## Community

### Getting Help

- **GitHub Issues**: For bugs and feature requests
- **GitHub Discussions**: For questions and community chat
- **README**: For setup and usage instructions

### Recognition

Contributors are recognized in:
- Release notes
- Contributors list

Thank you for making VPN Manager better for everyone! 💙
