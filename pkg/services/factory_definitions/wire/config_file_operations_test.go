package wire

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestFactoryConfigRootResolverAcceptsSelectedFileOrDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := NewFactoryConfigRootResolver(osFS{})(path)
	if err != nil || root != dir {
		t.Fatalf("result = (%q, %v), want (%q, nil)", root, err, dir)
	}
	root, err = NewFactoryConfigRootResolver(osFS{})(dir)
	if err != nil || root != dir {
		t.Fatalf("directory result = (%q, %v), want (%q, nil)", root, err, dir)
	}
	resolver := NewFactoryConfigRootResolver(factoryConfigPathSourceStub{
		info: factoryConfigPathInfoStub{mode: fs.ModeNamedPipe},
	})
	if _, err := resolver("factory.pipe"); err == nil ||
		!strings.Contains(err.Error(), "must be a file or directory") {
		t.Fatalf("non-regular path error = %v", err)
	}
}

func TestFactoryConfigFileLoaderPreservesReadAndParseContext(t *testing.T) {
	want := &factorydefinitions.FactoryConfig{Name: "alpha"}
	load := NewFactoryConfigFileLoader(
		func(path string) (factorydefinitions.AuthoredFactorySource, error) {
			if path == "missing.json" {
				return factorydefinitions.AuthoredFactorySource{}, fs.ErrNotExist
			}
			return factorydefinitions.AuthoredFactorySource{
				Path:   path,
				Format: factorydefinitions.AuthoredFactoryFormatJSON,
				Data:   []byte("payload"),
			}, nil
		},
		func(payload []byte) (*factorydefinitions.FactoryConfig, error) {
			if string(payload) != "payload" {
				t.Fatalf("payload = %q", payload)
			}
			return want, nil
		},
	)
	got, err := load("factory.json")
	if err != nil || got != want {
		t.Fatalf("result = (%#v, %v)", got, err)
	}
	if _, err := load("missing.json"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing error = %v", err)
	}
	parseErr := errors.New("invalid")
	load = NewFactoryConfigFileLoader(
		func(path string) (factorydefinitions.AuthoredFactorySource, error) {
			return factorydefinitions.AuthoredFactorySource{
				Path:   path,
				Format: factorydefinitions.AuthoredFactoryFormatJSON,
				Data:   []byte("bad"),
			}, nil
		},
		func([]byte) (*factorydefinitions.FactoryConfig, error) { return nil, parseErr },
	)
	if _, err := load("factory.json"); !errors.Is(err, parseErr) {
		t.Fatalf("parse error = %v", err)
	}
}

type osFS struct{}

func (osFS) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

type factoryConfigPathSourceStub struct {
	info fs.FileInfo
}

func (s factoryConfigPathSourceStub) Stat(string) (fs.FileInfo, error) { return s.info, nil }

type factoryConfigPathInfoStub struct {
	mode fs.FileMode
}

func (factoryConfigPathInfoStub) Name() string        { return "factory.pipe" }
func (factoryConfigPathInfoStub) Size() int64         { return 0 }
func (i factoryConfigPathInfoStub) Mode() fs.FileMode { return i.mode }
func (factoryConfigPathInfoStub) ModTime() time.Time  { return time.Time{} }
func (i factoryConfigPathInfoStub) IsDir() bool       { return i.mode.IsDir() }
func (factoryConfigPathInfoStub) Sys() any            { return nil }
