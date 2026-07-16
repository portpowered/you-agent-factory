package configinitcmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	factorypackages "github.com/portpowered/infinite-you/pkg/factory/packages"
)

func TestInit_FreshHomeCreatesSystemConfigAndReportsOutcome(t *testing.T) {
	homeDir := t.TempDir()
	var stdout bytes.Buffer

	err := Init(InitConfig{
		HomeDir: homeDir,
		Output:  &stdout,
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	configPath := defaultpaths.OperatorConfigPath(homeDir)
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("Stat(configPath): %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "Created system config at "+filepath.Clean(configPath)) {
		t.Fatalf("stdout = %q, want created system config message", got)
	}
	for _, name := range factorypackages.Names() {
		if !strings.Contains(got, "Created packaged factory "+name) {
			t.Fatalf("stdout = %q, want created packaged factory message for %q", got, name)
		}
	}
}

func TestInit_JSONEmitsStructuredSummary(t *testing.T) {
	homeDir := t.TempDir()
	var stdout bytes.Buffer

	if err := Init(InitConfig{
		HomeDir: homeDir,
		JSON:    true,
		Output:  &stdout,
	}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	var payload InitResult
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal stdout: %v\n%s", err, stdout.String())
	}
	if payload.SystemConfigOutcome != "created" {
		t.Fatalf("systemConfigOutcome = %q, want created", payload.SystemConfigOutcome)
	}
	if payload.ConfigPath != defaultpaths.OperatorConfigPath(homeDir) {
		t.Fatalf("configPath = %q, want %q", payload.ConfigPath, defaultpaths.OperatorConfigPath(homeDir))
	}
	if payload.NamedFactoriesRoot != defaultpaths.NamedFactoriesRoot(homeDir) {
		t.Fatalf("namedFactoriesRoot = %q, want %q", payload.NamedFactoriesRoot, defaultpaths.NamedFactoriesRoot(homeDir))
	}
	wantNames := factorypackages.Names()
	if len(payload.PackagedFactories) != len(wantNames) {
		t.Fatalf("packagedFactories count = %d, want %d", len(payload.PackagedFactories), len(wantNames))
	}
	for i, factory := range payload.PackagedFactories {
		if factory.Name != wantNames[i] {
			t.Fatalf("packagedFactories[%d].Name = %q, want %q", i, factory.Name, wantNames[i])
		}
		if factory.Outcome != "created" {
			t.Fatalf("packagedFactories[%d].Outcome = %q, want created", i, factory.Outcome)
		}
		wantDir, err := factoryconfig.MapNamedFactoryDir(payload.NamedFactoriesRoot, factory.Name)
		if err != nil {
			t.Fatalf("MapNamedFactoryDir(%q): %v", factory.Name, err)
		}
		if factory.FactoryDir != wantDir {
			t.Fatalf("packagedFactories[%d].FactoryDir = %q, want %q", i, factory.FactoryDir, wantDir)
		}
	}
}

func TestInit_ExistingConfigReportsSkippedWithoutRewrite(t *testing.T) {
	homeDir := t.TempDir()
	configPath := defaultpaths.OperatorConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	original := []byte(`{"defaults":{"workerModelProvider":"codex"}}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	if err := Init(InitConfig{HomeDir: homeDir, Output: &stdout}); err != nil {
		t.Fatalf("Init() error = %v", err)
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
	var stdout bytes.Buffer
	if err := Init(InitConfig{
		HomeDir:     homeDir,
		Output:      &stdout,
		Diagnostics: io.Discard,
	}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := os.Stat(defaultpaths.OperatorConfigPath(homeDir)); err != nil {
		t.Fatalf("expected config under provided home: %v", err)
	}
}

func TestInit_DoubleRunReportsSkippedOutcomes(t *testing.T) {
	homeDir := t.TempDir()
	var stdout bytes.Buffer

	if err := Init(InitConfig{HomeDir: homeDir, Output: &stdout}); err != nil {
		t.Fatalf("first Init() error = %v", err)
	}
	firstOut := stdout.String()
	if !strings.Contains(firstOut, "Created system config at") {
		t.Fatalf("first stdout = %q, want created system config message", firstOut)
	}

	stdout.Reset()
	if err := Init(InitConfig{HomeDir: homeDir, Output: &stdout}); err != nil {
		t.Fatalf("second Init() error = %v", err)
	}
	secondOut := stdout.String()
	if !strings.Contains(secondOut, "System config already present at") {
		t.Fatalf("second stdout = %q, want already-present system config message", secondOut)
	}
	for _, name := range factorypackages.Names() {
		if !strings.Contains(secondOut, "Packaged factory "+name+" already present at") {
			t.Fatalf("second stdout = %q, want already-present packaged factory message for %q", secondOut, name)
		}
	}
}

func TestInit_DoubleRunJSONReportsSkippedOutcomes(t *testing.T) {
	homeDir := t.TempDir()
	var stdout bytes.Buffer

	if err := Init(InitConfig{HomeDir: homeDir, JSON: true, Output: &stdout}); err != nil {
		t.Fatalf("first Init() error = %v", err)
	}

	stdout.Reset()
	if err := Init(InitConfig{HomeDir: homeDir, JSON: true, Output: &stdout}); err != nil {
		t.Fatalf("second Init() error = %v", err)
	}

	var payload InitResult
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal stdout: %v\n%s", err, stdout.String())
	}
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
	configPath := defaultpaths.OperatorConfigPath(homeDir)
	parentDir := filepath.Dir(configPath)
	if err := os.WriteFile(parentDir, []byte("blocks config directory creation\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(parent blocker): %v", err)
	}

	var stderr bytes.Buffer
	err := Init(InitConfig{
		HomeDir:     homeDir,
		Output:      io.Discard,
		Diagnostics: &stderr,
		Verbose:     true,
	})
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
	namedFactoriesRoot := defaultpaths.NamedFactoriesRoot(homeDir)
	if err := os.MkdirAll(namedFactoriesRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(namedFactoriesRoot): %v", err)
	}
	blocker := filepath.Join(namedFactoriesRoot, "@you")
	if err := os.WriteFile(blocker, []byte("blocks hierarchical factory layout\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(factory scope blocker): %v", err)
	}

	err := Init(InitConfig{
		HomeDir:     homeDir,
		Output:      io.Discard,
		Diagnostics: io.Discard,
	})
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
