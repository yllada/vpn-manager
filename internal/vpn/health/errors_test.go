package health

import (
	"errors"
	"testing"
)

func TestSentinelErrorsExist(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrProbeTimeout", ErrProbeTimeout},
		{"ErrProbeConnectionRefused", ErrProbeConnectionRefused},
		{"ErrAllProbesFailed", ErrAllProbesFailed},
		{"ErrICMPNotAvailable", ErrICMPNotAvailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s should not be nil", tt.name)
			}
		})
	}
}

func TestSentinelErrorsAreDistinct(t *testing.T) {
	errs := []error{
		ErrProbeTimeout,
		ErrProbeConnectionRefused,
		ErrAllProbesFailed,
		ErrICMPNotAvailable,
	}

	for i := 0; i < len(errs); i++ {
		for j := i + 1; j < len(errs); j++ {
			if errors.Is(errs[i], errs[j]) {
				t.Errorf("errors should be distinct: %v and %v", errs[i], errs[j])
			}
		}
	}
}
