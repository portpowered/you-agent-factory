package contracts

import (
	"io"
	"io/fs"
)

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
