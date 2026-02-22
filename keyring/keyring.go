// Package keyring provides secure credential storage.
// It uses the system keyring when available, falling back to
// encrypted local file storage when not.
package keyring

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/argon2"
)

const (
	// serviceName is the identifier used in the system keyring.
	serviceName = "vpn-manager"
)

// Common errors returned by keyring operations.
var (
	ErrNotFound    = errors.New("credential not found")
	ErrAccess      = errors.New("keyring access denied")
	ErrUnavailable = errors.New("keyring service unavailable")
)

// Argon2id parameters (OWASP recommended)
const (
	argon2Time    = 1         // Number of iterations
	argon2Memory  = 64 * 1024 // 64 MB memory cost
	argon2Threads = 4         // Parallelism factor
	argon2KeyLen  = 32        // Output key length (256 bits)
	saltSize      = 32        // Salt size in bytes
)

// Storage backend state
var (
	useLocalStorage bool
	localStoreMu    sync.RWMutex
	localStore      map[string]string
	localStoreFile  string
	saltFile        string
	encryptionKey   []byte
	initialized     bool
)

// init initializes the storage backend
func init() {
	initStorage()
}

func initStorage() {
	if initialized {
		return
	}

	// Try system keyring first
	testKey := "vpn-manager-test-init"
	err := keyring.Set(serviceName, testKey, "test")
	if err == nil {
		keyring.Delete(serviceName, testKey)
		useLocalStorage = false
	} else {
		useLocalStorage = true
		initLocalStorage()
	}
	initialized = true
}

func initLocalStorage() {
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "vpn-manager")
	os.MkdirAll(configDir, 0700)
	localStoreFile = filepath.Join(configDir, ".credentials")
	saltFile = filepath.Join(configDir, ".keyring-salt")

	// Load or create cryptographically secure salt
	salt, isNewSalt, err := loadOrCreateSalt()
	if err != nil {
		// Critical error - cannot proceed without salt
		panic("keyring: failed to initialize salt: " + err.Error())
	}

	// Derive encryption key using Argon2id
	// Using a fixed password combined with salt ensures key uniqueness per installation
	password := []byte("vpn-manager-local-storage")
	encryptionKey = argon2.IDKey(password, salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	// Load existing credentials
	localStore = make(map[string]string)

	// If this is a new salt and old credentials exist, attempt migration
	if isNewSalt {
		migrateOldCredentials()
	}

	loadLocalStore()
}

// loadOrCreateSalt loads an existing salt from file or creates a new one.
// Returns the salt, whether it's newly created, and any error.
func loadOrCreateSalt() ([]byte, bool, error) {
	// Try to load existing salt
	if data, err := os.ReadFile(saltFile); err == nil {
		if len(data) == saltSize {
			return data, false, nil
		}
		// Invalid salt size, regenerate
	}

	// Generate new cryptographically secure salt
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, false, err
	}

	// Persist salt with secure permissions (owner read/write only)
	if err := os.WriteFile(saltFile, salt, 0600); err != nil {
		return nil, false, err
	}

	return salt, true, nil
}

// migrateOldCredentials attempts to migrate credentials from old encryption scheme.
// This provides backward compatibility during the transition period.
func migrateOldCredentials() {
	// Check if old credentials file exists
	data, err := os.ReadFile(localStoreFile)
	if err != nil {
		return // No old credentials to migrate
	}

	// Try to decrypt with old key derivation (SHA256 of predictable data)
	oldKey := deriveOldKey()
	if oldKey == nil {
		return
	}

	decrypted, err := decryptWithKey(data, oldKey)
	if err != nil {
		return // Old credentials not decryptable or already migrated
	}

	var oldStore map[string]string
	if err := json.Unmarshal(decrypted, &oldStore); err != nil {
		return
	}

	// Backup old file
	backupFile := localStoreFile + ".bak"
	os.Rename(localStoreFile, backupFile)

	// Re-encrypt with new key
	localStore = oldStore
	if err := saveLocalStore(); err != nil {
		// Restore backup on failure
		os.Rename(backupFile, localStoreFile)
		return
	}

	// Remove backup after successful migration
	os.Remove(backupFile)
}

// deriveOldKey recreates the old insecure key for migration purposes.
// This uses the same vulnerable derivation as the previous implementation.
func deriveOldKey() []byte {
	hostname, _ := os.Hostname()
	machineID := getOldMachineID()
	keyData := fmt.Sprintf("vpn-manager-%s-%s-%d", hostname, machineID, os.Getuid())
	hash := sha256.Sum256([]byte(keyData))
	return hash[:]
}

// getOldMachineID replicates the old machine ID logic for migration.
func getOldMachineID() string {
	data, err := os.ReadFile("/etc/machine-id")
	if err == nil {
		return strings.TrimSpace(string(data))
	}
	return "default-machine-id"
}

// decryptWithKey decrypts data using a specific key.
func decryptWithKey(data []byte, key []byte) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func loadLocalStore() {
	data, err := os.ReadFile(localStoreFile)
	if err != nil {
		return
	}

	decrypted, err := decrypt(data)
	if err != nil {
		return
	}

	json.Unmarshal(decrypted, &localStore)
}

func saveLocalStore() error {
	localStoreMu.RLock()
	data, err := json.Marshal(localStore)
	localStoreMu.RUnlock()
	if err != nil {
		return err
	}

	encrypted, err := encrypt(data)
	if err != nil {
		return err
	}

	return os.WriteFile(localStoreFile, encrypted, 0600)
}

func encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return []byte(base64.StdEncoding.EncodeToString(ciphertext)), nil
}

func decrypt(data []byte) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// Store saves a password for a VPN profile.
func Store(profileID string, password string) error {
	if profileID == "" {
		return errors.New("profile ID cannot be empty")
	}
	if password == "" {
		return errors.New("password cannot be empty")
	}

	if useLocalStorage {
		localStoreMu.Lock()
		localStore[profileID] = password
		localStoreMu.Unlock()
		return saveLocalStore()
	}

	if err := keyring.Set(serviceName, profileID, password); err != nil {
		// Fallback to local storage
		useLocalStorage = true
		initLocalStorage()
		localStoreMu.Lock()
		localStore[profileID] = password
		localStoreMu.Unlock()
		return saveLocalStore()
	}
	return nil
}

// Get retrieves a password for a VPN profile.
func Get(profileID string) (string, error) {
	if profileID == "" {
		return "", errors.New("profile ID cannot be empty")
	}

	if useLocalStorage {
		localStoreMu.RLock()
		password, exists := localStore[profileID]
		localStoreMu.RUnlock()
		if !exists {
			return "", ErrNotFound
		}
		return password, nil
	}

	password, err := keyring.Get(serviceName, profileID)
	if err != nil {
		if err == keyring.ErrNotFound {
			return "", ErrNotFound
		}
		// Try local storage as fallback
		localStoreMu.RLock()
		password, exists := localStore[profileID]
		localStoreMu.RUnlock()
		if exists {
			return password, nil
		}
		return "", ErrNotFound
	}
	return password, nil
}

// Delete removes a password for a VPN profile.
func Delete(profileID string) error {
	if profileID == "" {
		return errors.New("profile ID cannot be empty")
	}

	if useLocalStorage {
		localStoreMu.Lock()
		delete(localStore, profileID)
		localStoreMu.Unlock()
		return saveLocalStore()
	}

	keyring.Delete(serviceName, profileID)

	// Also remove from local storage if present
	localStoreMu.Lock()
	delete(localStore, profileID)
	localStoreMu.Unlock()
	saveLocalStore()

	return nil
}

// Exists checks if a credential exists for a VPN profile.
func Exists(profileID string) bool {
	_, err := Get(profileID)
	return err == nil
}
