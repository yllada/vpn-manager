package tailscale

import (
	"context"
	"testing"
	"time"
)

// TestReceivedFileStructExists verifies that ReceivedFile struct exists
// with required fields.
func TestReceivedFileStructExists(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		sender   string
	}{
		{"PDF document", "document.pdf", "laptop"},
		{"Image file", "photo.jpg", "phone"},
		{"With spaces", "My Document.pdf", "work-desktop"},
		{"Special chars", "report_2024-04-12.xlsx", "server-01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now()
			rf := ReceivedFile{
				Filename: tt.filename,
				Sender:   tt.sender,
				Time:     now,
			}

			if rf.Filename != tt.filename {
				t.Errorf("Filename: got %s, want %s", rf.Filename, tt.filename)
			}
			if rf.Sender != tt.sender {
				t.Errorf("Sender: got %s, want %s", rf.Sender, tt.sender)
			}
			if rf.Time != now {
				t.Errorf("Time: got %v, want %v", rf.Time, now)
			}
		})
	}
}

// TestReceivedFilePatternParsing verifies that the regex pattern correctly
// parses tailscale file get --verbose output.
func TestReceivedFilePatternParsing(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		wantFilename string
		wantSender   string
		wantMatch    bool
	}{
		{
			name:         "basic file",
			line:         "Wrote photo.jpg (from laptop)",
			wantFilename: "photo.jpg",
			wantSender:   "laptop",
			wantMatch:    true,
		},
		{
			name:         "file with spaces",
			line:         "Wrote My Document.pdf (from work-desktop)",
			wantFilename: "My Document.pdf",
			wantSender:   "work-desktop",
			wantMatch:    true,
		},
		{
			name:         "file with underscores",
			line:         "Wrote report_2024-04-12.xlsx (from server-01)",
			wantFilename: "report_2024-04-12.xlsx",
			wantSender:   "server-01",
			wantMatch:    true,
		},
		{
			name:         "sender with dots",
			line:         "Wrote file.txt (from node.tailnet.ts.net)",
			wantFilename: "file.txt",
			wantSender:   "node.tailnet.ts.net",
			wantMatch:    true,
		},
		{
			name:      "non-matching line",
			line:      "Some other output",
			wantMatch: false,
		},
		{
			name:      "empty line",
			line:      "",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := receivedFilePattern.FindStringSubmatch(tt.line)

			if tt.wantMatch {
				if matches == nil {
					t.Fatalf("expected pattern to match, but got nil")
				}
				if len(matches) != 3 {
					t.Fatalf("expected 3 capture groups, got %d", len(matches))
				}

				filename := matches[1]
				sender := matches[2]

				if filename != tt.wantFilename {
					t.Errorf("filename: got %q, want %q", filename, tt.wantFilename)
				}
				if sender != tt.wantSender {
					t.Errorf("sender: got %q, want %q", sender, tt.wantSender)
				}
			} else {
				if matches != nil {
					t.Errorf("expected no match, but got matches: %v", matches)
				}
			}
		})
	}
}

// TestStartReceiveLoopExists verifies that StartReceiveLoop method exists
// and returns a cancel function.
func TestStartReceiveLoopExists(t *testing.T) {
	m := &Manager{binaryPath: "/usr/bin/tailscale"}
	ctx := context.Background()

	// Track received files
	var received []ReceivedFile
	onReceive := func(rf ReceivedFile) {
		received = append(received, rf)
	}

	// Should return a cancel function
	cancel := m.StartReceiveLoop(ctx, "/tmp/taildrop", onReceive)
	if cancel == nil {
		t.Fatal("StartReceiveLoop should return a non-nil cancel function")
	}

	// Call cancel immediately to stop the loop
	cancel()
}
