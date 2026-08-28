package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func replaySession(
	sessionID, orchestratorKind string,
	status LifecycleStatus,
	startedAt *time.Time,
	sourceHash, policyHash, phase string,
) SessionReadResult {
	return SessionReadResult{
		SessionID:        sessionID,
		Status:           status,
		OrchestratorKind: orchestratorKind,
		SourceHash:       sourceHash,
		Policy:           PolicyProjection{EffectiveHash: policyHash},
		ResolvedSource:   ResolvedSource{SourceHash: sourceHash},
		Lifecycle:        &LifecycleTimestamps{StartedAt: startedAt},
		Phase:            phase,
	}
}

func javascriptReplaySession(sessionID string, status LifecycleStatus, startedAt *time.Time) SessionReadResult {
	return replaySession(sessionID, "JAVASCRIPT", status, startedAt, "sha256:fixture", "sha256:policy", "")
}

func javascriptReplayEvents(
	sessionID string,
	status LifecycleStatus,
	startedAt *time.Time,
	result ResultReadResult,
) []json.RawMessage {
	result.SessionID = sessionID
	return BuildCanonicalRuntimeSessionEvents(javascriptReplaySession(sessionID, status, startedAt), result)
}

func canonicalReplayEvents(session SessionReadResult, status ResultStatus) []json.RawMessage {
	return BuildCanonicalSessionEvents(session, ResultReadResult{SessionID: session.SessionID, ResultStatus: status})
}

func replayResult(sessionID string, status ResultStatus, artifactIDs ...string) ResultReadResult {
	return ResultReadResult{SessionID: sessionID, ResultStatus: status, ArtifactIDs: artifactIDs}
}

func replayResultAvailability(sessionID string, sessionStatus LifecycleStatus, status ResultStatus, primary json.RawMessage) ResultReadResult {
	return ResultReadResult{SessionID: sessionID, SessionStatus: sessionStatus, ResultStatus: status, PrimaryResult: primary}
}

func syncTimeoutReplayResult(sessionID string) ResultReadResult {
	return ResultReadResult{SessionID: sessionID, ResultStatus: ResultStatusNotReady, Availability: &ResultAvailabilityDetail{Reason: "SYNC_WAIT_TIMED_OUT", Message: "Sync wait ended before a terminal result was available.", Retryable: true}}
}

func orchestratorReplaySession(
	sessionID, orchestratorKind string,
	status LifecycleStatus,
	startedAt *time.Time,
) SessionReadResult {
	return replaySession(sessionID, orchestratorKind, status, startedAt, "", "", "")
}

func replayOrchestratorSessionID(prefix, orchestratorKind string) string {
	return prefix + "-" + strings.ToLower(orchestratorKind)
}

func replayDispatchProjectionForOrchestrator(
	t *testing.T,
	prefix, orchestratorKind string,
	sessionStatus LifecycleStatus,
	resultStatus ResultStatus,
	startedAt *time.Time,
	phase string,
	live DispatchSummary,
) []DispatchSummary {
	t.Helper()
	sessionID := replayOrchestratorSessionID(prefix, orchestratorKind)
	session := orchestratorReplaySession(sessionID, orchestratorKind, sessionStatus, startedAt)
	session.Phase = phase
	events := BuildCanonicalRuntimeSessionEvents(
		session,
		ResultReadResult{SessionID: sessionID, ResultStatus: resultStatus},
		RuntimeDispatchEventInput{Dispatches: []DispatchSummary{live}},
	)
	replayed, err := ReplayDispatchProjection(events)
	if err != nil {
		t.Fatalf("ReplayDispatchProjection: %v", err)
	}
	return replayed
}

func replaySessionProjection(t *testing.T, events []json.RawMessage) (SessionReadResult, ResultReadResult) {
	t.Helper()
	session, result, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	return session, result
}

func replaySessionPublicFields(t *testing.T, session SessionReadResult) [9]any {
	t.Helper()
	if session.Lifecycle == nil || session.Lifecycle.StartedAt == nil {
		t.Fatalf("lifecycle = %#v, want startedAt", session.Lifecycle)
	}
	if session.ResultSummary == nil {
		t.Fatal("resultSummary missing")
	}
	return [9]any{session.SessionID, session.Status, session.SourceHash, session.Policy.EffectiveHash, session.Phase, session.Lifecycle.StartedAt.UTC(), session.ResultSummary.ResultStatus, session.ArtifactCount, session.Links}
}

func filteredReconnectEvents(
	t *testing.T,
	events []json.RawMessage,
	request EventReconnectRequest,
	sessionID string,
	want int,
) []json.RawMessage {
	t.Helper()
	filtered, err := FilterEventsAfterReconnect(events, request, sessionID)
	if err != nil {
		t.Fatalf("FilterEventsAfterReconnect: %v", err)
	}
	if len(filtered) != want {
		t.Fatalf("filtered events = %d, want %d", len(filtered), want)
	}
	return filtered
}

func TestPersistedTokenFailureHistoryRetainsHeadTailAndReloads(t *testing.T) {
	const historySize = defaultPersistedTokenFailureLogCapacity + 8
	history := failureHistoryForRetryCount(historySize)
	state := runtimeSessionState{
		petriMutations: []interfaces.TokenMutationRecord{failureMutation(history, 1)},
	}

	snapshot := persistedSnapshotFromRuntimeStateWithFailureLogCapacity(state, defaultPersistedTokenFailureLogCapacity)
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

	snapshot := persistedSnapshotFromRuntimeStateWithFailureLogCapacity(state, defaultPersistedTokenFailureLogCapacity)
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
		liveChildDispatchRecord("dispatch-2", factory.JavaScriptChildDispatchStatusQueued, "child-1", ""),
		liveChildDispatchRecord("dispatch-2", factory.JavaScriptChildDispatchStatusRunning, "child-1", ""),
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

	filteredReconnectEvents(t, events, EventReconnectRequest{}, "s1", 3)
	filteredReconnectEvents(t, events, EventReconnectRequest{
		AfterEventID: "session-started/s1",
	}, "s1", 2)

	sequence := 1
	filteredReconnectEvents(t, events, EventReconnectRequest{
		AfterSequence: &sequence,
	}, "s1", 1)

	eventsWithoutSessionSequence := append([]json.RawMessage(nil), events...)
	eventsWithoutSessionSequence[1] = json.RawMessage(
		`{"id":"session-result-updated/s1","context":{"sequence":42}}`,
	)
	canonicalSequence := 42
	filteredReconnectEvents(t, eventsWithoutSessionSequence, EventReconnectRequest{
		AfterSequence: &canonicalSequence,
	}, "s1", 1)

	_, err := FilterEventsAfterReconnect(events, EventReconnectRequest{
		AfterEventID: "missing-event",
	}, "s1")
	if !errors.Is(err, ErrReconnectCursorNotFound) {
		t.Fatalf("missing cursor error = %v, want ErrReconnectCursorNotFound", err)
	}
}

const (
	terminalWorkerSuccess      = "success"
	terminalWorkerFailure      = "failure"
	terminalWorkerCancellation = "cancellation"
	terminalWorkerTimeout      = "timeout"
)

type childTerminalResponseCase struct {
	name          string
	behavior      string
	progress      []workerexecution.ProgressFragment
	wantKind      responseevents.Kind
	wantPhase     responseevents.Phase
	wantErrorCode string
}

func TestChildWorkerExecutor_PublishesExactlyOneDurableTerminalResponseForEveryOutcome(t *testing.T) {
	t.Parallel()
	for _, test := range childTerminalResponseCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runChildTerminalResponseCase(t, test)
		})
	}
}

func terminalResponseCase(name, behavior string, progress []workerexecution.ProgressFragment, kind responseevents.Kind, phase responseevents.Phase, errorCode string) childTerminalResponseCase {
	return childTerminalResponseCase{name: name, behavior: behavior, progress: progress, wantKind: kind, wantPhase: phase, wantErrorCode: errorCode}
}

func childTerminalResponseCases() []childTerminalResponseCase {
	return []childTerminalResponseCase{
		terminalResponseCase("success with provider terminal progress", terminalWorkerSuccess, []workerexecution.ProgressFragment{{Kind: workerexecution.ProgressFragmentKind, Payload: "provider progress"}, {Kind: workerexecution.CompletedFragmentKind, Type: "COMPLETED"}}, responseevents.KindRun, responseevents.PhaseCompleted, ""),
		terminalResponseCase("failure without provider terminal progress", terminalWorkerFailure, []workerexecution.ProgressFragment{{Kind: workerexecution.ProgressFragmentKind, Payload: "failure progress"}}, responseevents.KindError, responseevents.PhaseFailed, "stream_failed"),
		terminalResponseCase("cancellation without provider terminal progress", terminalWorkerCancellation, nil, responseevents.KindError, responseevents.PhaseFailed, "stream_canceled"),
		terminalResponseCase("timeout without provider terminal progress", terminalWorkerTimeout, nil, responseevents.KindError, responseevents.PhaseFailed, "timeout"),
	}
}

func runChildTerminalResponseCase(t *testing.T, test childTerminalResponseCase) {
	t.Helper()
	const sessionID = "dur-sess-terminal-bridge"
	service := newDurableResponseEventsService(t)
	state := seedResponseEventSession(t, service, sessionID)
	if err := service.ensureSessionResponseEvents(sessionID, state); err != nil {
		t.Fatalf("ensure response events: %v", err)
	}

	provider, started := newTerminalWorkerProvider(test.behavior)
	workerService := newTerminalWorkersService(t, provider)
	execution := terminalWorkerExecution{service: workerService, progress: test.progress}
	executor := newChildWorkerExecutor(
		sessionID, execution, newChildRecordSink(), childTestValues{},
		service.observeWorkerDispatch, "/project", 0,
	)
	executor.publish = func(_ string, fragment workerexecution.ProgressFragment) {
		service.PublishWorkerProgress(fragment)
	}
	executionErr := executeTerminalChild(t, executor, test.behavior, started)

	cursor, err := service.SubscribeResponseEvents(context.Background(), sessionID, factorysessions.ResponseEventSubscriptionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatalf("SubscribeResponseEvents: %v", err)
	}
	defer cursor.Detach()
	events, err := cursor.Drain()
	if err != nil {
		t.Fatalf("Drain response events: %v", err)
	}
	assertTerminalResponse(t, events, test, executionErr)
}

func executeTerminalChild(
	t *testing.T,
	executor *childWorkerExecutor,
	behavior string,
	started <-chan struct{},
) error {
	t.Helper()
	request := factory.JavaScriptChildExecutionRequest{
		Prompt: "run", Preset: "agent", ModelProvider: "codex", Model: "terminal-model",
	}
	if behavior == terminalWorkerSuccess || behavior == terminalWorkerFailure {
		_, err := executor.Execute(context.Background(), request)
		return err
	}

	ctx := context.Background()
	var cancel context.CancelFunc
	if behavior == terminalWorkerCancellation {
		ctx, cancel = context.WithCancel(ctx)
	} else {
		ctx, cancel = context.WithTimeout(ctx, 2*time.Second)
	}
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := executor.Execute(ctx, request)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("real Workers provider was not started")
	}
	if behavior == terminalWorkerCancellation {
		cancel()
	} else {
		<-ctx.Done()
	}
	return <-done
}

func assertTerminalResponse(t *testing.T, events []responseevents.FactoryResponseEvent, test childTerminalResponseCase, executionErr error) {
	t.Helper()
	assertTerminalExecutionResult(t, test, executionErr)
	terminal := terminalResponseWithExpectedOrder(t, events, test)
	assertTerminalKindAndPhase(t, terminal, test)
	assertTerminalErrorPayload(t, terminal, test)
}

func assertTerminalExecutionResult(t *testing.T, test childTerminalResponseCase, executionErr error) {
	t.Helper()
	if test.behavior == terminalWorkerSuccess {
		if executionErr != nil {
			t.Fatalf("success child execution error = %v", executionErr)
		}
		return
	}
	if executionErr == nil {
		t.Fatal("unhappy child execution error = nil")
	}
}

func terminalResponseWithExpectedOrder(t *testing.T, events []responseevents.FactoryResponseEvent, test childTerminalResponseCase) responseevents.FactoryResponseEvent {
	t.Helper()
	terminals := terminalResponseEvents(events)
	if len(terminals) != 1 {
		t.Fatalf("response events = %#v, want exactly one terminal event", events)
	}
	terminal := terminals[0]
	if len(test.progress) > 0 && (len(events) < 2 || events[len(events)-1].EventID != terminal.EventID) {
		t.Fatalf("events = %#v, want progress before the terminal event", events)
	}
	return terminal
}

func assertTerminalKindAndPhase(t *testing.T, terminal responseevents.FactoryResponseEvent, test childTerminalResponseCase) {
	t.Helper()
	if terminal.Kind != test.wantKind || terminal.Phase != test.wantPhase {
		t.Fatalf("terminal event = %#v, want kind=%q phase=%q", terminal, test.wantKind, test.wantPhase)
	}
}

func assertTerminalErrorPayload(t *testing.T, terminal responseevents.FactoryResponseEvent, test childTerminalResponseCase) {
	t.Helper()
	if test.wantErrorCode == "" {
		return
	}
	var payload responseevents.ErrorPayload
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
		t.Fatalf("decode terminal error payload: %v", err)
	}
	if payload.Code != test.wantErrorCode {
		t.Fatalf("terminal error code = %q, want %q", payload.Code, test.wantErrorCode)
	}
}

func newTerminalWorkerProvider(behavior string) (providers.Service, <-chan struct{}) {
	started := make(chan struct{})
	var once sync.Once
	provider := testutil.NativeProvider{
		ExecuteFunc: func(ctx context.Context, _ providers.ExecuteRequest) (providers.ExecuteResult, error) {
			once.Do(func() { close(started) })
			switch behavior {
			case terminalWorkerSuccess:
				return providers.ExecuteResult{
					Outcome: providers.ExecuteOutcomeAccepted, Content: "completed",
				}, nil
			case terminalWorkerFailure:
				return providers.ExecuteResult{}, errors.New("provider failed")
			default:
				<-ctx.Done()
				return providers.ExecuteResult{}, ctx.Err()
			}
		},
	}
	return provider, started
}

type terminalWorkerExecution struct {
	service  WorkerExecution
	progress []workerexecution.ProgressFragment
}

func (execution terminalWorkerExecution) Execute(
	ctx context.Context,
	request workerexecution.ExecuteRequest,
) (workerexecution.ExecuteResult, error) {
	if request.Input.ProgressPublisher == nil {
		return workerexecution.ExecuteResult{}, errors.New("progress publisher is required")
	}
	for _, fragment := range execution.progress {
		request.Input.ProgressPublisher(fragment)
	}
	return execution.service.Execute(ctx, request)
}

func terminalResponseEvents(events []responseevents.FactoryResponseEvent) []responseevents.FactoryResponseEvent {
	terminals := make([]responseevents.FactoryResponseEvent, 0, 2)
	for _, event := range events {
		isRunTerminal := event.Kind == responseevents.KindRun && event.Phase == responseevents.PhaseCompleted
		isErrorTerminal := event.Kind == responseevents.KindError &&
			(event.Phase == responseevents.PhaseFailed || event.Phase == responseevents.PhaseCanceled)
		if isRunTerminal || isErrorTerminal {
			terminals = append(terminals, event)
		}
	}
	return terminals
}

func TestReplaySessionProjection_EquivalentOrchestratorsSharePublicSessionProjection(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 11, 18, 30, 0, 0, time.UTC)
	sessionID := "dur-sess-orchestrator-parity-001"
	baseSession := replaySession(
		sessionID, "", LifecycleStatusRunning, &startedAt,
		"sha256:equivalent-source", "sha256:equivalent-policy", "execute",
	)
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

	petriProjection, _ := replaySessionProjection(t, petriEvents)
	javascriptProjection, _ := replaySessionProjection(t, javascriptEvents)

	petriPublic := replaySessionPublicFields(t, petriProjection)
	javascriptPublic := replaySessionPublicFields(t, javascriptProjection)
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
			replayed := replayDispatchProjectionForOrchestrator(
				t, "dispatch-parity", orchestratorKind, LifecycleStatusSucceeded, ResultStatusFinal, &startedAt, "execute", live,
			)
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
			live := DispatchSummary{ID: "dispatch-no-provider", Status: DispatchStatusFailed, DispatchKind: "AGENT"}
			replayed := replayDispatchProjectionForOrchestrator(
				t, "dispatch-no-provider", orchestratorKind, LifecycleStatusFailed, ResultStatusUnavailable, &startedAt, "", live,
			)
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
			sessionID := replayOrchestratorSessionID("artifact-parity", orchestratorKind)
			session := orchestratorReplaySession(sessionID, orchestratorKind, LifecycleStatusRunning, &startedAt)
			session.Lifecycle.PausedAt = &pausedAt
			session.Lifecycle.ResumedAt = &resumedAt
			events := BuildCanonicalRuntimeSessionEvents(session, replayResult(sessionID, ResultStatusPartial, "artifact-parity-1"))
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

			replayed, _ := replaySessionProjection(t, events)
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
				sessionID := replayOrchestratorSessionID("result-parity", orchestratorKind)
				session := orchestratorReplaySession(sessionID, orchestratorKind, test.sessionStatus, &startedAt)
				if IsTerminalLifecycleStatus(test.sessionStatus) {
					finishedAt := startedAt.Add(time.Minute)
					session.Lifecycle.FinishedAt = &finishedAt
				}
				live := replayResultAvailability(sessionID, test.sessionStatus, test.resultStatus, test.primaryResult)
				events := BuildCanonicalRuntimeSessionEvents(session, live)
				if test.precedingPartial {
					partialEvent := canonicalTypedInternalEvent(t, "SESSION_RESULT_UPDATED", sessionID, map[string]any{
						"resultStatus":  "PARTIAL",
						"resultSummary": []map[string]any{{"type": "text", "text": "earlier partial output"}},
					})
					events = append(events[:1], append([]json.RawMessage{partialEvent}, events[1:]...)...)
				}

				replayedSession, replayedResult := replaySessionProjection(t, events)
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
