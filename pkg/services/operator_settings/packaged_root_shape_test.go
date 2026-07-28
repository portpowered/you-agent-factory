package operatorsettings_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// DEL-SET story 005 seals the canonical packaged-service root: wire/, internal/,
// transports/, and INV-retained test-only testdata/ package directories plus thin
// root contract files. Transitional public siblings deleted in pss-del-set-002
// must not return.
func TestPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "operator_settings")
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

	for _, deleted := range []string{"identityinventory", "servicewire", "testlink", "testproviders"} {
		path := filepath.Join(serviceRoot, deleted)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("pkg/services/operator_settings/%s must not exist after DEL-SET", deleted)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s/ = %v", deleted, err)
		}
	}

	wantSubservices := []string{"document", "resolution"}
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
}
