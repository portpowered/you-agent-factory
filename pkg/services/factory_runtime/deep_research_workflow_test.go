package factory_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimetestkit "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/testkit"
)

//go:embed testdata/deep_research.workflow.js
var deepResearchWorkflowSource string

var deepResearchArgsSchema = json.RawMessage(`{
  "type":"object",
  "required":["topic"],
  "properties":{
    "topic":{"type":"string","minLength":1},
    "researchDepth":{"type":"integer","minimum":1,"maximum":3,"default":2},
    "maxSubagents":{"type":"integer","minimum":0,"maximum":2,"default":2},
    "modelProvider":{"type":"string","enum":["CODEX"],"default":"CODEX"},
    "model":{"type":"string","enum":["gpt-5"],"minLength":1,"default":"gpt-5"},
    "reasoningEffort":{"type":"string","enum":["medium"],"minLength":1,"default":"medium"}
  },
  "additionalProperties":false
}`)

var deepResearchDefaultPolicy = json.RawMessage(`{
  "mode":"READ_ONLY",
  "maxAgents":3,
  "concurrency":2,
  "maxDepth":1,
  "maxRetries":0,
  "allowNetwork":false,
  "allowConnectors":false,
  "allowDangerFullAccess":false,
  "writableRoots":[],
  "allowedModels":["gpt-5"],
  "allowedReasoningEfforts":["medium"]
}`)

func TestBuiltInFactoryWorkflow_AcceptsApprovedExecutionFlagsWithoutExpandingCapabilities(t *testing.T) {
	t.Parallel()
	outcome := runDeepResearchWorkflow(t, map[string]any{
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
		if child.ModelProvider != "codex" || child.Model != "gpt-5" || child.ReasoningEffort != "medium" {
			t.Fatalf("child execution selection = %#v, want canonical approved codex/gpt-5/medium", child)
		}
	}
	assertLeadSynthesis(t, outcome, "Compare event sourcing versus state machines for distributed workflow orchestration", 2, 2)
}

func TestBuiltInFactoryWorkflow_RejectsDisallowedExecutionSelectionBeforeChildDispatch(t *testing.T) {
	t.Parallel()
	outcome := runDeepResearchWorkflow(t, map[string]any{
		"topic": "Compare event sourcing versus state machines for distributed workflow orchestration",
		"model": "gpt-unapproved",
	})
	if outcome.OK || outcome.Failure.Code != factory.JavaScriptRuntimeCodePreExecutionInvalid {
		t.Fatalf("workflow outcome = %#v, want pre-execution validation failure", outcome)
	}
	if !strings.Contains(outcome.Failure.Message, "'/model'") {
		t.Fatalf("failure = %#v, want actionable model validation diagnostic", outcome.Failure)
	}
	if children := completedChildDispatches(outcome.Records); len(children) != 0 {
		t.Fatalf("completed child dispatches = %#v, want no disallowed execution", children)
	}
	resolution := factory.ResolveJavaScriptFactoryDefaultPolicy(deepResearchDefaultPolicy)
	if err := factory.ValidateJavaScriptPolicyChildRequest(resolution.Policy, factory.JavaScriptPolicyChildRequest{Model: "gpt-unapproved"}); err == nil || !strings.Contains(err.Error(), `policy denied: model "gpt-unapproved" is not listed in allowedModels`) {
		t.Fatalf("policy diagnostic = %v, want existing denied-model diagnostic", err)
	}
}

func TestBuiltInFactoryWorkflow_CompletesWithoutDelegation(t *testing.T) {
	t.Parallel()
	outcome := runDeepResearchWorkflow(t, map[string]any{"topic": "Explain event sourcing"})
	if !outcome.OK {
		t.Fatalf("workflow failure = %#v", outcome.Failure)
	}
	if got := completedSpecialistDispatches(outcome.Records); len(got) != 0 {
		t.Fatalf("completed specialist dispatches = %#v, want none", got)
	}
	assertLeadSynthesis(t, outcome, "Explain event sourcing", 2, 0)
}

func TestBuiltInFactoryWorkflow_DelegatesBoundedSpecialistsAndSynthesizes(t *testing.T) {
	t.Parallel()
	outcome := runDeepResearchWorkflow(t, map[string]any{"topic": "Compare event sourcing versus state machines"})
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
	t.Parallel()
	topic := "Compare event sourcing versus state machines"
	delegated := runDeepResearchWorkflow(t, map[string]any{"topic": topic, "maxSubagents": 1})
	withoutSpecialists := runDeepResearchWorkflow(t, map[string]any{"topic": topic, "maxSubagents": 0})
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
	t.Parallel()
	outcome := runDeepResearchWorkflow(t, map[string]any{
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
	t.Parallel()
	outcome := runDeepResearchWorkflow(t, map[string]any{
		"topic": "Compare event sourcing versus state machines", "maxSubagents": 3,
	})
	if outcome.OK || outcome.Failure.Code != factory.JavaScriptRuntimeCodePreExecutionInvalid {
		t.Fatalf("workflow outcome = %#v, want pre-execution argument validation failure", outcome)
	}
	if len(outcome.Records) != 0 {
		t.Fatalf("runtime records = %#v, want no lead or child dispatch activity", outcome.Records)
	}
}

func TestBuiltInFactoryWorkflow_CanceledContextStopsBeforeChildDispatch(t *testing.T) {
	args, err := json.Marshal(map[string]any{
		"topic": "Cancel this packaged deep-research invocation",
	})
	if err != nil {
		t.Fatalf("Marshal args: %v", err)
	}
	resolution := factory.ResolveJavaScriptFactoryDefaultPolicy(deepResearchDefaultPolicy)
	if len(resolution.Issues) > 0 {
		t.Fatalf("ResolveFromFactoryDefault issues = %#v", resolution.Issues)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome, err := factoryruntimetestkit.JavaScriptWorkflows().Run(ctx, factory.JavaScriptRuntimeRequest{
		Source:     deepResearchWorkflowSource,
		SourceRef:  "deep-research.workflow.js",
		SessionID:  "deep-research-canceled",
		Args:       args,
		ArgsSchema: deepResearchArgsSchema,
		Policy:     resolution.Policy,
	}, factory.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if outcome.OK || outcome.Failure.Code != factory.JavaScriptRuntimeCodeCanceled {
		t.Fatalf("workflow outcome = %#v, want canceled failure", outcome)
	}
	if len(completedChildDispatches(outcome.Records)) != 0 {
		t.Fatalf("completed child dispatches = %#v, want none after cancellation", completedChildDispatches(outcome.Records))
	}
}

func runDeepResearchWorkflow(t *testing.T, arguments map[string]any) factory.JavaScriptRuntimeOutcome {
	t.Helper()
	args, err := json.Marshal(arguments)
	if err != nil {
		t.Fatalf("Marshal args: %v", err)
	}
	resolution := factory.ResolveJavaScriptFactoryDefaultPolicy(deepResearchDefaultPolicy)
	if len(resolution.Issues) > 0 {
		t.Fatalf("ResolveFromFactoryDefault issues = %#v", resolution.Issues)
	}
	outcome, err := factoryruntimetestkit.JavaScriptWorkflows().Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source:     deepResearchWorkflowSource,
		SourceRef:  "deep-research.workflow.js",
		SessionID:  "deep-research-session",
		Args:       args,
		ArgsSchema: deepResearchArgsSchema,
		Policy:     resolution.Policy,
	}, factory.JavaScriptRuntimeHooks{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return outcome
}

func completedChildDispatches(records []factory.JavaScriptRuntimeRecord) []*factory.JavaScriptChildDispatchRecord {
	completed := make([]*factory.JavaScriptChildDispatchRecord, 0)
	for _, record := range records {
		if record.Kind == factory.JavaScriptRecordKindChildDispatch && record.ChildDispatch != nil && record.ChildDispatch.Status == factory.JavaScriptChildDispatchStatusCompleted {
			completed = append(completed, record.ChildDispatch)
		}
	}
	return completed
}

func completedSpecialistDispatches(records []factory.JavaScriptRuntimeRecord) []*factory.JavaScriptChildDispatchRecord {
	completed := completedChildDispatches(records)
	specialists := make([]*factory.JavaScriptChildDispatchRecord, 0, len(completed))
	for _, child := range completed {
		if child.Label != "lead-research-synthesis" {
			specialists = append(specialists, child)
		}
	}
	return specialists
}

func leadSynthesisText(t *testing.T, outcome factory.JavaScriptRuntimeOutcome) string {
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

func assertLeadSynthesis(t *testing.T, outcome factory.JavaScriptRuntimeOutcome, topic string, wantDepth, wantSpecialists int) {
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
