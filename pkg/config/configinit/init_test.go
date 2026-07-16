package configinit

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/namedfactorypath"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
	factorypackages "github.com/portpowered/infinite-you/pkg/factory/packages"
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
	assertBaselineClassifierWorkerPresets(t, configPath)

	scope, err := systemconfig.EnsureLocalBackendScope(configPath)
	if err != nil {
		t.Fatalf("EnsureLocalBackendScope(created): %v", err)
	}
	if scope.Outcome != systemconfig.OutcomeReused {
		t.Fatalf("backend scope outcome = %q, want %q", scope.Outcome, systemconfig.OutcomeReused)
	}
}

func assertBaselineClassifierWorkerPresets(t *testing.T, configPath string) {
	t.Helper()

	config, err := operatorconfig.LoadFileConfig(configPath)
	if err != nil {
		t.Fatalf("LoadFileConfig(%q): %v", configPath, err)
	}
	want := map[string]operatorconfig.WorkerPreset{}
	for _, preset := range operatorconfig.BaselineClassifierWorkerPresets() {
		want[preset.ID] = preset
	}
	if len(config.WorkerPresets) != len(want) {
		t.Fatalf("worker preset count = %d, want %d", len(config.WorkerPresets), len(want))
	}
	for _, preset := range config.WorkerPresets {
		if expected, ok := want[preset.ID]; !ok || preset != expected {
			t.Fatalf("worker preset %q = %#v, want %#v", preset.ID, preset, expected)
		}
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

func TestInit_RejectsExistingConfigDirectory(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := defaultpaths.OperatorConfigPath(homeDir)
	if err := os.MkdirAll(configPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(config path): %v", err)
	}

	_, err := Init(homeDir)
	if err == nil {
		t.Fatal("Init() succeeded with a directory at the operator config path")
	}
	for _, want := range []string{"read existing operator config", configPath} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err, want)
		}
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

	wantNames := factorypackages.Names()
	if len(result.PackagedFactories) != len(wantNames) {
		t.Fatalf("packaged factory count = %d, want %d", len(result.PackagedFactories), len(wantNames))
	}

	projectRoot := filepath.Join(homeDir, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectRoot): %v", err)
	}

	for i, factory := range result.PackagedFactories {
		assertFreshMaterializedPackagedFactory(t, projectRoot, namedFactoriesRoot, factory, wantNames[i], i)
	}
	assertNoEncodedGoalFactoryDir(t, namedFactoriesRoot)
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

func TestInit_PreservesAllCustomerOwnedFactoryFilesOnRerun(t *testing.T) {
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

	writeCustomerOwnedFactoryEdits(t, goalDir)
	beforeSnapshot := snapshotDirectoryContents(t, goalDir)

	second, err := Init(homeDir)
	if err != nil {
		t.Fatalf("second Init() error = %v", err)
	}
	if second.SystemConfigOutcome != SystemConfigSkipped {
		t.Fatalf("second system config outcome = %q, want %q", second.SystemConfigOutcome, SystemConfigSkipped)
	}

	assertDirectorySnapshotUnchanged(t, goalDir, beforeSnapshot)
}

func TestInit_SkipsValidPackageWithoutMaterializingInlineBundledFiles(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	first, err := Init(homeDir)
	if err != nil {
		t.Fatalf("first Init() error = %v", err)
	}

	goalDir := goalFactoryDir(t, first.PackagedFactories)
	bundledFilePath := writeCustomerEditedInlineBundledFile(t, goalDir)
	beforeBundledFileInfo, err := os.Stat(bundledFilePath)
	if err != nil {
		t.Fatalf("Stat(customer-edited bundled file before init): %v", err)
	}
	beforeSnapshot := snapshotDirectoryContents(t, goalDir)

	second, err := Init(homeDir)
	if err != nil {
		t.Fatalf("second Init() error = %v", err)
	}
	for _, factory := range second.PackagedFactories {
		if factory.Name == "@you/goal" && factory.Outcome != PackagedFactorySkipped {
			t.Fatalf("@you/goal outcome = %q, want %q", factory.Outcome, PackagedFactorySkipped)
		}
	}

	assertDirectorySnapshotUnchanged(t, goalDir, beforeSnapshot)
	afterBundledFileInfo, err := os.Stat(bundledFilePath)
	if err != nil {
		t.Fatalf("Stat(customer-edited bundled file after init): %v", err)
	}
	if !afterBundledFileInfo.ModTime().Equal(beforeBundledFileInfo.ModTime()) {
		t.Fatalf(
			"customer-edited bundled file modification time changed: before=%s after=%s",
			beforeBundledFileInfo.ModTime(),
			afterBundledFileInfo.ModTime(),
		)
	}
}

func TestInit_InvalidExistingPackagedFactoryFailsWithoutReplacement(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(homeDir)
	factoryName := factorypackages.Names()[0]
	factoryDir, err := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, factoryName)
	if err != nil {
		t.Fatalf("MapNamedFactoryDir(%q): %v", factoryName, err)
	}
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(invalid factory): %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDir, interfaces.FactoryConfigFile),
		[]byte("customer-owned invalid factory config\n"),
		0o640,
	); err != nil {
		t.Fatalf("WriteFile(invalid factory config): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "inspect-me.txt"), []byte("preserve for inspection\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(customer marker): %v", err)
	}
	beforeSnapshot := snapshotDirectoryContents(t, factoryDir)

	_, err = Init(homeDir)
	if err == nil {
		t.Fatal("expected invalid existing packaged factory to fail init")
	}
	for _, want := range []string{"install packaged factory", factoryName, "existing target", factoryDir, "invalid"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err, want)
		}
	}

	assertDirectorySnapshotUnchanged(t, factoryDir, beforeSnapshot)
}

func TestInit_RejectsExistingPackageWithBundledFileLinkEscapeWithoutWrites(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	first, err := Init(homeDir)
	if err != nil {
		t.Fatalf("first Init() error = %v", err)
	}

	goalDir := goalFactoryDir(t, first.PackagedFactories)
	writeCustomerEditedInlineBundledFile(t, goalDir)
	linkPath := filepath.Join(goalDir, "scripts")
	if err := os.RemoveAll(linkPath); err != nil {
		t.Fatalf("RemoveAll(bundled file directory): %v", err)
	}
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "customer-owned.txt"), []byte("preserve external content\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(external customer content): %v", err)
	}
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("Windows symlink privilege unavailable; junctions do not exercise symlink resolution semantics")
		}
		t.Fatalf("Symlink(%s -> %s): %v", linkPath, outsideDir, err)
	}

	beforeFactory := snapshotDirectoryContents(t, goalDir)
	beforeOutside := snapshotDirectoryContents(t, outsideDir)

	result, err := Init(homeDir)
	if err == nil {
		t.Fatal("expected filesystem-link-invalid packaged factory to fail init")
	}
	if len(result.PackagedFactories) != 0 {
		t.Fatalf("packaged factory results = %#v, want no skipped result on failure", result.PackagedFactories)
	}
	for _, want := range []string{
		"install packaged factory",
		"@you/goal",
		"existing target",
		"factory/scripts/customer-owned.sh",
		"cannot escape the expand target through filesystem links",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err, want)
		}
	}

	assertDirectorySnapshotUnchanged(t, goalDir, beforeFactory)
	assertDirectorySnapshotUnchanged(t, outsideDir, beforeOutside)
}

func TestInit_CreatesMissingPackagedDefaultsWithoutTouchingExisting(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	first, err := Init(homeDir)
	if err != nil {
		t.Fatalf("first Init() error = %v", err)
	}

	goalDir := goalFactoryDir(t, first.PackagedFactories)
	workerPath, editedBody := writeEditedGoalWorker(t, goalDir)

	ttsDir := removeMaterializedFactory(t, homeDir, "@you/tts")

	second, err := Init(homeDir)
	if err != nil {
		t.Fatalf("second Init() error = %v", err)
	}

	assertPartialRerunOutcomes(t, second.PackagedFactories)
	assertFileContentsUnchanged(t, workerPath, editedBody)
	assertRecreatedFactoryLoadable(t, ttsDir)
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

func assertFreshMaterializedPackagedFactory(
	t *testing.T,
	projectRoot string,
	namedFactoriesRoot string,
	factory PackagedFactoryResult,
	wantName string,
	index int,
) {
	t.Helper()

	if factory.Name != wantName {
		t.Fatalf("packagedFactories[%d].Name = %q, want %q", index, factory.Name, wantName)
	}
	if factory.Outcome != PackagedFactoryCreated {
		t.Fatalf("packagedFactories[%d].Outcome = %q, want %q", index, factory.Outcome, PackagedFactoryCreated)
	}

	wantDir, err := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, factory.Name)
	if err != nil {
		t.Fatalf("MapNamedFactoryDir(%q): %v", factory.Name, err)
	}
	if factory.FactoryDir != wantDir {
		t.Fatalf("packagedFactories[%d].FactoryDir = %q, want %q", index, factory.FactoryDir, wantDir)
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

func assertNoEncodedGoalFactoryDir(t *testing.T, namedFactoriesRoot string) {
	t.Helper()

	encodedSegment, err := namedfactorypath.LegacyLayoutSegment("@you/goal")
	if err != nil {
		t.Fatalf("LegacyLayoutSegment(@you/goal): %v", err)
	}
	encodedDir := filepath.Join(namedFactoriesRoot, encodedSegment)
	if _, err := os.Stat(encodedDir); err == nil {
		t.Fatalf("expected fresh init not to create encoded factory dir %q", encodedDir)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(encodedDir): %v", err)
	}
}

func goalFactoryDir(t *testing.T, factories []PackagedFactoryResult) string {
	t.Helper()

	for _, factory := range factories {
		if factory.Name == "@you/goal" {
			return factory.FactoryDir
		}
	}
	t.Fatal("expected @you/goal in packaged factory results")
	return ""
}

func writeEditedGoalWorker(t *testing.T, goalDir string) (string, string) {
	t.Helper()

	workerPath := filepath.Join(goalDir, interfaces.WorkersDir, "goal-executor", interfaces.FactoryAgentsFileName)
	editedBody := "customer edited goal worker body for partial rerun\n"
	if err := os.WriteFile(workerPath, []byte(editedBody), 0o644); err != nil {
		t.Fatalf("WriteFile(edited worker): %v", err)
	}
	return workerPath, editedBody
}

func writeCustomerOwnedFactoryEdits(t *testing.T, factoryDir string) {
	t.Helper()

	edits := map[string]string{
		interfaces.FactoryConfigFile: "\n ",
		filepath.Join(interfaces.WorkersDir, "goal-executor", interfaces.FactoryAgentsFileName): "customer edited goal worker body\n",
		filepath.Join(interfaces.WorkstationsDir, "execute-goal", "prompts", "executor.md"):     "customer edited goal workstation prompt\n",
		filepath.Join("scripts", "customer-hook.sh"):                                            "#!/bin/sh\necho customer-owned\n",
		"customer-notes.txt": "customer-owned package notes\n",
	}
	for relativePath, body := range edits {
		path := filepath.Join(factoryDir, relativePath)
		if relativePath == interfaces.FactoryConfigFile {
			existing, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", relativePath, err)
			}
			body = string(existing) + body
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", relativePath, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o640); err != nil {
			t.Fatalf("WriteFile(%s): %v", relativePath, err)
		}
	}
}

func writeCustomerEditedInlineBundledFile(t *testing.T, factoryDir string) string {
	t.Helper()

	configPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(factory config): %v", err)
	}
	var authored map[string]any
	if err := json.Unmarshal(payload, &authored); err != nil {
		t.Fatalf("Unmarshal(factory config): %v", err)
	}
	authored["supportingFiles"] = map[string]any{
		"bundledFiles": []any{map[string]any{
			"id":         "factory/scripts/customer-owned.sh",
			"type":       interfaces.BundledFileTypeScript,
			"targetPath": "factory/scripts/customer-owned.sh",
			"content": map[string]any{
				"encoding": interfaces.BundledFileEncodingUTF8,
				"inline":   "#!/bin/sh\necho catalog-content\n",
			},
		}},
	}
	payload, err = json.MarshalIndent(authored, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(factory config): %v", err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(configPath, payload, 0o644); err != nil {
		t.Fatalf("WriteFile(factory config): %v", err)
	}

	targetPath := filepath.Join(factoryDir, "scripts", "customer-owned.sh")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(bundled file parent): %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("#!/bin/sh\necho customer-edit\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(customer-edited bundled file): %v", err)
	}
	if err := os.Chmod(targetPath, 0o600); err != nil {
		t.Fatalf("Chmod(customer-edited bundled file): %v", err)
	}
	fixedModTime := time.Unix(1_700_000_000, 0).UTC()
	if err := os.Chtimes(targetPath, fixedModTime, fixedModTime); err != nil {
		t.Fatalf("Chtimes(customer-edited bundled file): %v", err)
	}
	return targetPath
}

func removeMaterializedFactory(t *testing.T, homeDir string, factoryName string) string {
	t.Helper()

	factoryDir, err := factoryconfig.MapNamedFactoryDir(defaultpaths.NamedFactoriesRoot(homeDir), factoryName)
	if err != nil {
		t.Fatalf("MapNamedFactoryDir(%s): %v", factoryName, err)
	}
	if err := os.RemoveAll(factoryDir); err != nil {
		t.Fatalf("RemoveAll(%s): %v", factoryName, err)
	}
	return factoryDir
}

func assertPartialRerunOutcomes(t *testing.T, factories []PackagedFactoryResult) {
	t.Helper()

	var ttsOutcome PackagedFactoryOutcome
	for _, factory := range factories {
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
}

func assertFileContentsUnchanged(t *testing.T, path string, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("file %s changed:\n%s", path, string(got))
	}
}

func assertRecreatedFactoryLoadable(t *testing.T, factoryDir string) {
	t.Helper()

	if _, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil); err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(%s): %v", factoryDir, err)
	}
	if strings.Contains(factoryDir, "%2F") {
		t.Fatalf("recreated factory used encoded path %q", factoryDir)
	}
}

func seedLegacyEncodedGoalFactory(t *testing.T, factoriesRoot string) string {
	t.Helper()

	encodedSegment, err := namedfactorypath.LegacyLayoutSegment("@you/goal")
	if err != nil {
		t.Fatalf("LegacyLayoutSegment(@you/goal): %v", err)
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

type directoryEntrySnapshot struct {
	Contents []byte
	Link     string
	Mode     fs.FileMode
	IsDir    bool
}

func snapshotDirectoryContents(t *testing.T, root string) map[string]directoryEntrySnapshot {
	t.Helper()

	snapshot := make(map[string]directoryEntrySnapshot)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		entrySnapshot := directoryEntrySnapshot{
			Mode:  info.Mode(),
			IsDir: entry.IsDir(),
		}
		if info.Mode()&os.ModeSymlink != 0 {
			entrySnapshot.Link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		} else if info.Mode().IsRegular() {
			entrySnapshot.Contents, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		snapshot[filepath.ToSlash(rel)] = entrySnapshot
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotDirectoryContents(%s): %v", root, err)
	}
	return snapshot
}

func assertDirectorySnapshotUnchanged(t *testing.T, root string, before map[string]directoryEntrySnapshot) {
	t.Helper()

	after := snapshotDirectoryContents(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("directory %s changed after init:\nbefore=%#v\nafter=%#v", root, before, after)
	}
}

func TestInit_ConfigCreationFailureReportsActionableError(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := defaultpaths.OperatorConfigPath(homeDir)
	parentDir := filepath.Dir(configPath)
	if err := os.WriteFile(parentDir, []byte("blocks config directory creation\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(parent blocker): %v", err)
	}

	_, err := Init(homeDir)
	if err == nil {
		t.Fatal("expected config init failure")
	}
	got := err.Error()
	for _, want := range []string{
		"create system config at",
		"config.json",
		".you-agent-factory",
		"is not a directory",
		"remove or rename",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("error = %q, want substring %q", got, want)
		}
	}
}

func TestInit_FactoryMaterializationFailureReportsActionableError(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(homeDir)
	if err := os.MkdirAll(namedFactoriesRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(namedFactoriesRoot): %v", err)
	}

	blocker := filepath.Join(namedFactoriesRoot, "@you")
	if err := os.WriteFile(blocker, []byte("blocks hierarchical factory layout\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(factory scope blocker): %v", err)
	}

	_, err := Init(homeDir)
	if err == nil {
		t.Fatal("expected factory materialization failure")
	}
	got := err.Error()
	for _, want := range []string{
		"install packaged factory",
		factorypackages.Names()[0],
		namedFactoriesRoot,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("error = %q, want substring %q", got, want)
		}
	}
}

func TestEnsurePackagedFactories_InvalidPayloadDoesNotCommitTarget(t *testing.T) {
	t.Parallel()

	namedFactoriesRoot := t.TempDir()
	definition := factorypackages.Definition{
		Name: "@test/invalid",
		JSON: []byte(`{"id":"invalid","workers":[`),
	}

	_, err := ensurePackagedFactories(namedFactoriesRoot, []factorypackages.Definition{definition})
	if err == nil {
		t.Fatal("expected invalid packaged factory payload to fail")
	}
	for _, want := range []string{"install packaged factory", definition.Name, namedFactoriesRoot} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err, want)
		}
	}

	targetDir, mapErr := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, definition.Name)
	if mapErr != nil {
		t.Fatalf("MapNamedFactoryDir(%q): %v", definition.Name, mapErr)
	}
	if _, statErr := os.Stat(targetDir); !os.IsNotExist(statErr) {
		t.Fatalf("Stat(%q) error = %v, want target absent", targetDir, statErr)
	}

	installed, listErr := factoryconfig.ListNamedFactories(namedFactoriesRoot)
	if listErr != nil {
		t.Fatalf("ListNamedFactories(%q): %v", namedFactoriesRoot, listErr)
	}
	if len(installed) != 0 {
		t.Fatalf("installed factories after failed install = %v, want none", installed)
	}
}

func TestEnsurePackagedFactories_InvalidScriptManifestDoesNotCommitTarget(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		files []any
	}{
		{
			name: "absolute target",
			files: []any{
				packagedScriptEntry("/tmp/setup.sh", "echo unsafe\n"),
			},
		},
		{
			name: "traversal target",
			files: []any{
				packagedScriptEntry("factory/scripts/../../outside.sh", "echo unsafe\n"),
			},
		},
		{
			name: "duplicate target",
			files: []any{
				packagedScriptEntry("factory/scripts/setup.sh", "echo first\n"),
				packagedScriptEntry("factory/scripts/setup.sh", "echo second\n"),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			namedFactoriesRoot := t.TempDir()
			definition := factorypackages.Definition{
				Name: "@you/goal",
				JSON: packagedGoalPayloadWithBundledFiles(t, tt.files),
			}

			_, err := ensurePackagedFactories(namedFactoriesRoot, []factorypackages.Definition{definition})
			if err == nil {
				t.Fatal("expected invalid packaged script manifest to fail")
			}
			for _, want := range []string{"install packaged factory", definition.Name, namedFactoriesRoot} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want context %q", err, want)
				}
			}

			targetDir, mapErr := factoryconfig.MapNamedFactoryDir(namedFactoriesRoot, definition.Name)
			if mapErr != nil {
				t.Fatalf("MapNamedFactoryDir(%q): %v", definition.Name, mapErr)
			}
			if _, statErr := os.Stat(targetDir); !os.IsNotExist(statErr) {
				t.Fatalf("Stat(%q) error = %v, want target absent", targetDir, statErr)
			}
		})
	}
}

func packagedGoalPayloadWithBundledFiles(t *testing.T, files []any) []byte {
	t.Helper()
	definition, ok := factorypackages.Lookup("@you/goal")
	if !ok {
		t.Fatal("lookup @you/goal packaged factory: not found")
	}
	var payload map[string]any
	if err := json.Unmarshal(definition.JSON, &payload); err != nil {
		t.Fatalf("decode @you/goal packaged factory: %v", err)
	}
	supportingFiles, _ := payload["supportingFiles"].(map[string]any)
	if supportingFiles == nil {
		supportingFiles = map[string]any{}
		payload["supportingFiles"] = supportingFiles
	}
	supportingFiles["bundledFiles"] = files
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode invalid packaged factory fixture: %v", err)
	}
	return encoded
}

func packagedScriptEntry(target, content string) map[string]any {
	return map[string]any{
		"id":         target,
		"type":       interfaces.BundledFileTypeScript,
		"targetPath": target,
		"content": map[string]any{
			"encoding": interfaces.BundledFileEncodingUTF8,
			"inline":   content,
		},
	}
}
