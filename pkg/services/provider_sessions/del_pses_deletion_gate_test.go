package providersessions_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

const deletedTransitionalServiceImportPath = "github.com/portpowered/infinite-you/pkg/services/provider_sessions/service"

// DEL-PSES story 002 deletes the emptied transitional service/ compile shim and
// clears production/test imports. Each subtest asserts one observable post-delete
// invariant; wire behavioral proofs live in sibling boundary tests.

func TestDelPsesDeletionGate_TransitionalServiceRemoved(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "provider_sessions")

	t.Run("transitional_service_directory_absent", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(serviceRoot, "service")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("transitional service/ must be deleted; stat = %v", err)
		}
	})

	t.Run("canonical_root_directories_after_deletion", func(t *testing.T) {
		t.Parallel()
		wantRootDirs := []string{"internal", "transports", "wire"}
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

	t.Run("top_level_inventory_matches_live_tree", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyProviderSessionsTopLevelInventory(root); err != nil {
			t.Fatalf("VerifyProviderSessionsTopLevelInventory() error = %v", err)
		}
	})

	t.Run("no_production_or_test_imports_of_deleted_service_package", func(t *testing.T) {
		t.Parallel()
		cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "./...")
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list repository deps: %v\n%s", err, output)
		}
		var violations []string
		for _, dep := range strings.Fields(string(output)) {
			if dep == deletedTransitionalServiceImportPath ||
				strings.HasPrefix(dep, deletedTransitionalServiceImportPath+"/") {
				violations = append(violations, dep)
			}
		}
		if len(violations) > 0 {
			t.Fatalf("deleted transitional service package still imported:\n%s", strings.Join(violations, "\n"))
		}
	})

	t.Run("reader_subservices_preserved", func(t *testing.T) {
		t.Parallel()
		wantSubservices := []string{"codex_reader", "cursor_reader"}
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
}
