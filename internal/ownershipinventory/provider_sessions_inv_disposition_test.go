package ownershipinventory_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestVerifyProviderSessionsZeroExtraPublicSiblingAbsencePassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsZeroExtraPublicSiblingAbsence(root); err != nil {
		t.Fatalf("VerifyProviderSessionsZeroExtraPublicSiblingAbsence() error = %v", err)
	}
}

func TestVerifyProviderSessionsZeroExtraPublicSiblingAbsenceFailsWhenLiveSiblingAppears(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeProviderSessionsTopLevelInventoryFixture(t, root, providerSessionsTopLevelInventoryFixture{
		children: []ownershipinventory.ProviderSessionsTopLevelChild{
			{Directory: "internal", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
			{Directory: "service", Classification: ownershipinventory.ProviderSessionsTopLevelUnexpectedPublicSibling},
			{Directory: "transports", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
			{Directory: "wire", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
		},
		hasUnexpectedBeyondService: false,
	})
	for _, directory := range []string{"internal", "service", "transports", "wire", "surprise"} {
		mkdirAll(t, filepath.Join(root, "pkg/services/provider_sessions", directory))
	}

	err := ownershipinventory.VerifyProviderSessionsZeroExtraPublicSiblingAbsence(root)
	if err == nil {
		t.Fatal("VerifyProviderSessionsZeroExtraPublicSiblingAbsence() error = nil, want live sibling drift failure")
	}
	if !strings.Contains(err.Error(), "drift") {
		t.Fatalf("VerifyProviderSessionsZeroExtraPublicSiblingAbsence() error = %v, want live sibling drift failure", err)
	}
}

func TestVerifyProviderSessionsINVDispositionBeyondServicePassesOnRepository(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	if err := ownershipinventory.VerifyProviderSessionsINVDispositionBeyondService(root); err != nil {
		t.Fatalf("VerifyProviderSessionsINVDispositionBeyondService() error = %v", err)
	}
}

func TestVerifyProviderSessionsINVDispositionBeyondServiceFailsWhenZeroExtraFlagDrifts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeProviderSessionsTopLevelInventoryFixture(t, root, providerSessionsTopLevelInventoryFixture{
		children: []ownershipinventory.ProviderSessionsTopLevelChild{
			{Directory: "internal", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
			{Directory: "service", Classification: ownershipinventory.ProviderSessionsTopLevelUnexpectedPublicSibling},
			{Directory: "surprise", Classification: ownershipinventory.ProviderSessionsTopLevelINVUnexpectedPublicSibling},
			{Directory: "transports", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
			{Directory: "wire", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
		},
		hasUnexpectedBeyondService: false,
	})
	for _, directory := range []string{"internal", "service", "surprise", "transports", "wire"} {
		mkdirAll(t, filepath.Join(root, "pkg/services/provider_sessions", directory))
	}

	err := ownershipinventory.VerifyProviderSessionsINVDispositionBeyondService(root)
	if err == nil {
		t.Fatal("VerifyProviderSessionsINVDispositionBeyondService() error = nil, want zero-extra drift failure")
	}
	if !strings.Contains(err.Error(), "zero-extra") && !strings.Contains(err.Error(), "hasUnexpectedPublicSiblingsBeyondService") {
		t.Fatalf("VerifyProviderSessionsINVDispositionBeyondService() error = %v, want zero-extra drift failure", err)
	}
}

func TestProviderSessionsUnexpectedPublicSiblingBeyondServicePackagePathsExcludeService(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	inventory, err := ownershipinventory.LoadProviderSessionsTopLevelInventory(root)
	if err != nil {
		t.Fatalf("LoadProviderSessionsTopLevelInventory() error = %v", err)
	}
	paths := ownershipinventory.ProviderSessionsUnexpectedPublicSiblingBeyondServicePackagePaths(inventory)
	for _, packagePath := range paths {
		if strings.HasSuffix(packagePath, "/service") {
			t.Fatalf("beyond-service package paths include service/: %v", paths)
		}
	}
	if len(paths) != 0 {
		t.Fatalf("beyond-service package paths = %v, want none on zero-extra INV disposition", paths)
	}
}
