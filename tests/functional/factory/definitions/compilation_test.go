package definitions

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	compilationFactoryName     = "compilation-equivalence"
	compilationWorkerName      = "executor"
	compilationWorkstationName = "execute-story"
	compilationWorkTypeName    = "story"
	compilationInvalidFactory  = "compilation-invalid"
	compilationMissingWorker   = "missing-executor"
)

// TestFactoryDefinitionsRejectInvalidReferenceWithoutPersistence proves a
// customer-facing named Factory create rejects an unresolved worker reference
// before it creates or activates a durable Factory directory.
func TestFactoryDefinitionsRejectInvalidReferenceWithoutPersistence(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, compilationInvalidFactoryConfig())
	process := buildDefinitionsProcess(t)
	namedFactoriesRoot := filepath.Join(t.TempDir(), "factories")
	if err := os.MkdirAll(namedFactoriesRoot, 0o755); err != nil {
		t.Fatalf("create named Factory root: %v", err)
	}

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "factory", "create", compilationInvalidFactory,
		"--from", filepath.Join(dir, factorydefinitions.FactoryConfigFile),
		"--dir", namedFactoriesRoot,
	})
	inputs.Input.Env = isolatedHomeEnvironment(t)
	inputs.Input.WorkingDirectory = dir
	err := process.Execute(inputs.Input)
	if err == nil {
		t.Fatalf(
			"Process.Execute(factory create invalid reference) error = nil; stdout=%q stderr=%q",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	diagnostic := err.Error() + "\n" + inputs.Stdout() + "\n" + inputs.Stderr()
	for _, want := range []string{
		"invalid factory config",
		validationCodeDanglingWorkerReference,
		compilationMissingWorker,
	} {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("customer diagnostic = %q, want %q", diagnostic, want)
		}
	}
	if _, statErr := os.Stat(filepath.Join(namedFactoriesRoot, compilationInvalidFactory)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf(
			"invalid Factory target stat error = %v, want target directory to remain absent",
			statErr,
		)
	}
}

func compilationFactoryConfig() map[string]any {
	return map[string]any{
		"name": compilationFactoryName,
		"workTypes": []map[string]any{{
			"name": compilationWorkTypeName,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]string{{"name": compilationWorkerName}},
		"workstations": []map[string]any{{
			"name":    compilationWorkstationName,
			"worker":  compilationWorkerName,
			"inputs":  []map[string]string{{"workType": compilationWorkTypeName, "state": "init"}},
			"outputs": []map[string]string{{"workType": compilationWorkTypeName, "state": "complete"}},
		}},
	}
}

func compilationInvalidFactoryConfig() map[string]any {
	config := compilationFactoryConfig()
	config["name"] = compilationInvalidFactory
	config["workstations"] = []map[string]any{{
		"name":    compilationWorkstationName,
		"worker":  compilationMissingWorker,
		"inputs":  []map[string]string{{"workType": compilationWorkTypeName, "state": "init"}},
		"outputs": []map[string]string{{"workType": compilationWorkTypeName, "state": "complete"}},
	}}
	return config
}
