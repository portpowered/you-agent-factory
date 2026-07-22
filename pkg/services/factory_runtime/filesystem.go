package factory

import (
	"io/fs"
)

// RuntimeDirectoryFileSystem materializes and inspects runtime-owned input
// directories after Factory Runtime has selected their paths and permissions.
type RuntimeDirectoryFileSystem interface {
	MkdirAll(string, fs.FileMode) error
	Stat(string) (fs.FileInfo, error)
}

// InputFileSystem reads the directory tree consumed by the Factory Runtime
// input watcher.
type InputFileSystem interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

// InputDirectoryWalker traverses the runtime input tree selected by Factory
// Runtime. Wire supplies the production implementation; owner tests may inject
// a deterministic traversal.
type InputDirectoryWalker func(string, fs.WalkDirFunc) error

// WorkflowSourceFileSystem reads candidate JavaScript workflow definitions
// after the source service has selected the lookup order and paths.
type WorkflowSourceFileSystem interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

// WorkflowSourceResolveSymlinks resolves filesystem links while Factory
// Runtime validates that workflow artifact roots remain outside the project.
type WorkflowSourceResolveSymlinks func(string) (string, error)

// WorkflowHomeResolver supplies the external user-home value used to build
// global workflow lookup roots.
type WorkflowHomeResolver func() (string, error)
