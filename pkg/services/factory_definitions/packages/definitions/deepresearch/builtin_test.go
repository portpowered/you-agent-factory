package deepresearch_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/definitions/deepresearch"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestBuiltInFactoryJSON_AssemblesRunnableJavaScriptWorkflow(t *testing.T) {
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(deepresearch.BuiltInFactoryJSON)
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
	factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(t.TempDir(), "@you/deep-research", deepresearch.BuiltInFactoryJSON, factoryvalidation.New(nil))
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
	loaded, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil)
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
	if len(parameters) != 6 {
		t.Fatalf("parameters = %#v, want topic, configuration, and execution parameters", parameters)
	}
	topic := parameters[0].(map[string]any)
	if topic["name"] != "topic" || topic["required"] != true {
		t.Fatalf("topic parameter = %#v, want required topic", topic)
	}
	for _, want := range []struct{ name, external string }{
		{name: "modelProvider", external: "model-provider"},
		{name: "model", external: "model"},
		{name: "reasoningEffort", external: "reasoning-effort"},
	} {
		found := false
		for _, raw := range parameters[1:] {
			parameter := raw.(map[string]any)
			if parameter["name"] == want.name && parameter["externalName"] == want.external {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("parameters = %#v, want named execution flag %q", parameters, want.external)
		}
	}
}

func TestFactoryJSON_DeclaresBoundedJavaScriptSchemaAndPolicy(t *testing.T) {
	var authored struct {
		Orchestrator struct {
			JavaScript struct {
				ArgsSchema struct {
					Required   []string `json:"required"`
					Properties map[string]struct {
						Maximum float64  `json:"maximum"`
						Enum    []string `json:"enum"`
					} `json:"properties"`
				} `json:"argsSchema"`
				DefaultPolicy struct {
					MaxAgents               int      `json:"maxAgents"`
					AllowedModels           []string `json:"allowedModels"`
					AllowedReasoningEfforts []string `json:"allowedReasoningEfforts"`
				} `json:"defaultPolicy"`
			} `json:"javascript"`
		} `json:"orchestrator"`
	}
	if err := json.Unmarshal(deepresearch.FactoryJSON(), &authored); err != nil {
		t.Fatalf("unmarshal authored factory.json: %v", err)
	}
	js := authored.Orchestrator.JavaScript
	if len(js.ArgsSchema.Required) != 1 || js.ArgsSchema.Required[0] != "topic" {
		t.Fatalf("required arguments = %#v, want topic", js.ArgsSchema.Required)
	}
	if got := js.ArgsSchema.Properties["maxSubagents"].Maximum; got != 2 {
		t.Fatalf("maxSubagents maximum = %v, want 2", got)
	}
	if got := js.ArgsSchema.Properties["model"].Enum; len(got) != 1 || got[0] != "gpt-5" {
		t.Fatalf("model enum = %#v, want gpt-5", got)
	}
	if js.DefaultPolicy.MaxAgents != 3 ||
		len(js.DefaultPolicy.AllowedModels) != 1 || js.DefaultPolicy.AllowedModels[0] != "gpt-5" ||
		len(js.DefaultPolicy.AllowedReasoningEfforts) != 1 || js.DefaultPolicy.AllowedReasoningEfforts[0] != "medium" {
		t.Fatalf("default policy = %#v, want bounded gpt-5/medium execution", js.DefaultPolicy)
	}
}
