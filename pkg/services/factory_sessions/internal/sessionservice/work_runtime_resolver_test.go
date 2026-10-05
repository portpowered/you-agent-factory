package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func newWorkResolverSessionState() *sessionruntime.Service {
	clock := platformclock.Real{}
	newStream := func() *responsestream.SessionResponseStream {
		return responsestream.NewSessionResponseStream(clock)
	}
	return sessionruntime.New(
		sessionregistry.New(),
		responsestream.NewRegistry(newStream, clock),
		nil,
		clock,
		func() string { return "response-event-test-id" },
		func() string { return "session-test-id" },
	)
}

func TestSubmitWorkFileRequiresInjectedReader(t *testing.T) {
	t.Parallel()

	runtime := &SessionRuntime{workFile: "work.json"}
	err := runtime.submitWorkFile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "initial Work file reader is required") {
		t.Fatalf("submitWorkFile missing reader error = %v", err)
	}
}

type replacementRuntimeBuilderFunc func(context.Context, string, string, string, string) (factory.RuntimeRecord, error)

func (builder replacementRuntimeBuilderFunc) BuildReplacement(
	ctx context.Context,
	folderPath string,
	factoryDir string,
	sessionID string,
	executionBaseDir string,
) (factory.RuntimeRecord, error) {
	return builder(ctx, folderPath, factoryDir, sessionID, executionBaseDir)
}

type replacementRuntimeRecord struct {
	factory.RuntimeRecord
	modelsScope models.RuntimeScopeRef
}

func (record *replacementRuntimeRecord) BindModelsRuntimeScope(scope models.RuntimeScopeRef) error {
	record.modelsScope = scope
	return nil
}

func TestBuildReplacementBindsModelsScopeForLocalModelRuntime(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("session-models-scope")
	if err != nil {
		t.Fatalf("parse Models scope: %v", err)
	}
	replacement := &replacementRuntimeRecord{}
	runtime := &SessionRuntime{
		modelsScope: scope,
		runtimeBuild: replacementRuntimeBuilderFunc(func(
			context.Context, string, string, string, string,
		) (factory.RuntimeRecord, error) {
			return replacement, nil
		}),
	}

	got, err := runtime.buildReplacementFactoryRuntime(
		context.Background(), "folder", "factory", "session",
	)
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime() error = %v", err)
	}
	if got != replacement {
		t.Fatalf("replacement record = %p, want %p", got, replacement)
	}
	if replacement.modelsScope != scope {
		t.Fatalf("replacement Models scope = %q, want opened scope %q", replacement.modelsScope, scope)
	}
}

type registeredWorkRuntime struct {
	factory.Service
}

func TestServiceRoutesWorkThroughRegisteredSessionRuntime(t *testing.T) {
	registered := &registeredWorkRuntime{}
	state := newWorkResolverSessionState()
	state.Register(sessionruntime.Registration{
		SessionID: "session-1",
		Handle:    struct{}{},
		Select:    true,
		Runtime: &factorysessions.LiveRuntime{
			Factory:        registered,
			BackendScopeID: "backend-1",
		},
	})
	assembly := &Assembly{state: state}

	resolved, err := assembly.ResolveWorkRuntime("session-1")
	if err != nil {
		t.Fatalf("ResolveWorkRuntime: %v", err)
	}
	if resolved == nil {
		t.Fatal("resolved Work runtime is nil")
	}
	session := assembly.Resolve("session-1")
	if session == nil || session.Runtime == nil ||
		session.Runtime.BackendScopeID != "backend-1" {
		t.Fatalf("resolved session = %#v, want registered backend scope", session)
	}
}

func TestBindRuntimePublishesOpaqueServiceToSession(t *testing.T) {
	t.Parallel()

	state := newWorkResolverSessionState()
	fallback := &registeredWorkRuntime{}
	bound := &registeredWorkRuntime{}
	state.Register(sessionruntime.Registration{
		SessionID: "session-bound",
		Handle:    struct{}{},
		Runtime: &factorysessions.LiveRuntime{
			Factory: fallback,
		},
	})

	runtime := &SessionRuntime{sessionState: state, scopeActivation: NewScopeActivation(state), openingSession: state.Resolve("session-bound")}
	binding := factory.RuntimeBinding{}.New("runtime-bound", bound)
	if err := runtime.BindRuntime("session-bound", binding); err != nil {
		t.Fatalf("BindRuntime: %v", err)
	}
	registered := state.Resolve("session-bound")
	if registered == nil || registered.Runtime == nil {
		t.Fatal("bound session runtime is unavailable")
	}
	if !registered.Runtime.Binding.Equal(binding) {
		t.Fatal("session did not retain the published opaque binding")
	}
	if got := registered.Runtime.Factory; got != bound {
		t.Fatalf("session Factory = %p, want bound Runtime service %p", got, bound)
	}
}

func TestSessionScopeActivationPreservesReplacementAndRetriesFailedPublication(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	activation := NewScopeActivation(state)
	register := func(id string, service factory.Service) *livesession.LiveSession {
		state.Register(sessionruntime.Registration{SessionID: id, Handle: struct{}{}, Runtime: &factorysessions.LiveRuntime{Factory: service}})
		return state.Resolve(id)
	}
	old := register("a", &scopedControlRuntime{status: "RUNNING"})
	peer := register("b", &scopedControlRuntime{status: "PAUSED"})
	replacement := register("a", &scopedControlRuntime{status: "PAUSED"})
	oldScope := SessionScope{Session: old, Binding: factory.RuntimeBinding{}.New("old", &scopedControlRuntime{status: "COMPLETED"})}
	if err := activation.Activate(context.Background(), oldScope); !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("stale activation = %v, want unavailable", err)
	}
	assertStatus := func(session *livesession.LiveSession, want string) {
		t.Helper()
		observed, err := session.Runtime.Factory.Observe(context.Background(), factory.ObserveRequest{Scope: factory.ObservationScopeHealth})
		if err != nil || string(observed.Observation.Health.FactoryState) != want {
			t.Fatalf("session %s status = %s, error = %v, want %s", session.ID, observed.Observation.Health.FactoryState, err, want)
		}
	}
	assertStatus(replacement, "PAUSED")
	assertStatus(peer, "PAUSED")
	if !old.Runtime.Binding.IsZero() {
		t.Fatal("failed publication modified the retired registration")
	}
	currentScope := SessionScope{Session: replacement, Binding: factory.RuntimeBinding{}.New("new", &scopedControlRuntime{status: "RUNNING"})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := activation.Activate(ctx, currentScope); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled activation = %v", err)
	}
	if err := activation.Activate(context.Background(), SessionScope{Session: replacement}); !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("empty binding activation = %v", err)
	}
	assertStatus(replacement, "PAUSED")
	if err := activation.Activate(context.Background(), currentScope); err != nil {
		t.Fatalf("retry activation: %v", err)
	}
	assertStatus(replacement, "RUNNING")
	assertStatus(peer, "PAUSED")
	// The opening owner's bridge retains its original generation throughout
	// Runtime activation and cannot reselect the replacement by public ID.
	owner := &SessionRuntime{sessionState: state, scopeActivation: activation, openingSession: old}
	if err := owner.BindRuntime("a", oldScope.Binding); !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("stale opening-owner publication = %v", err)
	}
	assertStatus(replacement, "RUNNING")
	assertStatus(peer, "PAUSED")
}

func TestSessionScopeActivationRetiresOnlyAddressedGeneration(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	activation := NewScopeActivation(state)
	registerScopeControlRuntime(state, "a", &scopedControlRuntime{status: "RUNNING"}, nil)
	old := state.Resolve("a")
	registerScopeControlRuntime(state, "b", &scopedControlRuntime{status: "PAUSED"}, nil)
	peer := state.Resolve("b")
	registerScopeControlRuntime(state, "a", &scopedControlRuntime{status: "RUNNING"}, nil)
	current := state.Resolve("a")
	if err := activation.Retire(context.Background(), SessionScope{Session: old}); err != nil {
		t.Fatal(err)
	}
	if state.Resolve("a") != current || state.Resolve("b") != peer {
		t.Fatal("stale retirement removed replacement or peer")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := activation.Retire(ctx, SessionScope{Session: current}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled retirement = %v", err)
	}
	if state.Resolve("a") != current {
		t.Fatal("canceled retirement lost retryable registration")
	}
	for range 2 {
		if err := activation.Retire(context.Background(), SessionScope{Session: current}); err != nil {
			t.Fatalf("retry retirement: %v", err)
		}
	}
	if state.Resolve("a") != nil || state.Resolve("b") != peer {
		t.Fatal("retirement did not remove only its addressed generation")
	}
	result, err := peer.Runtime.Factory.Observe(context.Background(), factory.ObserveRequest{Scope: factory.ObservationScopeHealth})
	if err != nil || string(result.Observation.Health.FactoryState) != "PAUSED" {
		t.Fatalf("peer query after retirement = %+v, %v", result, err)
	}
}

func TestServiceReturnsCanonicalSessionNotFound(t *testing.T) {
	assembly := &Assembly{
		state: newWorkResolverSessionState(),
	}
	_, err := assembly.ResolveWorkRuntime("missing")
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
}

type submitWorkFactory struct {
	factory.Service
	request work.WorkRequest
	result  work.WorkRequestSubmitResult
	err     error
}

func (f *submitWorkFactory) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	f.request = request
	return f.result, f.err
}

func (f *submitWorkFactory) SubscribeFactoryEvents(
	context.Context,
	*interfaces.FactoryEventReconnectCursor,
	interfaces.FactoryEventReconnectScope,
) (*interfaces.FactoryEventStream, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestWorkRuntimeAdapterSubmitWorkRequestDelegatesToCanonicalRuntime(t *testing.T) {
	canonical := &submitWorkFactory{result: work.WorkRequestSubmitResult{RequestID: "request-1"}}
	adapter := workRuntimeAdapter{runtime: canonical, ingress: canonical}

	got, err := adapter.SubmitWorkRequest(context.Background(), work.WorkRequest{RequestID: "request-1"})
	if err != nil {
		t.Fatalf("SubmitWorkRequest() error = %v, want nil", err)
	}
	if got.RequestID != "request-1" || canonical.request.RequestID != "request-1" {
		t.Fatalf("SubmitWorkRequest() = %#v, request = %#v, want delegated round trip", got, canonical.request)
	}
}

type serviceOnlyRuntime struct {
	factory.Service
}

func TestWorkRuntimeAdapterSubmitWorkRequestRejectsServiceOnlyRuntimeSafely(t *testing.T) {
	adapter := workRuntimeAdapter{runtime: serviceOnlyRuntime{}}

	_, err := adapter.SubmitWorkRequest(context.Background(), work.WorkRequest{RequestID: "request-1"})
	if err == nil || !strings.Contains(err.Error(), "Factory Runtime work submission is required") {
		t.Fatalf("SubmitWorkRequest() error = %v, want safe legacy-submission-required error", err)
	}
}

// TestWorkRuntimeAdapterDoesNotRecoverSubmitterFromRuntimeValue is the guard
// that the retired Work projection fallback stays retired. The bound runtime
// value here does serve SubmitWorkRequest, so a type assertion on the runtime
// value would have succeeded; the adapter must instead fail closed because
// Factory Sessions declared no Work and event ingress when it bound the
// runtime.
func TestWorkRuntimeAdapterDoesNotRecoverSubmitterFromRuntimeValue(t *testing.T) {
	canonical := &submitWorkFactory{result: work.WorkRequestSubmitResult{RequestID: "request-1"}}
	adapter := workRuntimeAdapter{runtime: canonical}

	_, err := adapter.SubmitWorkRequest(context.Background(), work.WorkRequest{RequestID: "request-1"})
	if err == nil || !strings.Contains(err.Error(), "Factory Runtime work submission is required") {
		t.Fatalf("SubmitWorkRequest() error = %v, want submission-required error", err)
	}
	if canonical.request.RequestID != "" {
		t.Fatalf(
			"submitted request = %#v, want the runtime value untouched without a declared ingress",
			canonical.request,
		)
	}
}

type conflictingRootRuntime struct {
	factory.Service
}

func (conflictingRootRuntime) ControlMoveWork(
	context.Context,
	factory.MoveWorkRequest,
) (factory.MoveWorkResult, error) {
	return factory.MoveWorkResult{}, factory.ErrMoveWorkRequestConflict
}

func TestWorkRuntimeAdapterMapsRootMoveConflictToWorkContract(t *testing.T) {
	adapter := workRuntimeAdapter{runtime: conflictingRootRuntime{}}
	_, err := adapter.MoveWork(context.Background(), "work-1", "done", work.WorkStateChangeSourceAPI, "request-1")
	if !errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
		t.Fatalf("MoveWork error = %v, want %v", err, work.ErrMoveWorkRequestAlreadyApplied)
	}
}

type detachedMoveRuntime struct {
	factory.Service
	request factory.MoveWorkRequest
}

func (r *detachedMoveRuntime) ControlMoveWork(
	_ context.Context,
	request factory.MoveWorkRequest,
) (factory.MoveWorkResult, error) {
	r.request = request
	return factory.MoveWorkResult{
		WorkID: request.WorkID, WorkTypeID: "story",
		FromState: "draft", ToState: request.StateName,
	}, nil
}

// TestWorkRuntimeAdapterMoveWorkReturnsDetachedStateFacts pins the success half
// of the Work move port: engine identity (place and token ids) ends at this
// adapter and only detached Work state facts cross into Work.
func TestWorkRuntimeAdapterMoveWorkReturnsDetachedStateFacts(t *testing.T) {
	runtime := &detachedMoveRuntime{}
	adapter := workRuntimeAdapter{runtime: runtime}

	got, err := adapter.MoveWork(
		context.Background(), "work-1", "review", work.WorkStateChangeSourceAPI, "move-1",
	)
	if err != nil {
		t.Fatalf("MoveWork() error = %v, want nil", err)
	}
	if runtime.request.WorkID != "work-1" || runtime.request.StateName != "review" ||
		runtime.request.RequestID != "move-1" ||
		runtime.request.Source != factory.WorkMoveSource(work.WorkStateChangeSourceAPI) {
		t.Fatalf("ControlMoveWork request = %#v, want the caller's move forwarded verbatim", runtime.request)
	}
	want := work.OperatorMoveResult{
		WorkID: "work-1", WorkTypeID: "story", FromState: "draft", ToState: "review",
	}
	if got != want {
		t.Fatalf("MoveWork() = %#v, want %#v", got, want)
	}
}

type failingMoveRuntime struct {
	factory.Service
	err error
}

func (r failingMoveRuntime) ControlMoveWork(
	context.Context,
	factory.MoveWorkRequest,
) (factory.MoveWorkResult, error) {
	return factory.MoveWorkResult{}, r.err
}

// TestWorkRuntimeAdapterTranslatesEngineMoveFailuresToWorkSentinels pins the
// failure half of the Work move port: every engine-classified move failure
// crosses into Work as the matching Work-owned sentinel, so Work's transports
// branch only on Work error identity. Unclassified failures pass through.
func TestWorkRuntimeAdapterTranslatesEngineMoveFailuresToWorkSentinels(t *testing.T) {
	opaque := errors.New("engine unavailable")
	checks := []struct {
		name     string
		from     error
		want     error
		wantText string
	}{
		{
			name: "request conflict",
			from: factory.ErrMoveWorkRequestConflict,
			want: work.ErrMoveWorkRequestAlreadyApplied,
			// The conflict sentinel is the one translation that already
			// restated the failure in Work's own operator wording.
			wantText: "operator move request was already applied",
		},
		{
			name: "work not found", from: factory.ErrMoveWorkNotFound,
			want: work.ErrMoveWorkNotFound, wantText: "work not found",
		},
		{
			name: "invalid state", from: factory.ErrMoveWorkInvalidState,
			want: work.ErrMoveWorkInvalidState, wantText: "invalid target state for work type",
		},
		{
			name: "in-flight dispatch", from: factory.ErrMoveWorkInFlightDispatch,
			want: work.ErrMoveWorkInFlightDispatch, wantText: "work is in an active dispatch",
		},
		{
			name: "engine terminated", from: factory.ErrMoveWorkEngineTerminated,
			want: work.ErrMoveWorkEngineTerminated, wantText: "engine has terminated",
		},
		{name: "unclassified failure", from: opaque, want: opaque, wantText: "engine unavailable"},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			adapter := workRuntimeAdapter{runtime: failingMoveRuntime{err: check.from}}

			_, err := adapter.MoveWork(
				context.Background(), "work-1", "done", work.WorkStateChangeSourceAPI, "move-1",
			)
			if !errors.Is(err, check.want) {
				t.Fatalf("MoveWork() error = %v, want %v", err, check.want)
			}
			if err.Error() != check.wantText {
				t.Fatalf("MoveWork() error text = %q, want %q", err.Error(), check.wantText)
			}
		})
	}
}

// TestWorkRuntimeAdapterMoveWorkFailsClosedWithoutRuntime keeps Work's move
// path fail-closed when Factory Sessions bound no runtime for the session.
func TestWorkRuntimeAdapterMoveWorkFailsClosedWithoutRuntime(t *testing.T) {
	adapter := workRuntimeAdapter{}

	_, err := adapter.MoveWork(
		context.Background(), "work-1", "done", work.WorkStateChangeSourceAPI, "move-1",
	)
	if err == nil || !strings.Contains(err.Error(), "Factory Runtime work move is required") {
		t.Fatalf("MoveWork() error = %v, want move-required error", err)
	}
}

type unavailableWorkHistoryRuntime struct {
	factory.Service
	snapshot   *legacysnapshot.Snapshot
	stream     *interfaces.FactoryEventStream
	historyErr error
}

func (runtime *unavailableWorkHistoryRuntime) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}

func (runtime *unavailableWorkHistoryRuntime) SubscribeFactoryEvents(
	context.Context,
	*interfaces.FactoryEventReconnectCursor,
	interfaces.FactoryEventReconnectScope,
) (*interfaces.FactoryEventStream, error) {
	return runtime.stream, runtime.historyErr
}

func (runtime *unavailableWorkHistoryRuntime) GetEngineStateSnapshot(context.Context) (*legacysnapshot.Snapshot, error) {
	return runtime.snapshot, nil
}

func TestWorkRuntimeAdapterFailsClosedWhenAdmissionHistoryUnavailable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		historyErr  error
		withIngress bool
		want        string
	}{
		{name: "missing ingress", want: "admission history is required"},
		{name: "subscription error", historyErr: errors.New("history unavailable"), withIngress: true, want: "history unavailable"},
		{name: "nil stream", withIngress: true, want: "stream is unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &unavailableWorkHistoryRuntime{
				snapshot:   &legacysnapshot.Snapshot{},
				historyErr: test.historyErr,
			}
			adapter := workRuntimeAdapter{sessionID: "session-1", runtime: runtime, clock: platformclock.Real{}}
			if test.withIngress {
				adapter.ingress = runtime
			}
			_, err := adapter.ReadWorkSnapshot(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ReadWorkSnapshot() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestWorkRuntimeAdapterProjectsDetachedPublicWorkIdentityStateAndRelations(t *testing.T) {
	tags := map[string]string{"owner": "docs"}
	previous := []string{"chain-a"}
	token := &workers.Token{
		ID:    "tok-review",
		State: "review",
		Color: workers.Color{
			WorkID: "work-review", WorkTypeID: "story", Name: "Review PRD",
			TraceID: "trace-1", PreviousChainingTraceIDs: previous, Tags: tags,
			Relations: []work.Relation{{Type: work.RelationDependsOn, TargetWorkID: "work-draft", RequiredState: "complete"}},
		},
	}
	net := &factory.Net{
		Places: map[string]*factory.PetriPlace{"story:review": {ID: "story:review", TypeID: "story", State: "review"}},
		WorkTypes: map[string]*factory.WorkType{"story": {
			ID: "story", States: []factory.StateDefinition{{Value: "review", Category: factory.StateCategoryProcessing}},
			ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{Name: "report", Pattern: "{{ .Context.Project }}/{{ .Context.SessionID }}/report.txt"}},
		}},
	}
	got := runtimeWorkItem(token, net, false, map[string]string{"work-draft": "Draft PRD"}, runtimeReadFacts{
		dispatchHistory: []factory.CompletedDispatch{{
			DispatchID: "dispatch-context", Outcome: workers.OutcomeAccepted,
			ExpectedArtifactContext: &work.ExpectedArtifactTemplateContext{Project: "project-7", SessionID: "session-9"},
			ConsumedTokens:          []workers.Token{{ID: token.ID, State: "review", Color: token.Color}},
		}},
	})
	if got.CursorID != "tok-review" || got.WorkID != "work-review" || got.State == nil || got.State.Name != "review" || got.State.Type != work.StateTypeProcessing {
		t.Fatalf("runtimeWorkItem = %#v", got)
	}
	if len(got.Relations) != 1 || got.Relations[0].SourceWorkName != "Review PRD" || got.Relations[0].TargetWorkName != "Draft PRD" {
		t.Fatalf("relations = %#v", got.Relations)
	}
	if len(got.ExpectedArtifacts) != 1 || got.ExpectedArtifacts[0].Pattern != "project-7/session-9/report.txt" ||
		got.ExpectedArtifacts[0].Verification != work.ExpectedArtifactVerificationSatisfied {
		t.Fatalf("expected artifacts = %#v, want recorded context", got.ExpectedArtifacts)
	}
	tags["owner"] = "mutated"
	previous[0] = "mutated"
	if got.Tags["owner"] != "docs" || got.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("projection retained runtime aliases: %#v", got)
	}
}

func TestWorkRuntimeAdapterProjectsDispatchOnlyWorkAsProcessing(t *testing.T) {
	token := &workers.Token{ID: "tok-dispatch", State: "review", Color: workers.Color{WorkID: "work-dispatch", WorkTypeID: "story"}}
	got := runtimeWorkItem(token, &factory.Net{}, true, nil)
	if got.State == nil || got.State.Name != "review" || got.State.Type != work.StateTypeProcessing {
		t.Fatalf("dispatch-only Work state = %#v", got.State)
	}
}

func TestWorkRuntimeAdapterProjectsLatestFailureDetailOnlyForCurrentFailedWork(t *testing.T) {
	token := &workers.Token{ID: "tok-failed", State: "failed", Color: workers.Color{WorkID: "work-failed", WorkTypeID: "story"}}
	net := &factory.Net{WorkTypes: map[string]*factory.WorkType{"story": {ID: "story", States: []factory.StateDefinition{{Value: "failed", Category: factory.StateCategoryFailed}}}}}
	history := []factory.CompletedDispatch{
		{
			DispatchID: "dispatch-old", Outcome: workers.OutcomeFailed,
			FailureDetail:  &workers.FailureDetail{Reason: workers.WorkFailureTypeUnknown, Message: "old failure"},
			ConsumedTokens: []workers.Token{{Color: workers.Color{WorkID: "work-failed"}}},
		},
		{
			DispatchID: "dispatch-latest", Outcome: workers.OutcomeFailed,
			FailureDetail:  &workers.FailureDetail{Reason: workers.WorkFailureTypeInternalServerError, Message: "repository root is dirty"},
			ConsumedTokens: []workers.Token{{Color: workers.Color{WorkID: "work-failed"}}},
		},
	}
	got := runtimeWorkItem(token, net, false, nil, runtimeReadFacts{dispatchHistory: history})
	if got.FailureDetail == nil || got.FailureDetail.Reason != string(workers.WorkFailureTypeInternalServerError) || got.FailureDetail.Message != "repository root is dirty" {
		t.Fatalf("runtimeWorkItem failure detail = %#v, want latest typed failure", got.FailureDetail)
	}

	token.State = "done"
	got = runtimeWorkItem(token, net, false, nil, runtimeReadFacts{dispatchHistory: history})
	if got.FailureDetail != nil {
		t.Fatalf("non-failed Work failure detail = %#v, want nil", got.FailureDetail)
	}
}

func TestWorkRuntimeAdapterRestoresPreviousChainingTraceFromCompletedDispatch(t *testing.T) {
	workID, traceID := "work-failed", "trace-parent"
	token := &workers.Token{
		ID:    "tok-failed",
		State: "failed",
		Color: workers.Color{
			WorkID: workID, WorkTypeID: "story", TraceID: traceID,
			CurrentChainingTraceID: traceID,
		},
	}
	net := &factory.Net{WorkTypes: map[string]*factory.WorkType{
		"story": {ID: "story", States: []factory.StateDefinition{{Value: "failed", Category: factory.StateCategoryFailed}}},
	}}
	history := []factory.CompletedDispatch{{
		DispatchID: "dispatch-failed",
		Outcome:    workers.OutcomeFailed,
		ConsumedTokens: []workers.Token{{
			Color: workers.Color{
				WorkID: workID, WorkTypeID: "story", TraceID: traceID,
				CurrentChainingTraceID: traceID,
			},
		}},
	}}

	got := runtimeWorkItem(token, net, false, nil, runtimeReadFacts{dispatchHistory: history})
	if len(got.PreviousChainingTraceIDs) != 1 || got.PreviousChainingTraceIDs[0] != traceID {
		t.Fatalf("previous chaining trace IDs = %v, want canonical dispatch lineage [%q]", got.PreviousChainingTraceIDs, traceID)
	}
}

func TestWorkRuntimeAdapterDetachesFactorySessionStopSummary(t *testing.T) {
	workID := "work-1"
	summary := &factorysessions.StopSummary{
		SessionID: "session-1", StopKind: factorysessions.StopKindBlocked, WorkID: &workID,
		LatestDispatch: &factorysessions.StopDispatchSummary{DispatchID: "dispatch-1", Status: factorysessions.StopDispatchStatusFailed, FailureDetail: &factorysessions.StopFailureDetail{Message: "provider failed"}},
	}
	got := runtimeWorkStopSummary(summary)
	if got == nil || got.WorkID == nil || *got.WorkID != "work-1" || got.LatestDispatch == nil || got.LatestDispatch.FailureDetail == nil {
		t.Fatalf("runtimeWorkStopSummary = %#v", got)
	}
	summary.LatestDispatch.FailureDetail.Message = "mutated"
	if got.LatestDispatch.FailureDetail.Message != "provider failed" {
		t.Fatalf("projection retained Factory Sessions alias: %#v", got)
	}
}

type admissionProjectionLedger struct {
	mu                sync.Mutex
	events            []recordings.FactoryEvent
	recorders         []func(recordings.FactoryEvent)
	recorderAdds      int
	streamGeneration  string
	recorderStarted   chan struct{}
	recorderStartOnce sync.Once
	allowReplay       chan struct{}
}

func (ledger *admissionProjectionLedger) CanonicalEvents() []recordings.FactoryEvent {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return append([]recordings.FactoryEvent(nil), ledger.events...)
}

func (ledger *admissionProjectionLedger) Subscribe(
	context.Context,
	*recordings.FactoryEventReconnectCursor,
	recordings.FactoryEventReconnectScope,
) (recordings.FactoryEventStream, error) {
	return recordings.FactoryEventStream{}, nil
}

func (ledger *admissionProjectionLedger) StreamGenerationID() string {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.streamGeneration
}

func (ledger *admissionProjectionLedger) AddEventRecorder(recorder func(recordings.FactoryEvent)) {
	ledger.mu.Lock()
	ledger.recorderAdds++
	ledger.recorders = append(ledger.recorders, recorder)
	prefix := append([]recordings.FactoryEvent(nil), ledger.events...)
	ledger.mu.Unlock()
	if ledger.recorderStarted != nil {
		ledger.recorderStartOnce.Do(func() { close(ledger.recorderStarted) })
		<-ledger.allowReplay
	}
	for _, event := range prefix {
		recorder(event)
	}
}

func (ledger *admissionProjectionLedger) AddEventTypeRecorder(func(recordings.FactoryEventType)) {}

func (ledger *admissionProjectionLedger) AppendRecordedEvent(event recordings.FactoryEvent) {
	ledger.mu.Lock()
	ledger.events = append(ledger.events, event)
	recorders := append([]func(recordings.FactoryEvent){}, ledger.recorders...)
	ledger.mu.Unlock()
	for _, recorder := range recorders {
		recorder(event)
	}
}

func (ledger *admissionProjectionLedger) recorderCount() int {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.recorderAdds
}

func TestWorkAdmissionProjectionSeedsAppliesAndRebindsToReplacementLedger(t *testing.T) {
	t.Parallel()

	first := &admissionProjectionLedger{}
	first.AppendRecordedEvent(admissionProjectionEvent(t, "event-1", "session-1", 1,
		work.WorkRequestEventWork{Name: "first", WorkID: "work-1"},
	))
	projection := newWorkAdmissionProjection("session-1", platformclock.Real{})
	projection.Bind(first)

	assertAdmissions(t, projection.Snapshot(), []work.WorkAdmission{{WorkID: "work-1", Name: "first", Order: 0}})
	if got := first.recorderCount(); got != 1 {
		t.Fatalf("initial recorder registrations = %d, want 1", got)
	}

	first.AppendRecordedEvent(admissionProjectionEvent(t, "event-other", "session-2", 2,
		work.WorkRequestEventWork{Name: "other", WorkID: "work-other"},
	))
	first.AppendRecordedEvent(admissionProjectionEvent(t, "event-2", "session-1", 3,
		work.WorkRequestEventWork{Name: "second", WorkID: "work-2"},
		work.WorkRequestEventWork{Name: "third", WorkID: "work-3"},
	))
	first.AppendRecordedEvent(admissionProjectionEvent(t, "event-2", "session-1", 3,
		work.WorkRequestEventWork{Name: "second", WorkID: "work-2"},
		work.WorkRequestEventWork{Name: "third", WorkID: "work-3"},
	))
	projection.Bind(first)

	assertAdmissions(t, projection.Snapshot(), []work.WorkAdmission{
		{WorkID: "work-1", Name: "first", Order: 0},
		{WorkID: "work-2", Name: "second", Order: 1},
		{WorkID: "work-3", Name: "third", Order: 2},
	})
	if got := first.recorderCount(); got != 1 {
		t.Fatalf("repeated recorder registrations = %d, want 1", got)
	}

	second := &admissionProjectionLedger{}
	second.AppendRecordedEvent(admissionProjectionEvent(t, "event-1", "session-1", 1,
		work.WorkRequestEventWork{Name: "first", WorkID: "work-1"},
	))
	second.AppendRecordedEvent(admissionProjectionEvent(t, "event-2", "session-1", 3,
		work.WorkRequestEventWork{Name: "second", WorkID: "work-2"},
		work.WorkRequestEventWork{Name: "third", WorkID: "work-3"},
	))
	second.AppendRecordedEvent(admissionProjectionEvent(t, "event-4", "session-1", 4,
		work.WorkRequestEventWork{Name: "recovered", WorkID: "work-4"},
	))
	projection.Bind(second)
	assertAdmissions(t, projection.Snapshot(), []work.WorkAdmission{
		{WorkID: "work-1", Name: "first", Order: 0},
		{WorkID: "work-2", Name: "second", Order: 1},
		{WorkID: "work-3", Name: "third", Order: 2},
		{WorkID: "work-4", Name: "recovered", Order: 3},
	})
	if got := second.recorderCount(); got != 1 {
		t.Fatalf("replacement recorder registrations = %d, want 1", got)
	}
	first.AppendRecordedEvent(admissionProjectionEvent(t, "event-stale", "session-1", 5,
		work.WorkRequestEventWork{Name: "stale", WorkID: "work-stale"},
	))
	second.AppendRecordedEvent(admissionProjectionEvent(t, "event-5", "session-1", 5,
		work.WorkRequestEventWork{Name: "current", WorkID: "work-5"},
	))
	assertAdmissions(t, projection.Snapshot(), []work.WorkAdmission{
		{WorkID: "work-1", Name: "first", Order: 0},
		{WorkID: "work-2", Name: "second", Order: 1},
		{WorkID: "work-3", Name: "third", Order: 2},
		{WorkID: "work-4", Name: "recovered", Order: 3},
		{WorkID: "work-5", Name: "current", Order: 4},
	})

	detached := projection.Snapshot()
	detached[0].Name = "mutated"
	if got := projection.Snapshot()[0].Name; got != "first" {
		t.Fatalf("projection admission name = %q after detached mutation, want first", got)
	}
}

func TestWorkAdmissionProjectionSupportsConcurrentSnapshotsAndAppends(t *testing.T) {
	t.Parallel()

	ledger := &admissionProjectionLedger{}
	projection := newWorkAdmissionProjection("session-1", platformclock.Real{})
	projection.Bind(ledger)

	const eventCount = 200
	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for range 250 {
				_ = projection.Snapshot()
			}
		}()
	}
	for index := 0; index < eventCount; index++ {
		ledger.AppendRecordedEvent(admissionProjectionEvent(
			t, "event-"+strconv.Itoa(index), "session-1", index+1,
			work.WorkRequestEventWork{Name: "work", WorkID: "work-" + strconv.Itoa(index)},
		))
	}
	readers.Wait()

	if got := len(projection.Snapshot()); got != eventCount {
		t.Fatalf("projection admissions = %d, want %d", got, eventCount)
	}
}

func admissionProjectionEvent(
	t testing.TB,
	id string,
	sessionID string,
	sequence int,
	items ...work.WorkRequestEventWork,
) recordings.FactoryEvent {
	t.Helper()
	payload, err := json.Marshal(work.WorkRequestEventPayload{
		Type:  work.WorkRequestTypeFactoryRequestBatch,
		Works: items,
	})
	if err != nil {
		t.Fatalf("marshal Work admission event: %v", err)
	}
	return recordings.FactoryEvent{
		Id:      id,
		Type:    interfaces.FactoryEventTypeWorkRequest,
		Payload: payload,
		Context: recordings.FactoryEventContext{
			Sequence: sequence,
			SessionID: func() *string {
				if sessionID == "" {
					return nil
				}
				return &sessionID
			}(),
		},
	}
}

func assertAdmissions(t testing.TB, got, want []work.WorkAdmission) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("admissions = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("admission[%d] = %#v, want %#v", index, got[index], want[index])
		}
	}
}

func TestAnnotateRuntimeWorkStateSequencesUsesLatestCanonicalStateFact(t *testing.T) {
	t.Parallel()

	workIDs := []string{"work-live"}
	ledger := &admissionProjectionLedger{
		streamGeneration: "generation-live",
		events: []recordings.FactoryEvent{
			{
				Type:    interfaces.FactoryEventTypeWorkRequest,
				Context: interfaces.FactoryEventContext{Sequence: 2, WorkIDs: &workIDs},
				Payload: []byte(`{"works":[{"workId":"work-live"}]}`),
			},
			{
				Type:    interfaces.FactoryEventTypeWorkStateChange,
				Context: interfaces.FactoryEventContext{Sequence: 5, WorkIDs: &workIDs},
				Payload: []byte(`{"workId":"work-live"}`),
			},
		},
	}
	snapshot := work.ReadSnapshot{Items: []work.ReadModel{{WorkID: "work-live"}}}

	annotateRuntimeWorkStateSequences(&snapshot, ledger)

	if snapshot.StreamGenerationID != "generation-live" {
		t.Fatalf("stream generation = %q, want generation-live", snapshot.StreamGenerationID)
	}
	if len(snapshot.Items) != 1 || !snapshot.Items[0].CurrentStateSequenceKnown || snapshot.Items[0].CurrentStateSequence != 5 {
		t.Fatalf("runtime state cursor = %#v, want known sequence 5", snapshot.Items)
	}
}

type gatewayHistoryStub struct {
	request factorysessions.ListSessionsRequest
	result  factorysessions.ListSessionsResult
	err     error
}

func (h *gatewayHistoryStub) ListSessions(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	h.request = request
	return h.result, h.err
}

type gatewayHistoryHost struct {
	LegacyHost
	durable durableexecution.Service
}

func (h gatewayHistoryHost) DurableExecution() durableexecution.Service { return h.durable }

func newGatewayHistoryFixture(history RecordedHistory, durable durableexecution.Service) *Service {
	host := gatewayHistoryHost{durable: durable}
	return NewWithLiveChangeCoordinator(host, stream.NewManagerWithDependencies(host, host, &responsestream.Registry{}), nil, nil, nil, nil, history, durable)
}

func TestServiceListSessionsUsesInjectedRecordedHistoryForHistoryScope(t *testing.T) {
	t.Parallel()
	history := &gatewayHistoryStub{result: factorysessions.ListSessionsResult{
		Scope: factorysessions.SessionListScopeHistory,
		RecordedSessions: []factorysessions.RecordedSessionListSummary{{
			SessionID: "recorded-session", Source: factorysessions.RecordedSessionListSourceHistory,
			ArtifactReference: "2026/08/24/recorded-session.jsonl", Format: factorysessions.RecordedSessionListFormatV2JSONL,
		}},
	}}
	service := newGatewayHistoryFixture(history, nil)

	result, err := service.ListSessions(context.Background(), factorysessions.ListSessionsRequest{
		Scope: factorysessions.SessionListScopeHistory,
	})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if history.request.Scope != factorysessions.SessionListScopeHistory {
		t.Fatalf("history request scope = %q, want history", history.request.Scope)
	}
	if len(result.RecordedSessions) != 1 || result.RecordedSessions[0].SessionID != "recorded-session" {
		t.Fatalf("recorded sessions = %#v, want injected history row", result.RecordedSessions)
	}
	history.err = errors.New("injected recorded history read failure")
	if _, err := service.ListSessions(context.Background(), history.request); !errors.Is(err, history.err) {
		t.Fatalf("history failure = %v", err)
	}
}

type gatewayHistoryDurableStub struct {
	durableexecution.Service
	request factorysessions.ListSessionsRequest
	result  factorysessions.ListSessionsResult
	err     error
}

func (d *gatewayHistoryDurableStub) ListSessions(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	d.request = request
	return d.result, d.err
}

func TestInjectedGatewayHistoryPreservesMergedOrderingAndFailures(t *testing.T) {
	t.Parallel()
	history := &gatewayHistoryStub{result: factorysessions.ListSessionsResult{
		RecordedSessions: []factorysessions.RecordedSessionListSummary{
			{SessionID: "second", ArtifactReference: "a"}, {SessionID: "first", ArtifactReference: "a"},
		},
	}}
	durable := &gatewayHistoryDurableStub{result: factorysessions.ListSessionsResult{
		Scope:            factorysessions.SessionListScopeAll,
		DurableSessions:  []factorysessions.DurableSessionListSummary{{SessionID: "persisted"}},
		RecordedSessions: []factorysessions.RecordedSessionListSummary{{SessionID: "first", ArtifactReference: "b"}},
	}}
	service := newGatewayHistoryFixture(history, durable)
	request := factorysessions.ListSessionsRequest{Scope: factorysessions.SessionListScopeAll,
		Filters: factorysessions.SessionListFilters{Statuses: []factorysessions.LifecycleStatus{factorysessions.LifecycleStatusPaused}, ProjectBoundary: "selected-project"}}
	result, err := service.ListSessions(context.Background(), request)
	if err != nil || result.Scope != request.Scope || len(result.DurableSessions) != 1 || result.DurableSessions[0].SessionID != "persisted" {
		t.Fatalf("combined sessions = %#v, %v", result, err)
	}
	want := []factorysessions.RecordedSessionListSummary{
		{SessionID: "first", ArtifactReference: "a"}, {SessionID: "first", ArtifactReference: "b"},
		{SessionID: "second", ArtifactReference: "a"},
	}
	if !reflect.DeepEqual(result.RecordedSessions, want) || !reflect.DeepEqual(durable.request, request) || history.request.Scope != factorysessions.SessionListScopeHistory || !reflect.DeepEqual(history.request.Filters, request.Filters) {
		t.Fatalf("merged history = %#v; durable/history requests = %#v/%#v", result.RecordedSessions, durable.request, history.request)
	}
	history.err = errors.New("injected history failure")
	if result, err := service.ListSessions(context.Background(), request); !errors.Is(err, history.err) || len(result.DurableSessions) != 0 {
		t.Fatalf("history failure returned partial success = %#v, %v", result, err)
	}
	durable.err = errors.New("injected durable failure")
	history.request = factorysessions.ListSessionsRequest{}
	if _, err := service.ListSessions(context.Background(), request); !errors.Is(err, durable.err) || history.request.Scope != "" {
		t.Fatalf("durable failure = %v; history request = %#v", err, history.request)
	}
}

func TestInjectedGatewayHistoryKeepsScopeExclusionPolicy(t *testing.T) {
	t.Parallel()
	for _, scope := range []factorysessions.SessionListScope{"", factorysessions.SessionListScopeLive,
		factorysessions.SessionListScopePersisted, factorysessions.SessionListScopeHistory, factorysessions.SessionListScopeAll} {
		t.Run(string(scope), func(t *testing.T) {
			t.Parallel()
			history := &gatewayHistoryStub{err: errors.New("history must not be selected")}
			durable := &gatewayHistoryDurableStub{}
			service := newGatewayHistoryFixture(history, durable)
			request := factorysessions.ListSessionsRequest{Scope: scope, ExcludeRecordedHistory: true}
			if _, err := service.ListSessions(context.Background(), request); err != nil || history.request.Scope != "" {
				t.Fatalf("excluded history = %v, request = %#v", err, history.request)
			}
			if scope == "" {
				request.Scope = factorysessions.DefaultSessionListScope
			}
			if !reflect.DeepEqual(durable.request, request) {
				t.Fatalf("durable request = %#v, want %#v", durable.request, request)
			}
		})
	}
}

// A default Factory Session is addressable as ~default and by its exact runtime
// ID. Its WORK_REQUEST events carry one of those forms, so a Work read through
// either selector must see the same admissions (and therefore payloads).
func TestWorkAdmissionsAreSelectorIndependentForDefaultSession(t *testing.T) {
	t.Parallel()

	const exactID = "8cc3988b-717c-4b3c-8710-25d0204a225e"
	session := &livesession.LiveSession{ID: "~default", RuntimeFactorySessionID: exactID}
	payload := json.RawMessage(`{"contract":"synthetic"}`)

	for _, selector := range []string{"~default", exactID} {
		for _, stamped := range []string{"~default", exactID} {
			t.Run(selector+" reads events stamped "+stamped, func(t *testing.T) {
				t.Parallel()
				ledger := &admissionProjectionLedger{}
				ledger.AppendRecordedEvent(admissionProjectionEvent(t, "event-1", stamped, 1,
					work.WorkRequestEventWork{Name: "idea", WorkID: "work-1", Payload: payload},
				))
				ledger.AppendRecordedEvent(admissionProjectionEvent(t, "event-other", "other-session", 2,
					work.WorkRequestEventWork{Name: "foreign", WorkID: "work-foreign"},
				))
				projection := newWorkAdmissionProjection(selector, platformclock.Real{})
				projection.sessionAliases = workSessionAliases(session)
				projection.Bind(ledger)

				assertAdmissions(t, projection.Snapshot(), []work.WorkAdmission{
					{WorkID: "work-1", Name: "idea", Payload: string(payload), Order: 0},
				})
			})
		}
	}
}
