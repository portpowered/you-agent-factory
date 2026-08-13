package internal

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
)

func TestRecordingScopesBeginAppendFlushFinalizeAndClose(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	service := NewService(ledger, NewProjectionService())
	if service == nil {
		t.Fatal("NewService returned nil")
	}
	root := service
	scope, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled: true,
		Scope:   recordings.CanonicalEventScope{FactorySessionID: "scope-session"},
		Target: recordings.RecordingTargetRequest{
			Artifact: "recording://scope-session",
		},
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

	event := scopedScopeEvent("scope-event-1", 0, scope.Status.EventScope)
	invalid := event
	invalid.Payload = "{"
	if _, err := root.AppendRecordingScopeEvent(
		context.Background(),
		recordings.AppendRecordingScopeEventRequest{Scope: scope.Scope, Event: invalid},
	); !errors.Is(err, recordings.ErrInvalidRecordingEvent) {
		t.Fatalf("invalid AppendRecordingScopeEvent error = %v, want ErrInvalidRecordingEvent", err)
	}
	if len(ledger.events) != 0 {
		t.Fatalf("invalid scope append mutated canonical ledger: %#v", ledger.events)
	}
	appended, err := root.AppendRecordingScopeEvent(
		context.Background(),
		recordings.AppendRecordingScopeEventRequest{Scope: scope.Scope, Event: event},
	)
	if err != nil {
		t.Fatalf("AppendRecordingScopeEvent: %v", err)
	}
	if appended.Event.ID != event.ID || appended.Event.Sequence != 0 ||
		appended.Status.AcceptedEvents != 1 {
		t.Fatalf("append result = %#v, want accepted first event", appended)
	}
	if len(ledger.events) != 1 || ledger.events[0].Id != string(event.ID) {
		t.Fatalf("canonical ledger events = %#v, want the scope event", ledger.events)
	}
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: ledger.StreamGenerationID(),
		History:            ledger.CanonicalEvents(),
	}
	subscription, err := root.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: scope.Status.EventScope,
	})
	if err != nil {
		t.Fatalf("SubscribeFrom after scoped append: %v", err)
	}
	observed := subscription.Subscription(context.Background())
	if observed.Kind != recordings.SubscriptionEvent || observed.Event.ID != event.ID ||
		observed.Event.Cursor != appended.Event.Cursor || observed.Event.Payload != event.Payload {
		t.Fatalf("scoped subscription outcome = %#v, want appended event", observed)
	}

	flushed, err := root.FlushRecordingScope(context.Background(), recordings.FlushRecordingScopeRequest{
		Scope: scope.Scope,
	})
	if err != nil || flushed.Status.FlushedThrough == nil {
		t.Fatalf("FlushRecordingScope = (%#v, %v), want durable cursor", flushed, err)
	}

	finishedAt := time.Unix(1_700_000_100, 0).UTC()
	finalized, err := root.FinalizeRecordingScope(
		context.Background(),
		recordings.FinalizeRecordingScopeRequest{Scope: scope.Scope, FinishedAt: finishedAt},
	)
	if err != nil || finalized.Status.State != recordings.RecordingFinalized ||
		finalized.Status.FinalizedAt == nil || !finalized.Status.FinalizedAt.Equal(finishedAt) {
		t.Fatalf("FinalizeRecordingScope = (%#v, %v), want finalized status", finalized, err)
	}
	repeated, err := root.FinalizeRecordingScope(
		context.Background(),
		recordings.FinalizeRecordingScopeRequest{
			Scope: scope.Scope, FinishedAt: finishedAt.Add(time.Hour),
		},
	)
	if err != nil || repeated.Status.FinalizedAt == nil ||
		!repeated.Status.FinalizedAt.Equal(finishedAt) {
		t.Fatalf("repeated FinalizeRecordingScope = (%#v, %v), want first terminal outcome", repeated, err)
	}

	closed, err := root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{
		Scope: scope.Scope,
	})
	if err != nil || !closed.Closed || closed.Status.AcceptedEvents != 1 {
		t.Fatalf("CloseRecordingScope = (%#v, %v), want idempotent close", closed, err)
	}
	repeatedClose, err := root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{
		Scope: scope.Scope,
	})
	if err != nil || !repeatedClose.Closed || !reflect.DeepEqual(repeatedClose.Status, closed.Status) {
		t.Fatalf("repeated CloseRecordingScope = (%#v, %v), want same detached outcome", repeatedClose, err)
	}

	active, err := root.BeginRecordingScope(context.Background(), recordings.BeginRecordingScopeRequest{
		Enabled: true,
		Scope:   recordings.CanonicalEventScope{FactorySessionID: "scope-active-close"},
		Target:  recordings.RecordingTargetRequest{Artifact: "recording://active-close"},
	})
	if err != nil {
		t.Fatalf("active Close BeginRecordingScope: %v", err)
	}
	activeClosed, err := root.CloseRecordingScope(context.Background(), recordings.CloseRecordingScopeRequest{
		Scope:      active.Scope,
		FinishedAt: finishedAt,
	})
	if err != nil || !activeClosed.Closed || activeClosed.Status.State != recordings.RecordingFinalized {
		t.Fatalf("active CloseRecordingScope = (%#v, %v), want implicit finalization", activeClosed, err)
	}
	if _, err := root.AppendRecordingScopeEvent(
		context.Background(),
		recordings.AppendRecordingScopeEventRequest{
			Scope: scope.Scope,
			Event: scopedScopeEvent("scope-event-after-close", 1, scope.Status.EventScope),
		},
	); !errors.Is(err, recordings.ErrRecordingScopeClosed) {
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
