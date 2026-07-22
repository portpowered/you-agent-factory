package factorydefinitions

import "io/fs"

// NamedFactoryCatalogFileSystem is the exact filesystem effect used to inspect
// and delete persisted named Factory catalog entries.
type NamedFactoryCatalogFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	ReadDir(string) ([]fs.DirEntry, error)
	RemoveAll(string) error
}
