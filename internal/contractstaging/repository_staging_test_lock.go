package contractstaging

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var repositoryStagingTestMu sync.Mutex

// LockRepositoryStagingForTest serializes integration tests that mutate or
// assume exclusive access to the checked-in contract staging tree. Go runs
// different packages in separate processes, so the OS lock is required in
// addition to the process-local mutex.
func LockRepositoryStagingForTest() func() {
	repositoryStagingTestMu.Lock()

	root, err := repositoryRootFromWorkingDirectory()
	if err != nil {
		repositoryStagingTestMu.Unlock()
		panic(err)
	}
	identity := sha256.Sum256([]byte(root))
	lockPath := filepath.Join(os.TempDir(), fmt.Sprintf("contractstaging-%x.lock", identity[:8]))
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		repositoryStagingTestMu.Unlock()
		panic(fmt.Errorf("open repository staging test lock: %w", err))
	}
	if err := lockRepositoryStagingFile(lockFile); err != nil {
		_ = lockFile.Close()
		repositoryStagingTestMu.Unlock()
		panic(fmt.Errorf("lock repository staging test file: %w", err))
	}

	return func() {
		unlockErr := unlockRepositoryStagingFile(lockFile)
		closeErr := lockFile.Close()
		repositoryStagingTestMu.Unlock()
		if unlockErr != nil {
			panic(fmt.Errorf("unlock repository staging test file: %w", unlockErr))
		}
		if closeErr != nil {
			panic(fmt.Errorf("close repository staging test lock: %w", closeErr))
		}
	}
}

func repositoryRootFromWorkingDirectory() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, ".git")); err == nil {
			return filepath.Clean(directory), nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect repository marker: %w", err)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("find repository root from %s", directory)
		}
		directory = parent
	}
}
