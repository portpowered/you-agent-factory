package factorysessions

import "io/fs"

// HomeDirectoryResolver resolves the process user's home directory at the
// external filesystem edge selected by Wire.
type HomeDirectoryResolver func() (string, error)

// DirectoryInspection is the exact host-filesystem capability used to inspect
// Factory Session target directories after session policy selects their paths.
type DirectoryInspection interface {
	Stat(string) (fs.FileInfo, error)
	ReadDir(string) ([]fs.DirEntry, error)
}
