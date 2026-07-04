package ui

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/yllada/vpn-manager/internal/logger"
)

// statusNotifierWatcherName is the well-known D-Bus name a StatusNotifierItem
// host (system tray) registers. If nobody owns it, tray icons never appear.
const statusNotifierWatcherName = "org.kde.StatusNotifierWatcher"

// trayDetectTimeout bounds the D-Bus probe so tray detection can never block
// application startup, even on a wedged session bus.
const trayDetectTimeout = 2 * time.Second

// IsTrayAvailable reports whether a StatusNotifierItem host (system tray) is
// present on the current session bus. GNOME vanilla ships no such host, so a
// fyne.io/systray icon would silently never appear there — hiding the window
// to a non-existent tray would strand the user with no way back.
//
// fyne.io/systray does not expose this, so we query the session bus directly
// for an owner of org.kde.StatusNotifierWatcher. The probe is fully defensive:
// any error (no session bus, timeout, malformed reply) is treated as "tray
// unavailable" rather than propagating, and the call is time-bounded so it can
// never block or panic during startup.
func IsTrayAvailable() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		logger.LogWarn("tray: session bus unavailable, assuming no system tray: %v", err)
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), trayDetectTimeout)
	defer cancel()

	obj := conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus")
	call := obj.CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, statusNotifierWatcherName)
	if call.Err != nil {
		logger.LogWarn("tray: could not query StatusNotifierWatcher owner, assuming no system tray: %v", call.Err)
		return false
	}

	var hasOwner bool
	if err := call.Store(&hasOwner); err != nil {
		logger.LogWarn("tray: could not read StatusNotifierWatcher reply, assuming no system tray: %v", err)
		return false
	}

	return hasOwner
}
