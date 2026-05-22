package config

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
