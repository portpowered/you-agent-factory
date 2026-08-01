package providers_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	serviceRoot := filepath.Join(providersRepositoryRoot(t), "pkg", "services", "providers")
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", serviceRoot, err)
	}
	var got []string
	for _, entry := range entries {
		if entry.IsDir() {
			got = append(got, entry.Name())
		}
	}
	slices.Sort(got)
	if want := []string{"internal", "transports", "wire"}; !slices.Equal(got, want) {
		t.Fatalf("Providers root directories = %v, want %v", got, want)
	}

	for _, forbidden := range []string{"catalog", "execution", "inference", "service", "services"} {
		path := filepath.Join(serviceRoot, forbidden)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("pkg/services/providers/%s must not exist as a public sibling", forbidden)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s/ = %v", forbidden, err)
		}
	}
}
