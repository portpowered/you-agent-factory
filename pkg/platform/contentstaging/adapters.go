// Package contentstaging supplies policy-free process effects for Work content
// staging. Wire selects these adapters; Work owns all staging policy.
package contentstaging

import (
	"crypto/rand"
	"io/fs"
	"os"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// FileSystem implements the exact filesystem effect declared by Work.
type FileSystem struct{}

func (FileSystem) MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}

func (FileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	return os.WriteFile(path, data, mode)
}

func (FileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (FileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// Random implements the exact entropy effect declared by Work.
type Random struct{}

func (Random) Read(buffer []byte) (int, error) {
	return rand.Read(buffer)
}

var (
	_ work.ContentStagingFileSystem = FileSystem{}
	_ work.ContentStagingRandom     = Random{}
)
