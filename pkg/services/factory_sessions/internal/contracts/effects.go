package contracts

import (
	"io"
	"io/fs"
)

// Effect-port contracts are owned here so the Sessions service root can publish
// them as type aliases without adding extra root InterfaceType declarations
// (pkg-structure requires exactly one named service interface aside from
// recorded deletion-only debt for Service + ExecutionService).

type ExecutionOpeningFileSystem interface {
	Getwd() (string, error)
	Stat(string) (fs.FileInfo, error)
}

type DirectoryInspection interface {
	Stat(string) (fs.FileInfo, error)
	ReadDir(string) ([]fs.DirEntry, error)
}

type CursorPersistenceFileSystem interface {
	MkdirAll(string, fs.FileMode) error
	ReadFile(string) ([]byte, error)
	Remove(string) error
	Rename(string, string) error
}

type CursorPersistenceTemporaryFile interface {
	io.Writer
	Name() string
	Chmod(fs.FileMode) error
	Sync() error
	Close() error
}

type CursorPersistenceCreateTemporaryFile func(string, string) (CursorPersistenceTemporaryFile, error)

type RuntimePersistenceFileSystem interface {
	MkdirAll(string, fs.FileMode) error
	ReadFile(string) ([]byte, error)
	WriteFile(string, []byte, fs.FileMode) error
}

type InvocationMetricsRecorder interface {
	RecordInvocationMetric(InvocationMetric)
}
