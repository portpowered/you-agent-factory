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
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
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
				&runtimeopening.Factory{}, nil, workingDirectoryStub{}, resolver,
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
	op := &operation{openRuntime: &runtimeopening.Factory{}, modelsRoot: root}
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

func TestHostedInvocationFinishesThroughPresentationOwnedEventBridge(t *testing.T) {
	t.Parallel()

	projection := factorysessions.SessionProjection{
		Context: factorysessions.ProjectionContext{FactoryCfg: &factorydefinitions.FactoryConfig{
			Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
				Kind: factorydefinitions.OrchestratorKindPetri,
			},
		}},
	}
	sessions := newHostedLiveSessionsFake(projection)
	sessions.invokeResult = factorysessions.InvocationResult{
		Status: factorysessions.InvocationTerminalStatusCompleted,
	}
	bridge := &recordingInvocationEventBridge{err: errors.New("post-result bridge failure")}
	op := &operation{
		logger:        zap.NewNop(),
		presentations: invocationPresentationOwnerWithBridge{bridge: bridge},
	}
	outcome, err := op.invokeFactoryOnHostedLiveRuntime(
		context.Background(), sessions,
		roles.InvocationTarget{EventScopeID: "scope-1"},
		factorysessions.InvocationRequest{},
	)
	if err != nil {
		t.Fatalf("invokeFactoryOnHostedLiveRuntime: %v", err)
	}
	if outcome.Result.Status != factorydefinitions.InvocationTerminalStatusCompleted {
		t.Fatalf("outcome status = %q, want completed", outcome.Result.Status)
	}
	if bridge.finishCalls != 1 {
		t.Fatalf("event bridge finish calls = %d, want one", bridge.finishCalls)
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
	t.Run("handles empty and missing resolvers", testJavaScriptInvocationArgsHandlesResolverBranches)
}

func testJavaScriptInvocationArgsPreservesRepeatedValues(t *testing.T) {
	t.Helper()
	resolved := factorysessions.ResolvedInvocationInput{NormalizedArguments: &work.NormalizedArguments{
		Arguments: map[string]work.NormalizedArgument{
			"tags": {Values: []string{"one", "two"}},
		},
	}}
	args, err := javaScriptInvocationArgs(
		&factorydefinitions.FactoryConfig{}, factorysessions.InvocationRequest{},
		nil,
		invocationInputResolver{resolved: resolved},
	)
	if err != nil {
		t.Fatalf("javaScriptInvocationArgs: %v", err)
	}
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
	args, err := javaScriptInvocationArgs(
		&factorydefinitions.FactoryConfig{}, factorysessions.InvocationRequest{},
		json.RawMessage(`{"properties":{"count":{"type":"integer"},"ratio":{"type":"number"},"enabled":{"type":"boolean"}}}`),
		invocationInputResolver{resolved: resolved},
	)
	if err != nil {
		t.Fatalf("javaScriptInvocationArgs: %v", err)
	}
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

func testJavaScriptInvocationArgsHandlesResolverBranches(t *testing.T) {
	t.Helper()
	empty, err := javaScriptInvocationArgs(&factorydefinitions.FactoryConfig{}, factorysessions.InvocationRequest{}, nil, invocationInputResolver{})
	if err != nil || len(empty) != 0 {
		t.Fatalf("nil normalized arguments = %#v, error = %v, want empty map", empty, err)
	}
	if _, err := javaScriptInvocationArgs(&factorydefinitions.FactoryConfig{}, factorysessions.InvocationRequest{}, nil, nil); err == nil {
		t.Fatal("nil resolver error = nil, want required resolver")
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

func TestInvocationOpenClosesRuntimeAfterStartupFailure(t *testing.T) {
	t.Parallel()

	startupErr := errors.New("startup failed")
	closeErr := errors.New("artifact close failed")
	opening := &invocationRuntimeOpeningStub{opened: roles.OpenedInvocationRuntime{
		Sessions:       &hostedLiveSessionsFake{},
		Lifecycle:      coverageLifecycleStub{startErr: startupErr},
		CloseArtifacts: func() error { return closeErr },
	}}
	op := &operation{
		openRuntime:   opening,
		artifactRoots: func(string) factoryruntime.RuntimeArtifactRoots { return factoryruntime.RuntimeArtifactRoots{} },
	}
	_, _, err := op.open(context.Background(), roles.InvocationTarget{})
	if !errors.Is(err, startupErr) || !errors.Is(err, closeErr) {
		t.Fatalf("open() error = %v, want startup and cleanup errors", err)
	}
}

func TestInvocationLifecycleCloseAggregatesCleanupFailures(t *testing.T) {
	t.Parallel()

	workerErr := errors.New("worker stop failed")
	lifecycleErr := errors.New("lifecycle stop failed")
	waitErr := errors.New("runtime wait failed")
	artifactErr := errors.New("artifact close failed")
	cancelled := false
	active := &lifecycle{
		cancel:     func() { cancelled = true },
		stopWorker: func(context.Context) error { return workerErr },
	}
	opened := roles.OpenedInvocationRuntime{
		Sessions: &hostedLiveSessionsFake{}, Lifecycle: coverageLifecycleStub{stopErr: lifecycleErr, waitErr: waitErr},
		CloseArtifacts: func() error { return artifactErr },
	}
	err := active.close(context.Background(), opened)
	if !cancelled || !errors.Is(err, workerErr) || !errors.Is(err, lifecycleErr) || !errors.Is(err, waitErr) || !errors.Is(err, artifactErr) {
		t.Fatalf("close() cancelled=%v error=%v, want all cleanup failures", cancelled, err)
	}
}

func TestInvokeJavaScriptFactoryUsesExecutionResultAndSessionFailure(t *testing.T) {
	t.Parallel()

	primary, err := json.Marshal([]work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "done"}})
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	projection := factorysessions.ProjectionContext{
		FactoryCfg: &factorydefinitions.FactoryConfig{Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{
			Kind:       factorydefinitions.OrchestratorKindJavaScript,
			JavaScript: &factorydefinitions.FactoryOrchestratorJavaScriptConfig{InlineSource: &factorydefinitions.FactoryOrchestratorJavaScriptInlineSource{Inline: "return 1"}},
		}},
	}
	execution := &javascriptExecutionStub{
		start:  factorysessions.SyncStartResult{AsyncStartResult: factorysessions.AsyncStartResult{SessionID: "session-js"}},
		result: factorysessions.ResultReadResult{SessionID: "session-js", SessionStatus: factorysessions.LifecycleStatusSucceeded, ResultStatus: factorysessions.ResultStatusFinal, PrimaryResult: primary},
	}
	opened := roles.OpenedInvocationRuntime{InputResolver: invocationInputResolver{}, Execution: execution}
	result, err := invokeJavaScriptFactory(context.Background(), opened, projection, roles.InvocationTarget{}, factorysessions.InvocationRequest{}, func() string { return "generated" })
	if err != nil || result.Status != factorydefinitions.InvocationTerminalStatusCompleted || len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != "done" {
		t.Fatalf("success result = %#v, error = %v", result, err)
	}
	if execution.request.RequestID != "run-generated" {
		t.Fatalf("start request id = %q, want generated request id", execution.request.RequestID)
	}

	failureMessage := "workflow failed before result"
	execution = &javascriptExecutionStub{
		start:   factorysessions.SyncStartResult{AsyncStartResult: factorysessions.AsyncStartResult{SessionID: "session-failed"}},
		result:  factorysessions.ResultReadResult{SessionID: "session-failed", SessionStatus: factorysessions.LifecycleStatusFailed, ResultStatus: factorysessions.ResultStatusUnavailable},
		session: factorysessions.SessionReadResult{Failure: &factorysessions.FailureSummary{Message: failureMessage}},
	}
	opened.Execution = execution
	result, err = invokeJavaScriptFactory(context.Background(), opened, projection, roles.InvocationTarget{}, factorysessions.InvocationRequest{}, func() string { return "generated" })
	if err != nil || result.Status != factorydefinitions.InvocationTerminalStatusFailed || result.Message != failureMessage {
		t.Fatalf("failure result = %#v, error = %v", result, err)
	}
}

func TestInvokeJavaScriptFactoryRejectsMissingInputResolver(t *testing.T) {
	t.Parallel()

	_, err := invokeJavaScriptFactory(context.Background(), roles.OpenedInvocationRuntime{}, factorysessions.ProjectionContext{}, roles.InvocationTarget{}, factorysessions.InvocationRequest{}, func() string { return "id" })
	if err == nil || !strings.Contains(err.Error(), "input resolver is required") {
		t.Fatalf("error = %v, want input-resolver error", err)
	}
}

type coverageLifecycleStub struct {
	startErr error
	stopErr  error
	waitErr  error
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

type recordingInvocationEventBridge struct {
	finishCalls int
	err         error
}

func (bridge *recordingInvocationEventBridge) Finish(context.Context, roles.FactoryEventReader, factorysessions.FactoryInvocationOutcome) error {
	bridge.finishCalls++
	return bridge.err
}

type invocationPresentationOwnerWithBridge struct {
	invocationPresentationOwnerStub
	bridge *recordingInvocationEventBridge
}

func (owner invocationPresentationOwnerWithBridge) StartFactoryEventBridge(context.Context, roles.FactoryEventReader, factorysessions.OpeningScopeID) (interface {
	Finish(context.Context, roles.FactoryEventReader, factorysessions.FactoryInvocationOutcome) error
}, error) {
	return owner.bridge, nil
}

func (stub coverageLifecycleStub) StartLifecycle(context.Context, context.Context) error {
	return stub.startErr
}

func (coverageLifecycleStub) StartWorkerLifecycle(context.Context) (factorysessions.RuntimeStop, error) {
	return nil, nil
}

func (coverageLifecycleStub) CompleteStartup(context.Context) error { return nil }

func (stub coverageLifecycleStub) WaitForRuntime(context.Context) error { return stub.waitErr }

func (stub coverageLifecycleStub) StopLifecycle(context.Context) error { return stub.stopErr }

func (coverageLifecycleStub) FailStartup(error) error { return nil }

func (coverageLifecycleStub) CurrentRuntimeBundle() factoryruntime.RuntimeRecord { return nil }

type javascriptExecutionStub struct {
	executionMethodsStub
	start      factorysessions.SyncStartResult
	startErr   error
	request    factorysessions.StartRequest
	result     factorysessions.ResultReadResult
	resultErr  error
	session    factorysessions.SessionReadResult
	sessionErr error
}

func (stub *javascriptExecutionStub) StartSync(_ context.Context, request factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	stub.request = request
	return stub.start, stub.startErr
}

func (stub *javascriptExecutionStub) GetResult(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	return stub.result, stub.resultErr
}

func (stub *javascriptExecutionStub) GetSession(context.Context, string) (factorysessions.SessionReadResult, error) {
	return stub.session, stub.sessionErr
}
