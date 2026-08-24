package packagedfactorycatalog_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestTTSPackagedFactoryCharacterization(t *testing.T) {
	t.Parallel()

	factory := loadTTSPackagedFactory(t)
	checks := []struct {
		name  string
		check func(*testing.T, *factorydefinitions.FactoryConfig)
	}{
		{name: "identity and work states", check: assertTTSPackagedFactoryIdentity},
		{name: "model resource", check: assertTTSModelResource},
		{name: "model worker and TTS operation", check: assertTTSModelWorker},
		{name: "model workstation routing", check: assertTTSModelWorkstation},
	}
	for _, check := range checks {
		check := check
		t.Run(check.name, func(t *testing.T) {
			t.Parallel()
			check.check(t, factory)
		})
	}
}

func assertTTSPackagedFactoryIdentity(t *testing.T, factory *factorydefinitions.FactoryConfig) {
	t.Helper()
	if factory.Name != "@you/tts" || factory.Project != "builtin-tts" {
		t.Fatalf("identity = (%q, %q), want (@you/tts, builtin-tts)", factory.Name, factory.Project)
	}
	want := []factorydefinitions.WorkTypeConfig{{
		Name:             "task",
		HandlingBehavior: []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
		States: []factorydefinitions.StateConfig{
			{Name: "init", Type: factorydefinitions.StateTypeInitial},
			{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
			{Name: "failed", Type: factorydefinitions.StateTypeFailed},
		},
	}}
	if !reflect.DeepEqual(factory.WorkTypes, want) {
		t.Fatalf("work types = %#v, want %#v", factory.WorkTypes, want)
	}
}

func assertTTSModelResource(t *testing.T, factory *factorydefinitions.FactoryConfig) {
	t.Helper()
	want := []factorydefinitions.ResourceConfig{{
		Name:       "tts-cache",
		Type:       factorydefinitions.ResourceTypeModel,
		Capacity:   1,
		Model:      "tts",
		Backend:    "LOCALAI-VIBEVOICE",
		LoadPolicy: "ON_DEMAND",
	}}
	if !reflect.DeepEqual(factory.Resources, want) {
		t.Fatalf("resources = %#v, want %#v", factory.Resources, want)
	}
}

func assertTTSModelWorker(t *testing.T, factory *factorydefinitions.FactoryConfig) {
	t.Helper()
	if len(factory.Workers) != 1 {
		t.Fatalf("workers = %#v, want exactly one worker", factory.Workers)
	}
	worker := factory.Workers[0]
	if worker.Name != "tts-executor" ||
		worker.Type != factorydefinitions.WorkerTypeModel ||
		worker.Model != "tts" ||
		worker.ModelProvider != "codex" ||
		factorydefinitions.PublicWorkerModelProviderFromInternalRuntime(worker.ModelProvider) != "CODEX" ||
		worker.ModelLocality != factorydefinitions.ModelLocalityLocal ||
		worker.Command != "vibevoice-cpp" ||
		!reflect.DeepEqual(worker.Args, []string{"--grpc-endpoint", "http://127.0.0.1:50051"}) {
		t.Fatalf("worker identity/runtime = %#v", worker)
	}
	wantResources := []factorydefinitions.ResourceConfig{{Name: "tts-cache", Capacity: 1}}
	if !reflect.DeepEqual(worker.Resources, wantResources) {
		t.Fatalf("worker resources = %#v, want %#v", worker.Resources, wantResources)
	}
	wantOperations := []factorydefinitions.ModelOperation{{
		Name: "TTS",
		Inputs: []factorydefinitions.ModelOperationSlot{{
			Name:         "text",
			ContentTypes: []string{factorydefinitions.ModelOperationContentTypeText},
			Required:     true,
		}},
		Outputs: []factorydefinitions.ModelOperationSlot{{
			Name:         "audio",
			ContentTypes: []string{factorydefinitions.ModelOperationContentTypeAudio},
		}},
	}}
	if !reflect.DeepEqual(worker.Operations, wantOperations) {
		t.Fatalf("worker operations = %#v, want %#v", worker.Operations, wantOperations)
	}
}

func assertTTSModelWorkstation(t *testing.T, factory *factorydefinitions.FactoryConfig) {
	t.Helper()
	if len(factory.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want exactly one workstation", factory.Workstations)
	}
	workstation := factory.Workstations[0]
	if workstation.Name != "execute-tts" ||
		workstation.Type != factorydefinitions.WorkstationTypeInvoke ||
		workstation.WorkerTypeName != "tts-executor" ||
		workstation.Operation != "TTS" {
		t.Fatalf("workstation identity = %#v", workstation)
	}
	wantBindings := []factorydefinitions.ModelOperationBinding{{
		Slot:     "text",
		Selector: &factorydefinitions.ModelOperationBindingSelector{Type: factorydefinitions.ModelOperationContentTypeText},
	}}
	if !reflect.DeepEqual(workstation.OperationBindings, wantBindings) {
		t.Fatalf("operation bindings = %#v, want %#v", workstation.OperationBindings, wantBindings)
	}
	wantInputs := []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}}
	wantOutputs := []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "complete"}}
	wantFailure := []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "failed"}}
	if !reflect.DeepEqual(workstation.Inputs, wantInputs) ||
		!reflect.DeepEqual(workstation.Outputs, wantOutputs) ||
		!reflect.DeepEqual(workstation.OnFailure, wantFailure) {
		t.Fatalf(
			"routing = inputs %#v, outputs %#v, onFailure %#v; want %#v, %#v, %#v",
			workstation.Inputs,
			workstation.Outputs,
			workstation.OnFailure,
			wantInputs,
			wantOutputs,
			wantFailure,
		)
	}
}

func loadTTSPackagedFactory(t *testing.T) *factorydefinitions.FactoryConfig {
	t.Helper()

	inventory, err := packagedfactorycatalog.Discover(
		context.Background(),
		packagedfactories.Source(),
		"factories",
	)
	if err != nil {
		t.Fatalf("Discover packaged Factories: %v", err)
	}
	for _, entry := range inventory.Entries {
		if entry.Slug == "tts" {
			return entry.Factory
		}
	}
	t.Fatalf("packaged Factory inventory does not contain @you/tts")
	return nil
}
