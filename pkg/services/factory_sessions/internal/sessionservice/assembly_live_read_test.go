package service

import (
	"context"
	"errors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type workerSessionsObservationMarker struct{ workersessions.Service }

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
	assembly.registry.Upsert(&livesession.LiveSession{ID: "first", Handle: &runtimebinding.SessionState{WorkerSessions: first}}, true)
	assembly.registry.Upsert(&livesession.LiveSession{ID: "second", Handle: &runtimebinding.SessionState{WorkerSessions: second}}, false)

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
