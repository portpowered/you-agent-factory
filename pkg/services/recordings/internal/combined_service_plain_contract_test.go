// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package internal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

type stubLedger struct {
	events          []factorydefinitions.FactoryEvent
	subscribeErr    error
	subscribeScope  factorydefinitions.FactoryEventReconnectScope
	subscribeStream factorydefinitions.FactoryEventStream
}

func (ledger *stubLedger) CanonicalEvents() []factorydefinitions.FactoryEvent {
	out := make([]factorydefinitions.FactoryEvent, len(ledger.events))
	copy(out, ledger.events)
	return out
}

func (ledger *stubLedger) Subscribe(
	_ context.Context,
	_ *factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	ledger.subscribeScope = scope
	if ledger.subscribeErr != nil {
		return factorydefinitions.FactoryEventStream{}, ledger.subscribeErr
	}
	return ledger.subscribeStream, nil
}

func (ledger *stubLedger) StreamGenerationID() string { return "gen-1" }

func (ledger *stubLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (ledger *stubLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {}

func (ledger *stubLedger) AppendRecordedEvent(event factorydefinitions.FactoryEvent) {
	event.Context.Sequence = len(ledger.events)
	ledger.events = append(ledger.events, event)
}

func (ledger *stubLedger) AppendRecordedEventWithValidation(
	event factorydefinitions.FactoryEvent,
	validate func(factorydefinitions.FactoryEvent) error,
) (factorydefinitions.FactoryEvent, error) {
	event.Context.Sequence = len(ledger.events)
	if validate != nil {
		if err := validate(event); err != nil {
			return factorydefinitions.FactoryEvent{}, err
		}
	}
	ledger.events = append(ledger.events, event)
	return event, nil
}

func TestNewServiceRejectsNilDependencies(t *testing.T) {
	t.Parallel()
	if got := NewService(nil, NewProjectionService()); got != nil {
		t.Fatalf("NewService(nil, projection) = %#v, want nil", got)
	}
	if got := NewService(&stubLedger{}, nil); got != nil {
		t.Fatalf("NewService(ledger, nil) = %#v, want nil", got)
	}
}

func TestNewServiceWithLifecycleEffectsUsesProvidedPublicationAndPlanner(t *testing.T) {
	t.Parallel()

	publication, err := NewPortableArtifactPublication(
		os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
	)
	if err != nil {
		t.Fatalf("NewPortableArtifactPublication: %v", err)
	}
	planner := recordings.LiveRecordingTargetPlannerFunc(
		func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
			return recordings.LiveRecordingTarget{ServicePath: "service/path"}, nil
		},
	)
	if got := NewService(&stubLedger{}, NewProjectionService(), planner); got == nil {
		t.Fatal("NewService with planner returned nil")
	}
	if got := NewServiceWithLifecycleEffects(
		&stubLedger{},
		NewProjectionService(),
		planner,
		nil,
		nil,
		publication,
	); got == nil {
		t.Fatal("NewServiceWithLifecycleEffects with publication returned nil")
	}
}

func TestCombinedServicePlainSlices_SuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	ledger := &stubLedger{}
	svc := NewService(ledger, NewProjectionService())
	if svc == nil {
		t.Fatal("NewService returned nil")
	}

	assertAppendSubscribe(t, svc, ledger)
	assertProjectionQuery(t, svc)
	assertRecordingLifecycle(t, svc)
	assertReplay(t, svc)
	assertArtifactExport(t, svc)
}

func TestCombinedServiceRecordingLifecycleAdapterPreservesDetachedOutcomes(t *testing.T) {
	t.Parallel()

	svc := NewService(&stubLedger{}, NewProjectionService())
	lifecycle, ok := svc.(recordings.RecordingLifecycle)
	if !ok {
		t.Fatal("Recordings root does not implement RecordingLifecycle")
	}
	scope := recordings.LifecycleScope{FactorySessionID: "lifecycle-adapter"}
	if _, err := lifecycle.Begin(recordings.BeginRecordingRequest{}); err != nil {
		t.Fatalf("Begin disabled: %v", err)
	}
	if _, err := lifecycle.Begin(recordings.BeginRecordingRequest{
		Enabled:     true,
		RecordingID: "lifecycle-adapter",
		Scope:       scope,
		Artifact:    "artifact://lifecycle-adapter",
	}); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{
		RecordingID: "lifecycle-adapter",
		Event: recordings.LifecycleEvent{
			ID:         "lifecycle-adapter-event",
			Sequence:   0,
			Scope:      scope,
			Cursor:     recordings.LifecycleEventCursor{StreamGenerationID: "generation-1"},
			RecordedAt: time.Unix(1_700_000_600, 0).UTC(),
			Kind:       "WORK_REQUEST",
			Payload:    "{}",
		},
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if _, err := lifecycle.Flush(recordings.FlushLifecycleRequest{RecordingID: "lifecycle-adapter"}); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if _, err := lifecycle.Status(recordings.LifecycleStatusRequest{RecordingID: "lifecycle-adapter"}); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if err := lifecycle.Stop(recordings.StopLifecycleRequest{RecordingID: "lifecycle-adapter"}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	finished, err := lifecycle.Finish(recordings.FinishLifecycleRequest{
		RecordingID: "lifecycle-adapter",
		FinishedAt:  time.Unix(1_700_000_601, 0).UTC(),
	})
	if err != nil || finished.Status.State != recordings.LifecycleStateFinalized {
		t.Fatalf("Finish = (%#v, %v), want finalized detached status", finished, err)
	}

	if _, err := lifecycle.Bind(recordings.BindLifecycleRequest{
		RecordingID: "lifecycle-failure",
		Artifact:    "artifact://lifecycle-failure",
		Scope:       scope,
	}); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	failed, err := lifecycle.RecordFailure(recordings.RecordLifecycleFailureRequest{
		RecordingID: "lifecycle-failure",
		Failure: recordings.LifecycleFailure{
			Code: "adapter-failure", Message: "adapter failure",
		},
	})
	if err != nil || failed.Status.State != recordings.LifecycleStateFailed || len(failed.Status.Failures) != 1 {
		t.Fatalf("RecordFailure = (%#v, %v), want detached failed status", failed, err)
	}
	if _, err := lifecycle.Begin(recordings.BeginRecordingRequest{
		Enabled: true, RecordingID: "missing-adapter-target", Scope: scope,
	}); !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("Begin missing target = %v, want ErrMissingRecordingTarget", err)
	}
}

func assertAppendSubscribe(t *testing.T, svc recordings.Service, ledger *stubLedger) {
	t.Helper()
	assertOrderedAppend(t, svc, ledger)
	assertReconnectSubscription(t, svc, ledger)
}

func assertOrderedAppend(t *testing.T, svc recordings.Service, ledger *stubLedger) {
	t.Helper()
	event := recordings.CanonicalEvent{
		ID:         "evt-1",
		Sequence:   7,
		Scope:      recordings.CanonicalEventScope{FactorySessionID: "session-1"},
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Kind:       recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest),
		Payload:    `{"work":"one"}`,
	}
	assertInvalidAppendsDoNotMutate(t, svc, ledger, event)
	accepted, err := svc.Append(recordings.AppendRecordedEventRequest{Event: event})
	if err != nil {
		t.Fatalf("Append valid event: %v", err)
	}
	if len(ledger.events) != 1 || ledger.events[0].Id != "evt-1" {
		t.Fatalf("Append did not delegate: %#v", ledger.events)
	}
	if ledger.events[0].Context.Sequence != 0 ||
		ledger.events[0].Context.SessionID == nil ||
		*ledger.events[0].Context.SessionID != "session-1" {
		t.Fatalf("Append canonical facts = %#v, want assigned sequence and session scope", ledger.events[0].Context)
	}
	if accepted.Event.Sequence != 0 ||
		accepted.Event.Cursor != (recordings.CanonicalEventCursor{
			StreamGenerationID: "gen-1",
			Sequence:           0,
		}) {
		t.Fatalf("Append accepted event = %#v, want Recordings-assigned ordering", accepted.Event)
	}
}

func assertInvalidAppendsDoNotMutate(
	t *testing.T,
	svc recordings.Service,
	ledger *stubLedger,
	valid recordings.CanonicalEvent,
) {
	t.Helper()
	tests := map[string]func(*recordings.CanonicalEvent){
		"missing identity": func(event *recordings.CanonicalEvent) { event.ID = "" },
		"missing kind":     func(event *recordings.CanonicalEvent) { event.Kind = "" },
		"missing timestamp": func(event *recordings.CanonicalEvent) {
			event.RecordedAt = time.Time{}
		},
		"whitespace scope": func(event *recordings.CanonicalEvent) {
			event.Scope.FactorySessionID = "   "
		},
		"invalid payload": func(event *recordings.CanonicalEvent) {
			event.Payload = `{"incomplete":`
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			event := valid
			mutate(&event)
			if _, err := svc.Append(recordings.AppendRecordedEventRequest{Event: event}); !errors.Is(
				err,
				recordings.ErrInvalidAppendEvent,
			) {
				t.Fatalf("Append invalid event error = %v, want ErrInvalidAppendEvent", err)
			}
			if len(ledger.events) != 0 {
				t.Fatalf("Append invalid event mutated ledger: %#v", ledger.events)
			}
		})
	}
}

func assertReconnectSubscription(t *testing.T, svc recordings.Service, ledger *stubLedger) {
	t.Helper()
	assertSubscribeFailures(t, svc, ledger)
	first := assertScopedRetainedAndReconnect(t, svc, ledger)
	assertScopedLiveDelivery(t, svc, ledger, first.Event.Cursor)
	assertScopedDeliveryGap(t, svc, ledger)
}

func assertSubscribeFailures(t *testing.T, svc recordings.Service, ledger *stubLedger) {
	t.Helper()
	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "   "},
	}); !errors.Is(err, recordings.ErrInvalidSubscribeScope) {
		t.Fatalf("SubscribeFrom whitespace scope = %v, want ErrInvalidSubscribeScope", err)
	}

	ledger.subscribeErr = recordings.ErrReconnectCursorNotFound
	if _, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &recordings.CanonicalEventCursor{StreamGenerationID: "gen-1", Sequence: 0},
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	}); !errors.Is(err, recordings.ErrReconnectCursorExpired) {
		t.Fatalf("SubscribeFrom stale cursor = %v, want ErrReconnectCursorExpired", err)
	}
	ledger.subscribeErr = nil
}

func assertScopedRetainedAndReconnect(
	t *testing.T,
	svc recordings.Service,
	ledger *stubLedger,
) recordings.SubscriptionOutcome {
	t.Helper()
	ledger.subscribeStream = factorydefinitions.FactoryEventStream{
		StreamGenerationID: "gen-1",
		History: []factorydefinitions.FactoryEvent{
			scopedLegacyEvent("session-1/0", 4, 0, "session-1"),
			scopedLegacyEvent("session-2/0", 5, 0, "session-2"),
			scopedLegacyEvent("session-1/1", 6, 1, "session-1"),
		},
	}
	subscribed, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom success path: %v", err)
	}
	first := subscribed.Subscription.Next(context.Background())
	if first.Kind != recordings.SubscriptionEvent || first.Event.ID != "session-1/0" ||
		first.Event.Sequence != 4 {
		t.Fatalf("SubscribeFrom first outcome = %#v, want global scoped event 4", first)
	}
	second := subscribed.Subscription.Next(context.Background())
	if second.Kind != recordings.SubscriptionEvent || second.Event.ID != "session-1/1" ||
		second.Event.Sequence != 6 {
		t.Fatalf("SubscribeFrom interleaved outcome = %#v, want global event 6", second)
	}

	cursor := first.Event.Cursor
	reconnected, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &cursor,
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom reconnect: %v", err)
	}
	if outcome := reconnected.Subscription.Next(context.Background()); outcome.Kind != recordings.SubscriptionEvent ||
		outcome.Event.ID != "session-1/1" {
		t.Fatalf("SubscribeFrom reconnect outcome = %#v, want session-1/1", outcome)
	}
	return first
}

func assertScopedLiveDelivery(
	t *testing.T,
	svc recordings.Service,
	ledger *stubLedger,
	cursor recordings.CanonicalEventCursor,
) {
	t.Helper()
	live := make(chan factorydefinitions.FactoryEvent, 2)
	live <- scopedLegacyEvent("session-2/1", 7, 1, "session-2")
	live <- scopedLegacyEvent("session-1/2", 8, 2, "session-1")
	ledger.subscribeStream.Events = live
	cursor.Sequence = 6
	liveSubscription, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Cursor: &cursor,
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom live: %v", err)
	}
	if outcome := liveSubscription.Subscription.Next(context.Background()); outcome.Kind != recordings.SubscriptionEvent ||
		outcome.Event.ID != "session-1/2" || outcome.Event.Sequence != 8 {
		t.Fatalf("SubscribeFrom interleaved live outcome = %#v, want global event 8", outcome)
	}
}

func assertScopedDeliveryGap(t *testing.T, svc recordings.Service, ledger *stubLedger) {
	t.Helper()
	ledger.subscribeStream.Events = nil
	ledger.subscribeStream.History = []factorydefinitions.FactoryEvent{
		scopedLegacyEvent("session-1/0", 4, 0, "session-1"),
		scopedLegacyEvent("session-2/0", 5, 0, "session-2"),
		scopedLegacyEvent("session-1/2", 8, 2, "session-1"),
	}
	gapped, err := svc.SubscribeFrom(context.Background(), recordings.SubscribeRequest{
		Scope: recordings.CanonicalEventScope{FactorySessionID: "session-1"},
	})
	if err != nil {
		t.Fatalf("SubscribeFrom gap setup: %v", err)
	}
	_ = gapped.Subscription.Next(context.Background())
	gap := gapped.Subscription.Next(context.Background())
	if gap.Kind != recordings.SubscriptionGap || gap.Gap == nil ||
		gap.Gap.Cause != recordings.SubscriptionSequenceDiscontinuity ||
		gap.Gap.ExpectedSequence != 1 || gap.Gap.ObservedSequence != 2 ||
		gap.Gap.ReconnectFrom.Sequence != 4 {
		t.Fatalf("SubscribeFrom discontinuity = %#v, want explicit gap 1 -> 2", gap)
	}
}

func scopedLegacyEvent(
	id string,
	globalSequence int,
	sessionSequence int,
	sessionID string,
) factorydefinitions.FactoryEvent {
	return factorydefinitions.FactoryEvent{
		Id: id,
		Context: factorydefinitions.FactoryEventContext{
			Sequence:        globalSequence,
			SessionID:       &sessionID,
			SessionSequence: &sessionSequence,
		},
	}
}

func assertProjectionQuery(t *testing.T, svc recordings.Service) {
	t.Helper()
	historical, err := svc.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{RecordingID: "unavailable"},
	})
	var historicalErr *recordings.HistoricalRecordingQueryError
	if !errors.As(err, &historicalErr) ||
		historicalErr.Kind != recordings.HistoricalRecordingQueryErrorUnavailable ||
		historical.Recording.RecordingID != "" {
		t.Fatalf("QueryHistoricalRecording without capability = (%#v, %v), want unavailable typed error", historical, err)
	}
	if _, err := svc.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		SelectedTick: -1,
	}); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ReconstructWorldState negative tick = %v, want ErrInvalidProjectionInput", err)
	}
	world, err := svc.ReconstructWorldState(recordings.ReconstructWorldStateRequest{
		Events:       nil,
		SelectedTick: 0,
	})
	if err != nil {
		t.Fatalf("ReconstructWorldState success path: %v", err)
	}
	if _, err := svc.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: world.WorldState,
	}); err != nil {
		t.Fatalf("QuerySimpleDashboard: %v", err)
	}
	if _, err := svc.QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest{
		WorldState: world.WorldState,
	}); err != nil {
		t.Fatalf("QueryWorkstationRequests: %v", err)
	}
	if _, err := svc.QuerySimpleDashboard(recordings.SimpleDashboardQueryRequest{
		WorldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Payload:       "{",
		},
	}); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("QuerySimpleDashboard invalid payload = %v, want ErrInvalidProjectionInput", err)
	}
	if _, err := svc.QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest{
		WorldState: recordings.WorldStateView{SchemaVersion: "unsupported", Payload: "{}"},
	}); !errors.Is(err, recordings.ErrUnsupportedProjectionView) {
		t.Fatalf("QueryWorkstationRequests unsupported view = %v, want ErrUnsupportedProjectionView", err)
	}
	assertReconnectReplayValidation(t, svc)
}

func assertReconnectReplayValidation(t *testing.T, svc recordings.Service) {
	t.Helper()
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-query"}
	history := []recordings.CanonicalEvent{
		canonicalProjectionEvent("query-0", 0, scope),
		canonicalProjectionEvent("query-2", 2, scope),
		canonicalProjectionEvent("query-4", 4, scope),
	}
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: history,
		Cursor: history[1].Cursor,
		Scope:  scope,
	}); err != nil {
		t.Fatalf("ValidateReconnectReplayFrom interleaved scoped history: %v", err)
	}
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: history[1:],
		Cursor: history[0].Cursor,
		Scope:  scope,
	}); !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf(
			"ValidateReconnectReplayFrom continuation-only history = %v, want ErrReconnectCursorNotFound",
			err,
		)
	}
	malformed := append([]recordings.CanonicalEvent(nil), history...)
	malformed[1], malformed[2] = malformed[2], malformed[1]
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: malformed,
		Cursor: malformed[0].Cursor,
		Scope:  scope,
	}); !errors.Is(err, recordings.ErrMalformedProjectionOrder) {
		t.Fatalf("ValidateReconnectReplayFrom malformed order = %v, want ErrMalformedProjectionOrder", err)
	}
	wrongScope := append([]recordings.CanonicalEvent(nil), history...)
	wrongScope[2].Scope = recordings.CanonicalEventScope{FactorySessionID: "other-session"}
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: wrongScope,
		Cursor: wrongScope[0].Cursor,
		Scope:  scope,
	}); !errors.Is(err, recordings.ErrInvalidProjectionScope) {
		t.Fatalf("ValidateReconnectReplayFrom wrong scope = %v, want ErrInvalidProjectionScope", err)
	}
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Events: history,
		Cursor: recordings.CanonicalEventCursor{Sequence: 0},
		Scope:  scope,
	}); !errors.Is(err, recordings.ErrMalformedProjectionOrder) {
		t.Fatalf("ValidateReconnectReplayFrom malformed cursor = %v, want ErrMalformedProjectionOrder", err)
	}
}

func canonicalProjectionEvent(
	id string,
	sequence recordings.CanonicalEventSequence,
	scope recordings.CanonicalEventScope,
) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:       recordings.CanonicalEventID(id),
		Sequence: sequence,
		Scope:    scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "gen-1",
			Sequence:           sequence,
		},
	}
}

func assertRecordingLifecycle(t *testing.T, svc recordings.Service) {
	t.Helper()
	assertRecordingLifecycleBindingCollision(t, svc)
	assertRecordingLifecycleHappyPath(t, svc)
	assertRecordingLifecycleFlushFailure(t, svc)
}

func assertRecordingLifecycleHappyPath(t *testing.T, svc recordings.Service) {
	t.Helper()
	if _, err := svc.BindRecording(recordings.BindRecordingRequest{}); !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("BindRecording empty artifact = %v, want ErrMissingRecordingTarget", err)
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recording"}
	bound, err := svc.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:recording",
		Scope:    scope,
	})
	if err != nil || bound.Status.RecordingID == "" {
		t.Fatalf("BindRecording success = (%#v, %v)", bound, err)
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event: recordings.CanonicalEvent{
			ID: "wrong-scope", Kind: "WORK_REQUEST",
			Scope: recordings.CanonicalEventScope{FactorySessionID: "other-session"},
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: "generation-1",
			},
		},
	}); !errors.Is(err, recordings.ErrInvalidRecordingEvent) {
		t.Fatalf("RecordRecordingEvent wrong scope = %v, want ErrInvalidRecordingEvent", err)
	}
	recorded, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event: recordings.CanonicalEvent{
			ID: "rec-evt-1", Kind: "WORK_REQUEST", Scope: scope,
			RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
			Payload:    "{}",
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: "generation-1",
			},
		},
	})
	if err != nil || recorded.Status.AcceptedEvents != 1 {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}
	if _, err := svc.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	}); err != nil {
		t.Fatalf("FlushRecording: %v", err)
	}
	finished, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_100, 0).UTC(),
	})
	if err != nil || finished.Status.State != recordings.RecordingFinalized {
		t.Fatalf("FinishRecording: %v", err)
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event: recordings.CanonicalEvent{
			ID: "after-finish", Kind: "WORK_REQUEST", Scope: scope, Sequence: 1,
			Cursor: recordings.CanonicalEventCursor{
				StreamGenerationID: "generation-1", Sequence: 1,
			},
		},
	}); !errors.Is(err, recordings.ErrRecordingWriteRejected) {
		t.Fatalf("post-finish write = %v, want ErrRecordingWriteRejected", err)
	}
}

func assertRecordingLifecycleFlushFailure(t *testing.T, svc recordings.Service) {
	t.Helper()
	boundFail, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "flush-fail",
		Artifact:    "artifact:fail",
	})
	if err != nil {
		t.Fatalf("BindRecording flush-fail: %v", err)
	}
	failed, err := svc.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: boundFail.Status.RecordingID,
		Failure: recordings.RecordingFailure{
			Code: "producer_failed", Message: "producer boom",
		},
	})
	if err != nil || failed.Status.State != recordings.RecordingFailed {
		t.Fatalf("RecordRecordingError: %v", err)
	}
	if _, err := svc.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: boundFail.Status.RecordingID,
	}); err != nil {
		t.Fatalf("FlushRecording after failure fact: %v", err)
	}
	status, err := svc.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: boundFail.Status.RecordingID,
	})
	if err != nil || status.Status.State != recordings.RecordingFailed ||
		len(status.Status.Failures) != 1 {
		t.Fatalf("QueryRecordingStatus = (%#v, %v)", status, err)
	}
	if _, err := svc.QueryRecordingStatus(recordings.RecordingStatusRequest{}); !errors.Is(err, recordings.ErrMissingRecordingTarget) {
		t.Fatalf("QueryRecordingStatus missing = %v, want ErrMissingRecordingTarget", err)
	}
}

func assertRecordingLifecycleBindingCollision(t *testing.T, svc recordings.Service) {
	t.Helper()
	assertGeneratedServiceRecordingIDDoesNotCollide(t, svc)
	request := recordings.BindRecordingRequest{
		RecordingID: "stable-recording",
		Artifact:    "artifact:stable",
		Scope: recordings.CanonicalEventScope{
			FactorySessionID: "session-stable",
		},
	}
	bound, err := svc.BindRecording(request)
	if err != nil {
		t.Fatalf("BindRecording stable: %v", err)
	}
	event := recordings.CanonicalEvent{
		ID: "stable-event", Kind: "WORK_REQUEST", Scope: request.Scope,
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(), Payload: "{}",
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-stable",
		},
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent stable: %v", err)
	}
	if _, err := svc.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	}); err != nil {
		t.Fatalf("FlushRecording stable: %v", err)
	}
	active := recordingLifecycleStatus(t, svc, request.RecordingID)
	assertServiceBindingCollision(t, svc, request, active, "active")

	producerErr := errors.New("preserve this failure")
	if _, err := svc.RecordRecordingError(recordings.RecordRecordingErrorRequest{
		RecordingID: request.RecordingID,
		Failure: recordings.RecordingFailure{
			Code: "producer_failed", Message: "preserve this failure",
		},
		Cause: producerErr,
	}); err != nil {
		t.Fatalf("RecordRecordingError stable: %v", err)
	}
	if _, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: request.RecordingID,
		FinishedAt:  time.Unix(1_700_000_100, 0).UTC(),
	}); !errors.Is(err, producerErr) {
		t.Fatalf("FinishRecording stable error = %v, want producer cause", err)
	}
	terminal := recordingLifecycleStatus(t, svc, request.RecordingID)
	if terminal.State != recordings.RecordingFailed || terminal.FinalizedAt == nil {
		t.Fatalf("terminal status = %#v, want finalized failed recording", terminal)
	}
	assertServiceBindingCollision(t, svc, request, terminal, "terminal")
}

func assertGeneratedServiceRecordingIDDoesNotCollide(t *testing.T, svc recordings.Service) {
	t.Helper()
	explicit, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-1",
		Artifact:    "artifact:explicit",
	})
	if err != nil {
		t.Fatalf("BindRecording explicit generated-form ID: %v", err)
	}
	generated, err := svc.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:generated",
	})
	if err != nil {
		t.Fatalf("BindRecording generated ID: %v", err)
	}
	if generated.Status.RecordingID == explicit.Status.RecordingID {
		t.Fatalf("generated RecordingID %q replaced an existing binding", generated.Status.RecordingID)
	}
	got := recordingLifecycleStatus(t, svc, explicit.Status.RecordingID)
	if got.Artifact != explicit.Status.Artifact {
		t.Fatalf("explicit binding after generated bind = %#v, want unchanged", got)
	}
}

func assertServiceBindingCollision(
	t *testing.T,
	svc recordings.Service,
	request recordings.BindRecordingRequest,
	want recordings.RecordingStatusFacts,
	phase string,
) {
	t.Helper()
	rebound, err := svc.BindRecording(request)
	if err != nil || !reflect.DeepEqual(rebound.Status, want) {
		t.Fatalf(
			"BindRecording identical %s = (%#v, %v), want unchanged %#v",
			phase,
			rebound.Status,
			err,
			want,
		)
	}
	conflict := request
	conflict.Artifact = "artifact:other"
	if _, err := svc.BindRecording(conflict); !errors.Is(
		err,
		recordings.ErrRecordingBindingConflict,
	) {
		t.Fatalf(
			"BindRecording conflicting %s = %v, want ErrRecordingBindingConflict",
			phase,
			err,
		)
	}
	got := recordingLifecycleStatus(t, svc, request.RecordingID)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(
			"status after conflicting %s bind = %#v, want unchanged %#v",
			phase,
			got,
			want,
		)
	}
}

func recordingLifecycleStatus(
	t *testing.T,
	svc recordings.Service,
	recordingID recordings.RecordingID,
) recordings.RecordingStatusFacts {
	t.Helper()
	result, err := svc.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		t.Fatalf("QueryRecordingStatus %q: %v", recordingID, err)
	}
	return result.Status
}

func assertReplay(t *testing.T, svc recordings.Service) {
	t.Helper()
	loaded := assertReplayLoadAndCompletion(t, svc)
	assertReplayTypedFailures(t, svc, loaded)
	assertReplayDivergence(t, svc, loaded)
	assertReplayOrderedProgress(t, svc)
}

func assertReplayLoadAndCompletion(
	t *testing.T,
	svc recordings.Service,
) recordings.ReplayRecordingFacts {
	t.Helper()
	if _, err := svc.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing",
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFound) {
		t.Fatalf("LoadReplayRecording missing = %v, want ErrReplayRecordingNotFound", err)
	}
	bound, err := svc.BindRecording(recordings.BindRecordingRequest{
		Artifact: "artifact:replay-service",
	})
	if err != nil {
		t.Fatalf("BindRecording replay = %v", err)
	}
	if _, err := svc.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFinalized) {
		t.Fatalf("LoadReplayRecording active = %v, want ErrReplayRecordingNotFinalized", err)
	}
	if _, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_000, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording replay = %v", err)
	}
	loaded, err := svc.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("LoadReplayRecording = %v", err)
	}
	planned, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     loaded.Recording,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan = %v", err)
	}
	observed, err := svc.ObserveReplay(recordings.ObserveReplayRequest{Plan: planned.Plan.Handle})
	if err != nil || observed.Observation.Kind != recordings.ReplayCompleted {
		t.Fatalf("ObserveReplay = (%#v, %v)", observed, err)
	}
	return loaded.Recording
}

func assertReplayTypedFailures(
	t *testing.T,
	svc recordings.Service,
	recording recordings.ReplayRecordingFacts,
) {
	t.Helper()
	if _, err := svc.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: "missing",
	}); !errors.Is(err, recordings.ErrReplayPlanNotFound) {
		t.Fatalf("ObserveReplay missing = %v, want ErrReplayPlanNotFound", err)
	}
	if _, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: "unsupported",
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     recording,
	}); !errors.Is(err, recordings.ErrUnsupportedReplayPlan) {
		t.Fatalf("CreateReplayPlan unsupported = %v, want ErrUnsupportedReplayPlan", err)
	}
	if _, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording:     recordings.ReplayRecordingFacts{},
	}); !errors.Is(err, recordings.ErrCorruptReplayInput) {
		t.Fatalf("CreateReplayPlan corrupt = %v, want ErrCorruptReplayInput", err)
	}
}

func assertReplayDivergence(
	t *testing.T,
	svc recordings.Service,
	recording recordings.ReplayRecordingFacts,
) {
	t.Helper()
	expected := recordings.CanonicalEventCursor{
		StreamGenerationID: "expected-generation",
		Sequence:           1,
	}
	divergencePlan, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion:   recordings.ReplayPlanSchemaV1,
		Timing:          recordings.ReplayTimingOrderOnly,
		Recording:       recording,
		ExpectedThrough: &expected,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan divergence = %v", err)
	}
	diverged, err := svc.ObserveReplay(recordings.ObserveReplayRequest{
		Plan: divergencePlan.Plan.Handle,
	})
	if err != nil || diverged.Observation.Kind != recordings.ReplayDiverged ||
		diverged.Observation.Divergence == nil {
		t.Fatalf("ObserveReplay divergence = (%#v, %v)", diverged, err)
	}
}

func assertReplayOrderedProgress(t *testing.T, svc recordings.Service) {
	t.Helper()
	events := []recordings.CanonicalEvent{
		replayStateEvent(0, `{"state":"RUNNING"}`),
		replayStateEvent(1, `{"state":"COMPLETED"}`),
	}
	expected := events[1].Cursor
	planned, err := svc.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1,
		Timing:        recordings.ReplayTimingOrderOnly,
		Recording: recordings.ReplayRecordingFacts{
			RecordingID: "recording-progress",
			Events:      events,
		},
		ExpectedThrough: &expected,
		SelectedTick:    1,
	})
	if err != nil {
		t.Fatalf("CreateReplayPlan progress = %v", err)
	}
	progress, err := svc.ObserveReplay(recordings.ObserveReplayRequest{Plan: planned.Plan.Handle})
	if err != nil || progress.Observation.Kind != recordings.ReplayProgress {
		t.Fatalf("ObserveReplay progress = (%#v, %v)", progress, err)
	}
	completed, err := svc.ObserveReplay(recordings.ObserveReplayRequest{Plan: planned.Plan.Handle})
	if err != nil || completed.Observation.Kind != recordings.ReplayCompleted {
		t.Fatalf("ObserveReplay completed = (%#v, %v)", completed, err)
	}
}

func replayStateEvent(sequence recordings.CanonicalEventSequence, payload string) recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:       recordings.CanonicalEventID("state-" + string(rune('0'+sequence))),
		Sequence: sequence,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-progress",
			Sequence:           sequence,
		},
		RecordedAt: time.Unix(1_700_000_000+int64(sequence), 0).UTC(),
		Kind:       "FACTORY_STATE_RESPONSE",
		Payload:    payload,
	}
}

func assertArtifactExport(t *testing.T, svc recordings.Service) {
	t.Helper()
	artifact := buildServicePortableArtifact(t, svc)
	assertServicePortableRoundTrip(t, svc, artifact)
	assertServicePortableFailures(t, svc, artifact)
}

func buildServicePortableArtifact(
	t *testing.T,
	svc recordings.Service,
) recordings.PortableArtifact {
	t.Helper()
	bound, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-export",
		Artifact:    "artifact:export",
		Scope: recordings.CanonicalEventScope{
			FactorySessionID: "session-service-1",
		},
	})
	if err != nil {
		t.Fatalf("BindRecording portable artifact: %v", err)
	}
	event := recordings.CanonicalEvent{
		ID: "export-event", Kind: "WORK_REQUEST",
		Scope:      bound.Status.Scope,
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Payload:    "{}",
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-export",
		},
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent portable artifact: %v", err)
	}
	if _, err := svc.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	}); !errors.Is(err, recordings.ErrPortableArtifactUnavailable) {
		t.Fatalf("BuildPortableArtifact active = %v", err)
	}
	if _, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording portable artifact: %v", err)
	}
	built, err := svc.BuildPortableArtifact(recordings.BuildPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	})
	if err != nil {
		t.Fatalf("BuildPortableArtifact: %v", err)
	}
	return built.Artifact
}

func assertServicePortableRoundTrip(
	t *testing.T,
	svc recordings.Service,
	artifact recordings.PortableArtifact,
) {
	t.Helper()
	if _, err := svc.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: artifact,
	}); err != nil {
		t.Fatalf("ValidatePortableArtifact: %v", err)
	}
	summary, err := svc.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{
		Artifact: artifact,
	})
	if err != nil || summary.Summary.Scope.FactorySessionID != "session-service-1" {
		t.Fatalf("SummarizePortableArtifact = (%#v, %v)", summary, err)
	}
	encoded, err := svc.EncodePortableArtifact(recordings.EncodePortableArtifactRequest{
		Artifact: artifact,
	})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("EncodePortableArtifact = (%d bytes, %v)", len(encoded.Payload), err)
	}
	decoded, err := svc.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{
		Payload: encoded.Payload,
	})
	if err != nil || decoded.Artifact.Summary.EventCount != 1 ||
		decoded.Artifact.Events[0].ID != artifact.Events[0].ID {
		t.Fatalf("DecodePortableArtifact = (%#v, %v)", decoded, err)
	}
}

func assertServicePortableFailures(
	t *testing.T,
	svc recordings.Service,
	artifact recordings.PortableArtifact,
) {
	t.Helper()
	if _, err := svc.SummarizePortableArtifact(recordings.SummarizePortableArtifactRequest{}); !errors.Is(err, recordings.ErrUnsupportedPortableArtifactSchema) {
		t.Fatalf("SummarizePortableArtifact empty = %v, want ErrUnsupportedPortableArtifactSchema", err)
	}
	if _, err := svc.DecodePortableArtifact(recordings.DecodePortableArtifactRequest{}); !errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("DecodePortableArtifact empty = %v, want ErrInvalidPortableArtifact", err)
	}
	tampered := artifact
	tampered.Events = append([]recordings.CanonicalEvent{}, artifact.Events...)
	tampered.Events[0].Payload = `{"changed":true}`
	if _, err := svc.ValidatePortableArtifact(recordings.ValidatePortableArtifactRequest{
		Artifact: tampered,
	}); !errors.Is(err, recordings.ErrInvalidPortableArtifactIntegrity) {
		t.Fatalf("ValidatePortableArtifact tampered = %v", err)
	}
}

func TestCombinedServicePortableExportAndReadDelegates(t *testing.T) {
	t.Parallel()

	destination := filepath.Join(t.TempDir(), "destination-is-directory")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	ledger := &stubLedger{}
	publication, err := NewPortableArtifactPublication(
		os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
	)
	if err != nil {
		t.Fatalf("NewPortableArtifactPublication: %v", err)
	}
	svc := NewServiceWithLifecycleEffects(
		ledger,
		NewProjectionService(),
		nil,
		nil,
		nil,
		publication,
	)
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-export-delegate"}
	bound, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: "recording-export-delegate",
		Artifact:    recordings.RecordingArtifactReference(destination),
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	event := recordings.CanonicalEvent{
		ID: "export-delegate-event", Kind: "WORK_REQUEST",
		Scope:      scope,
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Payload:    "{}",
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-export-delegate",
		},
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}
	if _, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	if _, err := svc.ExportPortableArtifact(context.Background(), recordings.ExportPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
	}); !errors.Is(err, recordings.ErrPortableArtifactExportFailed) {
		t.Fatalf("ExportPortableArtifact = %v, want ErrPortableArtifactExportFailed", err)
	}
	if _, err := svc.ReadPortableArtifact(context.Background(), recordings.ReadPortableArtifactRequest{
		RecordingID: bound.Status.RecordingID,
		Reference:   recordings.RecordingArtifactReference(destination),
	}); !errors.Is(err, recordings.ErrPortableArtifactUnavailable) &&
		!errors.Is(err, recordings.ErrInvalidPortableArtifact) {
		t.Fatalf("ReadPortableArtifact = %v, want ErrPortableArtifactUnavailable or ErrInvalidPortableArtifact", err)
	}
}

func TestProjectionServiceDelegates(t *testing.T) {
	t.Parallel()
	projection := NewProjectionService()
	state, err := projection.ReconstructFactoryWorldState(nil, 0)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	_ = projection.SimpleDashboardRenderData(state)
	_ = projection.ProjectWorkstationRequests(state)
	if err := projection.ValidateReconnectReplay(nil, factorydefinitions.FactoryEventReconnectCursor{}, factorydefinitions.FactoryEventReconnectScope{}); err != nil {
		t.Fatalf("ValidateReconnectReplay: %v", err)
	}
}

func TestNewReplayClockAndExecutionNilArtifact(t *testing.T) {
	t.Parallel()
	if got := NewReplayClock(nil); got != nil {
		t.Fatalf("NewReplayClock(nil) = %#v, want nil", got)
	}
	provider, runner, hooks, planner, err := NewReplayExecution(nil, nil, nil)
	if provider != nil || runner != nil || hooks != nil || planner != nil || err != nil {
		t.Fatalf("NewReplayExecution(nil) = (%v,%v,%v,%v,%v), want nils", provider, runner, hooks, planner, err)
	}
}

func TestCombinedServiceReplayArtifactCapabilityRoundTripsDetachedOutcomes(t *testing.T) {
	t.Parallel()

	destination := filepath.Join(t.TempDir(), "portable", "recording.json")
	publication, err := NewPortableArtifactPublication(
		os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
	)
	if err != nil {
		t.Fatalf("NewPortableArtifactPublication: %v", err)
	}
	svc := NewServiceWithLifecycleEffects(
		&stubLedger{},
		NewProjectionService(),
		nil,
		nil,
		nil,
		publication,
	)
	capability, ok := svc.(recordings.RecordingReplayArtifacts)
	if !ok {
		t.Fatal("Recordings service does not expose RecordingReplayArtifacts")
	}

	const recordingID = recordings.ReplayRecordingID("recording-neutral-artifacts")
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-neutral-artifacts"}
	bound, err := svc.BindRecording(recordings.BindRecordingRequest{
		RecordingID: recordings.RecordingID(recordingID),
		Artifact:    recordings.RecordingArtifactReference(destination),
		Scope:       scope,
	})
	if err != nil {
		t.Fatalf("BindRecording: %v", err)
	}
	event := recordings.CanonicalEvent{
		ID:         "neutral-artifact-event",
		Sequence:   0,
		Scope:      scope,
		RecordedAt: time.Unix(1_700_000_000, 0).UTC(),
		Kind:       "WORK_REQUEST",
		Payload:    `{"work":"preserved"}`,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-neutral-artifacts",
			Sequence:           0,
		},
	}
	if _, err := svc.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent: %v", err)
	}

	assertCombinedReplayArtifactActiveErrors(t, capability, recordingID)

	if _, err := svc.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Unix(1_700_000_001, 0).UTC(),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	loaded := loadCombinedReplayArtifact(t, capability, recordingID)
	assertCombinedLoadedReplay(t, loaded, recordingID, scope, event)
	built := buildCombinedReplayArtifact(t, capability, recordingID)
	assertCombinedBuiltArtifact(t, built, recordingID, destination, event)
	assertCombinedArtifactOperations(t, capability, built.Artifact, scope, event)
	assertCombinedArtifactPublication(t, capability, recordingID, destination)
}

func assertCombinedReplayArtifactActiveErrors(t *testing.T, capability recordings.RecordingReplayArtifacts, recordingID recordings.ReplayRecordingID) {
	t.Helper()
	if _, err := capability.LoadReplay(recordings.LoadReplayRequest{RecordingID: recordingID}); !hasReplayArtifactErrorKind(err, recordings.ReplayArtifactErrorNotFinalized) {
		t.Fatalf("LoadReplay active = %v, want NOT_FINALIZED", err)
	}
	if _, err := capability.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: recordingID}); !hasReplayArtifactErrorKind(err, recordings.ReplayArtifactErrorUnavailable) {
		t.Fatalf("BuildArtifact active = %v, want UNAVAILABLE", err)
	}
	if _, err := capability.LoadReplay(recordings.LoadReplayRequest{RecordingID: "missing-neutral-artifacts"}); !hasReplayArtifactErrorKind(err, recordings.ReplayArtifactErrorNotFound) {
		t.Fatalf("LoadReplay missing = %v, want NOT_FOUND", err)
	}
}

func loadCombinedReplayArtifact(t *testing.T, capability recordings.RecordingReplayArtifacts, recordingID recordings.ReplayRecordingID) recordings.LoadReplayResult {
	t.Helper()
	loaded, err := capability.LoadReplay(recordings.LoadReplayRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("LoadReplay: %v", err)
	}
	return loaded
}

func assertCombinedLoadedReplay(t *testing.T, loaded recordings.LoadReplayResult, recordingID recordings.ReplayRecordingID, scope recordings.CanonicalEventScope, event recordings.CanonicalEvent) {
	t.Helper()
	if loaded.Replay.RecordingID != recordingID || loaded.Replay.Scope.FactorySessionID != scope.FactorySessionID ||
		len(loaded.Replay.Events) != 1 || loaded.Replay.Events[0].ID != string(event.ID) || loaded.Replay.Events[0].Payload != event.Payload {
		t.Fatalf("LoadReplay = %#v, want detached recording facts", loaded)
	}
}

func buildCombinedReplayArtifact(t *testing.T, capability recordings.RecordingReplayArtifacts, recordingID recordings.ReplayRecordingID) recordings.BuildArtifactResult {
	t.Helper()
	built, err := capability.BuildArtifact(recordings.BuildArtifactRequest{RecordingID: recordingID})
	if err != nil {
		t.Fatalf("BuildArtifact: %v", err)
	}
	return built
}

func assertCombinedBuiltArtifact(t *testing.T, built recordings.BuildArtifactResult, recordingID recordings.ReplayRecordingID, destination string, event recordings.CanonicalEvent) {
	t.Helper()
	if built.Artifact.SchemaVersion != recordings.ArtifactSchemaV1 || built.Artifact.Summary.RecordingID != recordingID ||
		built.Artifact.Summary.Reference != recordings.ArtifactReference(destination) || built.Artifact.Summary.EventCount != 1 ||
		len(built.Artifact.Events) != 1 || built.Artifact.Events[0].ID != string(event.ID) {
		t.Fatalf("BuildArtifact = %#v, want finalized detached artifact", built)
	}
}

func assertCombinedArtifactOperations(t *testing.T, capability recordings.RecordingReplayArtifacts, artifact recordings.ArtifactEnvelope, scope recordings.CanonicalEventScope, event recordings.CanonicalEvent) {
	t.Helper()
	validated, err := capability.ValidateArtifact(recordings.ValidateArtifactRequest{Artifact: artifact})
	if err != nil || validated.Summary.EventCount != 1 {
		t.Fatalf("ValidateArtifact = (%#v, %v), want one-event summary", validated, err)
	}
	summarized, err := capability.SummarizeArtifact(recordings.SummarizeArtifactRequest{Artifact: artifact})
	if err != nil || summarized.Summary.Scope.FactorySessionID != scope.FactorySessionID {
		t.Fatalf("SummarizeArtifact = (%#v, %v), want session scope", summarized, err)
	}
	encoded, err := capability.EncodeArtifact(recordings.EncodeArtifactRequest{Artifact: artifact})
	if err != nil || len(encoded.Payload) == 0 {
		t.Fatalf("EncodeArtifact = (%d bytes, %v), want payload", len(encoded.Payload), err)
	}
	decoded, err := capability.DecodeArtifact(recordings.DecodeArtifactRequest{Payload: encoded.Payload})
	if err != nil || decoded.Artifact.Events[0].Payload != event.Payload {
		t.Fatalf("DecodeArtifact = (%#v, %v), want preserved event payload", decoded, err)
	}
	assertCombinedArtifactValidationFailures(t, capability, artifact)
}

func assertCombinedArtifactValidationFailures(t *testing.T, capability recordings.RecordingReplayArtifacts, artifact recordings.ArtifactEnvelope) {
	t.Helper()
	tampered := artifact
	tampered.Events = append([]recordings.ReplayEvent(nil), artifact.Events...)
	tampered.Events[0].Payload = `{"changed":true}`
	if _, err := capability.ValidateArtifact(recordings.ValidateArtifactRequest{Artifact: tampered}); !hasReplayArtifactErrorKind(err, recordings.ReplayArtifactErrorInvalidIntegrity) {
		t.Fatalf("ValidateArtifact tampered = %v, want INVALID_INTEGRITY", err)
	}
	unsupported := artifact
	unsupported.SchemaVersion = "recordings.portable-artifact.v99"
	if _, err := capability.ValidateArtifact(recordings.ValidateArtifactRequest{Artifact: unsupported}); !hasReplayArtifactErrorKind(err, recordings.ReplayArtifactErrorUnsupportedSchema) {
		t.Fatalf("ValidateArtifact unsupported = %v, want UNSUPPORTED_SCHEMA", err)
	}
	if _, err := capability.DecodeArtifact(recordings.DecodeArtifactRequest{}); !hasReplayArtifactErrorKind(err, recordings.ReplayArtifactErrorInvalid) {
		t.Fatalf("DecodeArtifact empty = %v, want INVALID", err)
	}
}

func assertCombinedArtifactPublication(t *testing.T, capability recordings.RecordingReplayArtifacts, recordingID recordings.ReplayRecordingID, destination string) {
	t.Helper()
	exported, err := capability.ExportArtifact(context.Background(), recordings.ExportArtifactRequest{RecordingID: recordingID})
	if err != nil || exported.Reference != recordings.ArtifactReference(destination) {
		t.Fatalf("ExportArtifact = (%#v, %v), want published destination", exported, err)
	}
	read, err := capability.ReadArtifact(context.Background(), recordings.ReadArtifactRequest{RecordingID: recordingID, Reference: exported.Reference})
	if err != nil || read.Artifact.Integrity.Digest != exported.Artifact.Integrity.Digest {
		t.Fatalf("ReadArtifact = (%#v, %v), want exported digest", read, err)
	}
	if _, err := capability.ReadArtifact(context.Background(), recordings.ReadArtifactRequest{
		RecordingID: recordingID, Reference: recordings.ArtifactReference(destination + ".other"),
	}); !hasReplayArtifactErrorKind(err, recordings.ReplayArtifactErrorForeign) {
		t.Fatalf("ReadArtifact foreign = %v, want FOREIGN", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := capability.ReadArtifact(canceled, recordings.ReadArtifactRequest{RecordingID: recordingID, Reference: exported.Reference}); !hasReplayArtifactErrorKind(err, recordings.ReplayArtifactErrorCancelled) {
		t.Fatalf("ReadArtifact canceled = %v, want CANCELLED", err)
	}
}

func hasReplayArtifactErrorKind(err error, want recordings.ReplayArtifactErrorKind) bool {
	var artifactErr *recordings.ReplayArtifactError
	return errors.As(err, &artifactErr) && artifactErr.Kind == want
}
func TestCombinedServiceReadHelpersRejectInvalidInputsAndPreserveClockFacts(t *testing.T) {
	t.Parallel()

	svc := NewService(&stubLedger{}, NewProjectionService()).(*combinedService)
	_, err := svc.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{})
	var historicalErr *recordings.HistoricalRecordingQueryError
	if !errors.As(err, &historicalErr) || historicalErr.Kind != recordings.HistoricalRecordingQueryErrorUnavailable {
		t.Fatalf("QueryHistoricalRecording without capability = %v, want unavailable typed error", err)
	}
	if err := svc.ValidateReconnectReplayFrom(recordings.ValidateReconnectReplayRequest{
		Scope:  recordings.CanonicalEventScope{FactorySessionID: "session-1"},
		Cursor: recordings.CanonicalEventCursor{Sequence: -1},
	}); !errors.Is(err, recordings.ErrMalformedProjectionOrder) {
		t.Fatalf("malformed reconnect cursor = %v, want ErrMalformedProjectionOrder", err)
	}
	svc.replayByKey = nil
	if _, err := svc.LoadReplayArtifact(recordings.LoadReplayArtifactRequest{ArtifactID: "artifact-1"}); !errors.Is(err, recordings.ErrInvalidReplayArtifact) {
		t.Fatalf("artifact with unavailable map = %v, want ErrInvalidReplayArtifact", err)
	}

	events := []recordings.CanonicalEvent{
		{Cursor: recordings.CanonicalEventCursor{StreamGenerationID: "stream-1", Sequence: 0}},
		{Cursor: recordings.CanonicalEventCursor{StreamGenerationID: "stream-1", Sequence: 1}},
	}
	if copied, err := scopeEventPrefix(events, nil); err != nil || len(copied) != len(events) {
		t.Fatalf("scopeEventPrefix without cursor = %#v, %v, want copied history", copied, err)
	}
	if _, err := scopeEventPrefix(events, &recordings.CanonicalEventCursor{Sequence: -1, StreamGenerationID: "stream-1"}); !errors.Is(err, recordings.ErrInvalidReconnectCursor) {
		t.Fatalf("negative scope cursor = %v, want ErrInvalidReconnectCursor", err)
	}
	if _, err := scopeEventPrefix(events, &recordings.CanonicalEventCursor{Sequence: 0, StreamGenerationID: "other-stream"}); !errors.Is(err, recordings.ErrReconnectCursorUnavailable) {
		t.Fatalf("foreign scope cursor = %v, want ErrReconnectCursorUnavailable", err)
	}
	if prefix, err := scopeEventPrefix(events, &events[0].Cursor); err != nil || len(prefix) != 1 {
		t.Fatalf("matching scope cursor = %#v, %v, want one-event prefix", prefix, err)
	}
	if _, err := scopeEventPrefix(events, &recordings.CanonicalEventCursor{Sequence: 8, StreamGenerationID: "stream-1"}); !errors.Is(err, recordings.ErrReconnectCursorNotFound) {
		t.Fatalf("missing scope cursor = %v, want ErrReconnectCursorNotFound", err)
	}
	if cursorInEvents(events, recordings.CanonicalEventCursor{Sequence: -1, StreamGenerationID: "stream-1"}) {
		t.Fatal("negative cursor reported as present")
	}
	if !cursorInEvents(events, events[1].Cursor) {
		t.Fatal("matching cursor reported as absent")
	}
	if cursorInEvents(events, recordings.CanonicalEventCursor{Sequence: 9, StreamGenerationID: "stream-1"}) {
		t.Fatal("missing cursor reported as present")
	}

	want := time.Date(2026, 8, 26, 12, 0, 0, 0, time.FixedZone("test", 2*60*60))
	if recordingClockNow(nil) != nil {
		t.Fatal("recordingClockNow without a clock returned a callback")
	}
	clockNow := recordingClockNow(staticRecordingClock{at: want})
	if clockNow == nil {
		t.Fatal("recordingClockNow with a clock returned a nil callback")
	}
	if got := clockNow(); !got.Equal(want) {
		t.Fatalf("recordingClockNow callback returned %v, want %v", got, want)
	}
	svc.clock = nil
	if got := svc.recordingFinishedAt(); !got.IsZero() {
		t.Fatalf("finished time without clock = %v, want zero", got)
	}
	svc.clock = staticRecordingClock{at: want}
	if got := svc.recordingFinishedAt(); !got.Equal(want.UTC()) || got.Location() != time.UTC {
		t.Fatalf("finished time = %v, want UTC %v", got, want.UTC())
	}
}

func TestRecordingLifecycleAdapterPreservesDetachedOperations(t *testing.T) {
	t.Parallel()

	service := NewService(&stubLedger{}, NewProjectionService()).(*combinedService)
	lifecycle := recordings.RecordingLifecycle(service)
	if _, err := service.LoadResumeInput(recordings.LoadResumeInputRequest{Path: "missing.json"}); !errors.Is(err, recordings.ErrMissingReplayArtifact) {
		t.Fatalf("LoadResumeInput without a replay source = %v, want ErrMissingReplayArtifact", err)
	}

	if result, err := lifecycle.Begin(recordings.BeginRecordingRequest{}); err != nil || !reflect.DeepEqual(result, recordings.RecordingLifecycleResult{}) {
		t.Fatalf("disabled lifecycle Begin = (%#v, %v), want zero result", result, err)
	}
	if _, err := lifecycle.Begin(recordings.BeginRecordingRequest{Enabled: true}); !hasLifecycleErrorKind(err, recordings.LifecycleErrorInvalidTarget) {
		t.Fatalf("lifecycle Begin without target = %v, want INVALID_TARGET", err)
	}

	begin, err := lifecycle.Begin(recordings.BeginRecordingRequest{
		Enabled:     true,
		RecordingID: "adapter-begin",
		Artifact:    "artifact:adapter-begin",
		Scope:       recordings.LifecycleScope{FactorySessionID: "adapter-session"},
	})
	if err != nil || begin.Status.RecordingID != "adapter-begin" {
		t.Fatalf("lifecycle Begin success = (%#v, %v)", begin, err)
	}
	if err := lifecycle.Stop(recordings.StopLifecycleRequest{RecordingID: begin.Status.RecordingID}); err != nil {
		t.Fatalf("lifecycle Stop after Begin: %v", err)
	}
	if finished, err := lifecycle.Finish(recordings.FinishLifecycleRequest{
		RecordingID: begin.Status.RecordingID,
		FinishedAt:  time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
	}); err != nil || finished.Status.State != recordings.LifecycleStateFinalized {
		t.Fatalf("lifecycle Finish after Begin = (%#v, %v), want finalized", finished, err)
	}

	bound, err := lifecycle.Bind(recordings.BindLifecycleRequest{
		RecordingID: "adapter-bind",
		Artifact:    "artifact:adapter-bind",
		Scope:       recordings.LifecycleScope{FactorySessionID: "adapter-session"},
	})
	if err != nil || bound.Status.RecordingID != "adapter-bind" {
		t.Fatalf("lifecycle Bind = (%#v, %v)", bound, err)
	}
	if _, err := lifecycle.Bind(recordings.BindLifecycleRequest{RecordingID: "adapter-invalid"}); !hasLifecycleErrorKind(err, recordings.LifecycleErrorInvalidTarget) {
		t.Fatalf("lifecycle Bind without target = %v, want INVALID_TARGET", err)
	}

	event := recordings.LifecycleEvent{
		ID:       "adapter-event",
		Sequence: 0,
		Scope:    recordings.LifecycleScope{FactorySessionID: "adapter-session"},
		Cursor: recordings.LifecycleEventCursor{
			StreamGenerationID: "adapter-generation",
			Sequence:           0,
		},
		RecordedAt: time.Date(2026, 8, 26, 12, 1, 0, 0, time.UTC),
		Kind:       "WORK_REQUEST",
		Payload:    "{}",
	}
	if _, err := lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); err != nil {
		t.Fatalf("lifecycle AppendEvent = %v", err)
	}
	invalidEvent := event
	invalidEvent.Payload = "not-json"
	if _, err := lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       invalidEvent,
	}); !hasLifecycleErrorKind(err, recordings.LifecycleErrorInvalidEvent) {
		t.Fatalf("lifecycle AppendEvent invalid payload = %v, want INVALID_EVENT", err)
	}
	if _, err := lifecycle.RecordFailure(recordings.RecordLifecycleFailureRequest{
		RecordingID: bound.Status.RecordingID,
		Failure:     recordings.LifecycleFailure{Code: "", Message: ""},
	}); !hasLifecycleErrorKind(err, recordings.LifecycleErrorInvalidFailure) {
		t.Fatalf("lifecycle RecordFailure without facts = %v, want INVALID_FAILURE", err)
	}
	if _, err := lifecycle.RecordFailure(recordings.RecordLifecycleFailureRequest{
		RecordingID: bound.Status.RecordingID,
		Failure: recordings.LifecycleFailure{
			Code:    "adapter_failed",
			Message: "adapter failure",
		},
	}); err != nil {
		t.Fatalf("lifecycle RecordFailure = %v", err)
	}
	if flushed, err := lifecycle.Flush(recordings.FlushLifecycleRequest{RecordingID: bound.Status.RecordingID}); err != nil || flushed.Status.FlushedThrough == nil {
		t.Fatalf("lifecycle Flush = (%#v, %v), want flushed cursor", flushed, err)
	}
	if status, err := lifecycle.Status(recordings.LifecycleStatusRequest{RecordingID: bound.Status.RecordingID}); err != nil || status.Status.State != recordings.LifecycleStateFailed {
		t.Fatalf("lifecycle Status before Finish = (%#v, %v), want failed", status, err)
	}

	finished, err := lifecycle.Finish(recordings.FinishLifecycleRequest{
		RecordingID: bound.Status.RecordingID,
		FinishedAt:  time.Date(2026, 8, 26, 12, 2, 0, 0, time.UTC),
	})
	if err == nil || !hasLifecycleErrorKind(err, recordings.LifecycleErrorWriteFailed) || finished.Status.State != recordings.LifecycleStateFailed {
		t.Fatalf("lifecycle Finish failed recording = (%#v, %v), want failed write outcome", finished, err)
	}
	if _, err := lifecycle.AppendEvent(recordings.AppendLifecycleEventRequest{
		RecordingID: bound.Status.RecordingID,
		Event:       event,
	}); !hasLifecycleErrorKind(err, recordings.LifecycleErrorTerminal) {
		t.Fatalf("lifecycle AppendEvent after Finish = %v, want TERMINAL", err)
	}
	if status, err := lifecycle.Status(recordings.LifecycleStatusRequest{RecordingID: bound.Status.RecordingID}); err != nil || status.Status.FinalizedAt == nil {
		t.Fatalf("lifecycle Status after Finish = (%#v, %v), want finalized timestamp", status, err)
	}
}

func hasLifecycleErrorKind(err error, want recordings.LifecycleErrorKind) bool {
	var lifecycleErr *recordings.LifecycleError
	return errors.As(err, &lifecycleErr) && lifecycleErr.Kind == want
}

func TestRuntimeLedgerRouterPublishesCallbacksAndRoutesOptionalProvenance(t *testing.T) {
	t.Parallel()

	var nilRouter *runtimeLedgerRouter
	nilRouter.AddEventRecorder(nil)
	nilRouter.AddEventTypeRecorder(nil)
	nilRouter.AppendRecordedEvent(factorydefinitions.FactoryEvent{})
	if _, err := nilRouter.AppendRecordedEventWithValidation(factorydefinitions.FactoryEvent{}, nil); err == nil {
		t.Fatal("nil router append returned nil error")
	}
	if nilRouter.SecretProvenanceForEvent(factorydefinitions.FactoryEvent{}) != nil || nilRouter.SecretProvenanceDuringAppend(factorydefinitions.FactoryEvent{}) != nil {
		t.Fatal("nil router returned provenance")
	}

	router := newRuntimeLedgerRouter(func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) })
	var recorded int
	var recordedTypes int
	router.AddEventRecorder(func(factorydefinitions.FactoryEvent) { recorded++ })
	router.AddEventTypeRecorder(func(factorydefinitions.FactoryEventType) { recordedTypes++ })
	event := factorydefinitions.FactoryEvent{
		Id:      "router-event",
		Type:    factorydefinitions.FactoryEventTypeWorkRequest,
		Payload: json.RawMessage(`{}`),
		Context: factorydefinitions.FactoryEventContext{EventTime: time.Date(2026, 8, 26, 12, 1, 0, 0, time.UTC)},
	}
	router.AppendRecordedEvent(event)
	if recorded != 1 || recordedTypes != 1 {
		t.Fatalf("router callbacks = (%d, %d), want one event and one type callback", recorded, recordedTypes)
	}
	if len(router.CanonicalEvents()) != 1 {
		t.Fatalf("router canonical events = %d, want one", len(router.CanonicalEvents()))
	}
	if _, err := router.AppendRecordedEventWithValidation(event, func(factorydefinitions.FactoryEvent) error { return nil }); err != nil {
		t.Fatalf("router validated append: %v", err)
	}
	if router.SecretProvenanceForEvent(event) != nil || router.SecretProvenanceDuringAppend(event) != nil {
		t.Fatal("router returned unexpected provenance for event without declared secrets")
	}
	if generation := router.StreamGenerationID(); generation != "recordings-root" {
		t.Fatalf("router stream generation = %q, want recordings-root", generation)
	}
}
