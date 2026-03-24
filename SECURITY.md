# Security Policy

## Supported Versions

We release patches for security vulnerabilities in the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 2.x.x   | :white_check_mark: |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Security Measures

VPN Manager implements multiple security layers to protect your data:

### Credential Storage

- **System Keyring**: Credentials are stored using the system's secret service (libsecret/GNOME Keyring) when available
- **Encrypted Fallback**: When keyring is unavailable, credentials are encrypted using:
  - **AES-256-GCM** for symmetric encryption
  - **Argon2id** for key derivation (memory-hard, resistant to GPU attacks)
  - **Secure random** IV generation for each encryption operation

### Memory Protection

- **SecureString**: Sensitive data in memory is zeroed after use
- **Constant-time comparison**: Prevents timing attacks on credential verification

### Network Security

- **No telemetry**: VPN Manager does not collect or transmit any user data
- **Local-only configuration**: All settings are stored locally
- **No automatic updates**: Updates only through your package manager or GitHub releases

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities through public GitHub issues.**

If you discover a security vulnerability, please report it by emailing:

📧 **vpn-manager-security@example.com**

### What to Include

Please include the following information in your report:

1. **Description** of the vulnerability
2. **Steps to reproduce** the issue
3. **Potential impact** assessment
4. **Suggested fix** (if you have one)
5. **Your contact information** for follow-up questions

### Response Timeline

- **Initial Response**: Within 48 hours
- **Status Update**: Within 7 days
- **Resolution Target**: Within 30 days for critical issues

### Disclosure Policy

- We follow **coordinated disclosure** – please give us reasonable time to fix the issue before public disclosure
- We will credit you in the security advisory (unless you prefer to remain anonymous)
- We do not offer bug bounties at this time, but we deeply appreciate your contributions to keeping VPN Manager secure

## Security Best Practices for Users

### Before Using VPN Manager

1. **Verify downloads**: Always download from official sources (GitHub releases)
2. **Check signatures**: Verify release signatures when available
3. **Keep updated**: Install security updates promptly

### During Use

1. **Strong passwords**: Use unique, strong passwords for VPN profiles
2. **OTP when available**: Enable two-factor authentication if your VPN provider supports it
3. **Review permissions**: VPN Manager only needs network access and keyring access

### Configuration Security

- **Profile files**: Keep `.ovpn` and WireGuard config files secure (they may contain private keys)
- **Logs**: Log files are stored in `~/.config/vpn-manager/logs/` – they do NOT contain credentials

## Known Security Considerations

### Permissions Required

VPN Manager requires certain elevated permissions:

| Feature | Requirement | Reason |
|---------|-------------|--------|
| OpenVPN | Root/sudo | Manages network interfaces |
| WireGuard | Root/sudo | Manages network interfaces |
| Tailscale | tailscaled daemon | Connects to Tailscale service |
| Kill Switch | Root/sudo | Modifies iptables rules |
| DNS Protection | Root/sudo | Modifies resolv.conf |

### Threat Model

VPN Manager protects against:
- ✅ Credential theft from disk (encrypted storage)
- ✅ Memory scraping (SecureString zeroing)
- ✅ Timing attacks (constant-time comparisons)
- ✅ Network leaks (kill switch, DNS protection)

VPN Manager does NOT protect against:
- ❌ Compromised system/root access
- ❌ Malicious VPN providers
- ❌ Physical access to unlocked system

## Security Audit History

| Date | Auditor | Scope | Result |
|------|---------|-------|--------|
| 2026-03 | Internal | Full codebase | 8.5/10 security score |

## Acknowledgments

We thank the following individuals for responsibly disclosing vulnerabilities:

*No vulnerabilities have been reported yet. Be the first!*

---

Thank you for helping keep VPN Manager and its users safe! 🔐
