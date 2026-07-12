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
	for _, name := range factoryconfig.BuiltInNamedFactoryNames() {
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
	if len(payload.PackagedFactories) != len(factoryconfig.BuiltInNamedFactoryNames()) {
		t.Fatalf("packagedFactories count = %d, want %d", len(payload.PackagedFactories), len(factoryconfig.BuiltInNamedFactoryNames()))
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
		HomeDir: homeDir,
		Output:  &stdout,
		Diagnostics: io.Discard,
	}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := os.Stat(defaultpaths.OperatorConfigPath(homeDir)); err != nil {
		t.Fatalf("expected config under provided home: %v", err)
	}
}
