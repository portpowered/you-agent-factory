package work_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	deletedTransitionalWorkServiceImportPath            = "github.com/portpowered/infinite-you/pkg/services/work/service"
	deletedTransitionalStateAccessRecordingsImportPath  = "github.com/portpowered/infinite-you/pkg/services/work/stateaccessrecordings"
)

// DEL-WORK story 002 deletes emptied transitional service/ and
// stateaccessrecordings/ compile shims and clears production/test imports.
// Each subtest asserts one observable post-delete invariant; wire behavioral
// proofs live in sibling boundary tests.

func TestDelWorkDeletionGate_TransitionalPublicPathsRemoved(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "work")

	t.Run("transitional_service_directory_absent", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(serviceRoot, "service")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("transitional service/ must be deleted; stat = %v", err)
		}
	})

	t.Run("transitional_stateaccessrecordings_directory_absent", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(serviceRoot, "stateaccessrecordings")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("transitional stateaccessrecordings/ must be deleted; stat = %v", err)
		}
	})

	t.Run("canonical_root_directories_after_deletion", func(t *testing.T) {
		t.Parallel()
		wantRootDirs := []string{"internal", "testdata", "transports", "wire"}
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
		slices.Sort(wantRootDirs)
		if !slices.Equal(gotRootDirs, wantRootDirs) {
			t.Fatalf("service root directories = %v, want %v", gotRootDirs, wantRootDirs)
		}
	})

	t.Run("no_production_or_test_imports_of_deleted_transitional_packages", func(t *testing.T) {
		t.Parallel()
		cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "./...")
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list repository deps: %v\n%s", err, output)
		}
		deleted := []string{
			deletedTransitionalWorkServiceImportPath,
			deletedTransitionalStateAccessRecordingsImportPath,
		}
		var violations []string
		for _, dep := range strings.Fields(string(output)) {
			for _, forbidden := range deleted {
				if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
					violations = append(violations, dep)
				}
			}
		}
		if len(violations) > 0 {
			t.Fatalf("deleted transitional work packages still imported:\n%s", strings.Join(violations, "\n"))
		}
	})

	t.Run("private_subservices_preserved", func(t *testing.T) {
		t.Parallel()
		wantSubservices := []string{"content_materialization", "content_staging", "state_access"}
		subservicesRoot := filepath.Join(serviceRoot, "internal", "services")
		entries, err := os.ReadDir(subservicesRoot)
		if err != nil {
			t.Fatalf("ReadDir(%q) = %v", subservicesRoot, err)
		}
		var gotSubservices []string
		for _, entry := range entries {
			if entry.IsDir() {
				gotSubservices = append(gotSubservices, entry.Name())
			}
		}
		slices.Sort(gotSubservices)
		slices.Sort(wantSubservices)
		if !slices.Equal(gotSubservices, wantSubservices) {
			t.Fatalf("internal/services directories = %v, want %v", gotSubservices, wantSubservices)
		}
	})

	t.Run("wire_construction_bridge_preserved", func(t *testing.T) {
		t.Parallel()
		_, err := os.Stat(filepath.Join(serviceRoot, "wire", "wire.go"))
		if err != nil {
			t.Fatalf("wire/wire.go must still construct Work after transitional deletion: %v", err)
		}
	})
}
