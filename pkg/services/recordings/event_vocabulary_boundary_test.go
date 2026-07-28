package recordings_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	recordingsRootImport          = "github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryDefinitionsRootImport  = "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryDefinitionsContractsImport = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
)

var eventVocabularyConsumerPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/recordings/events/kinds",
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
