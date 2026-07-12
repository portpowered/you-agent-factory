package configinit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
)

func TestInit_FreshHomeCreatesOperatorSystemConfig(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	result, err := Init(homeDir)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if result.SystemConfigOutcome != SystemConfigCreated {
		t.Fatalf("outcome = %q, want %q", result.SystemConfigOutcome, SystemConfigCreated)
	}

	configPath := defaultpaths.OperatorConfigPath(homeDir)
	if result.ConfigPath != configPath {
		t.Fatalf("configPath = %q, want %q", result.ConfigPath, configPath)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Stat(configPath): %v", err)
	}

	if _, err := operatorconfig.LoadFileConfig(configPath); err != nil {
		t.Fatalf("LoadFileConfig(created): %v", err)
	}

	scope, err := systemconfig.EnsureLocalBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope(created): %v", err)
	}
	if scope.Outcome != systemconfig.OutcomeReused {
		t.Fatalf("backend scope outcome = %q, want %q", scope.Outcome, systemconfig.OutcomeReused)
	}
}

func TestInit_ExistingConfigIsSkippedWithoutRewrite(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := defaultpaths.OperatorConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	original := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := Init(homeDir)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if result.SystemConfigOutcome != SystemConfigSkipped {
		t.Fatalf("outcome = %q, want %q", result.SystemConfigOutcome, SystemConfigSkipped)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("config contents changed:\n%s", string(got))
	}
}

func TestInit_RejectsEmptyHomeDir(t *testing.T) {
	t.Parallel()

	if _, err := Init("  "); err == nil {
		t.Fatal("expected error for empty home directory")
	}
}
