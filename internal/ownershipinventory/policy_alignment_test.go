package ownershipinventory_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestVerifyOperatorSettingsRootGoInventoryRejectsPolicyUnclassifiedArtifactRow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeOperatorSettingsRootGoInventoryFixture(t, root, operatorSettingsRootGoInventoryFixture{
		files: []ownershipinventory.OperatorSettingsRootGoFile{
			{File: "doc.go", Classification: ownershipinventory.OperatorSettingsRootGoThinContract},
			{File: "surprise.go", Classification: ownershipinventory.OperatorSettingsRootGoThinContract},
		},
	})
	for _, name := range []string{"doc.go", "surprise.go"} {
		writeFile(t, filepath.Join(root, "pkg/services/operator_settings", name), "package operatorsettings\n")
	}

	err := ownershipinventory.VerifyOperatorSettingsRootGoInventory(root)
	assertPolicyAlignmentFailure(t, err, "surprise.go")
}

func TestVerifyOperatorSettingsTopLevelInventoryRejectsPolicyUnclassifiedArtifactRow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeOperatorSettingsTopLevelInventoryFixture(t, root, operatorSettingsTopLevelInventoryFixture{
		children: []ownershipinventory.OperatorSettingsTopLevelChild{
			{Directory: "internal", Classification: ownershipinventory.OperatorSettingsTopLevelCanonicalRetain},
			{Directory: "surprise", Classification: ownershipinventory.OperatorSettingsTopLevelCanonicalRetain},
			{Directory: "transports", Classification: ownershipinventory.OperatorSettingsTopLevelCanonicalRetain},
			{Directory: "wire", Classification: ownershipinventory.OperatorSettingsTopLevelCanonicalRetain},
		},
	})
	for _, directory := range []string{"internal", "surprise", "transports", "wire"} {
		mkdirAll(t, filepath.Join(root, "pkg/services/operator_settings", directory))
	}

	err := ownershipinventory.VerifyOperatorSettingsTopLevelInventory(root)
	assertPolicyAlignmentFailure(t, err, "surprise")
}

func TestVerifyProviderSessionsRootGoInventoryRejectsPolicyUnclassifiedArtifactRow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeProviderSessionsRootGoInventoryFixture(t, root, providerSessionsRootGoInventoryFixture{
		files: []ownershipinventory.ProviderSessionsRootGoFile{
			{File: "contracts.go", Classification: ownershipinventory.ProviderSessionsRootGoThinContract},
			{File: "surprise.go", Classification: ownershipinventory.ProviderSessionsRootGoThinContract},
		},
	})
	for _, name := range []string{"contracts.go", "surprise.go"} {
		writeFile(t, filepath.Join(root, "pkg/services/provider_sessions", name), "package providersessions\n")
	}

	err := ownershipinventory.VerifyProviderSessionsRootGoInventory(root)
	assertPolicyAlignmentFailure(t, err, "surprise.go")
}

func TestVerifyProviderSessionsTopLevelInventoryRejectsPolicyUnclassifiedArtifactRow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeProviderSessionsTopLevelInventoryFixture(t, root, providerSessionsTopLevelInventoryFixture{
		children: []ownershipinventory.ProviderSessionsTopLevelChild{
			{Directory: "internal", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
			{Directory: "surprise", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
			{Directory: "transports", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
			{Directory: "wire", Classification: ownershipinventory.ProviderSessionsTopLevelCanonicalRetain},
		},
	})
	for _, directory := range []string{"internal", "surprise", "transports", "wire"} {
		mkdirAll(t, filepath.Join(root, "pkg/services/provider_sessions", directory))
	}

	err := ownershipinventory.VerifyProviderSessionsTopLevelInventory(root)
	assertPolicyAlignmentFailure(t, err, "surprise")
}

func assertPolicyAlignmentFailure(t *testing.T, err error, unit string) {
	t.Helper()
	if err == nil {
		t.Fatalf("verification error = nil, want production-policy failure for %q", unit)
	}
	if !strings.Contains(err.Error(), unit) || !strings.Contains(err.Error(), "production ownership policy") {
		t.Fatalf("verification error = %v, want named production-policy failure for %q", err, unit)
	}
}
