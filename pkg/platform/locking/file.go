// Package locking supplies policy-free, cross-process filesystem ownership.
//
// Callers choose the resource identity and lifecycle. The platform adapter
// only provides cancellation-aware advisory locking on a stable marker file.
package locking

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const lockRetryInterval = 10 * time.Millisecond

// ErrBusy means another owner holds the requested coordination marker.
var ErrBusy = errors.New("filesystem coordination is busy")

// Service is the policy-free cross-process locking effect used by services
// that need recoverable ownership of a filesystem transaction.
type Service interface {
	Lock(context.Context, string) (io.Closer, error)
	TryLock(context.Context, string) (io.Closer, error)
	TryLockTarget(context.Context, string, string) (io.Closer, error)
}

// File is the narrow host handle required by the OS-specific lock adapter.
// The platform implementation never exposes an operating-system-specific
// descriptor type to its callers.
type File interface {
	Fd() uintptr
	Stat() (fs.FileInfo, error)
	Close() error
}

// FileSystem is the exact filesystem effect selected by Wire for ownership
// marker creation and inspection.
type FileSystem interface {
	MkdirAll(string, fs.FileMode) error
	Lstat(string) (fs.FileInfo, error)
	OpenFile(string, int, fs.FileMode) (File, error)
}

// LocalFileSystem is the Wire-selected host implementation of FileSystem.
type LocalFileSystem struct{}

func (LocalFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}

func (LocalFileSystem) Lstat(path string) (fs.FileInfo, error) {
	return os.Lstat(path)
}

func (LocalFileSystem) OpenFile(path string, flags int, mode fs.FileMode) (File, error) {
	return os.OpenFile(path, flags, mode)
}

type localService struct {
	filesystem FileSystem
}

// New constructs the host filesystem locking effect. The marker file is kept
// after release; the operating system lock, which is released automatically
// when a process exits, provides ownership and crash recovery.
func New(filesystem FileSystem) (Service, error) {
	if filesystem == nil {
		return nil, errors.New("filesystem locking effect is required")
	}
	return localService{filesystem: filesystem}, nil
}

func (service localService) Lock(ctx context.Context, path string) (io.Closer, error) {
	return service.acquire(ctx, path, true)
}

// TryLock refuses an occupied marker without waiting for its owner to exit.
func (service localService) TryLock(ctx context.Context, path string) (io.Closer, error) {
	return service.acquire(ctx, path, false)
}

// TryLockTarget refuses aliases that cannot share a stable pathname marker.
// The caller selects both the protected target and its coordination marker.
func (service localService) TryLockTarget(ctx context.Context, target, marker string) (io.Closer, error) {
	before, err := service.inspectTarget(target)
	if err != nil {
		return nil, err
	}
	lease, err := service.TryLock(ctx, marker)
	if err != nil {
		return nil, err
	}
	after, err := service.inspectTarget(target)
	if err != nil {
		return nil, errors.Join(err, lease.Close())
	}
	if !sameTarget(before, after) {
		return nil, errors.Join(fmt.Errorf("ownership target %q changed during acquisition", target), lease.Close())
	}
	return &targetLock{Closer: lease, service: service, target: target, original: after}, nil
}

// targetLock retains the identity observed while acquiring the marker so a
// caller can reject a changed input after reading it, before publishing.
type targetLock struct {
	io.Closer
	service  localService
	target   string
	original fs.FileInfo
}

func (lock *targetLock) Validate() error {
	current, err := lock.service.inspectTarget(lock.target)
	if err != nil {
		return err
	}
	if !sameTarget(lock.original, current) {
		return fmt.Errorf("ownership target %q changed after acquisition", lock.target)
	}
	return nil
}

func sameTarget(before, after fs.FileInfo) bool {
	if before == nil || after == nil {
		return before == nil && after == nil
	}
	return os.SameFile(before, after) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}

func (service localService) inspectTarget(path string) (fs.FileInfo, error) {
	info, err := service.filesystem.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect ownership target %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("ownership target %q must be a regular file without aliases", path)
	}
	file, err := service.filesystem.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open ownership target %q: %w", path, err)
	}
	openedInfo, statErr := file.Stat()
	count, inspectErr := fileLinkCount(file)
	closeErr := file.Close()
	if err := errors.Join(statErr, inspectErr, closeErr); err != nil {
		return nil, fmt.Errorf("inspect ownership target links %q: %w", path, err)
	}
	if !sameTarget(info, openedInfo) {
		return nil, fmt.Errorf("ownership target %q changed during inspection", path)
	}
	if count != 1 {
		return nil, fmt.Errorf("ownership target %q has multiple hard links; use a separate copy", path)
	}
	return openedInfo, nil
}

func (service localService) acquire(ctx context.Context, path string, wait bool) (io.Closer, error) {
	ctx, err := validateContext(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := service.prepareLockDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	file, err := service.openLockFile(path)
	if err != nil {
		return nil, err
	}
	for {
		locked, lockErr := tryLockFile(file)
		if lockErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("lock filesystem coordination file %q: %w", path, lockErr)
		}
		if locked {
			return &fileLock{file: file}, nil
		}
		if !wait {
			return nil, errors.Join(ErrBusy, file.Close())
		}
		timer := time.NewTimer(lockRetryInterval)
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

func validateContext(ctx context.Context, path string) (context.Context, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("filesystem coordination path is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ctx, nil
}

func (service localService) prepareLockDirectory(path string) error {
	if err := service.filesystem.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create filesystem coordination directory %q: %w", path, err)
	}
	info, err := service.filesystem.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect filesystem coordination directory %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("filesystem coordination directory %q is not a directory", path)
	}
	return nil
}

func (service localService) openLockFile(path string) (File, error) {
	if info, err := service.filesystem.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("filesystem coordination file %q is a symlink", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect filesystem coordination file %q: %w", path, err)
	}
	file, err := service.filesystem.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open filesystem coordination file %q: %w", path, err)
	}
	return file, nil
}

type fileLock struct {
	file File
	once sync.Once
	err  error
}

// Close releases the OS lock before closing the descriptor. The marker file
// remains in place so a closing owner cannot unlink a replacement owner's
// pathname between unlock and the next acquisition.
func (lock *fileLock) Close() error {
	if lock == nil {
		return nil
	}
	lock.once.Do(func() {
		unlockErr := unlockFile(lock.file)
		closeErr := lock.file.Close()
		lock.err = errors.Join(unlockErr, closeErr)
	})
	return lock.err
}
