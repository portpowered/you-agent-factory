package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"strings"
	"testing"
	"time"
)

func TestProjectResultRead_ModePartialAndFinal(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-run-n-001")

	partial, err := service.GetResult(context.Background(), "dur-sess-js-run-n-001", ResultRequest{Mode: ResultModePartial})
	if err != nil {
		t.Fatalf("GetResult partial: %v", err)
	}
	if partial.ResultStatus != ResultStatusPartial {
		t.Fatalf("partial status = %q, want PARTIAL", partial.ResultStatus)
	}
	if len(partial.PrimaryResult) == 0 {
		t.Fatal("partial primaryResult missing")
	}
	if partial.Mode != ResultModePartial {
		t.Fatalf("mode = %q, want partial", partial.Mode)
	}

	final, err := service.GetResult(context.Background(), "dur-sess-js-run-n-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult final: %v", err)
	}
	if final.ResultStatus != ResultStatusNotReady {
		t.Fatalf("final status = %q, want NOT_READY", final.ResultStatus)
	}
	if len(final.PrimaryResult) != 0 {
		t.Fatal("final primaryResult should be omitted for running session")
	}
	if final.Availability == nil || final.Availability.Reason != "RESULT_NOT_READY" {
		t.Fatalf("availability = %#v, want RESULT_NOT_READY", final.Availability)
	}
}

func TestProjectResultRead_TerminalFinalAndUnavailable(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")

	final, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult terminal final: %v", err)
	}
	if final.ResultStatus != ResultStatusFinal {
		t.Fatalf("status = %q, want FINAL", final.ResultStatus)
	}
	if len(final.PrimaryResult) == 0 {
		t.Fatal("final primaryResult missing")
	}

	startAsyncByRequestID(t, service, "req-petri-cancel-001")
	unavailable, err := service.GetResult(context.Background(), "dur-sess-petri-cancel-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult unavailable: %v", err)
	}
	if unavailable.ResultStatus != ResultStatusUnavailable {
		t.Fatalf("status = %q, want UNAVAILABLE", unavailable.ResultStatus)
	}
	if unavailable.Availability == nil || unavailable.Availability.Reason != "SESSION_CANCELED" {
		t.Fatalf("availability = %#v", unavailable.Availability)
	}
}

func TestProjectResultRead_FailedWithPartialHonorsPartialMode(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-js-failed-partial-001")

	result, err := service.GetResult(context.Background(), "dur-sess-js-failed-partial-001", ResultRequest{Mode: ResultModePartial})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != ResultStatusFailedWithPartial {
		t.Fatalf("status = %q, want FAILED_WITH_PARTIAL", result.ResultStatus)
	}
	if len(result.PrimaryResult) == 0 {
		t.Fatal("partial primaryResult missing")
	}
	if result.Failure == nil || !result.Failure.PartialResultAvailable {
		t.Fatal("failure detail missing")
	}
}

func TestRuntimeRecordProjection_RebuildsCanonicalPhaseCheckpointAndDispatchEvents(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Minute)
	checkpointAt := startedAt.Add(time.Minute)
	sessionID := "dur-sess-runtime-record-projection-001"
	checkpoint := factory.JavaScriptRuntimeRecord{
		Sequence: 2,
		Kind:     factory.JavaScriptRecordKindCheckpoint,
		Checkpoint: &factory.JavaScriptCheckpointRecord{
			ID: "checkpoint-1", Summary: "checkpoint summary",
		},
	}
	state := &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusSucceeded,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          "you-workflow-v1",
			SourceHash:       "sha256:runtime-records",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: &finishedAt},
		},
		result: ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusFinal,
		},
		dispatches: []DispatchSummary{{
			ID: "dispatch-1", Status: DispatchStatusCompleted,
			Label: "summarize", RunnerID: "runner-1", PresetID: "preset-1",
			ModelProvider: "provider-1", Model: "model-1", ReasoningEffort: "medium",
			Provider: "provider-1",
		}},
		checkpointSummary: &factory.JavaScriptCheckpointSummary{
			CheckpointID: "checkpoint-1", CreatedAt: checkpointAt,
		},
		runtimeRecords: []factory.JavaScriptRuntimeRecord{
			{Sequence: 1, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: " plan "}},
			{Sequence: 1, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: " "}},
			checkpoint,
			checkpoint,
			{Sequence: 3, Kind: factory.JavaScriptRecordKindPhase, Phase: &factory.JavaScriptPhaseRecord{Name: "execute"}},
			{Sequence: 4, Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: " "}},
		},
	}

	input := runtimeDispatchEventInputFromState(state)
	if got, want := len(input.RuntimeRecords), 5; got != want {
		t.Fatalf("unique runtime records = %d, want %d", got, want)
	}
	events := rebuildRuntimeSessionCanonicalEvents(state)
	var eventTypes []string
	for _, raw := range events {
		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode canonical event: %v", err)
		}
		eventTypes = append(eventTypes, event.Type)
	}
	for _, want := range []string{
		"ORCHESTRATOR_PHASE_CHANGED",
		"ORCHESTRATOR_CHECKPOINT_WRITTEN",
		"DISPATCH_QUEUED",
		"DISPATCH_RECONCILED",
		"SESSION_COMPLETED",
	} {
		found := false
		for _, eventType := range eventTypes {
			if eventType == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("event types = %v, missing %s", eventTypes, want)
		}
	}
}

func TestProjectResultRead_IncludeArtifactsShaping(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-success-001")

	excluded, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{
		Mode:             ResultModeFinal,
		IncludeArtifacts: false,
	})
	if err != nil {
		t.Fatalf("GetResult excluded: %v", err)
	}
	if excluded.IncludeArtifacts {
		t.Fatal("includeArtifacts = true, want false")
	}
	if len(excluded.ArtifactRefs) != 0 {
		t.Fatalf("artifactRefs = %#v, want omitted", excluded.ArtifactRefs)
	}
	if len(excluded.ArtifactIDs) != 1 || excluded.ArtifactIDs[0] != "art-petri-final-001" {
		t.Fatalf("artifactIds = %#v", excluded.ArtifactIDs)
	}

	included, err := service.GetResult(context.Background(), "dur-sess-petri-success-001", ResultRequest{
		Mode:             ResultModeFinal,
		IncludeArtifacts: true,
	})
	if err != nil {
		t.Fatalf("GetResult included: %v", err)
	}
	if !included.IncludeArtifacts {
		t.Fatal("includeArtifacts = false, want true")
	}
	if len(included.ArtifactRefs) != 1 || included.ArtifactRefs[0].ID != "art-petri-final-001" {
		t.Fatalf("artifactRefs = %#v", included.ArtifactRefs)
	}
	if len(included.ArtifactIDs) != 0 {
		t.Fatalf("artifactIds = %#v, want omitted when refs included", included.ArtifactIDs)
	}
}

func TestProjectResultRead_NotReadyRunningSession(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	startAsyncByRequestID(t, service, "req-petri-run-001")

	result, err := service.GetResult(context.Background(), "dur-sess-petri-run-001", ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != ResultStatusNotReady {
		t.Fatalf("status = %q, want NOT_READY", result.ResultStatus)
	}
	if result.Availability == nil || result.Availability.Message == "" {
		t.Fatal("availability missing")
	}
}

func TestProjectResultRead_DefaultsToFinalMode(t *testing.T) {
	t.Parallel()
	canonical := ResultReadResult{
		SessionID:     "dur-sess-001",
		ResultStatus:  ResultStatusFinal,
		SessionStatus: LifecycleStatusSucceeded,
		PrimaryResult: json.RawMessage(`[{"type":"text","text":"done"}]`),
	}
	session := SessionReadResult{
		SessionID: "dur-sess-001",
		Status:    LifecycleStatusSucceeded,
		ResultSummary: &ResultSummary{
			ResultStatus: string(ResultStatusFinal),
		},
	}

	projected, err := ProjectResultRead(canonical, session, nil, ResultRequest{})
	if err != nil {
		t.Fatalf("ProjectResultRead: %v", err)
	}
	if projected.Mode != ResultModeFinal {
		t.Fatalf("mode = %q, want final", projected.Mode)
	}
	if projected.ResultStatus != ResultStatusFinal {
		t.Fatalf("status = %q, want FINAL", projected.ResultStatus)
	}
}

func TestJavaScriptRuntimeService_ReplayAndReadErrorBranches(t *testing.T) {
	t.Parallel()
	service := newDefaultJavaScriptRuntimeService(t)
	req := inlineWorkflowStartRequest(
		"req-runtime-replay-001",
		simpleFinalWorkflowSource,
		map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
		nil,
	)

	first, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("StartAsync(first): %v", err)
	}
	second, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("StartAsync(replay): %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("replay sessionID = %q, want %q", second.SessionID, first.SessionID)
	}
	waitUntilSessionStatus(t, service, first.SessionID, LifecycleStatusSucceeded, 5*time.Second)

	syncReq := inlineWorkflowStartRequest(
		"req-runtime-replay-sync-001",
		simpleFinalWorkflowSource,
		map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
		nil,
	)
	syncFirst, err := service.StartSync(context.Background(), syncReq)
	if err != nil {
		t.Fatalf("StartSync(first): %v", err)
	}
	syncSecond, err := service.StartSync(context.Background(), syncReq)
	if err != nil {
		t.Fatalf("StartSync(replay): %v", err)
	}
	if syncSecond.SessionID != syncFirst.SessionID {
		t.Fatalf("sync replay sessionID = %q, want %q", syncSecond.SessionID, syncFirst.SessionID)
	}

	if _, err := service.GetSession(context.Background(), ""); err == nil {
		t.Fatal("GetSession(empty) error = nil, want validation error")
	}
	if _, err := service.GetSession(context.Background(), "dur-sess-dddddddddddddddddddddddddddddddd"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("GetSession(missing) = %v, want ErrSessionNotFound", err)
	}
	if _, err := service.GetDispatch(context.Background(), syncFirst.SessionID, "missing-dispatch"); !errors.Is(err, ErrDispatchNotFound) {
		t.Fatalf("GetDispatch(missing) = %v, want ErrDispatchNotFound", err)
	}
	if _, err := service.GetArtifact(context.Background(), syncFirst.SessionID, "missing-artifact"); !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("GetArtifact(missing) = %v, want ErrArtifactNotFound", err)
	}
	if _, err := service.ReadEvents(context.Background(), syncFirst.SessionID, EventReconnectRequest{AfterEventID: "missing"}); !errors.Is(err, ErrReconnectCursorNotFound) {
		t.Fatalf("ReadEvents(missing cursor) = %v, want ErrReconnectCursorNotFound", err)
	}
}

func TestListingFiltersAndNormalizationBranches(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	later := now.Add(2 * time.Hour)
	summary := DurableSessionListSummary{
		SessionID:        "dur-sess-filter-1",
		Status:           LifecycleStatusRunning,
		OrchestratorKind: "JAVASCRIPT",
		ResolvedSource: ResolvedSource{
			Kind:      factory.WorkflowSourceKindWorkflowName,
			SourceRef: "customer/support",
			Metadata:  map[string]string{"project": "/workspace/customer"},
		},
		Recoverable: true,
		StaleLease:  true,
		Lifecycle: &LifecycleTimestamps{
			QueuedAt:   &now,
			StartedAt:  &later,
			UpdatedAt:  &later,
			FinishedAt: &later,
		},
	}
	yes := true
	after := now.Add(-time.Minute)
	before := later.Add(time.Minute)
	if !MatchesDurableSessionListFilters(summary, SessionListFilters{
		Statuses:          []LifecycleStatus{LifecycleStatusRunning},
		OrchestratorKinds: []string{" javascript "},
		SourceKind:        factory.WorkflowSourceKindWorkflowName,
		SourceRef:         "support",
		ProjectBoundary:   "workspace",
		Recoverable:       &yes,
		StaleLease:        &yes,
		CreatedAfter:      &after,
		CreatedBefore:     &before,
		UpdatedAfter:      &after,
		UpdatedBefore:     &before,
	}) {
		t.Fatal("expected summary to match all listing filters")
	}
	no := false
	if MatchesDurableSessionListFilters(summary, SessionListFilters{Recoverable: &no}) {
		t.Fatal("recoverable mismatch unexpectedly matched")
	}
	if containsLifecycleStatus([]LifecycleStatus{LifecycleStatusPaused}, LifecycleStatusRunning) {
		t.Fatal("containsLifecycleStatus mismatch unexpectedly matched")
	}
	if containsString([]string{"Alpha"}, "beta") {
		t.Fatal("containsString mismatch unexpectedly matched")
	}
	if firstLifecycleTimestamp(nil, &later) != &later {
		t.Fatal("firstLifecycleTimestamp did not return first non-nil value")
	}
	if latestLifecycleTimestamp(summary.Lifecycle) != &later {
		t.Fatal("latestLifecycleTimestamp did not return latest time")
	}

	normalized, err := NormalizeListSessionsRequest(ListSessionsRequest{
		Scope: SessionListScopeAll,
		Filters: SessionListFilters{
			Statuses:          []LifecycleStatus{LifecycleStatusRunning},
			OrchestratorKinds: []string{" JAVASCRIPT ", ""},
			SourceKind:        factory.WorkflowSourceKindWorkflowName,
			CreatedAfter:      &after,
			CreatedBefore:     &before,
		},
	})
	if err != nil {
		t.Fatalf("NormalizeListSessionsRequest: %v", err)
	}
	if normalized.Scope != SessionListScopeAll || len(normalized.Filters.OrchestratorKinds) != 1 {
		t.Fatalf("normalized list request = %#v", normalized)
	}
	if _, err := NormalizeListSessionsRequest(ListSessionsRequest{Scope: SessionListScope("bad")}); err == nil {
		t.Fatal("NormalizeListSessionsRequest(bad scope) error = nil, want validation error")
	}
	if _, err := NormalizeListSessionsRequest(ListSessionsRequest{
		Filters: SessionListFilters{
			SourceKind:    factory.WorkflowSourceKind("unknown"),
			CreatedAfter:  &before,
			CreatedBefore: &after,
		},
	}); err == nil {
		t.Fatal("NormalizeListSessionsRequest(invalid filters) error = nil, want validation error")
	}
}

func TestProjectionCloneHelpers(t *testing.T) {
	t.Parallel()
	observedAt := time.Now().UTC()
	artifact := artifactSummaryFromRuntimeRecord("dur-sess-helper-1", factory.JavaScriptArtifactRecord{
		ID:         "art-helper-1",
		Kind:       "RESULT",
		Visibility: "PUBLIC",
		Label:      "helper",
	}, observedAt)
	if artifact.ID != "art-helper-1" || artifact.RetrievalRef == nil || artifact.RetrievalRef.Href == "" {
		t.Fatalf("artifact summary = %#v", artifact)
	}

	js := cloneDispatchJavaScriptProjections(map[string]DispatchJavaScriptProjection{
		"disp-1": {TaskLabel: "child"},
	})
	if js["disp-1"].TaskLabel != "child" {
		t.Fatalf("cloned javascript projections = %#v", js)
	}
	transitions := cloneDispatchStatusTransitions(map[string][]DispatchStatus{
		"disp-1": {DispatchStatusQueued, DispatchStatusRunning},
	})
	if len(transitions["disp-1"]) != 2 {
		t.Fatalf("cloned transitions = %#v", transitions)
	}
}

func testJavaScriptRuntimeSyncCompletedSession(t *testing.T, service *JavaScriptRuntimeService, sessionID string) {
	t.Helper()

	session, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != LifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED", session.Status)
	}
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", session.ResultSummary)
	}
}

func testJavaScriptRuntimeSyncCompletedResult(t *testing.T, service *JavaScriptRuntimeService, sessionID string) {
	t.Helper()

	result, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != ResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
	projected := decodePrimaryResultMap(t, result.PrimaryResult)
	if projected["echo"] != "you:workflows" {
		t.Fatalf("primaryResult echo = %#v, want you:workflows", projected["echo"])
	}
}

func testJavaScriptRuntimeSyncCompletedEvents(t *testing.T, service *JavaScriptRuntimeService, sessionID string) {
	t.Helper()

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events.Events) != 3 {
		t.Fatalf("events = %d, want 3 canonical lifecycle events", len(events.Events))
	}
}

func TestApplyRuntimeSuccessProjection_InvalidResultMarksFailed(t *testing.T) {
	t.Parallel()
	sessionID := "dur-sess-invalid-result-001"
	foreignURI := factory.FormatArtifactURI("dur-sess-other-001", "artifact-1")
	raw, err := json.Marshal(foreignURI)
	if err != nil {
		t.Fatalf("marshal foreign uri: %v", err)
	}
	state := &runtimeSessionState{
		artifacts: []ArtifactSummary{{
			ID:         "artifact-1",
			Kind:       "IMAGE",
			Label:      "output",
			Visibility: "PUBLIC",
		}},
	}
	applyRuntimeSuccessProjection(state, sessionID, factory.JavaScriptRuntimeOutcome{
		OK:    true,
		Value: factory.TypedValue{JSON: raw},
	}, time.Now().UTC())
	if state.session.Status != LifecycleStatusFailed {
		t.Fatalf("status = %q, want FAILED", state.session.Status)
	}
	if state.session.Failure == nil || state.session.Failure.Reason != "WORKFLOW_RUNTIME_INVALID_RESULT" {
		t.Fatalf("failure = %#v, want WORKFLOW_RUNTIME_INVALID_RESULT", state.session.Failure)
	}
}

func TestChildWorkerExecutor_ResourceLeaseSurroundsTerminalChild(t *testing.T) {
	released := 0
	var leaseRequests []factory.ResourceCapacityLeaseRequest
	invoker := &recordingWorkerExecution{
		result: workers.ExecuteResult{
			Outcome: workers.ExecutionOutcomeAccepted,
			Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: `{"text":"resource-bound child finished"}`,
			}}},
			Continuation: &workers.ProviderContinuationRef{
				Provider:          "codex",
				ProviderSessionID: "codex-session-resource",
			},
		},
	}
	sink := newChildRecordSink()
	executor := newTestChildWorkerExecutor(invoker, sink, nil)
	executor.resourceLeaseAcquirer = func(_ context.Context, request factory.ResourceCapacityLeaseRequest) (*childResourceLease, error) {
		leaseRequests = append(leaseRequests, request)
		return &childResourceLease{factoryRevision: 7, release: func() { released++ }}, nil
	}

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:     "review",
		Label:      "resource-review",
		ResourceID: "reviewers",
	})
	if err != nil {
		t.Fatalf("Execute resource-bound child: %v", err)
	}
	if result.Request.ResourceID != "reviewers" || result.Request.FactoryRevision != 7 {
		t.Fatalf("child result request = %#v, want resource reviewers at revision 7", result.Request)
	}
	terminal := sink.terminalChildDispatch(t)
	if terminal.Status != factory.JavaScriptChildDispatchStatusCompleted || terminal.ResourceID != "reviewers" || terminal.FactoryRevision != 7 {
		t.Fatalf("terminal resource child = %#v, want completed reviewers at revision 7", terminal)
	}
	if len(leaseRequests) != 1 || leaseRequests[0].ResourceID != "reviewers" {
		t.Fatalf("resource lease requests = %#v, want one reviewers request", leaseRequests)
	}
	if released != 1 {
		t.Fatalf("resource lease releases = %d, want exactly one after terminal child", released)
	}
}

// TestChildWorkerExecutor_FailedChildCarriesItsProviderWithTheSessionReference
// is the regression pin for the defect that turned a crashed provider into an
// internal error.
//
// A provider session reference without its provider is rejected when the
// session's runtime facts are mapped to canonical events, and that failure is
// not scoped to the child -- it fails the whole execution. A crashed ACP peer
// surfaced as HTTP 500 rather than a FAILED session.
func TestChildWorkerExecutor_FailedChildCarriesItsProviderWithTheSessionReference(t *testing.T) {
	retryable := true
	invoker := &recordingWorkerExecution{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeFailed,
		Continuation: &workers.ProviderContinuationRef{
			Provider:          "codex",
			ProviderSessionID: "codex-session-1",
		},
		Failure: &workers.ExecutionFailure{
			Type:      workers.WorkFailureTypeInternalServerError,
			Message:   "the provider exited before completing",
			RetryHint: retryable,
		},
	}}
	sink := newChildRecordSink()
	executor := newTestChildWorkerExecutor(invoker, sink, nil)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt: "summarize",
	})
	if err == nil {
		t.Fatal("Execute error = nil, want the child's failure surfaced to the workflow")
	}
	if result.Status != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("child status = %q, want FAILED", result.Status)
	}

	terminal := sink.terminalChildDispatch(t)
	if terminal.ProviderSessionRef == "" {
		t.Fatal("terminal record provider session ref = empty, want the observed reference")
	}
	if terminal.Provider == "" {
		t.Fatal("terminal record provider = empty; a session reference without its provider " +
			"fails canonical event mapping and takes the whole execution with it")
	}
	if terminal.FailureClassification != workers.WorkFailureTypeInternalServerError {
		t.Fatalf("failure classification = %q, want the Workers-owned classification",
			terminal.FailureClassification)
	}
	if terminal.Retryable == nil || !*terminal.Retryable {
		t.Fatalf("retryable = %#v, want the classification's retry verdict", terminal.Retryable)
	}
	if terminal.FailureDetail == nil || terminal.FailureDetail.Message != "the provider exited before completing" {
		t.Fatalf("failure detail = %#v, want the bounded diagnostic", terminal.FailureDetail)
	}
}

// TestChildWorkerExecutor_InvocationErrorStillRecordsAFailedChild proves a
// Worker that could not be invoked at all is still a failed child rather than
// an unrecorded one: the session already committed QUEUED and RUNNING, and
// leaving those without a terminal would strand the dispatch forever.
func TestChildWorkerExecutor_InvocationErrorStillRecordsAFailedChild(t *testing.T) {
	invoker := &recordingWorkerExecution{err: errors.New("worker sessions unavailable")}
	sink := newChildRecordSink()
	executor := newTestChildWorkerExecutor(invoker, sink, nil)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{Prompt: "go"})
	if err == nil {
		t.Fatal("Execute error = nil, want the invocation failure surfaced")
	}
	if result.Status != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("child status = %q, want FAILED", result.Status)
	}
	if got := sink.terminalChildDispatch(t).Status; got != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("terminal record status = %q, want FAILED", got)
	}
}

func TestChildWorkerExecutor_PreparationErrorCompletesReturnedAttempt(t *testing.T) {
	beginErr := errors.New("worker attempt preparation failed")
	invoker := &recordingWorkerExecution{}
	executor := newTestChildWorkerExecutor(invoker, newChildRecordSink(), nil)

	var completed workers.ExecuteResult
	var completeErr error
	completeCalls := 0
	executor.attemptStarter = func(_ context.Context, _ workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error) {
		return func(_ context.Context, result workers.ExecuteResult, err error) error {
			completeCalls++
			completed = result
			completeErr = err
			return nil
		}, beginErr
	}

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:           "fail before worker admission",
		ExecutorProvider: "SCRIPT_WRAP",
		ModelProvider:    "codex",
		Model:            "codex-model",
	})
	if err == nil || !strings.Contains(err.Error(), beginErr.Error()) {
		t.Fatalf("Execute() error = %v, want preparation failure", err)
	}
	if result.Status != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("child status = %q, want FAILED", result.Status)
	}
	if invoker.request.Correlation.DispatchID != "" {
		t.Fatal("Workers Execute called after preparation failed")
	}
	if completeCalls != 1 || completeErr == nil || completed.Failure == nil || completed.Failure.Message != beginErr.Error() {
		t.Fatalf("completed attempt = calls:%d result:%#v error:%v, want one failed terminal", completeCalls, completed, completeErr)
	}
}

// TestChildWorkerExecutor_ScopesTheWorkersIdentityAndReleasesItAfterTheWorker
// pins both halves of the Workers identity contract: the identity handed to
// Workers is scoped to this session, and the claim that routes the Worker's
// progress back is released once the Worker is terminal.
func TestChildWorkerExecutor_ScopesTheWorkersIdentityAndReleasesItAfterTheWorker(t *testing.T) {
	invoker := &recordingWorkerExecution{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
	}}
	sink := newChildRecordSink()
	var observed []string
	var releasedWhileRunning bool
	released := 0
	executor := newTestChildWorkerExecutor(invoker, sink, func(workerDispatchID, sessionID string) func() {
		observed = append(observed, workerDispatchID+"@"+sessionID)
		return func() { released++ }
	})
	invoker.onInvoke = func() { releasedWhileRunning = released > 0 }

	if _, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{Prompt: "go"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(observed) != 1 || observed[0] != "dur-sess-1/dispatch-1@dur-sess-1" {
		t.Fatalf("observed Worker dispatch = %v, want the session-scoped identity", observed)
	}
	if invoker.request.Correlation.DispatchID != "dur-sess-1/dispatch-1" {
		t.Fatalf("Workers dispatch ID = %q, want the session-scoped identity", invoker.request.Correlation.DispatchID)
	}
	if releasedWhileRunning {
		t.Fatal("the Worker's progress claim was released while the Worker was still running")
	}
	if released != 1 {
		t.Fatalf("claim releases = %d, want exactly one once the Worker is terminal", released)
	}
	// The session's own record keeps the identity its customer sees.
	if got := sink.terminalChildDispatch(t).DispatchID; got != "dispatch-1" {
		t.Fatalf("recorded dispatch ID = %q, want the session's own unqualified identity", got)
	}
}

// TestChildWorkerExecutor_CarriesTheAuthoredWorkerNameAndPermissionPolicy
// keeps the two selections a mock-worker configuration and the provider both
// depend on attached to the Worker itself.
func TestChildWorkerExecutor_CarriesTheAuthoredWorkerNameAndPermissionPolicy(t *testing.T) {
	invoker := &recordingWorkerExecution{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
	}}
	executor := newTestChildWorkerExecutor(invoker, newChildRecordSink(), nil)

	if _, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:          "go",
		Preset:          "worker-a",
		SkipPermissions: true,
		ModelProvider:   "codex",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if invoker.request.Target.WorkerName != "worker-a" {
		t.Fatalf("worker name = %q, want the authored preset", invoker.request.Target.WorkerName)
	}
	if !invoker.request.Target.Permissions.SkipPermissions {
		t.Fatal("skip-permissions = false, want the child's resolved policy")
	}
	if invoker.request.Target.RunnerID != "codex" {
		t.Fatalf("runner = %q, want the runner resolved from the child's model provider", invoker.request.Target.RunnerID)
	}
}

// TestDirectChildExecutor_CarriesCanonicalPermissionsToWorkersExecuteRequest
// is the standalone composition regression. Its child has no Factory Runtime
// or Worker Session behind it, so the direct executor must translate the
// canonical child permission into the detached Workers request itself.
func TestDirectChildExecutor_CarriesCanonicalPermissionsToWorkersExecuteRequest(t *testing.T) {
	for _, test := range []struct {
		name       string
		permission factory.JavaScriptChildPermission
		want       bool
	}{
		{name: "DEFAULT", permission: factory.JavaScriptChildPermissionDefault, want: false},
		{name: "SKIP_PERMISSIONS", permission: factory.JavaScriptChildPermissionSkipPermissions, want: true},
		{name: "omitted", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			invocation := &recordingWorkerExecution{result: workers.ExecuteResult{
				Outcome: workers.ExecutionOutcomeAccepted,
				Output: workers.ProposedOutput{
					Primary: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "child output",
					}},
				},
			}}
			executor := newDirectChildExecutor(
				"direct-sess-1",
				invocation,
				newChildRecordSink(),
				childTestValues{},
				"/project",
				0,
			)
			request := factory.JavaScriptChildExecutionRequest{Prompt: "run", Permissions: test.permission}

			if _, err := executor.Execute(context.Background(), request); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if invocation.request.Target.Permissions.SkipPermissions != test.want {
				t.Fatalf("Workers skip-permissions = %v, want %v", invocation.request.Target.Permissions.SkipPermissions, test.want)
			}
		})
	}
}

func TestJavaScriptRuntimeService_StandaloneChildUsesInjectedWorkersExecute(t *testing.T) {
	invoker := &recordingWorkerExecution{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
	}}
	service := &JavaScriptRuntimeService{
		projectRoot: "/project",
		childValues: childTestValues{},
	}
	service.SetDirectWorkerExecution(invoker)

	hooks := service.childExecutorHooks(ChildExecutorModeLive, "standalone-session")
	if hooks.NewChildExecutor == nil {
		t.Fatal("standalone child executor hook = nil")
	}
	sink := newChildRecordSink()
	if _, err := hooks.NewChildExecutor("standalone-session", sink, factory.DefaultJavaScriptPolicy()).Execute(
		context.Background(),
		factory.JavaScriptChildExecutionRequest{Prompt: "run", Preset: "worker-a"},
	); err != nil {
		t.Fatalf("Execute standalone child: %v", err)
	}
	if invoker.request.Correlation.FactorySessionID != "standalone-session" {
		t.Fatalf("standalone child session ID = %q, want standalone-session", invoker.request.Correlation.FactorySessionID)
	}
	if invoker.request.Target.WorkerName != "worker-a" {
		t.Fatalf("standalone child worker name = %q, want worker-a", invoker.request.Target.WorkerName)
	}
}

func TestLiveChildWithoutWorkersExecutionFailsWithChildSessionID(t *testing.T) {
	service := &JavaScriptRuntimeService{
		projectRoot: "/project",
		childValues: childTestValues{},
	}
	hooks := service.childExecutorHooks(ChildExecutorModeLive, "parent-session")
	if hooks.NewChildExecutor == nil {
		t.Fatal("live child executor hook = nil")
	}

	_, err := hooks.NewChildExecutor(
		"child-session-42",
		newChildRecordSink(),
		factory.DefaultJavaScriptPolicy(),
	).Execute(context.Background(), factory.JavaScriptChildExecutionRequest{Prompt: "run"})
	if err == nil || !strings.Contains(err.Error(), "child-session-42") || !strings.Contains(err.Error(), "Workers Execute capability is required") {
		t.Fatalf("missing Workers Execute error = %v, want child session and capability", err)
	}
}

func newTestChildWorkerExecutor(
	invoke childExecuteService,
	sink *childRecordSink,
	observe workerDispatchObserver,
) *childWorkerExecutor {
	return newChildWorkerExecutor("dur-sess-1", invoke, sink, childTestValues{}, observe, "/project", 0)
}

// recordingWorkerExecution is the narrow Workers Execute seam a child reaches
// through. It intentionally does not embed Factory Runtime or an executor.
type recordingWorkerExecution struct {
	request   workers.ExecuteRequest
	result    workers.ExecuteResult
	err       error
	onInvoke  func()
	onExecute func(workers.ExecuteRequest)
}

func (i *recordingWorkerExecution) Execute(
	_ context.Context,
	req workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	i.request = req
	if i.onExecute != nil {
		i.onExecute(req)
	}
	if i.onInvoke != nil {
		i.onInvoke()
	}
	return i.result, i.err
}

type childRecordSink struct {
	records  []factory.JavaScriptRuntimeRecord
	statuses []string
	next     int
}

func newChildRecordSink() *childRecordSink { return &childRecordSink{} }

func (s *childRecordSink) Append(record factory.JavaScriptRuntimeRecord) {
	s.records = append(s.records, record)
}

func (s *childRecordSink) AppendChildDispatch(_ factory.JavaScriptChildDispatchRecord, status string) {
	s.statuses = append(s.statuses, status)
}

func (s *childRecordSink) NextChildDispatchIdentity() (string, int) {
	s.next++
	return "dispatch-1", s.next - 1
}

func (s *childRecordSink) NextChildArtifactID() string { return "artifact-1" }

func (s *childRecordSink) terminalChildDispatch(t *testing.T) factory.JavaScriptChildDispatchRecord {
	t.Helper()
	for index := len(s.records) - 1; index >= 0; index-- {
		if record := s.records[index]; record.ChildDispatch != nil {
			return *record.ChildDispatch
		}
	}
	t.Fatal("no terminal child dispatch record was appended")
	return factory.JavaScriptChildDispatchRecord{}
}

type childTestValues struct{}

func (childTestValues) TextDigest(string) string           { return "digest" }
func (childTestValues) SchemaDigest(map[string]any) string { return "schema-digest" }
func (childTestValues) CloneOutputMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	clone := make(map[string]any, len(m))
	for key, value := range m {
		clone[key] = value
	}
	return clone
}

func TestJavaScriptRuntimeService_SnapshotSizeLimitHonorsExactBoundAndRejectsBeforeSave(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	store := &runtimeRecordingStore{}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: store,
	})
	if defaultPersistedSnapshotMaxBytes <= 0 {
		t.Fatalf("default persisted snapshot maximum = %d, want positive bound", defaultPersistedSnapshotMaxBytes)
	}

	const sessionID = "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	state := runtimeSessionState{session: SessionReadResult{
		SessionID: sessionID,
		Status:    LifecycleStatusSucceeded,
	}}
	snapshot := persistedSnapshotFromRuntimeStateWithFailureLogCapacity(state, defaultPersistedTokenFailureLogCapacity)
	exact, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal exact snapshot: %v", err)
	}

	service.persistedSnapshotMaxBytes = len(exact)
	if err := service.persistSessionSnapshot(state); err != nil {
		t.Fatalf("persist snapshot at exact bound: %v", err)
	}
	store.mu.Lock()
	saveCalls := store.saveCalls
	stored := append([]byte(nil), store.payload...)
	store.mu.Unlock()
	if saveCalls != 1 || string(stored) != string(exact) {
		t.Fatalf("exact-bound save = calls %d payload %d bytes, want calls 1 payload %d bytes", saveCalls, len(stored), len(exact))
	}

	previous := []byte(`{"status":"RUNNING","sequence":7}`)
	store.mu.Lock()
	store.saveCalls = 0
	store.payload = append([]byte(nil), previous...)
	store.mu.Unlock()
	service.persistedSnapshotMaxBytes = len(exact) - 1
	err = service.persistSessionSnapshot(state)
	var sizeErr *SnapshotSizeLimitError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("one-byte-over persistence error = %v, want SnapshotSizeLimitError", err)
	}
	wantPath := runtimepersist.SnapshotPathForProjectRoot(projectRoot, sessionID)
	if sizeErr.Path != wantPath || sizeErr.ActualBytes != len(exact) || sizeErr.MaxBytes != len(exact)-1 {
		t.Fatalf("size diagnostic = %#v, want path %q actual %d bound %d", sizeErr, wantPath, len(exact), len(exact)-1)
	}
	if strings.Contains(err.Error(), "RUNNING") || strings.Contains(err.Error(), "sequence") {
		t.Fatalf("size diagnostic leaked snapshot content: %v", err)
	}
	store.mu.Lock()
	saveCalls = store.saveCalls
	stored = append([]byte(nil), store.payload...)
	store.mu.Unlock()
	if saveCalls != 0 {
		t.Fatalf("one-byte-over save calls = %d, want writer not invoked", saveCalls)
	}
	if string(stored) != string(previous) {
		t.Fatalf("one-byte-over snapshot = %s, want prior %s", stored, previous)
	}
}

func TestJavaScriptRuntimeService_CloseReportsNonCooperativeRunTimeout(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Workflows: scriptedRuntimeWorkflows(func(
			context.Context,
			factory.JavaScriptRuntimeRequest,
			factory.JavaScriptRuntimeHooks,
		) (factory.JavaScriptRuntimeOutcome, error) {
			close(entered)
			<-release
			return factory.JavaScriptRuntimeOutcome{OK: true}, nil
		}),
	})
	defer func() {
		close(release)
		service.runWaitGroup.Wait()
	}()
	if _, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-close-timeout-001",
		busyLoopWorkflowSource,
		nil,
		nil,
	)); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the non-cooperative workflow to start")
	}

	if err := service.closeWithTimeout(time.Millisecond); !errors.Is(err, ErrDurableExecutionShutdownTimeout) {
		t.Fatalf("closeWithTimeout error = %v, want ErrDurableExecutionShutdownTimeout", err)
	}
}
