package protocol

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// TestCodecCloseDuringRead pins the read-response-deadlock regression: Close
// must return even while a ReadResponse is blocked on the connection.
func TestCodecCloseDuringRead(t *testing.T) {
	a, b := net.Pipe()
	codec := NewCodec(b)

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		codec.ReadResponse() // blocks: a is silent
	}()

	// Give the reader a moment to block on a.
	time.Sleep(50 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- codec.Close() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: Close did not return while a read was in flight")
	}
	b.Close()
	a.Close()
	<-readDone
}

// TestClientCloseDuringPendingCall pins the same guarantee through the public
// API: a client with a call in flight (a daemon that never answers) must have
// Close return promptly.
func TestClientCloseDuringPendingCall(t *testing.T) {
	sr, err := net.Listen("unix", filepath.Join(t.TempDir(), "sock"))
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { sr.Close() })
	go func() {
		for {
			conn, err := sr.Accept()
			if err != nil {
				return
			}
			// Accept and stay silent: keep the connection open so the client
			// blocks in read; never respond.
			_ = conn
		}
	}()

	c := NewClient(WithSocketPath(sr.Addr().String()), WithTimeout(20*time.Second))
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	go c.Call(context.Background(), "system.ping", nil, nil) // blocks

	done := make(chan error, 1)
	go func() { done <- c.Close() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: Client.Close did not return with a pending call")
	}
}
