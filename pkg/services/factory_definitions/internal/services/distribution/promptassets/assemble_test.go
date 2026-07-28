package promptassets

import (
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"strings"
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

func TestAssemble_RejectsInvalidPromptDeclarations(t *testing.T) {
	const packageName = "@you/example"
	for _, tt := range invalidPromptDeclarationCases(packageName) {
		t.Run(tt.name, func(t *testing.T) {
			assembled, err := Assemble(Definition{
				Package:     packageName,
				FactoryJSON: []byte(tt.factoryJSON),
				Assets:      tt.assets,
				AssetRoot:   "assets",
			})
			if err == nil {
				t.Fatalf("Assemble() error = nil, want rejection; payload = %s", assembled)
			}
			if assembled != nil {
				t.Fatalf("Assemble() payload = %s, want nil on failure", assembled)
			}
			if tt.name == "missing asset" && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("Assemble() error = %v, want wrapped fs.ErrNotExist", err)
			}
			for _, fragment := range tt.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("Assemble() error = %q, want fragment %q", err, fragment)
				}
			}
			if strings.Contains(err.Error(), "inline secret") {
				t.Fatalf("Assemble() error exposed prompt contents: %q", err)
			}
		})
	}
}

type invalidPromptDeclarationCase struct {
	name        string
	factoryJSON string
	assets      fs.FS
	want        []string
}

func invalidPromptDeclarationCases(packageName string) []invalidPromptDeclarationCase {
	return []invalidPromptDeclarationCase{
		{
			name:        "missing asset",
			factoryJSON: `{"workers":[{"name":"author","promptFile":"prompts/missing.md"}]}`,
			assets:      fstest.MapFS{},
			want:        []string{packageName, "worker", "author", "prompts/missing.md", "read asset"},
		},
		{
			name:        "unreadable asset",
			factoryJSON: `{"workstations":[{"name":"draft","promptFile":"prompts"}]}`,
			assets: fstest.MapFS{
				"assets/prompts": {Mode: fs.ModeDir},
			},
			want: []string{packageName, "workstation", "draft", "prompts", "read asset"},
		},
		{
			name:        "absolute path",
			factoryJSON: `{"workers":[{"name":"author","promptFile":"/private/prompt.md"}]}`,
			assets:      fstest.MapFS{},
			want:        []string{packageName, "worker", "author", "/private/prompt.md", "package-relative"},
		},
		{
			name:        "path escapes asset root",
			factoryJSON: `{"workstations":[{"name":"draft","promptFile":"prompts/../../secret.md"}]}`,
			assets:      fstest.MapFS{},
			want:        []string{packageName, "workstation", "draft", "prompts/../../secret.md", "escapes"},
		},
		{
			name:        "worker has inline and file prompt",
			factoryJSON: `{"workers":[{"name":"author","body":"inline secret","promptFile":"prompts/worker.md"}]}`,
			assets:      fstest.MapFS{},
			want:        []string{packageName, "worker", "author", "prompts/worker.md", "both promptFile"},
		},
		{
			name:        "workstation has inline and file prompt",
			factoryJSON: `{"workstations":[{"name":"draft","body":"inline secret","promptFile":"prompts/workstation.md"}]}`,
			assets:      fstest.MapFS{},
			want:        []string{packageName, "workstation", "draft", "prompts/workstation.md", "both promptFile"},
		},
		{
			name:        "prompt subject has no name",
			factoryJSON: `{"workers":[{"promptFile":"prompts/worker.md"}]}`,
			assets:      fstest.MapFS{},
			want:        []string{packageName, "worker", "<unnamed>", "prompts/worker.md", "name must"},
		},
		{
			name:        "prompt path is not a string",
			factoryJSON: `{"workers":[{"name":"author","promptFile":42}]}`,
			assets:      fstest.MapFS{},
			want:        []string{packageName, "worker", "author", "<non-string>", "path must be a string"},
		},
		{
			name:        "prompt path is empty",
			factoryJSON: `{"workers":[{"name":"author","promptFile":""}]}`,
			assets:      fstest.MapFS{},
			want:        []string{packageName, "worker", "author", `prompt ""`, "path must be non-empty"},
		},
		{
			name:        "inline prompt is not a string",
			factoryJSON: `{"workers":[{"name":"author","body":42,"promptFile":"prompts/worker.md"}]}`,
			assets:      fstest.MapFS{},
			want:        []string{packageName, "worker", "author", "prompts/worker.md", "body must be a string"},
		},
		{
			name:        "worker collection is malformed",
			factoryJSON: `{"workers":{"name":"author"}}`,
			assets:      fstest.MapFS{},
			want:        []string{packageName, "workers must be an array"},
		},
		{
			name:        "workstation entry is malformed",
			factoryJSON: `{"workstations":["draft"]}`,
			assets:      fstest.MapFS{},
			want:        []string{packageName, "workstation at index 0 must be an object"},
		},
		{
			name:        "non-prompt subject has invalid name",
			factoryJSON: `{"workers":[{}]}`,
			assets:      fstest.MapFS{},
			want:        []string{packageName, "worker name must be a non-empty string"},
		},
	}
}

func TestAssemble_StopsBeforeReadingLaterPromptAfterFailure(t *testing.T) {
	assets := &countingFS{FS: fstest.MapFS{
		"assets/prompts/valid.md": {Data: []byte("valid prompt")},
	}}
	assembled, err := Assemble(Definition{
		Package: "@you/example",
		FactoryJSON: []byte(`{"workers":[
			{"name":"invalid","promptFile":"../outside.md"},
			{"name":"later","promptFile":"prompts/valid.md"}
		]}`),
		Assets:    assets,
		AssetRoot: "assets",
	})
	if err == nil {
		t.Fatal("Assemble() error = nil, want rejection")
	}
	if assembled != nil {
		t.Fatalf("Assemble() payload = %s, want nil on failure", assembled)
	}
	if assets.opens != 0 {
		t.Fatalf("asset opens = %d, want no reads after validation failure", assets.opens)
	}
}

type countingFS struct {
	fs.FS
	opens int
}

func (f *countingFS) Open(name string) (fs.File, error) {
	f.opens++
	if f.FS == nil {
		return nil, errors.New("unexpected asset read")
	}
	return f.FS.Open(name)
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
