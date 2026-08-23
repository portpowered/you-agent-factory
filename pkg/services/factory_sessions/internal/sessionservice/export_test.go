package service

import (
	"context"
	"errors"
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

func TestWorkReadKeepsRuntimeGenerationDuringConcurrentReplacement(t *testing.T) {
	t.Parallel()

	oldLedger := &admissionProjectionLedger{}
	oldLedger.AppendRecordedEvent(admissionProjectionEvent(t, "old-admission", "session-1", 1,
		work.WorkRequestEventWork{Name: "old", WorkID: "old-work"},
	))
	oldRuntime := &generationWorkRuntime{
		snapshot: snapshotWithWork("old-work"),
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
	newRuntime := &generationWorkRuntime{snapshot: snapshotWithWork("new-work")}
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

func snapshotWithWork(workID string) *legacysnapshot.Snapshot {
	tokenID := "token-" + workID
	return &legacysnapshot.Snapshot{
		Marking: factory.PetriMarkingSnapshot{Tokens: map[string]*factory.RuntimeToken{
			tokenID: {
				ID: tokenID, PlaceID: "task:todo",
				Color: factory.RuntimeTokenColor{
					DataType: factory.RuntimeTokenDataTypeWork, WorkID: workID,
					WorkTypeID: "task", Name: workID,
				},
			},
		}},
	}
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
