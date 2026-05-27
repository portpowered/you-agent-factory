package config

import (
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"testing"
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
		Name:      "speak",
		Type:      interfaces.WorkstationTypeInvoke,
		Operation: "TTS",
		OperationBindings: []interfaces.ModelOperationBinding{{
			Slot: "text",
			Selector: &interfaces.ModelOperationBindingSelector{
				Label: "utterance",
			},
		}},
		WorkerTypeName: "tts-worker",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
	}}

	findings := ruleModelInvokeWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %#v", findings)
	}
}

func TestRuleModelInvokeWorkstations_AcceptsCompatibleModelInvokeWorkstationAcrossWorkerLocality(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		worker interfaces.WorkerConfig
	}{
		{
			name: "local worker",
			worker: interfaces.WorkerConfig{
				Name:          "tts-worker",
				Type:          interfaces.WorkerTypeModel,
				Model:         "OMNIVOICE_Q4_K_M",
				ModelProvider: interfaces.RunnerIDCodex,
				ModelLocality: interfaces.ModelLocalityLocal,
				Operations: []interfaces.ModelOperation{{
					Name: "TTS",
					Inputs: []interfaces.ModelOperationSlot{
						{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeText}, Required: true},
						{Name: "voice", ContentTypes: []string{interfaces.ModelOperationContentTypeJSON}},
					},
					Outputs: []interfaces.ModelOperationSlot{{Name: "audio", ContentTypes: []string{interfaces.ModelOperationContentTypeAudio}}},
				}},
			},
		},
		{
			name: "cloud worker",
			worker: interfaces.WorkerConfig{
				Name:          "tts-worker",
				Type:          interfaces.WorkerTypeModel,
				Model:         "gpt-4o-mini-tts",
				ModelProvider: interfaces.RunnerIDCodex,
				ModelLocality: interfaces.ModelLocalityCloud,
				Operations: []interfaces.ModelOperation{{
					Name: "TTS",
					Inputs: []interfaces.ModelOperationSlot{
						{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeText}, Required: true},
						{Name: "voice", ContentTypes: []string{interfaces.ModelOperationContentTypeJSON}},
					},
					Outputs: []interfaces.ModelOperationSlot{{Name: "audio", ContentTypes: []string{interfaces.ModelOperationContentTypeAudio}}},
				}},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testBaseConfig()
			cfg.Workers = []interfaces.WorkerConfig{tt.worker}
			cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
				Name:           "speak",
				Type:           interfaces.WorkstationTypeInvoke,
				Operation:      "TTS",
				WorkerTypeName: "tts-worker",
				OperationBindings: []interfaces.ModelOperationBinding{
					{Slot: "text", Selector: &interfaces.ModelOperationBindingSelector{Label: "utterance"}},
					{Slot: "voice", Config: []interfaces.WorkContentPart{{Type: interfaces.WorkContentPartTypeJSON, JSON: []byte(`{"name":"alloy"}`)}}},
				},
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			}}

			findings := ruleModelInvokeWorkstations(cfg)
			if len(findings) != 0 {
				t.Fatalf("expected no findings, got %#v", findings)
			}
		})
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

func TestRuleModelInvokeWorkstations_RejectsDuplicateUnknownAndEmptyOperationBindings(t *testing.T) {
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
		OperationBindings: []interfaces.ModelOperationBinding{
			{Slot: "text", Selector: &interfaces.ModelOperationBindingSelector{Label: "utterance"}},
			{Slot: "text", Config: []interfaces.WorkContentPart{{Type: interfaces.WorkContentPartTypeText, Text: "fallback"}}},
			{Slot: "voice", Selector: &interfaces.ModelOperationBindingSelector{Role: "system"}},
			{Slot: "style"},
		},
	}}

	findings := ruleModelInvokeWorkstations(cfg)
	assertFindingMatch(t, findings, "workstation-model-invoke-binding-duplicate", "workstations[0](speak).operationBindings[1](text).slot", `duplicate operation binding for slot "text"`)
	assertFindingMatch(t, findings, "workstation-model-invoke-binding-unknown-slot", "workstations[0](speak).operationBindings[2](voice).slot", `unknown input slot "voice"`)
	assertFindingMatch(t, findings, "workstation-model-invoke-binding-empty", "workstations[0](speak).operationBindings[3](style)", "must declare a selector, config content, or default content")
}

func TestRuleWorkerModelOperations_RejectsDuplicateOperationAndSlotNames(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{
		Name:          "tts-worker",
		Type:          interfaces.WorkerTypeModel,
		ModelLocality: interfaces.ModelLocalityLocal,
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{
				{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeText}},
				{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeJSON}},
			},
			Outputs: []interfaces.ModelOperationSlot{{Name: "audio", ContentTypes: []string{interfaces.ModelOperationContentTypeAudio}}},
		}, {
			Name:    "TTS",
			Outputs: []interfaces.ModelOperationSlot{{Name: "audio", ContentTypes: []string{interfaces.ModelOperationContentTypeAudio}}},
		}},
	}}

	findings := ruleWorkerModelOperations(cfg)

	assertFindingMatch(t, findings, "worker-model-operation-duplicate", "workers[0](tts-worker).operations[1](TTS).name", `duplicate operation name "TTS"`)
	assertFindingMatch(t, findings, "worker-model-operation-slot-duplicate", "workers[0](tts-worker).operations[0](TTS).inputs[1](text).name", `duplicate input slot name "text"`)
}

func TestRuleWorkerModelOperations_RejectsCapabilityDeclarationsOnScriptWorkers(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{
		Name:          "scripted",
		Type:          interfaces.WorkerTypeScript,
		ModelLocality: interfaces.ModelLocalityCloud,
		Operations:    []interfaces.ModelOperation{{Name: "TTS"}},
	}}

	findings := ruleWorkerModelOperations(cfg)

	assertFindingMatch(t, findings, "worker-model-operation-worker-type", "workers[0](scripted)", "require worker type MODEL_WORKER")
}

func TestRuleWorkerModelOperations_RejectsMissingSlotContentTypes(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{
		Name: "tts-worker",
		Type: interfaces.WorkerTypeModel,
		Operations: []interfaces.ModelOperation{{
			Name:   "TTS",
			Inputs: []interfaces.ModelOperationSlot{{Name: "text"}},
		}},
	}}

	findings := ruleWorkerModelOperations(cfg)

	assertFindingMatch(t, findings, "worker-model-operation-slot-content-types", "workers[0](tts-worker).operations[0](TTS).inputs[0](text).contentTypes", "at least one content type is required")
}
