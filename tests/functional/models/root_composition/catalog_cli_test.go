package root_composition_test

import (
	"encoding/json"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestModelsCatalogCLIProjectsFactoryDiscoveryThroughRootBuildProcess proves
// the ordinary public Models CLI path observes the effective Factory catalog,
// preserves canonical ordering, and exposes the same readiness projection as
// the list and inspect API surfaces.
func TestModelsCatalogCLIProjectsFactoryDiscoveryThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	factoryDir := support.ScaffoldFactory(t, catalogDiscoveryFactoryConfig())
	server := characterizationStartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{},
	})
	t.Cleanup(func() { server.Stop(t) })

	listInputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", server.URL(), "models", "list",
	})
	if err := server.Execute(t, listInputs.Input); err != nil {
		t.Fatalf("Process.Execute(remote models list) error = %v\nstderr=%s", err, listInputs.Stderr())
	}
	var listed factoryapi.ListModelsResponse
	if err := json.Unmarshal([]byte(listInputs.Stdout()), &listed); err != nil {
		t.Fatalf("decode remote models list output: %v\n%s", err, listInputs.Stdout())
	}
	assertCatalogCLIOrdering(t, listed.Results)

	factoryModel, ok := findModelSummary(listed.Results, "OMNIVOICE_Q4_K_M")
	if !ok {
		t.Fatalf("models list omitted Factory model: %#v", listed.Results)
	}
	if factoryModel.ProviderLocality != factoryapi.WorkerModelLocalityCloud ||
		factoryModel.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		factoryModel.ManagedRuntime.Identity != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("models list Factory model = %#v, want CLOUD/READY with stable identity", factoryModel)
	}

	inspectInputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", server.URL(), "models", "inspect", "OMNIVOICE_Q4_K_M",
	})
	if err := server.Execute(t, inspectInputs.Input); err != nil {
		t.Fatalf("Process.Execute(remote models inspect) error = %v\nstderr=%s", err, inspectInputs.Stderr())
	}
	var detail factoryapi.ModelDetail
	if err := json.Unmarshal([]byte(inspectInputs.Stdout()), &detail); err != nil {
		t.Fatalf("decode remote models inspect output: %v\n%s", err, inspectInputs.Stdout())
	}
	if detail.Name != "OMNIVOICE_Q4_K_M" || len(detail.Operations) != 1 ||
		detail.Operations[0].Name != "TTS" ||
		detail.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		detail.ManagedRuntime.Identity != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("models inspect Factory model = %#v, want TTS and READY runtime", detail)
	}
	if detail.Diagnostics["workerCount"] != "1" ||
		detail.Diagnostics["workers"] != "tts-worker" ||
		detail.Diagnostics["statusReason"] == "" {
		t.Fatalf("models inspect diagnostics = %#v, want Factory worker projection", detail.Diagnostics)
	}

	builtIn, ok := findModelSummary(listed.Results, models.BuiltInModelNameLLM)
	if !ok {
		t.Fatalf("models list omitted built-in effective definition: %#v", listed.Results)
	}
	if builtIn.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING ||
		builtIn.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED {
		t.Fatalf("models list built-in runtime = %#v, want MISSING/NOT_INSTALLED baseline", builtIn.ManagedRuntime)
	}

	builtInInspectInputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "--server", server.URL(), "models", "inspect", models.BuiltInModelNameLLM,
	})
	if err := server.Execute(t, builtInInspectInputs.Input); err != nil {
		t.Fatalf("Process.Execute(remote models inspect built-in) error = %v\nstderr=%s", err, builtInInspectInputs.Stderr())
	}
	var builtInDetail factoryapi.ModelDetail
	if err := json.Unmarshal([]byte(builtInInspectInputs.Stdout()), &builtInDetail); err != nil {
		t.Fatalf("decode built-in models inspect output: %v\n%s", err, builtInInspectInputs.Stdout())
	}
	if builtInDetail.Diagnostics["catalogSource"] != "EFFECTIVE_DEFINITION" ||
		builtInDetail.Diagnostics["sourceKind"] != "HUGGING_FACE" ||
		builtInDetail.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING ||
		builtInDetail.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED {
		t.Fatalf("built-in models inspect = %#v, want effective definition readiness baseline", builtInDetail)
	}
}

func assertCatalogCLIOrdering(t *testing.T, results []factoryapi.ModelSummary) {
	t.Helper()
	for index := 1; index < len(results); index++ {
		if strings.ToLower(results[index-1].Name) > strings.ToLower(results[index].Name) {
			t.Fatalf("models list order = %#v, want canonical name ordering", results)
		}
	}
}
