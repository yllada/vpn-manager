package notify

import (
	"testing"
)

// TestFileReceivedFunctionExists verifies that FileReceived function exists
// and can be called with filename and sender parameters.
func TestFileReceivedFunctionExists(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		sender   string
	}{
		{
			name:     "PDF from laptop",
			filename: "document.pdf",
			sender:   "laptop",
		},
		{
			name:     "image from phone",
			filename: "photo.jpg",
			sender:   "phone",
		},
		{
			name:     "file with spaces",
			filename: "My Document.pdf",
			sender:   "work-desktop",
		},
		{
			name:     "file with special chars",
			filename: "report_2024-04-12.xlsx",
			sender:   "server-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call should not panic
			FileReceived(tt.filename, tt.sender)
		})
	}
}
