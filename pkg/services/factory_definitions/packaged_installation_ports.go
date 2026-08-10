package factorydefinitions

import (
	"io/fs"
)

// PackagedInstallationFileSystem is the exact filesystem effect used to
// inspect and materialize a packaged Factory installation after the ownership
// reservation capability has selected the staging resource.
type PackagedInstallationFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	ReadDir(string) ([]fs.DirEntry, error)
	MkdirAll(string, fs.FileMode) error
	RemoveAll(string) error
	ReadFile(string) ([]byte, error)
	WriteFile(string, []byte, fs.FileMode) error
	Rename(string, string) error
}

// PackagedInstallationDirectoryCreator is the exact exclusive-directory
// reservation effect used by packaged installation ownership. It is kept
// separate from the generic filesystem adapter because Mkdir is a
// synchronization boundary, not a general filesystem read/write capability.
type PackagedInstallationDirectoryCreator func(string, fs.FileMode) error
