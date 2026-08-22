package internal

import (
	"context"
	"errors"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCombinedServiceCanonicalAppendCanRecordReplayAndExport(t *testing.T) {
	t.Parallel()

	svc := NewService(&stubLedger{}, NewProjectionService())
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-canonical"}
	appended, err := svc.Append(recordings.AppendRecordedEventRequest{
		Event: recordings.CanonicalEvent{
			ID:          "canonical-lifecycle-event",
			FactoryTick: 0,
			Scope:       scope,
			RecordedAt:  time.Unix(1_700_000_000, 0).UTC(),
			Kind:        "FACTORY_STATE_RESPONSE",
			Payload:     `{"state":"RUNNING"}`,
		},
	})
	if err != nil {
		t.Fatalf("Append canonical lifecycle event: %v", err)
	}
	bound, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-canonical",
		Artifact:    "artifact:canonical",
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording canonical lifecycle: %v", err)
	}
	assertInvalidCanonicalRecordingEventsDoNotMutate(
		t,
		svc,
		bound.Status,
		appended.Event,
	)
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       appended.Event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent appended fact: %v", err)
	}
	if _, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording canonical lifecycle: %v", err)
	}
	loaded, err := svc.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording appended fact: %v", err)
	}
	plan, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     loaded.Recording,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan appended fact: %v", err)
	}
	observed, err := svc.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: plan.Plan.Handle,
	})
	if err != nil || observed.Observation.Kind != recordings.ReplayCompleted {
		t.Fatalf("ObserveReplay appended fact = (%#v, %v)", observed, err)
	}
	built, err := svc.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil || len(built.Artifact.Events) != 1 ||
		built.Artifact.Events[0] != appended.Event {
		t.Fatalf("BuildPortableArtifact appended fact = (%#v, %v)", built, err)
	}
}

func assertInvalidCanonicalRecordingEventsDoNotMutate(
	t *testing.T,
	svc recordings.Service,
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

func TestCombinedServiceRejectsMalformedCanonicalReplayEvents(t *testing.T) {
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

			svc := NewService(&stubLedger{}, NewProjectionService())
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

	ledger, root, scope, event := beginLifecycleScope(t)
	assertInvalidScopeAppend(t, root, ledger, scope.Scope, event)
	appended := appendLifecycleScopeEvent(t, root, ledger, scope, event)
	assertLifecycleScopeSubscription(t, root, scope.Status.EventScope, event, appended)
	finishedAt := time.Unix(1_700_000_100, 0).UTC()
	assertLifecycleScopeFinalization(t, root, scope.Scope, finishedAt)
	assertLifecycleScopeClose(t, root, scope.Scope, finishedAt)
	assertClosedScopeRejectsAppend(t, root, scope)
}

func beginLifecycleScope(t *testing.T) (*stubLedger, recordings.Service, recordings.BeginRecordingScopeResult, recordings.CanonicalEvent) {
	t.Helper()
	ledger := &stubLedger{}
	root := NewService(ledger, NewProjectionService())
	if root == nil {
		t.Fatal("NewService returned nil")
	}
	scope, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled: true,
		Scope:   recordings.CanonicalEventScope{FactorySessionID: "scope-session"},
		Target:  recordings.RecordingTargetRequest{Artifact: "recording://scope-session"},
	})
	if err != nil {
		t.Fatalf("BeginRecordingScope: %v", err)
	}
	if scope.Scope.IsZero() || scope.Scope.String() == "" {
		t.Fatal("BeginRecordingScope returned a zero opaque scope")
	}
	if scope.Status.State != recordings.RecordingActive {
		t.Fatalf("begin status = %#v, want active", scope.Status)
	}
	return ledger, root, scope, scopedScopeEvent("scope-event-1", 0, scope.Status.EventScope)
}

func assertInvalidScopeAppend(
	t *testing.T,
	root recordings.Service,
	ledger *stubLedger,
	ref recordings.RecordingScopeRef,
	event recordings.CanonicalEvent,
) {
	t.Helper()
	invalid := event
	invalid.Payload = "{"
	if _, err := root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{Scope: ref, Event: invalid}); !errors.Is(err, recordings.ErrInvalidRecordingEvent) {
		t.Fatalf("invalid AppendRecordingScopeEvent error = %v, want ErrInvalidRecordingEvent", err)
	}
	if len(ledger.events) != 0 {
		t.Fatalf("invalid scope append mutated canonical ledger: %#v", ledger.events)
	}
}

func appendLifecycleScopeEvent(
	t *testing.T,
	root recordings.Service,
	ledger *stubLedger,
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
	if len(ledger.events) != 1 || ledger.events[0].Id != string(event.ID) {
		t.Fatalf("canonical ledger events = %#v, want the scope event", ledger.events)
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
	ledger := root.(*combinedService).Ledger.(*stubLedger)
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{StreamGenerationID: ledger.StreamGenerationID(), History: ledger.CanonicalEvents()}
	subscription, err := root.SubscribeFrom(context.Background(), recordings.SubscribeRequest{Scope: eventScope})
	if err != nil {
		t.Fatalf("SubscribeFrom after scoped append: %v", err)
	}
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

func assertLifecycleScopeClose(t *testing.T, root recordings.Service, ref recordings.RecordingScopeRef, finishedAt time.Time) {
	t.Helper()
	closed, err := root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{Scope: ref})
	if err != nil || !closed.Closed || closed.Status.AcceptedEvents != 1 {
		t.Fatalf("CloseRecordingScope = (%#v, %v), want idempotent close", closed, err)
	}
	repeated, err := root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{Scope: ref})
	if err != nil || !repeated.Closed || !reflect.DeepEqual(repeated.Status, closed.Status) {
		t.Fatalf("repeated CloseRecordingScope = (%#v, %v), want same detached outcome", repeated, err)
	}
	active, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{Enabled: true, Scope: recordings.CanonicalEventScope{FactorySessionID: "scope-active-close"}, Target: recordings.RecordingTargetRequest{Artifact: "recording://active-close"}})
	if err != nil {
		t.Fatalf("active Close BeginRecordingScope: %v", err)
	}
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

func TestRecordingScopesPreserveGlobalCanonicalOrderAcrossScopes(t *testing.T) {
	t.Parallel()

	service := NewService(&stubLedger{}, NewProjectionService())
	opened := make([]recordings.BeginRecordingScopeResult, 2)
	for index, sessionID := range []string{"scope-order-a", "scope-order-b"} {
		result, err := service.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
			Enabled: true,
			Scope:   recordings.CanonicalEventScope{FactorySessionID: sessionID},
			Target:  recordings.RecordingTargetRequest{Artifact: recordings.RecordingArtifactReference("recording://" + sessionID)},
		})
		if err != nil {
			t.Fatalf("BeginRecordingScope(%s): %v", sessionID, err)
		}
		opened[index] = result
	}
	appendEvent := func(scope recordings.BeginRecordingScopeResult, id string, sequence recordings.CanonicalEventSequence) recordings.AppendRecordingScopeEventResult {
		t.Helper()
		result, err := service.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
			Scope: scope.Scope,
			Event: scopedScopeEvent(id, sequence, scope.Status.EventScope),
		})
		if err != nil {
			t.Fatalf("AppendRecordingScopeEvent(%s): %v", id, err)
		}
		return result
	}

	first := appendEvent(opened[0], "order-a-1", 0)
	second := appendEvent(opened[1], "order-b-1", 1)
	third := appendEvent(opened[0], "order-a-2", 2)
	if first.Event.Sequence != 0 || second.Event.Sequence != 1 || third.Event.Sequence != 2 {
		t.Fatalf("global scope order = (%d, %d, %d), want (0, 1, 2)", first.Event.Sequence, second.Event.Sequence, third.Event.Sequence)
	}
	for index, want := range []int{2, 1} {
		status, err := service.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{Scope: opened[index].Scope})
		if err != nil {
			t.Fatalf("QueryRecordingScope(%d): %v", index, err)
		}
		if status.Status.AcceptedEvents != want {
			t.Fatalf("scope %d accepted events = %d, want %d", index, status.Status.AcceptedEvents, want)
		}
	}
}

func TestRecordingScopesRejectMalformedForeignStaleAndFinalizedReferences(t *testing.T) {
	t.Parallel()

	first := NewService(&stubLedger{}, NewProjectionService())
	second := NewService(&stubLedger{}, NewProjectionService())
	opened, err := first.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled: true,
		Scope:   recordings.CanonicalEventScope{FactorySessionID: "scope-errors"},
		Target:  recordings.RecordingTargetRequest{Artifact: "recording://errors"},
	})
	if err != nil {
		t.Fatalf("BeginRecordingScope: %v", err)
	}
	foreign, err := second.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled: true,
		Scope:   recordings.CanonicalEventScope{FactorySessionID: "scope-foreign"},
		Target:  recordings.RecordingTargetRequest{Artifact: "recording://foreign"},
	})
	if err != nil {
		t.Fatalf("foreign BeginRecordingScope: %v", err)
	}
	malformed, _ := (recordings.RecordingScopeRef{}).Parse("malformed")
	issuer, _, _ := strings.Cut(opened.Scope.String(), ".")
	stale, _ := (recordings.RecordingScopeRef{}).Parse(issuer + "." + strings.Repeat("a", 32))

	assertScopeQueryError(t, first, recordings.RecordingScopeRef{}, recordings.ErrRecordingScopeInvalid)
	assertScopeQueryError(t, first, malformed, recordings.ErrRecordingScopeInvalid)
	assertScopeQueryError(t, first, stale, recordings.ErrRecordingScopeStale)
	assertScopeQueryError(t, first, foreign.Scope, recordings.ErrRecordingScopeForeign)

	if _, err := first.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
		Scope: opened.Scope,
		Event: scopedScopeEvent("bad", 0, opened.Status.EventScope),
	}); err != nil {
		t.Fatalf("baseline append: %v", err)
	}
	if _, err := first.FinalizeRecordingScope(context.Background(), recordings.FinalizeRecordingScopeRequest{
		Scope: opened.Scope, FinishedAt: time.Unix(1_700_000_200, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinalizeRecordingScope: %v", err)
	}
	_, err = first.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
		Scope: opened.Scope,
		Event: scopedScopeEvent("after-finalize", 1, opened.Status.EventScope),
	})
	if !errors.Is(err, recordings.ErrRecordingScopeFinalized) ||
		!errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("append after finalize error = %v, want finalized and terminal failures", err)
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
}

func TestRecordingScopeAppendDoesNotPublishWhenLifecycleRejects(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	root := NewService(ledger, NewProjectionService()).(*combinedService)
	started, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled: true,
		Scope:   recordings.CanonicalEventScope{FactorySessionID: "atomic-rejection"},
		Target:  recordings.RecordingTargetRequest{Artifact: "recording://atomic-rejection"},
	})
	if err != nil {
		t.Fatalf("BeginRecordingScope: %v", err)
	}
	original := root.Service
	root.Service = rejectingRecordingEventLifecycle{Service: original}
	_, err = root.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
		Scope: started.Scope,
		Event: scopedScopeEvent("rejected-event", 0, started.Status.EventScope),
	})
	if !errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("rejected scoped append error = %v, want ErrRecordingWriteRejected", err)
	}
	if len(ledger.events) != 0 {
		t.Fatalf("rejected scoped append published canonical events: %#v", ledger.events)
	}
	binding, ok := root.scopeByRef[started.Scope]
	if !ok {
		t.Fatal("scope binding disappeared after rejected append")
	}
	status, err := original.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: binding.recordingID,
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus: %v", err)
	}
	if status.Status.AcceptedEvents != 0 {
		t.Fatalf("rejected scoped append changed lifecycle events: %#v", status.Status)
	}
}

func TestRecordingScopeCancellationAfterTargetPlanningRemovesBinding(t *testing.T) {
	t.Parallel()

	planner := &blockingRecordingTargetPlanner{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	root := NewServiceWithLifecycleEffects(
		&stubLedger{},
		NewProjectionService(),
		planner,
		nil,
		nil,
		nil,
		staticRecordingClock{at: time.Unix(1_700_000_300, 0).UTC()},
	).(*combinedService)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, beginErr := root.BeginRecordingScope(ctx, recordings.BeginRecordingScopeRequest{
			Enabled: true,
			Scope:   recordings.CanonicalEventScope{FactorySessionID: "cancel-after-start"},
			Target:  recordings.RecordingTargetRequest{HomeDir: "home"},
		})
		result <- beginErr
	}()
	select {
	case <-planner.started:
	case <-time.After(time.Second):
		t.Fatal("recording target planning did not start")
	}
	cancel()
	close(planner.release)
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("post-start canceled BeginRecordingScope error = %v, want context.Canceled", err)
	}
	root.scopeMu.RLock()
	remaining := len(root.scopeByRef)
	root.scopeMu.RUnlock()
	if remaining != 0 {
		t.Fatalf("post-start cancellation left %d scope bindings", remaining)
	}
}

func TestRecordingScopesKeepConcurrentSessionsIsolated(t *testing.T) {
	t.Parallel()

	service := NewService(&stubLedger{}, NewProjectionService())
	type openedScope struct {
		ref   recordings.RecordingScopeRef
		event recordings.CanonicalEvent
	}
	opened := make([]openedScope, 2)
	for index, sessionID := range []string{"scope-a", "scope-b"} {
		started, err := service.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
			Enabled: true,
			Scope:   recordings.CanonicalEventScope{FactorySessionID: sessionID},
			Target: recordings.RecordingTargetRequest{
				Artifact: recordings.RecordingArtifactReference("recording://" + sessionID),
			},
		})
		if err != nil {
			t.Fatalf("BeginRecordingScope(%s): %v", sessionID, err)
		}
		opened[index] = openedScope{
			ref:   started.Scope,
			event: scopedScopeEvent(sessionID+"-event", 0, started.Status.EventScope),
		}
	}

	var wait sync.WaitGroup
	errs := make(chan error, len(opened))
	for _, scope := range opened {
		scope := scope
		wait.Add(1)
		go func() {
			defer wait.Done()
			appended, err := service.AppendRecordingScopeEvent(context.Background(), recordings.AppendRecordingScopeEventRequest{
				Scope: scope.ref, Event: scope.event,
			})
			if err != nil {
				errs <- err
				return
			}
			if appended.Status.EventScope != scope.event.Scope || appended.Status.AcceptedEvents != 1 {
				errs <- errors.New("concurrent recording scope crossed its event boundary")
			}
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	for _, scope := range opened {
		status, err := service.QueryRecordingScope(context.Background(), recordings.QueryRecordingScopeRequest{
			Scope: scope.ref,
		})
		if err != nil {
			t.Fatalf("QueryRecordingScope(%s): %v", scope.event.Scope.FactorySessionID, err)
		}
		if status.Status.EventScope != scope.event.Scope || status.Status.AcceptedEvents != 1 {
			t.Fatalf("scope %s status = %#v, want one isolated event", scope.event.Scope.FactorySessionID, status.Status)
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
	recordinglifecycle.Service
}

func (rejectingRecordingEventLifecycle) RecordRecordingEvent(
	recordings.RecordRecordingEventRequest,
) (recordings.RecordRecordingEventResult, error) {
	return recordings.RecordRecordingEventResult{}, recordings.ErrRecordingWriteRejected
}

type blockingRecordingTargetPlanner struct {
	started chan struct{}
	release chan struct{}
}

func (planner *blockingRecordingTargetPlanner) PlanLiveRecordingTarget(
	recordings.LiveRecordingTargetRequest,
) (recordings.LiveRecordingTarget, error) {
	close(planner.started)
	<-planner.release
	return recordings.LiveRecordingTarget{
		ServicePath:  "recording-target",
		ReportedPath: "recording-target",
	}, nil
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
