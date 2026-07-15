package promptassets

import (
	"encoding/json"
	"reflect"
	"testing"
	"testing/fstest"
)

func TestAssemble_ResolvesDeclaredPromptAssetsExactly(t *testing.T) {
	const (
		workerPrompt      = "\n  worker prompt with preserved whitespace  \n"
		workstationPrompt = "workstation prompt without final newline"
	)

	tests := []struct {
		name            string
		factoryJSON     string
		wantWorker      string
		wantWorkstation string
		wantPromptFile  bool
	}{
		{
			name:        "worker only",
			factoryJSON: `{"workers":[{"name":"author","promptFile":"prompts/worker.md"}]}`,
			wantWorker:  workerPrompt,
		},
		{
			name:            "workstation only",
			factoryJSON:     `{"workstations":[{"name":"draft","promptFile":"prompts/workstation.md"}]}`,
			wantWorkstation: workstationPrompt,
			wantPromptFile:  true,
		},
		{
			name:            "worker and workstation",
			factoryJSON:     `{"workers":[{"name":"author","promptFile":"prompts/worker.md"}],"workstations":[{"name":"draft","promptFile":"prompts/workstation.md"}]}`,
			wantWorker:      workerPrompt,
			wantWorkstation: workstationPrompt,
			wantPromptFile:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := Definition{
				Package:     "@you/example",
				FactoryJSON: []byte(tt.factoryJSON),
				Assets: fstest.MapFS{
					"assets/prompts/worker.md":      {Data: []byte(workerPrompt)},
					"assets/prompts/workstation.md": {Data: []byte(workstationPrompt)},
				},
				AssetRoot: "assets",
			}

			assembled, err := Assemble(definition)
			if err != nil {
				t.Fatalf("Assemble: %v", err)
			}
			root := decodePayload(t, assembled)

			if tt.wantWorker != "" {
				worker := firstSubject(t, root, workerCollection)
				if got := worker["body"]; got != tt.wantWorker {
					t.Fatalf("worker body = %q, want exact authored content %q", got, tt.wantWorker)
				}
				if _, exists := worker["promptFile"]; exists {
					t.Fatal("assembled worker retained package-only promptFile metadata")
				}
			}
			if tt.wantWorkstation != "" {
				workstation := firstSubject(t, root, workstationCollection)
				if got := workstation["body"]; got != tt.wantWorkstation {
					t.Fatalf("workstation body = %q, want exact authored content %q", got, tt.wantWorkstation)
				}
				_, hasPromptFile := workstation["promptFile"]
				if hasPromptFile != tt.wantPromptFile {
					t.Fatalf("workstation promptFile present = %v, want %v", hasPromptFile, tt.wantPromptFile)
				}
			}
		})
	}
}

func TestAssemble_IsDeterministicAndDoesNotMutateDefinition(t *testing.T) {
	original := []byte(`{
  "workers": [{"name":"author","promptFile":"prompt.md"}],
  "workstations": [{"name":"draft","promptFile":"prompt.md"}]
}`)
	definition := Definition{
		Package:     "@you/example",
		FactoryJSON: append([]byte(nil), original...),
		Assets:      fstest.MapFS{"prompt.md": {Data: []byte("exact prompt\n")}},
	}

	first, err := Assemble(definition)
	if err != nil {
		t.Fatalf("first Assemble: %v", err)
	}
	second, err := Assemble(definition)
	if err != nil {
		t.Fatalf("second Assemble: %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("repeated assembly differs:\nfirst:  %s\nsecond: %s", first, second)
	}
	if !reflect.DeepEqual(decodePayload(t, first), decodePayload(t, second)) {
		t.Fatal("repeated assembly produced semantically different payloads")
	}
	if string(definition.FactoryJSON) != string(original) {
		t.Fatal("assembly mutated the authored factory definition")
	}
}

func decodePayload(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		t.Fatalf("decode assembled payload: %v", err)
	}
	return root
}

func firstSubject(t *testing.T, root map[string]any, collection string) map[string]any {
	t.Helper()
	entries, ok := root[collection].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("%s = %#v, want one subject", collection, root[collection])
	}
	subject, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("%s[0] = %#v, want object", collection, entries[0])
	}
	return subject
}
