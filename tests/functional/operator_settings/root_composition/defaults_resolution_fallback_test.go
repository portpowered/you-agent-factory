package root_composition_test

import (
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

// TestResolveFromHomeFallbackPreservesAcceptedSemantics proves defaults
// resolution through the published Operator Settings root contract preserves
// accepted environment-over-file semantics when adapter ownership is unset.
func TestResolveFromHomeFallbackPreservesAcceptedSemantics(t *testing.T) {
	operatorsettings.ConfigureDefaultsResolutionFromHome(nil)
	t.Cleanup(settingswire.RegisterDefaultsResolutionFromHome)

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(config dir): %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{
		"defaults": {
			"workerModelProvider": "claude",
			"workerModel": "file-model"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}
	t.Setenv(operatorsettings.EnvDefaultWorkerModelProvider, "codex")
	t.Setenv(operatorsettings.EnvDefaultWorkerModel, "env-model")

	resolved, err := operatorsettings.ResolveFromHomeWithEnvironment(
		platformfilesystem.Local{},
		globalconfigmapping.Decode,
		homeDir,
		operatorsettings.Defaults{
			WorkerModelProvider: os.Getenv(operatorsettings.EnvDefaultWorkerModelProvider),
			WorkerModel:         os.Getenv(operatorsettings.EnvDefaultWorkerModel),
		},
		operatorsettings.FlagOverrides{},
	)
	if err != nil {
		t.Fatalf("ResolveFromHomeWithEnvironment() error = %v", err)
	}
	if resolved.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX from fallback path", resolved.WorkerModelProvider)
	}
	if resolved.WorkerModel != "env-model" {
		t.Fatalf("model = %q, want env-model", resolved.WorkerModel)
	}
	if resolved.ConfigPath != configPath {
		t.Fatalf("config path = %q, want %q", resolved.ConfigPath, configPath)
	}
}
