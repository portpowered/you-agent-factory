package catalog

import "io/fs"

// PathFileSystem is the narrow filesystem capability used by the catalog path
// resolver. It remains inside the Definitions implementation tree.
type PathFileSystem interface {
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
}

// PathResolver is the catalog-owned named-path policy surface.
type PathResolver interface {
	ResolveExistingDir(string, string) (string, error)
	RequireDefinitionDir(string) error
	ResolveCurrentDir(string) (string, error)
	ReadCurrentPointer(string) (string, error)
	WriteCurrentPointer(string, string) error
}

// CatalogFileSystem is the catalog read/delete filesystem capability.
type CatalogFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	ReadDir(string) ([]fs.DirEntry, error)
	RemoveAll(string) error
}

// PersistenceFileSystem is the filesystem capability for atomic catalog-owned
// layout persistence.
type PersistenceFileSystem interface {
	MkdirTemp(string, string) (string, error)
	RemoveAll(string) error
	Rename(string, string) error
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
}

// CurrentFactoryPointerWriter is the narrow pointer write operation used by
// catalog persistence.
type CurrentFactoryPointerWriter func(string, string) error

// DefinitionDirectoryRequirer validates one selected Factory definition dir.
type DefinitionDirectoryRequirer func(string) error

// DirectoryReplacementStore provides atomic directory replacement and restore
// mechanics for catalog persistence rollback.
type DirectoryReplacementStore interface {
	Commit(parentDir, targetDir, stagingDir string) (backupDir string, err error)
	Restore(targetDir, backupDir string)
}
