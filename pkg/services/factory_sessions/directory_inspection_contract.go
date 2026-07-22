package factorysessions

import "io/fs"

// DirectoryInspection is the exact host-filesystem capability used to inspect
// Factory Session target directories after session policy selects their paths.
type DirectoryInspection interface {
	Stat(string) (fs.FileInfo, error)
	ReadDir(string) ([]fs.DirEntry, error)
}
