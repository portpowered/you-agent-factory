package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
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

	runtime := &SessionRuntime{sessionState: state}
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

type detachedRouterOwnerFake struct {
	factorysessions.Service
	name             string
	invokedSessionID string
	pausedSessionID  string
}

func (fake *detachedRouterOwnerFake) InvokeFactorySession(_ context.Context, sessionID string, _ factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	fake.invokedSessionID = sessionID
	return factorysessions.InvocationResult{SessionID: sessionID}, nil
}

func (fake *detachedRouterOwnerFake) GetFactorySession(_ context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	return factorysessions.SessionProjection{
		Context: factorysessions.ProjectionContext{FactorySessionID: sessionID},
	}, nil
}

func (fake *detachedRouterOwnerFake) PauseLiveFactorySession(_ context.Context, sessionID string, _ factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	fake.pausedSessionID = sessionID
	return factorysessions.LifecycleControlResult{
		Outcome: factorysessions.LifecycleControlOutcomeAccepted,
		Status:  factorysessions.LifecycleStatusPaused,
	}, nil
}

func (fake *detachedRouterOwnerFake) OpenFactorySession(context.Context, factorysessions.LiveControlOpenRequest) (*factorysessions.LiveControlOpenResult, error) {
	return nil, errors.New("not implemented")
}

func (fake *detachedRouterOwnerFake) ListFactorySessions(context.Context) ([]factorysessions.LiveControlListItem, error) {
	return nil, errors.New("not implemented")
}

func (fake *detachedRouterOwnerFake) ResumeLiveFactorySession(context.Context, string, factorysessions.LiveControlRequest) (factorysessions.LiveControlResult, error) {
	return factorysessions.LiveControlResult{}, errors.New("not implemented")
}

func (fake *detachedRouterOwnerFake) CloseFactorySession(context.Context, string) error {
	return errors.New("not implemented")
}

func (fake *detachedRouterOwnerFake) GetFactorySessionResult(context.Context, string) (factory.LiveSessionResult, error) {
	return factory.LiveSessionResult{}, errors.New("not implemented")
}

func (fake *detachedRouterOwnerFake) GetFactorySessionPartialResult(context.Context, string) (factory.PartialSessionResult, error) {
	return factory.PartialSessionResult{}, errors.New("not implemented")
}

func TestDetachedRouterRoutesSessionOperationsToOwningGateway(t *testing.T) {
	first := &detachedRouterOwnerFake{name: "first"}
	second := &detachedRouterOwnerFake{name: "second"}
	assembly := &Assembly{}
	assembly.registerDetachedGateway("session-first", first)
	assembly.registerDetachedGateway("session-second", second)

	operations, err := (&factorysessions.DetachedOperations{}).Bind(assembly)
	if err != nil {
		t.Fatalf("bind detached operations: %v", err)
	}

	for _, test := range []struct {
		name    string
		session string
		owner   *detachedRouterOwnerFake
		other   *detachedRouterOwnerFake
	}{
		{name: "first", session: "session-first", owner: first, other: second},
		{name: "second", session: "session-second", owner: second, other: first},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := operations.Get(context.Background(), factorysessions.SessionGetRequest{
				SessionID: test.session,
				Mode:      factorysessions.SessionOperationModeLive,
			})
			if err != nil {
				t.Fatalf("get session: %v", err)
			}
			if got.Session.SessionID != test.session {
				t.Fatalf("session id = %q, want %q", got.Session.SessionID, test.session)
			}

			otherInvokedBefore := test.other.invokedSessionID
			if _, err := operations.Invoke(context.Background(), factorysessions.SessionInvokeRequest{
				SessionID: test.session,
			}); err != nil {
				t.Fatalf("invoke session: %v", err)
			}
			if test.owner.invokedSessionID != test.session || test.other.invokedSessionID != otherInvokedBefore {
				t.Fatalf("invoke routing = owner %q, other %q", test.owner.invokedSessionID, test.other.invokedSessionID)
			}

			otherPausedBefore := test.other.pausedSessionID
			if _, err := operations.Control(context.Background(), factorysessions.SessionControlRequest{
				SessionID: test.session,
				Mode:      factorysessions.SessionOperationModeLive,
				Operation: factorysessions.SessionControlPause,
			}); err != nil {
				t.Fatalf("pause session: %v", err)
			}
			if test.owner.pausedSessionID != test.session || test.other.pausedSessionID != otherPausedBefore {
				t.Fatalf("pause routing = owner %q, other %q", test.owner.pausedSessionID, test.other.pausedSessionID)
			}
		})
	}

	if _, err := operations.Get(context.Background(), factorysessions.SessionGetRequest{
		SessionID: "missing",
		Mode:      factorysessions.SessionOperationModeLive,
	}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing session error = %v, want ErrSessionNotFound", err)
	}
}

func TestDetachedRouterKeepsConcurrentSessionsIsolated(t *testing.T) {
	first := &detachedRouterOwnerFake{}
	second := &detachedRouterOwnerFake{}
	assembly := &Assembly{}
	assembly.registerDetachedGateway("session-first", first)
	assembly.registerDetachedGateway("session-second", second)
	operations, err := (&factorysessions.DetachedOperations{}).Bind(assembly)
	if err != nil {
		t.Fatalf("bind detached operations: %v", err)
	}

	var wait sync.WaitGroup
	errorsCh := make(chan error, 2)
	for _, sessionID := range []string{"session-first", "session-second"} {
		sessionID := sessionID
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := operations.Invoke(context.Background(), factorysessions.SessionInvokeRequest{SessionID: sessionID})
			errorsCh <- err
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent invoke: %v", err)
		}
	}
	if first.invokedSessionID != "session-first" || second.invokedSessionID != "session-second" {
		t.Fatalf("concurrent routing = first %q, second %q", first.invokedSessionID, second.invokedSessionID)
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

func TestServiceListSessionsUsesBoundRecordedHistoryForHistoryScope(t *testing.T) {
	var got factorysessions.ListSessionsRequest
	service := &Service{}
	service.bindRecordedSessionHistory(func(_ context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
		got = request
		return factorysessions.ListSessionsResult{
			Scope: factorysessions.SessionListScopeHistory,
			RecordedSessions: []factorysessions.RecordedSessionListSummary{{
				SessionID:         "recorded-session",
				Source:            factorysessions.RecordedSessionListSourceHistory,
				ArtifactReference: "2026/08/24/recorded-session.jsonl",
				Format:            factorysessions.RecordedSessionListFormatV2JSONL,
			}},
		}, nil
	})

	result, err := service.ListSessions(context.Background(), factorysessions.ListSessionsRequest{
		Scope: factorysessions.SessionListScopeHistory,
	})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if got.Scope != factorysessions.SessionListScopeHistory {
		t.Fatalf("history request scope = %q, want history", got.Scope)
	}
	if len(result.RecordedSessions) != 1 || result.RecordedSessions[0].SessionID != "recorded-session" {
		t.Fatalf("recorded sessions = %#v, want bound history row", result.RecordedSessions)
	}
}
