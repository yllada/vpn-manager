package keyring

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// setupTestEnv creates a temporary directory for test storage
// and resets the keyring package state for isolated testing.
func setupTestEnv(t *testing.T) func() {
	t.Helper()

	// Save original state
	origUseLocalStorage := useLocalStorage
	origLocalStore := localStore
	origLocalStoreFile := localStoreFile
	origSaltFile := saltFile
	origEncryptionKey := encryptionKey
	origInitialized := initialized

	// Create temp directory
	tmpDir := t.TempDir()

	// Reset state for testing
	useLocalStorage = true
	localStore = make(map[string]string)
	localStoreFile = filepath.Join(tmpDir, ".credentials")
	saltFile = filepath.Join(tmpDir, ".keyring-salt")
	encryptionKey = nil
	initialized = false

	// Initialize local storage for tests
	initLocalStorage()

	// Return cleanup function
	return func() {
		useLocalStorage = origUseLocalStorage
		localStore = origLocalStore
		localStoreFile = origLocalStoreFile
		saltFile = origSaltFile
		encryptionKey = origEncryptionKey
		initialized = origInitialized
	}
}

func TestStore_ValidInput(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	err := Store("test-profile", "test-password")
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	password, err := Get("test-profile")
	if err != nil {
		t.Fatalf("Get after Store failed: %v", err)
	}
	if password != "test-password" {
		t.Errorf("Expected 'test-password', got '%s'", password)
	}
}

func TestStore_EmptyProfileID(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	err := Store("", "test-password")
	if err == nil {
		t.Fatal("Expected error for empty profile ID")
	}
	if err.Error() != "profile ID cannot be empty" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestStore_EmptyPassword(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	err := Store("test-profile", "")
	if err == nil {
		t.Fatal("Expected error for empty password")
	}
	if err.Error() != "password cannot be empty" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestGet_NotFound(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	_, err := Get("nonexistent-profile")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got: %v", err)
	}
}

func TestGet_EmptyProfileID(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	_, err := Get("")
	if err == nil {
		t.Fatal("Expected error for empty profile ID")
	}
}

func TestDelete_ValidProfile(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	if err := Store("test-profile", "test-password"); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if !Exists("test-profile") {
		t.Fatal("Profile should exist after Store")
	}

	err := Delete("test-profile")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if Exists("test-profile") {
		t.Error("Profile should not exist after Delete")
	}
}

func TestDelete_EmptyProfileID(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	err := Delete("")
	if err == nil {
		t.Fatal("Expected error for empty profile ID")
	}
}

func TestDelete_NonexistentProfile(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	err := Delete("nonexistent-profile")
	if err != nil {
		t.Errorf("Delete of nonexistent profile should not fail: %v", err)
	}
}

func TestExists_True(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	if err := Store("test-profile", "test-password"); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	if !Exists("test-profile") {
		t.Error("Exists should return true for stored profile")
	}
}

func TestExists_False(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	if Exists("nonexistent-profile") {
		t.Error("Exists should return false for nonexistent profile")
	}
}

func TestStore_OverwriteExisting(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	if err := Store("test-profile", "password1"); err != nil {
		t.Fatalf("First Store failed: %v", err)
	}

	if err := Store("test-profile", "password2"); err != nil {
		t.Fatalf("Second Store failed: %v", err)
	}

	password, err := Get("test-profile")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if password != "password2" {
		t.Errorf("Expected 'password2', got '%s'", password)
	}
}

func TestMultipleProfiles(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	profiles := map[string]string{
		"profile1": "password1",
		"profile2": "password2",
		"profile3": "password3",
	}

	for id, pw := range profiles {
		if err := Store(id, pw); err != nil {
			t.Fatalf("Store %s failed: %v", id, err)
		}
	}

	for id, expectedPW := range profiles {
		pw, err := Get(id)
		if err != nil {
			t.Fatalf("Get %s failed: %v", id, err)
		}
		if pw != expectedPW {
			t.Errorf("Profile %s: expected '%s', got '%s'", id, expectedPW, pw)
		}
	}
}

func TestEncryptDecrypt(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	testCases := []string{
		"simple",
		"with spaces and special chars: !@#$%^&*()",
		"unicode: test123",
		"",
	}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			encrypted, err := encrypt([]byte(tc))
			if err != nil {
				t.Fatalf("encrypt failed: %v", err)
			}

			decrypted, err := decrypt(encrypted)
			if err != nil {
				t.Fatalf("decrypt failed: %v", err)
			}

			if string(decrypted) != tc {
				t.Errorf("Expected '%s', got '%s'", tc, string(decrypted))
			}
		})
	}
}

func TestDecrypt_InvalidData(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	_, err := decrypt([]byte("not base64!!!"))
	if err == nil {
		t.Error("Expected error for invalid base64")
	}

	_, err = decrypt([]byte("dGVzdA=="))
	if err == nil {
		t.Error("Expected error for invalid ciphertext")
	}
}

func TestLoadOrCreateSalt_NewSalt(t *testing.T) {
	tmpDir := t.TempDir()
	origSaltFile := saltFile
	saltFile = filepath.Join(tmpDir, ".keyring-salt")
	defer func() { saltFile = origSaltFile }()

	salt, isNew, err := loadOrCreateSalt()
	if err != nil {
		t.Fatalf("loadOrCreateSalt failed: %v", err)
	}
	if !isNew {
		t.Error("Expected isNew=true for new salt")
	}
	if len(salt) != saltSize {
		t.Errorf("Salt size: expected %d, got %d", saltSize, len(salt))
	}

	if _, err := os.Stat(saltFile); os.IsNotExist(err) {
		t.Error("Salt file was not created")
	}
}

func TestLoadOrCreateSalt_ExistingSalt(t *testing.T) {
	tmpDir := t.TempDir()
	origSaltFile := saltFile
	saltFile = filepath.Join(tmpDir, ".keyring-salt")
	defer func() { saltFile = origSaltFile }()

	salt1, _, err := loadOrCreateSalt()
	if err != nil {
		t.Fatalf("First loadOrCreateSalt failed: %v", err)
	}

	salt2, isNew, err := loadOrCreateSalt()
	if err != nil {
		t.Fatalf("Second loadOrCreateSalt failed: %v", err)
	}
	if isNew {
		t.Error("Expected isNew=false for existing salt")
	}

	if string(salt1) != string(salt2) {
		t.Error("Salts should be identical")
	}
}

func TestPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	origUseLocalStorage := useLocalStorage
	origLocalStore := localStore
	origLocalStoreFile := localStoreFile
	origSaltFile := saltFile
	origEncryptionKey := encryptionKey
	origInitialized := initialized

	defer func() {
		useLocalStorage = origUseLocalStorage
		localStore = origLocalStore
		localStoreFile = origLocalStoreFile
		saltFile = origSaltFile
		encryptionKey = origEncryptionKey
		initialized = origInitialized
	}()

	useLocalStorage = true
	localStore = make(map[string]string)
	localStoreFile = filepath.Join(tmpDir, ".credentials")
	saltFile = filepath.Join(tmpDir, ".keyring-salt")
	encryptionKey = nil
	initialized = false
	initLocalStorage()

	if err := Store("persistent-profile", "persistent-password"); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	localStore = make(map[string]string)
	encryptionKey = nil
	initialized = false
	initLocalStorage()

	password, err := Get("persistent-profile")
	if err != nil {
		t.Fatalf("Get after restart failed: %v", err)
	}
	if password != "persistent-password" {
		t.Errorf("Expected 'persistent-password', got '%s'", password)
	}
}

func TestConcurrentAccess(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				profileID := "profile"
				password := "password"
				_ = Store(profileID, password)
				_, _ = Get(profileID)
			}
		}(i)
	}
	wg.Wait()

	password, err := Get("profile")
	if err != nil {
		t.Fatalf("Final Get failed: %v", err)
	}
	if password != "password" {
		t.Errorf("Unexpected final password: %s", password)
	}
}

func TestSpecialCharactersInPassword(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	passwords := []string{
		"simple123",
		"with spaces",
		"special!@#$%^&*()_+-=[]{}|;':,./<>?",
		"newlines\nand\ttabs",
	}

	for i, pw := range passwords {
		profileID := "test-profile"
		if err := Store(profileID, pw); err != nil {
			t.Fatalf("Test %d: Store failed: %v", i, err)
		}

		retrieved, err := Get(profileID)
		if err != nil {
			t.Fatalf("Test %d: Get failed: %v", i, err)
		}

		if retrieved != pw {
			t.Errorf("Test %d: password mismatch (len original=%d, retrieved=%d)",
				i, len(pw), len(retrieved))
		}
	}
}

func TestSpecialCharactersInProfileID(t *testing.T) {
	cleanup := setupTestEnv(t)
	defer cleanup()

	profileIDs := []string{
		"simple",
		"with-dashes",
		"with_underscores",
		"with.dots",
		"MixedCase123",
		"uuid-like-a1b2c3d4-e5f6-7890-abcd-ef1234567890",
	}

	for _, id := range profileIDs {
		t.Run(id, func(t *testing.T) {
			if err := Store(id, "password"); err != nil {
				t.Fatalf("Store failed for '%s': %v", id, err)
			}

			pw, err := Get(id)
			if err != nil {
				t.Fatalf("Get failed for '%s': %v", id, err)
			}
			if pw != "password" {
				t.Errorf("Password mismatch for '%s'", id)
			}

			if err := Delete(id); err != nil {
				t.Fatalf("Delete failed for '%s': %v", id, err)
			}

			if Exists(id) {
				t.Errorf("Profile '%s' should not exist after delete", id)
			}
		})
	}
}
