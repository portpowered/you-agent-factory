package recordings_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// DEL-REC story 005 seals the canonical packaged-service root: only wire/,
// internal/, transports/, and thin root contract files after transitional
// public deletion.
func TestPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "recordings")
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", serviceRoot, err)
	}

	wantRootDirs := []string{"internal", "transports", "wire"}
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

	for _, deleted := range []string{"artifacts", "events", "projections", "replay", "service"} {
		path := filepath.Join(serviceRoot, deleted)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("pkg/services/recordings/%s must not exist after DEL-REC", deleted)
		}
	}

	wantSubservices := []string{
		"artifacts_export",
		"canonical_ledger",
		"projection_query",
		"recording_lifecycle",
		"replay",
	}
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

	if err := ownershipinventory.VerifyRecordingsTopLevelInventory(root); err != nil {
		t.Fatalf("VerifyRecordingsTopLevelInventory() error = %v", err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return dir
}
