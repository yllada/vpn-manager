package wireguard

import (
	"errors"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/config"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
)

// fakeHost is a minimal ports.PanelHost implementation for exercising
// showError without a GTK main loop.
type fakeHost struct {
	cfg        *config.Config
	errorCalls int
	errorTitle string
	errorBody  string
}

func (f *fakeHost) ShowToast(message string, timeout uint) {}
func (f *fakeHost) ShowToastWithAction(message, actionLabel, actionName string, timeout uint) {
}
func (f *fakeHost) SetStatus(text string) {}
func (f *fakeHost) ShowError(title, message string) {
	f.errorCalls++
	f.errorTitle = title
	f.errorBody = message
}
func (f *fakeHost) ShowInfo(title, message string)                             {}
func (f *fakeHost) IsDaemonAvailable() bool                                    { return true }
func (f *fakeHost) RefreshDaemonStatus()                                       {}
func (f *fakeHost) RefreshAllPanels()                                          {}
func (f *fakeHost) ConnectExclusive(proto, id, name string, connect func() error) {
	_ = connect()
}
func (f *fakeHost) GetWindow() gtk.Widgetter                                   { return nil }
func (f *fakeHost) GetGtkWindow() *gtk.Window                                  { return nil }
func (f *fakeHost) GetClipboard() *gdk.Clipboard                               { return nil }
func (f *fakeHost) VPNManager() ports.VPNController                            { return nil }
func (f *fakeHost) GetConfig() *config.Config                                  { return f.cfg }
func (f *fakeHost) UpdateTrayStatus(state ports.TrayState, profileName string) {}

// TestShowError_SurfacesDialogWithNotificationsOff is the regression test for
// the silent-error bug: failures must reach the UI even when desktop
// notifications are disabled.
func TestShowError_SurfacesDialogWithNotificationsOff(t *testing.T) {
	host := &fakeHost{cfg: &config.Config{ShowNotifications: false}}
	wp := &WireGuardPanel{host: host}

	wp.showError("Connection Failed", errors.New("some unmapped weirdness"))

	if host.errorCalls != 1 {
		t.Fatalf("ShowError calls = %d, want 1", host.errorCalls)
	}
	if host.errorTitle != "Connection Failed" {
		t.Errorf("title = %q, want fallback title %q", host.errorTitle, "Connection Failed")
	}
	if host.errorBody != "some unmapped weirdness" {
		t.Errorf("body = %q, want the raw error text", host.errorBody)
	}
}

// TestShowError_TranslatesKnownFailures verifies showError routes through
// components.ExplainError so known failures get an actionable title.
func TestShowError_TranslatesKnownFailures(t *testing.T) {
	host := &fakeHost{cfg: &config.Config{ShowNotifications: false}}
	wp := &WireGuardPanel{host: host}

	wp.showError("Connection Failed", errors.New("open /dev/net/tun: permission denied"))

	if host.errorTitle != "Permission needed" {
		t.Errorf("title = %q, want %q", host.errorTitle, "Permission needed")
	}
}

// TestShowError_NilErrorIsNoOp guards against accidental empty dialogs.
func TestShowError_NilErrorIsNoOp(t *testing.T) {
	host := &fakeHost{cfg: &config.Config{ShowNotifications: false}}
	wp := &WireGuardPanel{host: host}

	wp.showError("Connection Failed", nil)

	if host.errorCalls != 0 {
		t.Errorf("ShowError calls = %d, want 0", host.errorCalls)
	}
}
