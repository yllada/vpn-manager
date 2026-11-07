package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestImportRejectsDangerousContent pins that Import will not persist a profile
// whose ConfigContent contains an executable directive (the audit's
// unvalidated-Import defect): the entry is skipped, no config file is written,
// and a clean profile in the same backup still imports.
func TestImportRejectsDangerousContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	malicious := "client\nremote 10.0.0.1 1194\nup /bin/sh -c id\n"
	clean := "client\nremote 10.0.0.2 1194\n"

	var b strings.Builder
	b.WriteString("version: \"1.0\"\nprofiles:\n")
	b.WriteString("  - name: \"Bad\"\n    config_content: |\n")
	for _, l := range strings.Split(strings.TrimRight(malicious, "\n"), "\n") {
		fmt.Fprintf(&b, "      %s\n", l)
	}
	b.WriteString("  - name: \"Good\"\n    config_content: |\n")
	for _, l := range strings.Split(strings.TrimRight(clean, "\n"), "\n") {
		fmt.Fprintf(&b, "      %s\n", l)
	}

	backup := filepath.Join(home, "backup.yaml")
	if err := os.WriteFile(backup, []byte(b.String()), 0600); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	pm, err := NewProfileManager()
	if err != nil {
		t.Fatalf("NewProfileManager: %v", err)
	}

	n, err := pm.Import(backup)
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if n != 1 {
		t.Fatalf("Import imported %d profiles, want 1 (only the clean one)", n)
	}

	// No config file may contain the dangerous content.
	entries, _ := os.ReadDir(pm.configDir)
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(pm.configDir, e.Name()))
		if strings.Contains(string(data), "up /bin/sh") {
			t.Fatalf("Import persisted a dangerous config: %s", e.Name())
		}
	}

	// The dangerous profile must not be listed.
	for _, p := range pm.List() {
		if p.Name == "Bad" {
			t.Fatalf("Import listed the dangerous profile %q", p.Name)
		}
	}
}
