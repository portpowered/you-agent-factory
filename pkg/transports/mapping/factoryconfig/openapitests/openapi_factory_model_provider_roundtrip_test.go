package openapitests

import (
	"fmt"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	. "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestGeneratedFactoryFromOpenAPIJSON_ModelProviderRoundTripsAllSupportedPublicValues(t *testing.T) {
	cases := []struct {
		public   factoryapi.WorkerModelProvider
		internal modelprovider.Provider
	}{
		{factoryapi.WorkerModelProviderClaude, modelprovider.ProviderClaude},
		{factoryapi.WorkerModelProviderCodex, modelprovider.ProviderCodex},
		{factoryapi.WorkerModelProviderCursor, modelprovider.ProviderCursor},
		{factoryapi.WorkerModelProviderGemini, modelprovider.ProviderGemini},
		{factoryapi.WorkerModelProviderKiro, modelprovider.ProviderKiro},
		{factoryapi.WorkerModelProviderOpenCode, modelprovider.ProviderOpenCode},
		{factoryapi.WorkerModelProviderPi, modelprovider.ProviderPi},
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
	for _, malformed := range []string{
		"MYSTERY-PROVIDER",
		"customer_provider",
		"customer..provider",
		"customer.provider-",
	} {
		assertGeneratedFactoryRejectsMisCasedEnumValue(
			t,
			"workers[0].modelProvider",
			malformed,
			string(factoryJSONWithModelProvider(malformed)),
		)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_PreservesExtensionWorkerAndGuardProviders(t *testing.T) {
	const identity = "customer.provider-v2"
	generated, err := GeneratedFactoryFromOpenAPIJSON(factoryJSONWithModelProviderAndGuard(identity))
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	assertGeneratedWorkerModelProvider(t, generated, factoryapi.WorkerModelProvider(identity))
	if generated.Guards == nil || len(*generated.Guards) != 1 || (*generated.Guards)[0].ModelProvider != factoryapi.WorkerModelProvider(identity) {
		t.Fatalf("generated guard modelProvider = %#v, want %q", generated.Guards, identity)
	}

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if got := cfg.Workers[0].ModelProvider; got != identity {
		t.Fatalf("runtime worker modelProvider = %q, want %q", got, identity)
	}
	if got := cfg.Guards[0].ModelProvider; got != identity {
		t.Fatalf("runtime guard modelProvider = %q, want %q", got, identity)
	}

	projected, err := FactoryConfigToOpenAPI(&cfg)
	if err != nil {
		t.Fatalf("FactoryConfigToOpenAPI: %v", err)
	}
	if projected.Workers == nil || (*projected.Workers)[0].ModelProvider == nil || string(*(*projected.Workers)[0].ModelProvider) != identity {
		t.Fatalf("projected worker modelProvider = %#v, want %q", projected.Workers, identity)
	}
	if projected.Guards == nil || string((*projected.Guards)[0].ModelProvider) != identity {
		t.Fatalf("projected guard modelProvider = %#v, want %q", projected.Guards, identity)
	}
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

func factoryJSONWithModelProviderAndGuard(provider string) []byte {
	return []byte(fmt.Sprintf(`{
		"name":"extension-provider-roundtrip-factory",
		"guards":[{"type":"INFERENCE_THROTTLE_GUARD","modelProvider":%q,"refreshWindow":"1m"}],
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers":[{"name":"executor","type":"MODEL_WORKER","modelProvider":%q}],
		"workstations":[{
			"name":"execute-story",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`, provider, provider))
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
