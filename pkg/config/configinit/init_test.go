package configinit

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const legacyEncodedGoalMarkerFile = "legacy-encoded-sentinel.txt"

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

func TestInit_DoubleRunIsSuccessfulNoOp(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	first, err := Init(homeDir)
	if err != nil {
		t.Fatalf("first Init() error = %v", err)
	}
	if first.SystemConfigOutcome != SystemConfigCreated {
		t.Fatalf("first system config outcome = %q, want %q", first.SystemConfigOutcome, SystemConfigCreated)
	}

	configPath := defaultpaths.OperatorConfigPath(homeDir)
	configBefore, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config before rerun): %v", err)
	}

	second, err := Init(homeDir)
	if err != nil {
		t.Fatalf("second Init() error = %v", err)
	}
	if second.SystemConfigOutcome != SystemConfigSkipped {
		t.Fatalf("second system config outcome = %q, want %q", second.SystemConfigOutcome, SystemConfigSkipped)
	}

	configAfter, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config after rerun): %v", err)
	}
	if string(configAfter) != string(configBefore) {
		t.Fatalf("config changed on rerun:\nbefore:\n%s\nafter:\n%s", configBefore, configAfter)
	}

	if len(second.PackagedFactories) != len(first.PackagedFactories) {
		t.Fatalf("packaged factory count = %d, want %d", len(second.PackagedFactories), len(first.PackagedFactories))
	}
	for i, factory := range second.PackagedFactories {
		if factory.Outcome != PackagedFactorySkipped {
			t.Fatalf("second packagedFactories[%d].Outcome = %q, want %q", i, factory.Outcome, PackagedFactorySkipped)
		}
		if factory.FactoryDir != first.PackagedFactories[i].FactoryDir {
			t.Fatalf(
				"second packagedFactories[%d].FactoryDir = %q, want %q",
				i,
				factory.FactoryDir,
				first.PackagedFactories[i].FactoryDir,
			)
		}
	}
}

func TestInit_PreservesUserEditedFactoryFilesOnRerun(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	first, err := Init(homeDir)
	if err != nil {
		t.Fatalf("first Init() error = %v", err)
	}

	var goalDir string
	for _, factory := range first.PackagedFactories {
		if factory.Name == "@you/goal" {
			goalDir = factory.FactoryDir
			break
		}
	}
	if goalDir == "" {
		t.Fatal("expected @you/goal in packaged factory results")
	}

	workerPath := filepath.Join(goalDir, interfaces.WorkersDir, "goal-executor", interfaces.FactoryAgentsFileName)
	editedBody := "customer edited goal worker body\n"
	if err := os.WriteFile(workerPath, []byte(editedBody), 0o644); err != nil {
		t.Fatalf("WriteFile(edited worker): %v", err)
	}

	second, err := Init(homeDir)
	if err != nil {
		t.Fatalf("second Init() error = %v", err)
	}
	if second.SystemConfigOutcome != SystemConfigSkipped {
		t.Fatalf("second system config outcome = %q, want %q", second.SystemConfigOutcome, SystemConfigSkipped)
	}

	got, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("ReadFile(edited worker after rerun): %v", err)
	}
	if string(got) != editedBody {
		t.Fatalf("edited worker changed on rerun:\n%s", string(got))
	}
}

func TestInit_CreatesMissingPackagedDefaultsWithoutTouchingExisting(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	first, err := Init(homeDir)
	if err != nil {
		t.Fatalf("first Init() error = %v", err)
	}

	var goalDir string
	for _, factory := range first.PackagedFactories {
		if factory.Name == "@you/goal" {
			goalDir = factory.FactoryDir
			break
		}
	}
	if goalDir == "" {
		t.Fatal("expected @you/goal in packaged factory results")
	}

	workerPath := filepath.Join(goalDir, interfaces.WorkersDir, "goal-executor", interfaces.FactoryAgentsFileName)
	editedBody := "customer edited goal worker body for partial rerun\n"
	if err := os.WriteFile(workerPath, []byte(editedBody), 0o644); err != nil {
		t.Fatalf("WriteFile(edited worker): %v", err)
	}

	ttsDir, err := factoryconfig.MapNamedFactoryDir(defaultpaths.NamedFactoriesRoot(homeDir), "@you/tts")
	if err != nil {
		t.Fatalf("MapNamedFactoryDir(@you/tts): %v", err)
	}
	if err := os.RemoveAll(ttsDir); err != nil {
		t.Fatalf("RemoveAll(ttsDir): %v", err)
	}

	second, err := Init(homeDir)
	if err != nil {
		t.Fatalf("second Init() error = %v", err)
	}

	var ttsOutcome PackagedFactoryOutcome
	for _, factory := range second.PackagedFactories {
		switch factory.Name {
		case "@you/goal":
			if factory.Outcome != PackagedFactorySkipped {
				t.Fatalf("@you/goal outcome = %q, want %q", factory.Outcome, PackagedFactorySkipped)
			}
		case "@you/tts":
			ttsOutcome = factory.Outcome
			if factory.Outcome != PackagedFactoryCreated {
				t.Fatalf("@you/tts outcome = %q, want %q", factory.Outcome, PackagedFactoryCreated)
			}
		default:
			if factory.Outcome != PackagedFactorySkipped {
				t.Fatalf("%s outcome = %q, want %q", factory.Name, factory.Outcome, PackagedFactorySkipped)
			}
		}
	}
	if ttsOutcome != PackagedFactoryCreated {
		t.Fatalf("missing @you/tts outcome in second init results")
	}

	got, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("ReadFile(edited worker after partial rerun): %v", err)
	}
	if string(got) != editedBody {
		t.Fatalf("edited goal worker changed during partial rerun:\n%s", string(got))
	}

	if _, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(ttsDir, nil); err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(recreated @you/tts): %v", err)
	}
	if strings.Contains(ttsDir, "%2F") {
		t.Fatalf("recreated factory used encoded path %q", ttsDir)
	}
}

func TestInit_LeavesLegacyEncodedDirectoryUntouched(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(homeDir)
	if err := os.MkdirAll(namedFactoriesRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(namedFactoriesRoot): %v", err)
	}

	encodedDir := seedLegacyEncodedGoalFactory(t, namedFactoriesRoot)
	beforeSnapshot := snapshotDirectoryContents(t, encodedDir)

	result, err := Init(homeDir)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	assertDirectorySnapshotUnchanged(t, encodedDir, beforeSnapshot)

	wantGoalDir, err := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, "@you/goal")
	if err != nil {
		t.Fatalf("MapNamedFactoryDir(@you/goal): %v", err)
	}
	if wantGoalDir == encodedDir {
		t.Fatalf("hierarchical goal dir must not resolve to legacy encoded dir %q", encodedDir)
	}
	if strings.Contains(wantGoalDir, "%2F") {
		t.Fatalf("hierarchical goal dir must not use encoded path %q", wantGoalDir)
	}

	var goalResult *PackagedFactoryResult
	for i := range result.PackagedFactories {
		if result.PackagedFactories[i].Name == "@you/goal" {
			goalResult = &result.PackagedFactories[i]
			break
		}
	}
	if goalResult == nil {
		t.Fatal("expected @you/goal in packaged factory results")
	}
	if goalResult.FactoryDir != wantGoalDir {
		t.Fatalf("@you/goal factory dir = %q, want hierarchical %q", goalResult.FactoryDir, wantGoalDir)
	}
	if goalResult.Outcome != PackagedFactoryCreated {
		t.Fatalf("@you/goal outcome = %q, want %q", goalResult.Outcome, PackagedFactoryCreated)
	}
	if _, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(wantGoalDir, nil); err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(hierarchical @you/goal): %v", err)
	}
}

func seedLegacyEncodedGoalFactory(t *testing.T, factoriesRoot string) string {
	t.Helper()

	encodedSegment, err := factoryconfig.NamedFactoryNameToLayoutSegment("@you/goal")
	if err != nil {
		t.Fatalf("NamedFactoryNameToLayoutSegment(@you/goal): %v", err)
	}
	encodedDir := filepath.Join(factoriesRoot, encodedSegment)
	if err := os.MkdirAll(encodedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(encoded legacy dir): %v", err)
	}

	markerPath := filepath.Join(encodedDir, legacyEncodedGoalMarkerFile)
	if err := os.WriteFile(markerPath, []byte("do-not-touch-legacy-encoded-goal\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(legacy marker): %v", err)
	}
	return encodedDir
}

func snapshotDirectoryContents(t *testing.T, root string) map[string][]byte {
	t.Helper()

	snapshot := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotDirectoryContents(%s): %v", root, err)
	}
	return snapshot
}

func assertDirectorySnapshotUnchanged(t *testing.T, root string, before map[string][]byte) {
	t.Helper()

	after := snapshotDirectoryContents(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("directory %s changed after init:\nbefore=%#v\nafter=%#v", root, before, after)
	}
}
