package root_composition_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const operatorSettingsServiceRootRelative = "pkg/services/operator_settings"

// TestOperatorSettingsPackagedRootShapeMatchesCanonicalServiceLayout proves Operator
// Settings ships the canonical packaged-service root: wire/, internal/, and
// transports/ plus thin root contract files. Residual public testdata/ may remain
// as committed unexpected move debt; deleted transitional public siblings must
// not reappear.
func TestOperatorSettingsPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	serviceRoot := filepath.Join(repoRoot, filepath.FromSlash(operatorSettingsServiceRootRelative))
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", serviceRoot, err)
	}

	spec, ok := ownershipinventory.OwnerTopLevelSpecFor("operator_settings")
	if !ok {
		t.Fatal("OwnerTopLevelSpecFor(operator_settings) ok = false")
	}

	var gotRootDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			gotRootDirs = append(gotRootDirs, entry.Name())
		}
	}
	slices.Sort(gotRootDirs)

	wantRetain := slices.Clone(spec.ExpectedRetain)
	slices.Sort(wantRetain)
	for _, name := range wantRetain {
		if !slices.Contains(gotRootDirs, name) {
			t.Fatalf("service root missing canonical retain directory %q; got %v", name, gotRootDirs)
		}
	}

	for _, deleted := range []string{"identityinventory", "servicewire", "testlink", "testproviders"} {
		if slices.Contains(gotRootDirs, deleted) {
			t.Fatalf("deleted transitional package %q must not remain as a public top-level directory", deleted)
		}
	}

	for _, unexpected := range spec.Unexpected {
		if !slices.Contains(gotRootDirs, unexpected) {
			t.Fatalf("committed unexpected move-debt directory %q missing from service root; got %v", unexpected, gotRootDirs)
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
	wantSubservices := []string{"document", "resolution"}
	slices.Sort(wantSubservices)
	if !slices.Equal(gotSubservices, wantSubservices) {
		t.Fatalf("internal/services directories = %v, want %v", gotSubservices, wantSubservices)
	}
}
