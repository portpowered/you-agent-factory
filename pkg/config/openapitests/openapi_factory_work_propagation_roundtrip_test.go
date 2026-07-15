package openapitests

import (
	"encoding/json"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func TestFactoryConfigFromOpenAPIJSON_MapsWorkPropagation(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"work-propagation-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"workPropagation":{"mode":"PRESERVE_INPUT"}
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Workstations[0].WorkPropagation == nil {
		t.Fatal("WorkPropagation = nil, want mapped policy")
	}
	if cfg.Workstations[0].WorkPropagation.Mode != interfaces.WorkPropagationModePreserveInput {
		t.Fatalf("WorkPropagation.Mode = %q, want %q", cfg.Workstations[0].WorkPropagation.Mode, interfaces.WorkPropagationModePreserveInput)
	}
}

func TestFactoryConfigToOpenAPI_PreservesWorkPropagation(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []workerconfig.Config{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute-story",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
			WorkPropagation: &interfaces.WorkPropagationConfig{
				Mode: interfaces.WorkPropagationModePreserveInput,
			},
		}},
	}

	generated := FactoryConfigToOpenAPI(cfg)
	workstation := requireSingleGeneratedWorkstation(t, generated)
	if workstation.WorkPropagation == nil {
		t.Fatal("expected generated workstation workPropagation")
	}
	if workstation.WorkPropagation.Mode != factoryapi.WorkPropagationModePreserveInput {
		t.Fatalf("generated workPropagation.mode = %#v, want %q", workstation.WorkPropagation.Mode, factoryapi.WorkPropagationModePreserveInput)
	}

	regenerated, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI round trip: %v", err)
	}
	if regenerated.Workstations[0].WorkPropagation == nil {
		t.Fatal("round-tripped WorkPropagation = nil, want preserved policy")
	}
	if regenerated.Workstations[0].WorkPropagation.Mode != interfaces.WorkPropagationModePreserveInput {
		t.Fatalf("round-tripped WorkPropagation.Mode = %q, want %q", regenerated.Workstations[0].WorkPropagation.Mode, interfaces.WorkPropagationModePreserveInput)
	}
}

func TestFactoryConfigToOpenAPI_OmitsUnsetWorkPropagation(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []workerconfig.Config{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute-story",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
		}},
	}

	generated := FactoryConfigToOpenAPI(cfg)
	workstation := requireSingleGeneratedWorkstation(t, generated)
	if workstation.WorkPropagation != nil {
		t.Fatalf("expected unset workPropagation to be omitted, got %#v", workstation.WorkPropagation)
	}

	generatedJSON, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("marshal generated factory boundary: %v", err)
	}
	var serialized struct {
		Workstations []map[string]any `json:"workstations"`
	}
	if err := json.Unmarshal(generatedJSON, &serialized); err != nil {
		t.Fatalf("unmarshal generated factory boundary JSON: %v", err)
	}
	if _, ok := serialized.Workstations[0]["workPropagation"]; ok {
		t.Fatalf("expected generated workstation JSON to omit workPropagation, got %#v", serialized.Workstations[0])
	}
}

func TestFactoryConfigMapper_FlattenRoundTripsWorkPropagation(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Name: "work-propagation-roundtrip-factory",
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []workerconfig.Config{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute-story",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
			WorkPropagation: &interfaces.WorkPropagationConfig{
				Mode: interfaces.WorkPropagationModeOutputAsPayload,
			},
		}},
	}

	mapper := NewFactoryConfigMapper()
	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	expanded, err := mapper.Expand(flattened)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if expanded.Workstations[0].WorkPropagation == nil {
		t.Fatal("expanded WorkPropagation = nil, want round-tripped policy")
	}
	if *expanded.Workstations[0].WorkPropagation != *cfg.Workstations[0].WorkPropagation {
		t.Fatalf("expanded WorkPropagation = %#v, want %#v", expanded.Workstations[0].WorkPropagation, cfg.Workstations[0].WorkPropagation)
	}
}

func TestFactoryConfigValidator_RejectsMalformedWorkPropagationObject(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"work-propagation-malformed-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"onFailure":[{"workType":"story","state":"failed"}],
			"workPropagation": {}
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Workstations[0].WorkPropagation == nil {
		t.Fatal("WorkPropagation = nil, want malformed authored object preserved for validation")
	}
	if cfg.Workstations[0].WorkPropagation.Mode != "" {
		t.Fatalf("WorkPropagation.Mode = %q, want empty mode for malformed object", cfg.Workstations[0].WorkPropagation.Mode)
	}

	result := NewConfigValidator().Validate(cfg)
	if !result.HasErrors() {
		t.Fatal("expected malformed workPropagation object to fail validation")
	}

	var matched bool
	for _, finding := range result.Errors() {
		if finding.Rule != factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode {
			continue
		}
		matched = true
		if finding.Path != "factory.workstations[0](execute-story).workPropagation.mode" {
			t.Fatalf("finding path = %q, want workstation workPropagation path", finding.Path)
		}
	}
	if !matched {
		t.Fatalf("expected %q finding, got %#v", factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode, result.Errors())
	}
}

func TestFactoryConfigValidator_RejectsUnsupportedWorkPropagationMode(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"work-propagation-validation-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"onFailure":[{"workType":"story","state":"failed"}],
			"workPropagation":{"mode":"preserve_input"}
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	result := NewConfigValidator().Validate(cfg)
	if !result.HasErrors() {
		t.Fatal("expected unsupported workPropagation.mode to fail validation")
	}

	var matched bool
	for _, finding := range result.Errors() {
		if finding.Rule != factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode {
			continue
		}
		matched = true
		if finding.Path != "factory.workstations[0](execute-story).workPropagation.mode" {
			t.Fatalf("finding path = %q, want workstation workPropagation path", finding.Path)
		}
	}
	if !matched {
		t.Fatalf("expected %q finding, got %#v", factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode, result.Errors())
	}
}
