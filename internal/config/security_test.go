package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSecurityConfigStructExists verifies that SecurityConfig struct exists
// with all required fields per the spec.
func TestSecurityConfigStructExists(t *testing.T) {
	tests := []struct {
		name   string
		config SecurityConfig
	}{
		{
			name: "default values",
			config: SecurityConfig{
				KillSwitchMode: "off",
				KillSwitchLAN:  false,
				DNSMode:        "system",
				CustomDNS:      []string{},
				BlockDoH:       false,
				BlockDoT:       false,
				IPv6Mode:       "auto",
				BlockWebRTC:    false,
			},
		},
		{
			name: "custom values",
			config: SecurityConfig{
				KillSwitchMode: "always-on",
				KillSwitchLAN:  true,
				DNSMode:        "custom",
				CustomDNS:      []string{"9.9.9.9", "149.112.112.112"},
				BlockDoH:       true,
				BlockDoT:       true,
				IPv6Mode:       "block",
				BlockWebRTC:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.config

			// Verify all fields are correctly assigned
			if cfg.KillSwitchMode != tt.config.KillSwitchMode {
				t.Errorf("KillSwitchMode: got %s, want %s", cfg.KillSwitchMode, tt.config.KillSwitchMode)
			}
			if cfg.KillSwitchLAN != tt.config.KillSwitchLAN {
				t.Errorf("KillSwitchLAN: got %v, want %v", cfg.KillSwitchLAN, tt.config.KillSwitchLAN)
			}
			if cfg.DNSMode != tt.config.DNSMode {
				t.Errorf("DNSMode: got %s, want %s", cfg.DNSMode, tt.config.DNSMode)
			}
			if len(cfg.CustomDNS) != len(tt.config.CustomDNS) {
				t.Errorf("CustomDNS length: got %d, want %d", len(cfg.CustomDNS), len(tt.config.CustomDNS))
			}
			if cfg.BlockDoH != tt.config.BlockDoH {
				t.Errorf("BlockDoH: got %v, want %v", cfg.BlockDoH, tt.config.BlockDoH)
			}
			if cfg.BlockDoT != tt.config.BlockDoT {
				t.Errorf("BlockDoT: got %v, want %v", cfg.BlockDoT, tt.config.BlockDoT)
			}
			if cfg.IPv6Mode != tt.config.IPv6Mode {
				t.Errorf("IPv6Mode: got %s, want %s", cfg.IPv6Mode, tt.config.IPv6Mode)
			}
			if cfg.BlockWebRTC != tt.config.BlockWebRTC {
				t.Errorf("BlockWebRTC: got %v, want %v", cfg.BlockWebRTC, tt.config.BlockWebRTC)
			}
		})
	}
}

// TestConfigHasSecurityField verifies that Config struct has Security field.
func TestConfigHasSecurityField(t *testing.T) {
	cfg := Config{
		Security: SecurityConfig{
			KillSwitchMode: "on-disconnect",
			DNSMode:        "cloudflare",
			IPv6Mode:       "block",
		},
	}

	if cfg.Security.KillSwitchMode != "on-disconnect" {
		t.Errorf("Security.KillSwitchMode: got %s, want on-disconnect", cfg.Security.KillSwitchMode)
	}
	if cfg.Security.DNSMode != "cloudflare" {
		t.Errorf("Security.DNSMode: got %s, want cloudflare", cfg.Security.DNSMode)
	}
	if cfg.Security.IPv6Mode != "block" {
		t.Errorf("Security.IPv6Mode: got %s, want block", cfg.Security.IPv6Mode)
	}
}

// TestSecurityConfigYAMLSerialization verifies YAML marshaling/unmarshaling.
func TestSecurityConfigYAMLSerialization(t *testing.T) {
	original := SecurityConfig{
		KillSwitchMode: "always-on",
		KillSwitchLAN:  true,
		DNSMode:        "custom",
		CustomDNS:      []string{"1.1.1.1", "8.8.8.8"},
		BlockDoH:       true,
		BlockDoT:       false,
		IPv6Mode:       "block",
		BlockWebRTC:    true,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal back
	var restored SecurityConfig
	if err := yaml.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify all fields preserved
	if restored.KillSwitchMode != original.KillSwitchMode {
		t.Errorf("KillSwitchMode: got %s, want %s", restored.KillSwitchMode, original.KillSwitchMode)
	}
	if restored.KillSwitchLAN != original.KillSwitchLAN {
		t.Errorf("KillSwitchLAN: got %v, want %v", restored.KillSwitchLAN, original.KillSwitchLAN)
	}
	if restored.DNSMode != original.DNSMode {
		t.Errorf("DNSMode: got %s, want %s", restored.DNSMode, original.DNSMode)
	}
	if len(restored.CustomDNS) != len(original.CustomDNS) {
		t.Errorf("CustomDNS length: got %d, want %d", len(restored.CustomDNS), len(original.CustomDNS))
	}
	for i := range original.CustomDNS {
		if restored.CustomDNS[i] != original.CustomDNS[i] {
			t.Errorf("CustomDNS[%d]: got %s, want %s", i, restored.CustomDNS[i], original.CustomDNS[i])
		}
	}
	if restored.IPv6Mode != original.IPv6Mode {
		t.Errorf("IPv6Mode: got %s, want %s", restored.IPv6Mode, original.IPv6Mode)
	}
}

// TestDefaultConfigHasSecurityDefaults verifies DefaultConfig() includes security defaults.
// Per spec: mode: off, dns: system, ipv6: auto
func TestDefaultConfigHasSecurityDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// Verify security defaults per spec
	if cfg.Security.KillSwitchMode != "off" {
		t.Errorf("Security.KillSwitchMode: got %s, want off", cfg.Security.KillSwitchMode)
	}
	if cfg.Security.KillSwitchLAN != false {
		t.Errorf("Security.KillSwitchLAN: got %v, want false", cfg.Security.KillSwitchLAN)
	}
	if cfg.Security.DNSMode != "system" {
		t.Errorf("Security.DNSMode: got %s, want system", cfg.Security.DNSMode)
	}
	if cfg.Security.CustomDNS == nil {
		t.Error("Security.CustomDNS should not be nil")
	}
	if len(cfg.Security.CustomDNS) != 0 {
		t.Errorf("Security.CustomDNS: got length %d, want 0", len(cfg.Security.CustomDNS))
	}
	if cfg.Security.BlockDoH != false {
		t.Errorf("Security.BlockDoH: got %v, want false", cfg.Security.BlockDoH)
	}
	if cfg.Security.BlockDoT != false {
		t.Errorf("Security.BlockDoT: got %v, want false", cfg.Security.BlockDoT)
	}
	if cfg.Security.IPv6Mode != "auto" {
		t.Errorf("Security.IPv6Mode: got %s, want auto", cfg.Security.IPv6Mode)
	}
	if cfg.Security.BlockWebRTC != false {
		t.Errorf("Security.BlockWebRTC: got %v, want false", cfg.Security.BlockWebRTC)
	}
}

// TestSecurityDefaultsInYAMLRoundTrip verifies security defaults survive save/load.
func TestSecurityDefaultsInYAMLRoundTrip(t *testing.T) {
	original := DefaultConfig()

	// Marshal to YAML
	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal back
	var restored Config
	if err := yaml.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify security defaults preserved
	if restored.Security.KillSwitchMode != "off" {
		t.Errorf("After round-trip, Security.KillSwitchMode: got %s, want off", restored.Security.KillSwitchMode)
	}
	if restored.Security.DNSMode != "system" {
		t.Errorf("After round-trip, Security.DNSMode: got %s, want system", restored.Security.DNSMode)
	}
	if restored.Security.IPv6Mode != "auto" {
		t.Errorf("After round-trip, Security.IPv6Mode: got %s, want auto", restored.Security.IPv6Mode)
	}
}

// TestValidateCustomDNS verifies DNS validation helper function.
func TestValidateCustomDNS(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantAddrs []string
		wantErr   bool
	}{
		{
			name:      "single valid IPv4",
			input:     "9.9.9.9",
			wantAddrs: []string{"9.9.9.9"},
			wantErr:   false,
		},
		{
			name:      "multiple valid IPv4 comma-separated",
			input:     "9.9.9.9,149.112.112.112",
			wantAddrs: []string{"9.9.9.9", "149.112.112.112"},
			wantErr:   false,
		},
		{
			name:      "multiple with spaces",
			input:     "1.1.1.1, 8.8.8.8, 9.9.9.9",
			wantAddrs: []string{"1.1.1.1", "8.8.8.8", "9.9.9.9"},
			wantErr:   false,
		},
		{
			name:      "invalid IP address",
			input:     "not-an-ip",
			wantAddrs: nil,
			wantErr:   true,
		},
		{
			name:      "mixed valid and invalid",
			input:     "1.1.1.1,invalid,8.8.8.8",
			wantAddrs: nil,
			wantErr:   true,
		},
		{
			name:      "empty string",
			input:     "",
			wantAddrs: []string{},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAddrs, err := ValidateCustomDNS(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCustomDNS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(gotAddrs) != len(tt.wantAddrs) {
					t.Errorf("ValidateCustomDNS() length = %d, want %d", len(gotAddrs), len(tt.wantAddrs))
					return
				}
				for i := range tt.wantAddrs {
					if gotAddrs[i] != tt.wantAddrs[i] {
						t.Errorf("ValidateCustomDNS()[%d] = %s, want %s", i, gotAddrs[i], tt.wantAddrs[i])
					}
				}
			}
		})
	}
}

// TestDedupeServers verifies server deduplication helper.
func TestDedupeServers(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "no duplicates",
			input: []string{"1.1.1.1", "8.8.8.8"},
			want:  []string{"1.1.1.1", "8.8.8.8"},
		},
		{
			name:  "with duplicates",
			input: []string{"1.1.1.1", "8.8.8.8", "1.1.1.1"},
			want:  []string{"1.1.1.1", "8.8.8.8"},
		},
		{
			name:  "all duplicates",
			input: []string{"1.1.1.1", "1.1.1.1", "1.1.1.1"},
			want:  []string{"1.1.1.1"},
		},
		{
			name:  "empty slice",
			input: []string{},
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DedupeServers(tt.input)

			if len(got) != len(tt.want) {
				t.Errorf("DedupeServers() length = %d, want %d", len(got), len(tt.want))
				return
			}

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("DedupeServers()[%d] = %s, want %s", i, got[i], tt.want[i])
				}
			}
		})
	}
}
