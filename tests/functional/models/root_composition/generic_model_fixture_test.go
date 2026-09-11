package root_composition_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

const genericLLMFixtureSource = "hf://fixture/root-composition/gemma-4-E4B-it-Q4_K_M.gguf@0000000000000000000000000000000000000000"

func genericModelFixtureSource(t *testing.T, home, source string) string {
	t.Helper()
	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameLLM)
	if !ok || strings.TrimSpace(source) != strings.TrimSpace(definition.Source) {
		return source
	}
	writeGenericLLMSourceOverride(t, home)
	return genericLLMFixtureSource
}

func writeGenericLLMSourceOverride(t *testing.T, home string) {
	t.Helper()
	configPath := filepath.Join(home, ".you-agent-factory", "config.json")
	config := map[string]any{}
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			t.Fatalf("decode existing operator config: %v", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read existing operator config: %v", err)
	}

	modelsConfig, ok := config["models"].(map[string]any)
	if !ok {
		if config["models"] != nil {
			t.Fatalf("operator config models has type %T, want object", config["models"])
		}
		modelsConfig = map[string]any{}
		config["models"] = modelsConfig
	}
	llmConfig, ok := modelsConfig[models.BuiltInModelNameLLM].(map[string]any)
	if !ok {
		if modelsConfig[models.BuiltInModelNameLLM] != nil {
			t.Fatalf("operator config llm has type %T, want object", modelsConfig[models.BuiltInModelNameLLM])
		}
		llmConfig = map[string]any{}
		modelsConfig[models.BuiltInModelNameLLM] = llmConfig
	}
	llmConfig["source"] = genericLLMFixtureSource

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal generic llm operator config: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create generic llm operator config directory: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write generic llm operator config: %v", err)
	}
}
