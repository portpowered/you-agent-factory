package factorydefinitions

import (
	"io/fs"
)

// AuthoredLayoutReaderFileSystem is the exact filesystem role used while
// reading split Factory Definition layouts and resolving authored sources.
type AuthoredLayoutReaderFileSystem interface {
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

// AuthoredLayoutWriterFileSystem is the exact filesystem role used while
// materializing and pruning split Factory Definition layouts.
type AuthoredLayoutWriterFileSystem interface {
	AuthoredLayoutReaderFileSystem
	MkdirAll(string, fs.FileMode) error
	ReadDir(string) ([]fs.DirEntry, error)
	RemoveAll(string) error
	WriteFile(string, []byte, fs.FileMode) error
}

// InputInboxSentinelEnsurer materializes one already-selected input inbox
// sentinel path. Factory Definitions owns the path policy; the injected
// adapter owns only the filesystem mechanics.
type InputInboxSentinelEnsurer interface {
	EnsureInputInboxGitkeep(targetDir, relativePath string) error
}
