package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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
