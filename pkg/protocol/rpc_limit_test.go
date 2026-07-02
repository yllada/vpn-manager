package protocol

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestReadLimitedLine(t *testing.T) {
	// A normal newline-delimited message reads back intact.
	r := bufio.NewReader(strings.NewReader("hello world\n"))
	line, err := readLimitedLine(r, 1024)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(line) != "hello world\n" {
		t.Errorf("got %q, want %q", line, "hello world\n")
	}
}

func TestReadLimitedLineExceedsLimit(t *testing.T) {
	// A stream with no newline that exceeds the limit must fail with
	// ErrMessageTooLarge rather than buffering unboundedly (OOM DoS on the daemon).
	huge := bytes.Repeat([]byte("A"), 5000) // no '\n'
	r := bufio.NewReader(bytes.NewReader(huge))
	_, err := readLimitedLine(r, 1024)
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("got %v, want ErrMessageTooLarge", err)
	}
}

func TestReadLimitedLineAtBoundary(t *testing.T) {
	// A line exactly at the limit (including the newline) is accepted.
	payload := strings.Repeat("B", 1023) + "\n" // 1024 bytes total
	r := bufio.NewReader(strings.NewReader(payload))
	line, err := readLimitedLine(r, 1024)
	if err != nil {
		t.Fatalf("boundary line should be accepted, got: %v", err)
	}
	if len(line) != 1024 {
		t.Errorf("got %d bytes, want 1024", len(line))
	}
}
