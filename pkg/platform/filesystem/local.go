// Package filesystem supplies policy-free local filesystem effects selected by
// Wire for domain-owned filesystem contracts.
package filesystem

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type Local struct{}

// WorkingDirectory is the exact process-directory effect supplied by this
// Platform adapter to services that resolve invocation-local paths.
type WorkingDirectory interface {
	Getwd() (string, error)
}

// ReadOpener is the exact streaming read effect used when callers must inspect
// file contents without loading an unbounded file into memory.
type ReadOpener interface {
	Open(string) (io.ReadCloser, error)
}

// PathInspector is the exact metadata lookup effect used after a caller has
// selected a path. It carries no service-owned path or existence policy.
type PathInspector interface {
	Stat(string) (fs.FileInfo, error)
}

// ReadFileInspector is the exact read-only filesystem effect used when
// services resolve authored files and distinguish existing absolute paths.
type ReadFileInspector interface {
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

// ReadFileTree is the policy-free read-only effect for inspecting, walking,
// and loading files beneath a caller-selected directory.
type ReadFileTree interface {
	Stat(string) (fs.FileInfo, error)
	WalkDir(string, fs.WalkDirFunc) error
	ReadFile(string) ([]byte, error)
}

// TemporaryFile is the exact writable handle returned by TemporaryFileSystem.
// It intentionally exposes only the operations needed to populate and close a
// caller-owned temporary file.
type TemporaryFile interface {
	Name() string
	WriteString(string) (int, error)
	Close() error
}

// TemporaryFileSystem is the policy-free effect for creating and removing a
// temporary file after the consuming service has selected its directory,
// naming pattern, contents, and lifecycle.
type TemporaryFileSystem interface {
	CreateTemp(string, string) (TemporaryFile, error)
	Remove(string) error
}

func (Local) Open(path string) (io.ReadCloser, error)    { return os.Open(path) }
func (Local) Create(path string) (io.WriteCloser, error) { return os.Create(path) }
func (Local) OpenFile(path string, flag int, perm fs.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(path, flag, perm)
}
func (Local) Getwd() (string, error)                       { return os.Getwd() }
func (Local) Stat(path string) (fs.FileInfo, error)        { return os.Stat(path) }
func (Local) Lstat(path string) (fs.FileInfo, error)       { return os.Lstat(path) }
func (Local) Readlink(path string) (string, error)         { return os.Readlink(path) }
func (Local) EvalSymlinks(path string) (string, error)     { return filepath.EvalSymlinks(path) }
func (Local) Abs(path string) (string, error)              { return filepath.Abs(path) }
func (Local) WalkDir(path string, fn fs.WalkDirFunc) error { return filepath.WalkDir(path, fn) }
func (Local) ReadFile(path string) ([]byte, error)         { return os.ReadFile(path) }
func (Local) ReadDir(path string) ([]fs.DirEntry, error)   { return os.ReadDir(path) }
func (Local) Remove(path string) error                     { return os.Remove(path) }
func (Local) RemoveAll(path string) error                  { return os.RemoveAll(path) }
func (Local) Chmod(path string, mode fs.FileMode) error    { return os.Chmod(path, mode) }
func (Local) Rename(oldPath, newPath string) error         { return os.Rename(oldPath, newPath) }
func (Local) WriteFile(path string, data []byte, mode fs.FileMode) error {
	return os.WriteFile(path, data, mode)
}
func (Local) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}
func (Local) MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}
func (Local) CreateTemp(dir, pattern string) (TemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

// DirectoryCreator is the exact directory-creation effect used after the
// owning policy has selected the target path and permissions.
type DirectoryCreator interface {
	MkdirAll(string, fs.FileMode) error
}

var _ WorkingDirectory = Local{}
var _ ReadOpener = Local{}
var _ PathInspector = Local{}
var _ ReadFileInspector = Local{}
var _ ReadFileTree = Local{}
var _ TemporaryFileSystem = Local{}
