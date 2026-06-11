package validationentry_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestRetiredRunnerMigration_LoadRuntimeConfigAndValidateFactoryAPIShareFieldPathLanguage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		mutate      func(map[string]any)
		wantMessage string
	}{
		{
			name: "factory runner",
			mutate: func(cfg map[string]any) {
				cfg["runner"] = "gemini"
			},
			wantMessage: "factory.runner is retired; use factory.modelProvider",
		},
		{
			name: "workstation runner",
			mutate: func(cfg map[string]any) {
				workstations := cfg["workstations"].([]map[string]any)
				workstations[0]["runner"] = "codex"
			},
			wantMessage: "workstations[0].runner is retired; use workstations[0].modelProvider",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload := retiredRunnerMigrationFactoryPayload()
			tc.mutate(payload)
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}

			cliErr := loadRuntimeConfigRetiredRunnerError(t, data)
			apiErr := generatedFactoryRetiredRunnerError(t, data)

			for label, got := range map[string]error{"cli": cliErr, "api": apiErr} {
				if got == nil {
					t.Fatalf("%s path: expected retired runner validation error", label)
				}
				if !strings.Contains(got.Error(), tc.wantMessage) {
					t.Fatalf("%s path error = %q, want message %q", label, got.Error(), tc.wantMessage)
				}
			}
		})
	}
}

func loadRuntimeConfigRetiredRunnerError(t *testing.T, data []byte) error {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
	_, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	return err
}

func generatedFactoryRetiredRunnerError(t *testing.T, data []byte) error {
	t.Helper()

	_, err := factoryconfig.GeneratedFactoryFromOpenAPIJSON(data)
	return err
}

func retiredRunnerMigrationFactoryPayload() map[string]any {
	return map[string]any{
		"name": "runner-migration",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "done", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name":               "w1",
				"type":               "MODEL_WORKER",
				"modelProvider":      "CODEX",
				"executorProvider":   "SCRIPT_WRAP",
			},
		},
		"workstations": []map[string]any{
			{
				"name":     "step",
				"worker":   "w1",
				"type":     "MODEL_WORKSTATION",
				"inputs":   []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":  []map[string]string{{"workType": "task", "state": "done"}},
			},
		},
	}
}
