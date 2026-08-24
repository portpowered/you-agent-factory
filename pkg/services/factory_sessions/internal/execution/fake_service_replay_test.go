package factorysessionexecution

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestBuildCanonicalSessionEvents_RunningAndTerminalSessions(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 8, 14, 10, 0, 0, time.UTC)

	runningEvents := BuildCanonicalSessionEvents(
		SessionReadResult{
			SessionID:        "dur-sess-js-run-n-001",
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "JAVASCRIPT",
			Dialect:          "you-workflow-v1",
			Phase:            "verify",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		ResultReadResult{
			SessionID:    "dur-sess-js-run-n-001",
			ResultStatus: ResultStatusPartial,
		},
	)
	if len(runningEvents) != 2 {
		t.Fatalf("running events = %d, want 2", len(runningEvents))
	}
	assertCanonicalEventEnvelope(t, runningEvents[0], "SESSION_STARTED", "session-started/dur-sess-js-run-n-001")
	assertCanonicalEventEnvelope(t, runningEvents[1], "SESSION_RESULT_UPDATED", "session-result-updated/dur-sess-js-run-n-001")

	terminalEvents := BuildCanonicalSessionEvents(
		SessionReadResult{
			SessionID:        "dur-sess-js-success-002",
			Status:           LifecycleStatusSucceeded,
			OrchestratorKind: "JAVASCRIPT",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: &finishedAt},
		},
		ResultReadResult{
			SessionID:    "dur-sess-js-success-002",
			ResultStatus: ResultStatusFinal,
		},
	)
	if len(terminalEvents) != 3 {
		t.Fatalf("terminal events = %d, want 3", len(terminalEvents))
	}
	assertCanonicalEventEnvelope(t, terminalEvents[2], "SESSION_COMPLETED", "session-completed/dur-sess-js-success-002")
}

func TestProjectRuntimeExecutionRecords_LiveChildDispatch_ProjectsLifecycleArtifactsAndProviderSession(t *testing.T) {
	t.Parallel()
	artifactRef := factory.FormatArtifactURI("session-live-child", "child-artifact-1")
	records := []factory.JavaScriptRuntimeRecord{
		{
			Kind: factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &factory.JavaScriptChildDispatchRecord{
				DispatchID:    "dispatch-1",
				Status:        factory.JavaScriptChildDispatchStatusQueued,
				Label:         "summarize-findings",
				ExecutionMode: ChildExecutorModeLive,
				ArtifactRef:   artifactRef,
			},
		},
		{
			Kind: factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &factory.JavaScriptChildDispatchRecord{
				DispatchID:    "dispatch-1",
				Status:        factory.JavaScriptChildDispatchStatusRunning,
				Label:         "summarize-findings",
				ExecutionMode: ChildExecutorModeLive,
				ArtifactRef:   artifactRef,
			},
		},
		{
			Kind: factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &factory.JavaScriptChildDispatchRecord{
				DispatchID:         "dispatch-1",
				Status:             factory.JavaScriptChildDispatchStatusCompleted,
				Label:              "summarize-findings",
				ExecutionMode:      ChildExecutorModeLive,
				Provider:           "mock",
				ProviderSessionRef: "provider-session-42",
				ArtifactRef:        artifactRef,
			},
		},
	}

	observedAt := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	projection := ProjectRuntimeExecutionRecords("session-live-child", records, observedAt)
	if len(projection.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one dispatch", projection.Dispatches)
	}
	dispatch := projection.Dispatches[0]
	if dispatch.Status != DispatchStatusCompleted {
		t.Fatalf("dispatch status = %q, want COMPLETED", dispatch.Status)
	}
	if len(dispatch.ProviderSessionRefs) != 1 || dispatch.ProviderSessionRefs[0].ID != "provider-session-42" {
		t.Fatalf("providerSessionRefs = %#v", dispatch.ProviderSessionRefs)
	}
	if len(dispatch.OutputArtifactIDs) != 1 || dispatch.OutputArtifactIDs[0] != "child-artifact-1" {
		t.Fatalf("outputArtifactIds = %#v", dispatch.OutputArtifactIDs)
	}
	transitions := projection.DispatchStatusTransitions["dispatch-1"]
	if len(transitions) != 3 {
		t.Fatalf("statusTransitions = %#v, want queued/running/completed", transitions)
	}
	if len(projection.Artifacts) != 1 || projection.Artifacts[0].ID != "child-artifact-1" {
		t.Fatalf("artifacts = %#v", projection.Artifacts)
	}
}

func TestPersistedTokenFailureHistoryRetainsHeadTailAndReloads(t *testing.T) {
	const historySize = defaultPersistedTokenFailureLogCapacity + 8
	history := failureHistoryForRetryCount(historySize)
	state := runtimeSessionState{
		petriMutations: []interfaces.TokenMutationRecord{failureMutation(history, 1)},
	}

	snapshot := persistedSnapshotFromRuntimeState(state)
	if len(state.petriMutations[0].Token.History.FailureLog) != historySize {
		t.Fatalf("live failure log length = %d, want %d", len(state.petriMutations[0].Token.History.FailureLog), historySize)
	}
	if got := state.petriMutations[0].Token.History.FailureLogDroppedCount; got != 0 {
		t.Fatalf("live dropped failure count = %d, want 0", got)
	}

	got := snapshot.Records[0].PetriMutation.Token.History
	if got.FailureLogDroppedCount != 8 {
		t.Fatalf("persisted dropped failure count = %d, want 8", got.FailureLogDroppedCount)
	}
	if len(got.FailureLog) != defaultPersistedTokenFailureLogCapacity {
		t.Fatalf("persisted failure log length = %d, want %d", len(got.FailureLog), defaultPersistedTokenFailureLogCapacity)
	}
	for index := range got.FailureLog {
		wantIndex := index
		if index >= defaultPersistedTokenFailureLogCapacity/2 {
			wantIndex = historySize - (defaultPersistedTokenFailureLogCapacity - defaultPersistedTokenFailureLogCapacity/2) + index - defaultPersistedTokenFailureLogCapacity/2
		}
		want := history.FailureLog[wantIndex]
		if got.FailureLog[index] != want {
			t.Fatalf("persisted failure log[%d] = %#v, want %#v", index, got.FailureLog[index], want)
		}
	}
	if got.LastError != history.LastError {
		t.Fatalf("persisted LastError = %q, want %q", got.LastError, history.LastError)
	}

	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal bounded snapshot: %v", err)
	}
	var reloaded PersistedRuntimeSessionState
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatalf("unmarshal bounded snapshot: %v", err)
	}
	reloadedHistory := reloaded.Records[0].PetriMutation.Token.History
	if reloadedHistory.FailureLogDroppedCount != got.FailureLogDroppedCount ||
		reloadedHistory.LastError != got.LastError ||
		len(reloadedHistory.FailureLog) != len(got.FailureLog) {
		t.Fatalf("reloaded failure history = %#v, want %#v", reloadedHistory, got)
	}
}

func TestPersistedTokenFailureHistoryWithinCapacityIsUnchanged(t *testing.T) {
	history := failureHistoryForRetryCount(defaultPersistedTokenFailureLogCapacity)
	state := runtimeSessionState{
		petriMutations: []interfaces.TokenMutationRecord{failureMutation(history, 1)},
	}

	snapshot := persistedSnapshotFromRuntimeState(state)
	got := snapshot.Records[0].PetriMutation.Token.History
	if len(got.FailureLog) != len(history.FailureLog) {
		t.Fatalf("failure log length = %d, want %d", len(got.FailureLog), len(history.FailureLog))
	}
	for index := range history.FailureLog {
		if got.FailureLog[index] != history.FailureLog[index] {
			t.Fatalf("failure log[%d] changed: got %#v, want %#v", index, got.FailureLog[index], history.FailureLog[index])
		}
	}
	if got.FailureLogDroppedCount != history.FailureLogDroppedCount || got.LastError != history.LastError {
		t.Fatalf("history metadata changed: got %#v, want %#v", got, history)
	}
}

func TestDurablePetriFailureHistorySnapshotGrowthIsBounded(t *testing.T) {
	retryCounts := []int{10, 100, 1000}
	baselineBytes := make(map[int]int, len(retryCounts))
	boundedBytes := make(map[int]int, len(retryCounts))

	for _, retryCount := range retryCounts {
		t.Run(fmt.Sprintf("N=%d", retryCount), func(t *testing.T) {
			store := &runtimeRecordingStore{}
			mutations := make([]interfaces.TokenMutationRecord, retryCount)
			for retry := 1; retry <= retryCount; retry++ {
				mutations[retry-1] = failureMutation(failureHistoryForRetryCount(retry), retry)
			}
			state := runtimeSessionState{
				session: SessionReadResult{
					SessionID: "~default",
					Status:    LifecycleStatusRunning,
				},
				petriMutations: mutations,
			}
			service := &JavaScriptRuntimeService{
				clock:       runtimeTestClock{now: time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)},
				persistence: store,
			}
			if err := service.persistSessionSnapshot(state); err != nil {
				t.Fatalf("persist retry sequence: %v", err)
			}

			live := cloneRuntimeSessionState(&state)
			last := live.petriMutations[len(live.petriMutations)-1].Token.History
			if len(last.FailureLog) != retryCount {
				t.Fatalf("live final failure log length = %d, want %d", len(last.FailureLog), retryCount)
			}
			if last.FailureLogDroppedCount != 0 {
				t.Fatalf("live final dropped failure count = %d, want 0", last.FailureLogDroppedCount)
			}

			unbounded := persistedSnapshotFromRuntimeStateWithFailureLogCapacity(live, 0)
			before, err := json.MarshalIndent(unbounded, "", "  ")
			if err != nil {
				t.Fatalf("marshal unbounded baseline: %v", err)
			}
			baselineBytes[retryCount] = len(before)
			boundedBytes[retryCount] = len(store.payload)
			t.Logf("N=%d before_bytes=%d after_bytes=%d", retryCount, len(before), len(store.payload))
		})
	}

	if boundedBytes[10] != baselineBytes[10] {
		t.Fatalf("N=10 changed despite fitting capacity: before=%d after=%d", baselineBytes[10], boundedBytes[10])
	}
	for _, retryCount := range []int{100, 1000} {
		if boundedBytes[retryCount] >= baselineBytes[retryCount] {
			t.Fatalf("N=%d bounded snapshot = %d, want less than unbounded baseline %d", retryCount, boundedBytes[retryCount], baselineBytes[retryCount])
		}
	}
	if boundedBytes[1000] > boundedBytes[100]*12 {
		t.Fatalf("bounded snapshots grew superlinearly: N=100=%d, N=1000=%d", boundedBytes[100], boundedBytes[1000])
	}
}

func failureMutation(history workerexecution.History, retry int) interfaces.TokenMutationRecord {
	return interfaces.TokenMutationRecord{
		DispatchID:   fmt.Sprintf("dispatch-%04d", retry),
		TransitionID: "retry",
		Outcome:      workerexecution.OutcomeFailed,
		Type:         interfaces.MutationMove,
		TokenID:      fmt.Sprintf("token-%04d", retry),
		FromPlace:    "task:running",
		ToPlace:      "task:failed",
		Reason:       "worker failed",
		Token: &workerexecution.Token{
			ID:    fmt.Sprintf("token-%04d", retry),
			State: "failed",
			Color: workerexecution.Color{
				WorkID:     "work-1",
				WorkTypeID: "task",
				DataType:   workerexecution.DataTypeWork,
			},
			History: history,
		},
	}
}

func failureHistoryForRetryCount(count int) workerexecution.History {
	log := make([]workerexecution.Failure, count)
	for index := range log {
		log[index] = workerexecution.Failure{
			TransitionID: "retry",
			Timestamp:    time.Date(2026, 8, 22, 12, 0, index, 0, time.UTC),
			Error:        fmt.Sprintf("failure-%04d", index+1),
			Attempt:      index + 1,
		}
	}
	lastError := ""
	if len(log) > 0 {
		lastError = log[len(log)-1].Error
	}
	return workerexecution.History{
		TotalVisits:         map[string]int{"retry": count},
		ConsecutiveFailures: map[string]int{"retry": count},
		PlaceVisits:         map[string]int{"task:failed": count},
		LastError:           lastError,
		FailureLog:          log,
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this focused projection regression keeps live and persisted-record assertions together.
func TestProjectRuntimeExecutionRecords_FailedLiveChild_ProjectsFailureDetail(t *testing.T) {
	t.Parallel()
	retryable := true
	records := []factory.JavaScriptRuntimeRecord{
		{
			Kind: factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &factory.JavaScriptChildDispatchRecord{
				DispatchID:    "dispatch-2",
				Status:        factory.JavaScriptChildDispatchStatusQueued,
				Label:         "child-1",
				ExecutionMode: ChildExecutorModeLive,
			},
		},
		{
			Kind: factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &factory.JavaScriptChildDispatchRecord{
				DispatchID:    "dispatch-2",
				Status:        factory.JavaScriptChildDispatchStatusRunning,
				Label:         "child-1",
				ExecutionMode: ChildExecutorModeLive,
			},
		},
		{
			Kind: factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &factory.JavaScriptChildDispatchRecord{
				DispatchID:    "dispatch-2",
				Status:        factory.JavaScriptChildDispatchStatusFailed,
				Label:         "child-1",
				ExecutionMode: ChildExecutorModeLive,
				FailureDetail: &workerexecution.FailureDetail{
					Reason:  workerexecution.WorkFailureTypeUnknown,
					Message: "live child failed: simulated child error",
				},
				Attempt:               3,
				Retryable:             &retryable,
				FailureClassification: workerexecution.WorkFailureTypeTimeout,
			},
		},
	}

	observedAt := time.Date(2026, 6, 16, 14, 0, 0, 0, time.UTC)
	projection := ProjectRuntimeExecutionRecords("session-live-child-failure", records, observedAt)
	if len(projection.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one failed dispatch", projection.Dispatches)
	}
	dispatch := projection.Dispatches[0]
	if dispatch.Status != DispatchStatusFailed {
		t.Fatalf("dispatch status = %q, want FAILED", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Reason != string(workerexecution.WorkFailureTypeUnknown) {
		t.Fatalf("failureDetail = %#v", dispatch.FailureDetail)
	}
	if dispatch.FailureDetail.Message != "live child failed: simulated child error" {
		t.Fatalf("failure message = %q", dispatch.FailureDetail.Message)
	}
	if dispatch.Attempt != 3 || dispatch.Retryable == nil || !*dispatch.Retryable || dispatch.FailureClassification != string(workerexecution.WorkFailureTypeTimeout) {
		t.Fatalf("retry diagnostics = attempt:%d retryable:%v classification:%q", dispatch.Attempt, dispatch.Retryable, dispatch.FailureClassification)
	}
	encoded, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("marshal records: %v", err)
	}
	var replayedRecords []factory.JavaScriptRuntimeRecord
	if err := json.Unmarshal(encoded, &replayedRecords); err != nil {
		t.Fatalf("unmarshal records: %v", err)
	}
	replayed := ProjectRuntimeExecutionRecords("session-live-child-failure", replayedRecords, observedAt).Dispatches[0]
	if replayed.Attempt != dispatch.Attempt || replayed.Retryable == nil || *replayed.Retryable != *dispatch.Retryable || replayed.FailureClassification != dispatch.FailureClassification {
		t.Fatalf("replayed retry diagnostics = %#v, want %#v", replayed, dispatch)
	}
	if len(projection.Artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want none for failed child", projection.Artifacts)
	}
	transitions := projection.DispatchStatusTransitions["dispatch-2"]
	if len(transitions) != 3 || transitions[2] != DispatchStatusFailed {
		t.Fatalf("statusTransitions = %#v, want queued/running/failed", transitions)
	}
}

func TestFilterEventsAfterReconnect_AfterEventIDAndSequence(t *testing.T) {
	t.Parallel()
	events := []json.RawMessage{
		json.RawMessage(`{"id":"session-started/s1","context":{"sequence":1,"sessionSequence":0}}`),
		json.RawMessage(`{"id":"session-result-updated/s1","context":{"sequence":2,"sessionSequence":1}}`),
		json.RawMessage(`{"id":"session-completed/s1","context":{"sequence":3,"sessionSequence":2}}`),
	}

	all, err := FilterEventsAfterReconnect(events, EventReconnectRequest{}, "s1")
	if err != nil {
		t.Fatalf("FilterEventsAfterReconnect all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("all events = %d, want 3", len(all))
	}

	afterStart, err := FilterEventsAfterReconnect(events, EventReconnectRequest{
		AfterEventID: "session-started/s1",
	}, "s1")
	if err != nil {
		t.Fatalf("FilterEventsAfterReconnect after start: %v", err)
	}
	if len(afterStart) != 2 {
		t.Fatalf("after start events = %d, want 2", len(afterStart))
	}

	sequence := 1
	afterSequence, err := FilterEventsAfterReconnect(events, EventReconnectRequest{
		AfterSequence: &sequence,
	}, "s1")
	if err != nil {
		t.Fatalf("FilterEventsAfterReconnect after sequence: %v", err)
	}
	if len(afterSequence) != 1 {
		t.Fatalf("after sequence events = %d, want 1", len(afterSequence))
	}

	eventsWithoutSessionSequence := append([]json.RawMessage(nil), events...)
	eventsWithoutSessionSequence[1] = json.RawMessage(
		`{"id":"session-result-updated/s1","context":{"sequence":42}}`,
	)
	canonicalSequence := 42
	afterCanonicalSequence, err := FilterEventsAfterReconnect(eventsWithoutSessionSequence, EventReconnectRequest{
		AfterSequence: &canonicalSequence,
	}, "s1")
	if err != nil {
		t.Fatalf("FilterEventsAfterReconnect canonical sequence fallback: %v", err)
	}
	if len(afterCanonicalSequence) != 1 {
		t.Fatalf("after canonical sequence events = %d, want 1", len(afterCanonicalSequence))
	}

	_, err = FilterEventsAfterReconnect(events, EventReconnectRequest{
		AfterEventID: "missing-event",
	}, "s1")
	if !errors.Is(err, ErrReconnectCursorNotFound) {
		t.Fatalf("missing cursor error = %v, want ErrReconnectCursorNotFound", err)
	}
}

func TestReplaySessionProjection_TerminalSessionBracket(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 11, 12, 0, 2, 0, time.UTC)
	sessionID := "dur-sess-replay-001"
	events := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusSucceeded,
			OrchestratorKind: "JAVASCRIPT",
			Dialect:          "you-workflow-v1",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource: ResolvedSource{
				SourceRef:  "workflow/simple-final",
				SourceHash: "sha256:fixture",
			},
			ResultSummary: &ResultSummary{
				ResultStatus: string(ResultStatusFinal),
				Summary:      "Completed simple workflow.",
			},
			Lifecycle: &LifecycleTimestamps{
				StartedAt:  &startedAt,
				FinishedAt: &finishedAt,
			},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusFinal,
		},
	)

	session, result, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if session.Status != LifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", session.Status)
	}
	if session.SourceHash != "sha256:fixture" {
		t.Fatalf("sourceHash = %q", session.SourceHash)
	}
	if session.Policy.EffectiveHash != "sha256:policy" {
		t.Fatalf("policyHash = %q", session.Policy.EffectiveHash)
	}
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", session.ResultSummary)
	}
	if session.Links.Session == "" || session.Links.Events == "" {
		t.Fatal("expected inspection links")
	}
	if result.ResultStatus != ResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
}

func TestReplaySessionProjection_IdempotentOnDuplicateSequence(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-replay-002"
	events := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "JAVASCRIPT",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusNotReady,
		},
	)

	firstSession, firstResult, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection first: %v", err)
	}
	secondSession, secondResult, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection second: %v", err)
	}
	if firstSession.SessionID != secondSession.SessionID ||
		firstSession.Status != secondSession.Status ||
		firstSession.SourceHash != secondSession.SourceHash ||
		firstSession.Policy.EffectiveHash != secondSession.Policy.EffectiveHash ||
		firstSession.Links.Session != secondSession.Links.Session {
		t.Fatalf("session projection changed on replay: %#v vs %#v", firstSession, secondSession)
	}
	if firstResult.ResultStatus != secondResult.ResultStatus ||
		firstResult.SessionStatus != secondResult.SessionStatus {
		t.Fatalf("result projection changed on replay: %#v vs %#v", firstResult, secondResult)
	}
}

func TestReplaySessionProjection_ReplacesArtifactStubsWithoutDuplication(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 11, 12, 0, 2, 0, time.UTC)
	sessionID := "dur-sess-replay-003"
	events := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusSucceeded,
			OrchestratorKind: "JAVASCRIPT",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: &finishedAt},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusFinal,
			ArtifactIDs:  []string{"art-1", "art-2"},
		},
	)

	session, _, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if len(session.ArtifactRefs) != 2 {
		t.Fatalf("artifact refs = %d, want 2", len(session.ArtifactRefs))
	}

	// Replaying the same events again must not duplicate artifact stubs.
	sessionAgain, _, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection again: %v", err)
	}
	if len(sessionAgain.ArtifactRefs) != 2 {
		t.Fatalf("artifact refs after second replay = %d, want 2", len(sessionAgain.ArtifactRefs))
	}
}

func TestReplaySessionProjection_FirstTerminalOutcomeWinsCompetingRace(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Second)
	sessionID := "dur-sess-replay-terminal-race"
	base := SessionReadResult{
		SessionID: sessionID, Status: LifecycleStatusCanceled, OrchestratorKind: "JAVASCRIPT",
		Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: &finishedAt},
	}
	canceled := BuildCanonicalRuntimeSessionEvents(base, ResultReadResult{
		SessionID: sessionID, ResultStatus: ResultStatusUnavailable, SessionStatus: LifecycleStatusCanceled,
	})
	base.Status = LifecycleStatusFailed
	failed := BuildCanonicalRuntimeSessionEvents(base, ResultReadResult{
		SessionID: sessionID, ResultStatus: ResultStatusFailedWithPartial, SessionStatus: LifecycleStatusFailed,
	})

	session, result, err := ReplaySessionProjection(append(canceled, failed...))
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if session.Status != LifecycleStatusCanceled || result.SessionStatus != LifecycleStatusCanceled || result.ResultStatus != ResultStatusUnavailable {
		t.Fatalf("late terminal outcome overwrote cancellation: session=%#v result=%#v", session, result)
	}
}

func TestReplaySessionProjection_PreservesSyncTimeoutAvailability(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-replay-timeout-availability"
	events := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "JAVASCRIPT",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusNotReady,
			Availability: &ResultAvailabilityDetail{
				Reason:    "SYNC_WAIT_TIMED_OUT",
				Message:   "Sync wait ended before a terminal result was available.",
				Retryable: true,
			},
		},
	)

	_, result, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if result.Availability == nil || result.Availability.Reason != "SYNC_WAIT_TIMED_OUT" {
		t.Fatalf("availability = %#v, want SYNC_WAIT_TIMED_OUT", result.Availability)
	}
}

func TestReplaySessionProjection_IgnoresUnknownEventTypes(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-replay-004"
	base := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "JAVASCRIPT",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusNotReady,
		},
	)
	events := append(append([]json.RawMessage(nil), base...), json.RawMessage(`{"type":"DISPATCH_QUEUED","id":"dispatch-queued/1","context":{"sequence":99},"payload":{}}`))

	session, _, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if session.SessionID != sessionID {
		t.Fatalf("sessionId = %q, want %q", session.SessionID, sessionID)
	}
}

func TestReplaySessionProjection_EquivalentOrchestratorsSharePublicSessionProjection(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 11, 18, 30, 0, 0, time.UTC)
	sessionID := "dur-sess-orchestrator-parity-001"
	baseSession := SessionReadResult{
		SessionID:      sessionID,
		Status:         LifecycleStatusRunning,
		SourceHash:     "sha256:equivalent-source",
		Policy:         PolicyProjection{EffectiveHash: "sha256:equivalent-policy"},
		ResolvedSource: ResolvedSource{SourceHash: "sha256:equivalent-source"},
		Lifecycle:      &LifecycleTimestamps{StartedAt: &startedAt},
		Phase:          "execute",
	}
	result := ResultReadResult{SessionID: sessionID, ResultStatus: ResultStatusNotReady}

	petriSession := baseSession
	petriSession.OrchestratorKind = "PETRI"
	petriEvents := BuildCanonicalRuntimeSessionEvents(petriSession, result)
	petriPayload := factoryapi.DispatchRequestEventPayload{
		Inputs:       []factoryapi.DispatchConsumedWorkRef{},
		TransitionId: "transition-review",
	}
	petriEvents = append(petriEvents, canonicalTypedInternalEvent(t, "DISPATCH_REQUEST", sessionID, petriPayload))

	javascriptSession := baseSession
	javascriptSession.OrchestratorKind = "JAVASCRIPT"
	javascriptSession.Dialect = "you-workflow-v1"
	javascriptEvents := BuildCanonicalRuntimeSessionEvents(javascriptSession, result)
	javascriptPayload := factoryapi.OrchestratorCheckpointWrittenEventPayload{
		Label:              "after-review",
		ResumabilityStatus: factoryapi.RESUMABLE,
		Timestamp:          &startedAt,
	}
	javascriptEvents = append(javascriptEvents, canonicalTypedInternalEvent(t, "ORCHESTRATOR_CHECKPOINT_WRITTEN", sessionID, javascriptPayload))

	petriProjection, _, err := ReplaySessionProjection(petriEvents)
	if err != nil {
		t.Fatalf("ReplaySessionProjection(PETRI): %v", err)
	}
	javascriptProjection, _, err := ReplaySessionProjection(javascriptEvents)
	if err != nil {
		t.Fatalf("ReplaySessionProjection(JAVASCRIPT): %v", err)
	}

	type sharedPublicSession struct {
		SessionID     string
		Status        LifecycleStatus
		SourceHash    string
		PolicyHash    string
		Phase         string
		StartedAt     time.Time
		ResultStatus  string
		ArtifactCount int
		Links         InspectionLinks
	}
	sharedProjection := func(session SessionReadResult) sharedPublicSession {
		t.Helper()
		if session.Lifecycle == nil || session.Lifecycle.StartedAt == nil {
			t.Fatalf("lifecycle = %#v, want startedAt", session.Lifecycle)
		}
		if session.ResultSummary == nil {
			t.Fatal("resultSummary missing")
		}
		return sharedPublicSession{
			SessionID:     session.SessionID,
			Status:        session.Status,
			SourceHash:    session.SourceHash,
			PolicyHash:    session.Policy.EffectiveHash,
			Phase:         session.Phase,
			StartedAt:     session.Lifecycle.StartedAt.UTC(),
			ResultStatus:  session.ResultSummary.ResultStatus,
			ArtifactCount: session.ArtifactCount,
			Links:         session.Links,
		}
	}
	petriPublic := sharedProjection(petriProjection)
	javascriptPublic := sharedProjection(javascriptProjection)
	if petriPublic != javascriptPublic {
		t.Fatalf("shared public session projection differs by orchestrator:\nPETRI: %#v\nJAVASCRIPT: %#v", petriPublic, javascriptPublic)
	}
	if petriProjection.OrchestratorKind != "PETRI" || javascriptProjection.OrchestratorKind != "JAVASCRIPT" {
		t.Fatalf("orchestrator identity lost: PETRI=%q JAVASCRIPT=%q", petriProjection.OrchestratorKind, javascriptProjection.OrchestratorKind)
	}
	if petriPayload.TransitionId == "" || javascriptPayload.Label == "" {
		t.Fatal("typed orchestrator-specific facts missing")
	}
}

func TestReplayDispatchProjection_EquivalentOrchestratorsMatchLiveDispatchSummary(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	providerRef := ProviderSessionRef{Provider: "mock", Kind: "session_id", ID: "provider-session-parity-1"}

	for _, orchestratorKind := range []string{"PETRI", "JAVASCRIPT"} {
		t.Run(orchestratorKind, func(t *testing.T) {
			session := SessionReadResult{
				SessionID:        "dispatch-parity-" + strings.ToLower(orchestratorKind),
				Status:           LifecycleStatusSucceeded,
				OrchestratorKind: orchestratorKind,
				Phase:            "execute",
				Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
			}
			live := DispatchSummary{
				ID:                  "dispatch-equivalent-1",
				Status:              DispatchStatusCompleted,
				DispatchKind:        "AGENT",
				Phase:               "execute",
				Label:               "summarize findings",
				RunnerID:            "worker-summary",
				Model:               "mock-model",
				Provider:            "mock",
				ProviderSessionRefs: []ProviderSessionRef{providerRef},
				OutputArtifactIDs:   []string{"artifact-equivalent-1"},
			}
			events := BuildCanonicalRuntimeSessionEvents(
				session,
				ResultReadResult{SessionID: session.SessionID, ResultStatus: ResultStatusFinal},
				RuntimeDispatchEventInput{Dispatches: []DispatchSummary{live}},
			)

			replayed, err := ReplayDispatchProjection(events)
			if err != nil {
				t.Fatalf("ReplayDispatchProjection: %v", err)
			}
			if len(replayed) != 1 || !reflect.DeepEqual(replayed[0], live) {
				t.Fatalf("replayed dispatch = %#v, want live projection %#v", replayed, live)
			}
		})
	}
}

func TestReplayDispatchProjection_EquivalentOrchestratorsPreserveAbsentProviderSession(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 11, 20, 5, 0, 0, time.UTC)
	for _, orchestratorKind := range []string{"PETRI", "JAVASCRIPT"} {
		t.Run(orchestratorKind, func(t *testing.T) {
			session := SessionReadResult{
				SessionID:        "dispatch-no-provider-" + strings.ToLower(orchestratorKind),
				Status:           LifecycleStatusFailed,
				OrchestratorKind: orchestratorKind,
				Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
			}
			live := DispatchSummary{ID: "dispatch-no-provider", Status: DispatchStatusFailed, DispatchKind: "AGENT"}
			events := BuildCanonicalRuntimeSessionEvents(
				session,
				ResultReadResult{SessionID: session.SessionID, ResultStatus: ResultStatusUnavailable},
				RuntimeDispatchEventInput{Dispatches: []DispatchSummary{live}},
			)
			replayed, err := ReplayDispatchProjection(events)
			if err != nil {
				t.Fatalf("ReplayDispatchProjection: %v", err)
			}
			if len(replayed) != 1 || len(replayed[0].ProviderSessionRefs) != 0 {
				t.Fatalf("replayed dispatch = %#v, want absent provider session refs", replayed)
			}
		})
	}
}

func TestReplaySessionProjection_EquivalentOrchestratorsRestoreArtifactsAndLatestLifecycle(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 11, 21, 0, 0, 0, time.UTC)
	pausedAt := startedAt.Add(time.Minute)
	resumedAt := pausedAt.Add(time.Minute)
	artifact := map[string]any{
		"id":          "artifact-parity-1",
		"kind":        "FINAL_RESULT",
		"visibility":  "PUBLIC",
		"contentHash": "sha256:artifact-parity",
		"sizeBytes":   128,
	}

	for _, orchestratorKind := range []string{"PETRI", "JAVASCRIPT"} {
		t.Run(orchestratorKind, func(t *testing.T) {
			sessionID := "artifact-parity-" + strings.ToLower(orchestratorKind)
			session := SessionReadResult{
				SessionID:        sessionID,
				Status:           LifecycleStatusRunning,
				OrchestratorKind: orchestratorKind,
				Lifecycle: &LifecycleTimestamps{
					StartedAt: &startedAt,
					PausedAt:  &pausedAt,
					ResumedAt: &resumedAt,
				},
			}
			events := BuildCanonicalRuntimeSessionEvents(session, ResultReadResult{
				SessionID:    sessionID,
				ResultStatus: ResultStatusPartial,
				ArtifactIDs:  []string{"artifact-parity-1"},
			})
			artifactEvent := canonicalTypedInternalEvent(t, "ARTIFACT_CREATED", sessionID, map[string]any{
				"artifact":   artifact,
				"capturedAt": resumedAt.Format(time.RFC3339),
			})
			var artifactEnvelope map[string]any
			if err := json.Unmarshal(artifactEvent, &artifactEnvelope); err != nil {
				t.Fatalf("unmarshal artifact envelope: %v", err)
			}
			context := artifactEnvelope["context"].(map[string]any)
			context["orchestratorKind"] = orchestratorKind
			artifactEvent, _ = json.Marshal(artifactEnvelope)

			internalType := "DISPATCH_REQUESTED"
			internalPayload := map[string]any{"transitionId": "transition-1"}
			if orchestratorKind == "JAVASCRIPT" {
				internalType = "ORCHESTRATOR_CHECKPOINT_WRITTEN"
				internalPayload = map[string]any{"checkpointId": "checkpoint-1"}
			}
			events = append(events, artifactEvent, canonicalTypedInternalEvent(t, internalType, sessionID, internalPayload))

			replayed, _, err := ReplaySessionProjection(events)
			if err != nil {
				t.Fatalf("ReplaySessionProjection: %v", err)
			}
			wantRef := ArtifactRefSummary{ID: "artifact-parity-1", Kind: "FINAL_RESULT", Visibility: "PUBLIC", ContentHash: "sha256:artifact-parity", SizeBytes: 128}
			if replayed.ArtifactCount != 1 || !reflect.DeepEqual(replayed.ArtifactRefs, []ArtifactRefSummary{wantRef}) {
				t.Fatalf("artifact projection = count %d refs %#v, want %#v", replayed.ArtifactCount, replayed.ArtifactRefs, wantRef)
			}
			if replayed.Status != LifecycleStatusRunning || replayed.Lifecycle == nil || !replayed.Lifecycle.ResumedAt.Equal(resumedAt) {
				t.Fatalf("lifecycle = status %q timestamps %#v, want latest RUNNING at %s", replayed.Status, replayed.Lifecycle, resumedAt)
			}
		})
	}
}

func TestReplaySessionProjection_EquivalentOrchestratorsPreserveResultAvailability(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 11, 22, 0, 0, 0, time.UTC)
	tests := []struct {
		name             string
		sessionStatus    LifecycleStatus
		resultStatus     ResultStatus
		primaryResult    json.RawMessage
		precedingPartial bool
	}{
		{
			name:          "partial result without terminal result",
			sessionStatus: LifecycleStatusRunning,
			resultStatus:  ResultStatusPartial,
			primaryResult: json.RawMessage(`[{"type":"text","text":"partial output"}]`),
		},
		{
			name:          "terminal result",
			sessionStatus: LifecycleStatusSucceeded,
			resultStatus:  ResultStatusFinal,
			primaryResult: json.RawMessage(`[{"type":"text","text":"final output"}]`),
		},
		{
			name:             "terminal result supersedes partial result",
			sessionStatus:    LifecycleStatusSucceeded,
			resultStatus:     ResultStatusFinal,
			primaryResult:    json.RawMessage(`[{"type":"text","text":"final output after partial"}]`),
			precedingPartial: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sharedResult *ResultReadResult
			for _, orchestratorKind := range []string{"PETRI", "JAVASCRIPT"} {
				sessionID := "result-parity-" + strings.ToLower(orchestratorKind)
				session := SessionReadResult{
					SessionID:        sessionID,
					Status:           test.sessionStatus,
					OrchestratorKind: orchestratorKind,
					Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
				}
				if IsTerminalLifecycleStatus(test.sessionStatus) {
					finishedAt := startedAt.Add(time.Minute)
					session.Lifecycle.FinishedAt = &finishedAt
				}
				live := ResultReadResult{
					SessionID:     sessionID,
					SessionStatus: test.sessionStatus,
					ResultStatus:  test.resultStatus,
					PrimaryResult: test.primaryResult,
				}
				events := BuildCanonicalRuntimeSessionEvents(session, live)
				if test.precedingPartial {
					partialEvent := canonicalTypedInternalEvent(t, "SESSION_RESULT_UPDATED", sessionID, map[string]any{
						"resultStatus":  "PARTIAL",
						"resultSummary": []map[string]any{{"type": "text", "text": "earlier partial output"}},
					})
					events = append(events[:1], append([]json.RawMessage{partialEvent}, events[1:]...)...)
				}

				replayedSession, replayedResult, err := ReplaySessionProjection(events)
				if err != nil {
					t.Fatalf("ReplaySessionProjection(%s): %v", orchestratorKind, err)
				}
				if replayedResult.ResultStatus != test.resultStatus || replayedResult.SessionStatus != test.sessionStatus {
					t.Fatalf("%s result availability = (%q, %q), want (%q, %q)", orchestratorKind, replayedResult.ResultStatus, replayedResult.SessionStatus, test.resultStatus, test.sessionStatus)
				}
				if !jsonEqual(replayedResult.PrimaryResult, test.primaryResult) {
					t.Fatalf("%s primary result = %s, want %s", orchestratorKind, replayedResult.PrimaryResult, test.primaryResult)
				}
				if replayedSession.ResultSummary == nil || replayedSession.ResultSummary.ResultStatus != string(test.resultStatus) {
					t.Fatalf("%s session result summary = %#v, want %q", orchestratorKind, replayedSession.ResultSummary, test.resultStatus)
				}

				comparable := replayedResult
				comparable.SessionID = ""
				if sharedResult == nil {
					sharedResult = &comparable
				} else if !reflect.DeepEqual(*sharedResult, comparable) {
					t.Fatalf("shared result projection differs by orchestrator:\nfirst: %#v\n%s: %#v", *sharedResult, orchestratorKind, comparable)
				}
			}
		})
	}
}
