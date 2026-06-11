package openapitests

import (
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFactoryConfigFromOpenAPIJSON_OpenCodeAgentRoundTripsWorkerAndWorkstation(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"opencode-agent-roundtrip-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{
			"name":"executor",
			"type":"MODEL_WORKER",
			"modelProvider":"OPENCODE",
			"openCodeAgent":"reviewer"
		}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			"modelProvider":"OPENCODE",
			"openCodeAgent":"implementer",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Workers[0].OpenCodeAgent != "reviewer" {
		t.Fatalf("worker openCodeAgent = %q, want reviewer", cfg.Workers[0].OpenCodeAgent)
	}
	if cfg.Workstations[0].OpenCodeAgent != "implementer" {
		t.Fatalf("workstation openCodeAgent = %q, want implementer", cfg.Workstations[0].OpenCodeAgent)
	}
	if got := cfg.Workstations[0].ModelProvider; got != string(interfaces.ModelProviderOpenCode) {
		t.Fatalf("workstation modelProvider = %q, want %q", got, interfaces.ModelProviderOpenCode)
	}

	publicWorker := WorkerConfigToOpenAPI(cfg.Workers[0])
	if publicWorker.OpenCodeAgent == nil || *publicWorker.OpenCodeAgent != "reviewer" {
		t.Fatalf("projected worker openCodeAgent = %#v, want reviewer", publicWorker.OpenCodeAgent)
	}
	publicWorkstation := WorkstationConfigToOpenAPI(cfg.Workstations[0])
	if publicWorkstation.OpenCodeAgent == nil || *publicWorkstation.OpenCodeAgent != "implementer" {
		t.Fatalf("projected workstation openCodeAgent = %#v, want implementer", publicWorkstation.OpenCodeAgent)
	}
	if publicWorkstation.ModelProvider == nil || *publicWorkstation.ModelProvider != "OPENCODE" {
		t.Fatalf("projected workstation modelProvider = %#v, want OPENCODE", publicWorkstation.ModelProvider)
	}
}

func TestFactoryConfigFromOpenAPIJSON_RejectsBlankOpenCodeAgentOnWorker(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"opencode-agent-invalid-worker-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","openCodeAgent":"   "}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	_, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected blank worker openCodeAgent validation error")
	}
	if !strings.Contains(err.Error(), "openCodeAgent") || !strings.Contains(err.Error(), "non-empty") {
		t.Fatalf("error = %v, want openCodeAgent non-empty validation", err)
	}
}

func TestFactoryConfigFromOpenAPIJSON_RejectsBlankOpenCodeAgentOnWorkstation(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"opencode-agent-invalid-workstation-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			"openCodeAgent":"",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	_, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected blank workstation openCodeAgent validation error")
	}
	if !strings.Contains(err.Error(), "openCodeAgent") || !strings.Contains(err.Error(), "non-empty") {
		t.Fatalf("error = %v, want openCodeAgent non-empty validation", err)
	}
}
