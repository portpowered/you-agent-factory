package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
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

func TestWorkRuntimeAdapterSubmitWorkRequestDelegatesToCanonicalRuntime(t *testing.T) {
	canonical := &submitWorkFactory{result: work.WorkRequestSubmitResult{RequestID: "request-1"}}
	adapter := workRuntimeAdapter{runtime: canonical}

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

func TestWorkRuntimeAdapterProjectsDetachedPublicWorkIdentityStateAndRelations(t *testing.T) {
	tags := map[string]string{"owner": "docs"}
	previous := []string{"chain-a"}
	token := &workers.Token{
		ID:      "tok-review",
		PlaceID: "story:review",
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
			ConsumedTokens:          []workers.Token{*token},
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
	token := &workers.Token{ID: "tok-dispatch", PlaceID: "story:review", Color: workers.Color{WorkID: "work-dispatch", WorkTypeID: "story"}}
	got := runtimeWorkItem(token, &factory.Net{}, true, nil)
	if got.State == nil || got.State.Name != "review" || got.State.Type != work.StateTypeProcessing {
		t.Fatalf("dispatch-only Work state = %#v", got.State)
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
