package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func createReplacementWatchChannel(t *testing.T, factoryDir, workType, channel string) {
	t.Helper()

	inputDir := filepath.Join(factoryDir, interfaces.InputsDir, workType, channel)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create watched input dir %q: %v", inputDir, err)
	}
}

func writeNamedFactoryFixture(t *testing.T, rootDir, name string) string {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"name": name,
		"id":   name,
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name": "executor",
				"type": "MODEL_WORKER",
				"body": "You are the executor.",
			},
		},
		"workstations": []map[string]any{
			{
				"name":      "execute-" + name,
				"worker":    "executor",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
				"type":      "MODEL_WORKSTATION",
				"body":      "Implement {{ .WorkID }}.",
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(named factory fixture): %v", err)
	}

	factoryDir, err := config.PersistNamedFactory(rootDir, name, payload)
	if err != nil {
		t.Fatalf("PersistNamedFactory(%s): %v", name, err)
	}
	return factoryDir
}
