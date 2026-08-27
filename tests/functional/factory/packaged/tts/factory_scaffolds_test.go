package tts

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func scaffoldPackagedTTSLikeFactoryWithOptionalVoiceAndFormat(t *testing.T) string {
	t.Helper()
	return support.ScaffoldFactory(t, map[string]any{
		"name":                "tts",
		"invocationSignature": packagedTTSOptionalInvocationSignature(),
		"resources":           packagedTTSModelResource(),
		"workTypes":           packagedTTSTaskWorkTypes(),
		"workers":             packagedTTSOptionalWorkers(),
		"workstations":        packagedTTSOptionalWorkstations(),
	})
}

func packagedTTSOptionalInvocationSignature() map[string]any {
	return map[string]any{
		"parameters": []map[string]any{
			{
				"name":     "text",
				"required": true,
				"bindings": []map[string]any{
					{"kind": "POSITIONAL", "position": 1},
					{"kind": "STDIN"},
				},
			},
			{
				"name":         "voice",
				"externalName": "voice",
				"typeHint":     "STRING",
				"bindings":     []map[string]any{{"kind": "NAMED"}},
			},
			{
				"name":         "format",
				"externalName": "format",
				"typeHint":     "STRING",
				"bindings":     []map[string]any{{"kind": "NAMED"}},
			},
		},
	}
}

func packagedTTSModelResource() []map[string]any {
	return []map[string]any{{
		"name":       "omnivoice-cache",
		"type":       "MODEL",
		"capacity":   1,
		"model":      factorydefinitions.DefaultTTSModelName,
		"backend":    factorydefinitions.DefaultTTSBackendName,
		"loadPolicy": "ON_DEMAND",
	}}
}

func packagedTTSTaskWorkTypes() []map[string]any {
	return []map[string]any{{
		"name": "task",
		"handlingBehavior": []string{
			factorydefinitions.WorkTypeHandlingBehaviorDefault,
		},
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "complete", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	}}
}

func packagedTTSOptionalWorkers() []map[string]any {
	return []map[string]any{{
		"name":          "tts-executor",
		"type":          factorydefinitions.WorkerTypeInference,
		"model":         factorydefinitions.DefaultTTSModelName,
		"modelLocality": factorydefinitions.ModelLocalityCloud,
		"modelProvider": "CODEX",
		"operations": []map[string]any{{
			"name": "TTS",
			"inputs": []map[string]any{
				{"name": "text", "contentTypes": []string{"TEXT"}, "required": true},
				{"name": "voice", "contentTypes": []string{"JSON"}},
				{"name": "format", "contentTypes": []string{"JSON"}},
			},
			"outputs": []map[string]any{{
				"name": "audio", "contentTypes": []string{"AUDIO"},
			}},
		}},
	}}
}

func packagedTTSOptionalWorkstations() []map[string]any {
	return []map[string]any{{
		"name":      "execute-tts",
		"type":      "INFERENCE_RUN",
		"operation": "TTS",
		"worker":    "tts-executor",
		"operationBindings": []map[string]any{
			{"slot": "text", "selector": map[string]any{"type": "TEXT"}},
			{"slot": "voice", "config": []map[string]any{{
				"type": "JSON", "role": "voice", "json": map[string]any{"name": "${voice}"},
			}}},
			{"slot": "format", "config": []map[string]any{{
				"type": "JSON", "role": "format", "json": map[string]any{"name": "${format}"},
			}}},
		},
		"inputs": []map[string]string{{
			"workType": "task", "state": "init",
		}},
		"outputs": []map[string]string{{
			"workType": "task", "state": "complete",
		}},
		"onFailure": []map[string]string{{
			"workType": "task", "state": "failed",
		}},
	}}
}
