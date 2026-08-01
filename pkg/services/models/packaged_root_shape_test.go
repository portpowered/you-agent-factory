package models_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestPackagedRootShapeMatchesCanonicalServiceLayout freezes the Models
// packaged-service root to the canonical internal/transports/wire boundary.
// Private legacy implementation packages remain below internal and are not
// alternate public siblings.
func TestPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	root := modelsRepositoryRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "models")
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", serviceRoot, err)
	}

	var gotRootDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			gotRootDirs = append(gotRootDirs, entry.Name())
		}
	}
	slices.Sort(gotRootDirs)
	wantRootDirs := []string{"internal", "transports", "wire"}
	if !slices.Equal(gotRootDirs, wantRootDirs) {
		t.Fatalf("Models root directories = %v, want %v", gotRootDirs, wantRootDirs)
	}

	for _, forbidden := range []string{
		"artifacts", "assets", "catalog", "host", "inference", "local",
		"managedruntime", "service", "servicewire",
	} {
		path := filepath.Join(serviceRoot, forbidden)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("pkg/services/models/%s must not exist as a public sibling", forbidden)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s/ = %v", forbidden, err)
		}
	}
}

func modelsRepositoryRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}
