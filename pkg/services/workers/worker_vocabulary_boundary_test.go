package workers_test

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	workersRootImport                   = "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryDefinitionsContractsImport = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
)

var workerVocabularyConsumerPackages = []string{
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal",
	"github.com/portpowered/infinite-you/pkg/transports/mapping/workerdiagnostics",
}

func TestWorkerVocabularyConsumers_UseWorkersRootNotDefinitionsContracts(t *testing.T) {
	t.Parallel()

	for _, packagePath := range workerVocabularyConsumerPackages {
		packagePath := packagePath
		t.Run(packagePath, func(t *testing.T) {
			t.Parallel()
			assertPackageImportsWorkersRoot(t, packagePath)
			assertPackageDoesNotImport(t, packagePath, factoryDefinitionsContractsImport)
		})
	}
}

func assertPackageImportsWorkersRoot(t *testing.T, packagePath string) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		if importPath == workersRootImport {
			return
		}
	}
	t.Fatalf("%s must import %s for worker execution vocabulary", packagePath, workersRootImport)
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
			t.Fatalf("%s must not import %s for worker execution vocabulary; use %s", packagePath, forbiddenImport, workersRootImport)
		}
	}
}
