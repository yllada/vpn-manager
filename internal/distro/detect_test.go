package distro

import (
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// TEST DATA
// =============================================================================

// Sample /etc/os-release contents for various distributions.
var (
	ubuntuOSRelease = `PRETTY_NAME="Ubuntu 24.04 LTS"
NAME="Ubuntu"
VERSION_ID="24.04"
VERSION="24.04 LTS (Noble Numbat)"
VERSION_CODENAME=noble
ID=ubuntu
ID_LIKE=debian
HOME_URL="https://www.ubuntu.com/"
SUPPORT_URL="https://help.ubuntu.com/"
`

	fedoraOSRelease = `NAME="Fedora Linux"
VERSION="41 (Workstation Edition)"
ID=fedora
VERSION_ID=41
VERSION_CODENAME=""
PLATFORM_ID="platform:f41"
PRETTY_NAME="Fedora Linux 41 (Workstation Edition)"
`

	archOSRelease = `NAME="Arch Linux"
PRETTY_NAME="Arch Linux"
ID=arch
BUILD_ID=rolling
ANSI_COLOR="38;2;23;147;209"
HOME_URL="https://archlinux.org/"
`

	manjaroOSRelease = `NAME="Manjaro Linux"
PRETTY_NAME="Manjaro Linux"
ID=manjaro
ID_LIKE=arch
BUILD_ID=rolling
`

	opensuseOSRelease = `NAME="openSUSE Tumbleweed"
ID="opensuse-tumbleweed"
ID_LIKE="opensuse suse"
VERSION_ID="20240115"
PRETTY_NAME="openSUSE Tumbleweed"
`

	linuxMintOSRelease = `NAME="Linux Mint"
VERSION="21.3 (Virginia)"
ID=linuxmint
ID_LIKE="ubuntu debian"
PRETTY_NAME="Linux Mint 21.3"
VERSION_ID="21.3"
`

	rockyOSRelease = `NAME="Rocky Linux"
VERSION="9.3 (Blue Onyx)"
ID="rocky"
ID_LIKE="rhel centos fedora"
VERSION_ID="9.3"
PLATFORM_ID="platform:el9"
PRETTY_NAME="Rocky Linux 9.3 (Blue Onyx)"
`

	popOSRelease = `NAME="Pop!_OS"
VERSION="22.04 LTS"
ID=pop
ID_LIKE="ubuntu debian"
PRETTY_NAME="Pop!_OS 22.04 LTS"
VERSION_ID="22.04"
`

	unknownOSRelease = `NAME="SomeRandomOS"
ID=randomos
VERSION="1.0"
`

	emptyOSRelease = ``

	malformedOSRelease = `NAME=Missing Quotes
ID
invalid line without equals
=value without key
ID=valid
`

	commentedOSRelease = `# This is a comment
NAME="Ubuntu"
# Another comment
ID=ubuntu
ID_LIKE=debian
# More comments
`
)

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// createTempOSRelease creates a temporary file with the given content
// and returns its path. The caller is responsible for cleanup.
func createTempOSRelease(t *testing.T, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "os-release")

	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create temp os-release: %v", err)
	}

	return path
}

// =============================================================================
// DISTRO DETECTION TESTS
// =============================================================================

func TestDetect_Ubuntu(t *testing.T) {
	path := createTempOSRelease(t, ubuntuOSRelease)
	got := detectFromPath(path)

	if got != DistroDebian {
		t.Errorf("Ubuntu: expected DistroDebian, got %v (%s)", got, got.String())
	}
}

func TestDetect_Fedora(t *testing.T) {
	path := createTempOSRelease(t, fedoraOSRelease)
	got := detectFromPath(path)

	if got != DistroFedora {
		t.Errorf("Fedora: expected DistroFedora, got %v (%s)", got, got.String())
	}
}

func TestDetect_Arch(t *testing.T) {
	path := createTempOSRelease(t, archOSRelease)
	got := detectFromPath(path)

	if got != DistroArch {
		t.Errorf("Arch: expected DistroArch, got %v (%s)", got, got.String())
	}
}

func TestDetect_Manjaro(t *testing.T) {
	path := createTempOSRelease(t, manjaroOSRelease)
	got := detectFromPath(path)

	if got != DistroArch {
		t.Errorf("Manjaro: expected DistroArch, got %v (%s)", got, got.String())
	}
}

func TestDetect_OpenSUSE(t *testing.T) {
	path := createTempOSRelease(t, opensuseOSRelease)
	got := detectFromPath(path)

	if got != DistroOpenSUSE {
		t.Errorf("openSUSE: expected DistroOpenSUSE, got %v (%s)", got, got.String())
	}
}

func TestDetect_LinuxMint(t *testing.T) {
	path := createTempOSRelease(t, linuxMintOSRelease)
	got := detectFromPath(path)

	if got != DistroDebian {
		t.Errorf("Linux Mint: expected DistroDebian, got %v (%s)", got, got.String())
	}
}

func TestDetect_Rocky(t *testing.T) {
	path := createTempOSRelease(t, rockyOSRelease)
	got := detectFromPath(path)

	if got != DistroFedora {
		t.Errorf("Rocky Linux: expected DistroFedora, got %v (%s)", got, got.String())
	}
}

func TestDetect_PopOS(t *testing.T) {
	path := createTempOSRelease(t, popOSRelease)
	got := detectFromPath(path)

	if got != DistroDebian {
		t.Errorf("Pop!_OS: expected DistroDebian, got %v (%s)", got, got.String())
	}
}

func TestDetect_Unknown(t *testing.T) {
	path := createTempOSRelease(t, unknownOSRelease)
	got := detectFromPath(path)

	if got != DistroUnknown {
		t.Errorf("Unknown distro: expected DistroUnknown, got %v (%s)", got, got.String())
	}
}

func TestDetect_Empty(t *testing.T) {
	path := createTempOSRelease(t, emptyOSRelease)
	got := detectFromPath(path)

	if got != DistroUnknown {
		t.Errorf("Empty file: expected DistroUnknown, got %v (%s)", got, got.String())
	}
}

func TestDetect_Malformed(t *testing.T) {
	path := createTempOSRelease(t, malformedOSRelease)
	got := detectFromPath(path)

	// The malformed file has "ID=valid" at the end, which should be parsed
	// but "valid" is not a known distro
	if got != DistroUnknown {
		t.Errorf("Malformed file: expected DistroUnknown, got %v (%s)", got, got.String())
	}
}

func TestDetect_Commented(t *testing.T) {
	path := createTempOSRelease(t, commentedOSRelease)
	got := detectFromPath(path)

	if got != DistroDebian {
		t.Errorf("Commented file: expected DistroDebian, got %v (%s)", got, got.String())
	}
}

func TestDetect_FileNotFound(t *testing.T) {
	got := detectFromPath("/nonexistent/path/os-release")

	if got != DistroUnknown {
		t.Errorf("File not found: expected DistroUnknown, got %v (%s)", got, got.String())
	}
}

// =============================================================================
// STRING METHOD TESTS
// =============================================================================

func TestDistroFamily_String(t *testing.T) {
	tests := []struct {
		family   DistroFamily
		expected string
	}{
		{DistroUnknown, "Unknown"},
		{DistroDebian, "Debian-based"},
		{DistroFedora, "Fedora-based"},
		{DistroArch, "Arch-based"},
		{DistroOpenSUSE, "openSUSE-based"},
		{DistroFamily(99), "Unknown"}, // Invalid value
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.family.String()
			if got != tt.expected {
				t.Errorf("String(): expected %q, got %q", tt.expected, got)
			}
		})
	}
}

// =============================================================================
// PACKAGE MANAGER TESTS
// =============================================================================

func TestGetPackageManager(t *testing.T) {
	tests := []struct {
		family   DistroFamily
		expected string
	}{
		{DistroDebian, "apt"},
		{DistroFedora, "dnf"},
		{DistroArch, "pacman"},
		{DistroOpenSUSE, "zypper"},
		{DistroUnknown, ""},
		{DistroFamily(99), ""}, // Invalid value
	}

	for _, tt := range tests {
		name := tt.family.String()
		if tt.expected == "" {
			name = "Unknown/Invalid"
		}

		t.Run(name, func(t *testing.T) {
			got := GetPackageManager(tt.family)
			if got != tt.expected {
				t.Errorf("GetPackageManager(%v): expected %q, got %q", tt.family, tt.expected, got)
			}
		})
	}
}

// =============================================================================
// TABLE-DRIVEN COMPREHENSIVE TESTS
// =============================================================================

func TestClassifyDistro(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		idLike   string
		expected DistroFamily
	}{
		// Direct ID matches
		{"Debian direct", "debian", "", DistroDebian},
		{"Ubuntu direct", "ubuntu", "", DistroDebian},
		{"Linux Mint direct", "linuxmint", "", DistroDebian},
		{"Pop!_OS direct", "pop", "", DistroDebian},
		{"Elementary direct", "elementary", "", DistroDebian},
		{"Zorin direct", "zorin", "", DistroDebian},
		{"Kali direct", "kali", "", DistroDebian},
		{"Raspbian direct", "raspbian", "", DistroDebian},

		{"Fedora direct", "fedora", "", DistroFedora},
		{"RHEL direct", "rhel", "", DistroFedora},
		{"CentOS direct", "centos", "", DistroFedora},
		{"Rocky direct", "rocky", "", DistroFedora},
		{"Alma direct", "almalinux", "", DistroFedora},
		{"Oracle Linux direct", "ol", "", DistroFedora},

		{"Arch direct", "arch", "", DistroArch},
		{"Manjaro direct", "manjaro", "", DistroArch},
		{"EndeavourOS direct", "endeavouros", "", DistroArch},
		{"Garuda direct", "garuda", "", DistroArch},
		{"Artix direct", "artix", "", DistroArch},

		{"openSUSE direct", "opensuse", "", DistroOpenSUSE},
		{"openSUSE Leap", "opensuse-leap", "", DistroOpenSUSE},
		{"openSUSE Tumbleweed", "opensuse-tumbleweed", "", DistroOpenSUSE},
		{"SLES direct", "sles", "", DistroOpenSUSE},

		// ID_LIKE fallback matches
		{"Unknown with debian ID_LIKE", "somedebian", "debian", DistroDebian},
		{"Unknown with ubuntu ID_LIKE", "someubuntu", "ubuntu", DistroDebian},
		{"Unknown with fedora ID_LIKE", "somefedora", "fedora", DistroFedora},
		{"Unknown with arch ID_LIKE", "somearch", "arch", DistroArch},
		{"Unknown with opensuse ID_LIKE", "somesuse", "opensuse", DistroOpenSUSE},
		{"Unknown with suse ID_LIKE", "somesuse", "suse", DistroOpenSUSE},

		// Multiple ID_LIKE values
		{"Linux Mint style", "mint", "ubuntu debian", DistroDebian},
		{"Rocky style", "rocky", "rhel centos fedora", DistroFedora},
		{"openSUSE style", "leap", "opensuse suse", DistroOpenSUSE},

		// Unknown
		{"Completely unknown", "randomos", "", DistroUnknown},
		{"Empty both", "", "", DistroUnknown},

		// Case handling (should be lowercase internally)
		{"Uppercase ID", "UBUNTU", "", DistroUnknown}, // Note: actual parsing lowercases
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyDistro(tt.id, tt.idLike)
			if got != tt.expected {
				t.Errorf("classifyDistro(%q, %q): expected %v, got %v",
					tt.id, tt.idLike, tt.expected, got)
			}
		})
	}
}
