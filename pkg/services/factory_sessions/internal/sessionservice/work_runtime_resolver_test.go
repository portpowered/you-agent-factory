package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func newWorkResolverSessionState() *sessionruntime.Service {
	clock := platformclock.Real{}
	newStream := func() *factorysessions.SessionResponseStream {
		return factorysessions.NewSessionResponseStream(clock)
	}
	return sessionruntime.New(
		sessionregistry.New(),
		factorysessions.NewResponseStreamRegistry(newStream, clock),
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
	factory.Factory
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

func TestServiceReturnsCanonicalSessionNotFound(t *testing.T) {
	assembly := &Assembly{
		state: newWorkResolverSessionState(),
	}
	_, err := assembly.ResolveWorkRuntime("missing")
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
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
		Places:    map[string]*factory.PetriPlace{"story:review": {ID: "story:review", TypeID: "story", State: "review"}},
		WorkTypes: map[string]*factory.WorkType{"story": {ID: "story", States: []factory.StateDefinition{{Value: "review", Category: factory.StateCategoryProcessing}}}},
	}
	got := runtimeWorkItem(token, net, false, map[string]string{"work-draft": "Draft PRD"})
	if got.CursorID != "tok-review" || got.WorkID != "work-review" || got.State == nil || got.State.Name != "review" || got.State.Type != work.StateTypeProcessing {
		t.Fatalf("runtimeWorkItem = %#v", got)
	}
	if len(got.Relations) != 1 || got.Relations[0].SourceWorkName != "Review PRD" || got.Relations[0].TargetWorkName != "Draft PRD" {
		t.Fatalf("relations = %#v", got.Relations)
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
