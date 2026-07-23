package packagedfactorycatalog_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
)

func TestCheckAcceptsCurrentCatalogWithoutWriting(t *testing.T) {
	root := catalogRepositoryFixture(t)
	before := snapshotCatalogPackage(t, root)

	drift, err := packagedfactorycatalog.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("Check() drift = %#v, want none", drift)
	}
	if after := snapshotCatalogPackage(t, root); !reflect.DeepEqual(after, before) {
		t.Fatal("Check() changed package bytes")
	}
}

func TestCheckReportsEveryByteAndPathDriftCategory(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(t *testing.T, root string)
		wantStale   []string
		wantMissing []string
		wantExtra   []string
	}{
		{
			name: "stale manifest",
			mutate: func(t *testing.T, root string) {
				writeCatalogFile(t, root, "generated/manifest.json", []byte("{}\n"))
			},
			wantStale: []string{"generated/manifest.json"},
		},
		{
			name: "stale JSON",
			mutate: func(t *testing.T, root string) {
				writeCatalogFile(t, root, "generated/factories/tts/factory.json", []byte("{}\n"))
			},
			wantStale: []string{"generated/factories/tts/factory.json"},
		},
		{
			name: "stale YAML",
			mutate: func(t *testing.T, root string) {
				writeCatalogFile(t, root, "generated/factories/tts/factory.yaml", []byte("{}\n"))
			},
			wantStale: []string{"generated/factories/tts/factory.yaml"},
		},
		{
			name: "missing output",
			mutate: func(t *testing.T, root string) {
				removeCatalogPath(t, root, "generated/factories/tts/factory.json")
			},
			wantMissing: []string{"generated/factories/tts/factory.json"},
		},
		{
			name: "unexpected output",
			mutate: func(t *testing.T, root string) {
				writeCatalogFile(t, root, "generated/unexpected.json", []byte("{}\n"))
			},
			wantExtra: []string{"generated/unexpected.json"},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			root := catalogRepositoryFixture(t)
			test.mutate(t, root)
			before := snapshotCatalogPackage(t, root)

			drift, err := packagedfactorycatalog.Check(root)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if !reflect.DeepEqual(drift.Stale, test.wantStale) ||
				!reflect.DeepEqual(drift.Missing, test.wantMissing) ||
				!reflect.DeepEqual(drift.Unexpected, test.wantExtra) {
				t.Fatalf("Check() drift = %#v", drift)
			}
			if after := snapshotCatalogPackage(t, root); !reflect.DeepEqual(after, before) {
				t.Fatal("Check() changed package bytes")
			}
		})
	}
}

func TestCheckRejectsManifestIntegrityAndLocatorDrift(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
	}{
		{
			name: "hash",
			from: `"sha256": "a38464667ca8ffcd525b2763fe6542c24e42c59cea885b38c96994d0bf4ee25f"`,
			to:   `"sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
		},
		{
			name: "locator",
			from: `"locator": "generated/factories/tts/factory.json"`,
			to:   `"locator": "generated/factories/tts/renamed.json"`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			root := catalogRepositoryFixture(t)
			target := catalogPath(root, "generated/manifest.json")
			payload, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			changed := strings.Replace(string(payload), test.from, test.to, 1)
			if changed == string(payload) {
				t.Fatalf("fixture does not contain %q", test.from)
			}
			if err := os.WriteFile(target, []byte(changed), 0o644); err != nil {
				t.Fatal(err)
			}

			drift, err := packagedfactorycatalog.Check(root)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if !reflect.DeepEqual(drift.Stale, []string{"generated/manifest.json"}) {
				t.Fatalf("Check() drift = %#v", drift)
			}
		})
	}
}

func TestCheckTracksAuthoredInventoryAdditionsAndRemovals(t *testing.T) {
	t.Run("addition", func(t *testing.T) {
		root := catalogRepositoryFixture(t)
		source := catalogPath(root, "factories/tts")
		destination := catalogPath(root, "factories/added")
		if err := copyCatalogTree(source, destination); err != nil {
			t.Fatal(err)
		}
		factoryPath := filepath.Join(destination, "factory.json")
		payload, err := os.ReadFile(factoryPath)
		if err != nil {
			t.Fatal(err)
		}
		payload = []byte(strings.ReplaceAll(string(payload), "@you/tts", "@you/added"))
		payload = []byte(strings.ReplaceAll(string(payload), "builtin-tts", "builtin-added"))
		if err := os.WriteFile(factoryPath, payload, 0o644); err != nil {
			t.Fatal(err)
		}

		drift, err := packagedfactorycatalog.Check(root)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		want := []string{
			"generated/factories/added/factory.json",
			"generated/factories/added/factory.yaml",
		}
		if !reflect.DeepEqual(drift.Stale, []string{"generated/manifest.json"}) ||
			!reflect.DeepEqual(drift.Missing, want) ||
			len(drift.Unexpected) != 0 {
			t.Fatalf("Check() drift = %#v", drift)
		}
	})

	t.Run("removal", func(t *testing.T) {
		root := catalogRepositoryFixture(t)
		if err := os.RemoveAll(catalogPath(root, "factories/tts")); err != nil {
			t.Fatal(err)
		}

		drift, err := packagedfactorycatalog.Check(root)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		want := []string{
			"generated/factories/tts/factory.json",
			"generated/factories/tts/factory.yaml",
		}
		if !reflect.DeepEqual(drift.Stale, []string{"generated/manifest.json"}) ||
			len(drift.Missing) != 0 ||
			!reflect.DeepEqual(drift.Unexpected, want) {
			t.Fatalf("Check() drift = %#v", drift)
		}
	})
}

func catalogRepositoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join("..", "..", "packages", "packaged-factories")
	destination := catalogPath(root, "")
	if err := copyCatalogTree(source, destination); err != nil {
		t.Fatalf("copy package fixture: %v", err)
	}
	return root
}

func catalogPath(root, relative string) string {
	return filepath.Join(
		root,
		"packages",
		"packaged-factories",
		filepath.FromSlash(relative),
	)
}

func writeCatalogFile(t *testing.T, root, relative string, payload []byte) {
	t.Helper()
	target := catalogPath(root, relative)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, payload, 0o644); err != nil {
		t.Fatal(err)
	}
}

func removeCatalogPath(t *testing.T, root, relative string) {
	t.Helper()
	if err := os.Remove(catalogPath(root, relative)); err != nil {
		t.Fatal(err)
	}
}

func copyCatalogTree(source, destination string) error {
	return filepath.WalkDir(source, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		payload, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		return os.WriteFile(target, payload, 0o644)
	})
}

func snapshotCatalogPackage(t *testing.T, root string) map[string]string {
	t.Helper()
	base := catalogPath(root, "")
	snapshot := make(map[string]string)
	if err := filepath.WalkDir(base, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		relative, err := filepath.Rel(base, current)
		if err != nil {
			return err
		}
		payload, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		snapshot[filepath.ToSlash(relative)] = string(payload)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return snapshot
}
