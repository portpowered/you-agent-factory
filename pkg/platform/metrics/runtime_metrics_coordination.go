package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	runtimeMetricsRootLockName = ".runtime-metrics-retention.lock"
	runtimeMetricsClaimSuffix  = ".active"
	runtimeMetricsLockRetry    = 10 * time.Millisecond
)

// ErrRuntimeMetricsRootBusy means another process is currently sweeping the
// same metrics root. A caller can safely retry later.
var ErrRuntimeMetricsRootBusy = errors.New("runtime metrics root is busy")

// ErrRuntimeMetricsArtifactBusy means a live writer currently owns the
// artifact claim. Retention must preserve the artifact and try again later.
var ErrRuntimeMetricsArtifactBusy = errors.New("runtime metrics artifact is active")

// RuntimeMetricsCoordination owns the cross-process locks used by metrics
// startup and root retention. Root locks serialize reservation with sweeps;
// artifact claims protect one writer path for its entire sink lifetime.
type RuntimeMetricsCoordination interface {
	LockRoot(context.Context, string) (io.Closer, error)
	TryLockRoot(string) (io.Closer, error)
	Claim(string) (io.Closer, error)
	TryClaim(string) (io.Closer, error)
}

type runtimeMetricsCoordination struct{}

// NewRuntimeMetricsCoordination constructs the host filesystem coordination
// used by runtime metrics. The lock files are deliberately stable: the OS
// lock, rather than file creation or deletion, provides liveness across
// processes and releases automatically after a crash.
func NewRuntimeMetricsCoordination() (RuntimeMetricsCoordination, error) {
	return runtimeMetricsCoordination{}, nil
}

func selectRuntimeMetricsCoordination(
	provided []RuntimeMetricsCoordination,
) (RuntimeMetricsCoordination, error) {
	if len(provided) > 1 {
		return nil, errors.New("runtime metrics coordination accepts at most one implementation")
	}
	if len(provided) == 1 {
		if provided[0] == nil {
			return nil, errors.New("runtime metrics coordination is required")
		}
		return provided[0], nil
	}
	return NewRuntimeMetricsCoordination()
}

func (runtimeMetricsCoordination) LockRoot(ctx context.Context, root string) (io.Closer, error) {
	return acquireRuntimeMetricsLock(ctx, root, rootLockPath(root), true, false)
}

func (runtimeMetricsCoordination) TryLockRoot(root string) (io.Closer, error) {
	return acquireRuntimeMetricsLock(context.Background(), root, rootLockPath(root), false, false)
}

func (runtimeMetricsCoordination) Claim(path string) (io.Closer, error) {
	return acquireRuntimeMetricsLock(context.Background(), path, runtimeMetricsClaimPath(path), true, true)
}

func (runtimeMetricsCoordination) TryClaim(path string) (io.Closer, error) {
	return acquireRuntimeMetricsLock(context.Background(), path, runtimeMetricsClaimPath(path), false, true)
}

func acquireRuntimeMetricsLock(
	ctx context.Context,
	resourcePath, lockPath string,
	wait, removeOnClose bool,
) (io.Closer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(resourcePath) == "" {
		return nil, errors.New("runtime metrics coordination path is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !removeOnClose {
		if err := os.MkdirAll(resourcePath, 0o755); err != nil {
			return nil, fmt.Errorf("create runtime metrics coordination root %q: %w", resourcePath, err)
		}
		info, err := os.Lstat(resourcePath)
		if err != nil {
			return nil, fmt.Errorf("inspect runtime metrics coordination root %q: %w", resourcePath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, fmt.Errorf("runtime metrics coordination root %q is not a directory", resourcePath)
		}
	}
	file, err := openRuntimeMetricsLockFile(lockPath)
	if err != nil {
		return nil, err
	}

	for {
		locked, lockErr := tryLockRuntimeMetricsFile(file)
		if lockErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("lock runtime metrics coordination file %q: %w", lockPath, lockErr)
		}
		if locked {
			return &runtimeMetricsLock{file: file, path: lockPath, remove: removeOnClose}, nil
		}
		if !wait {
			_ = file.Close()
			if removeOnClose {
				return nil, ErrRuntimeMetricsArtifactBusy
			}
			return nil, ErrRuntimeMetricsRootBusy
		}
		timer := time.NewTimer(runtimeMetricsLockRetry)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func openRuntimeMetricsLockFile(path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("runtime metrics coordination file %q is a symlink", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect runtime metrics coordination file %q: %w", path, err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open runtime metrics coordination file %q: %w", path, err)
	}
	return file, nil
}

type runtimeMetricsLock struct {
	file   *os.File
	path   string
	remove bool
	once   sync.Once
	err    error
}

func (lock *runtimeMetricsLock) Close() error {
	if lock == nil {
		return nil
	}
	lock.once.Do(func() {
		unlockErr := unlockRuntimeMetricsFile(lock.file)
		closeErr := lock.file.Close()
		var removeErr error
		if lock.remove {
			removeErr = os.Remove(lock.path)
			if errors.Is(removeErr, os.ErrNotExist) {
				removeErr = nil
			}
		}
		lock.err = errors.Join(unlockErr, closeErr, removeErr)
	})
	return lock.err
}

func rootLockPath(root string) string {
	return filepath.Join(filepath.Clean(root), runtimeMetricsRootLockName)
}

func runtimeMetricsClaimPath(path string) string {
	return path + runtimeMetricsClaimSuffix
}
