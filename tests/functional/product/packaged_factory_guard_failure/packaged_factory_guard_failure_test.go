package packaged_factory_guard_failure

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const existingOperatorConfig = `{
  "backendScopeId": "scope-existing",
  "defaults": {
    "workerModelProvider": "claude",
    "workerModel": "old-model"
  },
  "runtime": {
    "logging": {
      "directory": "custom-logs",
      "maxSizeMB": 12,
      "maxBackups": 3,
      "maxAgeDays": 7,
      "compress": true
    },
    "metrics": {
      "directory": "custom-metrics",
      "maxSizeMB": 14,
      "maxBackups": 4,
      "maxAgeDays": 8,
      "compress": false
    }
  },
  "workerPresets": [
    {
      "id": "review",
      "modelProvider": "codex",
      "model": "preset-model"
    }
  ]
}`

// TestInitUnknownPackagedFactoryFailsClosedWithCatalogInventory proves the public
// init command rejects unknown first-party packaged identities before install.
func TestInitUnknownPackagedFactoryFailsClosedWithCatalogInventory(t *testing.T) {
	homeDir := t.TempDir()
	workingDir := t.TempDir()
	configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config directory): %v", err)
	}
	if err := os.WriteFile(configPath, []byte(existingOperatorConfig), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	err = process.Execute(root.Input{
		Args: []string{
			"you", "init",
			"--package", "@you/missing",
			"--dir", filepath.Join(workingDir, "packaged-factories"),
		},
		Env: append(
			os.Environ(),
			"HOME="+homeDir,
			"USERPROFILE="+homeDir,
		),
		Stdin:            strings.NewReader(""),
		Stdout:           io.Discard,
		Stderr:           io.Discard,
		Context:          t.Context(),
		WorkingDirectory: workingDir,
	})
	if !errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("Process.Execute() error = %v, want ErrUnknownPackagedFactoryIdentity", err)
	}
	if !strings.Contains(err.Error(), "@you/goal") ||
		!strings.Contains(err.Error(), "@you/review") ||
		strings.Contains(err.Error(), "generated/") {
		t.Fatalf("init error = %q, want stable public inventory", err.Error())
	}
	if _, statErr := os.Stat(filepath.Join(workingDir, "packaged-factories", "@you", "missing")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unknown packaged factory directory exists or returned unexpected error: %v", statErr)
	}
}
