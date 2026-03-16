package app

import (
	"testing"
)

func TestSecureString_Basic(t *testing.T) {
	secret := "my-secret-password"
	ss := NewSecureString(secret)

	// Verify content
	if ss.String() != secret {
		t.Errorf("Expected %s, got %s", secret, ss.String())
	}

	// Verify length
	if ss.Len() != len(secret) {
		t.Errorf("Expected length %d, got %d", len(secret), ss.Len())
	}

	// Verify not empty
	if ss.IsEmpty() {
		t.Error("Expected non-empty")
	}
}

func TestSecureString_Zero(t *testing.T) {
	secret := "sensitive-data"
	ss := NewSecureString(secret)

	// Zero the data
	ss.Zero()

	// Verify zeroed
	if !ss.IsEmpty() {
		t.Error("Expected empty after Zero()")
	}

	if ss.String() != "" {
		t.Error("Expected empty string after Zero()")
	}

	if ss.Len() != 0 {
		t.Error("Expected length 0 after Zero()")
	}
}

func TestSecureString_Equal(t *testing.T) {
	s1 := NewSecureString("password123")
	s2 := NewSecureString("password123")
	s3 := NewSecureString("different")

	if !s1.Equal(s2) {
		t.Error("Expected equal strings to match")
	}

	if s1.Equal(s3) {
		t.Error("Expected different strings to not match")
	}
}

func TestSecureString_EqualString(t *testing.T) {
	ss := NewSecureString("test")

	if !ss.EqualString("test") {
		t.Error("Expected match with same string")
	}

	if ss.EqualString("other") {
		t.Error("Expected no match with different string")
	}
}

func TestSecureString_Clone(t *testing.T) {
	original := NewSecureString("original-secret")
	clone := original.Clone()

	// Verify clone has same content
	if !original.Equal(clone) {
		t.Error("Clone should equal original")
	}

	// Zero original, clone should still work
	original.Zero()

	if clone.String() != "original-secret" {
		t.Error("Clone should be independent of original")
	}

	if clone.IsEmpty() {
		t.Error("Clone should not be affected by original.Zero()")
	}
}

func TestSecureCredentials(t *testing.T) {
	creds := NewSecureCredentials("user", "pass")

	if creds.Username.String() != "user" {
		t.Error("Username mismatch")
	}

	if creds.Password.String() != "pass" {
		t.Error("Password mismatch")
	}

	// Test OTP
	if creds.HasOTP() {
		t.Error("Should not have OTP")
	}

	credsOTP := NewSecureCredentialsWithOTP("user", "pass", "123456")
	if !credsOTP.HasOTP() {
		t.Error("Should have OTP")
	}
}

func TestSecureCredentials_Zero(t *testing.T) {
	creds := NewSecureCredentialsWithOTP("user", "pass", "otp")
	creds.Zero()

	if !creds.Username.IsEmpty() {
		t.Error("Username should be zeroed")
	}
	if !creds.Password.IsEmpty() {
		t.Error("Password should be zeroed")
	}
	if !creds.OTP.IsEmpty() {
		t.Error("OTP should be zeroed")
	}
}

func TestSecureCredentials_CombinedPassword(t *testing.T) {
	creds := NewSecureCredentialsWithOTP("user", "pass", "123456")
	combined := creds.GetCombinedPassword()
	defer combined.Zero()

	if combined.String() != "pass123456" {
		t.Errorf("Expected pass123456, got %s", combined.String())
	}
}

func TestSecureBufferPool(t *testing.T) {
	pool := NewSecureBufferPool(1024)

	buf := pool.Get()
	if buf.Len() != 1024 {
		t.Errorf("Expected buffer size 1024, got %d", buf.Len())
	}

	// Write some data
	copy(buf.Bytes(), []byte("secret"))

	// Release should zero the buffer
	buf.Release()

	// Get another buffer (might be the same one)
	buf2 := pool.Get()
	for _, b := range buf2.Bytes() {
		if b != 0 {
			t.Error("Buffer should be zeroed after release")
			break
		}
	}
}

func TestZeroBytes(t *testing.T) {
	data := []byte("sensitive")
	ZeroBytes(data)

	for i, b := range data {
		if b != 0 {
			t.Errorf("Byte %d not zeroed: %d", i, b)
		}
	}
}
