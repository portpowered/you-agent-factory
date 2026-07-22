package config_init

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

var packagedFactoryNames = []string{
	"@you/deep-research",
	"@you/fusion",
	"@you/goal",
	"@you/quorum",
	"@you/review",
	"@you/subagent",
	"@you/tts",
}

type configInitResult struct {
	HomeDir             string                      `json:"homeDir"`
	ConfigPath          string                      `json:"configPath"`
	NamedFactoriesRoot  string                      `json:"namedFactoriesRoot"`
	SystemConfigOutcome string                      `json:"systemConfigOutcome"`
	PackagedFactories   []packagedFactoryInitResult `json:"packagedFactories"`
}

type packagedFactoryInitResult struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDirectory"`
	Outcome    string `json:"outcome"`
}

// observedFileSystem is a typed process-edge replacement. It delegates to the
// host filesystem while retaining the paths observed through the injected
// inspection role, so these tests never need service-owned path policy.
type observedFileSystem struct {
	platformfilesystem.Local
	inspected    []string
	missingPaths map[string]bool
}

func (files *observedFileSystem) Stat(path string) (fs.FileInfo, error) {
	path = filepath.Clean(path)
	files.inspected = append(files.inspected, path)
	if files.missingPaths[path] {
		return nil, fs.ErrNotExist
	}
	return files.Local.Stat(path)
}

func TestInit_FreshHomeCreatesSystemConfigAndReportsOutcome(t *testing.T) {
	homeDir := t.TempDir()
	var stdout bytes.Buffer
	files := &observedFileSystem{}

	err := executeConfigInit(t, files, homeDir, false, false, &stdout, io.Discard)
	if err != nil {
		t.Fatalf("Process.Execute() error = %v", err)
	}

	configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Stat(configPath): %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "Created system config at "+filepath.Clean(configPath)) {
		t.Fatalf("stdout = %q, want created system config message", got)
	}
	for _, name := range packagedFactoryNames {
		if !strings.Contains(got, "Created packaged factory "+name) {
			t.Fatalf("stdout = %q, want created packaged factory message for %q", got, name)
		}
	}
	if !containsPath(files.inspected, configPath) {
		t.Fatalf("injected filesystem inspections = %#v, want %q", files.inspected, configPath)
	}
}

func TestInit_JSONEmitsStructuredSummary(t *testing.T) {
	homeDir := t.TempDir()
	var stdout bytes.Buffer

	if err := executeConfigInit(t, &observedFileSystem{}, homeDir, true, false, &stdout, io.Discard); err != nil {
		t.Fatalf("Process.Execute() error = %v", err)
	}

	payload := decodeConfigInitResult(t, stdout.Bytes())
	if payload.SystemConfigOutcome != "created" {
		t.Fatalf("systemConfigOutcome = %q, want created", payload.SystemConfigOutcome)
	}
	if payload.HomeDir != homeDir {
		t.Fatalf("homeDir = %q, want %q", payload.HomeDir, homeDir)
	}
	if payload.ConfigPath != filepath.Join(homeDir, ".you-agent-factory", "config.json") {
		t.Fatalf("configPath = %q, want config below supplied home", payload.ConfigPath)
	}
	if payload.NamedFactoriesRoot != filepath.Join(homeDir, ".you-agent-factory", "factories") {
		t.Fatalf("namedFactoriesRoot = %q, want catalog below supplied home", payload.NamedFactoriesRoot)
	}
	if len(payload.PackagedFactories) != len(packagedFactoryNames) {
		t.Fatalf("packagedFactories count = %d, want %d", len(payload.PackagedFactories), len(packagedFactoryNames))
	}
	for i, factory := range payload.PackagedFactories {
		if factory.Name != packagedFactoryNames[i] {
			t.Fatalf("packagedFactories[%d].Name = %q, want %q", i, factory.Name, packagedFactoryNames[i])
		}
		if factory.Outcome != "created" {
			t.Fatalf("packagedFactories[%d].Outcome = %q, want created", i, factory.Outcome)
		}
		wantDir := filepath.Join(append([]string{payload.NamedFactoriesRoot}, strings.Split(factory.Name, "/")...)...)
		if factory.FactoryDir != wantDir {
			t.Fatalf("packagedFactories[%d].FactoryDir = %q, want %q", i, factory.FactoryDir, wantDir)
		}
		if _, err := os.Stat(filepath.Join(factory.FactoryDir, "factory.json")); err != nil {
			t.Fatalf("Stat(packagedFactories[%d] factory.json): %v", i, err)
		}
	}
}

func TestInit_ExistingConfigReportsSkippedWithoutRewrite(t *testing.T) {
	homeDir := t.TempDir()
	configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	original := []byte(`{"defaults":{"workerModelProvider":"codex"}}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	if err := executeConfigInit(t, &observedFileSystem{}, homeDir, false, false, &stdout, io.Discard); err != nil {
		t.Fatalf("Process.Execute() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "already present") {
		t.Fatalf("stdout = %q, want already-present message", got)
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("config changed:\n%s", string(got))
	}
}

func TestInit_UsesProvidedHomeDirWithoutReadingProcessHome(t *testing.T) {
	homeDir := t.TempDir()
	files := &observedFileSystem{}
	if err := executeConfigInit(t, files, homeDir, false, false, io.Discard, io.Discard); err != nil {
		t.Fatalf("Process.Execute() error = %v", err)
	}
	configPath := filepath.Join(homeDir, ".you-agent-factory", "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config under provided home: %v", err)
	}
	if !containsPath(files.inspected, configPath) {
		t.Fatalf("injected filesystem inspections = %#v, want supplied-home config path", files.inspected)
	}
}

func TestInit_DoubleRunReportsSkippedOutcomes(t *testing.T) {
	homeDir := t.TempDir()
	var stdout bytes.Buffer
	files := &observedFileSystem{}

	if err := executeConfigInit(t, files, homeDir, false, false, &stdout, io.Discard); err != nil {
		t.Fatalf("first Process.Execute() error = %v", err)
	}
	firstOut := stdout.String()
	if !strings.Contains(firstOut, "Created system config at") {
		t.Fatalf("first stdout = %q, want created system config message", firstOut)
	}

	stdout.Reset()
	if err := executeConfigInit(t, files, homeDir, false, false, &stdout, io.Discard); err != nil {
		t.Fatalf("second Process.Execute() error = %v", err)
	}
	secondOut := stdout.String()
	if !strings.Contains(secondOut, "System config already present at") {
		t.Fatalf("second stdout = %q, want already-present system config message", secondOut)
	}
	for _, name := range packagedFactoryNames {
		if !strings.Contains(secondOut, "Packaged factory "+name+" already present at") {
			t.Fatalf("second stdout = %q, want already-present packaged factory message for %q", secondOut, name)
		}
	}
}

func TestInit_DoubleRunJSONReportsSkippedOutcomes(t *testing.T) {
	homeDir := t.TempDir()
	var stdout bytes.Buffer
	files := &observedFileSystem{}

	if err := executeConfigInit(t, files, homeDir, true, false, &stdout, io.Discard); err != nil {
		t.Fatalf("first Process.Execute() error = %v", err)
	}

	stdout.Reset()
	if err := executeConfigInit(t, files, homeDir, true, false, &stdout, io.Discard); err != nil {
		t.Fatalf("second Process.Execute() error = %v", err)
	}

	payload := decodeConfigInitResult(t, stdout.Bytes())
	if payload.SystemConfigOutcome != "skipped" {
		t.Fatalf("systemConfigOutcome = %q, want skipped", payload.SystemConfigOutcome)
	}
	for i, factory := range payload.PackagedFactories {
		if factory.Outcome != "skipped" {
			t.Fatalf("packagedFactories[%d].Outcome = %q, want skipped", i, factory.Outcome)
		}
	}
}

func TestInit_ConfigCreationFailureSurfacesActionableCLIError(t *testing.T) {
	homeDir := t.TempDir()
	parentDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.WriteFile(parentDir, []byte("blocks config directory creation\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(parent blocker): %v", err)
	}

	var stderr bytes.Buffer
	legacyRoot := filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories")
	files := &observedFileSystem{missingPaths: map[string]bool{legacyRoot: true}}
	err := executeConfigInit(t, files, homeDir, false, true, io.Discard, &stderr)
	if err == nil {
		t.Fatal("expected config init failure")
	}
	got := err.Error()
	for _, want := range []string{
		"create system config at",
		"config.json",
		"is not a directory",
		"remove or rename",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("error = %q, want substring %q", got, want)
		}
	}
}

func TestInit_FactoryMaterializationFailureSurfacesActionableCLIError(t *testing.T) {
	homeDir := t.TempDir()
	namedFactoriesRoot := filepath.Join(homeDir, ".you-agent-factory", "factories")
	if err := os.MkdirAll(namedFactoriesRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(namedFactoriesRoot): %v", err)
	}
	blocker := filepath.Join(namedFactoriesRoot, "@you")
	if err := os.WriteFile(blocker, []byte("blocks hierarchical factory layout\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(factory scope blocker): %v", err)
	}

	err := executeConfigInit(t, &observedFileSystem{}, homeDir, false, false, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("expected factory materialization failure")
	}
	got := err.Error()
	for _, want := range []string{
		"install packaged factory",
		"@you/deep-research",
		namedFactoriesRoot,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("error = %q, want substring %q", got, want)
		}
	}
}

func executeConfigInit(
	t *testing.T,
	files *observedFileSystem,
	homeDir string,
	jsonOutput bool,
	verbose bool,
	stdout io.Writer,
	stderr io.Writer,
) error {
	t.Helper()

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		OperatorSettingsFileSystem:                      files,
		SystemInitializationInspectPath:                 files.Stat,
		SystemInitializationMigrationFileSystem:         files,
		FactoryDefinitionPortableFileSystem:             files,
		FactoryDefinitionLoadingFileSystem:              files,
		FactoryDefinitionVersionFileSystem:              files,
		FactoryDefinitionPackagedGoalPromptFileSystem:   files,
		FactoryDefinitionPersistenceFileSystem:          files,
		FactoryDefinitionNamedPathFileSystem:            files,
		FactoryDefinitionNamedFactoryCatalogFileSystem:  files,
		FactoryDefinitionPackagedInstallationFileSystem: files,
		FactoryDefinitionAuthoredReaderFileSystem:       files,
		FactoryDefinitionAuthoredWriterFileSystem:       files,
	})
	if err != nil {
		return err
	}

	args := []string{"you"}
	if jsonOutput {
		args = append(args, "--json")
	}
	if verbose {
		args = append(args, "--verbose")
	}
	args = append(args, "config", "init")
	return process.Execute(root.Input{
		Args: args,
		Env: append(os.Environ(),
			"HOME="+homeDir,
			"USERPROFILE="+homeDir,
		),
		Stdout:           stdout,
		Stderr:           stderr,
		Context:          t.Context(),
		WorkingDirectory: homeDir,
	})
}

func decodeConfigInitResult(t *testing.T, output []byte) configInitResult {
	t.Helper()
	var payload configInitResult
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatalf("Unmarshal stdout: %v\n%s", err, output)
	}
	return payload
}

func containsPath(paths []string, want string) bool {
	want = filepath.Clean(want)
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
