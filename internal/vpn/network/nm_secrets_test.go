package network

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func sampleSettings() map[string]map[string]dbus.Variant {
	return map[string]map[string]dbus.Variant{
		"connection": {
			"id":   dbus.MakeVariant("Work VPN"),
			"type": dbus.MakeVariant("vpn"),
		},
		"vpn": {
			"service-type": dbus.MakeVariant("org.freedesktop.NetworkManager.openvpn"),
			"data": dbus.MakeVariant(map[string]string{
				"remote":         "vpn.example.com",
				"password-flags": "0",
			}),
		},
	}
}

func TestMergeVPNPasswordSecretSetsPassword(t *testing.T) {
	merged := mergeVPNPasswordSecret(sampleSettings(), "s3cret-pw")

	secretsVar, ok := merged["vpn"]["secrets"]
	if !ok {
		t.Fatal("merged settings missing vpn.secrets")
	}
	secrets, ok := secretsVar.Value().(map[string]string)
	if !ok {
		t.Fatalf("vpn.secrets has wrong type: %T", secretsVar.Value())
	}
	if secrets["password"] != "s3cret-pw" {
		t.Fatalf("vpn.secrets.password = %q, want %q", secrets["password"], "s3cret-pw")
	}
}

func TestMergeVPNPasswordSecretPreservesExistingSettings(t *testing.T) {
	merged := mergeVPNPasswordSecret(sampleSettings(), "s3cret-pw")

	if id, _ := merged["connection"]["id"].Value().(string); id != "Work VPN" {
		t.Fatalf("connection.id lost during merge: %q", id)
	}
	data, ok := merged["vpn"]["data"].Value().(map[string]string)
	if !ok || data["remote"] != "vpn.example.com" {
		t.Fatalf("vpn.data lost during merge: %v", merged["vpn"]["data"].Value())
	}
}

func TestMergeVPNPasswordSecretPreservesOtherSecrets(t *testing.T) {
	settings := sampleSettings()
	settings["vpn"]["secrets"] = dbus.MakeVariant(map[string]string{
		"cert-pass": "keep-me",
	})

	merged := mergeVPNPasswordSecret(settings, "s3cret-pw")

	secrets := merged["vpn"]["secrets"].Value().(map[string]string)
	if secrets["cert-pass"] != "keep-me" {
		t.Fatalf("existing secret lost during merge: %v", secrets)
	}
	if secrets["password"] != "s3cret-pw" {
		t.Fatalf("password not set during merge: %v", secrets)
	}
}

func TestMergeVPNPasswordSecretHandlesMissingVPNSection(t *testing.T) {
	settings := map[string]map[string]dbus.Variant{
		"connection": {"id": dbus.MakeVariant("Work VPN")},
	}

	merged := mergeVPNPasswordSecret(settings, "s3cret-pw")

	secrets, ok := merged["vpn"]["secrets"].Value().(map[string]string)
	if !ok || secrets["password"] != "s3cret-pw" {
		t.Fatalf("expected vpn.secrets.password to be created, got %v", merged["vpn"])
	}
}

func TestMergeVPNPasswordSecretDoesNotMutateInput(t *testing.T) {
	settings := sampleSettings()
	_ = mergeVPNPasswordSecret(settings, "s3cret-pw")

	if _, ok := settings["vpn"]["secrets"]; ok {
		t.Fatal("mergeVPNPasswordSecret mutated its input settings map")
	}
}
