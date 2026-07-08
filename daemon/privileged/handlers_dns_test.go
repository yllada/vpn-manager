package privileged

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"testing"

	"github.com/yllada/vpn-manager/daemon"
	dnsresolver "github.com/yllada/vpn-manager/daemon/privileged/dns"
	"github.com/yllada/vpn-manager/pkg/protocol"
)

// fakeResolver is a dnsResolverOps whose Apply/Restore outcomes are scripted, so
// the DNS handlers' orchestration can be tested without root or real resolvectl.
type fakeResolver struct {
	applyErr     error
	restoreErr   error
	backend      dnsresolver.Backend
	applyCalls   int
	restoreCalls int
}

func (f *fakeResolver) Apply(string, []string, string) error { f.applyCalls++; return f.applyErr }
func (f *fakeResolver) Restore() error                       { f.restoreCalls++; return f.restoreErr }
func (f *fakeResolver) Backend() dnsresolver.Backend         { return f.backend }

// fwSpy records the firewall calls the DNS handlers make.
type fwSpy struct {
	enableDNS, disableDNS, blockDoT, unblockDoT int
	enableErr                                   error
}

// installDNSSeams swaps the DNS-handler seams (resolver + firewall) for the test
// and restores them afterwards.
func installDNSSeams(t *testing.T, r dnsResolverOps) *fwSpy {
	t.Helper()
	spy := &fwSpy{}
	origR, origEn, origDis, origBlk, origUnblk := getDNSResolver, fwEnableDNS, fwDisableDNS, fwBlockDoT, fwUnblockDoT
	getDNSResolver = func() dnsResolverOps { return r }
	fwEnableDNS = func(string) error { spy.enableDNS++; return spy.enableErr }
	fwDisableDNS = func() error { spy.disableDNS++; return nil }
	fwBlockDoT = func() error { spy.blockDoT++; return nil }
	fwUnblockDoT = func() error { spy.unblockDoT++; return nil }
	t.Cleanup(func() {
		getDNSResolver, fwEnableDNS, fwDisableDNS, fwBlockDoT, fwUnblockDoT = origR, origEn, origDis, origBlk, origUnblk
	})
	return spy
}

func dnsCtx(t *testing.T, state *daemon.State, params any) *daemon.HandlerContext {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return &daemon.HandlerContext{
		Context: context.Background(),
		Request: &protocol.Request{Params: raw},
		State:   state,
		Logger:  log.New(io.Discard, "", 0),
	}
}

// TestDNSEnableHandlerRollsBackFirewallOnResolverFailure pins that a resolver
// apply failure tears down the firewall rules the handler just installed, so no
// orphaned leak-block / DoT rules survive, and protection is not marked enabled.
func TestDNSEnableHandlerRollsBackFirewallOnResolverFailure(t *testing.T) {
	spy := installDNSSeams(t, &fakeResolver{
		applyErr: fmt.Errorf("apply boom"),
		backend:  dnsresolver.BackendSystemdResolved,
	})
	state := daemon.NewState()
	ctx := dnsCtx(t, state, DNSEnableParams{
		VPNInterface: "tun0", Servers: []string{"1.1.1.1"},
		Mode: "strict", BlockDoT: true, LeakBlocking: true,
	})

	if _, err := DNSEnableHandler(state)(ctx); err == nil {
		t.Fatal("expected an error when resolver.Apply fails")
	}
	if spy.disableDNS == 0 {
		t.Error("leak-block firewall rules were not rolled back after resolver failure")
	}
	if spy.unblockDoT == 0 {
		t.Error("DoT block was not rolled back after resolver failure")
	}
	if state.GetDNSProtection().Enabled {
		t.Error("protection marked enabled despite a resolver failure")
	}
}

// TestDNSDisableHandlerFailsClosedOnRestoreFailure pins that a resolver restore
// failure surfaces as an error and leaves protection marked ENABLED, so the
// daemon never reports "off" while the DNS override may still be live.
func TestDNSDisableHandlerFailsClosedOnRestoreFailure(t *testing.T) {
	installDNSSeams(t, &fakeResolver{restoreErr: fmt.Errorf("revert boom")})
	state := daemon.NewState()
	state.SetDNSProtectionEnabled(true)

	if _, err := DNSDisableHandler(state)(dnsCtx(t, state, nil)); err == nil {
		t.Fatal("expected an error when resolver.Restore fails")
	}
	if !state.GetDNSProtection().Enabled {
		t.Error("protection reported disabled despite a failed revert (false 'off')")
	}
}

// TestDNSDisableHandlerSucceeds pins the happy path clears state.
func TestDNSDisableHandlerSucceeds(t *testing.T) {
	installDNSSeams(t, &fakeResolver{})
	state := daemon.NewState()
	state.SetDNSProtectionEnabled(true)

	if _, err := DNSDisableHandler(state)(dnsCtx(t, state, nil)); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if state.GetDNSProtection().Enabled {
		t.Error("protection still enabled after a successful disable")
	}
}

// TestDNSEnableHandlerSucceeds pins the happy path marks state enabled and
// installs the leak-block firewall.
func TestDNSEnableHandlerSucceeds(t *testing.T) {
	spy := installDNSSeams(t, &fakeResolver{backend: dnsresolver.BackendSystemdResolved})
	state := daemon.NewState()
	ctx := dnsCtx(t, state, DNSEnableParams{
		VPNInterface: "tun0", Servers: []string{"1.1.1.1"},
		Mode: "custom", LeakBlocking: true,
	})

	if _, err := DNSEnableHandler(state)(ctx); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !state.GetDNSProtection().Enabled {
		t.Error("protection not marked enabled after a successful enable")
	}
	if spy.enableDNS == 0 {
		t.Error("leak-block firewall was not applied")
	}
}
