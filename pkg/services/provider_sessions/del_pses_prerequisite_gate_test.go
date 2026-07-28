package providersessions_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

// DEL-PSES story 001 confirms CLN-PSES-FOLD-SERVICE, CLN-PSES-LEGACY-PACKAGES,
// and CLN-PSES-CONTRACT-ROOTS are Factory-complete before leased deletion of
// emptied transitional public paths begins. Each subtest asserts one observable
// tree invariant left by the prerequisite fold packets. Fold behavioral proofs
// live in sibling boundary tests; this gate only seals prerequisite completion.

func TestDelPsesPrerequisiteGate_AllFoldPacketsFactoryComplete(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "provider_sessions")

	t.Run("CLN-PSES-FOLD-SERVICE_internal_service_folded", func(t *testing.T) {
		t.Parallel()
		_, err := os.Stat(filepath.Join(serviceRoot, "internal", "service", "service.go"))
		if err != nil {
			t.Fatalf("internal/service/service.go must exist after CLN-PSES-FOLD-SERVICE: %v", err)
		}
	})

	t.Run("CLN-PSES-FOLD-SERVICE_wire_construction_bridge", func(t *testing.T) {
		t.Parallel()
		_, err := os.Stat(filepath.Join(serviceRoot, "wire", "wire.go"))
		if err != nil {
			t.Fatalf("wire/wire.go must construct Provider Sessions after fold; stat = %v", err)
		}
	})

	t.Run("CLN-PSES-FOLD-SERVICE_implementation_moved_from_root", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{
			"construction_ports.go",
			"wire_behavioral_proof_test.go",
			"details_providers_boundary_test.go",
			"inspect_providers_boundary_test.go",
			"project_providers_boundary_test.go",
			"readers_providers_boundary_test.go",
			"service_test.go",
		} {
			path := filepath.Join(serviceRoot, name)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("%s must be folded to internal before DEL-PSES; stat = %v", name, err)
			}
		}
		internalPath := filepath.Join(serviceRoot, "internal", "construction_ports.go")
		if _, err := os.Stat(internalPath); err != nil {
			t.Fatalf("internal/construction_ports.go must exist after fold: %v", err)
		}
	})

	t.Run("CLN-PSES-LEGACY-PACKAGES_zero_extra_public_siblings", func(t *testing.T) {
		t.Parallel()
		if err := ownershipinventory.VerifyProviderSessionsZeroExtraPublicSiblingAbsence(root); err != nil {
			t.Fatalf("VerifyProviderSessionsZeroExtraPublicSiblingAbsence() error = %v", err)
		}
	})

	t.Run("CLN-PSES-LEGACY-PACKAGES_reader_subservices_present", func(t *testing.T) {
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

	t.Run("CLN-PSES-LEGACY-PACKAGES_transitional_service_retained_for_DEL", func(t *testing.T) {
		t.Parallel()
		info, err := os.Stat(filepath.Join(serviceRoot, "service"))
		if err != nil {
			t.Fatalf("transitional service/ must remain until DEL-PSES deletes it; stat = %v", err)
		}
		if !info.IsDir() {
			t.Fatal("transitional service/ must remain a directory until DEL-PSES")
		}
	})

	t.Run("CLN-PSES-CONTRACT-ROOTS_thin_root_contract_sealed", func(t *testing.T) {
		t.Parallel()
		inventory, err := ownershipinventory.LoadProviderSessionsRootGoInventory(root)
		if err != nil {
			t.Fatalf("LoadProviderSessionsRootGoInventory() error = %v", err)
		}
		if foldTargets := ownershipinventory.ProviderSessionsRootGoFoldTargets(inventory); len(foldTargets) != 0 {
			t.Fatalf("fold targets remain in inventory = %v, want none at sealed root", foldTargets)
		}
		wantProduction := []string{"contracts.go", "doc.go"}
		var gotProduction []string
		for _, file := range inventory.Files {
			if file.Classification == ownershipinventory.ProviderSessionsRootGoThinContract {
				gotProduction = append(gotProduction, file.File)
			}
		}
		slices.Sort(gotProduction)
		if !slices.Equal(gotProduction, wantProduction) {
			t.Fatalf("thin root production files = %v, want %v", gotProduction, wantProduction)
		}
		for _, name := range []string{
			"construction_ports.go",
			"details_providers_boundary_test.go",
			"inspect_providers_boundary_test.go",
			"project_providers_boundary_test.go",
			"readers_providers_boundary_test.go",
			"service_test.go",
			"wire_behavioral_proof_test.go",
		} {
			if _, err := os.Stat(filepath.Join(serviceRoot, name)); !os.IsNotExist(err) {
				t.Fatalf("%s still present at public provider_sessions root after CLN-PSES-CONTRACT-ROOTS", name)
			}
		}
	})

	t.Run("CLN-PSES-CONTRACT-ROOTS_canonical_root_directories", func(t *testing.T) {
		t.Parallel()
		wantRootDirs := []string{"internal", "service", "transports", "wire"}
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
}