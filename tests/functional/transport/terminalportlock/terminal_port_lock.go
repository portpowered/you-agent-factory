package terminalportlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const lockFileName = "you-functional-terminal-loopback-65535.lock"

// Acquire reserves the shared terminal loopback endpoint for functional tests
// that must hold 127.0.0.1:65535 while an application attempts its final
// automatic bind. The file is intentionally stable in the OS temp directory:
// the OS lock, rather than file creation or deletion, provides cross-process
// ownership and is released when the returned cleanup closes the descriptor.
func Acquire() (func() error, error) {
	path := filepath.Join(os.TempDir(), lockFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open terminal loopback test lock: %w", err)
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock terminal loopback test file: %w", err)
	}

	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() {
			releaseErr = errors.Join(unlockFile(file), file.Close())
		})
		return releaseErr
	}, nil
}
