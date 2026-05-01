package support

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
)

func AgentFactoryPath(t *testing.T, rel string) string {
	t.Helper()
	return testutil.MustRepoPath(t, rel)
}

func LegacyFixtureDir(t *testing.T, name string) string {
	t.Helper()
	return testutil.MustRepoPath(t, filepath.Join("tests", "functional_test", "testdata", name))
}

func ClearSeedInputs(t *testing.T, dir string) {
	t.Helper()

	if err := os.RemoveAll(filepath.Join(dir, interfaces.InputsDir)); err != nil {
		t.Fatalf("clear seed inputs: %v", err)
	}
}

func WriteAgentConfig(t *testing.T, dir, workerName, content string) {
	t.Helper()

	path := filepath.Join(dir, "workers", workerName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create worker config dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func WriteWorkstationConfig(t *testing.T, dir, workstationName, content string) {
	t.Helper()

	path := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create workstation config dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func AssertArgsContainSequence(t *testing.T, args, want []string) {
	t.Helper()

	for i := 0; i <= len(args)-len(want); i++ {
		match := true
		for j := range want {
			if args[i+j] != want[j] {
				match = false
				break
			}
		}
		if match {
			return
		}
	}

	t.Fatalf("expected args %v to contain sequence %v", args, want)
}

func WriteWorkRequestFile(t *testing.T, path string, request interfaces.SubmitRequest) {
	t.Helper()

	workName := request.Name
	if workName == "" {
		workName = request.WorkID
	}
	if workName == "" {
		workName = "work-1"
	}
	data, err := json.Marshal(interfaces.WorkRequest{
		RequestID: request.RequestID,
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       workName,
			WorkID:     request.WorkID,
			RequestID:  request.RequestID,
			WorkTypeID: request.WorkTypeID,
			State:      request.TargetState,
			TraceID:    request.TraceID,
			Payload:    append([]byte(nil), request.Payload...),
			Tags:       request.Tags,
		}},
	})
	if err != nil {
		t.Fatalf("marshal work request file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write work request file: %v", err)
	}
}

func UpdateFactoryConfig(t *testing.T, dir string, mutate func(map[string]any)) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}

	mutate(cfg)

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}
