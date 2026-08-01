package root_composition_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestOperatorSettingsTransportActivationUsesRootProcess proves the public
// init transport mutates operator settings only through Process.Execute on the
// root-built application graph.
func TestOperatorSettingsTransportActivationUsesRootProcess(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir operator config directory: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"old-model"}}`), 0o600); err != nil {
		t.Fatalf("write operator config: %v", err)
	}

	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "init", "--provider", "claude", "--model", "root-process-model",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(init) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}

	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read updated operator config: %v", err)
	}
	var document struct {
		Defaults struct {
			WorkerModelProvider string `json:"workerModelProvider"`
			WorkerModel         string `json:"workerModel"`
		} `json:"defaults"`
	}
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode updated operator config: %v", err)
	}
	if document.Defaults.WorkerModelProvider != "claude" || document.Defaults.WorkerModel != "root-process-model" {
		t.Fatalf("updated defaults = %#v, want claude/root-process-model", document.Defaults)
	}
}
