// Package atomicfile provides crash-safe file writes: data is written to a
// temporary file in the same directory, flushed to disk, and then renamed over
// the destination. A crash or interruption leaves either the old file or the
// fully-written new one, never a truncated mix.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write atomically writes data to path with the given permissions.
//
// The temp file is created in the same directory as path so the final rename is
// a same-filesystem operation (atomic). On any failure before the rename the
// temp file is removed. The file is fsync'd before the rename, and the parent
// directory is fsync'd afterward (best effort) so the rename itself survives a
// power loss.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	// Remove the temp file on every error path; after a successful rename the
	// name no longer exists and this is a harmless no-op.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	// Persist the directory entry so the rename survives a crash. Best effort:
	// the data is already durable and renamed at this point.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}

	return nil
}
