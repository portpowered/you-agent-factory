package factoryrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestResolveFactoryRootFromConfigFile_ReturnsParentDirectory(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(`{"id":"test"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ResolveFactoryRootFromConfigFile(factoryPath)
	if err != nil {
		t.Fatalf("ResolveFactoryRootFromConfigFile: %v", err)
	}
	want, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if got != want {
		t.Fatalf("factory root = %q, want %q", got, want)
	}
}

func TestResolveFactoryRootFromConfigFile_RejectsMissingPath(t *testing.T) {
	_, err := ResolveFactoryRootFromConfigFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected error for missing factory config file")
	}
	if !strings.Contains(err.Error(), "factory config file not found") {
		t.Fatalf("error = %q, want not-found message", err.Error())
	}
}

func TestResolveFactoryRootFromConfigFile_RejectsDirectoryPath(t *testing.T) {
	dir := t.TempDir()
	_, err := ResolveFactoryRootFromConfigFile(dir)
	if err == nil {
		t.Fatal("expected error for directory factory config path")
	}
	if !strings.Contains(err.Error(), "must be a file") {
		t.Fatalf("error = %q, want file requirement message", err.Error())
	}
}

func TestResolveFactoryRootFromConfigFile_RejectsEmptyPath(t *testing.T) {
	_, err := ResolveFactoryRootFromConfigFile("  ")
	if err == nil {
		t.Fatal("expected error for empty factory config path")
	}
	if !strings.Contains(err.Error(), "factory config path is required") {
		t.Fatalf("error = %q, want required-path message", err.Error())
	}
}

func TestLoadFactoryConfigFromConfigFile_LoadsExpandedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(path, []byte(`{
  "name": "portable",
  "workTypes": [{
    "name": "story",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"}
    ]
  }]
}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadFactoryConfigFromConfigFile(path)
	if err != nil {
		t.Fatalf("LoadFactoryConfigFromConfigFile: %v", err)
	}
	if len(cfg.WorkTypes) != 1 || cfg.WorkTypes[0].Name != "story" {
		t.Fatalf("work types = %#v, want one story work type", cfg.WorkTypes)
	}
}

func TestDefaultHandlingWorkTypeName_ReturnsSingleDefaultWorkType(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "task", States: testStoryStates()},
			{Name: "story", States: testStoryStates(), HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
		},
	}

	got, err := DefaultHandlingWorkTypeName(cfg)
	if err != nil {
		t.Fatalf("DefaultHandlingWorkTypeName: %v", err)
	}
	if got != "story" {
		t.Fatalf("work type = %q, want story", got)
	}
}

func TestValidateFactoryForPromptRun_RequiresDefaultHandlingWorkType(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "story", States: testStoryStates()}},
	}

	err := ValidateFactoryForPromptRun(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing DEFAULT handling work type")
	}
	if !strings.Contains(err.Error(), "handlingBehavior DEFAULT") {
		t.Fatalf("error = %q, want DEFAULT handling guidance", err.Error())
	}
}

func testStoryStates() []interfaces.StateConfig {
	return []interfaces.StateConfig{
		{Name: "init", Type: interfaces.StateTypeInitial},
		{Name: "complete", Type: interfaces.StateTypeTerminal},
	}
}
