package configinit

import (
	"os"
	"path/filepath"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
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

func TestInit_FreshHomeMaterializesPackagedDefaultFactories(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	result, err := Init(homeDir)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(homeDir)
	if result.NamedFactoriesRoot != namedFactoriesRoot {
		t.Fatalf("namedFactoriesRoot = %q, want %q", result.NamedFactoriesRoot, namedFactoriesRoot)
	}

	wantNames := factoryconfig.BuiltInNamedFactoryNames()
	if len(result.PackagedFactories) != len(wantNames) {
		t.Fatalf("packaged factory count = %d, want %d", len(result.PackagedFactories), len(wantNames))
	}

	projectRoot := filepath.Join(homeDir, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectRoot): %v", err)
	}

	for i, factory := range result.PackagedFactories {
		if factory.Name != wantNames[i] {
			t.Fatalf("packagedFactories[%d].Name = %q, want %q", i, factory.Name, wantNames[i])
		}
		if factory.Outcome != PackagedFactoryCreated {
			t.Fatalf("packagedFactories[%d].Outcome = %q, want %q", i, factory.Outcome, PackagedFactoryCreated)
		}

		wantDir, err := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, factory.Name)
		if err != nil {
			t.Fatalf("MapNamedFactoryDir(%q): %v", factory.Name, err)
		}
		if factory.FactoryDir != wantDir {
			t.Fatalf("packagedFactories[%d].FactoryDir = %q, want %q", i, factory.FactoryDir, wantDir)
		}

		resolution, err := factoryconfig.ResolveNamedFactoryAcrossRoots(projectRoot, namedFactoriesRoot, factory.Name)
		if err != nil {
			t.Fatalf("ResolveNamedFactoryAcrossRoots(%q): %v", factory.Name, err)
		}
		if resolution.FactoryDir != wantDir {
			t.Fatalf("ResolveNamedFactoryAcrossRoots(%q).FactoryDir = %q, want %q", factory.Name, resolution.FactoryDir, wantDir)
		}
		if _, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(resolution.FactoryDir, nil); err != nil {
			t.Fatalf("LoadRuntimeConfigFromFactoryDir(%q): %v", factory.Name, err)
		}
	}

	encodedSegment, err := factoryconfig.NamedFactoryNameToLayoutSegment("@you/goal")
	if err != nil {
		t.Fatalf("NamedFactoryNameToLayoutSegment(@you/goal): %v", err)
	}
	encodedDir := filepath.Join(namedFactoriesRoot, encodedSegment)
	if _, err := os.Stat(encodedDir); err == nil {
		t.Fatalf("expected fresh init not to create encoded factory dir %q", encodedDir)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(encodedDir): %v", err)
	}
}
