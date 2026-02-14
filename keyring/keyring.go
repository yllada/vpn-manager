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

// Storage backend state
var (
	useLocalStorage bool
	localStoreMu    sync.RWMutex
	localStore      map[string]string
	localStoreFile  string
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

	// Generate encryption key from machine-specific data
	hostname, _ := os.Hostname()
	machineID := getMachineID()
	keyData := fmt.Sprintf("vpn-manager-%s-%s-%d", hostname, machineID, os.Getuid())
	hash := sha256.Sum256([]byte(keyData))
	encryptionKey = hash[:]

	// Load existing credentials
	localStore = make(map[string]string)
	loadLocalStore()
}

func getMachineID() string {
	// Try to read machine-id
	data, err := os.ReadFile("/etc/machine-id")
	if err == nil {
		return strings.TrimSpace(string(data))
	}
	// Fallback
	return "default-machine-id"
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
