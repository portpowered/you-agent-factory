package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
