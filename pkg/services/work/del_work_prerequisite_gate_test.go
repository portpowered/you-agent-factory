package work_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// DEL-WORK story 001 confirms CLN-WORK-FOLD-SERVICE, CLN-WORK-LEGACY-PACKAGES,
// and CLN-WORK-CONTRACT-ROOTS are Factory-complete before leased deletion of
// emptied transitional public paths begins. Each subtest asserts one observable
// tree invariant left by the prerequisite fold packets. Fold behavioral proofs
// live in sibling boundary tests; this gate only seals prerequisite completion.

func TestDelWorkPrerequisiteGate_AllFoldPacketsFactoryComplete(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "work")

	t.Run("CLN-WORK-FOLD-SERVICE_internal_service_folded", func(t *testing.T) {
		t.Parallel()
		_, err := os.Stat(filepath.Join(serviceRoot, "internal", "service", "service.go"))
		if err != nil {
			t.Fatalf("internal/service/service.go must exist after CLN-WORK-FOLD-SERVICE: %v", err)
		}
	})

	t.Run("CLN-WORK-FOLD-SERVICE_wire_construction_bridge", func(t *testing.T) {
		t.Parallel()
		_, err := os.Stat(filepath.Join(serviceRoot, "wire", "wire.go"))
		if err != nil {
			t.Fatalf("wire/wire.go must construct Work after fold; stat = %v", err)
		}
	})

	t.Run("CLN-WORK-FOLD-SERVICE_implementation_moved_from_root", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{
			"service.go",
			"read.go",
			"live_session_runtime.go",
			"construction_ports.go",
		} {
			path := filepath.Join(serviceRoot, name)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("%s must be folded to internal before DEL-WORK; stat = %v", name, err)
			}
		}
	})

	t.Run("CLN-WORK-LEGACY-PACKAGES_private_subservices_present", func(t *testing.T) {
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

	t.Run("CLN-WORK-LEGACY-PACKAGES_root_behavior_proofs_committed", func(t *testing.T) {
		t.Parallel()
		for _, name := range workRootBehaviorProofFiles {
			path := filepath.Join(serviceRoot, name)
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("root behavior proof file %q missing: %v", name, err)
			}
		}
	})

	t.Run("CLN-WORK-CONTRACT-ROOTS_thin_root_contract_sealed", func(t *testing.T) {
		t.Parallel()
		if len(ownershipinventory.WorkExcessRootContractFolds) != 0 {
			t.Fatalf("fold targets remain in inventory = %v, want none at sealed root", ownershipinventory.WorkExcessRootContractFolds)
		}
		live, err := ownershipinventory.ListWorkRootGoFiles(root)
		if err != nil {
			t.Fatalf("ListWorkRootGoFiles() error = %v", err)
		}
		want := ownershipinventory.WorkRootContractInventory()
		if !slices.Equal(live, want) {
			t.Fatalf("live root .go files = %v, want committed inventory %v", live, want)
		}
		for _, name := range []string{
			"arguments.go",
			"query_list.go",
			"request_codec.go",
			"lineage.go",
			"primary_result.go",
		} {
			if _, err := os.Stat(filepath.Join(serviceRoot, name)); !os.IsNotExist(err) {
				t.Fatalf("%s still present at public work root after CLN-WORK-CONTRACT-ROOTS", name)
			}
		}
	})

	t.Run("CLN-WORK-CONTRACT-ROOTS_private_fold_destinations_present", func(t *testing.T) {
		t.Parallel()
		for _, rel := range []string{
			"internal/requestadmission",
			"internal/invocationreturnpolicy",
			"internal/services/state_access/stateaccessquery",
			"internal/services/state_access/lineagegraph",
		} {
			path := filepath.Join(serviceRoot, filepath.FromSlash(rel))
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("private fold destination %q must exist after CLN-WORK-CONTRACT-ROOTS: %v", rel, err)
			}
			if !info.IsDir() {
				t.Fatalf("private fold destination %q is not a directory", rel)
			}
		}
	})
}
