package loading

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestLoaderReadFactoryConfigSourceUsesInjectedFileSystem(t *testing.T) {
	t.Parallel()

	var statPath string
	var loadedPath string
	fileSystem := loadingFileSystemStub{
		stat: func(path string) (fs.FileInfo, error) {
			statPath = path
			return loadingFileInfo{directory: true}, nil
		},
		readFile: func(string) ([]byte, error) { return nil, fs.ErrInvalid },
	}
	wantSource := filepath.Join("factory-dir", "factory.yaml")
	loader := &Loader{
		fileSystem: fileSystem,
		loadAuthoredSource: func(path string) (factorydefinitions.AuthoredFactorySource, error) {
			if path != "factory-dir" {
				t.Fatalf("loaded path = %q, want factory-dir", path)
			}
			loadedPath = wantSource
			return factorydefinitions.AuthoredFactorySource{
				Path:   wantSource,
				Format: factorydefinitions.AuthoredFactoryFormatYAML,
				Data:   []byte(`{"name":"injected"}`),
			}, nil
		},
	}

	source, factoryDir, split, err := loader.readFactoryConfigSource("factory-dir")
	if err != nil {
		t.Fatalf("read Factory source: %v", err)
	}
	if statPath != "factory-dir" {
		t.Fatalf("stat path = %q, want factory-dir", statPath)
	}
	if loadedPath != wantSource || source.Path != wantSource {
		t.Fatalf("loaded/source paths = %q/%q, want %q", loadedPath, source.Path, wantSource)
	}
	if source.Format != factorydefinitions.AuthoredFactoryFormatYAML ||
		factoryDir != "factory-dir" ||
		!split ||
		string(source.Data) != `{"name":"injected"}` {
		t.Fatalf("source result = (%+v, %q, %v), want injected YAML directory source", source, factoryDir, split)
	}
}

func TestLoaderFailsClosedWithoutLoadingFileSystem(t *testing.T) {
	t.Parallel()

	_, _, _, err := (&Loader{}).readFactoryConfigSource("factory.json")
	if err == nil || !strings.Contains(err.Error(), "loading filesystem is required") {
		t.Fatalf("error = %v, want missing loading filesystem", err)
	}
}

func TestSourceContextErrorNamesAuthoredFormat(t *testing.T) {
	t.Parallel()

	want := fs.ErrInvalid
	for _, test := range []struct {
		source     factorydefinitions.AuthoredFactorySource
		wantFormat string
	}{
		{
			source: factorydefinitions.AuthoredFactorySource{
				Path: "factory.json", Format: factorydefinitions.AuthoredFactoryFormatJSON,
			},
			wantFormat: "(JSON)",
		},
		{
			source: factorydefinitions.AuthoredFactorySource{
				Path: "factory.yaml", Format: factorydefinitions.AuthoredFactoryFormatYAML,
			},
			wantFormat: "(YAML)",
		},
		{source: factorydefinitions.AuthoredFactorySource{Path: "factory.toml"}},
	} {
		test := test
		t.Run(test.source.Path, func(t *testing.T) {
			t.Parallel()
			err := sourceContextError(test.source, "parse factory config", want)
			if !errors.Is(err, want) {
				t.Fatalf("error = %v, want wrapped %v", err, want)
			}
			if !strings.Contains(err.Error(), test.source.Path) ||
				(test.wantFormat != "" && !strings.Contains(err.Error(), test.wantFormat)) {
				t.Fatalf("error = %q, want path and format %q", err, test.wantFormat)
			}
		})
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
