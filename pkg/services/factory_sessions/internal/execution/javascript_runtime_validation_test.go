package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestExecutionServiceAndHelperNormalization(t *testing.T) {
	t.Parallel()
	t.Run("execution service providers", testExecutionServiceProviders)
	t.Run("explicit persistence choices", testExecutionServicePersistenceChoices)
	t.Run("child executor and smoke provider", testExecutionServiceChildExecutorHelpers)
	t.Run("source request helpers", testExecutionServiceSourceRequestHelpers)
}

func testExecutionServiceProviders(t *testing.T) {
	t.Helper()

	fakeService, err := newExecutionService(ExecutionProviderFake, serviceConfig{})
	if err != nil {
		t.Fatalf("NewExecutionService(fake): %v", err)
	}
	if _, ok := fakeService.(*FakeService); !ok {
		t.Fatalf("fake provider type = %T, want *FakeService", fakeService)
	}

	projectRoot := t.TempDir()
	persistence, err := ProjectPersistence(projectRoot, testRuntimePersistenceStoreFactory)
	if err != nil {
		t.Fatalf("ProjectPersistence: %v", err)
	}
	runtimeService, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{ProjectRoot: projectRoot, Persistence: persistence, Clock: durableFixedClock{now: time.Now()}})
	if err != nil {
		t.Fatalf("NewExecutionService(runtime): %v", err)
	}
	jsService, ok := runtimeService.(*JavaScriptRuntimeService)
	if !ok {
		t.Fatalf("runtime provider type = %T, want *JavaScriptRuntimeService", runtimeService)
	}
	if jsService.persistence == nil {
		t.Fatal("expected runtime service to use the injected persisted session store")
	}

	if _, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{}); err == nil {
		t.Fatal("NewExecutionService(runtime without projectRoot) error = nil, want validation error")
	}
	if _, err := newExecutionService(ExecutionProvider("unknown"), serviceConfig{}); err == nil {
		t.Fatal("NewExecutionService(unknown) error = nil, want validation error")
	}
}

func testExecutionServiceChildExecutorHelpers(t *testing.T) {
	t.Helper()

	if err := validateChildExecutorMode(ChildExecutorModeLive); err != nil {
		t.Fatalf("validateChildExecutorMode(live) error = %v", err)
	}
	if err := validateChildExecutorMode(ChildExecutorModeFake); err != nil {
		t.Fatalf("validateChildExecutorMode(fake) error = %v", err)
	}
	if err := validateChildExecutorMode("nonsense"); err == nil {
		t.Fatal("validateChildExecutorMode(nonsense) error = nil, want validation error")
	}

	smoke := SmokeLiveChildProvider()
	response, err := smoke.Infer(context.Background(), workerexecution.ProviderInferenceRequest{})
	if err != nil {
		t.Fatalf("SmokeLiveChildProvider().Infer: %v", err)
	}
	if response.ProviderSession == nil || response.ProviderSession.ID == "" {
		t.Fatalf("provider session = %#v, want stable session metadata", response.ProviderSession)
	}
}

func testExecutionServiceSourceRequestHelpers(t *testing.T) {
	t.Helper()

	inlineReq := startSourceRequest(Source{
		Kind: factory.WorkflowSourceKindInlineWorkflow,
		InlineWorkflow: &InlineWorkflowSource{
			InlineSource: "return 1;",
		},
	})
	if inlineReq.Value != "return 1;" || inlineReq.InlineSource != "return 1;" {
		t.Fatalf("startSourceRequest(inline) = %#v", inlineReq)
	}
	if resolutionOrderForLookupStage(factory.WorkflowSourceLookupStageProjectClaude) != "PROJECT_CLAUDE_WORKFLOWS" {
		t.Fatal("unexpected lookup stage mapping for project claude")
	}
	if resolutionOrderForLookupStage(factory.WorkflowSourceLookupStageNamedJavaScript) != "BUILTIN_GLOBAL_JAVASCRIPT_FACTORIES" {
		t.Fatal("unexpected lookup stage mapping for named javascript")
	}
	if resolutionOrderForLookupStage("unknown") != "" {
		t.Fatal("unexpected lookup stage mapping for unknown stage")
	}
}

func TestNormalizationAndIdempotencyHelpers(t *testing.T) {
	t.Parallel()
	t.Run("approve and source normalization", testNormalizationApproveAndSourceBranches)
	t.Run("canonical json and replay helpers", testNormalizationCanonicalAndReplayBranches)
	t.Run("idempotency hash stability", testNormalizationIdempotencyHashBranches)
}

func testNormalizationApproveAndSourceBranches(t *testing.T) {
	t.Helper()

	approved, err := NormalizeApproveRequest(ApproveRequest{
		ControlRequest:    ControlRequest{RequestID: "  ctrl-1  ", Reason: "  ok  "},
		ApprovalPreviewID: "  preview-1  ",
		ApprovedPolicy:    map[string]any{"policyHash": " hash-1 "},
	})
	if err != nil {
		t.Fatalf("NormalizeApproveRequest: %v", err)
	}
	if approved.RequestID != "ctrl-1" || approved.Reason != "ok" || approved.ApprovalPreviewID != "preview-1" {
		t.Fatalf("normalized approve request = %#v", approved)
	}

	inlineSource, err := normalizeSourceForIdempotency(Source{
		Kind: factory.WorkflowSourceKindInlineWorkflow,
		InlineWorkflow: &InlineWorkflowSource{
			InlineSource: " return 1; ",
			Dialect:      " you-workflow-v1 ",
			Entrypoint:   " default ",
			Metadata: map[string]string{
				"b": "2",
				"a": "1",
			},
		},
	})
	if err != nil {
		t.Fatalf("normalizeSourceForIdempotency(inline): %v", err)
	}
	inlineWorkflow, ok := inlineSource["inlineWorkflow"].(map[string]any)
	if !ok || inlineWorkflow["inlineSource"] != "return 1;" {
		t.Fatalf("inline workflow projection = %#v", inlineSource["inlineWorkflow"])
	}
	metadata, ok := inlineWorkflow["metadata"].(map[string]string)
	if !ok || len(metadata) != 2 || metadata["a"] != "1" || metadata["b"] != "2" {
		t.Fatalf("inline workflow metadata = %#v", inlineWorkflow["metadata"])
	}

	if _, err := normalizeSourceForIdempotency(Source{Kind: factory.WorkflowSourceKindInlineWorkflow}); err == nil {
		t.Fatal("normalizeSourceForIdempotency(missing inline) error = nil, want validation error")
	}
}

func testNormalizationCanonicalAndReplayBranches(t *testing.T) {
	t.Helper()

	if _, err := canonicalizeRawJSON(json.RawMessage("{")); err == nil {
		t.Fatal("canonicalizeRawJSON(invalid) error = nil, want parse error")
	}
	canonical, err := canonicalizeRawJSON(json.RawMessage(`{"b":2,"a":[{"d":4,"c":3}]}`))
	if err != nil {
		t.Fatalf("canonicalizeRawJSON(valid): %v", err)
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		t.Fatalf("json.Marshal(canonical): %v", err)
	}
	if string(encoded) != `{"a":[{"c":3,"d":4}],"b":2}` {
		t.Fatalf("canonical json = %s, want sorted object keys", encoded)
	}

	if err := CheckSyncStartReplayMode(&AsyncStartResult{}, nil, false); !errors.Is(err, ErrExecutionRequestIDConflict) {
		t.Fatalf("CheckSyncStartReplayMode(async replay mismatch) = %v, want ErrExecutionRequestIDConflict", err)
	}
	if err := CheckSyncStartReplayMode(nil, nil, false); err != nil {
		t.Fatalf("CheckSyncStartReplayMode(empty replay) = %v, want nil", err)
	}
	if err := CheckAsyncStartReplayMode(nil); !errors.Is(err, ErrExecutionRequestIDConflict) {
		t.Fatalf("CheckAsyncStartReplayMode(nil) = %v, want ErrExecutionRequestIDConflict", err)
	}
}

func testNormalizationIdempotencyHashBranches(t *testing.T) {
	t.Helper()

	hashA, err := IdempotencyTupleHash(inlineWorkflowStartRequest("req-hash-1", simpleFinalWorkflowSource, map[string]any{"x": 1}, map[string]any{"policyHash": "same"}))
	if err != nil {
		t.Fatalf("IdempotencyTupleHash(first): %v", err)
	}
	hashB, err := IdempotencyTupleHash(inlineWorkflowStartRequest("req-hash-1", simpleFinalWorkflowSource, map[string]any{"x": 1}, map[string]any{"policyHash": "same"}))
	if err != nil {
		t.Fatalf("IdempotencyTupleHash(second): %v", err)
	}
	if hashA != hashB {
		t.Fatalf("tuple hashes differ: %q vs %q", hashA, hashB)
	}
}

func TestValidateNamedAgentPresetsRejectsUnknownPresetBeforeStart(t *testing.T) {
	t.Parallel()
	err := validateNamedAgentPresets(
		map[string]interfaces.FactoryOrchestratorJavaScriptAgent{
			"reviewer": {Preset: "missing-preset"},
		},
		map[string]struct{}{"known-preset": {}},
	)
	if err == nil || !strings.Contains(err.Error(), `factory agent "reviewer" references unknown operator worker preset "missing-preset"`) {
		t.Fatalf("validateNamedAgentPresets() error = %v", err)
	}
}

func TestNormalizeStartRequestAndErrorHelpers(t *testing.T) {
	t.Parallel()
	t.Run("normalize valid and invalid start requests", testNormalizeStartRequestBranches)
	t.Run("normalize source and child executor mode", testNormalizeSourceAndExecutorModeBranches)
	t.Run("error helper strings", testControlAndValidationErrorHelpers)
}

func testNormalizeStartRequestBranches(t *testing.T) {
	t.Helper()

	normalized, err := NormalizeStartRequest(StartRequest{
		RequestID: " req-1 ",
		Source: Source{
			Kind:          factory.WorkflowSourceKindFactoryInline,
			FactoryInline: json.RawMessage(`{"b":2,"a":1}`),
		},
		Orchestrator: &OrchestratorOverride{
			Kind: " custom ",
			Raw:  json.RawMessage(`{"z":2,"a":1}`),
		},
		Runtime: &RuntimeOptions{ChildExecutorMode: " live-provider "},
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest(factory inline): %v", err)
	}
	if normalized.RequestID != "req-1" {
		t.Fatalf("requestID = %q, want req-1", normalized.RequestID)
	}
	if string(normalized.Source.FactoryInline) == "" {
		t.Fatalf("factory inline unexpectedly empty: %#v", normalized.Source)
	}
	if normalized.Runtime == nil || normalized.Runtime.ChildExecutorMode != ChildExecutorModeLive {
		t.Fatalf("runtime = %#v, want live mode", normalized.Runtime)
	}
	if normalized.Orchestrator == nil || normalized.Orchestrator.Kind != "custom" {
		t.Fatalf("orchestrator = %#v, want trimmed kind", normalized.Orchestrator)
	}

	if _, err := NormalizeStartRequest(StartRequest{}); err == nil {
		t.Fatal("NormalizeStartRequest(missing requestID) error = nil, want validation error")
	}
	if _, err := NormalizeStartRequest(StartRequest{
		RequestID: "req-2",
		Source: Source{
			Kind:         factory.WorkflowSourceKindWorkflowFile,
			WorkflowFile: " path/to/workflow.js ",
		},
		Orchestrator: &OrchestratorOverride{
			Raw: json.RawMessage("{"),
		},
	}); err == nil {
		t.Fatal("NormalizeStartRequest(invalid orchestrator) error = nil, want validation error")
	}
}

func testNormalizeSourceAndExecutorModeBranches(t *testing.T) {
	t.Helper()

	if _, err := normalizeSource(Source{Kind: factory.WorkflowSourceKindWorkflowName, WorkflowName: "  "}); err == nil {
		t.Fatal("normalizeSource(empty workflow name) error = nil, want validation error")
	}
	if got := normalizeChildExecutorMode(" live-provider "); got != ChildExecutorModeLive {
		t.Fatalf("normalizeChildExecutorMode = %q, want live", got)
	}
	if got := resolveChildExecutorMode("fake", StartRequest{Runtime: &RuntimeOptions{ChildExecutorMode: "live-provider"}}); got != ChildExecutorModeLive {
		t.Fatalf("resolveChildExecutorMode = %q, want live override", got)
	}
}

func testControlAndValidationErrorHelpers(t *testing.T) {
	t.Helper()

	var controlErr *ControlError
	if controlErr.Error() != "" {
		t.Fatalf("nil control error message = %q, want empty", controlErr.Error())
	}
	controlErr = &ControlError{Outcome: LifecycleControlOutcomeConflict}
	if controlErr.Error() != string(LifecycleControlOutcomeConflict) {
		t.Fatalf("control error message = %q, want outcome text", controlErr.Error())
	}
	var validationErr *ValidationError
	if validationErr.Error() != "" {
		t.Fatalf("nil validation error message = %q, want empty", validationErr.Error())
	}
}

func TestRuntimeAndValidationHelperBranches(t *testing.T) {
	t.Parallel()
	t.Run("child executor hooks and marshal args", testRuntimeHookAndMarshalBranches)
	t.Run("workflow metadata and source validation errors", testRuntimeMetadataAndSourceValidationBranches)
	t.Run("policy validation errors", testRuntimePolicyValidationBranches)
}

func testRuntimeHookAndMarshalBranches(t *testing.T) {
	t.Helper()

	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	if hooks := service.childExecutorHooks(ChildExecutorModeFake, "session-fake"); hooks.NewChildExecutor != nil {
		t.Fatalf("fake hooks = %#v, want no child executor override", hooks)
	}
	liveService := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot:        t.TempDir(),
		InvocationExecutor: constructorInvocationExecutor{},
	})

	if hooks := liveService.childExecutorHooks(ChildExecutorModeLive, "session-live"); hooks.NewChildExecutor == nil {
		t.Fatal("expected live child executor hook")
	}

	if raw, err := marshalStartArgs(nil); err != nil || raw != nil {
		t.Fatalf("marshalStartArgs(nil) = %q, %v, want nil,nil", raw, err)
	}
	if _, err := marshalStartArgs(map[string]any{"bad": func() {}}); err == nil {
		t.Fatal("marshalStartArgs(non-json) error = nil, want validation error")
	}
}

func testRuntimeMetadataAndSourceValidationBranches(t *testing.T) {
	t.Helper()

	metadata := workflowMetadataFromResolved(ResolvedSource{
		SourceRef: "resolved-ref",
		Metadata: map[string]string{
			"project": "root",
		},
	}, StartRequest{
		Source: Source{
			WorkflowName: "named-workflow",
			InlineWorkflow: &InlineWorkflowSource{
				Metadata: map[string]string{"team": "ops"},
			},
		},
	})
	if metadata["name"] != "named-workflow" || metadata["team"] != "ops" || metadata["project"] != "root" {
		t.Fatalf("workflow metadata = %#v", metadata)
	}

	if err := validationErrorFromSourceIssues(nil); err == nil || err.Error() == "" {
		t.Fatalf("validationErrorFromSourceIssues(nil) = %v, want default validation error", err)
	}
	if err := validationErrorFromSourceIssues([]factory.WorkflowValidationIssue{{Message: "bad source", Line: 3, Column: 5}}); err == nil || err.Error() != "bad source (line 3, column 5)" {
		t.Fatalf("validationErrorFromSourceIssues(location) = %v", err)
	}
	if err := validationErrorFromSourceIssues([]factory.WorkflowValidationIssue{
		{Code: factory.WorkflowValidationCodeImportNotFound, Message: "missing module"},
	}); err == nil || err.Error() != "[workflow.source.notFound] missing module" {
		t.Fatalf("validationErrorFromSourceIssues(code) = %v", err)
	}
	if err := validationErrorFromSourceIssues([]factory.WorkflowValidationIssue{{}}); err == nil || err.Error() != "workflow source validation failed" {
		t.Fatalf("validationErrorFromSourceIssues(default message) = %v", err)
	}
}

func testRuntimePolicyValidationBranches(t *testing.T) {
	t.Helper()

	if err := validationErrorFromPolicyIssues(nil); err != nil {
		t.Fatalf("validationErrorFromPolicyIssues(nil) = %v, want nil", err)
	}
	if err := validationErrorFromPolicyIssues([]factory.JavaScriptPolicyIssue{{Message: "blocked"}}); err == nil || err.Error() != "blocked" {
		t.Fatalf("validationErrorFromPolicyIssues = %v, want blocked", err)
	}
	if err := validationErrorFromPolicyIssues([]factory.JavaScriptPolicyIssue{{}}); err == nil || err.Error() != "requested policy is invalid" {
		t.Fatalf("validationErrorFromPolicyIssues(default message) = %v", err)
	}
}

func TestStartSourceRequestAndResolutionOrderBranches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		source Source
		want   string
	}{
		{Source{Kind: factory.WorkflowSourceKindFactoryID, FactoryID: "factory-1"}, "factory-1"},
		{Source{Kind: factory.WorkflowSourceKindFactoryInline, FactoryInline: json.RawMessage(`{"name":"factory"}`)}, `{"name":"factory"}`},
		{Source{Kind: factory.WorkflowSourceKindWorkflowFile, WorkflowFile: "wf.js"}, "wf.js"},
		{Source{Kind: factory.WorkflowSourceKindWorkflowName, WorkflowName: "name"}, "name"},
	}
	for _, tc := range cases {
		if got := startSourceRequest(tc.source); got.Value != tc.want {
			t.Fatalf("startSourceRequest(%s) value = %q, want %q", tc.source.Kind, got.Value, tc.want)
		}
	}
	if got := startSourceRequest(Source{Kind: factory.WorkflowSourceKindInlineWorkflow}); got.Value != "" || got.InlineSource != "" {
		t.Fatalf("startSourceRequest(missing inline) = %#v, want empty inline request", got)
	}

	stages := []factory.WorkflowSourceLookupStage{
		factory.WorkflowSourceLookupStageProjectClaude,
		factory.WorkflowSourceLookupStageExplicitSourceKind,
		factory.WorkflowSourceLookupStageGlobalUser,
		factory.WorkflowSourceLookupStagePackageRelative,
		factory.WorkflowSourceLookupStageNamedJavaScript,
		factory.WorkflowSourceLookupStageExplicitFactory,
	}
	for _, stage := range stages {
		if resolutionOrderForLookupStage(stage) == "" {
			t.Fatalf("resolutionOrderForLookupStage(%q) returned empty mapping", stage)
		}
	}
}

func TestNormalizeStartRequestAdditionalSourceBranches(t *testing.T) {
	t.Parallel()
	cases := []StartRequest{
		{
			RequestID: "req-file",
			Source: Source{
				Kind:         factory.WorkflowSourceKindWorkflowFile,
				WorkflowFile: " workflow.js ",
			},
		},
		{
			RequestID: "req-name",
			Source: Source{
				Kind:         factory.WorkflowSourceKindWorkflowName,
				WorkflowName: " named-workflow ",
			},
		},
		{
			RequestID: "req-inline",
			Source: Source{
				Kind: factory.WorkflowSourceKindInlineWorkflow,
				InlineWorkflow: &InlineWorkflowSource{
					InlineSource: " return 1; ",
					Dialect:      " you-workflow-v1 ",
					Entrypoint:   " default ",
					Metadata:     map[string]string{"k": "v"},
				},
			},
		},
	}
	for _, req := range cases {
		normalized, err := NormalizeStartRequest(req)
		if err != nil {
			t.Fatalf("NormalizeStartRequest(%s): %v", req.RequestID, err)
		}
		if normalized.Source.Kind != req.Source.Kind {
			t.Fatalf("normalized source kind = %q, want %q", normalized.Source.Kind, req.Source.Kind)
		}
	}
	if _, err := normalizeSource(Source{}); err == nil {
		t.Fatal("normalizeSource(unknown kind) error = nil, want validation error")
	}
}

func TestSmallHelperBranches(t *testing.T) {
	t.Parallel()
	if got := resolvedDialect(ResolvedSource{Dialect: "custom"}); got != "custom" {
		t.Fatalf("resolvedDialect(custom) = %q, want custom", got)
	}
	if got := resolvedDialect(ResolvedSource{}); got != "you-workflow-v1" {
		t.Fatalf("resolvedDialect(default) = %q, want you-workflow-v1", got)
	}
	if id, err := NormalizeSessionID(" session-1 "); err != nil || id != "session-1" {
		t.Fatalf("NormalizeSessionID = %q, %v, want session-1,nil", id, err)
	}
	if _, err := NormalizeSessionID("   "); err == nil {
		t.Fatal("NormalizeSessionID(empty) error = nil, want validation error")
	}
}

func TestJavaScriptRuntimeService_StartSync_WorkflowFilePolicyDeniesDisallowedModel(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	workflowPath := filepath.Join(projectRoot, "workflow.js")
	workflowSource := `agent.run({
  prompt: "summarize workflows",
  label: "denied-model",
  model: "gpt-denied",
});
return { ok: true };`
	if err := os.WriteFile(workflowPath, []byte(workflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	defaultPolicy := json.RawMessage(`{
  "mode":"READ_ONLY",
  "maxAgents":4,
  "concurrency":2,
  "allowedModels":["gpt-allowed"],
  "allowedReasoningEfforts":["low"]
}`)
	var requestedPolicy map[string]any
	if err := json.Unmarshal(defaultPolicy, &requestedPolicy); err != nil {
		t.Fatalf("unmarshal default policy: %v", err)
	}

	workflows := policyDeniedModelWorkflows(workflowSource)
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: ChildExecutorModeFake,
		Workflows:         workflows,
	})

	started, err := service.StartSync(context.Background(), StartRequest{
		RequestID: "req-policy-denied-model",
		Source: Source{
			Kind:         factory.WorkflowSourceKindWorkflowFile,
			WorkflowFile: workflowPath,
			InlineWorkflow: &InlineWorkflowSource{
				DefaultPolicy: defaultPolicy,
			},
		},
		Args:            map[string]any{"prompt": "hello"},
		RequestedPolicy: requestedPolicy,
		Runtime:         &RuntimeOptions{ChildExecutorMode: ChildExecutorModeFake},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if started.Status != string(LifecycleStatusFailed) {
		t.Fatalf("status = %q, want FAILED; outcome = %#v", started.Status, started)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	result, err := service.GetResult(context.Background(), started.SessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	failureMessage := ""
	if result.Failure != nil {
		failureMessage = result.Failure.Message
	} else if session.Failure != nil {
		failureMessage = session.Failure.Message
	}
	if !strings.Contains(failureMessage, `policy denied: model "gpt-denied" is not listed in allowedModels`) {
		t.Fatalf("session failure = %#v result failure = %#v, want stable policy diagnostic", session.Failure, result.Failure)
	}
}

func policyDeniedModelWorkflows(workflowSource string) factory.JavaScriptWorkflows {
	policyMessage := `policy denied: model "gpt-denied" is not listed in allowedModels`
	return factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		ResolveSourceFunc: func(
			request factory.WorkflowSourceRequest,
			_ factory.WorkflowSourceContext,
		) factory.WorkflowSourceResolution {
			return factory.WorkflowSourceResolution{
				RequestKind:  request.Kind,
				RequestValue: request.Value,
				ResolvedKind: request.Kind,
				SourceRef:    request.Value,
				SourceHash:   "sha256:policy-denied",
				Dialect:      "you-workflow-v1",
				Content:      workflowSource,
				Found:        true,
			}
		},
		LoadSourceFunc: func(request factory.WorkflowValidationLoadRequest) (
			factory.WorkflowValidationLoadedSource,
			[]factory.WorkflowValidationIssue,
		) {
			return factory.WorkflowValidationLoadedSource{
				SourceRef:        request.SourceRef,
				SourceHash:       "sha256:policy-denied",
				Format:           factory.WorkflowValidationFormatJavaScript,
				AuthoredSource:   request.Content,
				ExecutableSource: request.Content,
			}, nil
		},
		RunFunc: func(
			context.Context,
			factory.JavaScriptRuntimeRequest,
			factory.JavaScriptRuntimeHooks,
		) (factory.JavaScriptRuntimeOutcome, error) {
			return factory.JavaScriptRuntimeOutcome{
				Failure: factory.JavaScriptRuntimeFailure{
					Code:    factory.JavaScriptRuntimeCodeScriptError,
					Message: policyMessage,
				},
			}, nil
		},
	}
}

func findDispatchInterruptedEventPayload(t *testing.T, events []json.RawMessage, dispatchID string) dispatchInterruptedEventPayload {
	t.Helper()
	for _, raw := range events {
		var envelope canonicalFactoryEvent
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if envelope.Type != "DISPATCH_INTERRUPTED" {
			continue
		}
		if envelope.Context.DispatchID == nil || *envelope.Context.DispatchID != dispatchID {
			continue
		}
		var payload dispatchInterruptedEventPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			t.Fatalf("unmarshal DISPATCH_INTERRUPTED payload: %v", err)
		}
		return payload
	}
	t.Fatalf("DISPATCH_INTERRUPTED event for %s not found in %#v", dispatchID, events)
	return dispatchInterruptedEventPayload{}
}

func containsEventType(events []json.RawMessage, eventType string) bool {
	for _, raw := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type == eventType {
			return true
		}
	}
	return false
}

func findDispatchByID(dispatches []DispatchSummary, dispatchID string) *DispatchSummary {
	for index := range dispatches {
		if dispatches[index].ID == dispatchID {
			return &dispatches[index]
		}
	}
	return nil
}

func TestCheckpointEventProjection_BuildsCanonicalCheckpointEvents(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-checkpoint-events-001"
	state := &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusInterrupted,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          "you-workflow-v1",
			Phase:            "execute",
			SourceHash:       "sha256:fixture",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		result: ResultReadResult{
			SessionID:     sessionID,
			SessionStatus: LifecycleStatusInterrupted,
			ResultStatus:  ResultStatusPartial,
		},
		checkpointSummary: &factory.JavaScriptCheckpointSummary{
			CheckpointID: "checkpoint-1",
			CreatedAt:    startedAt.Add(time.Minute),
		},
		runtimeRecords: []factory.JavaScriptRuntimeRecord{{
			Kind: factory.JavaScriptRecordKindCheckpoint,
			Checkpoint: &factory.JavaScriptCheckpointRecord{
				ID:      "checkpoint-1",
				Label:   "after-first-child",
				Summary: "checkpoint after first child",
			},
		}},
	}
	checkpoints := checkpointEventsFromRuntimeState(state)
	if len(checkpoints) != 1 || checkpoints[0].CheckpointID != "checkpoint-1" {
		t.Fatalf("checkpoint events = %#v", checkpoints)
	}
	if checkpoints[0].ResumabilityStatus != "RESUMABLE" {
		t.Fatalf("resumability = %q, want RESUMABLE", checkpoints[0].ResumabilityStatus)
	}

	events := BuildCanonicalRuntimeSessionEvents(state.session, state.result, runtimeDispatchEventInputFromState(state))
	events = appendCanonicalOrchestratorCheckpointEvents(events, state.session, checkpoints, canonicalEventSourceRuntimeService)
	found := false
	for _, raw := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type == "ORCHESTRATOR_CHECKPOINT_WRITTEN" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected ORCHESTRATOR_CHECKPOINT_WRITTEN canonical event")
	}
}

func TestPhaseEventProjection_PreservesOrderedRunningAndTerminalPhases(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	session := SessionReadResult{
		SessionID:        "dur-sess-phase-events-001",
		Status:           LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Dialect:          "you-workflow-v1",
		Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		PhaseSummaries: []PhaseSummary{
			{Phase: "setup"}, {Phase: " "}, {Phase: "execute"},
		},
	}
	events := appendCanonicalOrchestratorPhaseEvents(nil, session, canonicalEventSourceRuntimeService)
	if got, want := phaseEventStatuses(t, events), []string{"setup:COMPLETED", "execute:ACTIVE"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("running phases = %v, want %v", got, want)
	}

	session.Status = LifecycleStatusSucceeded
	events = appendCanonicalOrchestratorPhaseEvents(nil, session, canonicalEventSourceRuntimeService)
	if got, want := phaseEventStatuses(t, events), []string{"setup:COMPLETED", "execute:COMPLETED"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal phases = %v, want %v", got, want)
	}
	if got := appendCanonicalOrchestratorPhaseEvents(events, SessionReadResult{}, canonicalEventSourceRuntimeService); len(got) != len(events) {
		t.Fatalf("empty phase projection changed event count from %d to %d", len(events), len(got))
	}
}

func phaseEventStatuses(t *testing.T, events []json.RawMessage) []string {
	t.Helper()
	statuses := make([]string, 0, len(events))
	for _, raw := range events {
		var event struct {
			Context struct {
				PhaseID *string `json:"phaseId"`
			} `json:"context"`
			Payload struct {
				PhaseStatus string `json:"phaseStatus"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode phase event: %v", err)
		}
		if event.Context.PhaseID != nil && event.Payload.PhaseStatus != "" {
			statuses = append(statuses, *event.Context.PhaseID+":"+event.Payload.PhaseStatus)
		}
	}
	return statuses
}

func TestJavaScriptRuntimeServiceWriteRecordingUsesCanonicalSnapshotAndCorrelatesFailure(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-1234567890abcdef1234567890abcdef"
	observedAt := time.Date(2026, 7, 12, 16, 30, 0, 0, time.UTC)
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	service.sessions[sessionID] = &runtimeSessionState{
		session:        SessionReadResult{SessionID: sessionID, Status: LifecycleStatusSucceeded, OrchestratorKind: interfaces.OrchestratorKindJavaScript, ResolvedSource: ResolvedSource{SourceRef: "workflow/audit.js"}, SourceHash: "sha256:" + strings.Repeat("1", 64), Policy: PolicyProjection{EffectiveHash: "sha256:" + strings.Repeat("2", 64)}},
		startRequest:   &StartRequest{Args: map[string]any{"customer": "north"}},
		artifacts:      []ArtifactSummary{{ID: "artifact-1", Kind: "RESULT", Visibility: "PUBLIC", ContentHash: "sha256:" + strings.Repeat("3", 64), SizeBytes: 2, CreatedAt: &observedAt}},
		events:         []json.RawMessage{json.RawMessage(`{"id":"event-1","type":"SESSION_COMPLETED","context":{"sequence":0,"eventTime":"2026-07-12T16:30:00Z"},"payload":{"artifactIds":["artifact-1"]}}`)},
		runtimeRecords: []factory.JavaScriptRuntimeRecord{{Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "checkpoint-secret", State: map[string]any{"secret": "raw-state"}}}},
	}
	path := filepath.Join(t.TempDir(), "session.recording.json")
	if err := service.WriteRecording(context.Background(), sessionID, path); err != nil {
		t.Fatalf("WriteRecording: %v", err)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(encoded), "checkpoint-secret") || strings.Contains(string(encoded), "raw-state") {
		t.Fatalf("recording leaked runtime state: %s", encoded)
	}
	badPath := filepath.Join(t.TempDir(), "missing", "\x00invalid")
	err = service.WriteRecording(context.Background(), sessionID, badPath)
	var recordingErr *RecordingError
	if !errors.As(err, &recordingErr) || recordingErr.SessionID != sessionID || recordingErr.Path != badPath {
		t.Fatalf("WriteRecording failure = %#v", err)
	}
	read, readErr := service.GetSession(context.Background(), sessionID)
	if readErr != nil || read.Status != LifecycleStatusSucceeded {
		t.Fatalf("live session changed after recording failure: read=%#v err=%v", read, readErr)
	}
}
