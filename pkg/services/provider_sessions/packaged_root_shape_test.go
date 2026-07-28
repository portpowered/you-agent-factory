package providersessions_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// DEL-PSES story 005 seals the canonical packaged-service root: only wire/,
// internal/, and transports/ package directories plus thin root contract files.
func TestPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "provider_sessions")
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

	if _, err := os.Stat(filepath.Join(serviceRoot, "service")); err == nil {
		t.Fatal("pkg/services/provider_sessions/service must not exist after DEL-PSES")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat service/ = %v", err)
	}

	wantSubservices := []string{"codex_reader", "cursor_reader"}
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
