// This file implements TOCTOU-safe staging of client-supplied VPN configs.
//
// SECURITY (C1): the daemon runs as root and hands config files to openvpn/wg-quick.
// A same-uid attacker who owns the config file can, after the daemon validates it,
// overwrite its contents in place (or swap the path) before the executing process
// reads it — smuggling a code-executing directive past the scan. Merely resolving
// the path, or handing the child /proc/self/fd/N, does NOT stop this: /proc/self/fd
// re-opens the inode and reads whatever is on disk at read time.
//
// The only robust defense is to copy the validated bytes into a root-only location
// (a directory the attacker cannot write to) and execute THAT copy. readValidatedConfig
// performs the validation half; each protocol writes the staged copy where its tool
// expects it.
package vpn

import (
	"bytes"
	"fmt"
	"io"

	"github.com/yllada/vpn-manager/daemon/privileged/validate"
)

// readValidatedConfig opens a client-supplied config path TOCTOU-safely (via
// validate.OpenConfig: O_NOFOLLOW, non-blocking, regular-file only), reads up to
// maxBytes, and runs scan on those exact bytes. It returns the validated bytes so
// the caller can write them to a root-only file and execute that copy — never the
// client path — guaranteeing the bytes validated are the bytes executed.
func readValidatedConfig(clientPath string, maxBytes int64, scan func(io.Reader) error) ([]byte, error) {
	f, err := validate.OpenConfig(clientPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("config exceeds %d bytes", maxBytes)
	}
	if err := scan(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return data, nil
}
