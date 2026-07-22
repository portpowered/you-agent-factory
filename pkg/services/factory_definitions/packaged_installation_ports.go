package factorydefinitions

import (
	"io/fs"
)

// PackagedInstallationFileSystem is the exact filesystem effect used to
// inspect a packaged Factory installation target before persistence policy is
// applied.
type PackagedInstallationFileSystem interface {
	Stat(string) (fs.FileInfo, error)
}
