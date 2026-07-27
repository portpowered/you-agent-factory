package service

import (
	"io/fs"
	"os"
)

type localInputFiles struct{}

func (localInputFiles) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (localInputFiles) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (localInputFiles) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }
