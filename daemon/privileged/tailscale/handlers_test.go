package tailscale

import (
	"context"
	"testing"

	"github.com/yllada/vpn-manager/daemon"
)

// TestTaildropSendParams_Validation tests TaildropSendParams validation.
func TestTaildropSendParams_Validation(t *testing.T) {
	tests := []struct {
		name    string
		params  TaildropSendParams
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid params",
			params: TaildropSendParams{
				FilePath: "/home/user/test.txt",
				Target:   "laptop",
			},
			wantErr: false,
		},
		{
			name: "empty file path",
			params: TaildropSendParams{
				FilePath: "",
				Target:   "laptop",
			},
			wantErr: true,
			errMsg:  "file_path is required",
		},
		{
			name: "empty target",
			params: TaildropSendParams{
				FilePath: "/home/user/test.txt",
				Target:   "",
			},
			wantErr: true,
			errMsg:  "target is required",
		},
		{
			name: "both empty",
			params: TaildropSendParams{
				FilePath: "",
				Target:   "",
			},
			wantErr: true,
			errMsg:  "file_path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("expected error message %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestManager_SendFile tests the Manager.SendFile method.
func TestManager_SendFile(t *testing.T) {
	// This test requires the tailscale binary to be present.
	// We'll test the error case for now (file not found).
	manager, err := NewManager()
	if err != nil {
		t.Skipf("tailscale binary not found: %v", err)
	}

	ctx := context.Background()
	params := TaildropSendParams{
		FilePath: "/nonexistent/file.txt",
		Target:   "test-device",
	}

	result, err := manager.SendFile(ctx, params)
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %+v", result)
	}
}

// TestTaildropSendHandler tests the TaildropSendHandler.
func TestTaildropSendHandler(t *testing.T) {
	state := daemon.NewState()
	handler := TaildropSendHandler(state)

	if handler == nil {
		t.Fatal("expected non-nil handler")
	}

	// Test that handler exists and has the right signature
	// Full integration testing would require mock daemon context
	// For now, we verify the handler can be created
}
