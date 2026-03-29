package components

import (
	"strings"
	"testing"

	"github.com/yllada/vpn-manager/cli/tui/styles"
)

func TestNewSparkline(t *testing.T) {
	t.Run("creates sparkline with default width", func(t *testing.T) {
		s := NewSparkline(0, styles.ColorConnected)
		if s == nil {
			t.Fatal("expected non-nil sparkline")
		}
		// Default width should be 20
		if s.width != 20 {
			t.Errorf("expected width 20, got %d", s.width)
		}
	})

	t.Run("creates sparkline with specified width", func(t *testing.T) {
		s := NewSparkline(30, styles.ColorConnected)
		if s.width != 30 {
			t.Errorf("expected width 30, got %d", s.width)
		}
	})

	t.Run("respects options", func(t *testing.T) {
		s := NewSparkline(20, styles.ColorConnected,
			WithLabel("Test"),
			WithShowValue(true),
			WithCharset(CharsetBraille),
		)
		if s.label != "Test" {
			t.Errorf("expected label 'Test', got '%s'", s.label)
		}
		if !s.showValue {
			t.Error("expected showValue to be true")
		}
		if s.charset != CharsetBraille {
			t.Errorf("expected CharsetBraille, got %d", s.charset)
		}
	})
}

func TestSparklinePush(t *testing.T) {
	t.Run("adds data points", func(t *testing.T) {
		s := NewSparkline(5, styles.ColorConnected)
		s.Push(1.0)
		s.Push(2.0)
		s.Push(3.0)

		if len(s.data) != 3 {
			t.Errorf("expected 3 data points, got %d", len(s.data))
		}
	})

	t.Run("trims old data when exceeding width", func(t *testing.T) {
		s := NewSparkline(3, styles.ColorConnected)
		s.Push(1.0)
		s.Push(2.0)
		s.Push(3.0)
		s.Push(4.0)
		s.Push(5.0)

		if len(s.data) != 3 {
			t.Errorf("expected 3 data points, got %d", len(s.data))
		}
		// Should have the newest values
		if s.data[0] != 3.0 || s.data[1] != 4.0 || s.data[2] != 5.0 {
			t.Errorf("expected [3, 4, 5], got %v", s.data)
		}
	})
}

func TestSparklinePushBatch(t *testing.T) {
	s := NewSparkline(5, styles.ColorConnected)
	s.PushBatch([]float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0})

	if len(s.data) != 5 {
		t.Errorf("expected 5 data points, got %d", len(s.data))
	}
	// Should have the newest values
	if s.data[0] != 2.0 {
		t.Errorf("expected first value 2.0, got %f", s.data[0])
	}
}

func TestSparklineCurrent(t *testing.T) {
	t.Run("returns 0 when empty", func(t *testing.T) {
		s := NewSparkline(5, styles.ColorConnected)
		if s.Current() != 0 {
			t.Errorf("expected 0, got %f", s.Current())
		}
	})

	t.Run("returns most recent value", func(t *testing.T) {
		s := NewSparkline(5, styles.ColorConnected)
		s.Push(1.0)
		s.Push(2.0)
		s.Push(3.0)

		if s.Current() != 3.0 {
			t.Errorf("expected 3.0, got %f", s.Current())
		}
	})
}

func TestSparklinePeak(t *testing.T) {
	t.Run("returns 0 when empty", func(t *testing.T) {
		s := NewSparkline(5, styles.ColorConnected)
		if s.Peak() != 0 {
			t.Errorf("expected 0, got %f", s.Peak())
		}
	})

	t.Run("returns maximum value", func(t *testing.T) {
		s := NewSparkline(5, styles.ColorConnected)
		s.Push(1.0)
		s.Push(5.0)
		s.Push(3.0)

		if s.Peak() != 5.0 {
			t.Errorf("expected 5.0, got %f", s.Peak())
		}
	})
}

func TestSparklineAverage(t *testing.T) {
	t.Run("returns 0 when empty", func(t *testing.T) {
		s := NewSparkline(5, styles.ColorConnected)
		if s.Average() != 0 {
			t.Errorf("expected 0, got %f", s.Average())
		}
	})

	t.Run("returns average of values", func(t *testing.T) {
		s := NewSparkline(5, styles.ColorConnected)
		s.Push(2.0)
		s.Push(4.0)
		s.Push(6.0)

		if s.Average() != 4.0 {
			t.Errorf("expected 4.0, got %f", s.Average())
		}
	})
}

func TestSparklineRender(t *testing.T) {
	t.Run("renders empty sparkline with low bars", func(t *testing.T) {
		s := NewSparkline(5, styles.ColorConnected)
		result := s.Render()

		// Should contain 5 low-level characters
		if len(result) < 5 {
			t.Error("expected at least 5 characters in output")
		}
	})

	t.Run("renders data with block characters", func(t *testing.T) {
		s := NewSparkline(3, styles.ColorConnected, WithCharset(CharsetBlocks))
		s.Push(0.0)
		s.Push(0.5)
		s.Push(1.0)

		result := s.Render()
		// Should contain block characters
		if !containsBlockChar(result) {
			t.Error("expected block characters in output")
		}
	})

	t.Run("renders data with braille characters", func(t *testing.T) {
		s := NewSparkline(3, styles.ColorConnected, WithCharset(CharsetBraille))
		s.Push(0.0)
		s.Push(0.5)
		s.Push(1.0)

		result := s.Render()
		// Should contain braille characters
		if !containsBrailleChar(result) {
			t.Error("expected braille characters in output")
		}
	})

	t.Run("pads with low bars when data < width", func(t *testing.T) {
		s := NewSparkline(5, styles.ColorConnected, WithCharset(CharsetBlocks))
		s.Push(1.0)

		result := s.Render()
		// Should start with padding (low bars)
		if !strings.Contains(result, "▁") {
			t.Error("expected padding with low bars")
		}
	})
}

func TestSparklineRenderWithLabel(t *testing.T) {
	t.Run("includes label", func(t *testing.T) {
		s := NewSparkline(5, styles.ColorConnected)
		result := s.RenderWithLabel("Download")

		if !strings.Contains(result, "Download") {
			t.Error("expected label in output")
		}
	})

	t.Run("includes value when enabled", func(t *testing.T) {
		s := NewSparkline(5, styles.ColorConnected,
			WithShowValue(true),
			WithValueFormatter(func(v float64) string { return "42.0" }),
		)
		s.Push(42.0)

		result := s.RenderWithLabel("Test")
		if !strings.Contains(result, "42.0") {
			t.Error("expected formatted value in output")
		}
	})
}

func TestBandwidthSparkline(t *testing.T) {
	t.Run("creates download sparkline", func(t *testing.T) {
		s := NewBandwidthSparkline(20, DirectionDownload)
		if s == nil {
			t.Fatal("expected non-nil sparkline")
		}
		if s.direction != DirectionDownload {
			t.Error("expected DirectionDownload")
		}
	})

	t.Run("creates upload sparkline", func(t *testing.T) {
		s := NewBandwidthSparkline(20, DirectionUpload)
		if s == nil {
			t.Fatal("expected non-nil sparkline")
		}
		if s.direction != DirectionUpload {
			t.Error("expected DirectionUpload")
		}
	})
}

func TestBandwidthPanel(t *testing.T) {
	t.Run("creates panel with both sparklines", func(t *testing.T) {
		p := NewBandwidthPanel(20)
		if p == nil {
			t.Fatal("expected non-nil panel")
		}
		if p.download == nil || p.upload == nil {
			t.Error("expected both download and upload sparklines")
		}
	})

	t.Run("pushes data to both sparklines", func(t *testing.T) {
		p := NewBandwidthPanel(20)
		p.Push(1000.0, 500.0)

		if p.GetDownloadCurrent() != 1000.0 {
			t.Errorf("expected download 1000.0, got %f", p.GetDownloadCurrent())
		}
		if p.GetUploadCurrent() != 500.0 {
			t.Errorf("expected upload 500.0, got %f", p.GetUploadCurrent())
		}
	})

	t.Run("clears both sparklines", func(t *testing.T) {
		p := NewBandwidthPanel(20)
		p.Push(1000.0, 500.0)
		p.Clear()

		if p.GetDownloadCurrent() != 0 {
			t.Error("expected download to be 0 after clear")
		}
		if p.GetUploadCurrent() != 0 {
			t.Error("expected upload to be 0 after clear")
		}
	})

	t.Run("renders view with both lines", func(t *testing.T) {
		p := NewBandwidthPanel(20)
		p.Push(1024*1024, 512*1024) // 1 MB/s down, 512 KB/s up

		result := p.View()
		if !strings.Contains(result, "↓") {
			t.Error("expected download arrow in view")
		}
		if !strings.Contains(result, "↑") {
			t.Error("expected upload arrow in view")
		}
	})

	t.Run("renders compact view", func(t *testing.T) {
		p := NewBandwidthPanel(20)
		p.Push(1024*1024, 512*1024)

		result := p.ViewCompact()
		// Should have both arrows on one line
		if !strings.Contains(result, "↓") || !strings.Contains(result, "↑") {
			t.Error("expected both arrows in compact view")
		}
	})
}

func TestFormatBandwidth(t *testing.T) {
	tests := []struct {
		input    float64
		contains string
	}{
		{100, "B/s"},
		{2048, "KB/s"},
		{1024 * 1024 * 2.5, "MB/s"},
		{1024 * 1024 * 1024 * 1.5, "GB/s"},
	}

	for _, tt := range tests {
		result := formatBandwidth(tt.input)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("formatBandwidth(%f) = %s, expected to contain %s",
				tt.input, result, tt.contains)
		}
	}
}

// Helper functions

func containsBlockChar(s string) bool {
	blocks := "▁▂▃▄▅▆▇█"
	for _, r := range s {
		if strings.ContainsRune(blocks, r) {
			return true
		}
	}
	return false
}

func containsBrailleChar(s string) bool {
	braille := "⡀⡄⡆⡇⣇⣧⣷⣿"
	for _, r := range s {
		if strings.ContainsRune(braille, r) {
			return true
		}
	}
	return false
}
