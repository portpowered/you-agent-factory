package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"os"
	"path/filepath"
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

func TestPersistSessionSnapshotWarnsBeforeConfiguredHardLimit(t *testing.T) {
	for _, limit := range []struct {
		name                    string
		maxBytes, wantThreshold int
	}{
		{"4 KiB", 4096, 3072},
		{"8 KiB", 8192, 6144},
	} {
		t.Run(limit.name, func(t *testing.T) {
			threshold := durableSessionSnapshotWarningThresholdForMax(limit.maxBytes)
			if int(threshold) != limit.wantThreshold {
				t.Fatalf("warning threshold = %d, want %d", threshold, limit.wantThreshold)
			}
			for _, boundary := range []struct {
				name  string
				delta int
				warn  bool
			}{{"one byte below", -1, false}, {"at threshold", 0, true}} {
				t.Run(boundary.name, func(t *testing.T) {
					core, observed := observer.New(zap.WarnLevel)
					store := &runtimeRecordingStore{}
					service := &JavaScriptRuntimeService{
						persistence:              store,
						persistenceWarningLogger: zap.New(core),
					}
					service.persistedSnapshotMaxBytes = limit.maxBytes
					state := exactEncodedSizeWarningState(t, int(threshold)+boundary.delta)
					if err := service.persistSessionSnapshot(state); err != nil {
						t.Fatalf("persistSessionSnapshot: %v", err)
					}
					if got := len(store.payload); got != int(threshold)+boundary.delta {
						t.Fatalf("persisted snapshot bytes = %d, want %d", got, int(threshold)+boundary.delta)
					}
					entries := observed.FilterMessage("durable Factory Session snapshot reached the size warning threshold").All()
					assertSnapshotWarning(t, entries, boundary.warn, threshold)
				})
			}
		})
	}
}

func assertSnapshotWarning(t *testing.T, entries []observer.LoggedEntry, wantWarn bool, threshold int64) {
	t.Helper()
	if !wantWarn {
		if len(entries) != 0 {
			t.Fatalf("warning entries = %d, want none below threshold", len(entries))
		}
		return
	}
	if len(entries) != 1 {
		t.Fatalf("warning entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["code"] != durableSessionSnapshotSizeWarningCode || fields["session_id"] != "dur-sess-warning-threshold" || fields["observed_bytes"] != threshold || fields["threshold_bytes"] != threshold || fields["retained_live_tokens"] != int64(1) || fields["retained_terminal_tokens"] != int64(1) {
		t.Fatalf("warning fields = %#v, want safe session, size, threshold, and token counts", fields)
	}
	for _, forbidden := range []string{"sourceContent", "payload", "credentials", "unsafe-path"} {
		if _, ok := fields[forbidden]; ok {
			t.Fatalf("warning contains forbidden field %q: %#v", forbidden, fields)
		}
	}
}

func TestPersistSessionSnapshotWarningDoesNotHideSaveFailure(t *testing.T) {
	wantErr := errors.New("checkpoint unavailable")
	core, observed := observer.New(zap.WarnLevel)
	const maxBytes = 4096
	service := &JavaScriptRuntimeService{
		persistence:              &runtimeRecordingStore{saveErr: wantErr},
		persistenceWarningLogger: zap.New(core),
	}
	service.persistedSnapshotMaxBytes = maxBytes
	if err := service.persistSessionSnapshot(exactEncodedSizeWarningState(t, int(durableSessionSnapshotWarningThresholdForMax(maxBytes)))); !errors.Is(err, wantErr) {
		t.Fatalf("persistSessionSnapshot error = %v, want %v", err, wantErr)
	}
	if got := observed.FilterMessage("durable Factory Session snapshot reached the size warning threshold").Len(); got != 1 {
		t.Fatalf("warning count = %d, want 1 before failed save", got)
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

func TestProjectResultRead_UnavailableRetainsTerminalFailure(t *testing.T) {
	t.Parallel()
	failure := &FailureSummary{Reason: "SCRIPT_ERROR", Message: "controlled workflow failure"}
	session := SessionReadResult{
		SessionID:     "dur-sess-failure",
		Status:        LifecycleStatusFailed,
		ResultSummary: &ResultSummary{ResultStatus: string(ResultStatusUnavailable)},
	}
	canonical := ResultReadResult{
		SessionID: session.SessionID, SessionStatus: LifecycleStatusFailed,
		ResultStatus: ResultStatusUnavailable, Failure: failure,
	}
	for _, mode := range []ResultMode{ResultModeFinal, ResultModePartial} {
		result, err := ProjectResultRead(canonical, session, nil, ResultRequest{Mode: mode})
		if err != nil {
			t.Fatalf("ProjectResultRead(%s): %v", mode, err)
		}
		if result.Failure == nil || result.Failure.Message != failure.Message || result.Failure == failure {
			t.Fatalf("ProjectResultRead(%s) Failure = %#v, want cloned original failure", mode, result.Failure)
		}
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
	executor.attemptStarter = func(_ context.Context, _ *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error) {
		return func(_ context.Context, result workers.ExecuteResult, err error) (workers.ExecuteResult, error) {
			completeCalls++
			completed = result
			completeErr = err
			return result, nil
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

func TestJavaScriptRuntimeService_CloseCancelsJoinsAndPersistsAsyncSession(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	store := mustTestRuntimePersistenceStore(t, projectRoot)
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: store,
		Workflows:   scriptedBlockingRuntimeWorkflows(),
	})
	started, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-close-joins-001",
		busyLoopWorkflowSource,
		map[string]any{"subject": "shutdown"},
		nil,
	))
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	if err := service.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession after Close: %v", err)
	}
	if session.Status != LifecycleStatusCanceled {
		t.Fatalf("session status after Close = %q, want CANCELED", session.Status)
	}
	if session.Failure == nil || session.Failure.Reason != "WORKFLOW_RUNTIME_CANCELED" {
		t.Fatalf("session failure after Close = %#v, want WORKFLOW_RUNTIME_CANCELED", session.Failure)
	}
	snapshotPath := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), started.SessionID+".json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("terminal snapshot after Close: %v", err)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("repeated Close: %v", err)
	}
	if _, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-close-rejected-001",
		busyLoopWorkflowSource,
		nil,
		nil,
	)); !errors.Is(err, ErrDurableExecutionClosed) {
		t.Fatalf("StartAsync after Close error = %v, want ErrDurableExecutionClosed", err)
	}
}

func newTerminalWorkersService(t *testing.T, provider providers.Service) WorkerExecution {
	t.Helper()
	return terminalWorkerService{provider: provider}
}

// terminalWorkerService is a service-root fake: the bridge test owns durable
// response publication, while Workers-owned wire tests cover construction and
// normalization of the real Execute implementation.
type terminalWorkerService struct {
	provider providers.Service
}

func (service terminalWorkerService) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	providerResult, err := service.provider.Execute(ctx, providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: request.Correlation.AttemptID,
		Correlation: providers.ExecuteCorrelation{
			FactorySessionID: request.Correlation.FactorySessionID,
			RuntimeID:        request.Correlation.RuntimeID,
			GenerationID:     request.Correlation.GenerationID,
			DispatchID:       request.Correlation.DispatchID,
			AttemptID:        request.Correlation.AttemptID,
			RequestID:        request.Correlation.RequestID,
			TraceID:          request.Correlation.TraceID,
		},
		UserMessage: request.Target.Prompt.UserMessage,
	})
	result := workers.ExecuteResult{Correlation: request.Correlation}
	if err != nil {
		outcome := workers.ExecutionOutcomeFailed
		failureType := workers.WorkFailureTypeUnknown
		if errors.Is(err, context.Canceled) {
			outcome = workers.ExecutionOutcomeCanceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			failureType = workers.WorkFailureTypeTimeout
		}
		result.Outcome = outcome
		result.Failure = &workers.ExecutionFailure{
			Type:    failureType,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: err.Error(),
		}
		return result, err
	}
	result.Outcome = workers.ExecutionOutcomeAccepted
	result.Output.Primary = []work.WorkContentPart{{Text: providerResult.Content}}
	return result, nil
}

func exactEncodedSizeWarningState(t *testing.T, targetSize int) runtimeSessionState {
	t.Helper()
	state := runtimeSessionState{
		session:        SessionReadResult{SessionID: "dur-sess-warning-threshold", Status: LifecycleStatusSucceeded},
		petriMutations: []interfaces.TokenMutationRecord{{Type: interfaces.MutationCreate, TokenID: "live-token", ToPlace: "task:running", TransitionReachable: true, Token: &workers.Token{ID: "live-token", Color: workers.Color{WorkID: "live-work"}}}},
		petriSummaries: []PetriTokenSummary{{TokenID: "terminal-token", WorkID: "terminal-work", PlaceID: "task:done"}},
	}
	base := encodedWarningStateBytes(t, state)
	if targetSize < base {
		t.Fatalf("target snapshot size %d is below base size %d", targetSize, base)
	}
	state.sourceContent = strings.Repeat("x", targetSize-base)
	if got := encodedWarningStateBytes(t, state); got != targetSize {
		t.Fatalf("constructed snapshot bytes = %d, want %d", got, targetSize)
	}
	return state
}

func TestSnapshotWarningThresholdConfiguration(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		maxBytes  int
		threshold int64
	}{
		{"default", 0, 48 << 20}, {"negative uses default", -1, 48 << 20},
		{"64 MiB", 64 << 20, 48 << 20}, {"100 MiB", 100 << 20, 100 << 20}, {"128 MiB", 128 << 20, 100 << 20},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := durableSessionSnapshotWarningThresholdForMax(test.maxBytes); got != test.threshold {
				t.Fatalf("threshold=%d, want %d", got, test.threshold)
			}
		})
	}
}

func TestChildWorkerExecutor_AdmissionObserverReachesExecutingRequest(t *testing.T) {
	t.Parallel()
	owned := &struct{ providers.AttemptControl }{}
	observed := false
	completed := false
	invoker := &recordingWorkerExecution{
		result: workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted},
		onExecute: func(request workers.ExecuteRequest) {
			if request.Input.AttemptControlObserver == nil {
				t.Error("Workers execution lost its admission-bound observer")
				return
			}
			request.Input.AttemptControlObserver(owned)
		},
	}
	executor := newTestChildWorkerExecutor(invoker, newChildRecordSink(), nil)
	executor.attemptStarter = func(ctx context.Context, request *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error) {
		if ctx.Err() != nil {
			t.Error("admission context canceled")
		}
		request.Input.AttemptControlObserver = func(control providers.AttemptControl) { observed = control == owned }
		return func(_ context.Context, result workers.ExecuteResult, err error) (workers.ExecuteResult, error) {
			completed = true
			if !observed || err != nil {
				t.Errorf("observed = %v, error = %v; want handle before completion", observed, err)
			}
			return result, err
		}, nil
	}
	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{Prompt: "observe attempt"})
	if err != nil || result.Status != factory.JavaScriptChildDispatchStatusCompleted || !completed || !observed {
		t.Fatalf("result = %#v, err = %v, completed = %v, observed = %v", result, err, completed, observed)
	}
}

func TestChildWorkerExecutor_RetryClosesAttemptBeforeBindingReplacement(t *testing.T) {
	t.Parallel()
	begun := 0
	completed := 0
	var attempts []string
	var observed []int
	invoker := &recordingWorkerExecution{}
	invoker.onExecute = func(request workers.ExecuteRequest) {
		if request.Input.AttemptControlObserver == nil {
			t.Error("retry execution lost its admission observer")
			return
		}
		request.Input.AttemptControlObserver(nil)
		invoker.result = workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}
		if begun == 1 {
			invoker.result.Outcome = workers.ExecutionOutcomeFailed
			invoker.result.Failure = &workers.ExecutionFailure{Type: workers.WorkFailureTypeInternalServerError, RetryHint: true}
		}
	}
	executor := newTestChildWorkerExecutor(invoker, newChildRecordSink(), nil)
	executor.maxAttempts = 2
	executor.attemptStarter = func(_ context.Context, request *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error) {
		if begun != completed {
			t.Errorf("retry admitted while prior attempt remained open: begun = %d, completed = %d", begun, completed)
		}
		begun++
		generation := begun
		attempts = append(attempts, request.Correlation.AttemptID)
		request.Input.AttemptControlObserver = func(providers.AttemptControl) { observed = append(observed, generation) }
		return func(_ context.Context, result workers.ExecuteResult, err error) (workers.ExecuteResult, error) {
			completed++
			if result.Correlation.AttemptID != attempts[generation-1] || err != nil {
				t.Errorf("completion = %#v, error = %v; want exact admitted attempt", result.Correlation, err)
			}
			return result, err
		}, nil
	}
	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{Prompt: "retry attempt"})
	if err != nil || result.Status != factory.JavaScriptChildDispatchStatusCompleted || begun != 2 || completed != 2 {
		t.Fatalf("result = %#v, error = %v, begun = %d, completed = %d", result, err, begun, completed)
	}
	if len(attempts) != 2 || attempts[0] == attempts[1] || len(observed) != 2 || observed[0] != 1 || observed[1] != 2 {
		t.Fatalf("attempts = %v, observer generations = %v; want distinct physical attempts", attempts, observed)
	}
}
