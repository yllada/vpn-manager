// Package security test helpers: a fake privileged daemon substituted through
// the seams in seams.go, mirroring the firewall package's runCmd seam pattern.
// Tests using these helpers must not call t.Parallel() — the seams are
// package-level vars.
package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yllada/vpn-manager/internal/daemon"
)

// fakeDaemon records daemon-client calls and lets tests script availability,
// results, and failures so client-side state machines can be pinned without a
// running vpn-managerd, without root, and without touching the real firewall.
type fakeDaemon struct {
	available  bool
	backend    string
	enableErr  error
	disableErr error

	ksEnableParams []daemon.KillSwitchEnableParams
	ksDisableCalls int
	ksBlockAll     int

	dnsEnableParams []daemon.DNSEnableParams
	dnsDisableCalls int

	v6EnableParams []daemon.IPv6EnableParams
	v6DisableCalls int
}

// installFakeDaemon swaps every daemon seam for the fake for the duration of
// the test.
func installFakeDaemon(t *testing.T, fd *fakeDaemon) {
	t.Helper()

	origAvailable := daemonAvailable
	origKSEnable := killSwitchEnable
	origKSDisable := killSwitchDisable
	origKSBlockAll := killSwitchEnableBlockAll
	origDNSEnable := dnsFirewallEnable
	origDNSDisable := dnsFirewallDisable
	origV6Enable := ipv6Enable
	origV6Disable := ipv6Disable

	daemonAvailable = func() bool { return fd.available }
	killSwitchEnable = func(p daemon.KillSwitchEnableParams) (*daemon.KillSwitchEnableResult, error) {
		fd.ksEnableParams = append(fd.ksEnableParams, p)
		if fd.enableErr != nil {
			return nil, fd.enableErr
		}
		return &daemon.KillSwitchEnableResult{Enabled: true, Backend: fd.backend}, nil
	}
	killSwitchDisable = func() error {
		fd.ksDisableCalls++
		return fd.disableErr
	}
	killSwitchEnableBlockAll = func() (*daemon.KillSwitchEnableResult, error) {
		fd.ksBlockAll++
		if fd.enableErr != nil {
			return nil, fd.enableErr
		}
		return &daemon.KillSwitchEnableResult{Enabled: true, Backend: fd.backend}, nil
	}
	dnsFirewallEnable = func(p daemon.DNSEnableParams) error {
		fd.dnsEnableParams = append(fd.dnsEnableParams, p)
		return fd.enableErr
	}
	dnsFirewallDisable = func() error {
		fd.dnsDisableCalls++
		return fd.disableErr
	}
	ipv6Enable = func(p daemon.IPv6EnableParams) error {
		fd.v6EnableParams = append(fd.v6EnableParams, p)
		return fd.enableErr
	}
	ipv6Disable = func() error {
		fd.v6DisableCalls++
		return fd.disableErr
	}

	t.Cleanup(func() {
		daemonAvailable = origAvailable
		killSwitchEnable = origKSEnable
		killSwitchDisable = origKSDisable
		killSwitchEnableBlockAll = origKSBlockAll
		dnsFirewallEnable = origDNSEnable
		dnsFirewallDisable = origDNSDisable
		ipv6Enable = origV6Enable
		ipv6Disable = origV6Disable
	})
}

// useTempStatePaths redirects the root-owned /var/lib state files to a
// per-test temp dir so persistence round-trips run without root.
func useTempStatePaths(t *testing.T) (ksPath, dnsPath string) {
	t.Helper()

	origKS := killSwitchStatePath
	origDNS := dnsStatePath
	origEnsure := ensureStateDir

	dir := t.TempDir()
	killSwitchStatePath = filepath.Join(dir, "killswitch.state")
	dnsStatePath = filepath.Join(dir, "dns.state")
	ensureStateDir = func() error { return nil }

	t.Cleanup(func() {
		killSwitchStatePath = origKS
		dnsStatePath = origDNS
		ensureStateDir = origEnsure
	})
	return killSwitchStatePath, dnsStatePath
}

// fileExists is a small assertion helper for state files.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
