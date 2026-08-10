package factorydefinitions

import (
	"io/fs"
)

// PackagedInstallationFileSystem is the exact filesystem effect used to
// inspect and atomically reserve a packaged Factory installation target before
// persistence policy is applied.
type PackagedInstallationFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	ReadDir(string) ([]fs.DirEntry, error)
	Mkdir(string, fs.FileMode) error
	MkdirAll(string, fs.FileMode) error
	RemoveAll(string) error
	ReadFile(string) ([]byte, error)
	WriteFile(string, []byte, fs.FileMode) error
	Rename(string, string) error
}
