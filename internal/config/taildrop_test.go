package config

import (
	"testing"
)

// TestTailscaleConfigHasTaildropAutoReceive verifies that TailscaleConfig has
// the TaildropAutoReceive field.
func TestTailscaleConfigHasTaildropAutoReceive(t *testing.T) {
	tests := []struct {
		name  string
		value bool
	}{
		{"enabled", true},
		{"disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := TailscaleConfig{
				TaildropAutoReceive: tt.value,
			}

			if cfg.TaildropAutoReceive != tt.value {
				t.Errorf("TaildropAutoReceive: got %v, want %v", cfg.TaildropAutoReceive, tt.value)
			}
		})
	}
}

// TestDefaultConfigHasTaildropAutoReceiveTrue verifies that DefaultConfig()
// sets TaildropAutoReceive to true by default.
func TestDefaultConfigHasTaildropAutoReceiveTrue(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Tailscale.TaildropAutoReceive != true {
		t.Errorf("DefaultConfig().Tailscale.TaildropAutoReceive: got %v, want true", cfg.Tailscale.TaildropAutoReceive)
	}

	// Verify related Taildrop settings are also enabled
	if cfg.Tailscale.Taildrop != true {
		t.Errorf("DefaultConfig().Tailscale.Taildrop: got %v, want true (should be enabled for auto-receive to work)", cfg.Tailscale.Taildrop)
	}

	if cfg.Tailscale.TaildropDir == "" {
		t.Error("DefaultConfig().Tailscale.TaildropDir: got empty string, want non-empty directory path")
	}
}
