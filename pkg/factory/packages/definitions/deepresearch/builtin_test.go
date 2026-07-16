package deepresearch_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/definitions/deepresearch"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
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

func TestBuiltInFactoryWorkflow_CompletesWithoutDelegation(t *testing.T) {
	outcome := runPackagedWorkflow(t, "Explain event sourcing")
	if !outcome.OK {
		t.Fatalf("workflow failure = %#v", outcome.Failure)
	}
	if got := completedChildDispatches(outcome.Records); len(got) != 0 {
		t.Fatalf("completed child dispatches = %#v, want none", got)
	}
	assertLeadSynthesis(t, outcome, "Explain event sourcing", 0)
}

func TestBuiltInFactoryWorkflow_DelegatesBoundedSpecialistsAndSynthesizes(t *testing.T) {
	outcome := runPackagedWorkflow(t, "Compare event sourcing versus state machines")
	if !outcome.OK {
		t.Fatalf("workflow failure = %#v", outcome.Failure)
	}
	children := completedChildDispatches(outcome.Records)
	if len(children) != 2 {
		t.Fatalf("completed child dispatches = %#v, want two bounded specialists", children)
	}
	labels := map[string]bool{}
	for _, child := range children {
		labels[child.Label] = true
	}
	if !labels["research-specialist-technical"] || !labels["research-specialist-tradeoffs"] {
		t.Fatalf("specialist labels = %#v, want stable technical and trade-off roles", labels)
	}
	assertLeadSynthesis(t, outcome, "Compare event sourcing versus state machines", 2)
}

func runPackagedWorkflow(t *testing.T, topic string) workflowruntime.Outcome {
	t.Helper()
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), "@you/deep-research", deepresearch.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	js := loaded.FactoryConfig().Orchestrator.JavaScript
	source, err := os.ReadFile(filepath.Join(factoryDir, filepath.FromSlash(js.SourceRef)))
	if err != nil {
		t.Fatalf("ReadFile workflow: %v", err)
	}
	args, err := json.Marshal(map[string]string{"topic": topic})
	if err != nil {
		t.Fatalf("Marshal args: %v", err)
	}
	resolution := workflowpolicy.ResolveFromFactoryDefault(js.DefaultPolicy)
	if len(resolution.Issues) > 0 {
		t.Fatalf("ResolveFromFactoryDefault issues = %#v", resolution.Issues)
	}
	outcome, err := workflowruntime.Run(context.Background(), workflowruntime.Request{
		Source:    string(source),
		SourceRef: js.SourceRef,
		SessionID: "deep-research-session",
		Args:      args,
		Policy:    resolution.Policy,
	}, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return outcome
}

func completedChildDispatches(records []workflowruntime.RuntimeRecord) []*workflowruntime.ChildDispatchRecord {
	completed := make([]*workflowruntime.ChildDispatchRecord, 0)
	for _, record := range records {
		if record.Kind == workflowruntime.RecordKindChildDispatch && record.ChildDispatch != nil && record.ChildDispatch.Status == workflowruntime.ChildDispatchStatusCompleted {
			completed = append(completed, record.ChildDispatch)
		}
	}
	return completed
}

func assertLeadSynthesis(t *testing.T, outcome workflowruntime.Outcome, topic string, wantSpecialists int) {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(outcome.Value.JSON, &result); err != nil {
		t.Fatalf("unmarshal workflow result: %v", err)
	}
	if result["topic"] != topic || result["role"] != "lead-researcher" {
		t.Fatalf("workflow result = %#v, want lead synthesis for %q", result, topic)
	}
	synthesis, ok := result["synthesis"].(map[string]any)
	if !ok {
		t.Fatalf("synthesis = %#v, want object", result["synthesis"])
	}
	findings, ok := synthesis["specialistFindings"].([]any)
	if !ok || len(findings) != wantSpecialists {
		t.Fatalf("specialistFindings = %#v, want %d findings", synthesis["specialistFindings"], wantSpecialists)
	}
}
