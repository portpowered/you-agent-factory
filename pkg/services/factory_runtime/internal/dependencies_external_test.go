package internal_test

import (
	"io/fs"
	"os"
)

type localRuntimeFiles struct{}

func (localRuntimeFiles) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (localRuntimeFiles) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (localRuntimeFiles) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (localRuntimeFiles) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }

func testRuntimeID() string { return "runtime-test-id" }
