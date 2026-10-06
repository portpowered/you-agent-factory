package factorysessionexecution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livechange"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPersistedRuntimeStateRestoresRequestWorkerSettings(t *testing.T) {
	state := runtimeSessionState{startRequest: &StartRequest{WorkerSettings: &factory.JavaScriptWorkerSettings{
		Presets: map[string]factory.JavaScriptWorkerPreset{"review": {ModelProvider: "codex", Model: "test-model"}},
	}}}
	snapshot := persistedSnapshotFromRuntimeStateWithFailureLogCapacity(state, 0)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PersistedRuntimeSessionState
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	restored := runtimeStateFromPersistedSnapshot(decoded)
	if restored.startRequest == nil || restored.startRequest.WorkerSettings == nil || restored.startRequest.WorkerSettings.Presets["review"].Model != "test-model" {
		t.Fatalf("restored worker settings = %#v", restored.startRequest)
	}
}

func TestChildWorkerExecutor_PreservesTypedProviderReasonWithoutSessionReference(t *testing.T) {
	const rejection = "Agy does not support a separate reasoning effort."
	invoker := &recordingWorkerExecution{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypePermanentBadRequest,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: "Provider rejected the request as invalid.",
			Detail: &workers.FailureDetail{
				Reason:  workers.WorkFailureTypePermanentBadRequest,
				Message: rejection,
			},
		},
		Diagnostics: &workers.SafeDiagnostics{
			Provider: &workers.SafeProviderDiagnostic{Provider: "antigravity"},
		},
	}}
	sink := newChildRecordSink()
	executor := newTestChildWorkerExecutor(invoker, sink, nil)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:        "invalid Antigravity request",
		ModelProvider: "antigravity",
		Model:         "gemini-3.6-flash-medium",
	})
	if err == nil || !strings.Contains(err.Error(), "Antigravity") || !strings.Contains(err.Error(), rejection) {
		t.Fatalf("child error = %v, want provider identity and safe rejection reason", err)
	}
	if result.Status != factory.JavaScriptChildDispatchStatusFailed {
		t.Fatalf("child status = %q, want FAILED", result.Status)
	}
	terminal := sink.terminalChildDispatch(t)
	if terminal.Provider != "antigravity" {
		t.Fatalf("terminal provider = %q, want antigravity without a session reference", terminal.Provider)
	}
	if terminal.FailureDetail == nil || terminal.FailureDetail.Message != rejection {
		t.Fatalf("terminal failure detail = %#v, want typed provider reason", terminal.FailureDetail)
	}
	summary := dispatchSummaryFromChildRecord("FAILED", terminal)
	if summary.Provider != "antigravity" {
		t.Fatalf("dispatch summary provider = %q, want antigravity", summary.Provider)
	}
	if summary.FailureDetail == nil || summary.FailureDetail.Message != rejection {
		t.Fatalf("dispatch summary failure detail = %#v, want typed provider reason", summary.FailureDetail)
	}
}

func TestTaggedDurableHistoryIsAuthoritativeDuringHydrationAndResave(t *testing.T) {
	t.Parallel()
	canonical := json.RawMessage(`{"type":"SESSION_STARTED","context":{"sessionId":"dur-sess-tagged"}}`)
	checkpoint := factory.JavaScriptRuntimeRecord{
		Sequence: 2,
		Kind:     factory.JavaScriptRecordKindCheckpoint,
		Checkpoint: &factory.JavaScriptCheckpointRecord{
			ID: "checkpoint-tagged", State: map[string]any{"position": "review"},
		},
	}
	mutation := interfaces.TokenMutationRecord{
		Type: interfaces.MutationMove, TransitionID: "approve", ToPlace: "review:approved",
	}
	snapshot := PersistedRuntimeSessionState{
		Events:         []json.RawMessage{json.RawMessage(`{"type":"LEGACY_EVENT"}`)},
		RuntimeRecords: []factory.JavaScriptRuntimeRecord{{Sequence: 99, Kind: factory.JavaScriptRecordKindPhase}},
		Records: []DurableSessionRecord{
			{Kind: DurableRecordKindCanonicalFactoryEvent, CanonicalEvent: canonical},
			{Kind: DurableRecordKindJavaScriptRuntime, JavaScriptRecord: &checkpoint},
			{Kind: DurableRecordKindPetriTokenMutation, PetriMutation: &mutation},
		},
	}

	hydrated := runtimeStateFromPersistedSnapshot(snapshot)
	if len(hydrated.events) != 1 || !bytes.Equal(hydrated.events[0], canonical) {
		t.Fatalf("hydrated events = %s, want tagged canonical event", hydrated.events)
	}
	if len(hydrated.runtimeRecords) != 1 || hydrated.runtimeRecords[0].Checkpoint == nil || hydrated.runtimeRecords[0].Checkpoint.ID != "checkpoint-tagged" {
		t.Fatalf("hydrated JavaScript records = %#v, want tagged checkpoint", hydrated.runtimeRecords)
	}
	if len(hydrated.petriMutations) != 1 || hydrated.petriMutations[0].TransitionID != "approve" || hydrated.petriMutations[0].ToPlace != "review:approved" {
		t.Fatalf("hydrated Petri mutations = %#v, want tagged transition", hydrated.petriMutations)
	}

	resaved := persistedSnapshotFromRuntimeStateWithFailureLogCapacity(hydrated, defaultPersistedTokenFailureLogCapacity)
	if len(resaved.Records) != 3 || resaved.Records[2].PetriMutation == nil || resaved.Records[2].PetriMutation.TransitionID != "approve" {
		t.Fatalf("resaved tagged history = %#v, want lossless mixed records", resaved.Records)
	}
}

func TestJavaScriptRuntimeService_DurableLiveChangeSharesAdmissionWithChildLeaseAndReplays(t *testing.T) {
	const sessionID = "dur-sess-live-capacity-admission"
	runtime := newDurableLiveChangeAdmissionTestRuntime(t)
	service := &JavaScriptRuntimeService{
		clock:                 runtimeTestClock{now: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)},
		liveChangeCoordinator: livechange.NewCoordinator(),
		sessions: map[string]*runtimeSessionState{
			sessionID: {session: SessionReadResult{SessionID: sessionID, Status: LifecycleStatusRunning}},
		},
	}
	releaseScope := service.BindLiveChangeScope(sessionID, runtimebinding.NewLiveChangeApplication(runtime),
		runtimebinding.NewLiveChangeAdmission(runtime), true, nil)
	defer releaseScope()
	request := factorysessions.LiveChangeRequest{
		RequestID:        "request-live-capacity-admission",
		ExpectedRevision: 0,
		Operation:        "resource.capacity.set",
		TargetID:         "reviewers",
		RequestedValue:   json.RawMessage("1"),
		Source:           "test",
	}

	if err := runtime.holdChildResourceLease(context.Background(), factory.ResourceCapacityLeaseRequest{ResourceID: "reviewers"}); err != nil {
		t.Fatalf("acquire child resource lease: %v", err)
	}
	defer runtime.releaseHeldChildResourceLease()

	firstDone := startDurableLiveChange(service, sessionID, request)
	<-runtime.changeAdmissionAttempt
	peerRuntime := newDurableLiveChangeAdmissionTestRuntime(t)
	releasePeer := service.BindLiveChangeScope("dur-sess-live-capacity-peer",
		runtimebinding.NewLiveChangeApplication(peerRuntime), runtimebinding.NewLiveChangeAdmission(peerRuntime), true, nil)
	defer releasePeer()
	assertDurableLiveChangePending(t, firstDone, "durable live change completed while child lease was held")

	secondDone := startDurableLiveChange(service, sessionID, request)
	assertDurableLiveChangePending(t, secondDone, "duplicate durable live change completed before the first admitted request")

	runtime.releaseHeldChildResourceLease()
	assertDurableLiveChangeOutcome(t, <-firstDone, factorysessions.LiveChangeOutcomeApplied, "first durable live change")
	assertDurableLiveChangeOutcome(t, <-secondDone, factorysessions.LiveChangeOutcomeReplayed, "duplicate durable live change")
	assertDurableLiveChangeApplicationCounts(t, runtime)
	if peerRuntime.setCalls != 0 || peerRuntime.previewCalls != 0 {
		t.Fatalf("held live change crossed peer route: set=%d preview=%d", peerRuntime.setCalls, peerRuntime.previewCalls)
	}

	events := (durableLiveChangeEventLog{service: service, sessionID: sessionID}).LiveChangeEvents()
	assertDurableLiveChangeEvents(t, events)
}

type durableLiveChangeOutcome struct {
	result factorysessions.LiveChangeResult
	err    error
}

func startDurableLiveChange(
	service *JavaScriptRuntimeService,
	sessionID string,
	request factorysessions.LiveChangeRequest,
) <-chan durableLiveChangeOutcome {
	done := make(chan durableLiveChangeOutcome, 1)
	go func() {
		result, err := service.ApplyLiveChange(context.Background(), sessionID, request)
		done <- durableLiveChangeOutcome{result: result, err: err}
	}()
	return done
}

func assertDurableLiveChangePending(t *testing.T, done <-chan durableLiveChangeOutcome, message string) {
	t.Helper()
	select {
	case outcome := <-done:
		t.Fatalf("%s: result=%#v err=%v", message, outcome.result, outcome.err)
	default:
	}
}

func assertDurableLiveChangeOutcome(t *testing.T, outcome durableLiveChangeOutcome, want factorysessions.LiveChangeOutcome, label string) {
	t.Helper()
	if outcome.err != nil || outcome.result.Outcome != want {
		t.Fatalf("%s = %#v, err %v; want outcome %q", label, outcome.result, outcome.err, want)
	}
}

func assertDurableLiveChangeApplicationCounts(t *testing.T, runtime *durableLiveChangeAdmissionTestRuntime) {
	t.Helper()
	if runtime.changeAdmissionAcquires != 2 || runtime.previewCalls != 1 || runtime.setCalls != 1 {
		t.Fatalf("durable admission calls = acquire %d preview %d set %d, want 2/1/1", runtime.changeAdmissionAcquires, runtime.previewCalls, runtime.setCalls)
	}
}

func assertDurableLiveChangeEvents(t *testing.T, events []interfaces.FactoryEvent) {
	t.Helper()
	if len(events) != 2 || events[0].Type != interfaces.FactoryEventTypeFactoryChangeRequest || events[1].Type != interfaces.FactoryEventTypeFactoryChange {
		t.Fatalf("durable live change events = %#v, want one request and one success", events)
	}
}

type runtimeTestClock struct{ now time.Time }

func (c runtimeTestClock) Now() time.Time { return c.now }

type durableLiveChangeAdmissionTestRuntime struct {
	factory.Service
	gate                    chan struct{}
	changeAdmissionAttempt  chan struct{}
	changeAdmissionAcquires int
	previewCalls            int
	setCalls                int
	mu                      sync.Mutex
	snapshot                *interfaces.FactorySnapshot
	changeAdmissionHeld     bool
	childResourceLeaseHeld  bool
}

func newDurableLiveChangeAdmissionTestRuntime(t *testing.T) *durableLiveChangeAdmissionTestRuntime {
	t.Helper()
	snapshot, err := interfaces.NewFactorySnapshot(map[string]any{"resources": map[string]any{"reviewers": map[string]any{"capacity": 1}}})
	if err != nil {
		t.Fatalf("create effective Factory snapshot: %v", err)
	}
	runtime := &durableLiveChangeAdmissionTestRuntime{
		gate:                   make(chan struct{}, 1),
		changeAdmissionAttempt: make(chan struct{}, 1),
		snapshot:               snapshot,
	}
	runtime.gate <- struct{}{}
	return runtime
}

func (r *durableLiveChangeAdmissionTestRuntime) AcquireResourceCapacityAdmission(ctx context.Context) (func(), error) {
	select {
	case r.changeAdmissionAttempt <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.gate:
	}
	r.mu.Lock()
	r.changeAdmissionAcquires++
	r.changeAdmissionHeld = true
	r.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			r.changeAdmissionHeld = false
			r.mu.Unlock()
			r.gate <- struct{}{}
		})
	}, nil
}

func (r *durableLiveChangeAdmissionTestRuntime) holdChildResourceLease(ctx context.Context, request factory.ResourceCapacityLeaseRequest) error {
	if request.ResourceID == "" {
		return errors.New("resource id is required")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.gate:
	}
	r.mu.Lock()
	r.childResourceLeaseHeld = true
	r.mu.Unlock()
	return nil
}

func (r *durableLiveChangeAdmissionTestRuntime) releaseHeldChildResourceLease() {
	r.mu.Lock()
	if !r.childResourceLeaseHeld {
		r.mu.Unlock()
		return
	}
	r.childResourceLeaseHeld = false
	r.mu.Unlock()
	r.gate <- struct{}{}
}

func (r *durableLiveChangeAdmissionTestRuntime) PreviewResourceCapacity(context.Context, factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	return factory.ResourceCapacityResult{}, errors.New("non-admitted preview should not be used")
}

func (r *durableLiveChangeAdmissionTestRuntime) SetResourceCapacity(context.Context, factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	return factory.ResourceCapacityResult{}, errors.New("non-admitted mutation should not be used")
}

func (r *durableLiveChangeAdmissionTestRuntime) PreviewResourceCapacityAdmitted(_ context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.changeAdmissionHeld {
		return factory.ResourceCapacityResult{}, errors.New("admitted preview called without the live-change admission lease")
	}
	r.previewCalls++
	return r.capacityResult(request), nil
}

func (r *durableLiveChangeAdmissionTestRuntime) SetResourceCapacityAdmitted(_ context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.changeAdmissionHeld {
		return factory.ResourceCapacityResult{}, errors.New("admitted mutation called without the live-change admission lease")
	}
	r.setCalls++
	return r.capacityResult(request), nil
}

func (r *durableLiveChangeAdmissionTestRuntime) capacityResult(request factory.ResourceCapacityRequest) factory.ResourceCapacityResult {
	return factory.ResourceCapacityResult{
		ResourceID:        request.ResourceID,
		PreviousCapacity:  2,
		RequestedCapacity: request.RequestedCapacity,
		EffectiveCapacity: request.RequestedCapacity,
		InUseCount:        0,
		AvailableCount:    request.RequestedCapacity,
		Outcome:           factory.ResourceCapacityOutcomeApplied,
		Factory:           r.snapshot,
	}
}

func TestJavaScriptRuntimeService_StartSync_PersistenceFailureDoesNotPublishSuccess(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{saveErr: errors.New("append unavailable")}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	started, err := service.StartSync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-sync-persist-failure-001",
		simpleFinalWorkflowSource,
		nil,
		nil,
	))
	if err == nil || !strings.Contains(err.Error(), "append unavailable") {
		t.Fatalf("StartSync result = %#v, error = %v, want persistence failure", started, err)
	}

	store.mu.Lock()
	saveCalls := store.saveCalls
	store.mu.Unlock()
	if saveCalls != 1 {
		t.Fatalf("persistence save calls = %d, want exactly one", saveCalls)
	}
	service.mu.RLock()
	defer service.mu.RUnlock()
	for _, state := range service.sessions {
		if state.session.Status == LifecycleStatusSucceeded || state.result.ResultStatus == ResultStatusFinal {
			t.Fatalf("unpersisted success became live: session=%#v result=%#v", state.session, state.result)
		}
	}
}

func TestJavaScriptRuntimeService_StartAsync_PersistenceFailurePublishesFailureNotSuccess(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{saveErr: errors.New("append unavailable")}
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
	})

	started, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-async-persist-failure-001",
		progressThenFinalWorkflowSource,
		nil,
		nil,
	))
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	failed := waitUntilSessionStatus(t, service, started.SessionID, LifecycleStatusFailed, 2*time.Second)
	if failed.Failure == nil || !strings.Contains(failed.Failure.Message, "append unavailable") {
		t.Fatalf("failure = %#v, want explicit persistence failure", failed.Failure)
	}
	assertPersistenceFailureRolledBackLiveProjections(t, service, started.SessionID)

	store.mu.Lock()
	saveCalls := store.saveCalls
	store.mu.Unlock()
	if saveCalls != 1 {
		t.Fatalf("persistence save calls = %d, want exactly one", saveCalls)
	}
}

func assertPersistenceFailureRolledBackLiveProjections(
	t *testing.T,
	service *JavaScriptRuntimeService,
	sessionID string,
) {
	t.Helper()
	result, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModeFinal})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus == ResultStatusFinal {
		t.Fatalf("unpersisted terminal result status = %q, want unavailable", result.ResultStatus)
	}
	dispatches, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 0 {
		t.Fatalf("unpersisted dispatches = %#v, want none", dispatches.Dispatches)
	}
	artifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts.Artifacts) != 0 {
		t.Fatalf("unpersisted artifacts = %#v, want none", artifacts.Artifacts)
	}
	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	encodedEvents, err := json.Marshal(events.Events)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}
	if bytes.Contains(encodedEvents, []byte("unpersisted-output")) || bytes.Contains(encodedEvents, []byte("checkpoint")) {
		t.Fatalf("event history retained unpersisted terminal records: %s", encodedEvents)
	}
	assertPersistenceFailureClearedInternalRuntimeState(t, service, sessionID)
}

func assertPersistenceFailureClearedInternalRuntimeState(t *testing.T, service *JavaScriptRuntimeService, sessionID string) {
	t.Helper()
	service.mu.RLock()
	live := cloneRuntimeSessionState(service.sessions[sessionID])
	service.mu.RUnlock()
	if live.session.Phase != "" || live.session.Progress != nil || len(live.session.ArtifactRefs) != 0 {
		t.Fatalf("session retained unpersisted projection: %#v", live.session)
	}
	if len(live.runtimeRecords) != 0 || live.checkpointSummary != nil {
		t.Fatalf("runtime state retained unpersisted records: records=%#v checkpoint=%#v", live.runtimeRecords, live.checkpointSummary)
	}
}

func TestJavaScriptRuntimeService_TerminateWaitsForTerminalPersistence(t *testing.T) {
	store := newBlockingRuntimePersistenceStore()
	t.Cleanup(store.release)
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Persistence: store,
		Workflows:   scriptedBlockingRuntimeWorkflows(),
	})

	started, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-terminate-persist-barrier-001",
		busyLoopWorkflowSource,
		nil,
		nil,
	))
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	controlDone := make(chan struct{})
	var controlResult LifecycleControlResult
	var controlErr error
	go func() {
		controlResult, controlErr = service.Terminate(context.Background(), started.SessionID, ControlRequest{})
		close(controlDone)
	}()

	select {
	case <-store.saveStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("terminal persistence did not start")
	}
	select {
	case <-controlDone:
		t.Fatal("Terminate returned before terminal persistence completed")
	default:
	}

	store.release()
	select {
	case <-controlDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Terminate did not return after terminal persistence completed")
	}
	if controlErr != nil {
		t.Fatalf("Terminate: %v", controlErr)
	}
	if controlResult.Outcome != LifecycleControlOutcomeAccepted || controlResult.Status != LifecycleStatusTerminated {
		t.Fatalf("Terminate result = %#v, want accepted TERMINATED control", controlResult)
	}
	select {
	case <-store.saveCompleted:
	default:
		t.Fatal("Terminate returned before the persistence writer completed")
	}
}

type blockingRuntimePersistenceStore struct {
	saveStarted    chan struct{}
	releaseSave    chan struct{}
	saveCompleted  chan struct{}
	releaseOnce    sync.Once
	saveStartOnce  sync.Once
	saveFinishOnce sync.Once
}

func newBlockingRuntimePersistenceStore() *blockingRuntimePersistenceStore {
	return &blockingRuntimePersistenceStore{
		saveStarted:   make(chan struct{}),
		releaseSave:   make(chan struct{}),
		saveCompleted: make(chan struct{}),
	}
}

func (s *blockingRuntimePersistenceStore) Save(string, []byte) error {
	s.saveStartOnce.Do(func() { close(s.saveStarted) })
	<-s.releaseSave
	s.saveFinishOnce.Do(func() { close(s.saveCompleted) })
	return nil
}

func (s *blockingRuntimePersistenceStore) Load(string) ([]byte, error) {
	return nil, os.ErrNotExist
}

func (s *blockingRuntimePersistenceStore) release() {
	s.releaseOnce.Do(func() { close(s.releaseSave) })
}

type runtimeRecordingStore struct {
	mu        sync.Mutex
	saveCalls int
	saveErr   error
	payload   []byte
}

func (s *runtimeRecordingStore) Save(_ string, encoded []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.payload = append([]byte(nil), encoded...)
	return nil
}

func (s *runtimeRecordingStore) Load(string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.payload) == 0 {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), s.payload...), nil
}

func testExecutionServicePersistenceChoices(t *testing.T) {
	t.Helper()
	projectRoot := t.TempDir()
	testApplicationPersistencePolicies(t, projectRoot)
	testExecutionServiceRequiredPersistenceDependencies(t, projectRoot)
	testExecutionServiceDisabledPersistence(t, projectRoot)
	testExecutionServiceInvalidPersistenceChoices(t, projectRoot)
}

func testExecutionServiceRequiredPersistenceDependencies(t *testing.T, projectRoot string) {
	t.Helper()
	if _, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{
		ProjectRoot: projectRoot,
		Persistence: DisabledPersistence(),
	}); err == nil {
		t.Fatal("NewExecutionService(runtime without clock) error = nil, want validation error")
	} else if validation, ok := err.(*ValidationError); !ok || validation.Field != "clock" {
		t.Fatalf("runtime without clock error = %#v, want clock ValidationError", err)
	}
	if _, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{ProjectRoot: projectRoot}); err == nil {
		t.Fatal("NewExecutionService(runtime without persistence choice) error = nil, want validation error")
	}
}

func testExecutionServiceDisabledPersistence(t *testing.T, projectRoot string) {
	t.Helper()
	disabled, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{
		ProjectRoot: projectRoot,
		Persistence: DisabledPersistence(),
		Clock:       durableFixedClock{now: time.Now()},
	})
	if err != nil {
		t.Fatalf("NewExecutionService(runtime with disabled persistence): %v", err)
	}
	if disabled.(*JavaScriptRuntimeService).persistence != nil {
		t.Fatal("disabled persistence unexpectedly configured a store")
	}
	// A live runtime constructs without a provider on purpose: a runtime-backed
	// session invokes its children as Workers through a Factory Runtime bound
	// after construction, so there is nothing a constructor could check. What is
	// still rejected is a mode this service cannot serve at all.
	if _, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: ChildExecutorModeLive,
		Persistence:       DisabledPersistence(),
		Clock:             durableFixedClock{now: time.Now()},
	}); err != nil {
		t.Fatalf("NewExecutionService(live runtime without provider) error = %v, want success", err)
	}
	if _, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: "NONSENSE",
		Persistence:       DisabledPersistence(),
		Clock:             durableFixedClock{now: time.Now()},
	}); err == nil {
		t.Fatal("NewExecutionService(unsupported child executor mode) error = nil, want validation error")
	} else if validation, ok := err.(*ValidationError); !ok || validation.Field != "runtime.childExecutorMode" {
		t.Fatalf("unsupported child executor mode error = %#v, want runtime.childExecutorMode ValidationError", err)
	}
}

func testExecutionServiceInvalidPersistenceChoices(t *testing.T, projectRoot string) {
	t.Helper()
	contradictory := PersistenceChoice{store: mustTestRuntimePersistenceStore(t, t.TempDir()), disabled: true}
	if _, err := newExecutionService(ExecutionProviderJavaScriptRuntime, serviceConfig{
		ProjectRoot: projectRoot,
		Persistence: contradictory,
		Clock:       durableFixedClock{now: time.Now()},
	}); err == nil {
		t.Fatal("NewExecutionService(runtime with contradictory persistence) error = nil, want validation error")
	} else if validation, ok := err.(*ValidationError); !ok || validation.Field != "persistence" {
		t.Fatalf("contradictory persistence error = %#v, want persistence ValidationError", err)
	}

	blockedRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedRoot, []byte("blocked"), 0o600); err != nil {
		t.Fatalf("write blocked persistence root: %v", err)
	}
	persistence, err := ProjectPersistence(blockedRoot, testRuntimePersistenceStoreFactory)
	if err != nil {
		t.Fatalf("lazy persistence construction: %v", err)
	}
	if err := persistence.store.Save("~default", []byte(`{}`)); err == nil {
		t.Fatal("Save(unavailable root) error = nil, want persistence error")
	}
}

func testApplicationPersistencePolicies(t *testing.T, projectRoot string) {
	t.Helper()
	for _, tc := range []struct {
		name     string
		policy   PersistencePolicy
		disabled bool
	}{
		{name: "default disabled", disabled: true},
		{name: "enabled", policy: PersistencePolicyEnabled},
		{name: "disabled", policy: PersistencePolicyDisabled, disabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			choice, err := PersistenceChoiceForPolicy(tc.policy, projectRoot, testRuntimePersistenceStoreFactory)
			if err != nil {
				t.Fatalf("PersistenceChoiceForPolicy: %v", err)
			}
			store, err := choice.resolve()
			if err != nil {
				t.Fatalf("resolve policy choice: %v", err)
			}
			if (store == nil) != tc.disabled {
				t.Fatalf("store nil = %t, want disabled = %t", store == nil, tc.disabled)
			}
		})
	}
	if _, err := PersistenceChoiceForPolicy(PersistencePolicy("invalid"), projectRoot, testRuntimePersistenceStoreFactory); err == nil {
		t.Fatal("PersistenceChoiceForPolicy(invalid) error = nil, want validation error")
	} else if validation, ok := err.(*ValidationError); !ok || validation.Field != "persistence.policy" {
		t.Fatalf("invalid policy error = %#v, want persistence.policy ValidationError", err)
	}
}

func TestPersistenceChoiceForPolicy_DefaultDoesNotCreateProjectDurableSessions(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	storeCalls := 0
	choice, err := PersistenceChoiceForPolicy(
		"",
		projectRoot,
		func(string) (runtimepersist.Store, error) {
			storeCalls++
			return testRuntimePersistenceStoreFactory(projectRoot)
		},
	)
	if err != nil {
		t.Fatalf("PersistenceChoiceForPolicy(default): %v", err)
	}
	store, err := choice.resolve()
	if err != nil {
		t.Fatalf("resolve default policy: %v", err)
	}
	if store != nil {
		t.Fatal("default persistence policy unexpectedly configured a store")
	}
	if storeCalls != 0 {
		t.Fatalf("default policy store factory calls = %d, want 0", storeCalls)
	}
	if _, err := os.Stat(runtimepersist.DirForProjectRoot(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("durable persistence path stat error = %v, want not exist", err)
	}
}

func TestPersistenceChoiceForPolicy_EnabledCreatesProjectDurableSessions(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	choice, err := PersistenceChoiceForPolicy(
		PersistencePolicyEnabled,
		projectRoot,
		testRuntimePersistenceStoreFactory,
	)
	if err != nil {
		t.Fatalf("PersistenceChoiceForPolicy(enabled): %v", err)
	}
	store, err := choice.resolve()
	if err != nil {
		t.Fatalf("resolve enabled policy: %v", err)
	}
	if store == nil {
		t.Fatal("enabled persistence policy returned a nil store")
	}
	sessionID := "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := store.Save(sessionID, []byte(`{"sessionId":"dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), sessionID+".json")); err != nil {
		t.Fatalf("enabled persistence snapshot stat error = %v, want exist", err)
	}
}

func TestPrepareStartAndPersistenceHelpers(t *testing.T) {
	t.Parallel()
	projectRoot := writeSimpleFinalWorkflowProject(t)
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: mustTestRuntimePersistenceStore(t, projectRoot),
	})

	prepared, err := service.prepareStart(StartRequest{
		RequestID:   "req-prepare-start-001",
		ProjectRoot: projectRoot,
		Source: Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   1,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("prepareStart: %v", err)
	}
	if prepared.SourceRef == "" || prepared.SourceContent == "" || prepared.TupleHash == "" {
		t.Fatalf("prepared start = %#v, want resolved source fields", prepared)
	}

	terminal, err := service.executeImmediateSyncSession(context.Background(), prepared.Request, prepared.ResolvedSource, prepared.SourceContent, policyResolutionFromPrepared(prepared), "dur-sess-prepare-001")
	if err != nil {
		t.Fatalf("executeImmediateSyncSession: %v", err)
	}
	sessionID, err := NewDurableSessionID(testSessionIDGenerator)
	if err != nil {
		t.Fatalf("NewDurableSessionID: %v", err)
	}
	terminal.session.SessionID = sessionID
	terminal.result.SessionID = sessionID
	if terminal.session.Status != LifecycleStatusSucceeded {
		t.Fatalf("terminal session status = %q, want SUCCEEDED", terminal.session.Status)
	}

	if err := service.persistTerminalSessionState(terminal); err != nil {
		t.Fatalf("persistTerminalSessionState: %v", err)
	}
	loaded, err := service.snapshotSessionState(sessionID)
	if err != nil {
		t.Fatalf("snapshotSessionState(load persisted): %v", err)
	}
	if loaded.session.SessionID != sessionID {
		t.Fatalf("loaded sessionID = %q, want %q", loaded.session.SessionID, sessionID)
	}
	if loaded.result.ResultStatus != ResultStatusFinal {
		t.Fatalf("loaded result status = %q, want FINAL", loaded.result.ResultStatus)
	}
}

func TestJavaScriptRuntimeService_ProjectRootAloneDoesNotEnablePersistence(t *testing.T) {
	t.Parallel()
	projectRoot := writeSimpleFinalWorkflowProject(t)
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: projectRoot})

	if _, err := service.StartSync(context.Background(), StartRequest{
		RequestID:   "req-runtime-no-implicit-persistence-001",
		ProjectRoot: projectRoot,
		Source: Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "simple-final",
		},
	}); err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	if _, err := os.Stat(runtimepersist.DirForProjectRoot(projectRoot)); !os.IsNotExist(err) {
		t.Fatalf("durable persistence path stat error = %v, want not exist", err)
	}
}

func TestJavaScriptRuntimeService_HasDurableStateReadsFreshOwnerAndRejectsCorruption(t *testing.T) {
	t.Parallel()
	const sessionID = "~default"
	projectRoot := t.TempDir()
	store := mustTestRuntimePersistenceStore(t, projectRoot)
	firstOwner := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: store,
	})
	if err := firstOwner.RecordPetriTokenMutations(sessionID, []interfaces.TokenMutationRecord{{
		Type:         interfaces.MutationCreate,
		TransitionID: "submit",
		TokenID:      "token-durable-state-probe",
	}}); err != nil {
		t.Fatalf("RecordPetriTokenMutations: %v", err)
	}

	// A new execution owner has no in-memory session map. Its answer must come
	// from the same injected store used by the prior owner.
	freshOwner := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: store,
	})
	hasDurableState, err := freshOwner.HasDurableState(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("HasDurableState after fresh owner: %v", err)
	}
	if !hasDurableState {
		t.Fatal("HasDurableState after fresh owner = false, want true")
	}

	if err := store.Save(sessionID, []byte("{")); err != nil {
		t.Fatalf("save corrupt snapshot: %v", err)
	}
	_, err = freshOwner.HasDurableState(context.Background(), sessionID)
	var resumeErr *ResumeError
	if !errors.As(err, &resumeErr) || resumeErr.Outcome != ResumeOutcomeCorruptedPersistence {
		t.Fatalf("HasDurableState corrupt snapshot error = %v, want %q ResumeError", err, ResumeOutcomeCorruptedPersistence)
	}
}

func TestPersistAndMetadataNoOpBranches(t *testing.T) {
	t.Parallel()
	if err := (&JavaScriptRuntimeService{}).persistTerminalSessionState(runtimeSessionState{}); err != nil {
		t.Fatalf("persistTerminalSessionState(no dir) = %v, want nil", err)
	}

	projectRoot := t.TempDir()
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: mustTestRuntimePersistenceStore(t, projectRoot),
	})

	if err := service.persistTerminalSessionState(runtimeSessionState{
		session: SessionReadResult{SessionID: "dur-sess-eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Status: LifecycleStatusRunning},
	}); err != nil {
		t.Fatalf("persistTerminalSessionState(non-terminal) = %v, want nil", err)
	}
	if err := service.persistTerminalSessionState(runtimeSessionState{
		session: SessionReadResult{Status: LifecycleStatusSucceeded},
	}); err != nil {
		t.Fatalf("persistTerminalSessionState(empty session id) = %v, want nil", err)
	}

	metadata := workflowMetadataFromResolved(ResolvedSource{SourceRef: "fallback-ref"}, StartRequest{})
	if metadata["name"] != "fallback-ref" {
		t.Fatalf("fallback metadata name = %#v, want fallback-ref", metadata["name"])
	}
}

func TestApplyInlineFactoryDeclarationPreservesWorkflowFileDefaultPolicy(t *testing.T) {
	t.Parallel()

	defaultPolicy := json.RawMessage(`{"allowedModels":["gpt-allowed"],"mode":"READ_ONLY"}`)
	resolution := factory.WorkflowSourceResolution{Found: true}
	applyInlineFactoryDeclaration(&resolution, Source{
		Kind:         factory.WorkflowSourceKindWorkflowFile,
		WorkflowFile: "/tmp/workflow.js",
		InlineWorkflow: &InlineWorkflowSource{
			DefaultPolicy: defaultPolicy,
		},
	})
	if string(resolution.DefaultPolicy) != string(defaultPolicy) {
		t.Fatalf("resolution defaultPolicy = %s, want %s", resolution.DefaultPolicy, defaultPolicy)
	}
}

const workersImportRoot = "github.com/portpowered/infinite-you/pkg/services/workers"

var executionWorkersLeaseImportRoots = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/...",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/livechild/...",
}

// TestExecutionPackagesImportWorkersOnlyThroughRoot seals execution and
// durable-provider binding call sites to the Workers service root contract.

// TestExecutionServiceRolesNameWorkersRootContracts proves durable execution
// constructors and live-child binding factories type Workers-facing inputs only
// through the Workers service root.
func TestExecutionServiceRolesNameWorkersRootContracts(t *testing.T) {
	t.Parallel()

	var (
		_ workers.InvocationExecutor
		_ providers.Service
		_ workers.ProgressPublisher
	)
}

func TestSmokeLiveChildProviderUsesWorkersRootInferenceContracts(t *testing.T) {
	t.Parallel()

	provider := SmokeLiveChildProvider()
	resp, err := provider.Infer(context.Background(), workers.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-boundary",
			WorkerType: "agent-run-fake-child",
		},
		UserMessage:   "summarize workflows",
		ModelProvider: "mock",
		Model:         "gpt-test",
		SessionID:     "session-boundary",
		RunnerID:      "runner-boundary",
		WorkerType:    "agent-run-fake-child",
	})
	if err != nil {
		t.Fatalf("Infer() error = %v, want nil", err)
	}
	if !strings.Contains(resp.Content, "live:agent-run-fake-child") {
		t.Fatalf("content = %q, want live child smoke payload", resp.Content)
	}
	providerSession := (resp.Continuation).SessionMetadata()
	if providerSession == nil || providerSession.ID != "live-provider-session-1" {
		t.Fatalf("provider session = %#v, want live-provider-session-1", providerSession)
	}
}

func TestDurableLiveChangeScopesIsolatePeersAndOwnedRelease(t *testing.T) {
	t.Parallel()
	const first = "dur-sess-capacity-first"
	const peer = "dur-sess-capacity-peer"
	service := &JavaScriptRuntimeService{
		clock:                 runtimeTestClock{now: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)},
		liveChangeCoordinator: livechange.NewCoordinator(),
		sessions: map[string]*runtimeSessionState{
			first: {session: SessionReadResult{SessionID: first, Status: LifecycleStatusRunning}},
			peer:  {session: SessionReadResult{SessionID: peer, Status: LifecycleStatusRunning}},
		},
	}
	firstRuntime := newDurableLiveChangeAdmissionTestRuntime(t)
	peerRuntime := newDurableLiveChangeAdmissionTestRuntime(t)
	replacement := newDurableLiveChangeAdmissionTestRuntime(t)
	revisions := map[string]int{}
	register := func(id string, runtime *durableLiveChangeAdmissionTestRuntime, name string) func() {
		return service.BindLiveChangeScope(id, runtimebinding.NewLiveChangeApplication(runtime),
			runtimebinding.NewLiveChangeAdmission(runtime), true, func(revision int) { revisions[name] = revision })
	}
	releaseFirst := register(first, firstRuntime, "first")
	releasePeer := register(peer, peerRuntime, "peer")
	t.Cleanup(releaseFirst)
	t.Cleanup(releasePeer)
	apply := func(id, requestID string, revision int) {
		t.Helper()
		result, err := service.ApplyLiveChange(context.Background(), id, factorysessions.LiveChangeRequest{
			RequestID: requestID, ExpectedRevision: revision, Operation: "resource.capacity.set",
			TargetID: "reviewers", RequestedValue: json.RawMessage("1"), Source: "test",
		})
		if err != nil || result.Outcome != factorysessions.LiveChangeOutcomeApplied {
			t.Fatalf("apply %s = %#v, %v", id, result, err)
		}
	}
	apply(first, "first-change", 0)
	if firstRuntime.setCalls != 1 || peerRuntime.setCalls != 0 || revisions["first"] != 1 {
		t.Fatalf("first change crossed peer route: first=%d peer=%d revisions=%v", firstRuntime.setCalls, peerRuntime.setCalls, revisions)
	}
	releaseReplacement := register(first, replacement, "replacement")
	t.Cleanup(releaseReplacement)
	releaseFirst()
	releaseFirst()
	apply(first, "replacement-change", 1)
	apply(peer, "peer-change", 0)
	if firstRuntime.setCalls != 1 || replacement.setCalls != 1 || peerRuntime.setCalls != 1 || revisions["replacement"] != 2 || revisions["peer"] != 1 {
		t.Fatalf("replacement routes: first=%d replacement=%d peer=%d revisions=%v", firstRuntime.setCalls, replacement.setCalls, peerRuntime.setCalls, revisions)
	}
	releaseReplacement()
	_, err := service.ApplyLiveChange(context.Background(), first, factorysessions.LiveChangeRequest{})
	var unavailable *factorysessions.LiveChangeError
	if !errors.As(err, &unavailable) || unavailable.Code != factorysessions.LiveChangeErrorApplicationUnavailable {
		t.Fatalf("released scope error = %v, want application unavailable", err)
	}
	apply(peer, "retained-peer-change", 1)
	if peerRuntime.setCalls != 2 {
		t.Fatalf("released first scope affected peer: %d", peerRuntime.setCalls)
	}
}

func TestDurableScopeRoutesSelectedPolicyClockAndPreActivationReads(t *testing.T) {
	t.Parallel()
	stores := map[string]*runtimeRecordingStore{"/first": {}, "/second": {}}
	router := NewScopePersistence(func(root string) (runtimepersist.Store, error) {
		store := stores[root]
		if store == nil {
			t.Fatalf("unexpected root %q", root)
		}
		return store, nil
	})
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{Persistence: router})
	baseTime := time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)
	for index, policy := range []PersistencePolicy{PersistencePolicyEnabled, PersistencePolicyDisabled, ""} {
		id := []string{"first", "second", "default"}[index]
		root := []string{"/first", "/second", "/second"}[index]
		selectedTime := baseTime.Add(time.Duration(index) * time.Hour)
		settings := factory.JavaScriptWorkerSettings{Presets: map[string]factory.JavaScriptWorkerPreset{"review": {Model: id}}}
		release, err := service.Acquire(t.Context(), durableexecution.ScopeFacts{
			FactorySessionID: id, RuntimeID: "runtime-" + id, ProjectRoot: root,
			Persistence: policy, ChildExecutorMode: ChildExecutorModeFake,
			WorkerPresetIDs: map[string]struct{}{"review": {}}, WorkerSettings: settings,
		}, durableFixedClock{now: selectedTime}, zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := release(context.Background()); err != nil {
				t.Error(err)
			}
		})
		settings.Presets["review"] = factory.JavaScriptWorkerPreset{Model: "mutated"}
		if service.settingsForSession(id).Presets["review"].Model != id {
			t.Fatal("scope settings alias caller")
		}
		if err := service.RecordPetriSessionCompletion(id, PetriSessionCompletion{Status: LifecycleStatusSucceeded}); err != nil {
			t.Fatal(err)
		}
		read, err := service.GetSession(t.Context(), id)
		if err != nil || !read.Lifecycle.FinishedAt.Equal(selectedTime) {
			t.Fatalf("selected clock read = %#v, %v", read, err)
		}
	}
	// Remove only the cache so the unmodified startup probe must cross Load.
	service.mu.Lock()
	service.sessions = make(map[string]*runtimeSessionState)
	service.mu.Unlock()
	for _, tc := range []struct {
		id   string
		want bool
	}{{"first", true}, {"second", false}, {"default", false}} {
		available, err := service.HasDurableState(t.Context(), tc.id)
		if err != nil || available != tc.want {
			t.Fatalf("probe %s = %t, %v", tc.id, available, err)
		}
	}
	if stores["/first"].saveCalls != 1 || stores["/second"].saveCalls != 0 {
		t.Fatal("selected or disabled persistence changed")
	}
}

func TestDurableScopeFailedAndCancelledAcquisitionCanRetryWithoutRetiringPeer(t *testing.T) {
	t.Parallel()
	failure := errors.New("store acquisition failed")
	ctx, cancel := context.WithCancel(t.Context())
	attempts := 0
	router := NewScopePersistence(func(string) (runtimepersist.Store, error) {
		attempts++
		if attempts == 1 {
			return nil, failure
		}
		if attempts == 2 {
			cancel()
		}
		return &runtimeRecordingStore{}, nil
	})
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{Persistence: router})
	facts := durableexecution.ScopeFacts{FactorySessionID: "candidate", ProjectRoot: "/candidate", Persistence: PersistencePolicyEnabled, ChildExecutorMode: ChildExecutorModeFake}
	peerFacts := facts
	peerFacts.FactorySessionID, peerFacts.Persistence = "peer", PersistencePolicyDisabled
	peerRelease, err := service.Acquire(t.Context(), peerFacts, durableFixedClock{}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer peerRelease(context.Background())
	for _, attempt := range []struct {
		ctx   context.Context
		cause error
	}{{t.Context(), nil}, {ctx, context.Canceled}} {
		release, err := service.Acquire(attempt.ctx, facts, durableFixedClock{}, zap.NewNop())
		if release == nil || err == nil || (attempt.cause != nil && !errors.Is(err, attempt.cause)) {
			t.Fatalf("failed acquisition = %v", err)
		}
		if err := release(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := service.RecordPetriSessionCompletion("peer", PetriSessionCompletion{Status: LifecycleStatusSucceeded}); err != nil {
			t.Fatalf("peer retired: %v", err)
		}
	}
	oldRelease, err := service.Acquire(t.Context(), facts, durableFixedClock{}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := oldRelease(t.Context()); err != nil {
		t.Fatal(err)
	}
	replacementRelease, err := service.Acquire(t.Context(), facts, durableFixedClock{}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer replacementRelease(context.Background())
	if err := oldRelease(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordPetriSessionCompletion("candidate", PetriSessionCompletion{Status: LifecycleStatusSucceeded}); err != nil {
		t.Fatalf("stale release retired replacement: %v", err)
	}
	if err := service.ensureOpen(); err != nil {
		t.Fatalf("scoped release closed shared owner: %v", err)
	}
}

func TestDurableScopeReleaseKeepsTerminalPersistenceUntilRunJoins(t *testing.T) {
	t.Parallel()
	store := &runtimeRecordingStore{}
	router := NewScopePersistence(func(string) (runtimepersist.Store, error) { return store, nil })
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{Persistence: router})
	release, err := service.Acquire(t.Context(), durableexecution.ScopeFacts{FactorySessionID: "owned", ProjectRoot: "/owned", Persistence: PersistencePolicyEnabled, ChildExecutorMode: ChildExecutorModeFake}, durableFixedClock{}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancelRun := context.WithCancel(t.Context())
	done := make(chan struct{})
	service.sessions["owned"] = &runtimeSessionState{session: SessionReadResult{SessionID: "owned"}, runCancel: cancelRun, runDone: done}
	cancelled, cancelRelease := context.WithCancel(t.Context())
	cancelRelease()
	if err := release(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("release = %v", err)
	}
	if runCtx.Err() == nil {
		t.Fatal("owned run was not cancelled")
	}
	if _, err := service.beginScopeRunAdmission("owned"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("retiring scope accepted run: %v", err)
	}
	snapshot := []byte(`{"session":{"sessionId":"owned"}}`)
	if err := router.Save("owned", snapshot); err != nil {
		t.Fatalf("terminal persistence lost: %v", err)
	}
	close(done)
	if err := release(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := release(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetSession(t.Context(), "owned"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("retired state = %v", err)
	}
	if !bytes.Equal(store.payload, snapshot) || store.saveCalls != 1 {
		t.Fatal("terminal snapshot lost or duplicated")
	}
}
