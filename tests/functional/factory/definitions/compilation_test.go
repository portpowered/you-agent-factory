package definitions

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Read-only definition commands use the shared process and scenario-owned paths
// and streams; no Current Factory or live Factory Session is needed.
func TestFactoryConfigReadbackMergesAuthoredWorkerAndWorkstation(t *testing.T) {
	t.Parallel()
	config := compilationFactoryConfig()
	config["workstations"].([]map[string]any)[0]["stopWords"] = []string{"CANONICAL"}
	dir := support.ScaffoldFactory(t, config)
	for _, file := range []struct{ path, contents string }{
		{filepath.Join("workers", compilationWorkerName, "AGENTS.md"), "---\ntype: SCRIPT_WORKER\ncommand: go\nargs: [\"test\", \"./...\"]\n---\nRun tests.\n"},
		{filepath.Join("workstations", compilationWorkstationName, "AGENTS.md"), "---\ntype: MODEL_WORKSTATION\nworker: executor\nstopWords: [\"RUNTIME\"]\n---\nImplement the story.\n"},
	} {
		path := filepath.Join(dir, file.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(file.contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	process, environment := buildDefinitionsProcess(t), isolatedHomeEnvironment(t)
	fromDirectory, err := support.FlattenFactoryConfigWithProcessAndEnv(t, process, environment, dir)
	if err != nil {
		t.Fatal(err)
	}
	fromFile, err := support.FlattenFactoryConfigWithProcessAndEnv(t, process, environment, filepath.Join(dir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	var directoryValue, fileValue any
	if err := json.Unmarshal(fromDirectory, &directoryValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fromFile, &fileValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(directoryValue, fileValue) {
		t.Fatalf("directory and file readback differ: %s versus %s", fromDirectory, fromFile)
	}
	loaded, err := support.DecodeFactoryDefinition(fromDirectory)
	if err != nil {
		t.Fatal(err)
	}
	worker, ok := support.FindFactoryWorker(loaded, compilationWorkerName)
	if !ok || worker.Command == nil || *worker.Command != "go" || stringValue(worker.Body) != "Run tests." {
		t.Fatalf("loaded worker = %#v; want authored command and body", worker)
	}
	workstation, ok := support.FindFactoryWorkstation(loaded, compilationWorkstationName)
	if !ok || stringValue(workstation.Worker) != compilationWorkerName || stringValue(workstation.Body) != "Implement the story." || workstation.StopWords == nil || !reflect.DeepEqual(*workstation.StopWords, []string{"CANONICAL", "RUNTIME"}) {
		t.Fatalf("loaded workstation = %#v; want authored binding/body and ordered canonical/runtime stop words", workstation)
	}
}

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
