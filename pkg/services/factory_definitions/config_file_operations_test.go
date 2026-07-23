package factorydefinitions

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFactoryConfigRootResolverValidatesSelectedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FactoryConfigFile)
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := NewFactoryConfigRootResolver(osFS{})(path)
	if err != nil || root != dir {
		t.Fatalf("result = (%q, %v), want (%q, nil)", root, err, dir)
	}
	if _, err := NewFactoryConfigRootResolver(osFS{})(dir); err == nil || !strings.Contains(err.Error(), "must be a file") {
		t.Fatalf("directory error = %v", err)
	}
}

func TestFactoryConfigFileLoaderPreservesReadAndParseContext(t *testing.T) {
	want := &FactoryConfig{Name: "alpha"}
	load := NewFactoryConfigFileLoader(
		func(path string) (AuthoredFactorySource, error) {
			if path == "missing.json" {
				return AuthoredFactorySource{}, fs.ErrNotExist
			}
			return AuthoredFactorySource{
				Path:   path,
				Format: AuthoredFactoryFormatJSON,
				Data:   []byte("payload"),
			}, nil
		},
		func(payload []byte) (*FactoryConfig, error) {
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
		func(path string) (AuthoredFactorySource, error) {
			return AuthoredFactorySource{
				Path:   path,
				Format: AuthoredFactoryFormatJSON,
				Data:   []byte("bad"),
			}, nil
		},
		func([]byte) (*FactoryConfig, error) { return nil, parseErr },
	)
	if _, err := load("factory.json"); !errors.Is(err, parseErr) {
		t.Fatalf("parse error = %v", err)
	}
}

type osFS struct{}

func (osFS) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }
