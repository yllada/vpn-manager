// D-Bus based secret storage for NetworkManager connections.
//
// The VPN password must NEVER be passed as a command-line argument: argv is
// world-readable in /proc/<pid>/cmdline while the process runs. Instead of
// `nmcli connection modify <conn> vpn.secrets.password <pw>`, the secret is
// written directly through the NetworkManager D-Bus API.
package network

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

const (
	nmDest          = "org.freedesktop.NetworkManager"
	nmSettingsPath  = "/org/freedesktop/NetworkManager/Settings"
	nmSettingsIface = "org.freedesktop.NetworkManager.Settings"
	nmConnIface     = "org.freedesktop.NetworkManager.Settings.Connection"
)

// mergeVPNPasswordSecret returns a copy of the connection settings with
// vpn.secrets.password set. GetSettings strips secrets from its reply, so the
// caller must merge the secret back in before calling Update, otherwise
// NetworkManager would persist the connection without any stored password.
// The input map is not mutated.
func mergeVPNPasswordSecret(settings map[string]map[string]dbus.Variant, password string) map[string]map[string]dbus.Variant {
	merged := make(map[string]map[string]dbus.Variant, len(settings)+1)
	for section, keys := range settings {
		copied := make(map[string]dbus.Variant, len(keys)+1)
		for key, value := range keys {
			copied[key] = value
		}
		merged[section] = copied
	}

	vpn := merged["vpn"]
	if vpn == nil {
		vpn = make(map[string]dbus.Variant, 1)
		merged["vpn"] = vpn
	}

	secrets := make(map[string]string)
	if existing, ok := vpn["secrets"]; ok {
		if current, ok := existing.Value().(map[string]string); ok {
			for key, value := range current {
				secrets[key] = value
			}
		}
	}
	secrets["password"] = password
	vpn["secrets"] = dbus.MakeVariant(secrets)

	return merged
}

// updateVPNSecretOverDBus stores the VPN password for the connection matching
// connName (by connection.id) using the NetworkManager D-Bus API, so the
// secret never appears in any process argv.
func updateVPNSecretOverDBus(connName, password string) error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return fmt.Errorf("failed to connect to system bus: %w", err)
	}
	// Note: dbus.SystemBus() returns a shared connection; do not close it.

	settingsObj := conn.Object(nmDest, dbus.ObjectPath(nmSettingsPath))
	var paths []dbus.ObjectPath
	if err := settingsObj.Call(nmSettingsIface+".ListConnections", 0).Store(&paths); err != nil {
		return fmt.Errorf("failed to list NetworkManager connections: %w", err)
	}

	for _, path := range paths {
		connObj := conn.Object(nmDest, path)

		var settings map[string]map[string]dbus.Variant
		if err := connObj.Call(nmConnIface+".GetSettings", 0).Store(&settings); err != nil {
			// Skip connections we cannot inspect (e.g. permission denied).
			continue
		}

		id, _ := settings["connection"]["id"].Value().(string)
		if id != connName {
			continue
		}

		merged := mergeVPNPasswordSecret(settings, password)
		if call := connObj.Call(nmConnIface+".Update", 0, merged); call.Err != nil {
			return fmt.Errorf("failed to update connection secrets: %w", call.Err)
		}
		return nil
	}

	return fmt.Errorf("connection %q not found in NetworkManager", connName)
}
