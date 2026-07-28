package authoredlayout

import (
	"io/fs"
	"os"
)

type localTestFileSystem struct{}

func (localTestFileSystem) ReadFile(path string) ([]byte, error)  { return os.ReadFile(path) }
func (localTestFileSystem) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }
func (localTestFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (localTestFileSystem) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (localTestFileSystem) RemoveAll(path string) error                { return os.RemoveAll(path) }
func (localTestFileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	return os.WriteFile(path, data, mode)
}

type testInboxEnsurer struct{ fileSystem localTestFileSystem }

func (e testInboxEnsurer) EnsureInputInboxGitkeep(targetDir, relativePath string) error {
	return e.fileSystem.WriteFile(targetDir+string(os.PathSeparator)+relativePath, nil, 0o644)
}
