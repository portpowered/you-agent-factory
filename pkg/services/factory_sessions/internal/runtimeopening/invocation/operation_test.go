package invocation

import (
	"encoding/json"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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
		Session:    &factorysessions.LiveSession{SessionState: factorysessions.SessionState{FactoryDir: factoryDir}},
	}
	started, err := javaScriptStartRequest(projection, factorysessions.InvocationTarget{
		FactoryDir: factoryDir, MockWorkersConfig: &workers.MockWorkersConfig{},
	}, factorysessions.InvocationRequest{Args: &args, RequestID: &requestID}, invocationInputResolver{
		resolved: factorysessions.ResolvedInvocationInput{NormalizedArguments: &work.NormalizedArguments{
			Arguments: map[string]work.NormalizedArgument{
				"topic": {Values: []string{"injection boundaries"}}, "researchDepth": {Values: []string{"3"}},
				"maxSubagents": {Values: []string{"2"}}, "enabled": {Values: []string{"true"}},
			},
		}},
	}, func() string { return "session-test-id" })
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

type invocationInputResolver struct {
	resolved factorysessions.ResolvedInvocationInput
}

func (r invocationInputResolver) ResolveInvocationInput(
	cfg *factorydefinitions.FactoryConfig,
	request factorysessions.InvocationRequest,
) (factorysessions.ResolvedInvocationInput, error) {
	return r.resolved, nil
}

func TestJavaScriptInvocationResultDecodesCanonicalWorkContent(t *testing.T) {
	primary, err := json.Marshal([]work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "research complete"}})
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	result := javaScriptInvocationResult("request-1", factorysessions.ResultReadResult{
		SessionID: "session-1", SessionStatus: factorysessions.LifecycleStatusSucceeded,
		ResultStatus: factorysessions.ResultStatusFinal, PrimaryResult: primary,
	})
	if result.Status != factorydefinitions.InvocationTerminalStatusCompleted || result.ErrorCode != "" {
		t.Fatalf("result = %#v, want completed", result)
	}
	if len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != "research complete" {
		t.Fatalf("primary result = %#v", result.PrimaryResult)
	}
}
