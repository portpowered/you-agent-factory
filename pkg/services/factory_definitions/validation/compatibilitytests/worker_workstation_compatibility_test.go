package compatibilitytests

import (
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func taxonomyValidationBaseConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "story",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
	}
}

func TestValidate_WorkerWorkstationCompatibility_AcceptsCompatiblePairings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		workerType      string
		workstationType string
		workstationKind factorydefinitions.WorkstationKind
	}{
		{name: "inference taxonomy", workerType: factorydefinitions.WorkerTypeInference, workstationType: factorydefinitions.WorkstationTypeInference},
		{name: "agent taxonomy", workerType: factorydefinitions.WorkerTypeAgent, workstationType: factorydefinitions.WorkstationTypeAgent},
		{name: "script taxonomy", workerType: factorydefinitions.WorkerTypeScript, workstationType: factorydefinitions.WorkstationTypeScript},
		{name: "poller taxonomy", workerType: factorydefinitions.WorkerTypePoller, workstationType: factorydefinitions.WorkstationTypePoller, workstationKind: factorydefinitions.WorkstationKindPoller},
		{name: "legacy invoke pairing", workerType: factorydefinitions.WorkerTypeModel, workstationType: factorydefinitions.WorkstationTypeInvoke},
		{name: "legacy agent pairing", workerType: factorydefinitions.WorkerTypeModel, workstationType: factorydefinitions.WorkstationTypeModel},
		{name: "legacy poller pairing", workerType: factorydefinitions.WorkerTypeHosted, workstationKind: factorydefinitions.WorkstationKindPoller},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := taxonomyValidationBaseConfig()
			cfg.Workers = []workerconfig.Config{{Name: "executor", Type: tt.workerType}}
			cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
				Name:           "execute-story",
				Type:           tt.workstationType,
				Kind:           tt.workstationKind,
				WorkerTypeName: "executor",
				Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "init"}},
				Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
				OnFailure:      []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
			}}

			result := factoryvalidation.Validate(cfg)
			for _, target := range result.Targets {
				if target.Code == factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility {
					t.Fatalf("unexpected incompatible behavior target: %#v", target)
				}
			}
		})
	}
}

func TestValidate_WorkerWorkstationCompatibility_RejectsIncompatiblePairings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		workerType      string
		workstationType string
		workstationKind factorydefinitions.WorkstationKind
		wantWorkerLabel string
		wantRunLabel    string
	}{
		{
			name:            "agent run with inference worker",
			workerType:      factorydefinitions.WorkerTypeInference,
			workstationType: factorydefinitions.WorkstationTypeAgent,
			wantWorkerLabel: factorydefinitions.WorkerTypeInference,
			wantRunLabel:    factorydefinitions.WorkstationTypeAgent,
		},
		{
			name:            "inference run with agent worker",
			workerType:      factorydefinitions.WorkerTypeAgent,
			workstationType: factorydefinitions.WorkstationTypeInference,
			wantWorkerLabel: factorydefinitions.WorkerTypeAgent,
			wantRunLabel:    factorydefinitions.WorkstationTypeInference,
		},
		{
			name:            "poller run with inference worker",
			workerType:      factorydefinitions.WorkerTypeInference,
			workstationType: factorydefinitions.WorkstationTypePoller,
			workstationKind: factorydefinitions.WorkstationKindPoller,
			wantWorkerLabel: factorydefinitions.WorkerTypeInference,
			wantRunLabel:    factorydefinitions.WorkstationTypePoller,
		},
		{
			name:            "legacy agent run alias with model worker",
			workerType:      factorydefinitions.WorkerTypeModel,
			workstationType: factorydefinitions.WorkstationTypeAgent,
			wantWorkerLabel: factorydefinitions.WorkerTypeModel,
			wantRunLabel:    factorydefinitions.WorkstationTypeAgent,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := taxonomyValidationBaseConfig()
			cfg.Workers = []workerconfig.Config{{Name: "executor", Type: tt.workerType}}
			cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
				Name:           "execute-story",
				Type:           tt.workstationType,
				Kind:           tt.workstationKind,
				WorkerTypeName: "executor",
				Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "init"}},
				Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
				OnFailure:      []factorydefinitions.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
			}}

			result := factoryvalidation.Validate(cfg)
			validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility)
			target := findTargetByCode(t, result.Targets, factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility)
			if !strings.Contains(target.Message, tt.wantRunLabel) {
				t.Fatalf("message %q missing workstation type %q", target.Message, tt.wantRunLabel)
			}
			if !strings.Contains(target.Message, tt.wantWorkerLabel) {
				t.Fatalf("message %q missing worker type %q", target.Message, tt.wantWorkerLabel)
			}
			if !strings.Contains(target.Message, "inference") && !strings.Contains(target.Message, "agent") && !strings.Contains(target.Message, "poller") {
				t.Fatalf("message %q missing behavior terminology", target.Message)
			}
		})
	}
}

func TestValidateFactoryAPI_WorkerWorkstationCompatibilityRoundTrip(t *testing.T) {
	t.Parallel()

	generated, err := factorymapping.GeneratedFactoryFromOpenAPIJSON([]byte(`{
		"name":"taxonomy-compat-factory",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"executor","type":"AGENT_WORKER"}],
		"workstations":[{"name":"execute-story","type":"INFERENCE_RUN","worker":"executor","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}],"onFailure":[{"workType":"story","state":"failed"}]}]
	}`))
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}

	result, err := validationentry.ValidateFactoryAPI(t.Context(), generated, testFactoryDefinitionValidator())
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility)
}

func TestValidateFactoryAPI_PreservesMixedLegacyModelWorkerAcrossAgentAndInferenceWorkstations(t *testing.T) {
	t.Parallel()

	generated, err := factorymapping.GeneratedFactoryFromOpenAPIJSON([]byte(`{
		"name":"mixed-legacy-model-worker",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"executor","type":"MODEL_WORKER","model":"claude-sonnet","modelProvider":"CLAUDE"}],
		"workstations":[
			{"name":"execute-story","type":"MODEL_WORKSTATION","worker":"executor","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}],"onFailure":[{"workType":"story","state":"failed"}]},
			{"name":"invoke-story","type":"MODEL_INVOKE","worker":"executor","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}],"onFailure":[{"workType":"story","state":"failed"}]}
		]
	}`))
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	if generated.Workers == nil || len(*generated.Workers) != 1 {
		t.Fatalf("generated workers = %#v, want one worker", generated.Workers)
	}
	worker := (*generated.Workers)[0]
	if worker.Type == nil || string(*worker.Type) != factorydefinitions.WorkerTypeModel {
		t.Fatalf("generated worker type = %#v, want %s", worker.Type, factorydefinitions.WorkerTypeModel)
	}

	result, err := validationentry.ValidateFactoryAPI(t.Context(), generated, testFactoryDefinitionValidator())
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility {
			t.Fatalf("unexpected incompatible behavior target: %#v", target)
		}
	}

	runtimeCfg, err := factorymapping.FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	structuralResult := factoryvalidation.Validate(&runtimeCfg)
	for _, target := range structuralResult.Targets {
		if target.Code == factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility {
			t.Fatalf("unexpected structural incompatible behavior target: %#v", target)
		}
	}
}

func findTargetByCode(t *testing.T, targets []factoryvalidation.Target, code string) factoryvalidation.Target {
	t.Helper()
	for _, target := range targets {
		if target.Code == code {
			return target
		}
	}
	t.Fatalf("target with code %q not found in %#v", code, targets)
	return factoryvalidation.Target{}
}
