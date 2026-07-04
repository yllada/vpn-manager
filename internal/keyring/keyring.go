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
	"log"
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
	saltFile        string // legacy: read only to migrate credentials from the previous scheme
	keyFile         string
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
		_ = keyring.Delete(serviceName, testKey)
		useLocalStorage = false
	} else {
		log.Printf("keyring: system keyring unavailable (%v); falling back to encrypted local file storage. "+
			"This is weaker than a system keyring — install gnome-keyring, kwallet, or pass for stronger protection.", err)
		useLocalStorage = true
		initLocalStorage()
	}
	initialized = true
}

func initLocalStorage() {
	if localStoreFile == "" {
		homeDir, _ := os.UserHomeDir()
		configDir := filepath.Join(homeDir, ".config", "vpn-manager")
		_ = os.MkdirAll(configDir, 0700)
		localStoreFile = filepath.Join(configDir, ".credentials")
		saltFile = filepath.Join(configDir, ".keyring-salt")
		keyFile = filepath.Join(configDir, ".keyring-key")
	}

	localStore = make(map[string]string)

	// Load or create a random 256-bit master key. Unlike the previous scheme,
	// the key is never derived from a hardcoded password: it is generated with
	// crypto/rand on first use and stored with 0600 permissions. This is
	// defense-in-depth against a leaked credentials file (backups, home-dir
	// sync, discarded disks); it is not, and cannot be, protection against a
	// live attacker running under the same UID — for that, a system keyring is
	// required.
	key, isNewKey, err := loadOrCreateKey()
	if err != nil {
		// Critical error - log and continue with an empty store.
		// Credential operations will fail gracefully.
		log.Printf("keyring: failed to initialize local encryption key: %v - credential storage disabled", err)
		return
	}
	encryptionKey = key

	// On first run with the new key, migrate credentials written by older
	// encryption schemes so upgrading users don't lose saved passwords.
	if isNewKey {
		migrateLegacyCredentials()
	}

	loadLocalStore()
}

// loadOrCreateKey loads the local master key from disk or creates a new random
// one. Returns the key, whether it was newly created, and any error.
func loadOrCreateKey() ([]byte, bool, error) {
	// Fast path: a valid key already exists.
	if data, err := os.ReadFile(keyFile); err == nil {
		if len(data) == argon2KeyLen {
			return data, false, nil
		}
		// A key file exists but is the wrong size (corrupt / partial write).
		// Regenerating means any .credentials it encrypted can no longer be
		// decrypted, so make that observable rather than losing data silently.
		log.Printf("keyring: local key file %s is corrupt (%d bytes, expected %d); regenerating — "+
			"previously saved credentials will need to be re-entered", keyFile, len(data), argon2KeyLen)
		_ = os.Remove(keyFile)
	}

	key := make([]byte, argon2KeyLen)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, false, err
	}

	// O_EXCL guards against a concurrent process creating the key at the same time.
	f, err := os.OpenFile(keyFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			// Lost the race: prefer the key already written by the other process.
			if data, rerr := os.ReadFile(keyFile); rerr == nil && len(data) == argon2KeyLen {
				return data, false, nil
			}
		}
		return nil, false, err
	}
	defer f.Close()

	if _, err := f.Write(key); err != nil {
		_ = os.Remove(keyFile)
		return nil, false, err
	}

	return key, true, nil
}

// legacyLocalPassword is the fixed password used by the previous key-derivation
// scheme. It is retained ONLY to decrypt and migrate credentials written by that
// scheme; it is never used to encrypt new data.
const legacyLocalPassword = "vpn-manager-local-storage"

// migrateLegacyCredentials re-encrypts credentials that were stored with an
// older encryption scheme using the current random master key. It runs once,
// when a fresh master key is created (typically on upgrade).
func migrateLegacyCredentials() {
	data, err := os.ReadFile(localStoreFile)
	if err != nil {
		return // No existing credentials to migrate.
	}

	for _, legacyKey := range legacyKeys() {
		if legacyKey == nil {
			continue
		}

		decrypted, err := decryptWithKey(data, legacyKey)
		if err != nil {
			continue // Not this scheme (or already migrated); try the next.
		}

		var oldStore map[string]string
		if err := json.Unmarshal(decrypted, &oldStore); err != nil {
			continue
		}

		// Back up, then re-encrypt with the current master key.
		backupFile := localStoreFile + ".bak"
		_ = os.Rename(localStoreFile, backupFile)

		localStore = oldStore
		if err := saveLocalStore(); err != nil {
			// Restore backup on failure.
			_ = os.Rename(backupFile, localStoreFile)
			return
		}

		_ = os.Remove(backupFile)
		return
	}

	// A credentials file exists but no known scheme could decrypt it — this
	// happens if the master key was lost or corrupted while the file was written
	// by the current scheme. The data is unrecoverable; surface it so the user
	// knows to re-enter passwords instead of failing silently.
	if len(data) > 0 {
		log.Printf("keyring: existing credentials file %s could not be decrypted with any known key; "+
			"saved passwords will need to be re-entered", localStoreFile)
	}
}

// legacyKeys returns the decryption keys for previous storage schemes, newest
// first, for migration purposes only.
func legacyKeys() [][]byte {
	var keys [][]byte

	// Previous scheme: Argon2id(fixed password, per-install salt).
	if salt, err := os.ReadFile(saltFile); err == nil && len(salt) == saltSize {
		keys = append(keys, argon2.IDKey([]byte(legacyLocalPassword), salt,
			argon2Time, argon2Memory, argon2Threads, argon2KeyLen))
	}

	// Oldest scheme: SHA256(hostname + machine-id + uid).
	keys = append(keys, deriveOldKey())

	return keys
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

	_ = json.Unmarshal(decrypted, &localStore)
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

	_ = keyring.Delete(serviceName, profileID)

	// Also remove from local storage if present
	localStoreMu.Lock()
	delete(localStore, profileID)
	localStoreMu.Unlock()
	_ = saveLocalStore()

	return nil
}

// Exists checks if a credential exists for a VPN profile.
func Exists(profileID string) bool {
	_, err := Get(profileID)
	return err == nil
}
