package root_composition_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const recordingsServiceRootRelative = "pkg/services/recordings"

var recordingsPackagedRootDirectories = []string{"internal", "transports", "wire"}

var recordingsInternalSubservices = []string{
	"artifacts_export",
	"canonical_ledger",
	"projection_query",
	"recording_lifecycle",
	"replay",
}

var recordingsDeletedTransitionalTopLevel = []string{
	"artifacts",
	"events",
	"projections",
	"replay",
	"service",
}

// TestRecordingsPackagedRootShapeMatchesCanonicalServiceLayout proves Recordings
// ships the canonical packaged-service root: only wire/, internal/, and
// transports/ package directories plus thin root contract files.
func TestRecordingsPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	serviceRoot := filepath.Join(repoRoot, filepath.FromSlash(recordingsServiceRootRelative))
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
	wantRootDirs := slices.Clone(recordingsPackagedRootDirectories)
	slices.Sort(wantRootDirs)
	if !slices.Equal(gotRootDirs, wantRootDirs) {
		t.Fatalf("service root directories = %v, want %v", gotRootDirs, wantRootDirs)
	}

	for _, deleted := range recordingsDeletedTransitionalTopLevel {
		path := filepath.Join(serviceRoot, deleted)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("pkg/services/recordings/%s must not exist after DEL-REC", deleted)
		}
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
	wantSubservices := slices.Clone(recordingsInternalSubservices)
	slices.Sort(wantSubservices)
	if !slices.Equal(gotSubservices, wantSubservices) {
		t.Fatalf("internal/services directories = %v, want %v", gotSubservices, wantSubservices)
	}

	if err := ownershipinventory.VerifyRecordingsTopLevelInventory(repoRoot); err != nil {
		t.Fatalf("VerifyRecordingsTopLevelInventory() error = %v", err)
	}
}
