package deepresearch_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestBuiltInFactoryWorkflow_AcceptsApprovedExecutionFlagsWithoutExpandingCapabilities(t *testing.T) {
	outcome := runPackagedWorkflow(t, map[string]any{
		"topic":         "Compare event sourcing versus state machines for distributed workflow orchestration",
		"modelProvider": "CODEX", "model": "gpt-5", "reasoningEffort": "medium",
	})
	if !outcome.OK {
		t.Fatalf("workflow failure = %#v", outcome.Failure)
	}
	children := completedSpecialistDispatches(outcome.Records)
	if len(children) != 2 {
		t.Fatalf("completed child dispatches = %#v, want two specialists", children)
	}
	for _, child := range children {
		if child.ModelProvider != "CODEX" || child.Model != "gpt-5" || child.ReasoningEffort != "medium" {
			t.Fatalf("child execution selection = %#v, want approved CODEX/gpt-5/medium", child)
		}
	}
	assertLeadSynthesis(t, outcome, "Compare event sourcing versus state machines for distributed workflow orchestration", 2, 2)
}

func TestBuiltInFactoryWorkflow_RejectsDisallowedExecutionSelectionBeforeChildDispatch(t *testing.T) {
	outcome := runPackagedWorkflow(t, map[string]any{
		"topic": "Compare event sourcing versus state machines for distributed workflow orchestration",
		"model": "gpt-unapproved",
	})
	if outcome.OK || outcome.Failure.Code != workflowruntime.CodePreExecutionInvalid {
		t.Fatalf("workflow outcome = %#v, want pre-execution validation failure", outcome)
	}
	if !strings.Contains(outcome.Failure.Message, "'/model'") {
		t.Fatalf("failure = %#v, want actionable model validation diagnostic", outcome.Failure)
	}
	if children := completedChildDispatches(outcome.Records); len(children) != 0 {
		t.Fatalf("completed child dispatches = %#v, want no disallowed execution", children)
	}
	resolution := workflowpolicy.ResolveFromFactoryDefault(deepresearchFactoryPolicy(t))
	if err := workflowpolicy.ValidateChildRequest(resolution.Policy, workflowpolicy.ChildRequest{Model: "gpt-unapproved"}); err == nil || !strings.Contains(err.Error(), `policy denied: model "gpt-unapproved" is not listed in allowedModels`) {
		t.Fatalf("policy diagnostic = %v, want existing denied-model diagnostic", err)
	}
}

func deepresearchFactoryPolicy(t *testing.T) json.RawMessage {
	t.Helper()
	var authored struct {
		Orchestrator struct {
			JavaScript struct {
				DefaultPolicy json.RawMessage `json:"defaultPolicy"`
			} `json:"javascript"`
		} `json:"orchestrator"`
	}
	if err := json.Unmarshal(deepresearch.FactoryJSON(), &authored); err != nil {
		t.Fatalf("unmarshal factory policy: %v", err)
	}
	return authored.Orchestrator.JavaScript.DefaultPolicy
}

func TestBuiltInFactoryWorkflow_CompletesWithoutDelegation(t *testing.T) {
	outcome := runPackagedWorkflow(t, map[string]any{"topic": "Explain event sourcing"})
	if !outcome.OK {
		t.Fatalf("workflow failure = %#v", outcome.Failure)
	}
	if got := completedSpecialistDispatches(outcome.Records); len(got) != 0 {
		t.Fatalf("completed specialist dispatches = %#v, want none", got)
	}
	assertLeadSynthesis(t, outcome, "Explain event sourcing", 2, 0)
}

func TestBuiltInFactoryWorkflow_DelegatesBoundedSpecialistsAndSynthesizes(t *testing.T) {
	outcome := runPackagedWorkflow(t, map[string]any{"topic": "Compare event sourcing versus state machines"})
	if !outcome.OK {
		t.Fatalf("workflow failure = %#v", outcome.Failure)
	}
	children := completedSpecialistDispatches(outcome.Records)
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
	assertLeadSynthesis(t, outcome, "Compare event sourcing versus state machines", 2, 2)
}

func TestBuiltInFactoryWorkflow_LeadSynthesisConsumesSpecialistFindings(t *testing.T) {
	topic := "Compare event sourcing versus state machines"
	delegated := runPackagedWorkflow(t, map[string]any{"topic": topic, "maxSubagents": 1})
	withoutSpecialists := runPackagedWorkflow(t, map[string]any{"topic": topic, "maxSubagents": 0})
	if !delegated.OK || !withoutSpecialists.OK {
		t.Fatalf("workflow outcomes = delegated %#v, without specialists %#v", delegated.Failure, withoutSpecialists.Failure)
	}

	delegatedLead := leadSynthesisText(t, delegated)
	withoutSpecialistsLead := leadSynthesisText(t, withoutSpecialists)
	if !strings.Contains(delegatedLead, "research-specialist-technical") {
		t.Fatalf("delegated lead synthesis = %q, want the specialist finding supplied to the lead", delegatedLead)
	}
	if delegatedLead == withoutSpecialistsLead {
		t.Fatalf("lead synthesis = %q, want specialist findings to change the lead result", delegatedLead)
	}
}

func TestBuiltInFactoryWorkflow_ConfiguresBreadthAndSpecialistCap(t *testing.T) {
	outcome := runPackagedWorkflow(t, map[string]any{
		"topic": "Compare event sourcing versus state machines", "researchDepth": 3, "maxSubagents": 1,
	})
	if !outcome.OK {
		t.Fatalf("workflow failure = %#v", outcome.Failure)
	}
	if children := completedSpecialistDispatches(outcome.Records); len(children) != 1 || children[0].Label != "research-specialist-technical" {
		t.Fatalf("completed child dispatches = %#v, want one technical specialist", children)
	}
	assertLeadSynthesis(t, outcome, "Compare event sourcing versus state machines", 3, 1)
}

func TestBuiltInFactoryWorkflow_RejectsInvalidConfigurationBeforeDispatch(t *testing.T) {
	outcome := runPackagedWorkflow(t, map[string]any{
		"topic": "Compare event sourcing versus state machines", "maxSubagents": 3,
	})
	if outcome.OK || outcome.Failure.Code != workflowruntime.CodePreExecutionInvalid {
		t.Fatalf("workflow outcome = %#v, want pre-execution argument validation failure", outcome)
	}
	if len(outcome.Records) != 0 {
		t.Fatalf("runtime records = %#v, want no lead or child dispatch activity", outcome.Records)
	}
}

func runPackagedWorkflow(t *testing.T, arguments map[string]any) workflowruntime.Outcome {
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
	args, err := json.Marshal(arguments)
	if err != nil {
		t.Fatalf("Marshal args: %v", err)
	}
	resolution := workflowpolicy.ResolveFromFactoryDefault(js.DefaultPolicy)
	if len(resolution.Issues) > 0 {
		t.Fatalf("ResolveFromFactoryDefault issues = %#v", resolution.Issues)
	}
	outcome, err := workflowruntime.Run(context.Background(), workflowruntime.Request{
		Source:     string(source),
		SourceRef:  js.SourceRef,
		SessionID:  "deep-research-session",
		Args:       args,
		ArgsSchema: js.ArgsSchema,
		Policy:     resolution.Policy,
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

func completedSpecialistDispatches(records []workflowruntime.RuntimeRecord) []*workflowruntime.ChildDispatchRecord {
	completed := completedChildDispatches(records)
	specialists := make([]*workflowruntime.ChildDispatchRecord, 0, len(completed))
	for _, child := range completed {
		if child.Label != "lead-research-synthesis" {
			specialists = append(specialists, child)
		}
	}
	return specialists
}

func leadSynthesisText(t *testing.T, outcome workflowruntime.Outcome) string {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(outcome.Value.JSON, &result); err != nil {
		t.Fatalf("unmarshal workflow result: %v", err)
	}
	synthesis, ok := result["synthesis"].(map[string]any)
	if !ok {
		t.Fatalf("synthesis = %#v, want object", result["synthesis"])
	}
	leadResult, ok := synthesis["leadResult"].(map[string]any)
	if !ok {
		t.Fatalf("leadResult = %#v, want lead output", synthesis["leadResult"])
	}
	text, ok := leadResult["text"].(string)
	if !ok {
		t.Fatalf("lead output text = %#v, want string", leadResult["text"])
	}
	return text
}

func assertLeadSynthesis(t *testing.T, outcome workflowruntime.Outcome, topic string, wantDepth, wantSpecialists int) {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(outcome.Value.JSON, &result); err != nil {
		t.Fatalf("unmarshal workflow result: %v", err)
	}
	if result["topic"] != topic || result["role"] != "lead-researcher" || result["researchDepth"] != float64(wantDepth) {
		t.Fatalf("workflow result = %#v, want lead synthesis for %q", result, topic)
	}
	if !strings.Contains(leadSynthesisText(t, outcome), topic) {
		t.Fatalf("lead synthesis = %q, want topic %q", leadSynthesisText(t, outcome), topic)
	}
	children := completedChildDispatches(outcome.Records)
	if len(children) != wantSpecialists+1 {
		t.Fatalf("completed child dispatches = %#v, want %d specialists plus lead", children, wantSpecialists)
	}
	if children[len(children)-1].Label != "lead-research-synthesis" {
		t.Fatalf("last dispatch label = %q, want lead-research-synthesis", children[len(children)-1].Label)
	}
	execution, ok := result["execution"].(map[string]any)
	if !ok || execution["modelProvider"] != "CODEX" || execution["model"] != "gpt-5" || execution["reasoningEffort"] != "medium" {
		t.Fatalf("execution = %#v, want approved default selection", result["execution"])
	}
}
