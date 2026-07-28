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
		if string(got.Data) != string(want) {
			t.Fatalf("LoadFactorySource(%q) = %q, want %q", source, got.Data, want)
		}
	}
}

func TestLoadFactorySourceSelectsEachSupportedDirectoryRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
	}{
		{name: "factory.json", payload: `{"name":"authored"}`},
		{name: "factory.yaml", payload: "name: authored\n"},
		{name: "factory.yml", payload: "name: authored\n"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, test.name)
			if err := os.WriteFile(path, []byte(test.payload), 0o644); err != nil {
				t.Fatalf("WriteFile(%s): %v", test.name, err)
			}

			got, err := NewFactorySourceLoader(localTestFileSystem{})(dir)
			if err != nil {
				t.Fatalf("LoadFactorySource(%q) error = %v", dir, err)
			}
			if string(got.Data) != `{"name":"authored"}` {
				t.Fatalf("LoadFactorySource(%q) = %q, want canonical authored payload", dir, got.Data)
			}
			if got.Path != path {
				t.Fatalf("selected source path = %q, want %q", got.Path, path)
			}
		})
	}
}

func TestLoadFactorySourceRejectsMissingAndAmbiguousDirectoryRoots(t *testing.T) {
	t.Parallel()

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		_, err := NewFactorySourceLoader(localTestFileSystem{})(dir)
		if err == nil {
			t.Fatal("LoadFactorySource(directory without root) succeeded")
		}
		for _, want := range []string{dir, "factory.json", "factory.yaml", "factory.yml"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("missing-root error = %q, want %q", err, want)
			}
		}
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			t.Fatalf("ReadDir(%q): %v", dir, readErr)
		}
		if len(entries) != 0 {
			t.Fatalf("directory resolution created entries: %#v", entries)
		}
	})

	for _, roots := range [][]string{
		{"factory.json", "factory.yaml"},
		{"factory.json", "factory.yml"},
		{"factory.yaml", "factory.yml"},
		{"factory.json", "factory.yaml", "factory.yml"},
	} {
		roots := roots
		t.Run(strings.Join(roots, "+"), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for _, root := range roots {
				if err := os.WriteFile(filepath.Join(dir, root), []byte(`{"name":"conflict"}`), 0o644); err != nil {
					t.Fatalf("WriteFile(%s): %v", root, err)
				}
			}

			_, err := NewFactorySourceLoader(localTestFileSystem{})(dir)
			if err == nil || !strings.Contains(err.Error(), "ambiguous roots") {
				t.Fatalf("LoadFactorySource(ambiguous directory) error = %v", err)
			}
			for _, root := range roots {
				if !strings.Contains(err.Error(), filepath.Join(dir, root)) {
					t.Fatalf("ambiguity error = %q, want conflicting root %q", err, root)
				}
			}
		})
	}
}

func TestLoadFactorySourceDirectoryDecodeFailureNamesSelectedRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "factory.yaml")
	if err := os.WriteFile(path, []byte("name: ["), 0o644); err != nil {
		t.Fatalf("WriteFile(factory.yaml): %v", err)
	}
	_, err := NewFactorySourceLoader(localTestFileSystem{})(dir)
	if err == nil {
		t.Fatal("LoadFactorySource(malformed directory root) succeeded")
	}
	for _, want := range []string{path, "YAML"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("decode error = %q, want %q", err, want)
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
		!strings.Contains(err.Error(), "has no supported root") {
		t.Fatalf("LoadFactorySource(directory without supported root) error = %v", err)
	}
}

func TestFactorySourceLoaderFailsClosedWithoutFileSystem(t *testing.T) {
	loadFactorySource := NewFactorySourceLoader(nil)
	if _, err := loadFactorySource("factory.json"); err == nil ||
		!strings.Contains(err.Error(), "authored-source filesystem is required") {
		t.Fatalf("loadFactorySource() error = %v", err)
	}
}
