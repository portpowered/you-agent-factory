package loading

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoaderReadFactoryConfigSourceUsesInjectedFileSystem(t *testing.T) {
	t.Parallel()

	var statPath string
	var readPath string
	fileSystem := loadingFileSystemStub{
		stat: func(path string) (fs.FileInfo, error) {
			statPath = path
			return loadingFileInfo{directory: true}, nil
		},
		readFile: func(path string) ([]byte, error) {
			readPath = path
			return []byte(`{"name":"injected"}`), nil
		},
	}
	loader := &Loader{fileSystem: fileSystem}

	data, sourcePath, factoryDir, split, err := loader.readFactoryConfigSource("factory-dir")
	if err != nil {
		t.Fatalf("read Factory source: %v", err)
	}
	if statPath != "factory-dir" {
		t.Fatalf("stat path = %q, want factory-dir", statPath)
	}
	wantSource := filepath.Join("factory-dir", "factory.json")
	if readPath != wantSource || sourcePath != wantSource {
		t.Fatalf("read/source paths = %q/%q, want %q", readPath, sourcePath, wantSource)
	}
	if factoryDir != "factory-dir" || !split || string(data) != `{"name":"injected"}` {
		t.Fatalf("source result = (%q, %q, %v), want injected directory source", data, factoryDir, split)
	}
}

func TestLoaderFailsClosedWithoutLoadingFileSystem(t *testing.T) {
	t.Parallel()

	_, _, _, _, err := (&Loader{}).readFactoryConfigSource("factory.json")
	if err == nil || !strings.Contains(err.Error(), "loading filesystem is required") {
		t.Fatalf("error = %v, want missing loading filesystem", err)
	}
}

type loadingFileSystemStub struct {
	stat     func(string) (fs.FileInfo, error)
	readFile func(string) ([]byte, error)
}

func (s loadingFileSystemStub) Stat(path string) (fs.FileInfo, error) {
	return s.stat(path)
}

func (s loadingFileSystemStub) ReadFile(path string) ([]byte, error) {
	return s.readFile(path)
}

type loadingFileInfo struct{ directory bool }

func (loadingFileInfo) Name() string       { return "factory-dir" }
func (loadingFileInfo) Size() int64        { return 0 }
func (loadingFileInfo) Mode() fs.FileMode  { return fs.ModeDir }
func (loadingFileInfo) ModTime() time.Time { return time.Time{} }
func (i loadingFileInfo) IsDir() bool      { return i.directory }
func (loadingFileInfo) Sys() any           { return nil }
