package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadToleratesDeprecatedTaildropReceiveKeys guards backward compatibility:
// the Taildrop auto-receive feature was removed, but its config keys
// (taildrop_auto_receive, taildrop_dir) were written by older versions and the
// loader uses a strict (KnownFields) decoder. An upgraded install with an
// existing config.yaml must still load instead of failing on an unknown key.
func TestLoadToleratesDeprecatedTaildropReceiveKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgDir := filepath.Join(home, ".config", ConfigDirName)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	// A config as written by an older version, including the now-removed keys.
	legacy := "" +
		"theme: dark\n" +
		"tailscale:\n" +
		"  taildrop: true\n" +
		"  taildrop_dir: /home/user/Downloads/Taildrop\n" +
		"  taildrop_auto_receive: true\n"
	if err := os.WriteFile(filepath.Join(cfgDir, ConfigFileName), []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() rejected a legacy config with deprecated taildrop keys: %v", err)
	}
	if !cfg.Tailscale.Taildrop {
		t.Error("expected taildrop (send) to remain enabled from the legacy config")
	}
}
