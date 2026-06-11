package compatibilitytests

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factory/validationentry"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func taxonomyValidationBaseConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
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
		workstationKind interfaces.WorkstationKind
	}{
		{name: "inference taxonomy", workerType: interfaces.WorkerTypeInference, workstationType: interfaces.WorkstationTypeInference},
		{name: "agent taxonomy", workerType: interfaces.WorkerTypeAgent, workstationType: interfaces.WorkstationTypeAgent},
		{name: "script taxonomy", workerType: interfaces.WorkerTypeScript, workstationType: interfaces.WorkstationTypeScript},
		{name: "poller taxonomy", workerType: interfaces.WorkerTypePoller, workstationType: interfaces.WorkstationTypePoller, workstationKind: interfaces.WorkstationKindPoller},
		{name: "legacy invoke pairing", workerType: interfaces.WorkerTypeModel, workstationType: interfaces.WorkstationTypeInvoke},
		{name: "legacy agent pairing", workerType: interfaces.WorkerTypeModel, workstationType: interfaces.WorkstationTypeModel},
		{name: "legacy poller pairing", workerType: interfaces.WorkerTypeHosted, workstationKind: interfaces.WorkstationKindPoller},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := taxonomyValidationBaseConfig()
			cfg.Workers = []interfaces.WorkerConfig{{Name: "executor", Type: tt.workerType}}
			cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
				Name:           "execute-story",
				Type:           tt.workstationType,
				Kind:           tt.workstationKind,
				WorkerTypeName: "executor",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
				OnFailure:      []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
			}}

			result := factoryvalidation.Validate(cfg)
			for _, target := range result.Targets {
				if target.Code == factoryvalidation.CodeWorkerWorkstationIncompatibleBehavior {
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
		workstationKind interfaces.WorkstationKind
		wantWorkerLabel string
		wantRunLabel    string
	}{
		{
			name:            "agent run with inference worker",
			workerType:      interfaces.WorkerTypeInference,
			workstationType: interfaces.WorkstationTypeAgent,
			wantWorkerLabel: interfaces.WorkerTypeInference,
			wantRunLabel:    interfaces.WorkstationTypeAgent,
		},
		{
			name:            "inference run with agent worker",
			workerType:      interfaces.WorkerTypeAgent,
			workstationType: interfaces.WorkstationTypeInference,
			wantWorkerLabel: interfaces.WorkerTypeAgent,
			wantRunLabel:    interfaces.WorkstationTypeInference,
		},
		{
			name:            "poller run with inference worker",
			workerType:      interfaces.WorkerTypeInference,
			workstationType: interfaces.WorkstationTypePoller,
			workstationKind: interfaces.WorkstationKindPoller,
			wantWorkerLabel: interfaces.WorkerTypeInference,
			wantRunLabel:    interfaces.WorkstationTypePoller,
		},
		{
			name:            "legacy agent run alias with model worker",
			workerType:      interfaces.WorkerTypeModel,
			workstationType: interfaces.WorkstationTypeAgent,
			wantWorkerLabel: interfaces.WorkerTypeModel,
			wantRunLabel:    interfaces.WorkstationTypeAgent,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := taxonomyValidationBaseConfig()
			cfg.Workers = []interfaces.WorkerConfig{{Name: "executor", Type: tt.workerType}}
			cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
				Name:           "execute-story",
				Type:           tt.workstationType,
				Kind:           tt.workstationKind,
				WorkerTypeName: "executor",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
				OnFailure:      []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
			}}

			result := factoryvalidation.Validate(cfg)
			validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkerWorkstationIncompatibleBehavior)
			target := findTargetByCode(t, result.Targets, factoryvalidation.CodeWorkerWorkstationIncompatibleBehavior)
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

	generated, err := config.GeneratedFactoryFromOpenAPIJSON([]byte(`{
		"name":"taxonomy-compat-factory",
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"executor","type":"AGENT_WORKER"}],
		"workstations":[{"name":"execute-story","type":"INFERENCE_RUN","worker":"executor","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}],"onFailure":[{"workType":"story","state":"failed"}]}]
	}`))
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}

	result, err := validationentry.ValidateFactoryAPI(t.Context(), generated, factoryvalidation.Options{})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkerWorkstationIncompatibleBehavior)
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
