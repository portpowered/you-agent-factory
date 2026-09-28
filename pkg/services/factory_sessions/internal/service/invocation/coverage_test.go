package invocation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

func TestNewOperationRequiresRemainingDependencies(t *testing.T) {
	t.Parallel()

	resolver := factorydefinitions.CurrentFactoryDirectoryResolver(func(root string) (string, error) {
		return root, nil
	})
	validRoots := factoryruntime.RuntimeArtifactRootResolver(func(string) factoryruntime.RuntimeArtifactRoots {
		return factoryruntime.RuntimeArtifactRoots{}
	})
	validGenerator := factorysessions.SessionIDGenerator(func() string { return "session-id" })
	validPresentations := invocationPresentationOwnerStub{}
	tests := []struct {
		name          string
		artifactRoots factoryruntime.RuntimeArtifactRootResolver
		generator     factorysessions.SessionIDGenerator
		logger        *zap.Logger
		presentations factorysessions.OpeningPresentationOwner
		want          string
	}{
		{name: "artifact roots", artifactRoots: nil, generator: validGenerator, logger: zap.NewNop(), presentations: validPresentations, want: "runtime artifact root resolver is required"},
		{name: "session id generator", artifactRoots: validRoots, generator: nil, logger: zap.NewNop(), presentations: validPresentations, want: "Factory Session ID generator is required"},
		{name: "logger", artifactRoots: validRoots, generator: validGenerator, logger: nil, presentations: validPresentations, want: "invocation logger is required"},
		{name: "presentations", artifactRoots: validRoots, generator: validGenerator, logger: zap.NewNop(), presentations: nil, want: "invocation presentation owner is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewOperation(
				&invocationSessionStub{}, nil, workingDirectoryStub{}, resolver,
				artifactExporterStub{}, factorysessions.DefaultModelInvocationTimeout,
				test.artifactRoots, test.generator, test.logger, test.presentations,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewOperation() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestJoinTeardownErrorUnlessResultDeterminedPreservesTerminalOutcome(t *testing.T) {
	t.Parallel()

	resultErr := errors.New("invocation failed")
	postResultErr := errors.New("event bridge failed")
	if got := joinTeardownErrorUnlessResultDetermined(
		roles.FactoryInvocationOutcome{}, resultErr, nil, zap.NewNop(),
	); !errors.Is(got, resultErr) {
		t.Fatalf("nil post-result error = %v, want invocation error", got)
	}
	joined := joinTeardownErrorUnlessResultDetermined(
		roles.FactoryInvocationOutcome{}, resultErr, postResultErr, zap.NewNop(),
	)
	if !errors.Is(joined, resultErr) || !errors.Is(joined, postResultErr) {
		t.Fatalf("non-terminal errors = %v, want both errors", joined)
	}
	terminal := roles.FactoryInvocationOutcome{
		Result: factorydefinitions.FactoryInvocationResult{
			Status: factorydefinitions.InvocationTerminalStatusCompleted,
		},
	}
	kept := joinTeardownErrorUnlessResultDetermined(terminal, resultErr, postResultErr, zap.NewNop())
	if !errors.Is(kept, resultErr) || errors.Is(kept, postResultErr) {
		t.Fatalf("terminal errors = %v, want only invocation error", kept)
	}
}

func TestStartFactoryEventBridgeRequiresPresentationForScopedEvents(t *testing.T) {
	t.Parallel()

	var nilOperation *operation
	if bridge, err := nilOperation.startFactoryEventBridge(context.Background(), nil, roles.InvocationTarget{EventScopeID: "scope-1"}); err == nil || bridge != nil {
		t.Fatalf("nil operation bridge = %v, error = %v, want required-owner error", bridge, err)
	}
	op := &operation{presentations: invocationPresentationOwnerStub{}}
	if bridge, err := op.startFactoryEventBridge(context.Background(), nil, roles.InvocationTarget{}); err != nil || bridge != nil {
		t.Fatalf("unscoped bridge = %v, error = %v, want no bridge", bridge, err)
	}
	if bridge, err := op.startFactoryEventBridge(context.Background(), nil, roles.InvocationTarget{EventScopeID: "scope-1"}); err != nil || bridge != nil {
		t.Fatalf("scoped bridge = %v, error = %v, want presentation-owned nil bridge", bridge, err)
	}
}

func TestOpenModelsCatalogScopeOwnsAnIndependentScopePerCaller(t *testing.T) {
	t.Parallel()

	root := &catalogScopeModelsStub{}
	op := &operation{modelsRoot: root}
	first, err := op.OpenModelsCatalogScope(context.Background())
	if err != nil {
		t.Fatalf("first OpenModelsCatalogScope: %v", err)
	}
	second, err := op.OpenModelsCatalogScope(context.Background())
	if err != nil {
		t.Fatalf("second OpenModelsCatalogScope: %v", err)
	}
	if first.Scope == second.Scope || root.opens != 2 {
		t.Fatalf("catalog scopes = (%v, %v), opens = %d; want distinct caller-owned scopes", first.Scope, second.Scope, root.opens)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatalf("close first catalog scope: %v", err)
	}
	if err := second.Close(context.Background()); err != nil {
		t.Fatalf("close second catalog scope: %v", err)
	}
	if root.closes != 2 {
		t.Fatalf("catalog scope closes = %d, want one per caller", root.closes)
	}
}

func TestJavaScriptWorkflowSourceCopiesInlineDefinitionAndRejectsMissingFile(t *testing.T) {
	t.Parallel()

	policy := json.RawMessage(`{"mode":"safe"}`)
	schema := json.RawMessage(`{"type":"object"}`)
	inline, err := javaScriptWorkflowSource(
		&factorydefinitions.FactoryOrchestratorJavaScriptConfig{
			Dialect: "v1", Entrypoint: "main", InlineSource: &factorydefinitions.FactoryOrchestratorJavaScriptInlineSource{Inline: "return 1"},
			Metadata: map[string]string{"owner": "test"}, Agents: map[string]factorydefinitions.FactoryOrchestratorJavaScriptAgent{"research": {Preset: "fast"}},
			ArgsSchema: schema, DefaultPolicy: policy,
		},
		factorysessions.ProjectionContext{}, roles.InvocationTarget{},
	)
	if err != nil {
		t.Fatalf("inline source: %v", err)
	}
	if inline.Kind != factoryruntime.WorkflowSourceKindInlineWorkflow || inline.InlineWorkflow == nil || inline.InlineWorkflow.InlineSource != "return 1" || inline.InlineWorkflow.Entrypoint != "main" {
		t.Fatalf("inline source = %#v", inline)
	}
	if inline.InlineWorkflow.Metadata["owner"] != "test" || string(inline.InlineWorkflow.DefaultPolicy) != string(policy) {
		t.Fatalf("inline metadata/policy = %#v", inline.InlineWorkflow)
	}
	if _, err := javaScriptWorkflowSource(
		&factorydefinitions.FactoryOrchestratorJavaScriptConfig{},
		factorysessions.ProjectionContext{}, roles.InvocationTarget{},
	); err == nil || !strings.Contains(err.Error(), "sourceRef is required") {
		t.Fatalf("missing sourceRef error = %v", err)
	}
	factoryDir := filepath.Join("target", "factory")
	source, err := javaScriptWorkflowSource(
		&factorydefinitions.FactoryOrchestratorJavaScriptConfig{SourceRef: "workflow.js"},
		factorysessions.ProjectionContext{Session: &factorysessions.ScopedLiveSessionSummary{FactoryDir: filepath.Join("session", "factory")}},
		roles.InvocationTarget{FactoryDir: factoryDir},
	)
	if err != nil {
		t.Fatalf("relative source: %v", err)
	}
	if source.WorkflowFile != filepath.Join("session", "factory", "workflow.js") {
		t.Fatalf("workflow file = %q", source.WorkflowFile)
	}
}

func TestJavaScriptInvocationArgsPreservesMultiplicityAndCoercesValues(t *testing.T) {
	t.Parallel()
	t.Run("preserves repeated values", testJavaScriptInvocationArgsPreservesRepeatedValues)
	t.Run("coerces typed values", testJavaScriptInvocationArgsCoercesTypedValues)
	t.Run("handles empty arguments", testJavaScriptInvocationArgsHandlesEmpty)
}

func testJavaScriptInvocationArgsPreservesRepeatedValues(t *testing.T) {
	t.Helper()
	resolved := factorysessions.ResolvedInvocationInput{NormalizedArguments: &work.NormalizedArguments{
		Arguments: map[string]work.NormalizedArgument{
			"tags": {Values: []string{"one", "two"}},
		},
	}}
	args := javaScriptInvocationArgs(nil, resolved)
	if got, ok := args["tags"].([]any); !ok || len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("tags = %#v, want two values", args["tags"])
	}
}

func testJavaScriptInvocationArgsCoercesTypedValues(t *testing.T) {
	t.Helper()
	resolved := factorysessions.ResolvedInvocationInput{NormalizedArguments: &work.NormalizedArguments{
		Arguments: map[string]work.NormalizedArgument{
			"count":   {Values: []string{"not-an-integer"}},
			"ratio":   {Values: []string{"1.25"}},
			"enabled": {Values: []string{"not-a-bool"}},
			"raw":     {Values: []string{"plain"}},
		},
	}}
	args := javaScriptInvocationArgs(
		json.RawMessage(`{"properties":{"count":{"type":"integer"},"ratio":{"type":"number"},"enabled":{"type":"boolean"}}}`),
		resolved,
	)
	if got, ok := args["count"].(string); !ok || got != "not-an-integer" {
		t.Fatalf("invalid integer = %#v, want original string", args["count"])
	}
	if got, ok := args["ratio"].(float64); !ok || got != 1.25 {
		t.Fatalf("ratio = %#v, want float64(1.25)", args["ratio"])
	}
	if got, ok := args["enabled"].(string); !ok || got != "not-a-bool" {
		t.Fatalf("invalid boolean = %#v, want original string", args["enabled"])
	}
	if got, ok := args["raw"].(string); !ok || got != "plain" {
		t.Fatalf("raw = %#v, want original string", args["raw"])
	}
}

func testJavaScriptInvocationArgsHandlesEmpty(t *testing.T) {
	t.Helper()
	empty := javaScriptInvocationArgs(nil, factorysessions.ResolvedInvocationInput{})
	if len(empty) != 0 {
		t.Fatalf("nil normalized arguments = %#v, want empty map", empty)
	}
}

func TestJavaScriptInvocationResultReportsDecodeAndAvailabilityFailures(t *testing.T) {
	t.Parallel()

	invalid := javaScriptInvocationResult("request-1", factorysessions.ResultReadResult{
		SessionID: "session-1", SessionStatus: factorysessions.LifecycleStatusSucceeded,
		ResultStatus: factorysessions.ResultStatusFinal, PrimaryResult: json.RawMessage("{"),
	}, nil)
	if invalid.Status != factorydefinitions.InvocationTerminalStatusFailed || !strings.Contains(invalid.Message, "decode JavaScript Factory result") {
		t.Fatalf("invalid result = %#v, want decode failure", invalid)
	}
	fromResult := javaScriptInvocationResult("request-2", factorysessions.ResultReadResult{
		SessionID: "session-2", SessionStatus: factorysessions.LifecycleStatusFailed,
		ResultStatus: factorysessions.ResultStatusUnavailable,
		Failure:      &factorysessions.FailureSummary{Reason: "RUNTIME_FAILED"},
	}, nil)
	if fromResult.Message != "RUNTIME_FAILED" {
		t.Fatalf("result failure message = %q, want reason", fromResult.Message)
	}
	fromAvailability := javaScriptInvocationResult("request-3", factorysessions.ResultReadResult{
		SessionID: "session-3", SessionStatus: factorysessions.LifecycleStatusRunning,
		ResultStatus: factorysessions.ResultStatusUnavailable,
		Availability: &factorysessions.ResultAvailabilityDetail{Message: "result is still pending"},
	}, nil)
	if fromAvailability.Message != "result is still pending" {
		t.Fatalf("availability message = %q", fromAvailability.Message)
	}
	defaultMessage := javaScriptInvocationResult("request-4", factorysessions.ResultReadResult{}, nil)
	if defaultMessage.Message == "" || !strings.Contains(defaultMessage.Message, "did not produce") {
		t.Fatalf("default message = %q", defaultMessage.Message)
	}
}

type catalogScopeModelsStub struct {
	models.Service
	opens  int
	closes int
}

func (stub *catalogScopeModelsStub) OpenRuntimeScope(context.Context, models.OpenRuntimeScopeRequest) (models.OpenRuntimeScopeResult, error) {
	stub.opens++
	scope, err := (models.RuntimeScopeRef{}).Parse(fmt.Sprintf("catalog:%d", stub.opens))
	return models.OpenRuntimeScopeResult{Scope: scope}, err
}

func (stub *catalogScopeModelsStub) CloseRuntimeScope(_ context.Context, request models.CloseRuntimeScopeRequest) (models.CloseRuntimeScopeResult, error) {
	stub.closes++
	return models.CloseRuntimeScopeResult{Scope: request.Scope, Closed: true}, nil
}
