package deepresearch_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/definitions/deepresearch"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuiltInFactoryJSON_AssemblesRunnableJavaScriptWorkflow(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(deepresearch.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if !interfaces.IsJavaScriptOrchestratorFactory(cfg) {
		t.Fatalf("orchestrator = %#v, want JAVASCRIPT", cfg.Orchestrator)
	}
	if got := cfg.Orchestrator.JavaScript.SourceRef; got != "scripts/deep-research.workflow.js" {
		t.Fatalf("sourceRef = %q, want packaged workflow path", got)
	}
	if cfg.ResourceManifest == nil || len(cfg.ResourceManifest.BundledFiles) != 1 {
		t.Fatalf("bundled files = %#v, want the authored workflow asset", cfg.ResourceManifest)
	}
}

func TestBuiltInFactoryJSON_MaterializesItsAuthoredWorkflow(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), "@you/deep-research", deepresearch.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	workflowPath := filepath.Join(factoryDir, "scripts", "deep-research.workflow.js")
	content, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", workflowPath, err)
	}
	if len(content) == 0 {
		t.Fatal("materialized workflow is empty")
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	if got := loaded.FactoryConfig().Orchestrator.JavaScript.SourceRef; got != "scripts/deep-research.workflow.js" {
		t.Fatalf("reloaded sourceRef = %q, want packaged workflow path", got)
	}
}

func TestFactoryJSON_DeclaresRequiredTopicInvocationContract(t *testing.T) {
	var authored map[string]any
	if err := json.Unmarshal(deepresearch.FactoryJSON(), &authored); err != nil {
		t.Fatalf("unmarshal authored factory.json: %v", err)
	}
	signature := authored["invocationSignature"].(map[string]any)
	parameters := signature["parameters"].([]any)
	if len(parameters) != 1 {
		t.Fatalf("parameters = %#v, want one topic parameter", parameters)
	}
	topic := parameters[0].(map[string]any)
	if topic["name"] != "topic" || topic["required"] != true {
		t.Fatalf("topic parameter = %#v, want required topic", topic)
	}
}
