package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	sessionidentity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type workerSessionsObservationMarker struct{ workersessions.Service }

type projectionIdentityStub struct {
	sessionidentity.Service
	err error
}

type projectionClockStub struct{ now time.Time }

func (c projectionClockStub) Now() time.Time { return c.now }

func (s projectionIdentityStub) Normalize(_ context.Context, request sessionidentity.NormalizeRequest) (sessionidentity.ResolvedIdentity, error) {
	return sessionidentity.ResolvedIdentity{LogicalSessionKeyID: "logical-" + request.FolderPath}, s.err
}

func TestSessionProjectionReaderUsesAddressedFactsAndErrors(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.FixedZone("selected", 3600))
	reader := sessionProjectionReader{
		state: state, identity: projectionIdentityStub{}, clock: projectionClockStub{now: now},
		checkpoints: func() factoryruntime.JavaScriptCheckpointStore {
			return &canonicalInspectionCheckpointStore{}
		},
	}
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
	// The production adapter must read checkpoints without an attached gateway.
	runtime := &SessionRuntime{
		sessionState: state, backendScopeID: "selected-backend", identity: reader.identity,
		clock: reader.clock, newJavaScriptCheckpointStore: reader.checkpoints,
	}
	got, err = SessionServiceHost(runtime).BuildSessionProjectionContext(ctx, first)
	if err != nil || got.BackendScopeID != "selected-backend" || len(got.JavaScriptCheckpoints) != 1 {
		t.Fatalf("host projection without gateway = %#v, %v", got, err)
	}
	failure := errors.New("addressed observation failed")
	first.Runtime.Factory.(*observeStubRuntime).err = failure
	if _, err := reader.BuildSessionProjectionContext(ctx, first); !errors.Is(err, failure) {
		t.Fatalf("observation error = %v", err)
	}
	if _, err := reader.BuildSessionProjectionContext(ctx, peer); err != nil {
		t.Fatalf("peer after first failure: %v", err)
	}
	first.Runtime.Factory.(*observeStubRuntime).err = nil
	reader.identity = projectionIdentityStub{err: failure}
	if _, err := reader.BuildSessionProjectionContext(ctx, first); !errors.Is(err, failure) {
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
	inventory := &recordedInventoryStub{result: recordings.RecordedSessionInventoryResult{Sessions: []recordings.RecordedSessionSummary{
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
		if err != nil || result.Scope != scope || !reflect.DeepEqual(result.RecordedSessions, want) {
			t.Fatalf("%s history = %#v, %v", scope, result, err)
		}
		result.RecordedSessions[0].SessionID = "mutated"
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
	status string
	cfg    *factorydefinitions.FactoryConfig
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
	first := &canonicalSessionInvokerFake{result: factorydefinitions.FactoryInvocationResult{SessionID: "first"}}
	second := &canonicalSessionInvokerFake{result: factorydefinitions.FactoryInvocationResult{SessionID: "second"}}
	assembly.registry.Upsert(&livesession.LiveSession{ID: "first", Handle: &runtimebinding.SessionState{Invoker: first}}, true)
	assembly.registry.Upsert(&livesession.LiveSession{ID: "second", Handle: &runtimebinding.SessionState{Invoker: second}}, false)
	input := &work.PreparedInvocationInput{ResolvedInput: &work.ResolvedInput{Text: "hello"}}
	result, err := assembly.Invoke(context.Background(), factorysessions.SessionInvokeRequest{SessionID: "second", Input: input})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.SessionID != "second" || second.canonicalCalls != 1 || first.canonicalCalls != 0 || second.contentProvided {
		t.Fatalf("invocation was not scoped to selected session: result=%#v first=%d second=%d content=%t", result, first.canonicalCalls, second.canonicalCalls, second.contentProvided)
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
	assembly := &Assembly{state: state, registry: state.Registry(), sessionResultProjection: &canonicalInspectionResultProjectionFake{
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
	assembly := &Assembly{state: state, registry: state.Registry()}
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

func (r *resumeScopeRuntime) BeginWorkerAttempt(context.Context, workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error) {
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
			if err != nil || got.WorkerSettings.DefaultModel != "selected-model" || got.MockWorkers.MockWorkers[0].ID != "selected-mock" {
				t.Fatalf("selected resume facts = %#v, %v", got, err)
			}
			got.WorkerSettings.DefaultModel = "mutated"
			got.MockWorkers.MockWorkers[0].ID = "mutated"
			again, err := scope.ResumeRuntimeScope(project)
			if err != nil || again.WorkerSettings.DefaultModel != "selected-model" || again.MockWorkers.MockWorkers[0].ID != "selected-mock" {
				t.Fatalf("resume facts were not detached: %#v, %v", again, err)
			}
			if _, err := got.WorkerAttemptStarter(context.Background(), workers.ExecuteRequest{}); !errors.Is(err, failure) {
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
	got, err := reader.ResolveSyncPreflightTarget(factorysessions.DefaultSessionID, nil)
	if err != nil || got.Session == nil || got.Session.ID != "first" || !got.Remapped || got.Unresolved {
		t.Fatalf("active default successor = %#v, %v", got, err)
	}
	active.ClearActive()
	got, err = reader.ResolveSyncPreflightTarget(factorysessions.DefaultSessionID, nil)
	if err != nil || got.Session == nil || got.Session.ID != "peer" || !got.Remapped {
		t.Fatalf("current default successor = %#v, %v", got, err)
	}
	// A real default wins over both active and current non-default records.
	first := state.Resolve("first")
	first.IsDefault = true
	state.Registry().Upsert(first, false)
	got, err = reader.ResolveSyncPreflightTarget(factorysessions.DefaultSessionID, nil)
	if err != nil || got.Session == nil || got.Session.ID != "first" || got.Remapped {
		t.Fatalf("real default = %#v, %v", got, err)
	}
	state.Registry().Remove("first")
	for _, selector := range []string{"peer", "missing", "first"} {
		got, err = reader.ResolveSyncPreflightTarget(selector, nil)
		if err != nil || got.Remapped || got.Unresolved || (selector == "peer") != (got.Session != nil) {
			t.Fatalf("after retirement %q = %#v, %v", selector, got, err)
		}
	}
	if reader.BackendScopeID() != "selected-backend" || reader.LogicalSessionKeyID(state.Resolve("peer")) != "logical-peer" || reader.LogicalSessionKeyID(nil) != "" {
		t.Fatal("addressed logical/backend facts changed")
	}
	host := SessionServiceHost(&SessionRuntime{sessionState: state, backendScopeID: reader.backendScope, identity: reader.identity})
	got, err = host.ResolveSyncPreflightTarget("old", &factorydefinitions.FactorySessionLogicalResolveHint{
		BackendScopeID: "selected-backend", LogicalSessionKeyID: "peer",
	})
	if err != nil || got.Session == nil || got.Session.ID != "peer" || !got.Remapped || host.LogicalSessionKeyID(got.Session) != "logical-peer" {
		t.Fatalf("production host routing without gateway = %#v, %v", got, err)
	}
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
