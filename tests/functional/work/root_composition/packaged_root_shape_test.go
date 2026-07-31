package root_composition_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const workServiceRootRelative = "pkg/services/work"

var workPackagedRootDirectories = []string{"internal", "transports", "wire"}

var workInternalSubservices = []string{
	"content_materialization",
	"content_staging",
	"state_access",
}

// TestWorkPackagedRootShapeMatchesCanonicalServiceLayout proves Work ships the
// canonical packaged-service root: wire/, internal/, transports/, and thin root
// contract files after DEL-WORK transitional deletion.
func TestWorkPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	serviceRoot := filepath.Join(repoRoot, filepath.FromSlash(workServiceRootRelative))
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
	wantRootDirs := slices.Clone(workPackagedRootDirectories)
	slices.Sort(wantRootDirs)
	if !slices.Equal(gotRootDirs, wantRootDirs) {
		t.Fatalf("service root directories = %v, want %v", gotRootDirs, wantRootDirs)
	}

	for _, deleted := range []string{"service", "stateaccessrecordings"} {
		path := filepath.Join(serviceRoot, deleted)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("pkg/services/work/%s must not exist after DEL-WORK", deleted)
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
	wantSubservices := slices.Clone(workInternalSubservices)
	slices.Sort(wantSubservices)
	if !slices.Equal(gotSubservices, wantSubservices) {
		t.Fatalf("internal/services directories = %v, want %v", gotSubservices, wantSubservices)
	}
}
