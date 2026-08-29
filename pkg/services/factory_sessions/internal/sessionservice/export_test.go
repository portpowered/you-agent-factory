package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

func TestSelectCompletionSessionIdentityUsesRetainedMetricIdentity(t *testing.T) {
	const canonicalID = "canonical-runtime-id"

	identity := selectCompletionSessionIdentity(
		factorysessions.DefaultSessionID,
		factory.SessionBuildSpec{
			SessionID:        factorysessions.DefaultSessionID,
			MetricsSessionID: canonicalID,
		},
	)

	if identity.id != factorysessions.DefaultSessionID {
		t.Fatalf("completion identity id = %q, want %q", identity.id, factorysessions.DefaultSessionID)
	}
	if !identity.isDefault {
		t.Fatal("completion identity isDefault = false, want true")
	}
	if identity.runtimeID != canonicalID {
		t.Fatalf("completion identity runtime ID = %q, want %q", identity.runtimeID, canonicalID)
	}
}

func TestCompletionEventScopeIDUsesResumeSourceIdentity(t *testing.T) {
	t.Parallel()

	const (
		successorID = "successor-runtime-id"
		sourceID    = "source-runtime-id"
	)
	identity := selectCompletionSessionIdentity(
		factorysessions.DefaultSessionID,
		factory.SessionBuildSpec{
			SessionID:                      factorysessions.DefaultSessionID,
			MetricsSessionID:               successorID,
			ResumeSourceCanonicalSessionID: sourceID,
		},
	)
	if identity.runtimeID != successorID {
		t.Fatalf("completion metrics identity = %q, want %q", identity.runtimeID, successorID)
	}
	if got := completionEventScopeID(identity.id, factory.SessionBuildSpec{
		ResumeSourceCanonicalSessionID: sourceID,
	}); got != sourceID {
		t.Fatalf("completion event scope = %q, want %q", got, sourceID)
	}
}

func TestCompletionEventScopeIDFallsBackToPublicSelector(t *testing.T) {
	t.Parallel()

	if got := completionEventScopeID(
		factorysessions.DefaultSessionID,
		factory.SessionBuildSpec{MetricsSessionID: "current-runtime-id"},
	); got != factorysessions.DefaultSessionID {
		t.Fatalf("completion event scope = %q, want public selector %q", got, factorysessions.DefaultSessionID)
	}
}

func TestRetainedRuntimeMetricsSessionIDsDeduplicatesSuccessorAndSource(t *testing.T) {
	const canonicalID = "canonical-runtime-id"

	got := retainedRuntimeMetricsSessionIDs(canonicalID, canonicalID)
	if len(got) != 1 || got[0] != canonicalID {
		t.Fatalf("retained metrics IDs = %#v, want one canonical identity", got)
	}

	got = retainedRuntimeMetricsSessionIDs(canonicalID, "source-runtime-id")
	want := []string{canonicalID, "source-runtime-id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("retained metrics IDs = %#v, want %#v", got, want)
	}
}

// NewDefinitionActivationGatewayForTest publishes the activation gateway backed by
// the supplied session state for unit tests.
func NewDefinitionActivationGatewayForTest(state *sessionruntime.Service) factorysessions.DefinitionActivationGateway {
	return NewDefinitionActivationGateway(&SessionRuntime{sessionState: state})
}

func TestWorkAdmissionProjectionWaitsForLedgerCatchUp(t *testing.T) {
	t.Parallel()

	ledger := &admissionProjectionLedger{
		recorderStarted: make(chan struct{}),
		allowReplay:     make(chan struct{}),
	}
	ledger.AppendRecordedEvent(admissionProjectionEvent(t, "event-1", "session-1", 1,
		work.WorkRequestEventWork{Name: "first", WorkID: "work-1"},
	))
	projection := newWorkAdmissionProjection("session-1", platformclock.Real{})
	bindDone := make(chan struct{})
	go func() {
		projection.Bind(ledger)
		close(bindDone)
	}()
	<-ledger.recorderStarted

	projection.mu.RLock()
	binding := projection.binding
	projection.mu.RUnlock()
	if binding == nil {
		t.Fatal("projection binding was not published before ledger replay")
	}
	select {
	case <-binding.ready:
		t.Fatal("projection became ready before ledger replay completed")
	default:
	}

	close(ledger.allowReplay)
	<-bindDone
	assertAdmissions(t, projection.Snapshot(), []work.WorkAdmission{{WorkID: "work-1", Name: "first", Order: 0}})
}

func TestWorkRuntimeSnapshotPrefersFastPublishedBoundary(t *testing.T) {
	t.Parallel()

	runtime := &fastWorkSnapshotRuntime{fastSnapshot: &legacysnapshot.Snapshot{}}
	snapshot, err := workRuntimeSnapshot(context.Background(), runtime)
	if err != nil {
		t.Fatalf("workRuntimeSnapshot() error = %v, want nil", err)
	}
	if snapshot != runtime.fastSnapshot {
		t.Fatalf("workRuntimeSnapshot() = %p, want fast snapshot %p", snapshot, runtime.fastSnapshot)
	}
	if !runtime.fastCalled {
		t.Fatal("fast Work snapshot provider was not called")
	}
	if runtime.engineCalled {
		t.Fatal("aggregate engine snapshot provider was called")
	}
}

func TestResolveWorkRuntimeReleasesProjectionAfterSessionDisappears(t *testing.T) {
	t.Parallel()

	assembly := &Assembly{
		workAdmissions: map[string][]*workAdmissionProjection{
			"closed": {newWorkAdmissionProjection("closed", platformclock.Real{})},
		},
	}
	if _, err := assembly.ResolveWorkRuntime("closed"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("ResolveWorkRuntime() error = %v, want ErrSessionNotFound", err)
	}
	assembly.workAdmissionsMu.Lock()
	_, retained := assembly.workAdmissions["closed"]
	assembly.workAdmissionsMu.Unlock()
	if retained {
		t.Fatal("closed session admission projection was retained")
	}
}

func TestResolveWorkRuntimeDiscardsProjectionFromReplacedGeneration(t *testing.T) {
	t.Parallel()

	oldLedger := &admissionProjectionLedger{}
	oldLedger.AppendRecordedEvent(admissionProjectionEvent(
		t, "old-admission", "session-1", 1,
		work.WorkRequestEventWork{Name: "old", WorkID: "old-work"},
	))
	oldRuntime := &registeredWorkRuntime{}
	oldRecord := &generationRuntimeRecord{service: oldRuntime, ledger: oldLedger}
	state := newWorkResolverSessionState()
	registerGenerationSession(state, "session-1", oldRecord, oldRuntime)
	oldLiveRuntime := state.Resolve("session-1").Runtime

	allowProjectionRegistration := make(chan struct{})
	capturedOldGeneration := make(chan struct{})
	var captureOnce sync.Once
	assembly := &Assembly{
		state:          state,
		workAdmissions: make(map[string][]*workAdmissionProjection),
		beforeWorkAdmissionProjectionRegistration: func() {
			captureOnce.Do(func() {
				close(capturedOldGeneration)
				<-allowProjectionRegistration
			})
		},
	}

	type resolution struct {
		runtime work.Runtime
		err     error
	}
	resolved := make(chan resolution, 1)
	go func() {
		runtime, err := assembly.ResolveWorkRuntime("session-1")
		resolved <- resolution{runtime: runtime, err: err}
	}()
	<-capturedOldGeneration

	newLedger := &admissionProjectionLedger{}
	newLedger.AppendRecordedEvent(admissionProjectionEvent(
		t, "new-admission", "session-1", 1,
		work.WorkRequestEventWork{Name: "new", WorkID: "new-work"},
	))
	newRuntime := &registeredWorkRuntime{}
	newRecord := &generationRuntimeRecord{service: newRuntime, ledger: newLedger}
	registerGenerationSession(state, "session-1", newRecord, newRuntime)
	assembly.retireWorkAdmissionProjection("session-1", oldLiveRuntime, oldRecord)
	close(allowProjectionRegistration)

	result := <-resolved
	if result.err != nil {
		t.Fatalf("ResolveWorkRuntime() error = %v", result.err)
	}
	adapter, ok := result.runtime.(workRuntimeAdapter)
	if !ok || adapter.runtime != newRuntime {
		t.Fatalf("resolved Work runtime = %#v, want replacement runtime %p", result.runtime, newRuntime)
	}

	assembly.workAdmissionsMu.Lock()
	retained := append([]*workAdmissionProjection(nil), assembly.workAdmissions["session-1"]...)
	assembly.workAdmissionsMu.Unlock()
	if len(retained) != 1 || !retained[0].matchesGeneration(state.Resolve("session-1").Runtime, newLedger) {
		t.Fatalf("retained Work projections = %#v, want only replacement generation", retained)
	}
	beforeStale := retained[0].Snapshot()
	oldLedger.AppendRecordedEvent(admissionProjectionEvent(
		t, "stale-old-admission", "session-1", 2,
		work.WorkRequestEventWork{Name: "stale", WorkID: "stale-work"},
	))
	assertAdmissions(t, retained[0].Snapshot(), beforeStale)
}

func TestWorkReadKeepsRuntimeGenerationDuringConcurrentReplacement(t *testing.T) {
	t.Parallel()

	oldLedger := &admissionProjectionLedger{}
	oldLedger.AppendRecordedEvent(admissionProjectionEvent(t, "old-admission", "session-1", 1,
		work.WorkRequestEventWork{Name: "old", WorkID: "old-work"},
	))
	oldRuntime := &generationWorkRuntime{
		snapshot: snapshotWithWork(t, "old-work"),
		entered:  make(chan struct{}),
		allow:    make(chan struct{}),
	}
	oldRecord := &generationRuntimeRecord{service: oldRuntime, ledger: oldLedger}
	state := newWorkResolverSessionState()
	registerGenerationSession(state, "session-1", oldRecord, oldRuntime)
	assembly := &Assembly{state: state, workAdmissions: make(map[string][]*workAdmissionProjection)}

	oldWorkRuntime, err := assembly.ResolveWorkRuntime("session-1")
	if err != nil {
		t.Fatalf("resolve old Work runtime: %v", err)
	}
	oldResult := make(chan work.ReadSnapshot, 1)
	oldErrors := make(chan error, 1)
	go func() {
		snapshot, readErr := oldWorkRuntime.ReadWorkSnapshot(context.Background())
		oldResult <- snapshot
		oldErrors <- readErr
	}()
	<-oldRuntime.entered

	newLedger := &admissionProjectionLedger{}
	newLedger.AppendRecordedEvent(admissionProjectionEvent(t, "new-admission", "session-1", 1,
		work.WorkRequestEventWork{Name: "new", WorkID: "new-work"},
	))
	newRuntime := &generationWorkRuntime{snapshot: snapshotWithWork(t, "new-work")}
	newRecord := &generationRuntimeRecord{service: newRuntime, ledger: newLedger}
	registerGenerationSession(state, "session-1", newRecord, newRuntime)
	newWorkRuntime, err := assembly.ResolveWorkRuntime("session-1")
	if err != nil {
		t.Fatalf("resolve replacement Work runtime: %v", err)
	}
	newSnapshot, err := newWorkRuntime.ReadWorkSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read replacement Work snapshot: %v", err)
	}
	assertGenerationWorkSnapshot(t, newSnapshot, "new-work", "new")

	close(oldRuntime.allow)
	oldSnapshot := <-oldResult
	if err := <-oldErrors; err != nil {
		t.Fatalf("read old Work snapshot: %v", err)
	}
	assertGenerationWorkSnapshot(t, oldSnapshot, "old-work", "old")
}

func TestRetireWorkAdmissionProjectionAfterRepeatedReplacement(t *testing.T) {
	t.Parallel()

	type generation struct {
		runtime    *factorysessions.LiveRuntime
		record     factory.RuntimeRecord
		ledger     *admissionProjectionLedger
		projection *workAdmissionProjection
	}
	assembly := &Assembly{workAdmissions: make(map[string][]*workAdmissionProjection)}
	var current generation
	var retired []generation

	for index := 0; index < 3; index++ {
		runtime := &factorysessions.LiveRuntime{}
		ledger := &admissionProjectionLedger{}
		projection := newWorkAdmissionProjectionForGeneration(
			"session-1", runtime, ledger, platformclock.Real{},
		)
		projection.Bind(ledger)
		ledger.AppendRecordedEvent(admissionProjectionEvent(
			t, "generation-"+strconv.Itoa(index), "session-1", index+1,
			work.WorkRequestEventWork{Name: "generation", WorkID: "work-" + strconv.Itoa(index)},
		))
		replacement := generation{
			runtime: runtime, record: &generationRuntimeRecord{ledger: ledger},
			ledger: ledger, projection: projection,
		}
		assembly.workAdmissionsMu.Lock()
		assembly.workAdmissions["session-1"] = append(
			assembly.workAdmissions["session-1"], projection,
		)
		assembly.workAdmissionsMu.Unlock()

		if current.projection != nil {
			assembly.retireWorkAdmissionProjection("session-1", current.runtime, current.record)
			retired = append(retired, current)
		}
		current = replacement
	}

	assembly.workAdmissionsMu.Lock()
	retained := assembly.workAdmissions["session-1"]
	assembly.workAdmissionsMu.Unlock()
	if len(retained) != 1 || retained[0] != current.projection {
		t.Fatalf("retained projections = %#v, want only current generation", retained)
	}

	for index, old := range retired {
		before := old.projection.Snapshot()
		old.ledger.AppendRecordedEvent(admissionProjectionEvent(
			t, "stale-"+strconv.Itoa(index), "session-1", index+10,
			work.WorkRequestEventWork{Name: "stale", WorkID: "stale-" + strconv.Itoa(index)},
		))
		assertAdmissions(t, old.projection.Snapshot(), before)

		old.projection.mu.RLock()
		closed := old.projection.closed
		binding := old.projection.binding
		generationRuntime := old.projection.generationRuntime
		generationLedger := old.projection.generationLedger
		old.projection.mu.RUnlock()
		if !closed || binding != nil || generationRuntime != nil || generationLedger != nil {
			t.Fatalf("retired projection %d retained generation state: closed=%v binding=%p runtime=%p ledger=%p", index, closed, binding, generationRuntime, generationLedger)
		}
	}
}

func registerGenerationSession(
	state *sessionruntime.Service,
	sessionID string,
	record factory.RuntimeRecord,
	runtime factory.Service,
) {
	state.Register(sessionruntime.Registration{
		SessionID: sessionID,
		Handle:    &runtimebinding.SessionState{Instance: record},
		Runtime:   &factorysessions.LiveRuntime{Factory: runtime, Clock: platformclock.Real{}},
		Select:    true,
	})
}

func snapshotWithWork(t testing.TB, workID string) *legacysnapshot.Snapshot {
	t.Helper()
	tokenID := "token-" + workID
	payload := map[string]any{
		"marking": map[string]any{
			"tokens": map[string]any{
				tokenID: map[string]any{
					"id": tokenID, "place_id": "task:todo",
					"color": map[string]string{
						"data_type": "work", "work_id": workID,
						"work_type_id": "task", "name": workID,
					},
				},
			},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal opaque Work snapshot fixture: %v", err)
	}
	var snapshot legacysnapshot.Snapshot
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		t.Fatalf("unmarshal opaque Work snapshot fixture: %v", err)
	}
	return &snapshot
}

func assertGenerationWorkSnapshot(t testing.TB, snapshot work.ReadSnapshot, wantWorkID, wantName string) {
	t.Helper()
	if len(snapshot.Items) != 1 || snapshot.Items[0].WorkID != wantWorkID {
		t.Fatalf("Work items = %#v, want %q", snapshot.Items, wantWorkID)
	}
	if len(snapshot.Admissions) != 1 || snapshot.Admissions[0].WorkID != wantWorkID || snapshot.Admissions[0].Name != wantName {
		t.Fatalf("Work admissions = %#v, want (%q, %q)", snapshot.Admissions, wantWorkID, wantName)
	}
}

type generationWorkRuntime struct {
	factory.Service
	snapshot  *legacysnapshot.Snapshot
	entered   chan struct{}
	allow     chan struct{}
	enterOnce sync.Once
}

func (runtime *generationWorkRuntime) GetWorkStateSnapshot(ctx context.Context) (*legacysnapshot.Snapshot, error) {
	if runtime.entered != nil {
		runtime.enterOnce.Do(func() { close(runtime.entered) })
		select {
		case <-runtime.allow:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return runtime.snapshot, nil
}

func (runtime *generationWorkRuntime) GetEngineStateSnapshot(ctx context.Context) (*legacysnapshot.Snapshot, error) {
	return runtime.GetWorkStateSnapshot(ctx)
}

type generationRuntimeRecord struct {
	service factory.Service
	ledger  recordings.Ledger
}

func (record *generationRuntimeRecord) RuntimeService() factory.Service { return record.service }
func (*generationRuntimeRecord) Directory() string                      { return "/factory" }
func (*generationRuntimeRecord) FolderDirectory() string                { return "/factory" }
func (*generationRuntimeRecord) BackendScope() string                   { return "generation-test" }
func (*generationRuntimeRecord) StartTime() time.Time                   { return time.Time{} }
func (*generationRuntimeRecord) LoadedRuntimeConfig() factory.LoadedConfig {
	return nil
}
func (record *generationRuntimeRecord) CanonicalEvents() []interfaces.FactoryEvent {
	if record.ledger == nil {
		return nil
	}
	return record.ledger.CanonicalEvents()
}
func (record *generationRuntimeRecord) AddEventTypeRecorder(recorder func(interfaces.FactoryEventType)) {
	if record.ledger != nil {
		record.ledger.AddEventTypeRecorder(recorder)
	}
}
func (record *generationRuntimeRecord) StreamGeneration() string {
	if record.ledger == nil {
		return ""
	}
	return record.ledger.StreamGenerationID()
}
func (*generationRuntimeRecord) RuntimeLogger() *zap.Logger { return zap.NewNop() }
func (*generationRuntimeRecord) RuntimeMetrics() factory.MetricsEmitter {
	return nil
}
func (*generationRuntimeRecord) RuntimeDiagnostics() factory.RuntimeLogDiagnostics {
	return factory.RuntimeLogDiagnostics{}
}
func (record *generationRuntimeRecord) RecordingLedger() recordings.Ledger { return record.ledger }
func (*generationRuntimeRecord) CloseArtifacts() error                     { return nil }

type fastWorkSnapshotRuntime struct {
	factory.Service
	fastSnapshot *legacysnapshot.Snapshot
	fastCalled   bool
	engineCalled bool
}

func (runtime *fastWorkSnapshotRuntime) GetEngineStateSnapshot(context.Context) (*legacysnapshot.Snapshot, error) {
	runtime.engineCalled = true
	return nil, errors.New("aggregate snapshot should not be called")
}

func (runtime *fastWorkSnapshotRuntime) GetWorkStateSnapshot(context.Context) (*legacysnapshot.Snapshot, error) {
	runtime.fastCalled = true
	return runtime.fastSnapshot, nil
}

func TestInvocationOutcomeRelevantEventSelectsStateAndLifecycleTypes(t *testing.T) {
	relevant := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeWorkStateChange,
		interfaces.FactoryEventTypeWorkRequest,
		interfaces.FactoryEventTypeRunResponse,
		interfaces.FactoryEventTypeFactoryStateResponse,
		interfaces.FactoryEventTypeSessionCompleted,
		interfaces.FactoryEventTypeSessionPaused,
		interfaces.FactoryEventTypeSessionResumed,
		interfaces.FactoryEventTypeSessionResultUpdated,
		interfaces.FactoryEventTypeSessionLifecycleControl,
	}
	for _, eventType := range relevant {
		if !invocationOutcomeRelevantEvent(eventType) {
			t.Errorf("invocationOutcomeRelevantEvent(%q) = false, want true", eventType)
		}
	}
	telemetry := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeModelRequest,
		interfaces.FactoryEventTypeModelResponse,
		interfaces.FactoryEventTypeInferenceRequest,
		interfaces.FactoryEventTypeInferenceResponse,
		interfaces.FactoryEventTypeScriptRequest,
		interfaces.FactoryEventTypeScriptResponse,
		interfaces.FactoryEventTypeDispatchRequest,
		interfaces.FactoryEventTypeDispatchQueued,
		interfaces.FactoryEventTypeAgentRunResponse,
	}
	for _, eventType := range telemetry {
		if invocationOutcomeRelevantEvent(eventType) {
			t.Errorf("invocationOutcomeRelevantEvent(%q) = true, want false", eventType)
		}
	}
}

func TestEventDrivenInvocationWaiterWakesOnRelevantEventBeforeFallback(t *testing.T) {
	events := make(chan interfaces.FactoryEvent, 2)
	wake := make(chan struct{}, 1)
	go relayInvocationWakeEvents(events, wake)

	events <- interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeModelResponse}
	events <- interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeWorkStateChange}
	close(events)

	waiter := newEventDrivenInvocationWaiter(wake)
	start := time.Now()
	if err := waiter(context.Background()); err != nil {
		t.Fatalf("waiter: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= invocationWaiterFallbackInterval {
		t.Fatalf("waiter took %v, want an event-driven wake before the %v fallback", elapsed, invocationWaiterFallbackInterval)
	}
}

func TestEventDrivenInvocationWaiterHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	waiter := newEventDrivenInvocationWaiter(make(chan struct{}))
	if err := waiter(ctx); err != context.Canceled {
		t.Fatalf("waiter err = %v, want context.Canceled", err)
	}
}

func TestRelayInvocationWakeEventsCoalescesBurstsWithoutBlocking(t *testing.T) {
	events := make(chan interfaces.FactoryEvent)
	wake := make(chan struct{}, 1)
	relayDone := make(chan struct{})
	go func() {
		relayInvocationWakeEvents(events, wake)
		close(relayDone)
	}()

	for range 8 {
		events <- interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeWorkStateChange}
	}
	close(events)
	<-relayDone

	select {
	case <-wake:
	default:
		t.Fatal("coalesced wake signal is missing after an event burst")
	}
	select {
	case <-wake:
		t.Fatal("wake signal is not coalesced: second token present")
	default:
	}
}
