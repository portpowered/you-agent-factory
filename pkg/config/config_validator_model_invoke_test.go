package config

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestRuleModelInvokeWorkstations_AcceptsCompatibleModelInvokeWorkstation(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{
		Name: "tts-worker",
		Type: interfaces.WorkerTypeModel,
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{interfaces.ModelOperationContentTypeText},
				Required:     true,
			}},
			Outputs: []interfaces.ModelOperationSlot{{
				Name:         "audio",
				ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
			}},
		}},
	}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "speak",
		Type:           interfaces.WorkstationTypeInvoke,
		Operation:      "TTS",
		WorkerTypeName: "tts-worker",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
	}}

	findings := ruleModelInvokeWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %#v", findings)
	}
}

func TestRuleModelInvokeWorkstations_RejectsOperationOnNonModelInvokeType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "legacy",
		Type:           interfaces.WorkstationTypeModel,
		Operation:      "TTS",
		WorkerTypeName: "w1",
	}}

	findings := ruleModelInvokeWorkstations(cfg)
	assertFindingMatch(t, findings, "workstation-model-invoke-type", "workstations[0](legacy).operation", "only supported on MODEL_INVOKE")
}

func TestRuleModelInvokeWorkstations_RejectsWorkerCompatibilityAndOperationMismatch(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{
		{
			Name: "scripted",
			Type: interfaces.WorkerTypeScript,
		},
		{
			Name: "tts-worker",
			Type: interfaces.WorkerTypeModel,
			Operations: []interfaces.ModelOperation{{
				Name: "EMBED",
				Inputs: []interfaces.ModelOperationSlot{{
					Name:         "text",
					ContentTypes: []string{interfaces.ModelOperationContentTypeText},
				}},
				Outputs: []interfaces.ModelOperationSlot{{
					Name:         "vector",
					ContentTypes: []string{interfaces.ModelOperationContentTypeJSON},
				}},
			}},
		},
	}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{
		{
			Name:           "bad-worker-type",
			Type:           interfaces.WorkstationTypeInvoke,
			Operation:      "TTS",
			WorkerTypeName: "scripted",
		},
		{
			Name:           "bad-operation",
			Type:           interfaces.WorkstationTypeInvoke,
			Operation:      "TTS",
			WorkerTypeName: "tts-worker",
		},
	}

	findings := ruleModelInvokeWorkstations(cfg)
	assertFindingMatch(t, findings, "workstation-model-invoke-worker-compatibility", "workstations[0](bad-worker-type).worker", `worker "scripted" is incompatible`)
	assertFindingMatch(t, findings, "workstation-model-invoke-operation-mismatch", "workstations[1](bad-operation).operation", `does not declare requested operation "TTS"`)
}

func TestRuleModelInvokeWorkstations_RejectsIncompleteContentContract(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{
		Name: "tts-worker",
		Type: interfaces.WorkerTypeModel,
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{{
				Name:         "text",
				ContentTypes: []string{interfaces.ModelOperationContentTypeText},
			}},
		}},
	}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "speak",
		Type:           interfaces.WorkstationTypeInvoke,
		Operation:      "TTS",
		WorkerTypeName: "tts-worker",
	}}

	findings := ruleModelInvokeWorkstations(cfg)
	assertFindingMatch(t, findings, "workstation-model-invoke-content-contract", "workstations[0](speak).operation", "incompatible content contract")
}
