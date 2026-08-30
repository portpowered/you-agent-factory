package ownershipinventory

import (
	"io"
	"io/fs"
	"os"
)

// fileSystem is the private host-filesystem boundary used by ownership
// snapshot publication. Keeping the operation set here lets package tests
// control individual filesystem failures without exposing a production hook
// or changing the command's public surface.
type fileSystem interface {
	mkdirAll(string, fs.FileMode) error
	writeFile(string, []byte, fs.FileMode) error
	chmod(string, fs.FileMode) error
	readFile(string) ([]byte, error)
	lstat(string) (fs.FileInfo, error)
	createTemp(string, string) (snapshotFile, error)
	rename(string, string) error
	remove(string) error
}

// snapshotFile is the narrow handle needed for destination-local staging and
// retained-byte recovery. It deliberately hides *os.File from the publisher.
type snapshotFile interface {
	io.Writer
	io.Closer
	Name() string
	Chmod(fs.FileMode) error
}

// osFileSystem is the default production implementation of fileSystem. It
// has no mutable state, configuration, or fault-injection behavior.
type osFileSystem struct{}

func (osFileSystem) mkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osFileSystem) writeFile(path string, payload []byte, perm fs.FileMode) error {
	return os.WriteFile(path, payload, perm)
}

func (osFileSystem) chmod(path string, perm fs.FileMode) error {
	return os.Chmod(path, perm)
}

func (osFileSystem) readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (osFileSystem) lstat(path string) (fs.FileInfo, error) {
	return os.Lstat(path)
}

func (osFileSystem) createTemp(directory, pattern string) (snapshotFile, error) {
	return os.CreateTemp(directory, pattern)
}

func (osFileSystem) rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (osFileSystem) remove(path string) error {
	return os.Remove(path)
}
