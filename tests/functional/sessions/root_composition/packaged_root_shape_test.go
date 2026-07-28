package root_composition_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const sessionsServiceRootRelative = "pkg/services/factory_sessions"

var sessionsPackagedRootDirectories = []string{"internal", "transports", "wire"}

// TestSessionsPackagedRootShapeMatchesCanonicalServiceLayout proves Factory
// Sessions ships the canonical packaged-service root: only wire/, internal/,
// and transports/ package directories plus thin root contract files.
func TestSessionsPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	serviceRoot := filepath.Join(repoRoot, filepath.FromSlash(sessionsServiceRootRelative))
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
	wantRootDirs := slices.Clone(sessionsPackagedRootDirectories)
	slices.Sort(wantRootDirs)
	if !slices.Equal(gotRootDirs, wantRootDirs) {
		t.Fatalf("service root directories = %v, want %v", gotRootDirs, wantRootDirs)
	}

	if _, err := os.Stat(filepath.Join(serviceRoot, "service")); err == nil {
		t.Fatal("pkg/services/factory_sessions/service must not exist; Sessions remains packaged as wire/, internal/, transports/")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat service/ = %v", err)
	}
}
