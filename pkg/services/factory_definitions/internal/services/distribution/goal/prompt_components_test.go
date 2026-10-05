package goal

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

type promptFileStub struct {
	path string
	body []byte
	err  error
}

func (f *promptFileStub) ReadFile(path string) ([]byte, error) {
	f.path = path
	return f.body, f.err
}

func TestPackagedGoalPromptRequiresFileSystem(t *testing.T) {
	if err := CheckPackagedGoalMaterializedPromptDrift(nil, "factory"); err == nil {
		t.Fatal("expected missing filesystem error")
	}
	if _, err := loadPackagedGoalRolePrompt(nil, "factory", PackagedGoalRolePromptSource{}); err == nil {
		t.Fatal("expected missing filesystem error when loading prompt")
	}
}

func TestPackagedGoalPromptComponents(t *testing.T) {
	var config packagedGoalPromptConfig
	if err := json.Unmarshal([]byte(`{"workers":[{"name":"worker","body":"worker prompt"}],"workstations":[{"name":"station","body":"station prompt"}]}`), &config); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		source     PackagedGoalRolePromptSource
		want, path string
	}{
		{"worker", PackagedGoalRolePromptSource{SourceKind: PackagedGoalRolePromptSourceKindWorkerBody, WorkerName: "worker"}, "worker prompt", filepath.Join("factory", "workers", "worker", "AGENTS.md")},
		{"station", PackagedGoalRolePromptSource{WorkstationName: "station", PromptFile: "prompts/task.md"}, "station prompt", filepath.Join("factory", "workstations", "station", "prompts/task.md")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := assembledPackagedGoalRolePrompt(&config, tc.source)
			if err != nil || body != tc.want {
				t.Fatalf("assembled prompt = %q, %v", body, err)
			}
			files := &promptFileStub{body: []byte(tc.want)}
			body, err = loadPackagedGoalRolePrompt(files, "factory", tc.source)
			if err != nil || body != tc.want || files.path != tc.path {
				t.Fatalf("loaded prompt = %q, %v; path = %q", body, err, files.path)
			}
			files.err = errors.New("unreadable prompt")
			if _, err := loadPackagedGoalRolePrompt(files, "factory", tc.source); !errors.Is(err, files.err) {
				t.Fatalf("read error = %v", err)
			}
		})
	}
	for _, source := range []PackagedGoalRolePromptSource{{SourceKind: PackagedGoalRolePromptSourceKindWorkerBody}, {WorkstationName: "missing"}} {
		if _, err := assembledPackagedGoalRolePrompt(&config, source); err == nil {
			t.Fatal("expected missing role error")
		}
	}
}
