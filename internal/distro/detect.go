// Package distro provides Linux distribution detection functionality.
// It parses /etc/os-release to identify the distribution family and
// provide appropriate package manager recommendations.
package distro

import (
	"bufio"
	"os"
	"strings"
)

// =============================================================================
// DISTRO FAMILY TYPES
// =============================================================================

// DistroFamily represents a family of Linux distributions that share
// a common package manager and installation commands.
type DistroFamily int

const (
	// DistroUnknown represents an unrecognized or non-Linux system.
	DistroUnknown DistroFamily = iota
	// DistroDebian includes Ubuntu, Debian, Linux Mint, Pop!_OS, elementary.
	DistroDebian
	// DistroFedora includes Fedora, RHEL, CentOS, Rocky Linux, Alma Linux.
	DistroFedora
	// DistroArch includes Arch Linux, Manjaro, EndeavourOS, Garuda.
	DistroArch
	// DistroOpenSUSE includes openSUSE Leap, Tumbleweed, SLES.
	DistroOpenSUSE
)

// osReleasePath is the path to the os-release file.
// This variable allows testing with mock files.
var osReleasePath = "/etc/os-release"

// =============================================================================
// DETECTION FUNCTIONS
// =============================================================================

// Detect reads /etc/os-release and returns the distribution family.
// Returns DistroUnknown if the file doesn't exist, cannot be read,
// or the distribution is not recognized.
func Detect() DistroFamily {
	return detectFromPath(osReleasePath)
}

// detectFromPath reads a specific os-release file and returns the distro family.
// This is separated for testing purposes.
func detectFromPath(path string) DistroFamily {
	file, err := os.Open(path)
	if err != nil {
		return DistroUnknown
	}
	defer func() { _ = file.Close() }()

	return parseOSRelease(file)
}

// parseOSRelease parses os-release content and returns the distro family.
// Exported for testing with arbitrary content.
func parseOSRelease(file *os.File) DistroFamily {
	scanner := bufio.NewScanner(file)
	var id, idLike string

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value pairs
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		// Remove quotes from value
		value = strings.Trim(value, "\"'")

		switch key {
		case "ID":
			id = strings.ToLower(value)
		case "ID_LIKE":
			idLike = strings.ToLower(value)
		}
	}

	// Check scanner error (but don't fail - partial data is still useful)
	if err := scanner.Err(); err != nil {
		// If we got some data, try to use it; otherwise return Unknown
		if id == "" && idLike == "" {
			return DistroUnknown
		}
	}

	return classifyDistro(id, idLike)
}

// classifyDistro maps ID and ID_LIKE values to a DistroFamily.
func classifyDistro(id, idLike string) DistroFamily {
	// First check ID directly for known distros
	switch id {
	case "debian", "ubuntu", "linuxmint", "pop", "elementary", "zorin", "kali", "raspbian":
		return DistroDebian
	case "fedora", "rhel", "centos", "rocky", "almalinux", "ol":
		return DistroFedora
	case "arch", "manjaro", "endeavouros", "garuda", "artix":
		return DistroArch
	case "opensuse", "opensuse-leap", "opensuse-tumbleweed", "sles":
		return DistroOpenSUSE
	}

	// If ID didn't match, check ID_LIKE for parent distro
	// ID_LIKE can contain multiple values, e.g., "ubuntu debian"
	likes := strings.Fields(idLike)
	for _, like := range likes {
		switch like {
		case "debian", "ubuntu":
			return DistroDebian
		case "fedora", "rhel", "centos":
			return DistroFedora
		case "arch":
			return DistroArch
		case "opensuse", "suse":
			return DistroOpenSUSE
		}
	}

	return DistroUnknown
}

// =============================================================================
// STRING REPRESENTATION
// =============================================================================

// String returns a human-readable name for the distribution family.
// This is useful for UI display and logging.
func (d DistroFamily) String() string {
	switch d {
	case DistroDebian:
		return "Debian-based"
	case DistroFedora:
		return "Fedora-based"
	case DistroArch:
		return "Arch-based"
	case DistroOpenSUSE:
		return "openSUSE-based"
	default:
		return "Unknown"
	}
}

// =============================================================================
// PACKAGE MANAGER HELPERS
// =============================================================================

// GetPackageManager returns the package manager command for the distro family.
// Returns an empty string for unknown distributions.
func GetPackageManager(d DistroFamily) string {
	switch d {
	case DistroDebian:
		return "apt"
	case DistroFedora:
		return "dnf"
	case DistroArch:
		return "pacman"
	case DistroOpenSUSE:
		return "zypper"
	default:
		return ""
	}
}
