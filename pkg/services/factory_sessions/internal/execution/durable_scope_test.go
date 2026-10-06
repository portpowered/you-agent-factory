package factorysessionexecution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livechange"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"go.uber.org/zap"
)

func TestDurableLiveChangeScopesIsolatePeersAndOwnedRelease(t *testing.T) {
	t.Parallel()
	const first = "dur-sess-capacity-first"
	const peer = "dur-sess-capacity-peer"
	service := &JavaScriptRuntimeService{
		clock: runtimeTestClock{now: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)},
		durableRuntimeState: &durableRuntimeState{sessions: map[string]*runtimeSessionState{
			first: {session: SessionReadResult{SessionID: first, Status: LifecycleStatusRunning}},
			peer:  {session: SessionReadResult{SessionID: peer, Status: LifecycleStatusRunning}},
		}}, durableRuntimeBehavior: &durableRuntimeBehavior{liveChangeCoordinator: livechange.NewCoordinator()},
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

func TestDurableScopeDefaultOpeningsPreserveProbeIdentityAndPeerState(t *testing.T) {
	t.Parallel()
	peerStore := &runtimeRecordingStore{}
	corruptStore := &runtimeRecordingStore{payload: []byte(`{"session":`)}
	stores := map[string]*runtimeRecordingStore{"/peer": peerStore, "/corrupt": corruptStore}
	owner := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		Persistence: NewScopePersistence(func(root string) (runtimepersist.Store, error) { return stores[root], nil }),
	})
	peerFacts := durableexecution.ScopeFacts{FactorySessionID: "~default", RuntimeID: "peer-runtime", ProjectRoot: "/peer", Persistence: PersistencePolicyEnabled, ChildExecutorMode: ChildExecutorModeFake}
	peer, peerRelease, err := owner.Acquire(t.Context(), peerFacts, durableFixedClock{}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	cleanupDurableScope(t, peerRelease)
	failedFacts := peerFacts
	failedFacts.RuntimeID, failedFacts.ProjectRoot = "corrupt-runtime", "/corrupt"
	candidate, release, err := owner.Acquire(t.Context(), failedFacts, durableFixedClock{}, zap.NewNop())
	if err != nil {
		t.Fatalf("peer blocked selected default opening before its persistence probe: %v", err)
	}
	cleanupDurableScope(t, release)
	_, err = candidate.(*JavaScriptRuntimeService).HasDurableState(t.Context(), "~default")
	var resumeErr *ResumeError
	if !errors.As(err, &resumeErr) || resumeErr.Outcome != ResumeOutcomeCorruptedPersistence || resumeErr.SessionID != "~default" {
		t.Fatalf("selected corrupt probe = %v", err)
	}
	if err := peer.(*JavaScriptRuntimeService).RecordPetriSessionCompletion("~default", PetriSessionCompletion{Status: LifecycleStatusSucceeded}); err != nil {
		t.Fatal(err)
	}
	if err := release(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertDefaultPeerState(t, peer, peerStore)
	// A retired handle cannot remove the replacement's admission or state.
	stores["/corrupt"] = &runtimeRecordingStore{}
	replacement, replacementRelease, err := owner.Acquire(t.Context(), failedFacts, durableFixedClock{}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	cleanupDurableScope(t, replacementRelease)
	if err := release(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := replacement.(*JavaScriptRuntimeService).RecordPetriSessionCompletion("~default", PetriSessionCompletion{Status: LifecycleStatusFailed}); err != nil {
		t.Fatal(err)
	}
	if _, duplicateRelease, err := owner.Acquire(t.Context(), failedFacts, durableFixedClock{}, zap.NewNop()); err == nil || duplicateRelease != nil {
		t.Fatalf("duplicate live scope admitted: %v", err)
	}
	assertDefaultPeerState(t, peer, peerStore)
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := owner.Acquire(t.Context(), peerFacts, durableFixedClock{}, zap.NewNop()); !errors.Is(err, ErrDurableExecutionClosed) {
		t.Fatalf("closed owner admitted scope: %v", err)
	}
}

func assertDefaultPeerState(t *testing.T, peer durableexecution.Service, store *runtimeRecordingStore) {
	t.Helper()
	read, err := peer.GetSession(t.Context(), "~default")
	if err != nil || read.SessionID != "~default" || read.Status != LifecycleStatusSucceeded {
		t.Fatalf("peer read after candidate retirement = %#v, %v", read, err)
	}
	var snapshot PersistedRuntimeSessionState
	if err := json.Unmarshal(store.payload, &snapshot); err != nil || snapshot.Session.SessionID != "~default" || snapshot.Session.Status != LifecycleStatusSucceeded {
		t.Fatalf("peer persisted identity/state changed: %#v, %v", snapshot.Session, err)
	}
}

func TestDurableScopeAcquisitionFailureRetriesAlongsideDefaultPeer(t *testing.T) {
	t.Parallel()
	failure := errors.New("selected store unavailable")
	ctx, cancel := context.WithCancel(t.Context())
	attempts := 0
	owner := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		Persistence: NewScopePersistence(func(string) (runtimepersist.Store, error) {
			attempts++
			if attempts == 1 {
				return nil, failure
			}
			if attempts == 2 {
				cancel()
			}
			return &runtimeRecordingStore{}, nil
		}),
	})
	facts := durableexecution.ScopeFacts{FactorySessionID: "~default", RuntimeID: "candidate", ProjectRoot: "/candidate", Persistence: PersistencePolicyEnabled, ChildExecutorMode: ChildExecutorModeFake}
	peerFacts := facts
	peerFacts.RuntimeID, peerFacts.Persistence = "peer", PersistencePolicyDisabled
	peer, peerRelease, err := owner.Acquire(t.Context(), peerFacts, durableFixedClock{}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	cleanupDurableScope(t, peerRelease)
	for _, attempt := range []struct {
		ctx   context.Context
		cause error
	}{{t.Context(), nil}, {ctx, context.Canceled}} {
		candidate, release, err := owner.Acquire(attempt.ctx, facts, durableFixedClock{}, zap.NewNop())
		if candidate != nil {
			t.Fatal("failed candidate published execution")
		}
		assertFailedDurableAcquisition(t, release, err, attempt.cause)
		if attempt.cause == nil {
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) || validationErr.Field != "persistence" {
				t.Fatalf("store acquisition lost existing validation error: %v", err)
			}
		}
		if err := peer.(*JavaScriptRuntimeService).RecordPetriSessionCompletion("~default", PetriSessionCompletion{Status: LifecycleStatusSucceeded}); err != nil {
			t.Fatalf("failed candidate retired default peer: %v", err)
		}
	}
	candidate, release, err := owner.Acquire(t.Context(), facts, durableFixedClock{}, zap.NewNop())
	if err != nil || candidate == nil {
		t.Fatalf("same scoped identity retry = %v", err)
	}
	cleanupDurableScope(t, release)
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
		release, err := service.acquire(t.Context(), durableexecution.ScopeFacts{
			FactorySessionID: id, RuntimeID: "runtime-" + id, ProjectRoot: root,
			Persistence: policy, ChildExecutorMode: ChildExecutorModeFake,
			WorkerPresetIDs: map[string]struct{}{"review": {}}, WorkerSettings: settings,
		}, durableFixedClock{now: selectedTime}, zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		cleanupDurableScope(t, release)
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
	peerRelease, err := service.acquire(t.Context(), peerFacts, durableFixedClock{}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	cleanupDurableScope(t, peerRelease)
	for _, attempt := range []struct {
		ctx   context.Context
		cause error
	}{{t.Context(), nil}, {ctx, context.Canceled}} {
		release, err := service.acquire(attempt.ctx, facts, durableFixedClock{}, zap.NewNop())
		assertFailedDurableAcquisition(t, release, err, attempt.cause)
		if err := service.RecordPetriSessionCompletion("peer", PetriSessionCompletion{Status: LifecycleStatusSucceeded}); err != nil {
			t.Fatalf("peer retired: %v", err)
		}
	}
	oldRelease, err := service.acquire(t.Context(), facts, durableFixedClock{}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := oldRelease(t.Context()); err != nil {
		t.Fatal(err)
	}
	replacementRelease, err := service.acquire(t.Context(), facts, durableFixedClock{}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	cleanupDurableScope(t, replacementRelease)
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
	release, err := service.acquire(t.Context(), durableexecution.ScopeFacts{FactorySessionID: "owned", ProjectRoot: "/owned", Persistence: PersistencePolicyEnabled, ChildExecutorMode: ChildExecutorModeFake}, durableFixedClock{}, zap.NewNop())
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

// The acquired owner, rather than public Invoke's independent T13 owner,
// applies the opening's persistence policy to child results.
func TestDurableScopeChildResultsReloadOnlyFromEnabledRoot(t *testing.T) {
	t.Parallel()
	roots := []string{"/enabled", "/disabled"}
	ids := []string{"dur-sess-00000000000040008000000000000001", "dur-sess-00000000000040008000000000000002"}
	generated := 0
	stores := map[string]*runtimeRecordingStore{roots[0]: {}, roots[1]: {}}
	router := NewScopePersistence(func(root string) (runtimepersist.Store, error) {
		store := stores[root]
		if store == nil {
			t.Fatalf("unexpected selected root %q", root)
		}
		return store, nil
	})
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		Persistence: router,
		Workflows: scriptedRuntimeWorkflows(func(_ context.Context, _ factory.JavaScriptRuntimeRequest, _ factory.JavaScriptRuntimeHooks) (factory.JavaScriptRuntimeOutcome, error) {
			outcome := successfulRuntimeOutcome([]factory.JavaScriptRuntimeRecord{{
				Kind: factory.JavaScriptRecordKindChildDispatch,
				ChildDispatch: &factory.JavaScriptChildDispatchRecord{
					DispatchID: "selected-child", Status: factory.JavaScriptChildDispatchStatusCompleted,
					ExecutionMode: ChildExecutorModeLive,
				},
			}})
			outcome.Value.JSON = json.RawMessage(`"owned child result"`)
			return outcome, nil
		}),
	})
	service.generateSessionID = func() string {
		id := []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"}[generated]
		generated++
		return id
	}
	for index, policy := range []PersistencePolicy{PersistencePolicyEnabled, PersistencePolicyDisabled} {
		release, err := service.acquire(t.Context(), durableexecution.ScopeFacts{
			FactorySessionID: ids[index], RuntimeID: "runtime-" + ids[index], ProjectRoot: roots[index],
			Persistence: policy, ChildExecutorMode: ChildExecutorModeLive,
		}, durableFixedClock{now: time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)}, zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		cleanupDurableScope(t, release)
		request := inlineWorkflowStartRequest("scope-child-"+ids[index], "selected child", nil, nil)
		request.ProjectRoot = roots[index]
		started, err := service.StartSync(t.Context(), request)
		if err != nil || started.SessionID != ids[index] {
			t.Fatalf("selected start = %#v, %v", started, err)
		}
		if selectedRoot := service.projectRootForSession(ids[index]); selectedRoot != roots[index] {
			t.Fatalf("selected child root = %q, want %q", selectedRoot, roots[index])
		}
		dispatches, err := service.ListDispatches(t.Context(), ids[index])
		if err != nil || len(dispatches.Dispatches) != 1 {
			t.Fatalf("selected child dispatches = %#v, %v", dispatches, err)
		}

		if err := release(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if stores[roots[1]].saveCalls != 0 {
		t.Fatal("disabled child persisted")
	}
	if stores[roots[0]].saveCalls == 0 {
		t.Fatal("enabled child was not persisted")
	}
	assertDurableChildReload(t, service, ids, roots)
}

func assertDurableChildReload(t *testing.T, service *JavaScriptRuntimeService, ids, roots []string) {
	t.Helper()

	release, err := service.acquire(t.Context(), durableexecution.ScopeFacts{
		FactorySessionID: ids[0], ProjectRoot: roots[0], Persistence: PersistencePolicyEnabled, ChildExecutorMode: ChildExecutorModeLive,
	}, durableFixedClock{}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	cleanupDurableScope(t, release)
	dispatches, err := service.ListDispatches(t.Context(), ids[0])
	if err != nil || len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].Status != "COMPLETED" {
		t.Fatalf("reloaded child dispatches = %#v, %v", dispatches, err)
	}
	result, err := service.GetResult(t.Context(), ids[0], ResultRequest{Mode: ResultModeFinal})
	if err != nil || !bytes.Contains(result.PrimaryResult, []byte("owned child result")) {
		t.Fatalf("reloaded child result = %#v, %v", result, err)
	}
	if _, err := service.GetSession(t.Context(), ids[1]); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("disabled peer reloaded = %v, want absent", err)
	}
}

func cleanupDurableScope(t *testing.T, release func(context.Context) error) {
	t.Helper()
	t.Cleanup(func() {
		if err := release(context.Background()); err != nil {
			t.Error(err)
		}
	})
}

func assertFailedDurableAcquisition(t *testing.T, release func(context.Context) error, err, cause error) {
	t.Helper()
	if release == nil || err == nil || (cause != nil && !errors.Is(err, cause)) {
		t.Fatalf("failed acquisition = %v", err)
	}
	if err := release(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentBoardAcquiredScopesIsolateReferenceReadsAndWrites(t *testing.T) {
	t.Parallel()
	firstStore := &currentBoardScopeStore{artifact: "first-recording"}
	peerStore := &currentBoardScopeStore{artifact: "peer-recording"}
	globalStore := &currentBoardScopeStore{artifact: "global-recording"}
	stores := map[string]*currentBoardScopeStore{"/first": firstStore, "/peer": peerStore}
	owner := &JavaScriptRuntimeService{
		persistence: globalStore,
		scopePersistence: NewScopePersistence(func(root string) (runtimepersist.Store, error) {
			return stores[root], nil
		}),
		durableRuntimeState: &durableRuntimeState{}, durableRuntimeBehavior: &durableRuntimeBehavior{},
	}
	acquire := func(root string) (*JavaScriptRuntimeService, func(context.Context) error) {
		t.Helper()
		service, release, err := owner.Acquire(t.Context(), durableexecution.ScopeFacts{
			FactorySessionID: "~default", RuntimeID: root, ProjectRoot: root,
			Persistence: PersistencePolicyEnabled, ChildExecutorMode: ChildExecutorModeFake,
		}, durableFixedClock{}, zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		cleanupDurableScope(t, release)
		return service.(*JavaScriptRuntimeService), release
	}
	first, releaseFirst := acquire("/first")
	peer, _ := acquire("/peer")
	assertScopedBoardReference(t, first, "/first/factory", "first-recording", "first-next")
	assertScopedBoardReference(t, peer, "/peer/factory", "peer-recording", "peer-next")
	assertBoardScopeStore(t, firstStore, "/first/factory", "first-next", 1, 1)
	assertBoardScopeStore(t, peerStore, "/peer/factory", "peer-next", 1, 1)
	assertBoardScopeStore(t, globalStore, "", "global-recording", 0, 0)
	if err := releaseFirst(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := first.LoadCurrentBoard(t.Context(), "/first/factory"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("released scope read = %v", err)
	}
	if err := first.SaveCurrentBoard(t.Context(), "/first/factory", "unowned"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("released scope write = %v", err)
	}
	assertScopedBoardReference(t, peer, "/peer/factory", "peer-next", "peer-retained")
	assertBoardScopeStore(t, firstStore, "/first/factory", "first-next", 1, 1)
	assertBoardScopeStore(t, peerStore, "/peer/factory", "peer-retained", 2, 2)
	assertBoardScopeStore(t, globalStore, "", "global-recording", 0, 0)
}

func assertBoardScopeStore(t *testing.T, store *currentBoardScopeStore, factoryDirectory, artifact string, reads, writes int) {
	t.Helper()
	if store.factory != factoryDirectory || store.artifact != artifact || store.reads != reads || store.writes != writes {
		t.Fatalf("reference operations crossed scoped routes: got %+v; want factory=%q artifact=%q reads=%d writes=%d", store, factoryDirectory, artifact, reads, writes)
	}
}

func assertScopedBoardReference(t *testing.T, service *JavaScriptRuntimeService, factoryDirectory, previous, next string) {
	t.Helper()
	got, err := service.LoadCurrentBoard(t.Context(), factoryDirectory)
	if err != nil || got != previous {
		t.Fatalf("scoped read = %q, %v; want %q", got, err, previous)
	}
	if err := service.SaveCurrentBoard(t.Context(), factoryDirectory, next); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentBoardUnavailableScopeDoesNotUsePeerStore(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"missing default", "retiring default", "disabled persistence", "unsupported store"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			peer := &currentBoardScopeStore{artifact: "peer-recording"}
			router := NewScopePersistence(nil)
			router.scopes["peer"] = &durableScope{store: peer}
			switch name {
			case "retiring default":
				router.scopes["~default"] = &durableScope{store: peer, retiring: true}
			case "disabled persistence":
				router.scopes["~default"] = &durableScope{}
			case "unsupported store":
				router.scopes["~default"] = &durableScope{store: &durableProbeStore{}}
			}
			service := &JavaScriptRuntimeService{persistence: router}
			if got, err := service.LoadCurrentBoard(t.Context(), "/factory"); err == nil || got != "" {
				t.Fatalf("unavailable scoped read = %q, %v", got, err)
			}
			if err := service.SaveCurrentBoard(t.Context(), "/factory", "replacement"); err == nil {
				t.Fatal("unavailable scoped write succeeded")
			}
			if peer.reads != 0 || peer.writes != 0 || peer.artifact != "peer-recording" {
				t.Fatal("unavailable scope touched peer reference")
			}
		})
	}
}

type currentBoardScopeStore struct {
	durableProbeStore
	artifact string
	factory  string
	reads    int
	writes   int
}

func (store *currentBoardScopeStore) LoadCurrentBoard(ctx context.Context, factoryDirectory string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	store.factory = factoryDirectory
	store.reads++
	return store.artifact, nil
}

func (store *currentBoardScopeStore) SaveCurrentBoard(ctx context.Context, factoryDirectory, artifact string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.factory = factoryDirectory
	store.writes++
	store.artifact = artifact
	return nil
}
