// Package app provides security utilities for sensitive data handling.
// This module implements secure credential handling following OWASP guidelines
// and industry best practices from ProtonVPN, Mullvad, and NordVPN.
package app

import (
	"crypto/rand"
	"crypto/subtle"
	"runtime"
	"sync"
	"unsafe"
)

// SecureString wraps sensitive string data with automatic memory cleaning.
// It uses a byte slice internally to allow zeroing memory on cleanup.
// OWASP: https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html
type SecureString struct {
	mu     sync.RWMutex
	data   []byte
	length int
	zeroed bool
}

// NewSecureString creates a SecureString from a regular string.
// The original string should be cleared by the caller if possible.
func NewSecureString(s string) *SecureString {
	ss := &SecureString{
		data:   make([]byte, len(s)),
		length: len(s),
		zeroed: false,
	}
	copy(ss.data, s)
	return ss
}

// NewSecureStringFromBytes creates a SecureString from a byte slice.
// The original slice is NOT zeroed - caller should handle that.
func NewSecureStringFromBytes(b []byte) *SecureString {
	ss := &SecureString{
		data:   make([]byte, len(b)),
		length: len(b),
		zeroed: false,
	}
	copy(ss.data, b)
	return ss
}

// String returns the string value. Use sparingly as strings are immutable.
// Prefer UnsafeBytes() when possible for operations that modify or compare.
func (ss *SecureString) String() string {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	if ss.zeroed || ss.data == nil {
		return ""
	}
	return string(ss.data)
}

// UnsafeBytes returns the underlying byte slice for secure operations.
// WARNING: Do not store this reference; contents may be zeroed.
func (ss *SecureString) UnsafeBytes() []byte {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	if ss.zeroed || ss.data == nil {
		return nil
	}
	return ss.data
}

// Len returns the length of the secure string.
func (ss *SecureString) Len() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.length
}

// IsEmpty returns true if the string is empty or zeroed.
func (ss *SecureString) IsEmpty() bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.length == 0 || ss.zeroed
}

// Equal performs constant-time comparison with another SecureString.
// Returns true if both contain the same data.
func (ss *SecureString) Equal(other *SecureString) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	if other == nil {
		return ss.zeroed || ss.length == 0
	}

	other.mu.RLock()
	defer other.mu.RUnlock()

	if ss.length != other.length {
		return false
	}

	return subtle.ConstantTimeCompare(ss.data, other.data) == 1
}

// EqualString performs constant-time comparison with a regular string.
func (ss *SecureString) EqualString(s string) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	if ss.zeroed {
		return len(s) == 0
	}

	if ss.length != len(s) {
		return false
	}

	return subtle.ConstantTimeCompare(ss.data, []byte(s)) == 1
}

// Zero securely wipes the memory containing the sensitive data.
// After calling Zero(), the SecureString should not be used.
func (ss *SecureString) Zero() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.zeroed || ss.data == nil {
		return
	}

	// Overwrite with cryptographically random data first
	// This helps against cold boot attacks
	_, _ = rand.Read(ss.data)

	// Then zero out
	for i := range ss.data {
		ss.data[i] = 0
	}

	// Ensure compiler doesn't optimize away the zeroing
	runtime.KeepAlive(ss.data)

	ss.zeroed = true
	ss.length = 0
}

// Clone creates a deep copy of the SecureString.
func (ss *SecureString) Clone() *SecureString {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	if ss.zeroed || ss.data == nil {
		return &SecureString{zeroed: true}
	}

	clone := &SecureString{
		data:   make([]byte, ss.length),
		length: ss.length,
		zeroed: false,
	}
	copy(clone.data, ss.data)
	return clone
}

// SecureCredentials holds username and password with automatic cleanup.
type SecureCredentials struct {
	Username *SecureString
	Password *SecureString
	OTP      *SecureString
	AuthKey  *SecureString
}

// NewSecureCredentials creates credentials from plain strings.
// Original strings should be cleared by caller if possible.
func NewSecureCredentials(username, password string) *SecureCredentials {
	return &SecureCredentials{
		Username: NewSecureString(username),
		Password: NewSecureString(password),
	}
}

// NewSecureCredentialsWithOTP creates credentials with OTP support.
func NewSecureCredentialsWithOTP(username, password, otp string) *SecureCredentials {
	return &SecureCredentials{
		Username: NewSecureString(username),
		Password: NewSecureString(password),
		OTP:      NewSecureString(otp),
	}
}

// Zero securely wipes all credential data.
func (sc *SecureCredentials) Zero() {
	if sc.Username != nil {
		sc.Username.Zero()
	}
	if sc.Password != nil {
		sc.Password.Zero()
	}
	if sc.OTP != nil {
		sc.OTP.Zero()
	}
	if sc.AuthKey != nil {
		sc.AuthKey.Zero()
	}
}

// HasOTP returns true if OTP is present and non-empty.
func (sc *SecureCredentials) HasOTP() bool {
	return sc.OTP != nil && !sc.OTP.IsEmpty()
}

// GetCombinedPassword returns password+OTP if OTP exists, else just password.
// Returns a new SecureString that must be Zero()'d by caller.
func (sc *SecureCredentials) GetCombinedPassword() *SecureString {
	if sc.Password == nil {
		return NewSecureString("")
	}

	if sc.OTP == nil || sc.OTP.IsEmpty() {
		return sc.Password.Clone()
	}

	// Combine password + OTP
	combined := make([]byte, 0, sc.Password.Len()+sc.OTP.Len())
	combined = append(combined, sc.Password.UnsafeBytes()...)
	combined = append(combined, sc.OTP.UnsafeBytes()...)

	result := NewSecureStringFromBytes(combined)

	// Zero the temporary
	for i := range combined {
		combined[i] = 0
	}

	return result
}

// SecureBuffer provides a reusable buffer for sensitive operations.
// It automatically zeros memory when released back to the pool.
type SecureBuffer struct {
	data []byte
	pool *SecureBufferPool
}

// Bytes returns the buffer's byte slice.
func (sb *SecureBuffer) Bytes() []byte {
	return sb.data
}

// Len returns the buffer length.
func (sb *SecureBuffer) Len() int {
	return len(sb.data)
}

// Zero clears the buffer contents.
func (sb *SecureBuffer) Zero() {
	if sb.data == nil {
		return
	}
	for i := range sb.data {
		sb.data[i] = 0
	}
	runtime.KeepAlive(sb.data)
}

// Release returns the buffer to the pool after zeroing.
func (sb *SecureBuffer) Release() {
	if sb.pool != nil {
		sb.Zero()
		sb.pool.put(sb)
	}
}

// SecureBufferPool manages reusable secure buffers.
type SecureBufferPool struct {
	pool sync.Pool
	size int
}

// NewSecureBufferPool creates a pool of secure buffers with given size.
func NewSecureBufferPool(bufferSize int) *SecureBufferPool {
	p := &SecureBufferPool{
		size: bufferSize,
	}
	p.pool = sync.Pool{
		New: func() interface{} {
			return &SecureBuffer{
				data: make([]byte, bufferSize),
				pool: p,
			}
		},
	}
	return p
}

// Get retrieves a buffer from the pool.
func (p *SecureBufferPool) Get() *SecureBuffer {
	return p.pool.Get().(*SecureBuffer)
}

func (p *SecureBufferPool) put(sb *SecureBuffer) {
	p.pool.Put(sb)
}

// ZeroBytes securely zeros a byte slice.
func ZeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}

// ZeroString attempts to zero a string's underlying memory.
// Note: This is best-effort and may not work due to string interning.
// Prefer SecureString for truly sensitive data.
func ZeroString(s *string) {
	if s == nil || len(*s) == 0 {
		return
	}

	// Use unsafe.StringData to get a direct pointer to the string's backing array
	// This is the safe way to access string internals in Go 1.20+
	ptr := unsafe.StringData(*s)
	if ptr == nil {
		return
	}

	b := unsafe.Slice(ptr, len(*s))
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(s)
}

// credentialBufferPool is a global pool for credential operations.
var credentialBufferPool = NewSecureBufferPool(4096)

// GetCredentialBuffer gets a buffer suitable for credential operations.
func GetCredentialBuffer() *SecureBuffer {
	return credentialBufferPool.Get()
}
