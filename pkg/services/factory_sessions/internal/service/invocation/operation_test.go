package invocation

import (
	"encoding/json"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestJavaScriptStartRequestPreservesWorkflowFileDefaultPolicy(t *testing.T) {
	factoryDir := t.TempDir()
	defaultPolicy := json.RawMessage(`{"allowedModels":["gpt-allowed"],"mode":"READ_ONLY"}`)
	cfg := &factorydefinitions.FactoryConfig{
		Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
			Kind: factorydefinitions.OrchestratorKindJavaScript,
			JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{
				SourceRef:     "workflow.js",
				DefaultPolicy: defaultPolicy,
			},
		},
	}
	projection := factorysessions.ProjectionContext{
		FactoryCfg: cfg,
		Session:    &factorysessions.ScopedLiveSessionSummary{FactoryDir: factoryDir},
	}
	started, err := javaScriptStartRequest(projection, roles.InvocationTarget{
		FactoryDir: factoryDir,
	}, factorysessions.InvocationRequest{}, factorysessions.ResolvedInvocationInput{}, func() string { return "session-policy-test" })
	if err != nil {
		t.Fatalf("javaScriptStartRequest: %v", err)
	}
	if started.Source.Kind != factoryruntime.WorkflowSourceKindWorkflowFile {
		t.Fatalf("source kind = %q, want WORKFLOW_FILE", started.Source.Kind)
	}
	if started.ProjectRoot != factoryDir {
		t.Fatalf("project root = %q, want %q", started.ProjectRoot, factoryDir)
	}
	if started.Source.InlineWorkflow == nil || string(started.Source.InlineWorkflow.DefaultPolicy) != string(defaultPolicy) {
		t.Fatalf("inline workflow overlay = %#v, want factory defaultPolicy preserved", started.Source.InlineWorkflow)
	}
}

func TestJavaScriptStartRequestUsesDefinitionAndNormalizedArguments(t *testing.T) {
	factoryDir := t.TempDir()
	requestID := "request-deep-research"
	args := map[string]any{"topic": "injection boundaries", "researchDepth": "3", "enabled": "true"}
	cfg := &factorydefinitions.FactoryConfig{
		InvocationSignature: &factorydefinitions.InvocationSignatureConfig{Parameters: []factorydefinitions.InvocationParameterConfig{
			{Name: "topic", Required: true},
			{Name: "researchDepth", DefaultValue: "2"},
			{Name: "maxSubagents", DefaultValue: "2"},
			{Name: "enabled", DefaultValue: "false"},
		}},
		Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
			Kind: factorydefinitions.OrchestratorKindJavaScript,
			JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{
				Dialect: "v1", SourceRef: filepath.Join("scripts", "deep-research.workflow.js"),
				ArgsSchema: json.RawMessage(`{"type":"object","properties":{"topic":{"type":"string"},"researchDepth":{"type":"integer"},"maxSubagents":{"type":"integer"},"enabled":{"type":"boolean"}}}`),
			},
		},
	}
	projection := factorysessions.ProjectionContext{
		FactoryCfg: cfg,
		Session:    &factorysessions.ScopedLiveSessionSummary{FactoryDir: factoryDir},
	}
	started, err := javaScriptStartRequest(projection, roles.InvocationTarget{
		FactoryDir: factoryDir, MockWorkersConfig: &workers.MockWorkersConfig{},
	}, factorysessions.InvocationRequest{Args: &args, RequestID: &requestID}, factorysessions.ResolvedInvocationInput{NormalizedArguments: &work.NormalizedArguments{
		Arguments: map[string]work.NormalizedArgument{
			"topic": {Values: []string{"injection boundaries"}}, "researchDepth": {Values: []string{"3"}},
			"maxSubagents": {Values: []string{"2"}}, "enabled": {Values: []string{"true"}},
		},
	}}, func() string { return "session-test-id" })
	if err != nil {
		t.Fatalf("javaScriptStartRequest: %v", err)
	}
	if started.RequestID != requestID {
		t.Fatalf("request id = %q, want %q", started.RequestID, requestID)
	}
	wantSource := filepath.Join(factoryDir, "scripts", "deep-research.workflow.js")
	if started.Source.Kind != factoryruntime.WorkflowSourceKindWorkflowFile || started.Source.WorkflowFile != wantSource {
		t.Fatalf("source = %#v, want workflow file %q", started.Source, wantSource)
	}
	if started.ProjectRoot != factoryDir {
		t.Fatalf("project root = %q, want %q", started.ProjectRoot, factoryDir)
	}
	if got, ok := started.Args["researchDepth"].(int64); !ok || got != 3 {
		t.Fatalf("researchDepth = %#v, want int64(3)", started.Args["researchDepth"])
	}
	if got, ok := started.Args["maxSubagents"].(int64); !ok || got != 2 {
		t.Fatalf("maxSubagents = %#v, want default int64(2)", started.Args["maxSubagents"])
	}
	if got, ok := started.Args["enabled"].(bool); !ok || !got {
		t.Fatalf("enabled = %#v, want true", started.Args["enabled"])
	}
	if started.Runtime == nil || started.Runtime.ChildExecutorMode != factorysessions.ChildExecutorModeFake {
		t.Fatalf("runtime = %#v, want fake child executor", started.Runtime)
	}
}

func TestJavaScriptStartRequestProjectRootUsesScopedSessionThenTarget(t *testing.T) {
	sessionDir := t.TempDir()
	targetDir := t.TempDir()
	projection := factorysessions.ProjectionContext{FactoryCfg: &factorydefinitions.FactoryConfig{
		Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{
			SourceRef: "workflow.js",
		}},
	}, Session: &factorysessions.ScopedLiveSessionSummary{FactoryDir: sessionDir}}
	target := roles.InvocationTarget{FactoryDir: targetDir}
	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "session", want: sessionDir},
		{name: "target fallback", want: targetDir},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "target fallback" {
				projection.Session = nil
			}
			started, err := javaScriptStartRequest(projection, target, factorysessions.InvocationRequest{}, factorysessions.ResolvedInvocationInput{}, func() string { return "session-id" })
			if err != nil {
				t.Fatalf("javaScriptStartRequest: %v", err)
			}
			if started.ProjectRoot != tc.want || started.Source.WorkflowFile != filepath.Join(tc.want, "workflow.js") {
				t.Fatalf("project root = %q, workflow file = %q, want root %q", started.ProjectRoot, started.Source.WorkflowFile, tc.want)
			}
		})
	}
}

func TestJavaScriptInvocationResultDecodesCanonicalWorkContent(t *testing.T) {
	primary, err := json.Marshal([]work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "research complete"}})
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	result := javaScriptInvocationResult("request-1", factorysessions.ResultReadResult{
		SessionID: "session-1", SessionStatus: factorysessions.LifecycleStatusSucceeded,
		ResultStatus: factorysessions.ResultStatusFinal, PrimaryResult: primary,
	}, nil)
	if result.Status != factorydefinitions.InvocationTerminalStatusCompleted || result.ErrorCode != "" {
		t.Fatalf("result = %#v, want completed", result)
	}
	if len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != "research complete" {
		t.Fatalf("primary result = %#v", result.PrimaryResult)
	}
}

func TestJavaScriptInvocationResultFallsBackToSessionFailureWhenResultUnavailable(t *testing.T) {
	policyMessage := `policy denied: model "gpt-denied" is not listed in allowedModels (label="denied-model")`
	result := javaScriptInvocationResult("request-1", factorysessions.ResultReadResult{
		SessionID:     "session-1",
		SessionStatus: factorysessions.LifecycleStatusFailed,
		ResultStatus:  factorysessions.ResultStatusUnavailable,
	}, &factorysessions.FailureSummary{
		Reason:  "POLICY_DENIED",
		Message: policyMessage,
	})
	if result.Status != factorydefinitions.InvocationTerminalStatusFailed {
		t.Fatalf("status = %q, want FAILED", result.Status)
	}
	if result.Message != policyMessage {
		t.Fatalf("message = %q, want %q", result.Message, policyMessage)
	}
}

func TestFactoryInvocationCallerMappingKeepsAuthorityOutOfInputs(t *testing.T) {
	t.Parallel()
	caller := &workersessions.CallerIdentity{WorkerSessionID: "exact-caller", Token: "planted-invocation-token"}
	request := factorysessions.InvocationRequest{Caller: caller}
	projection := factorysessions.ProjectionContext{FactoryCfg: &factorydefinitions.FactoryConfig{Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{Kind: factorydefinitions.OrchestratorKindJavaScript, JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{SourceRef: "workflow.js"}}}}
	start, err := javaScriptStartRequest(projection, roles.InvocationTarget{FactoryDir: "/project"}, request, factorysessions.ResolvedInvocationInput{}, func() string { return "request" })
	if err != nil {
		t.Fatal(err)
	}
	invoke := sessionInvokeRequest("selected", request)
	caller.Token = "changed-token"
	for _, got := range []*workersessions.CallerIdentity{start.Caller, invoke.Caller} {
		if got == nil || got == caller || got.Token != "planted-invocation-token" || got.WorkerSessionID != "exact-caller" {
			t.Fatal("Factory mapping lost detached caller")
		}
	}
	for _, value := range []any{start, invoke} {
		encoded, err := json.Marshal(value)
		if err != nil || strings.Contains(string(encoded), "planted-invocation-token") || strings.Contains(string(encoded), "exact-caller") {
			t.Fatal("Factory input serialized caller authority")
		}
	}
	if sessionInvokeRequest("selected", factorysessions.InvocationRequest{}).Caller != nil {
		t.Fatal("absent caller acquired authority")
	}
}
