package canonicalledger_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	modulePrefix                      = "github.com/portpowered/infinite-you/"
	recordingsRootImport              = modulePrefix + "pkg/services/recordings"
	factoryDefinitionsRootImport      = modulePrefix + "pkg/services/factory_definitions"
	factoryDefinitionsContractsImport = modulePrefix + "pkg/services/factory_definitions/contracts"
	factoryRuntimeRootImport          = modulePrefix + "pkg/services/factory_runtime"
)

var eventVocabularyConsumerPackages = []string{
	modulePrefix + "pkg/services/recordings/internal/services/canonical_ledger/events/kinds",
}

var worldStateVocabularyConsumerPackages = []string{
	modulePrefix + "pkg/services/recordings/internal/services/projection_query/internal/service",
}

var replayVocabularyConsumerPackages = []string{
	modulePrefix + "pkg/services/recordings/internal",
}

func TestEventVocabularyConsumers_UseRecordingsRootNotDefinitionsContracts(t *testing.T) {
	t.Parallel()

	for _, packagePath := range eventVocabularyConsumerPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageImportsRecordingsRoot(t, packagePath)
			assertPackageDoesNotImport(t, packagePath, factoryDefinitionsContractsImport)
		})
	}
}

func TestWorldStateVocabularyConsumers_UseRecordingsRootNotDefinitionsContracts(t *testing.T) {
	t.Parallel()

	for _, packagePath := range worldStateVocabularyConsumerPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageImportsRecordingsRoot(t, packagePath)
			assertPackageDoesNotImport(t, packagePath, factoryDefinitionsContractsImport)
			assertPackageDoesNotImportFactoryDefinitionsRoot(t, packagePath)
		})
	}
}

func TestReplayVocabularyConsumers_UseRecordingsRootNotDefinitionsContracts(t *testing.T) {
	t.Parallel()

	for _, packagePath := range replayVocabularyConsumerPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageImportsRecordingsRoot(t, packagePath)
			assertPackageDoesNotImportContractsOnly(t, packagePath, factoryDefinitionsContractsImport)
		})
	}
}

func TestDispatchContractPublishedAtFactoryRuntimeRoot(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-f", "{{.GoFiles}}", factoryRuntimeRootImport)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list files for %s: %v\n%s", factoryRuntimeRootImport, err, output)
	}
	if !strings.Contains(string(output), "dispatch_contract.go") {
		t.Fatalf("%s must publish dispatch_contract.go at the Runtime root", factoryRuntimeRootImport)
	}
}

func assertPackageDoesNotImportFactoryDefinitionsRoot(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == factoryDefinitionsRootImport {
			t.Fatalf("%s must not import %s for world-state vocabulary; use %s", packagePath, factoryDefinitionsRootImport, recordingsRootImport)
		}
	}
}

func assertPackageImportsRecordingsRoot(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == recordingsRootImport {
			return
		}
	}
	t.Fatalf("%s must import %s for event envelope vocabulary", packagePath, recordingsRootImport)
}

func assertPackageDoesNotImportContractsOnly(t *testing.T, packagePath, forbiddenImport string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == forbiddenImport || strings.HasPrefix(importPath, forbiddenImport+"/") {
			t.Fatalf("%s must not import %s; use %s contracts", packagePath, forbiddenImport, recordingsRootImport)
		}
	}
}

func assertPackageDoesNotImport(t *testing.T, packagePath, forbiddenImport string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == forbiddenImport || strings.HasPrefix(importPath, forbiddenImport+"/") {
			t.Fatalf("%s must not import %s; use %s event contracts", packagePath, forbiddenImport, recordingsRootImport)
		}
		if importPath == factoryDefinitionsRootImport {
			t.Fatalf("%s must not import %s for event envelope vocabulary; use %s", packagePath, factoryDefinitionsRootImport, recordingsRootImport)
		}
	}
}
