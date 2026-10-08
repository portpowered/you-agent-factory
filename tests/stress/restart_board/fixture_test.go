package restart_board_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scaffoldBoard(t *testing.T, repo string) {
	t.Helper()
	dir := filepath.Join(repo, "factory")
	config := map[string]any{
		"name": "large-restart", "workers": []map[string]string{{"name": "worker-a"}},
		"workTypes": []map[string]any{{"name": "task", "states": []map[string]string{
			{"name": "init", "type": "INITIAL"}, {"name": "waiting", "type": "PROCESSING"},
			{"name": "processing", "type": "PROCESSING"}, {"name": "complete", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"},
		}}},
		"workstations": []map[string]any{{"name": "process", "worker": "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
	writeFile(t, filepath.Join(dir, "factory.json"), marshal(t, config))
	writeFile(t, filepath.Join(dir, "workers/worker-a/AGENTS.md"), []byte("---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n"))
	writeFile(t, filepath.Join(dir, "workstations/process/AGENTS.md"), []byte("---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n"))
}

func (h *boardHost) admit(t *testing.T, baseURL string) {
	t.Helper()
	works := make([]map[string]any, 0, boardSize)
	// 1,997 retained non-terminal Work, plus the representative three-Work DAG.
	for i := range boardSize - 3 {
		works = append(works, map[string]any{"workId": workID(i), "name": workID(i), "workTypeName": "task", "state": "waiting", "payload": strings.Repeat("payload § — ", 32), "tags": map[string]string{"witness": "§ —"}})
	}
	for _, w := range []struct{ id, state string }{{"A", "init"}, {"B", "waiting"}, {"C", "init"}} {
		works = append(works, map[string]any{"workId": w.id, "name": w.id, "workTypeName": "task", "state": w.state, "payload": "DAG § — " + w.id, "tags": map[string]string{"witness": "§ —"}})
	}
	body := marshal(t, map[string]any{"requestId": "large-board", "type": "FACTORY_REQUEST_BATCH", "works": works,
		"relations": []map[string]string{
			{"type": "DEPENDS_ON", "sourceWorkName": "B", "targetWorkName": "A", "requiredState": "complete"},
			{"type": "DEPENDS_ON", "sourceWorkName": "C", "targetWorkName": "B", "requiredState": "complete"},
		},
	})
	h.request(t, "PUT", baseURL+"/factory-sessions/~default/work-requests/large-board", body, nil)
}

func marshal(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func writeFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
}

func readJSONFile(t *testing.T, path string, result any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, result); err != nil {
		t.Fatal(err)
	}
}
