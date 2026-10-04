package internal

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	canonicalledgerwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/canonical_ledger/wire"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	recordinglifecyclewire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/wire"
	replaywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/replay/wire"
)

func TestLifecycleOwnerRejectsInvalidFactsWithoutMutatingAcceptedHistory(t *testing.T) {
	t.Parallel()
	svc := recordinglifecyclewire.NewService(nil, nil, nil, staticRecordingClock{})
	event := recordings.CanonicalEvent{
		ID: "canonical-lifecycle-event", FactoryTick: 0,
		Scope:      recordings.CanonicalEventScope{FactorySessionID: "session-canonical"},
		Cursor:     recordings.CanonicalEventCursor{StreamGenerationID: "gen-1", Sequence: 0},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Kind:       "FACTORY_STATE_RESPONSE", Payload: `{"state":"RUNNING"}`,
	}
	bound, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-canonical", Artifact: "artifact:canonical", Scope: event.Scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	assertInvalidCanonicalRecordingEventsDoNotMutate(t, svc, bound.Status, event)
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID, Event: event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent valid fact after rejection: %v", err)
	}
	finishedAt := time.Unix(1_700_000_001, 0).UTC()
	finished, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID, FinishedAt: finishedAt,
	})
	if err != nil || finished.Status.State != recordings.RecordingFinalized {
		t.Fatalf("FinishRecording = (%#v, %v), want finalized", finished, err)
	}
	snapshot, err := svc.Snapshot(bound.Status.RecordingID)
	if err != nil || !reflect.DeepEqual(snapshot.Events, []recordings.CanonicalEvent{event}) {
		t.Fatalf("Snapshot = (%#v, %v), want only the admitted fact", snapshot, err)
	}
	if snapshot.Status.AcceptedEvents != 1 || snapshot.Status.LastEvent == nil ||
		*snapshot.Status.LastEvent != event.Cursor || snapshot.Status.FinalizedAt == nil ||
		!snapshot.Status.FinalizedAt.Equal(finishedAt) {
		t.Fatalf("finalized status = %#v, want accepted cursor and exact finish time", snapshot.Status)
	}
}

func assertInvalidCanonicalRecordingEventsDoNotMutate(
	t *testing.T,
	svc recordinglifecycle.Service,
	status recordings.RecordingStatusFacts,
	valid recordings.CanonicalEvent,
) {
	t.Helper()
	tests := map[string]func(*recordings.CanonicalEvent){
		"missing identity":    func(event *recordings.CanonicalEvent) { event.ID = "" },
		"whitespace identity": func(event *recordings.CanonicalEvent) { event.ID = " " },
		"missing kind":        func(event *recordings.CanonicalEvent) { event.Kind = "" },
		"whitespace kind":     func(event *recordings.CanonicalEvent) { event.Kind = " " },
		"missing timestamp": func(event *recordings.CanonicalEvent) {
			event.RecordedAt = time.Time{}
		},
		"invalid JSON": func(event *recordings.CanonicalEvent) { event.Payload = "{" },
	}
	for name, mutate := range tests {
		event := valid
		mutate(&event)
		if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: status.RecordingID,
			Event:       event,
		}); !errors.Is(err, recordings.ErrInvalidRecordingEvent) {
			t.Fatalf("%s error = %v, want ErrInvalidRecordingEvent", name, err)
		}
		got := recordingLifecycleStatus(t, svc, status.RecordingID)
		if got.AcceptedEvents != status.AcceptedEvents || got.LastEvent != nil {
			t.Fatalf("%s mutated recording status: %#v", name, got)
		}
	}
}

func TestReplayOwnerRejectsMalformedCanonicalEvents(t *testing.T) {
	t.Parallel()

	valid := recordings.ReplayRecordingFacts{
		RecordingID: "recording-canonical-replay",
		Events: []recordings.CanonicalEvent{
			replayStateEvent(0, `{"state":"RUNNING"}`),
			replayStateEvent(1, `{"state":"COMPLETED"}`),
		},
	}
	for name, mutate := range malformedReplayRecordingMutations() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc := replaywire.NewService(nil, &plainReplayProjection{}, nil, nil)
			corrupt := cloneReplayRecording(valid)
			mutate(&corrupt)
			result, err := svc.CreateReplayPlan(replayPlanRequest(corrupt))
			if !errors.Is(err, recordings.ErrCorruptReplayInput) {
				t.Fatalf("CreateReplayPlan malformed event error = %v, want ErrCorruptReplayInput", err)
			}
			if result != (recordings.CreateReplayPlanResult{}) {
				t.Fatalf("CreateReplayPlan malformed event result = %#v, want no observable plan", result)
			}

			planned, err := svc.CreateReplayPlan(replayPlanRequest(valid))
			if err != nil {
				t.Fatalf("CreateReplayPlan valid event after rejection: %v", err)
			}
			var observed recordings.ObserveReplayResult
			for observation := 0; observation < len(valid.Events); observation++ {
				observed, err = svc.ObserveReplay(recordings.ObserveReplayRequest{
					Plan: planned.Plan.Handle,
				})
				if err != nil {
					t.Fatalf("ObserveReplay valid event after rejection: %v", err)
				}
			}
			if observed.Observation.Kind != recordings.ReplayCompleted {
				t.Fatalf("ObserveReplay valid event after rejection = %#v, want completed", observed)
			}
		})
	}
}

func malformedReplayRecordingMutations() map[string]func(*recordings.ReplayRecordingFacts) {
	return map[string]func(*recordings.ReplayRecordingFacts){
		"missing identity": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].ID = ""
		},
		"whitespace identity": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].ID = "   "
		},
		"missing kind": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].Kind = ""
		},
		"whitespace kind": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].Kind = "   "
		},
		"zero timestamp": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].RecordedAt = time.Time{}
		},
		"whitespace scope": func(recording *recordings.ReplayRecordingFacts) {
			recording.Scope.FactorySessionID = "   "
			recording.Events[0].Scope = recording.Scope
			recording.Events[1].Scope = recording.Scope
		},
		"invalid JSON": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].Payload = `{"incomplete":`
		},
		"missing cursor generation": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].Cursor.StreamGenerationID = ""
		},
		"cursor sequence mismatch": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].Cursor.Sequence++
		},
		"negative sequence": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[0].Sequence = -1
			recording.Events[0].Cursor.Sequence = -1
		},
		"generation mismatch": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[1].Cursor.StreamGenerationID = "other-generation"
		},
		"out of order": func(recording *recordings.ReplayRecordingFacts) {
			recording.Events[1].Sequence = recording.Events[0].Sequence
			recording.Events[1].Cursor.Sequence = recording.Events[1].Sequence
		},
	}
}

func cloneReplayRecording(recording recordings.ReplayRecordingFacts) recordings.ReplayRecordingFacts {
	cloned := recording
	cloned.Events = append([]recordings.CanonicalEvent(nil), recording.Events...)
	return cloned
}

func replayPlanRequest(recording recordings.ReplayRecordingFacts) recordings.CreateReplayPlanRequest {
	return recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     recording,
	}
}

func TestRecordingScopesBeginAppendFlushFinalizeAndClose(t *testing.T) {
	t.Parallel()

	owner, canonical, root, scope, event := beginLifecycleScope(t)
	assertInvalidScopeAppend(t, root, owner, canonical, scope.Scope, event)
	owner.status.AcceptedEvents = 1
	appended := appendLifecycleScopeEvent(t, root, owner, canonical, scope, event)
	canonical.outcomes = []recordings.CanonicalEvent{appended.Event}
	assertLifecycleScopeSubscription(t, root, scope.Status.EventScope, event, appended)
	finishedAt := time.Unix(1_700_000_100, 0).UTC()
	owner.status.LastEvent = &event.Cursor
	owner.status.FlushedThrough = &event.Cursor
	owner.status.State = recordings.RecordingFinalized
	owner.status.FinalizedAt = &finishedAt
	assertLifecycleScopeFinalization(t, root, scope.Scope, finishedAt)
	wantFlush := recordings.FlushRecordingRequest{RecordingID: owner.status.RecordingID}
	wantFinish := []recordings.FinishRecordingRequest{{RecordingID: owner.status.RecordingID, FinishedAt: finishedAt}}
	if owner.flush != wantFlush || !reflect.DeepEqual(owner.finishes, wantFinish) {
		t.Fatalf("selected flush/finish = %#v/%#v, want %#v/%#v", owner.flush, owner.finishes, wantFlush, wantFinish)
	}
	assertLifecycleScopeClose(t, root, owner, scope.Scope, finishedAt)
	if len(owner.finishes) != 2 || owner.finishes[1] != (recordings.FinishRecordingRequest{RecordingID: "active-close", FinishedAt: finishedAt}) {
		t.Fatalf("close finishes = %#v, want one finish per recording", owner.finishes)
	}
	assertClosedScopeRejectsAppend(t, root, scope)
}

type scopeAdapterLifecycle struct {
	*activeRuntimeLifecycle
	flush recordings.FlushRecordingRequest
}

func (owner *scopeAdapterLifecycle) FlushRecording(request recordings.FlushRecordingRequest) (recordings.FlushRecordingResult, error) {
	owner.flush = request
	return recordings.FlushRecordingResult{Status: owner.status}, nil
}

type scopeAdapterCanonical struct {
	liveSubscriptionOwner
	appends []recordings.AppendRecordedEventRequest
}

func (owner *scopeAdapterCanonical) AppendWithValidation(request recordings.AppendRecordedEventRequest, validate func(recordings.CanonicalEvent) error) (recordings.AppendRecordedEventResult, error) {
	owner.appends = append(owner.appends, request)
	if err := validate(request.Event); err != nil {
		return recordings.AppendRecordedEventResult{}, err
	}
	return recordings.AppendRecordedEventResult{Event: request.Event}, nil
}

func beginLifecycleScope(t *testing.T) (*scopeAdapterLifecycle, *scopeAdapterCanonical, recordings.Service, recordings.BeginRecordingScopeResult, recordings.CanonicalEvent) {
	t.Helper()
	owner := &scopeAdapterLifecycle{activeRuntimeLifecycle: &activeRuntimeLifecycle{status: recordings.RecordingStatusFacts{
		RecordingID: "scope-recording", Scope: recordings.CanonicalEventScope{FactorySessionID: "scope-session"}, State: recordings.RecordingActive,
	}}}
	canonical := &scopeAdapterCanonical{}
	root := NewCombinedService(nil, nil, owner, nil, nil, canonical, nil,
		staticRecordingClock{}, logging.NoopLogger{}, nil, nil, nil, nil, nil)
	request := recordings.BeginRecordingScopeRequest{
		Enabled:       true,
		Scope:         owner.status.Scope,
		Target:        recordings.RecordingTargetRequest{Artifact: "recording://scope-session"},
		FlushInterval: time.Second,
	}
	scope, err := root.BeginRecordingScope(context.Background(), request)
	if err != nil {
		t.Fatalf("BeginRecordingScope: %v", err)
	}
	if scope.Scope.IsZero() || scope.Scope.String() == "" {
		t.Fatal("BeginRecordingScope returned a zero opaque scope")
	}
	if scope.Status.State != recordings.RecordingActive {
		t.Fatalf("begin status = %#v, want active", scope.Status)
	}
	want := recordings.StartRecordingRequest{Enabled: true, Scope: request.Scope, Target: request.Target,
		FlushInterval: request.FlushInterval, RecordingID: scopeRecordingID(scope.Scope, request.RecordingID)}
	if !reflect.DeepEqual(owner.started, want) {
		t.Fatalf("start request = %#v, want %#v", owner.started, want)
	}
	return owner, canonical, root, scope, scopedScopeEvent("scope-event-1", 0, scope.Status.EventScope)
}

func assertInvalidScopeAppend(
	t *testing.T,
	root recordings.Service,
	owner *scopeAdapterLifecycle,
	canonical *scopeAdapterCanonical,
	ref recordings.RecordingScopeRef,
	event recordings.CanonicalEvent,
) {
	t.Helper()
	invalid := event
	invalid.Payload = "{"
	if _, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{Scope: ref, Event: invalid}); !errors.Is(err, recordings.ErrInvalidRecordingEvent) {
		t.Fatalf("invalid AppendRecordingScopeEvent error = %v, want ErrInvalidRecordingEvent", err)
	}
	if len(owner.events) != 0 || len(canonical.appends) != 0 {
		t.Fatalf("invalid scope append delegated: lifecycle %#v, canonical %#v", owner.events, canonical.appends)
	}
}

func appendLifecycleScopeEvent(
	t *testing.T,
	root recordings.Service,
	owner *scopeAdapterLifecycle,
	canonical *scopeAdapterCanonical,
	scope recordings.BeginRecordingScopeResult,
	event recordings.CanonicalEvent,
) recordings.AppendRecordingScopeEventResult {
	t.Helper()
	appended, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{Scope: scope.Scope, Event: event})
	if err != nil {
		t.Fatalf("AppendRecordingScopeEvent: %v", err)
	}
	if appended.Event.ID != event.ID || appended.Event.Sequence != 0 || appended.Status.AcceptedEvents != 1 {
		t.Fatalf("append result = %#v, want accepted first event", appended)
	}
	want := []recordings.RecordRecordingEventRequest{{RecordingID: owner.status.RecordingID, Event: event}}
	if !reflect.DeepEqual(owner.events, want) || !reflect.DeepEqual(canonical.appends, []recordings.AppendRecordedEventRequest{{Event: event}}) {
		t.Fatalf("append requests = %#v/%#v, want selected recording and event %#v", owner.events, canonical.appends, want)
	}
	return appended
}

func assertLifecycleScopeSubscription(
	t *testing.T,
	root recordings.Service,
	eventScope recordings.CanonicalEventScope,
	event recordings.CanonicalEvent,
	appended recordings.AppendRecordingScopeEventResult,
) {
	t.Helper()
	subscription, err := root.SubscribeFrom(context.Background(), recordings.SubscribeRequest{Scope: eventScope})
	if err != nil {
		t.Fatalf("SubscribeFrom after scoped append: %v", err)
	}
	canonical := root.(*combinedService).canonicalLedger.(*scopeAdapterCanonical)
	assertLiveSubscriptionRequest(t, &canonical.liveSubscriptionOwner, eventScope, nil)
	observed := subscription.Subscription(context.Background())
	if observed.Kind != recordings.SubscriptionEvent || observed.Event.ID != event.ID || observed.Event.Cursor != appended.Event.Cursor || observed.Event.Payload != event.Payload {
		t.Fatalf("scoped subscription outcome = %#v, want appended event", observed)
	}
}

func assertLifecycleScopeFinalization(t *testing.T, root recordings.Service, ref recordings.RecordingScopeRef, finishedAt time.Time) {
	t.Helper()
	flushed, err := root.FlushRecordingScope(context.Background(), recordings.FlushRecordingScopeRequest{Scope: ref})
	if err != nil || flushed.Status.FlushedThrough == nil {
		t.Fatalf("FlushRecordingScope = (%#v, %v), want durable cursor", flushed, err)
	}
	finalized, err := root.FinalizeRecordingScope(context.Background(), recordings.FinalizeRecordingScopeRequest{Scope: ref, FinishedAt: finishedAt})
	if err != nil || finalized.Status.State != recordings.RecordingFinalized || finalized.Status.FinalizedAt == nil || !finalized.Status.FinalizedAt.Equal(finishedAt) {
		t.Fatalf("FinalizeRecordingScope = (%#v, %v), want finalized status", finalized, err)
	}
	repeated, err := root.FinalizeRecordingScope(context.Background(), recordings.FinalizeRecordingScopeRequest{Scope: ref, FinishedAt: finishedAt.Add(time.Hour)})
	if err != nil || repeated.Status.FinalizedAt == nil || !repeated.Status.FinalizedAt.Equal(finishedAt) {
		t.Fatalf("repeated FinalizeRecordingScope = (%#v, %v), want first terminal outcome", repeated, err)
	}
}

func assertLifecycleScopeClose(t *testing.T, root recordings.Service, owner *scopeAdapterLifecycle, ref recordings.RecordingScopeRef, finishedAt time.Time) {
	t.Helper()
	closed, err := root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{Scope: ref})
	if err != nil || !closed.Closed || closed.Status.AcceptedEvents != 1 {
		t.Fatalf("CloseRecordingScope = (%#v, %v), want idempotent close", closed, err)
	}
	repeated, err := root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{Scope: ref})
	if err != nil || !repeated.Closed || !reflect.DeepEqual(repeated.Status, closed.Status) {
		t.Fatalf("repeated CloseRecordingScope = (%#v, %v), want same detached outcome", repeated, err)
	}
	owner.status = recordings.RecordingStatusFacts{RecordingID: "active-close", Scope: recordings.CanonicalEventScope{FactorySessionID: "scope-active-close"}, State: recordings.RecordingActive}
	active, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{Enabled: true, Scope: owner.status.Scope, Target: recordings.RecordingTargetRequest{Artifact: "recording://active-close"}})
	if err != nil {
		t.Fatalf("active Close BeginRecordingScope: %v", err)
	}
	owner.status.State = recordings.RecordingFinalized
	owner.status.FinalizedAt = &finishedAt
	activeClosed, err := root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{Scope: active.Scope, FinishedAt: finishedAt})
	if err != nil || !activeClosed.Closed || activeClosed.Status.State != recordings.RecordingFinalized {
		t.Fatalf("active CloseRecordingScope = (%#v, %v), want implicit finalization", activeClosed, err)
	}
}

func assertClosedScopeRejectsAppend(t *testing.T, root recordings.Service, scope recordings.BeginRecordingScopeResult) {
	t.Helper()
	if _, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{Scope: scope.Scope, Event: scopedScopeEvent("scope-event-after-close", 1, scope.Status.EventScope)}); !errors.Is(err, recordings.ErrRecordingScopeClosed) {
		t.Fatalf("append after close error = %v, want ErrRecordingScopeClosed", err)
	}
}

// Global event ordering belongs to the canonical owner. Scope adapters separately
// prove selected lifecycle requests; functional tests retain composed isolation.
func TestRecordingScopesPreserveGlobalCanonicalOrderAcrossScopes(t *testing.T) {
	t.Parallel()
	ledger := &stubLedger{}
	owner := canonicalledgerwire.NewService(ledger)
	scopes := []recordings.CanonicalEventScope{{FactorySessionID: "scope-order-a"}, {FactorySessionID: "scope-order-b"}}
	for index, selected := range []int{0, 1, 0} {
		event := scopedScopeEvent(fmt.Sprintf("order-event-%d", index), 0, scopes[selected])
		accepted, err := owner.Append(recordings.AppendRecordedEventRequest{Event: event})
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		if accepted.Event.Sequence != recordings.CanonicalEventSequence(index) || accepted.Event.Cursor.Sequence != recordings.CanonicalEventSequence(index) || accepted.Event.Scope != scopes[selected] || accepted.Event.ID != event.ID {
			t.Fatalf("accepted event = %#v, want selected event at global position %d", accepted.Event, index)
		}
	}
	counts := map[string]int{}
	for _, event := range ledger.CanonicalEvents() {
		if event.Context.SessionID == nil {
			t.Fatalf("retained event lost session identity: %#v", event)
		}
		counts[*event.Context.SessionID]++
	}
	if counts["scope-order-a"] != 2 || counts["scope-order-b"] != 1 {
		t.Fatalf("retained scope counts = %#v, want 2/1", counts)
	}
}

func TestRecordingScopesRejectMalformedForeignStaleAndFinalizedReferences(t *testing.T) {
	t.Parallel()

	eventScope := recordings.CanonicalEventScope{FactorySessionID: "scope-errors"}
	first, ref, owner, _ := newActiveScopeAdapter(eventScope)
	canonical := &activeRuntimeCanonical{}
	first.canonicalLedger = canonical
	_, foreign, _, _ := newActiveScopeAdapter(recordings.CanonicalEventScope{FactorySessionID: "scope-foreign"})
	malformed, _ := (recordings.RecordingScopeRef{}).Parse("malformed")
	issuer, _, _ := strings.Cut(ref.String(), ".")
	stale, _ := (recordings.RecordingScopeRef{}).Parse(issuer + "." + strings.Repeat("a", 32))

	assertScopeQueryError(t, first, recordings.RecordingScopeRef{}, recordings.ErrRecordingScopeInvalid)
	assertScopeQueryError(t, first, malformed, recordings.ErrRecordingScopeInvalid)
	assertScopeQueryError(t, first, stale, recordings.ErrRecordingScopeStale)
	assertScopeQueryError(t, first, foreign, recordings.ErrRecordingScopeForeign)
	if owner.statusRequest.RecordingID != "" {
		t.Fatalf("invalid reference delegated status request: %#v", owner.statusRequest)
	}

	if _, err := first.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
		Scope: ref,
		Event: scopedScopeEvent("bad", 0, eventScope),
	}); err != nil {
		t.Fatalf("baseline append: %v", err)
	}
	finishedAt := time.Unix(1_700_000_200, 0).UTC()
	owner.status.State = recordings.RecordingFinalized
	owner.status.FinalizedAt = &finishedAt
	if _, err := first.FinalizeRecordingScope(context.Background(), recordings.FinalizeRecordingScopeRequest{
		Scope: ref, FinishedAt: finishedAt,
	}); err != nil {
		t.Fatalf("FinalizeRecordingScope: %v", err)
	}
	_, err := first.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
		Scope: ref,
		Event: scopedScopeEvent("after-finalize", 1, eventScope),
	})
	if !errors.Is(err, recordings.ErrRecordingScopeFinalized) ||
		!errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("append after finalize error = %v, want finalized and terminal failures", err)
	}
	want := []recordings.RecordRecordingEventRequest{{RecordingID: owner.status.RecordingID, Event: scopedScopeEvent("bad", 0, eventScope)}}
	if !reflect.DeepEqual(owner.events, want) || canonical.event != want[0].Event {
		t.Fatalf("append requests = %#v/%#v, want only accepted request %#v", owner.events, canonical.event, want)
	}
	wantFinish := []recordings.FinishRecordingRequest{{RecordingID: owner.status.RecordingID, FinishedAt: finishedAt}}
	if !reflect.DeepEqual(owner.finishes, wantFinish) {
		t.Fatalf("finalize requests = %#v, want %#v", owner.finishes, wantFinish)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := first.BeginRecordingScope(canceled, recordings.BeginRecordingScopeRequest{
		Enabled: true,
		Scope:   recordings.CanonicalEventScope{FactorySessionID: "cancelled"},
		Target:  recordings.RecordingTargetRequest{Artifact: "recording://cancelled"},
	})
	if !errors.Is(err, context.Canceled) || !result.Scope.IsZero() {
		t.Fatalf("cancelled BeginRecordingScope = (%#v, %v), want context cancellation", result, err)
	}
	if owner.started.Enabled {
		t.Fatalf("pre-start cancellation delegated start: %#v", owner.started)
	}
}

func TestRecordingScopeAppendForwardsLifecycleRejectionWithoutAcceptedResult(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "atomic-rejection"}
	owner := &activeRuntimeLifecycle{status: recordings.RecordingStatusFacts{
		RecordingID: "rejected-recording", Scope: scope, State: recordings.RecordingActive,
	}}
	lifecycle := &rejectingRecordingEventLifecycle{activeRuntimeLifecycle: owner}
	canonical := &activeRuntimeCanonical{}
	root := NewCombinedService(nil, nil, lifecycle, nil, nil, canonical, nil,
		staticRecordingClock{}, logging.NoopLogger{}, nil, nil, nil, nil, nil).(*combinedService)
	ref := root.newRecordingScope()
	root.scopeByRef[ref] = &recordingScopeBinding{recordingID: owner.status.RecordingID, eventScope: scope}
	event := scopedScopeEvent("rejected-event", 0, scope)
	result, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
		Scope: ref, Event: event,
	})
	if !errors.Is(err, recordings.ErrRecordingWriteRejected) || !reflect.DeepEqual(result, recordings.AppendRecordingScopeEventResult{}) {
		t.Fatalf("rejected scoped append = (%#v, %v), want no accepted result and typed rejection", result, err)
	}
	want := recordings.RecordRecordingEventRequest{RecordingID: owner.status.RecordingID, Event: event}
	if !reflect.DeepEqual(lifecycle.request, want) || !reflect.DeepEqual(canonical.event, event) {
		t.Fatalf("lifecycle request = %#v, canonical request = %#v, want selected event %#v", lifecycle.request, canonical.event, want)
	}
	status, err := root.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{Scope: ref})
	if err != nil || status.Status.AcceptedEvents != 0 || status.Status.EventScope != scope {
		t.Fatalf("scope after rejected append = (%#v, %v), want intact empty binding", status, err)
	}
}

func TestRecordingScopeCancellationAfterStartRemovesBinding(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner := &activeRuntimeLifecycle{status: recordings.RecordingStatusFacts{
		RecordingID: "cancelled-recording", Scope: recordings.CanonicalEventScope{FactorySessionID: "cancel-after-start"},
		State: recordings.RecordingActive,
	}}
	lifecycle := cancelAfterStartLifecycle{activeRuntimeLifecycle: owner, cancel: cancel}
	finishedAt := time.Unix(1_700_000_300, 0).UTC()
	root := NewCombinedService(nil, nil, lifecycle, nil, nil, nil, nil,
		staticRecordingClock{at: finishedAt}, logging.NoopLogger{}, nil, nil, nil, nil, nil).(*combinedService)
	request := recordings.BeginRecordingScopeRequest{
		Enabled: true, Scope: owner.status.Scope,
		Target: recordings.RecordingTargetRequest{HomeDir: "home"},
	}
	result, err := root.BeginRecordingScope(ctx, request)
	if !errors.Is(err, context.Canceled) || !result.Scope.IsZero() {
		t.Fatalf("post-start canceled BeginRecordingScope = (%#v, %v), want no scope and context.Canceled", result, err)
	}
	if !owner.started.Enabled || owner.started.Scope != request.Scope || owner.started.Target != request.Target || owner.started.RecordingID == "" {
		t.Fatalf("start request = %#v, want selected scope and target", owner.started)
	}
	want := []recordings.FinishRecordingRequest{{RecordingID: owner.status.RecordingID, FinishedAt: finishedAt}}
	if !reflect.DeepEqual(owner.finishes, want) {
		t.Fatalf("abandoned scope cleanup = %#v, want one exact finish %#v", owner.finishes, want)
	}
	if len(root.scopeByRef) != 0 {
		t.Fatalf("post-start cancellation left %d scope bindings", len(root.scopeByRef))
	}
}

func TestRecordingScopesKeepConcurrentSessionsIsolated(t *testing.T) {
	t.Parallel()
	owner := recordinglifecyclewire.NewService(nil, nil, nil, staticRecordingClock{})
	type openedRecording struct {
		id    recordings.RecordingID
		event recordings.CanonicalEvent
	}
	opened := make([]openedRecording, 2)
	for index, sessionID := range []string{"scope-a", "scope-b"} {
		scope := recordings.CanonicalEventScope{FactorySessionID: sessionID}
		bound, err := owner.BindRecording(recordings.BindRecordingRequest{RecordingID: recordings.RecordingID(sessionID), Scope: scope, Artifact: recordings.RecordingArtifactReference("recording://" + sessionID)})
		if err != nil {
			t.Fatalf("BindRecording(%s): %v", sessionID, err)
		}
		opened[index] = openedRecording{id: bound.Status.RecordingID, event: scopedScopeEvent(sessionID+"-event", 0, scope)}
	}
	var wait sync.WaitGroup
	errs := make(chan error, len(opened))
	for _, selected := range opened {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := owner.RecordRecordingEvent(recordings.RecordRecordingEventRequest{RecordingID: selected.id, Event: selected.event})
			if err != nil {
				errs <- err
				return
			}
			if result.Status.Scope != selected.event.Scope || result.Status.AcceptedEvents != 1 {
				errs <- errors.New("concurrent lifecycle crossed its event boundary")
			}
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	for _, selected := range opened {
		snapshot, err := owner.Snapshot(selected.id)
		if err != nil {
			t.Fatalf("Snapshot(%s): %v", selected.id, err)
		}
		if snapshot.Status.Scope != selected.event.Scope || snapshot.Status.AcceptedEvents != 1 || !reflect.DeepEqual(snapshot.Events, []recordings.CanonicalEvent{selected.event}) {
			t.Fatalf("recording %s = %#v, want exactly its own fact", selected.id, snapshot)
		}
	}
}

func assertScopeQueryError(
	t *testing.T,
	service recordings.Service,
	ref recordings.RecordingScopeRef,
	want error,
) {
	t.Helper()
	_, err := service.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{
		Scope: ref,
	})
	if !errors.Is(err, want) {
		t.Fatalf("QueryRecordingScope(%q) error = %v, want %v", ref.String(), err, want)
	}
}

func scopedScopeEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:         recordings.CanonicalEventID(id),
		Sequence:   sequence,
		Scope:      scope,
		Cursor:     recordings.CanonicalEventCursor{StreamGenerationID: "gen-1", Sequence: sequence},
		RecordedAt: time.Unix(1_700_000_000+int64(sequence), 0).UTC(),
		Kind:       recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
		Payload:    `{"type":"FACTORY_REQUEST_BATCH","works":[]}`,
	}
}

type rejectingRecordingEventLifecycle struct {
	*activeRuntimeLifecycle
	request recordings.RecordRecordingEventRequest
}

func (owner *rejectingRecordingEventLifecycle) RecordRecordingEvent(
	request recordings.RecordRecordingEventRequest,
) (recordings.RecordRecordingEventResult, error) {
	owner.request = request
	return recordings.RecordRecordingEventResult{}, recordings.ErrRecordingWriteRejected
}

type cancelAfterStartLifecycle struct {
	*activeRuntimeLifecycle
	cancel context.CancelFunc
}

func (owner cancelAfterStartLifecycle) StartRecording(request recordings.StartRecordingRequest) (recordings.StartRecordingResult, error) {
	result, err := owner.activeRuntimeLifecycle.StartRecording(request)
	owner.cancel()
	return result, err
}

type staticRecordingClock struct {
	at time.Time
}

func (clock staticRecordingClock) Now() time.Time { return clock.at }

// TestRecordingScopeReplayIsEquivalentAndIsolatedUnderConcurrentAccess proves
// canonical replay stays equivalent to the retained projection of the same
// finalized recording scope, and that equivalence survives concurrent access:
// several replays of two distinct scopes run at once and each one completes
// with exactly its own scope's retained world state, never another scope's.
func TestRecordingScopeReplayIsEquivalentAndIsolatedUnderConcurrentAccess(t *testing.T) {
	t.Parallel()

	root := newScopedQueryRoot(t)
	scopes := []finalizedReplayScope{
		newFinalizedReplayScope(t, root, "concurrent-replay-a"),
		newFinalizedReplayScope(t, root, "concurrent-replay-b"),
	}

	var wait sync.WaitGroup
	errs := make(chan error, len(scopes)*8)
	for _, scope := range scopes {
		for range 8 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				replayed, err := replayScopeWorldState(root, scope)
				if err != nil {
					errs <- err
					return
				}
				if !reflect.DeepEqual(replayed, scope.retained) {
					errs <- errors.New("concurrent replay of " + scope.eventScope.FactorySessionID +
						" diverged from its retained projection")
				}
			}()
		}
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if reflect.DeepEqual(scopes[0].retained, scopes[1].retained) {
		t.Fatal("the two recording scopes projected identical world states, so isolation is unobservable")
	}
}

type finalizedReplayScope struct {
	ref        recordings.RecordingScopeRef
	eventScope recordings.CanonicalEventScope
	events     []recordings.CanonicalEvent
	retained   recordings.WorldStateView
}

// newFinalizedReplayScope records two canonical facts for one Factory Session,
// finalizes the recording, opens its scope, and captures the retained
// projection every concurrent replay of that scope must reproduce.
func newFinalizedReplayScope(t *testing.T, root recordings.Service, sessionID string) finalizedReplayScope {
	t.Helper()
	eventScope := recordings.CanonicalEventScope{FactorySessionID: sessionID}
	bound, err := root.BindRecording(recordings.BindRecordingRequest{
		RecordingID: recordings.RecordingID("recording-" + sessionID),
		Artifact:    recordings.RecordingArtifactReference("recording://" + sessionID),
		Scope:       eventScope,
	})
	if err != nil {
		t.Fatalf("BindRecording(%s): %v", sessionID, err)
	}
	events := []recordings.CanonicalEvent{
		scopedScopeEvent(sessionID+"-event-1", 0, eventScope),
		scopedScopeEvent(sessionID+"-event-2", 1, eventScope),
	}
	for index, event := range events {
		if _, err := root.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
			RecordingID: bound.Status.RecordingID, Event: event,
		}); err != nil {
			t.Fatalf("RecordRecordingEvent(%s)[%d]: %v", sessionID, index, err)
		}
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_100, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording(%s): %v", sessionID, err)
	}
	opened, err := root.OpenRecordingScope(context.Background(), recordings.OpenRecordingScopeRequest{
		RecordingID: bound.Status.RecordingID, Scope: eventScope,
	})
	if err != nil {
		t.Fatalf("OpenRecordingScope(%s): %v", sessionID, err)
	}
	retained, err := root.ReconstructRecordingScope(context.Background(), recordings.ReconstructRecordingScopeRequest{
		Scope: opened.Scope, SelectedTick: 4,
	})
	if err != nil {
		t.Fatalf("ReconstructRecordingScope(%s): %v", sessionID, err)
	}
	if retained.WorldState.Scope != eventScope {
		t.Fatalf("retained projection scope = %#v, want %#v", retained.WorldState.Scope, eventScope)
	}
	return finalizedReplayScope{
		ref: opened.Scope, eventScope: eventScope, events: events, retained: retained.WorldState,
	}
}

// replayScopeWorldState drives one complete canonical replay of a finalized
// scope and returns the world state its terminal observation reports.
func replayScopeWorldState(
	root recordings.Service,
	scope finalizedReplayScope,
) (recordings.WorldStateView, error) {
	planned, err := root.CreateReplayPlanScope(context.Background(), recordings.CreateReplayPlanScopeRequest{
		Scope:         scope.ref,
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		SelectedTick:  4,
	})
	if err != nil {
		return recordings.WorldStateView{}, err
	}
	var observed recordings.ObserveReplayScopeResult
	for range scope.events {
		observed, err = root.ObserveReplayScope(context.Background(), recordings.ObserveReplayScopeRequest{
			Scope: scope.ref, Plan: planned.Plan.Handle,
		})
		if err != nil {
			return recordings.WorldStateView{}, err
		}
	}
	if observed.Observation.Kind != recordings.ReplayCompleted {
		return recordings.WorldStateView{}, errors.New("replay of " + scope.eventScope.FactorySessionID +
			" did not complete: " + string(observed.Observation.Kind))
	}
	return observed.Observation.WorldState, nil
}
