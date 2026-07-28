package automations_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// DEL-AUTO proves Automations ships the canonical packaged-service root: only
// wire/, internal/, and transports/ package directories plus root contract files.
func TestPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	serviceRoot := serviceRootDir(t)
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
		t.Fatal("pkg/services/automations/service must not exist after DEL-AUTO")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat service/ = %v", err)
	}

	wantSubservices := []string{
		"cron",
		"filesystem_watchers",
		"hosted_sources",
		"reconciliation",
		"script_pollers",
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
}

func serviceRootDir(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() = %v", err)
	}
	if strings.HasSuffix(filepath.ToSlash(root), "/pkg/services/automations") {
		return root
	}
	t.Fatalf("unexpected test working directory %q; run from pkg/services/automations", root)
	return ""
}
