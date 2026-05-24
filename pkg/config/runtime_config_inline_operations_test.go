package config

import (
	"reflect"
	"testing"
)

func TestLoadRuntimeConfig_PreservesInlineWorkerOperationsWithoutAgentsFiles(t *testing.T) {
	factoryDir := t.TempDir()

	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name":          "executor",
				"type":          "MODEL_WORKER",
				"model":         "claude-sonnet-4-20250514",
				"modelProvider": "CLAUDE",
				"body":          "You are the executor.",
				"operations": []map[string]any{
					{
						"name": "TTS",
						"inputs": []map[string]any{
							{"name": "text", "contentTypes": []string{"TEXT"}, "required": true},
						},
						"outputs": []map[string]any{
							{"name": "audio", "contentTypes": []string{"AUDIO"}},
						},
					},
				},
			},
		},
		"workstations": []map[string]any{
			{
				"name":    "execute-story",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
				"type":    "MODEL_WORKSTATION",
				"body":    "Implement {{ .WorkID }}.",
			},
		},
	})

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	workerDef, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected inline executor worker definition")
	}
	if len(workerDef.Operations) != 1 {
		t.Fatalf("expected one inline operation, got %#v", workerDef.Operations)
	}
	operation := workerDef.Operations[0]
	if operation.Name != "TTS" {
		t.Fatalf("expected TTS operation, got %#v", operation)
	}
	if len(operation.Inputs) != 1 || operation.Inputs[0].Name != "text" || !operation.Inputs[0].Required {
		t.Fatalf("expected text input slot to load intact, got %#v", operation.Inputs)
	}
	if !reflect.DeepEqual(operation.Inputs[0].ContentTypes, []string{"TEXT"}) {
		t.Fatalf("expected text input content types [TEXT], got %#v", operation.Inputs[0].ContentTypes)
	}
	if len(operation.Outputs) != 1 || operation.Outputs[0].Name != "audio" {
		t.Fatalf("expected audio output slot to load intact, got %#v", operation.Outputs)
	}
	if !reflect.DeepEqual(operation.Outputs[0].ContentTypes, []string{"AUDIO"}) {
		t.Fatalf("expected audio output content types [AUDIO], got %#v", operation.Outputs[0].ContentTypes)
	}
}
