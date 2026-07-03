// Package common provides shared utilities for UI panels.
// Contains formatting functions, icon helpers, and other common utilities
// used across multiple panel implementations.
package common

import (
	"fmt"
	"time"

	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
)

// =============================================================================
// DURATION FORMATTING
// =============================================================================

// FormatDuration formats a duration in a human-readable format.
// Example: 1h 30m 45s, 5m 30s, 45s
func FormatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// FormatDurationCompact formats a duration compactly, including days for long durations.
// Example: 2d 5h, 1h 30m, 5m 30s, 45s
func FormatDurationCompact(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 24 {
		days := hours / 24
		hours = hours % 24
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// =============================================================================
// BYTE/BANDWIDTH FORMATTING
// =============================================================================

const (
	KB = 1024
	MB = KB * 1024
	GB = MB * 1024
)

// FormatBandwidth formats bytes per second as human-readable bandwidth.
// Example: 1.5 MB/s, 256 KB/s, 512 B/s
func FormatBandwidth(bytesPerSec float64) string {
	switch {
	case bytesPerSec >= float64(GB):
		return fmt.Sprintf("%.1f GB/s", bytesPerSec/float64(GB))
	case bytesPerSec >= float64(MB):
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/float64(MB))
	case bytesPerSec >= float64(KB):
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/float64(KB))
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}

// =============================================================================
// VPN PROVIDER HELPERS
// =============================================================================

// GetProviderIcon returns the appropriate icon name for a VPN provider type.
func GetProviderIcon(providerType vpntypes.VPNProviderType) string {
	switch providerType {
	case vpntypes.ProviderOpenVPN:
		return "network-vpn-symbolic"
	case vpntypes.ProviderTailscale:
		return "network-workgroup-symbolic"
	case vpntypes.ProviderWireGuard:
		return "security-high-symbolic"
	default:
		return "network-vpn-symbolic"
	}
}

// GetProviderDisplayName returns a human-readable name for a VPN provider type.
func GetProviderDisplayName(providerType vpntypes.VPNProviderType) string {
	switch providerType {
	case vpntypes.ProviderOpenVPN:
		return "OpenVPN"
	case vpntypes.ProviderTailscale:
		return "Tailscale"
	case vpntypes.ProviderWireGuard:
		return "WireGuard"
	default:
		return string(providerType)
	}
}
