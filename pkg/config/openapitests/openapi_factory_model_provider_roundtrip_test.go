package openapitests

import (
	"fmt"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestGeneratedFactoryFromOpenAPIJSON_ModelProviderRoundTripsAllSupportedPublicValues(t *testing.T) {
	cases := []struct {
		public   factoryapi.WorkerModelProvider
		internal interfaces.ModelProvider
	}{
		{factoryapi.WorkerModelProviderClaude, interfaces.ModelProviderClaude},
		{factoryapi.WorkerModelProviderCodex, interfaces.ModelProviderCodex},
		{factoryapi.WorkerModelProviderCursor, interfaces.ModelProviderCursor},
		{factoryapi.WorkerModelProviderGemini, interfaces.ModelProviderGemini},
		{factoryapi.WorkerModelProviderKiro, interfaces.ModelProviderKiro},
		{factoryapi.WorkerModelProviderOpenCode, interfaces.ModelProviderOpenCode},
	}

	for _, tc := range cases {
		t.Run(string(tc.public), func(t *testing.T) {
			generated, err := GeneratedFactoryFromOpenAPIJSON(factoryJSONWithModelProvider(string(tc.public)))
			if err != nil {
				t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
			}
			assertGeneratedWorkerModelProvider(t, generated, tc.public)

			cfg, err := FactoryConfigFromOpenAPI(generated)
			if err != nil {
				t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
			}
			if got := cfg.Workers[0].ModelProvider; got != string(tc.internal) {
				t.Fatalf("runtime modelProvider = %q, want %q", got, tc.internal)
			}

			public := WorkerConfigToOpenAPI(cfg.Workers[0])
			if public.ModelProvider == nil || *public.ModelProvider != tc.public {
				t.Fatalf("projected modelProvider = %#v, want %q", public.ModelProvider, tc.public)
			}
		})
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsUnknownWorkerModelProviderAtBoundary(t *testing.T) {
	assertGeneratedFactoryRejectsMisCasedEnumValue(
		t,
		"workers[0].modelProvider",
		"MYSTERY-PROVIDER",
		string(factoryJSONWithModelProvider("MYSTERY-PROVIDER")),
	)
}

func factoryJSONWithModelProvider(provider string) []byte {
	return []byte(fmt.Sprintf(`{
		"name":"model-provider-roundtrip-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","modelProvider":%q}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`, provider))
}

func assertGeneratedWorkerModelProvider(t *testing.T, generated factoryapi.Factory, want factoryapi.WorkerModelProvider) {
	t.Helper()

	if generated.Workers == nil || len(*generated.Workers) != 1 {
		t.Fatalf("expected one generated worker, got %#v", generated.Workers)
	}
	worker := (*generated.Workers)[0]
	if worker.ModelProvider == nil || *worker.ModelProvider != want {
		t.Fatalf("generated worker modelProvider = %#v, want %q", worker.ModelProvider, want)
	}
}
