package authoredlayout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestLoadFactorySourceResolvesFileAndDirectoryPaths(t *testing.T) {
	loadFactorySource := NewFactorySourceLoader(localTestFileSystem{})
	dir := t.TempDir()
	path := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
	want := []byte(`{"name":"authored"}`)
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	for _, source := range []string{path, dir} {
		got, err := loadFactorySource(source)
		if err != nil {
			t.Fatalf("LoadFactorySource(%q) error = %v", source, err)
		}
		if string(got) != string(want) {
			t.Fatalf("LoadFactorySource(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestLoadFactorySourceReportsResolutionAndReadFailures(t *testing.T) {
	loadFactorySource := NewFactorySourceLoader(localTestFileSystem{})
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := loadFactorySource(missing); err == nil ||
		!strings.Contains(err.Error(), "find factory config source") {
		t.Fatalf("LoadFactorySource(missing) error = %v", err)
	}

	dirWithoutFactory := t.TempDir()
	if _, err := loadFactorySource(dirWithoutFactory); err == nil ||
		!strings.Contains(err.Error(), "read factory config") {
		t.Fatalf("LoadFactorySource(directory without factory.json) error = %v", err)
	}
}

func TestFactorySourceLoaderFailsClosedWithoutFileSystem(t *testing.T) {
	loadFactorySource := NewFactorySourceLoader(nil)
	if _, err := loadFactorySource("factory.json"); err == nil ||
		!strings.Contains(err.Error(), "authored-source filesystem is required") {
		t.Fatalf("loadFactorySource() error = %v", err)
	}
}
