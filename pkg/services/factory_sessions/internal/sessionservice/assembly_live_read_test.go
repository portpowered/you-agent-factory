package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	sessionidentity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type workerSessionsObservationMarker struct{ workersessions.Service }

type hostLifecycleStub struct {
	factoryruntime.RuntimeLifecycle
	err     error
	stopped factoryruntime.RuntimeRun
	onStop  func()
}

func (s *hostLifecycleStub) Stop(run factoryruntime.RuntimeRun) error {
	s.stopped = run
	if s.onStop != nil {
		s.onStop()
	}
	return s.err
}

type hostControlMetrics struct {
	factoryruntime.NoopEmitter
	fields []factoryruntime.Fields
}

func (m *hostControlMetrics) Counter(_ context.Context, name string, value float64, fields factoryruntime.Fields) error {
	if name == factoryruntime.RuntimeLifecycleControl && value == 1 {
		m.fields = append(m.fields, fields)
	}
	return nil
}

type hostControlRecord struct {
	generationRuntimeRecord
	metrics factoryruntime.MetricsEmitter
}

func (r *hostControlRecord) RuntimeMetrics() factoryruntime.MetricsEmitter { return r.metrics }

type hostWorkerObservation struct {
	workersessions.Service
	workID string
}

func (s hostWorkerObservation) ListObservations(_ context.Context, _ workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	return workersessions.ListObservationsResult{Observations: []workersessions.Observation{{WorkIDs: []string{s.workID}}}}, nil
}

type hostWorkerRuntime struct {
	factoryruntime.Service
	id        string
	requested string
}

func (s *hostWorkerRuntime) WorkerSessionsObservationForSession(id string) workersessions.ObservationService {
	s.requested = id
	return hostWorkerObservation{workID: s.id}
}

func TestSessionHostLifecyclePreservesFailedStopRetryAndPeers(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	active := &runtimebinding.State{}
	failure := errors.New("injected addressed stop failure")
	lifecycle := &hostLifecycleStub{err: failure}
	var released []string
	control := NewScopeControl(state, func(run factoryruntime.RuntimeRun, _ factoryruntime.Clock) error {
		return lifecycle.Stop(run)
	}, zap.NewNop())
	host := SessionServiceHost(state, active, control,
		func(id string) { released = append(released, id) }, "", nil, nil, nil, nil, nil)
	for _, id := range []string{"first", "peer"} {
		run := invocationQueryRun{record: &generationRuntimeRecord{service: &observeStubRuntime{}}}
		state.Registry().Upsert(&livesession.LiveSession{ID: id, Handle: &runtimebinding.SessionState{Handle: run}}, id == "first")
	}
	selected := state.Resolve("first")
	active.SetActive(context.Background(), selected.ID, runtimebinding.HandleFromSession(selected))
	if err := host.StopLiveSession("first"); !errors.Is(err, failure) {
		t.Fatalf("failed stop = %v", err)
	}
	if !reflect.DeepEqual(state.Registry().IDs(), []string{"first", "peer"}) || len(released) != 0 || active.Active().SessionID != "first" {
		t.Fatal("failed stop retired a session or released admission")
	}
	if lifecycle.stopped != runtimebinding.HandleFromSession(selected) {
		t.Fatal("stop effect targeted a peer run")
	}
	lifecycle.err = nil
	if err := host.StopLiveSession("first"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimebinding.RequireLiveSession(state, "first"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("retired session read = %v", err)
	}
	if _, err := runtimebinding.RequireLiveSession(state, "peer"); err != nil {
		t.Fatalf("peer read after retry = %v", err)
	}
	if !reflect.DeepEqual(released, []string{"first"}) || active.Active().SessionID != "peer" {
		t.Fatalf("retirement effects = %v, active = %#v", released, active.Active())
	}
	if err := host.StopLiveSession("missing"); !errors.Is(err, factorysessions.ErrSessionNotFound) || len(released) != 1 {
		t.Fatalf("missing stop = %v, releases = %v", err, released)
	}
}

func TestSessionHostWorkerReadsPreserveAddressedAndStartupFallback(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	active := &runtimebinding.State{}
	first, peer, startup := &hostWorkerRuntime{id: "first-work"}, &hostWorkerRuntime{id: "peer-work"}, &hostWorkerRuntime{id: "startup-work"}
	for id, runtime := range map[string]*hostWorkerRuntime{"first": first, "peer": peer} {
		state.Registry().Upsert(&livesession.LiveSession{ID: id, Handle: &runtimebinding.SessionState{
			Handle: invocationQueryRun{record: &generationRuntimeRecord{service: runtime}},
		}}, id == "peer")
	}
	active.SetStartup(&generationRuntimeRecord{service: startup})
	reader := sessionLifecycleReader{state: state, active: active}
	for _, tc := range []struct{ selector, work string }{{"first", "first-work"}, {"peer", "peer-work"}, {"missing", "startup-work"}} {
		view := reader.WorkerSessionsObservationForSession(tc.selector)
		got, err := view.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: "requested"})
		if err != nil || len(got.Observations) != 1 || !reflect.DeepEqual(got.Observations[0].WorkIDs, []string{tc.work}) {
			t.Fatalf("%s worker read = %#v, %v", tc.selector, got, err)
		}
	}
	if first.requested != "first" || peer.requested != "peer" || startup.requested != "missing" {
		t.Fatal("worker observation lost the requested public identity")
	}
	// Construct without an opening owner or gateway and retain addressed reads.
	host := SessionServiceHost(state, active, nil, nil, "", nil, nil, nil, nil, nil)
	got, err := host.(interface {
		WorkerSessionsObservationForSession(string) workersessions.ObservationService
	}).WorkerSessionsObservationForSession("peer").ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: "requested"})
	if err != nil || len(got.Observations) != 1 || !reflect.DeepEqual(got.Observations[0].WorkIDs, []string{"peer-work"}) {
		t.Fatalf("captured production host worker read = %#v, %v", got, err)
	}
}

func TestSessionHostLifecycleDiagnosticsPreserveMissingClassification(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zap.InfoLevel)
	reader := sessionLifecycleReader{state: newWorkResolverSessionState(), logger: zap.New(core)}
	reader.ObserveLiveLifecycleControl("missing", factorysessions.LifecycleControlCancel, factorysessions.ControlRequest{}, "", "", fmt.Errorf("private detail: %w", factorysessions.ErrSessionNotFound))
	entries := logs.All()
	if len(entries) != 1 || entries[0].Level != zap.InfoLevel || entries[0].ContextMap()["session_id"] != "missing" || entries[0].ContextMap()["outcome"] != "ERROR" {
		t.Fatalf("missing-session diagnostic = %#v", entries)
	}
}

func TestSessionHostLifecycleDiagnosticsStayOnAddressedRecord(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	first, peer := &hostControlMetrics{}, &hostControlMetrics{}
	for id, metrics := range map[string]*hostControlMetrics{"first": first, "peer": peer} {
		record := &hostControlRecord{generationRuntimeRecord: generationRuntimeRecord{service: &observeStubRuntime{}}, metrics: metrics}
		state.Registry().Upsert(&livesession.LiveSession{ID: id, Handle: &runtimebinding.SessionState{Handle: invocationQueryRun{record: record}}}, id == "peer")
	}
	core, logs := observer.New(zap.InfoLevel)
	reader := sessionLifecycleReader{state: state, logger: zap.New(core)}
	reader.ObserveLiveLifecycleControl("first", factorysessions.LifecycleControlPause, factorysessions.ControlRequest{RequestID: "request-first"}, factorysessions.LifecycleControlOutcomeAccepted, factorysessions.LifecycleStatusPaused, nil)
	if len(first.fields) != 1 || first.fields[0].Reason != "PAUSE" || first.fields[0].Outcome != "ACCEPTED" || len(peer.fields) != 0 {
		t.Fatalf("control metrics first=%#v peer=%#v", first.fields, peer.fields)
	}
	if entries := logs.All(); len(entries) != 1 || entries[0].ContextMap()["session_id"] != "first" {
		t.Fatalf("addressed control logs = %#v", entries)
	}
}

func TestSessionHostStopKeepsReplacementGenerationReadable(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	owner := &SessionRuntime{owner: &Assembly{}}
	active := &owner.runtimeState
	newSession := func() *livesession.LiveSession {
		return &livesession.LiveSession{ID: "selected", Runtime: &factorysessions.LiveRuntime{}, Handle: &runtimebinding.SessionState{
			Handle: invocationQueryRun{record: &generationRuntimeRecord{service: &observeStubRuntime{}}},
		}}
	}
	previous, replacement := newSession(), newSession()
	previousProjection := newWorkAdmissionProjectionForGeneration("selected", previous.Runtime, nil, state.Clock())
	replacementProjection := newWorkAdmissionProjectionForGeneration("selected", replacement.Runtime, nil, state.Clock())
	owner.owner.workAdmissions = map[string][]*workAdmissionProjection{"selected": {previousProjection, replacementProjection}}
	runtimebinding.SessionStateFrom(previous).Owner = owner
	runtimebinding.SessionStateFrom(replacement).Owner = &SessionRuntime{
		owner: owner.owner,
	}
	state.Registry().Upsert(previous, true)
	active.SetActive(context.Background(), previous.ID, runtimebinding.HandleFromSession(previous))
	lifecycle := &hostLifecycleStub{onStop: func() { state.Registry().Upsert(replacement, true) }}
	control := NewScopeControl(state, func(run factoryruntime.RuntimeRun, _ factoryruntime.Clock) error {
		return lifecycle.Stop(run)
	}, zap.NewNop())
	reader := sessionLifecycleReader{state: state, control: control}
	if err := reader.StopLiveSession("selected"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimebinding.RequireLiveSession(state, "selected"); err != nil {
		t.Fatalf("replacement read after captured generation stop = %v", err)
	}
	if lifecycle.stopped != runtimebinding.HandleFromSession(previous) || active.ActiveHandle() != runtimebinding.HandleFromSession(replacement) {
		t.Fatal("stop or active selection crossed replacement generations")
	}
	if !previousProjection.closed || replacementProjection.closed {
		t.Fatal("cleanup did not stay on the captured generation's owner")
	}
}

func TestSessionHostStopPreservesCleanupFailureAndSelectedClock(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	active := &runtimebinding.State{}
	for _, id := range []string{"selected", "peer"} {
		registerScopeControlRuntime(state, id, &scopedControlRuntime{status: "RUNNING"}, zap.NewNop())
	}
	selected := state.Resolve("selected")
	bound := runtimebinding.SessionStateFrom(selected)
	bound.Clock = projectionClockStub{now: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)}
	active.SetActive(context.Background(), selected.ID, bound.Handle)
	failure := errors.New("injected binding cleanup failure")
	cleanupErr := failure
	binding := (factoryruntime.RuntimeBinding{}).New("selected-generation", selected.Runtime.Factory,
		func(context.Context) (factoryruntime.RuntimeDeactivationResult, error) {
			return factoryruntime.RuntimeDeactivationResult{}, cleanupErr
		})
	if err := NewScopeActivation(state).Activate(context.Background(), SessionScope{Session: selected, Binding: binding}); err != nil {
		t.Fatal(err)
	}
	var stoppedAt time.Time
	control := NewScopeControl(state, func(run factoryruntime.RuntimeRun, clock factoryruntime.Clock) error {
		if run != bound.Handle {
			t.Fatal("stop targeted a peer run")
		}
		stoppedAt = clock.Now()
		return nil
	}, zap.NewNop())
	var released []string
	host := SessionServiceHost(state, active, control, func(id string) { released = append(released, id) }, "", nil, nil, nil, nil, nil)
	if err := host.StopLiveSession("selected"); !errors.Is(err, failure) {
		t.Fatalf("cleanup failure = %v", err)
	}
	if !stoppedAt.Equal(bound.Clock.Now()) || state.Resolve("selected") == nil || state.Resolve("peer") == nil || len(released) != 0 {
		t.Fatal("failed cleanup lost selected clock, registration, peer or admission")
	}
	cleanupErr = nil
	if err := host.StopLiveSession("selected"); err != nil {
		t.Fatalf("cleanup retry = %v", err)
	}
	if state.Resolve("selected") != nil || state.Resolve("peer") == nil || !reflect.DeepEqual(released, []string{"selected"}) || active.Active().SessionID != "peer" {
		t.Fatal("successful cleanup failed to retire only the addressed generation")
	}
}

type hostDurableRead struct {
	durableexecution.Service
	err error
}

func (d *hostDurableRead) GetSession(_ context.Context, id string) (factorysessions.SessionReadResult, error) {
	return factorysessions.SessionReadResult{SessionID: id, Status: factorysessions.LifecycleStatusPaused}, d.err
}

func TestIndependentSessionHostGatewayRetainsDirectDurableReadsAndErrors(t *testing.T) {
	t.Parallel()
	durable := &hostDurableRead{}
	state := newWorkResolverSessionState()
	host := SessionServiceHost(state, nil, nil, nil, "", nil, nil, nil, nil, nil)
	streams := sessionruntime.NewResponseStreamObserver(runtimebinding.ResponseStreamRuntimeFromSessionHandle)
	gateway := NewWithLiveChangeCoordinator(host, stream.NewManagerWithDependencies(state, streams, &responsestream.Registry{}), nil, nil, nil, nil, nil, durable, nil, nil, nil)
	for _, id := range []string{"selected", "peer"} {
		got, err := gateway.GetSession(context.Background(), id)
		if err != nil || got.SessionID != id || got.Status != factorysessions.LifecycleStatusPaused {
			t.Fatalf("durable read %s = %#v, %v", id, got, err)
		}
	}
	failure := errors.New("selected durable read failure")
	durable.err = failure
	if _, err := gateway.GetSession(context.Background(), "selected"); !errors.Is(err, failure) {
		t.Fatalf("durable error = %v", err)
	}
	optional := NewWithLiveChangeCoordinator(host, stream.NewManagerWithDependencies(state, streams, &responsestream.Registry{}), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if _, err := optional.GetSession(context.Background(), "selected"); !errors.Is(err, factorysessions.ErrExecutionServiceNotConfigured) {
		t.Fatalf("optional durable read = %v", err)
	}
}

func TestSessionHostWithoutLiveStatePreservesOptionalReads(t *testing.T) {
	t.Parallel()
	host := SessionServiceHost(nil, nil, nil, nil, "selected-backend", nil, nil, nil, nil, nil)
	if host.GetLiveSession("missing") != nil || len(host.ListLiveSessionIDs()) != 0 {
		t.Fatal("absent live state published a session")
	}
	for _, selector := range []string{"", "missing"} {
		if _, err := host.RequireSession(selector); err == nil || err.Error() != "factory service is required" {
			t.Fatalf("required session %q = %v", selector, err)
		}
		if _, err := host.SessionFactory(selector); err == nil || err.Error() != "factory service is required" {
			t.Fatalf("required runtime %q = %v", selector, err)
		}
	}
	session := &livesession.LiveSession{ID: "detached"}
	if len(host.LiveSessionEvents(session)) != 0 || host.JavaScriptCheckpointStore(session) != nil {
		t.Fatal("absent live state exposed detached events or created a checkpoint store")
	}
	if host.BackendScopeID() != "selected-backend" {
		t.Fatal("optional live state discarded selected backend identity")
	}
}

type projectionIdentityStub struct {
	sessionidentity.Service
	err error
}

type projectionClockStub struct{ now time.Time }

func (c projectionClockStub) Now() time.Time { return c.now }

func (s projectionIdentityStub) Normalize(_ context.Context, request sessionidentity.NormalizeRequest) (sessionidentity.ResolvedIdentity, error) {
	return sessionidentity.ResolvedIdentity{LogicalSessionKeyID: "logical-" + request.FolderPath}, s.err
}

func TestSessionHostUsesAddressedFactsAndErrors(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.FixedZone("selected", 3600))
	checkpoints := func() factoryruntime.JavaScriptCheckpointStore {
		return &canonicalInspectionCheckpointStore{}
	}
	reader := SessionServiceHost(state, nil, nil, nil, "", projectionIdentityStub{}, projectionClockStub{now: now}, nil, checkpoints, nil)
	ctx := context.Background()
	for _, id := range []string{"first", "peer"} {
		runtime := &observeStubRuntime{result: factoryruntime.ObserveResult{Observation: factoryruntime.Observation{
			Health: factoryruntime.ObservationHealth{StreamGenerationID: "generation-" + id},
		}}}
		state.Registry().Upsert(&livesession.LiveSession{
			ID: id, IsDefault: id == "first",
			Handle:       &runtimebinding.SessionState{Handle: invocationQueryRun{record: &generationRuntimeRecord{service: runtime}}},
			SessionState: livesession.SessionState{FolderPath: id},
			Runtime: &factorysessions.LiveRuntime{
				BackendScopeID: "backend-" + id,
				RuntimeConfig: invocationQueryConfig{config: &factorydefinitions.FactoryConfig{
					Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{Kind: factorydefinitions.OrchestratorKindJavaScript},
				}},
				Factory: runtime,
			},
		}, id == "first")
	}
	assertSessionProjectionIdentities(t, state, reader, now)
	first, peer := state.Resolve("first"), state.Resolve("peer")
	sessionCheckpointStore(first, nil).Put(factorydefinitions.JavaScriptCheckpointRecord{ID: "first-checkpoint"})
	got, err := reader.BuildSessionProjectionContext(ctx, first)
	if err != nil || len(got.JavaScriptCheckpoints) != 1 || got.JavaScriptCheckpoints[0].ID != "first-checkpoint" {
		t.Fatalf("retained first checkpoint = %#v, %v", got.JavaScriptCheckpoints, err)
	}
	got, err = reader.BuildSessionProjectionContext(ctx, peer)
	if err != nil || len(got.JavaScriptCheckpoints) != 0 || got.Session.FolderPath != "peer" {
		t.Fatalf("isolated peer projection = %#v, %v", got, err)
	}
	// The production adapter reads checkpoints without an opening owner or gateway.
	host := SessionServiceHost(state, nil, nil, nil, "selected-backend", projectionIdentityStub{}, projectionClockStub{now: now}, nil, checkpoints, nil)
	got, err = host.BuildSessionProjectionContext(ctx, first)
	if err != nil || got.BackendScopeID != "selected-backend" || len(got.JavaScriptCheckpoints) != 1 {
		t.Fatalf("host projection without gateway = %#v, %v", got, err)
	}
	assertSessionProjectionFailures(t, state, reader)
}

func assertSessionProjectionIdentities(t *testing.T, state *sessionruntime.Service, reader Host, now time.Time) {
	t.Helper()
	ctx := context.Background()
	for _, selector := range []string{factorysessions.DefaultSessionID, "peer", "first"} {
		session := state.Resolve(selector)
		got, err := reader.BuildSessionProjectionContext(ctx, session)
		if err != nil || got.FactorySessionID != session.ID || got.Session.IsDefault != session.IsDefault ||
			got.BackendScopeID != "backend-"+session.ID || got.LogicalSessionKeyID != "logical-"+session.ID ||
			got.Observation.Health.StreamGenerationID != "generation-"+session.ID || !got.Now.Equal(now) || got.Now.Location() != time.UTC {
			t.Fatalf("projection %q = %#v, %v", selector, got, err)
		}
		got.Session.FolderPath = "detached mutation"
	}
}

func assertSessionProjectionFailures(t *testing.T, state *sessionruntime.Service, reader Host) {
	t.Helper()
	ctx := context.Background()
	first, peer := state.Resolve("first"), state.Resolve("peer")
	failure := errors.New("addressed observation failed")
	first.Runtime.Factory.(*observeStubRuntime).err = failure
	if _, err := reader.BuildSessionProjectionContext(ctx, first); !errors.Is(err, failure) {
		t.Fatalf("observation error = %v", err)
	}
	if _, err := reader.BuildSessionProjectionContext(ctx, peer); err != nil {
		t.Fatalf("peer after first failure: %v", err)
	}
	first.Runtime.Factory.(*observeStubRuntime).err = nil
	if _, err := reader.BuildSessionProjectionContext(ctx, first); err != nil {
		t.Fatalf("addressed observation retry: %v", err)
	}
	failingIdentity := SessionServiceHost(state, nil, nil, nil, "", projectionIdentityStub{err: failure}, nil, nil, nil, nil)
	if _, err := failingIdentity.BuildSessionProjectionContext(ctx, first); !errors.Is(err, failure) {
		t.Fatalf("identity error = %v", err)
	}
	for _, session := range []*livesession.LiveSession{nil, {ID: "missing"}} {
		if _, err := reader.BuildSessionProjectionContext(ctx, session); !errors.Is(err, factorysessions.ErrSessionNotFound) {
			t.Fatalf("missing error = %v", err)
		}
	}
	first.Handle = nil
	if _, err := reader.BuildSessionProjectionContext(ctx, first); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("gone runtime error = %v", err)
	}
	if _, err := reader.BuildSessionProjectionContext(ctx, peer); err != nil {
		t.Fatalf("peer after addressed runtime removal: %v", err)
	}
}

func TestSessionCheckpointStorePreservesOptionalFactoryAndRetainedRecords(t *testing.T) {
	t.Parallel()
	if got := sessionCheckpointStore(nil, nil); got != nil {
		t.Fatalf("nil session checkpoints = %v", got)
	}
	session := &livesession.LiveSession{ID: "chosen"}
	if got := sessionCheckpointStore(session, nil); got != nil {
		t.Fatalf("optional absent factory = %v", got)
	}
	create := func() factoryruntime.JavaScriptCheckpointStore { return &canonicalInspectionCheckpointStore{} }
	sessionCheckpointStore(session, create).Put(factorydefinitions.JavaScriptCheckpointRecord{ID: "retained"})
	for _, factory := range []factoryruntime.JavaScriptCheckpointStoreFactory{nil, create} {
		records := sessionCheckpointStore(session, factory).List()
		if len(records) != 1 || records[0].ID != "retained" {
			t.Fatalf("retained checkpoints = %#v", records)
		}
	}
}

type recordedInventoryStub struct {
	result recordings.RecordedSessionInventoryResult
	err    error
	root   string
}

func (s *recordedInventoryStub) ListRecordedSessions(request recordings.RecordedSessionInventoryRequest) (recordings.RecordedSessionInventoryResult, error) {
	s.root = request.RecordingRoot
	return s.result, s.err
}

func TestRecordedHistoryPreservesScopeOrderingAndReadFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	failure := errors.New("selected recording read failed")
	warnings := []factorysessions.RecordedSessionDiagnostic{{ArtifactReference: "bad.json", Code: "UNREADABLE_RECORDING", Reason: "Recording could not be decoded."}}
	inventory := &recordedInventoryStub{result: recordings.RecordedSessionInventoryResult{Warnings: warnings, Sessions: []recordings.RecordedSessionSummary{
		{FactorySessionID: "peer", ArtifactReference: "b", Format: recordings.RecordedSessionFormatV2JSONL},
		{FactorySessionID: "chosen", ArtifactReference: "z", Format: recordings.RecordedSessionFormatV1JSON},
		{FactorySessionID: "chosen", ArtifactReference: "a", Format: recordings.RecordedSessionFormatV2JSONL},
	}}}
	history := NewRecordedHistory(func() (string, error) { return "selected-home", nil }, inventory)
	for _, scope := range []factorysessions.SessionListScope{factorysessions.SessionListScopeHistory, factorysessions.SessionListScopeAll} {
		result, err := history.ListSessions(ctx, factorysessions.ListSessionsRequest{Scope: scope})
		want := []factorysessions.RecordedSessionListSummary{
			{SessionID: "chosen", Source: factorysessions.RecordedSessionListSourceHistory, ArtifactReference: "a", Format: "V2_JSONL"},
			{SessionID: "chosen", Source: factorysessions.RecordedSessionListSourceHistory, ArtifactReference: "z", Format: "V1_JSON"},
			{SessionID: "peer", Source: factorysessions.RecordedSessionListSourceHistory, ArtifactReference: "b", Format: "V2_JSONL"},
		}
		if err != nil || result.Scope != scope || !reflect.DeepEqual(result.RecordedSessions, want) || !reflect.DeepEqual(result.Warnings, warnings) {
			t.Fatalf("%s history = %#v, %v", scope, result, err)
		}
		result.RecordedSessions[0].SessionID = "mutated"
		result.Warnings[0].Reason = "mutated"
	}
	if inventory.root != filepath.Join("selected-home", ".you-agent-factory", "recordings") {
		t.Fatalf("recording root = %q", inventory.root)
	}
	inventory.err = failure
	for _, request := range []factorysessions.ListSessionsRequest{
		{}, {Scope: factorysessions.SessionListScopeLive}, {Scope: factorysessions.SessionListScopePersisted},
		{Scope: factorysessions.SessionListScopeAll, ExcludeRecordedHistory: true},
	} {
		result, err := history.ListSessions(ctx, request)
		if err != nil || len(result.RecordedSessions) != 0 {
			t.Fatalf("excluded history = %#v, %v", result, err)
		}
	}
	_, err := history.ListSessions(ctx, factorysessions.ListSessionsRequest{Scope: factorysessions.SessionListScopeHistory, ExcludeRecordedHistory: true})
	if !errors.Is(err, failure) {
		t.Fatalf("history error = %v, want wrapped inventory cause", err)
	}
	inventory.err = nil
	inventory.result.Sessions = nil
	result, err := history.ListSessions(ctx, factorysessions.ListSessionsRequest{Scope: factorysessions.SessionListScopeHistory})
	if err != nil || result.RecordedSessions == nil || len(result.RecordedSessions) != 0 {
		t.Fatalf("empty inventory = %#v, %v", result, err)
	}
	failedHome := NewRecordedHistory(func() (string, error) { return "", failure }, inventory)
	if _, err := failedHome.ListSessions(ctx, factorysessions.ListSessionsRequest{Scope: factorysessions.SessionListScopeAll}); !errors.Is(err, failure) {
		t.Fatalf("home error = %v, want wrapped home cause", err)
	}
}

func TestRecordedHistoryPreservesUnavailableInventoryAndHomeErrors(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		home      factorysessions.HomeDirectoryResolver
		inventory recordings.RecordedSessionInventory
		scope     factorysessions.SessionListScope
		wantError string
	}{
		{name: "history needs inventory", scope: factorysessions.SessionListScopeHistory, wantError: "recorded session inventory is required"},
		{name: "all tolerates missing inventory", scope: factorysessions.SessionListScopeAll},
		{name: "history needs home", inventory: &recordedInventoryStub{}, scope: factorysessions.SessionListScopeHistory, wantError: "recorded session home directory resolver is required"},
		{name: "history rejects empty home", home: func() (string, error) { return "  ", nil }, inventory: &recordedInventoryStub{}, scope: factorysessions.SessionListScopeHistory, wantError: "resolve recorded session home directory: empty path"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			history := NewRecordedHistory(test.home, test.inventory)
			result, err := history.ListSessions(context.Background(), factorysessions.ListSessionsRequest{Scope: test.scope})
			if test.wantError != "" {
				if err == nil || err.Error() != test.wantError {
					t.Fatalf("history error = %v, want %q", err, test.wantError)
				}
			} else if err != nil || result.Scope != test.scope || len(result.RecordedSessions) != 0 {
				t.Fatalf("optional history = %#v, %v", result, err)
			}
		})
	}
}

type syncPreflightGatewayStub struct {
	roles.SessionGateway
	requestedID string
	reconnect   *factorydefinitions.FactoryEventReconnectCursor
	logical     *factorydefinitions.FactorySessionLogicalResolveHint
}

func (stub *syncPreflightGatewayStub) GetFactorySessionSyncPreflight(
	_ context.Context,
	sessionID string,
	reconnect *factorydefinitions.FactoryEventReconnectCursor,
	logical *factorydefinitions.FactorySessionLogicalResolveHint,
) (factorysessions.SyncPreflightResult, error) {
	stub.requestedID, stub.reconnect, stub.logical = sessionID, reconnect, logical
	return factorysessions.SyncPreflightResult{RequestedSessionID: sessionID, Reason: factorysessions.SyncPreflightReasonOK}, nil
}

type syncPreflightOwnerStub struct {
	projectionOwnerStub
	gateway roles.SessionGateway
}

func (stub syncPreflightOwnerStub) Gateway() roles.SessionGateway { return stub.gateway }

func TestAssemblySyncPreflightUsesSelectedSessionGateway(t *testing.T) {
	state := newWorkResolverSessionState()
	assembly := &Assembly{SessionGateway: &Service{}, state: state, registry: state.Registry()}
	first := &syncPreflightGatewayStub{}
	second := &syncPreflightGatewayStub{}
	assembly.registry.Upsert(&livesession.LiveSession{ID: "first", Handle: &runtimebinding.SessionState{Owner: syncPreflightOwnerStub{gateway: first}}}, true)
	assembly.registry.Upsert(&livesession.LiveSession{ID: "second", Handle: &runtimebinding.SessionState{Owner: syncPreflightOwnerStub{gateway: second}}}, false)
	reconnect := &factorydefinitions.FactoryEventReconnectCursor{AfterEventID: "event-1"}
	logical := &factorydefinitions.FactorySessionLogicalResolveHint{BackendScopeID: "scope", LogicalSessionKeyID: "key"}

	got, err := assembly.GetFactorySessionSyncPreflight(context.Background(), "second", reconnect, logical)
	if err != nil || got.Reason != factorysessions.SyncPreflightReasonOK || second.requestedID != "second" || second.reconnect != reconnect || second.logical != logical || first.requestedID != "" {
		t.Fatalf("second session preflight = %#v, %v; first=%q second=%q", got, err, first.requestedID, second.requestedID)
	}
	got, err = assembly.GetFactorySessionSyncPreflight(context.Background(), "~default", nil, logical)
	if err != nil || got.Reason != factorysessions.SyncPreflightReasonOK || first.requestedID != "~default" {
		t.Fatalf("stale default preflight = %#v, %v; current gateway request=%q", got, err, first.requestedID)
	}
}

func TestAssemblyWorkerSessionsObservationUsesSelectedSession(t *testing.T) {
	state := newWorkResolverSessionState()
	assembly := &Assembly{state: state, registry: state.Registry()}
	first := &workerSessionsObservationMarker{}
	second := &workerSessionsObservationMarker{}
	assembly.registry.Upsert(&livesession.LiveSession{ID: "first", Handle: workerSessionsState(first)}, true)
	assembly.registry.Upsert(&livesession.LiveSession{ID: "second", Handle: workerSessionsState(second)}, false)

	if got := assembly.WorkerSessionsObservationForSession("second"); got != second {
		t.Fatalf("second session observation = %T(%[1]v), want second session", got)
	}
	if got := assembly.WorkerSessionsObservationForSession("first"); got != first {
		t.Fatalf("first session observation = %T(%[1]v), want first session", got)
	}
	if got := assembly.WorkerSessionsObservationForSession("missing"); got != nil {
		t.Fatalf("missing session observation = %T(%[1]v), want nil", got)
	}
}

func TestAssemblyWorkerSessionsObservationIsSafeAgainstConcurrentStartBinding(t *testing.T) {
	t.Parallel()

	state := newWorkResolverSessionState()
	assembly := &Assembly{state: state, registry: state.Registry()}
	bound := &runtimebinding.SessionState{}
	// Start binds Worker Sessions after the session is already resolvable, so
	// fleet observation reads can run concurrently with the bind.
	assembly.registry.Upsert(&livesession.LiveSession{ID: "starting", Handle: bound}, true)
	observation := &workerSessionsObservationMarker{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 200 {
			bound.SetWorkerSessions(observation)
		}
	}()
	for range 200 {
		_ = assembly.WorkerSessionsObservationForSession("starting")
	}
	<-done

	if got := assembly.WorkerSessionsObservationForSession("starting"); got != observation {
		t.Fatalf("bound session observation = %T(%[1]v), want bound observation", got)
	}
}

func workerSessionsState(observation workersessions.ObservationService) *runtimebinding.SessionState {
	state := &runtimebinding.SessionState{}
	state.SetWorkerSessions(observation)
	return state
}

func TestAssemblyRuntimeScopeRecognizesExplicitDefaultSessionID(t *testing.T) {
	state := newWorkResolverSessionState()
	assembly := &Assembly{state: state, registry: state.Registry()}
	const defaultID = "550e8400-e29b-41d4-a716-446655440000"
	assembly.registry.Upsert(&livesession.LiveSession{ID: defaultID, IsDefault: true}, true)

	for _, selector := range []string{factorysessions.DefaultSessionID, defaultID} {
		gotID, isDefault, err := assembly.ResolveFactorySessionRuntimeScope(selector)
		if err != nil || gotID != defaultID || !isDefault {
			t.Fatalf("scope for %q = (%q, %t, %v), want (%q, true, nil)", selector, gotID, isDefault, err, defaultID)
		}
	}
}

type projectionOwnerStub struct {
	statuses map[string]string
	status   string
	cfg      *factorydefinitions.FactoryConfig
}

type durableListStub struct {
	roles.SessionGateway
	requests []factorysessions.ListSessionsRequest
}

func (stub *durableListStub) ListSessions(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	stub.requests = append(stub.requests, request)
	return factorysessions.ListSessionsResult{DurableSessions: []factorysessions.DurableSessionListSummary{{SessionID: "durable"}}}, nil
}

func TestAssemblyListsCanonicalRegistryAndProcessDurableSessions(t *testing.T) {
	state := newWorkResolverSessionState()
	durable := &durableListStub{}
	assembly := &Assembly{SessionGateway: durable, state: state, registry: state.Registry()}
	assembly.registry.Upsert(&livesession.LiveSession{ID: "first", SessionState: livesession.SessionState{FolderPath: "first-dir"}}, true)
	assembly.registry.Upsert(&livesession.LiveSession{ID: "second", SessionState: livesession.SessionState{FolderPath: "second-dir"}}, false)

	result, err := assembly.ListSessions(context.Background(), factorysessions.ListSessionsRequest{Scope: factorysessions.SessionListScopeAll})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(result.LiveSessions) != 2 || result.LiveSessions[0].ID != "first" || result.LiveSessions[1].ID != "second" {
		t.Fatalf("live sessions = %#v, want both canonical registry records", result.LiveSessions)
	}
	if len(result.DurableSessions) != 1 || result.DurableSessions[0].SessionID != "durable" {
		t.Fatalf("durable sessions = %#v, want process durable owner", result.DurableSessions)
	}
	if len(durable.requests) != 1 || durable.requests[0].Scope != factorysessions.SessionListScopePersisted || !durable.requests[0].ExcludeRecordedHistory {
		t.Fatalf("durable requests = %#v, want one persisted-only read", durable.requests)
	}
}

func TestAssemblyInvokesOwnerOfSelectedCanonicalSession(t *testing.T) {
	state := newWorkResolverSessionState()
	assembly := &Assembly{state: state, registry: state.Registry()}
	second := &canonicalSessionInvokerFake{result: factorydefinitions.FactoryInvocationResult{SessionID: "second"}}
	assembly.invoker = second
	assembly.registry.Upsert(&livesession.LiveSession{ID: "first", Handle: &runtimebinding.SessionState{}}, true)
	assembly.registry.Upsert(&livesession.LiveSession{ID: "second", Handle: &runtimebinding.SessionState{}}, false)
	input := &work.PreparedInvocationInput{ResolvedInput: &work.ResolvedInput{Text: "hello"}}
	result, err := assembly.Invoke(context.Background(), factorysessions.SessionInvokeRequest{SessionID: "second", Input: input})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.SessionID != "second" || second.sessionID != "second" || second.canonicalCalls != 1 || second.contentProvided {
		t.Fatalf("invocation was not scoped to selected session: result=%#v selected=%s calls=%d content=%t", result, second.sessionID, second.canonicalCalls, second.contentProvided)
	}
	if _, err := assembly.Invoke(context.Background(), factorysessions.SessionInvokeRequest{SessionID: "missing"}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing session error = %v, want ErrSessionNotFound", err)
	}
}

func TestAssemblySubscribesToSelectedCanonicalSessionResponses(t *testing.T) {
	state := newWorkResolverSessionState()
	cursor := &factorysessions.ResponseEventCursor{}
	responses := &canonicalInspectionResponseStreamFake{cursor: cursor}
	assembly := &Assembly{state: state, registry: state.Registry(), responseStreams: responses}
	assembly.registry.Upsert(&livesession.LiveSession{ID: "first", ResponseEvents: &responseeventstore.SessionResponseEventStore{}}, true)
	assembly.registry.Upsert(&livesession.LiveSession{ID: "second", ResponseEvents: &responseeventstore.SessionResponseEventStore{}}, false)
	got, err := assembly.SubscribeResponses(context.Background(), factorysessions.SessionResponseSubscriptionRequest{
		SessionID: "second", AfterSequence: 3, DispatchID: "dispatch-1", Kinds: []factorysessions.ResponseEventKind{factorysessions.ResponseEventKindMessage},
	})
	if err != nil {
		t.Fatalf("SubscribeResponses: %v", err)
	}
	if got.Cursor != cursor || responses.calls != 1 || responses.request.AfterSequence != 3 || responses.request.DispatchID != "dispatch-1" {
		t.Fatalf("response owner request lost selection or filter: cursor=%p calls=%d request=%#v", got.Cursor, responses.calls, responses.request)
	}
}

func (owner projectionOwnerStub) BuildSessionProjectionContext(_ context.Context, session *livesession.LiveSession) (factorysessions.ProjectionContext, error) {
	if owner.statuses != nil {
		owner.status = owner.statuses[session.ID]
	}
	return factorysessions.ProjectionContext{
		FactorySessionID: session.ID,
		Session: &factorysessions.ScopedLiveSessionSummary{
			ID: session.ID, FactoryDir: session.FactoryDir, FolderPath: session.FolderPath,
			Project: session.Project, IsDefault: session.IsDefault,
			Runtime: &factorysessions.RuntimeProjection{},
		},
		LifecycleControlStatus: owner.status,
		BackendScopeID:         "backend-" + session.ID,
		FactoryCfg:             owner.cfg,
	}, nil
}

func TestAssemblyReadsLiveResultFromCanonicalSession(t *testing.T) {
	state := newWorkResolverSessionState()
	assembly := &Assembly{state: state, registry: state.Registry(), projectionReader: projectionOwnerStub{cfg: &factorydefinitions.FactoryConfig{Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{Kind: factorydefinitions.OrchestratorKindJavaScript}}}, sessionResultProjection: &canonicalInspectionResultProjectionFake{
		result: factoryruntime.SessionResultProjection{Live: factoryruntime.LiveSessionResult{SessionID: "result-1", Status: "SUCCEEDED"}},
	}}
	checkpoint := &canonicalInspectionCheckpointStore{records: []factorydefinitions.JavaScriptCheckpointRecord{{ID: "checkpoint-1", ArtifactID: "artifact-1"}}}
	assembly.registry.Upsert(&livesession.LiveSession{
		ID: "result-1", JavaScriptCheckpoints: checkpoint,
		Handle: &runtimebinding.SessionState{Owner: projectionOwnerStub{cfg: &factorydefinitions.FactoryConfig{
			Orchestrator: &factorydefinitions.FactoryOrchestratorConfig{Kind: factorydefinitions.OrchestratorKindJavaScript},
		}}},
	}, true)
	complete, err := assembly.ReadResult(context.Background(), factorysessions.SessionResultReadRequest{SessionID: "result-1", Mode: factorysessions.SessionOperationModeLive})
	if err != nil || complete.Live == nil || complete.Live.Status != "SUCCEEDED" {
		t.Fatalf("complete result = %#v, error %v", complete, err)
	}
	partial, err := assembly.ReadResult(context.Background(), factorysessions.SessionResultReadRequest{
		SessionID: "result-1", Mode: factorysessions.SessionOperationModeLive,
		Request: factorysessions.ResultRequest{Mode: factorysessions.ResultModePartial},
	})
	if err != nil || partial.Live == nil || len(partial.Live.CheckpointRefs) != 1 || partial.Live.CheckpointRefs[0].ID != "checkpoint-1" {
		t.Fatalf("partial result = %#v, error %v", partial, err)
	}
}

func TestAssemblyReadsFullProjectionFromCanonicalSessionRegistry(t *testing.T) {
	state := newWorkResolverSessionState()
	assembly := &Assembly{state: state, registry: state.Registry(), projectionReader: projectionOwnerStub{statuses: map[string]string{"session-first": "running", "session-second": "paused"}}}
	for _, record := range []struct {
		id, status string
		isDefault  bool
	}{
		{id: "session-second", status: "paused"},
		{id: "session-first", status: "running", isDefault: true},
	} {
		assembly.registry.Upsert(&livesession.LiveSession{
			ID:           record.id,
			SessionState: livesession.SessionState{FactoryDir: "/factory/" + record.id, FolderPath: "/project"},
			IsDefault:    record.isDefault,
			Runtime:      &factorysessions.LiveRuntime{},
			Handle:       &runtimebinding.SessionState{Owner: projectionOwnerStub{status: record.status}},
		}, record.isDefault)
	}

	assertCanonicalFullProjection(t, assembly)
	assertCanonicalGetProjection(t, assembly)
	assertCanonicalListProjection(t, assembly)

	if _, err := assembly.GetFactorySession(context.Background(), "missing"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing session error = %v, want ErrSessionNotFound", err)
	}
}

func assertCanonicalFullProjection(t *testing.T, assembly *Assembly) {
	t.Helper()
	projection, err := assembly.GetFactorySession(context.Background(), "session-second")
	if err != nil {
		t.Fatalf("GetFactorySession: %v", err)
	}
	if projection.Context.BackendScopeID != "backend-session-second" || projection.Runtime.LifecycleControlStatus == nil || *projection.Runtime.LifecycleControlStatus != "paused" {
		t.Fatalf("full projection was lost: %#v", projection)
	}
}

func assertCanonicalGetProjection(t *testing.T, assembly *Assembly) {
	t.Helper()
	got, err := assembly.Get(context.Background(), factorysessions.SessionGetRequest{Mode: factorysessions.SessionOperationModeLive, SessionID: "session-second"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Session.SessionID != "session-second" || got.Session.FactoryDir != "/factory/session-second" || !got.Session.RuntimeAvailable {
		t.Fatalf("canonical get lost live session data: %#v", got.Session)
	}
}

func assertCanonicalListProjection(t *testing.T, assembly *Assembly) {
	t.Helper()
	listed, err := assembly.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	if len(listed) != 2 || listed[0].Context.FactorySessionID != "session-first" || listed[1].Context.FactorySessionID != "session-second" {
		t.Fatalf("listed sessions = %#v", listed)
	}
	if !listed[0].RuntimeAvailable || !listed[1].RuntimeAvailable || listed[0].Context.BackendScopeID == listed[1].Context.BackendScopeID {
		t.Fatalf("list lost separate runtime projections: %#v", listed)
	}
}

func TestProcessDurableScopePreservesProjectAndResumeSelection(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	scope := NewProcessDurableScope(state)
	assertSelection := func(project string, wantError error) {
		t.Helper()
		if got := scope.CurrentProjectRoot(); got != project {
			t.Fatalf("project = %q, want %q", got, project)
		}
		_, err := scope.ResumeRuntimeScope(project)
		if !errors.Is(err, wantError) {
			t.Fatalf("resume %q error = %v, want %v", project, err, wantError)
		}
	}
	assertSelection("", factorysessions.ErrRuntimeNotAvailable)
	first := &livesession.LiveSession{ID: "first", SessionState: livesession.SessionState{FactoryDir: filepath.Join("projects", "first")}}
	state.Registry().Upsert(first, true)
	assertSelection(first.FactoryDir, factorysessions.ErrRuntimeNotAvailable)
	if _, err := scope.ResumeRuntimeScope("other"); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("project mismatch = %v", err)
	}
	peer := &livesession.LiveSession{ID: "peer", SessionState: livesession.SessionState{FactoryDir: filepath.Join("projects", "peer")}}
	state.Registry().Upsert(peer, true)
	assertSelection("", factorysessions.ErrRuntimeNotAvailable)
	defaultSession := &livesession.LiveSession{ID: factorysessions.DefaultSessionID, IsDefault: true, SessionState: livesession.SessionState{FactoryDir: first.FactoryDir}}
	state.Registry().Upsert(defaultSession, false)
	assertSelection(first.FactoryDir, factorysessions.ErrRuntimeNotAvailable)
	// An absent runtime is different from a different project, even with peers.
	if _, err := scope.ResumeRuntimeScope(peer.FactoryDir); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("default project mismatch = %v", err)
	}
	// Queries use current keyed facts after removal, rather than a captured Root.
	state.Registry().Remove(factorysessions.DefaultSessionID)
	state.Registry().Remove(first.ID)
	assertSelection(peer.FactoryDir, factorysessions.ErrRuntimeNotAvailable)
	peer.Handle = &runtimebinding.SessionState{Instance: &generationRuntimeRecord{}}
	got, err := scope.ResumeRuntimeScope(peer.FactoryDir)
	if err != nil || got.WorkerSettings != nil || got.MockWorkers != nil || got.WorkerAttemptStarter != nil || got.WorkerResourceAdmission != nil || got.WorkerProgressPublisher != nil {
		t.Fatalf("optional resume capabilities = %#v, %v", got, err)
	}
}

// Exercise capabilities through their actual invocation rather than proving
// constructor or pointer identity. The record and service fallbacks are distinct.
type resumeScopeRuntime struct {
	factoryruntime.Service
	factoryruntime.ResourceCapacityLeaseAdmission
	attempts int
	progress int
	failure  error
}

func (r *resumeScopeRuntime) BeginWorkerAttempt(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error) {
	r.attempts++
	return nil, r.failure
}

func (r *resumeScopeRuntime) RuntimeProgressPublisher() workers.ProgressPublisher {
	return func(workers.ProgressFragment) { r.progress++ }
}

func (r *resumeScopeRuntime) AcquireResourceCapacityLease(context.Context, factoryruntime.ResourceCapacityLeaseRequest) (*factoryruntime.ResourceCapacityLease, error) {
	return nil, r.failure
}

type resumeScopeRecord struct {
	generationRuntimeRecord
	*resumeScopeRuntime
}

func TestProcessDurableScopeRetainsSelectedCapabilitiesAndDetachedSettings(t *testing.T) {
	t.Parallel()
	for _, recordCapabilities := range []bool{false, true} {
		t.Run(fmt.Sprint("recordCapabilities=", recordCapabilities), func(t *testing.T) {
			t.Parallel()
			state := newWorkResolverSessionState()
			failure := errors.New("selected worker failure")
			selected := &resumeScopeRuntime{failure: failure}
			peer := &resumeScopeRuntime{}
			var record runtimebinding.RuntimeInstance = &generationRuntimeRecord{service: selected}
			if recordCapabilities {
				record = &resumeScopeRecord{generationRuntimeRecord: generationRuntimeRecord{service: peer}, resumeScopeRuntime: selected}
			}
			bound := &runtimebinding.SessionState{Instance: record}
			bound.SetWorkerSettings(&factoryruntime.JavaScriptWorkerSettings{DefaultModel: "selected-model"})
			bound.SetMockWorkers(&workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{ID: "selected-mock"}}})
			project := filepath.Join("project", "selected")
			state.Registry().Upsert(&livesession.LiveSession{ID: factorysessions.DefaultSessionID, IsDefault: true, SessionState: livesession.SessionState{FactoryDir: project}, Handle: bound}, false)
			state.Registry().Upsert(&livesession.LiveSession{ID: "peer", SessionState: livesession.SessionState{FactoryDir: "other"}, Handle: &runtimebinding.SessionState{Instance: &generationRuntimeRecord{service: peer}}}, true)
			scope := NewProcessDurableScope(state)
			got, err := scope.ResumeRuntimeScope(filepath.Join(project, "."))
			if err != nil {
				t.Fatal(err)
			}
			assertDetachedResumeSettings(t, got.WorkerSettings, got.MockWorkers)
			got.WorkerSettings.DefaultModel = "mutated"
			got.MockWorkers.MockWorkers[0].ID = "mutated"
			again, err := scope.ResumeRuntimeScope(project)
			if err != nil {
				t.Fatal(err)
			}
			assertDetachedResumeSettings(t, again.WorkerSettings, again.MockWorkers)
			if _, err := got.WorkerAttemptStarter(context.Background(), &workers.ExecuteRequest{}); !errors.Is(err, failure) {
				t.Fatalf("selected worker error = %v", err)
			}
			got.WorkerProgressPublisher(workers.ProgressFragment{})
			admissionError := failure
			if recordCapabilities {
				admissionError = nil // Admission always belongs to RuntimeService.
			}
			if _, err := got.WorkerResourceAdmission.AcquireResourceCapacityLease(context.Background(), factoryruntime.ResourceCapacityLeaseRequest{}); !errors.Is(err, admissionError) {
				t.Fatalf("admission error = %v, want %v", err, admissionError)
			}
			if selected.attempts != 1 || selected.progress != 1 || peer.attempts != 0 || peer.progress != 0 {
				t.Fatalf("worker effects crossed session selection: selected=%+v peer=%+v", selected, peer)
			}
		})
	}
}

// Logical resolution is an identity-owner edge; this fixture only selects
// records when the reader forwards the expected scope and logical key.
type routingIdentityStub struct{ projectionIdentityStub }

func (s routingIdentityStub) ResolveLogical(registry sessionregistry.Service, scope, key string) *livesession.LiveSession {
	if scope != "selected-backend" {
		return nil
	}
	return registry.Get(key)
}

func TestSessionIdentityReaderPreservesDefaultSuccessorAndPeer(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	active := &runtimebinding.State{}
	reader := sessionIdentityReader{state: state, active: active, backendScope: " selected-backend ", identity: routingIdentityStub{}}
	for _, id := range []string{"first", "peer"} {
		state.Registry().Upsert(&livesession.LiveSession{
			ID: id, SessionState: livesession.SessionState{FolderPath: id},
			Handle: &runtimebinding.SessionState{Handle: invocationQueryRun{record: &generationRuntimeRecord{service: &observeStubRuntime{}}}},
		}, id == "peer")
	}
	active.SetActive(context.Background(), "first", nil)
	assertDefaultSuccessor(t, reader, "first", true)
	active.ClearActive()
	assertDefaultSuccessor(t, reader, "peer", true)
	// A real default wins over both active and current non-default records.
	first := state.Resolve("first")
	first.IsDefault = true
	state.Registry().Upsert(first, false)
	assertDefaultSuccessor(t, reader, "first", false)
	state.Registry().Remove("first")
	for _, selector := range []string{"peer", "missing", "first"} {
		got, err := reader.ResolveSyncPreflightTarget(selector, nil)
		if err != nil || got.Remapped || got.Unresolved || (selector == "peer") != (got.Session != nil) {
			t.Fatalf("after retirement %q = %#v, %v", selector, got, err)
		}
	}
	if reader.BackendScopeID() != "selected-backend" || reader.LogicalSessionKeyID(state.Resolve("peer")) != "logical-peer" || reader.LogicalSessionKeyID(nil) != "" {
		t.Fatal("addressed logical/backend facts changed")
	}
	assertHostLogicalRemapping(t, reader)
	reader.identity = projectionIdentityStub{err: errors.New("identity unavailable")}
	if reader.LogicalSessionKeyID(state.Resolve("peer")) != "" {
		t.Fatal("failed optional logical identity must stay empty")
	}
}

func TestSessionIdentityReaderLogicalPreflight(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, requested, scope, key, wantID string
		remapped, unresolved                bool
	}{
		{"remapped", "old", "selected-backend", "peer", "peer", true, false},
		{"empty selector", "", "selected-backend", "peer", "peer", false, false},
		{"scope mismatch", "old", "foreign", "peer", "", false, true},
		{"missing logical", "old", "selected-backend", "absent", "", false, true},
		{"incomplete hint", "old", "", "peer", "", false, false},
		{"direct beats hint", "peer", "foreign", "absent", "peer", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := newWorkResolverSessionState()
			state.Registry().Upsert(&livesession.LiveSession{ID: "peer", Handle: &runtimebinding.SessionState{
				Handle: invocationQueryRun{record: &generationRuntimeRecord{service: &observeStubRuntime{}}},
			}}, true)
			reader := sessionIdentityReader{state: state, backendScope: "selected-backend", identity: routingIdentityStub{}}
			got, err := reader.ResolveSyncPreflightTarget(tc.requested, &factorydefinitions.FactorySessionLogicalResolveHint{BackendScopeID: tc.scope, LogicalSessionKeyID: tc.key})
			id := ""
			if got.Session != nil {
				id = got.Session.ID
			}
			if err != nil || id != tc.wantID || got.Remapped != tc.remapped || got.Unresolved != tc.unresolved {
				t.Fatalf("logical preflight = %#v, %v", got, err)
			}
		})
	}
}

func assertDetachedResumeSettings(t *testing.T, settings *factoryruntime.JavaScriptWorkerSettings, mock *workers.MockWorkersConfig) {
	t.Helper()
	if settings.DefaultModel != "selected-model" || mock.MockWorkers[0].ID != "selected-mock" {
		t.Fatalf("selected detached resume settings = %#v, mock = %#v", settings, mock)
	}
}

func assertDefaultSuccessor(t *testing.T, reader sessionIdentityReader, wantID string, remapped bool) {
	t.Helper()
	got, err := reader.ResolveSyncPreflightTarget(factorysessions.DefaultSessionID, nil)
	if err != nil || got.Session == nil {
		t.Fatalf("default successor = %#v, %v", got, err)
	}
	if got.Session.ID != wantID || got.Remapped != remapped || got.Unresolved {
		t.Fatalf("default successor = %#v, want %s remapped=%t", got, wantID, remapped)
	}
}

func assertHostLogicalRemapping(t *testing.T, reader sessionIdentityReader) {
	t.Helper()
	host := SessionServiceHost(reader.state, reader.active, nil, nil, reader.backendScope, reader.identity, nil, nil, nil, nil)
	got, err := host.ResolveSyncPreflightTarget("old", &factorydefinitions.FactorySessionLogicalResolveHint{
		BackendScopeID: "selected-backend", LogicalSessionKeyID: "peer",
	})
	if err != nil || got.Session == nil || got.Session.ID != "peer" || !got.Remapped || host.LogicalSessionKeyID(got.Session) != "logical-peer" {
		t.Fatalf("production host routing without gateway = %#v, %v", got, err)
	}
}

func TestRecordedHistoryOpeningUsesExplicitProfile(t *testing.T) {
	t.Parallel()
	inventory := &recordedInventoryStub{}
	history := NewRecordedHistory(func() (string, error) { t.Fatal("opening consulted process-global home"); return "", nil }, inventory)
	assembly := &Assembly{recordedHistory: history}
	_, err := assembly.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{RecordingRoot: "invocation-profile"})
	if err != nil || inventory.root != "invocation-profile" {
		t.Fatalf("scoped inventory root=%q, error=%v", inventory.root, err)
	}
}
