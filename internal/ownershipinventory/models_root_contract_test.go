package ownershipinventory_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestModelsRootGoFilesMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyModelsRootContractInventory(root); err != nil {
		t.Fatalf("VerifyModelsRootContractInventory() error = %v", err)
	}
}

func TestModelsThinRootContractFiles(t *testing.T) {
	t.Parallel()

	want := []string{
		"asset_scope_characterization_test.go",
		"assets_contract.go",
		"catalog_contract.go",
		"catalog_scope_characterization_test.go",
		"host_contract.go",
		"host_scope_characterization_test.go",
		"local_execution_contract.go",
		"managed_runtime_contract.go",
		"root_authority_seal_characterization_test.go",
		"root_slice_characterization_test.go",
		"runtime_config_contract.go",
		"runtime_construction_contract.go",
		"service_contract.go",
	}
	if !slices.Equal(ownershipinventory.ModelsThinRootContractFiles, want) {
		t.Fatalf("ModelsThinRootContractFiles = %v, want %v", ownershipinventory.ModelsThinRootContractFiles, want)
	}

	for _, fileName := range ownershipinventory.ModelsThinRootContractFiles {
		kind, _, ok := ownershipinventory.ClassifyModelsRootContractFile(fileName)
		if !ok {
			t.Fatalf("ClassifyModelsRootContractFile(%q) ok = false", fileName)
		}
		if kind != "thin_root_retain" {
			t.Fatalf("ClassifyModelsRootContractFile(%q) = %q, want thin_root_retain", fileName, kind)
		}
	}
}

func TestModelsRootUsesCanonicalServiceChildren(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	serviceRoot := filepath.Join(root, filepath.FromSlash(ownershipinventory.ModelsOwnerPackagePath))
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
	wantRootDirs := []string{"internal", "transports", "wire"}
	if !slices.Equal(gotRootDirs, wantRootDirs) {
		t.Fatalf("Models root directories = %v, want %v", gotRootDirs, wantRootDirs)
	}

	for _, forbidden := range []string{
		"artifacts", "assets", "catalog", "host", "inference", "local",
		"managedruntime", "service", "servicewire",
	} {
		path := filepath.Join(serviceRoot, forbidden)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("pkg/services/models/%s must not exist as a public sibling", forbidden)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s/ = %v", forbidden, err)
		}
	}
}

func TestModelsExcessRootContractFoldDestinations(t *testing.T) {
	t.Parallel()

	if len(ownershipinventory.ModelsExcessRootContractFolds) != 0 {
		t.Fatalf("ModelsExcessRootContractFolds = %#v, want empty for the root-contract freeze", ownershipinventory.ModelsExcessRootContractFolds)
	}
}

func TestModelsRootContractFoldDestinationsRejectOwnerRootRetain(t *testing.T) {
	t.Parallel()

	const ownerRoot = ownershipinventory.ModelsOwnerPackagePath
	for _, target := range ownershipinventory.ModelsExcessRootContractFolds {
		if target.Destination == ownerRoot {
			t.Fatalf("cluster %q folds to owner root retain destination", target.Cluster)
		}
		if !ownershipinventory.IsModelsPrivateRootContractFoldDestination(target.Destination) {
			t.Fatalf("cluster %q destination = %q, want private path under %s/internal", target.Cluster, target.Destination, ownerRoot)
		}
		if !strings.HasPrefix(ownershipinventory.ModelsRootContractFoldCondition(target.Cluster), "CLN-MODELS-CONTRACT-ROOTS") {
			t.Fatalf("fold condition for %q does not name CLN-MODELS-CONTRACT-ROOTS", target.Cluster)
		}
	}
}

func TestModelsRootContractInventoryDetectsUnexpectedLiveFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	serviceRoot := filepath.Join(root, filepath.FromSlash(ownershipinventory.ModelsOwnerPackagePath))
	if err := os.MkdirAll(serviceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", serviceRoot, err)
	}
	for _, fileName := range ownershipinventory.ModelsRootContractInventory() {
		if err := os.WriteFile(filepath.Join(serviceRoot, fileName), []byte("package models\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", fileName, err)
		}
	}
	if err := os.WriteFile(filepath.Join(serviceRoot, "unexpected.go"), []byte("package models\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(unexpected.go): %v", err)
	}

	err := ownershipinventory.VerifyModelsRootContractInventory(root)
	if err == nil {
		t.Fatal("VerifyModelsRootContractInventory() error = nil, want drift failure")
	}
	if !strings.Contains(err.Error(), "drift") {
		t.Fatalf("VerifyModelsRootContractInventory() error = %v, want drift failure", err)
	}
}
