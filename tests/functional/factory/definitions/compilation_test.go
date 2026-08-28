package definitions

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

// TestFactoryDefinitionsCompileAuthoredAndCanonicalSources proves the public
// Factory Definitions root compiles an authored directory and its flattened
// canonical representation into the same effective customer-visible source.
func TestFactoryDefinitionsCompileAuthoredAndCanonicalSources(t *testing.T) {
	dir := support.ScaffoldFactory(t, compilationFactoryConfig())
	support.WriteAgentConfig(t, dir, compilationWorkerName, `---
type: SCRIPT_WORKER
command: go
args: ["test", "./..."]
---
Execute the story.
`)

	process := buildDefinitionsProcess(t)
	canonical, err := support.FlattenFactoryConfigWithProcessAndEnv(
		t,
		process,
		isolatedHomeEnvironment(t),
		dir,
	)
	if err != nil {
		t.Fatalf("Process.Execute(factory config flatten): %v", err)
	}
	if len(strings.TrimSpace(string(canonical))) == 0 {
		t.Fatal("flattened canonical Factory source is empty")
	}

	service := newFunctionalDefinitionsService(t)
	fromDirectory, err := service.CompileEffectiveFactorySource(
		t.Context(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{FactoryDir: dir},
	)
	if err != nil {
		t.Fatalf("Definitions.CompileEffectiveFactorySource(directory): %v", err)
	}
	fromCanonical, err := service.CompileEffectiveFactorySource(
		t.Context(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{
			Canonical:  canonical,
			FactoryDir: dir,
		},
	)
	if err != nil {
		t.Fatalf("Definitions.CompileEffectiveFactorySource(canonical): %v", err)
	}

	if fromDirectory.Effective.FactoryDir != dir ||
		fromDirectory.Effective.RuntimeBaseDir != dir {
		t.Fatalf(
			"directory effective identity = %#v, want FactoryDir and RuntimeBaseDir %q",
			fromDirectory.Effective,
			dir,
		)
	}
	if fromCanonical.Effective.FactoryDir != dir ||
		fromCanonical.Effective.RuntimeBaseDir != dir {
		t.Fatalf(
			"canonical effective identity = %#v, want FactoryDir and RuntimeBaseDir %q",
			fromCanonical.Effective,
			dir,
		)
	}
	if fromDirectory.Effective.ContentIdentity != fromCanonical.Effective.ContentIdentity {
		t.Fatalf(
			"equivalent Factory sources produced different effective identities:\n directory=%s\n canonical=%s",
			fromDirectory.Effective.ContentIdentity,
			fromCanonical.Effective.ContentIdentity,
		)
	}

	effective := decodeCompiledFactory(t, fromDirectory.Effective.ContentIdentity)
	if effective.Name != compilationFactoryName {
		t.Fatalf("effective Factory name = %q, want %q", effective.Name, compilationFactoryName)
	}
	worker, ok := support.FindFactoryWorker(effective, compilationWorkerName)
	if !ok {
		t.Fatalf("effective Factory is missing worker %q", compilationWorkerName)
	}
	if worker.Command == nil || *worker.Command != "go" || worker.Args == nil || len(*worker.Args) != 2 || (*worker.Args)[0] != "test" || (*worker.Args)[1] != "./..." {
		t.Fatalf("effective worker = %#v, want authored command and args", worker)
	}
	workstation, ok := support.FindFactoryWorkstation(effective, compilationWorkstationName)
	if !ok {
		t.Fatalf("effective Factory is missing workstation %q", compilationWorkstationName)
	}
	if workstation.Worker == nil || *workstation.Worker != compilationWorkerName {
		t.Fatalf("effective workstation worker = %v, want %q", workstation.Worker, compilationWorkerName)
	}
	if len(workstation.Inputs) != 1 || workstation.Inputs[0].WorkType != compilationWorkTypeName {
		t.Fatalf("effective workstation inputs = %#v, want one %q input; effective=%#v", workstation.Inputs, compilationWorkTypeName, effective)
	}

	assertCompileFailures(t, service)
}

func decodeCompiledFactory(t *testing.T, identity string) factoryapi.Factory {
	t.Helper()
	var effective factoryapi.Factory
	if err := json.Unmarshal([]byte(identity), &effective); err != nil {
		t.Fatalf("decode effective Factory identity: %v", err)
	}
	return effective
}

func assertCompileFailures(t *testing.T, service factorydefinitions.Service) {
	t.Helper()
	if _, err := service.CompileEffectiveFactorySource(
		t.Context(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{Canonical: []byte("{")},
	); !errors.Is(err, factorydefinitions.ErrInvalidAuthoredFactorySource) {
		t.Fatalf("invalid canonical compile error = %v, want ErrInvalidAuthoredFactorySource", err)
	}
	if _, err := service.CompileEffectiveFactorySource(
		t.Context(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{
			Canonical: []byte(`{"worker":"$unresolved"}`),
		},
	); !errors.Is(err, factorydefinitions.ErrUnresolvedDefinitionReference) {
		t.Fatalf("unresolved canonical compile error = %v, want ErrUnresolvedDefinitionReference", err)
	}
}

// TestFactoryDefinitionsRejectInvalidReferenceWithoutPersistence proves a
// customer-facing named Factory create rejects an unresolved worker reference
// before it creates or activates a durable Factory directory.
func TestFactoryDefinitionsRejectInvalidReferenceWithoutPersistence(t *testing.T) {
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
