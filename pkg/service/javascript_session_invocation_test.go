package service

import (
	"context"
	"encoding/json"
	"testing"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/factory/sessions/invocation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workinvocation "github.com/portpowered/infinite-you/pkg/work/invocation"
)

func TestInvokeJavaScriptFactorySession_UsesDurableResultAndTypedArguments(t *testing.T) {
	t.Parallel()
	requestID := "deep-research-invocation-001"
	service := &FactoryService{durableExecution: factorysessionexecution.NewFakeService(factorysessionexecution.WithFakeScenarios(factorysessionexecution.FakeScenario{
		ID:        "deep-research",
		RequestID: requestID,
		Session: factorysessionexecution.SessionReadResult{
			SessionID: "dur-sess-deep-research-001",
			Status:    factorysessionexecution.LifecycleStatusSucceeded,
		},
		Result: factorysessionexecution.ResultReadResult{
			SessionID: "dur-sess-deep-research-001", SessionStatus: factorysessionexecution.LifecycleStatusSucceeded,
			ResultStatus:  factorysessionexecution.ResultStatusFinal,
			PrimaryResult: json.RawMessage(`[{"type":"text","text":"Synthesized research result"}]`),
		},
	}))}
	result, err := service.invokeJavaScriptFactorySession(context.Background(), "~default", deepResearchInvocationConfig(), factoryapi.InvocationRequest{
		RequestId: &requestID,
		Args:      &map[string]any{"topic": "event sourcing", "researchDepth": "3", "maxSubagents": "1"},
	})
	if err != nil {
		t.Fatalf("invoke JavaScript factory session: %v", err)
	}
	if result.Status != factoryapi.InvocationTerminalStatusCompleted || result.SessionID != "dur-sess-deep-research-001" {
		t.Fatalf("result = %#v, want completed durable-session result", result)
	}
	if len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != "Synthesized research result" {
		t.Fatalf("primary result = %#v", result.PrimaryResult)
	}
}

func TestJavaScriptInvocationArgs_CoercesSchemaTypedSignatureArguments(t *testing.T) {
	t.Parallel()
	resolved, err := sessionInvocationInputForTest(deepResearchInvocationConfig())
	if err != nil {
		t.Fatal(err)
	}
	args, err := javascriptInvocationArgs(deepResearchInvocationConfig(), resolved)
	if err != nil {
		t.Fatalf("javascriptInvocationArgs: %v", err)
	}
	if depth, ok := args["researchDepth"].(int); !ok || depth != 3 {
		t.Fatalf("researchDepth = %#v, want int(3)", args["researchDepth"])
	}
	if limit, ok := args["maxSubagents"].(int); !ok || limit != 1 {
		t.Fatalf("maxSubagents = %#v, want int(1)", args["maxSubagents"])
	}
}

func sessionInvocationInputForTest(cfg *interfaces.FactoryConfig) (*workinvocation.NormalizedArguments, error) {
	resolved, err := sessioninvocation.ResolveSessionInvocationInput(cfg, factoryapi.InvocationRequest{
		Args: &map[string]any{"topic": "event sourcing", "researchDepth": "3", "maxSubagents": "1"},
	})
	return resolved.NormalizedArguments, err
}

func deepResearchInvocationConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Name: "@you/deep-research",
		InvocationSignature: &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{
			{Name: "topic", Required: true, Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "POSITIONAL", Position: 1}}},
			{Name: "researchDepth", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "NAMED"}}},
			{Name: "maxSubagents", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "NAMED"}}},
		}},
		Orchestrator: &interfaces.FactoryOrchestratorConfig{Kind: interfaces.OrchestratorKindJavaScript, JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
			InlineSource: &interfaces.FactoryOrchestratorJavaScriptInlineSource{Inline: `meta({ name: "deep-research", version: 1 }); final("done");`},
			ArgsSchema:   json.RawMessage(`{"type":"object","properties":{"topic":{"type":"string"},"researchDepth":{"type":"integer"},"maxSubagents":{"type":"integer"}}}`),
		}},
	}
}
