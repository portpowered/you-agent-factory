package work_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// DEL-WORK story 005 seals the canonical packaged-service root: only wire/,
// internal/, transports/, and legitimate testdata/ package directories plus
// thin root contract files after transitional public deletion.
func TestPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "work")
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", serviceRoot, err)
	}

	wantRootDirs := []string{"internal", "testdata", "transports", "wire"}
	var gotRootDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		gotRootDirs = append(gotRootDirs, entry.Name())
	}
	slices.Sort(gotRootDirs)
	if !slices.Equal(gotRootDirs, wantRootDirs) {
		t.Fatalf("service root directories = %v, want %v", gotRootDirs, wantRootDirs)
	}

	for _, deleted := range []string{"service", "stateaccessrecordings"} {
		path := filepath.Join(serviceRoot, deleted)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("pkg/services/work/%s must not exist after DEL-WORK", deleted)
		}
	}

	wantSubservices := []string{"content_materialization", "content_staging", "state_access"}
	subservicesRoot := filepath.Join(serviceRoot, "internal", "services")
	subentries, err := os.ReadDir(subservicesRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", subservicesRoot, err)
	}
	var gotSubservices []string
	for _, entry := range subentries {
		if entry.IsDir() {
			gotSubservices = append(gotSubservices, entry.Name())
		}
	}
	slices.Sort(gotSubservices)
	if !slices.Equal(gotSubservices, wantSubservices) {
		t.Fatalf("internal/services directories = %v, want %v", gotSubservices, wantSubservices)
	}

	if err := ownershipinventory.VerifyWorkTopLevelInventory(root); err != nil {
		t.Fatalf("VerifyWorkTopLevelInventory() error = %v", err)
	}
}
