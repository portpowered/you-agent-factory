package ownershipinventory_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestProvidersRootGoFilesMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	if err := ownershipinventory.VerifyProvidersRootContractInventory(repositoryRoot(t)); err != nil {
		t.Fatalf("VerifyProvidersRootContractInventory() error = %v", err)
	}
}

func TestProvidersThinRootContractFiles(t *testing.T) {
	t.Parallel()

	want := []string{
		"acp_configuration_contract.go",
		"acp_contract.go",
		"catalog_characterization_test.go",
		"catalog_contract.go",
		"doc.go",
		"execute_characterization_test.go",
		"execute_contract.go",
		"identity_characterization_test.go",
		"identity_contract.go",
		"lifecycle_contract.go",
		"root_catalog_delegation_test.go",
		"root_contract_characterization_test.go",
		"selection_characterization_test.go",
		"selection_contract.go",
		"service_contract.go",
	}
	if !slices.Equal(ownershipinventory.ProvidersThinRootContractFiles, want) {
		t.Fatalf("ProvidersThinRootContractFiles = %v, want %v", ownershipinventory.ProvidersThinRootContractFiles, want)
	}
	for _, fileName := range ownershipinventory.ProvidersThinRootContractFiles {
		kind, _, ok := ownershipinventory.ClassifyProvidersRootContractFile(fileName)
		if !ok || kind != "thin_root_retain" {
			t.Fatalf("ClassifyProvidersRootContractFile(%q) = (%q, %v), want thin_root_retain", fileName, kind, ok)
		}
	}
}

func TestProvidersRootContractFoldDestinationsRejectOwnerRootRetain(t *testing.T) {
	t.Parallel()

	for _, target := range ownershipinventory.ProvidersExcessRootContractFolds {
		if target.Destination == ownershipinventory.ProvidersOwnerPackagePath {
			t.Fatalf("cluster %q folds to owner root retain destination", target.Cluster)
		}
		if !ownershipinventory.IsProvidersPrivateRootContractFoldDestination(target.Destination) {
			t.Fatalf("cluster %q destination = %q, want private path under %s/internal", target.Cluster, target.Destination, ownershipinventory.ProvidersOwnerPackagePath)
		}
		if !strings.HasPrefix(ownershipinventory.ProvidersRootContractFoldCondition(target.Cluster), "CLN-PROV-CONTRACT-ROOTS") {
			t.Fatalf("fold condition for %q does not name CLN-PROV-CONTRACT-ROOTS", target.Cluster)
		}
	}
}

func TestProvidersRootContractInventoryDetectsUnexpectedLiveFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	serviceRoot := filepath.Join(root, filepath.FromSlash(ownershipinventory.ProvidersOwnerPackagePath))
	if err := os.MkdirAll(serviceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", serviceRoot, err)
	}
	for _, fileName := range ownershipinventory.ProvidersRootContractInventory() {
		if err := os.WriteFile(filepath.Join(serviceRoot, fileName), []byte("package providers\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", fileName, err)
		}
	}
	if err := os.WriteFile(filepath.Join(serviceRoot, "unexpected.go"), []byte("package providers\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(unexpected.go): %v", err)
	}

	err := ownershipinventory.VerifyProvidersRootContractInventory(root)
	if err == nil || !strings.Contains(err.Error(), "drift") {
		t.Fatalf("VerifyProvidersRootContractInventory() error = %v, want drift failure", err)
	}
}
