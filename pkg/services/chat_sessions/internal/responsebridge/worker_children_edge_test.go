package responsebridge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// scriptedWorkerEvents keeps child-ingestion tests deterministic: each Read
// or Subscribe observation is explicitly supplied, so lifecycle and
// retention behavior need no timing sleeps.
type scriptedWorkerEvents struct {
	events.Service
	reads         []events.ReadResult
	readErr       error
	readIndex     int
	deliveries    []events.Delivery
	deliveryIndex int
	subscribeErr  error
}

func (f *scriptedWorkerEvents) Read(_ context.Context, _ events.ReadRequest) (events.ReadResult, error) {
	if f.readErr != nil {
		return events.ReadResult{}, f.readErr
	}
	if f.readIndex >= len(f.reads) {
		return events.ReadResult{Outcome: events.ReadOutcomeAtHead}, nil
	}
	result := f.reads[f.readIndex]
	f.readIndex++
	return result, nil
}

func (f *scriptedWorkerEvents) Subscribe(_ context.Context, _ events.SubscribeRequest) (events.Subscription, error) {
	if f.subscribeErr != nil {
		return nil, f.subscribeErr
	}
	return func(context.Context) events.Delivery {
		if f.deliveryIndex >= len(f.deliveries) {
			return events.Delivery{Kind: events.DeliveryClosed}
		}
		delivery := f.deliveries[f.deliveryIndex]
		f.deliveryIndex++
		return delivery
	}, nil
}

func newWorkerChildDrainState() *drainState {
	return &drainState{
		chatItemIDByFactoryItemID: make(map[string]string),
		childrenByWorkerSessionID: make(map[string]*workerChild),
		workerSessionIDByDispatch: make(map[string]string),
	}
}

func testWorkerAssociation() chatsessions.WorkerSessionAssociation {
	return chatsessions.WorkerSessionAssociation{DispatchID: "dispatch", WorkerSessionID: "worker"}
}

func testWorkerChild(t *testing.T, service *Service, state *drainState) *workerChildren {
	t.Helper()
	return &workerChildren{service: service, liveCtx: context.Background(), deliveryCtx: context.Background(), chatSessionID: "chat", factorySessionID: "factory", state: state}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestWorkerChildAssociationValidationAndRegistration(t *testing.T) {
	dispatchID := "dispatch"
	validPayload, err := json.Marshal(factorydefinitions.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker"})
	if err != nil {
		t.Fatalf("marshal association payload: %v", err)
	}
	tests := []struct {
		name       string
		event      factorydefinitions.FactoryEvent
		associated bool
		wantErr    error
	}{
		{"unrelated event", factorydefinitions.FactoryEvent{Type: factorydefinitions.FactoryEventTypeDispatchQueued}, false, nil},
		{"missing dispatch", factorydefinitions.FactoryEvent{Type: factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc}, false, ErrMalformedWorkerAssociation},
		{"invalid payload", factorydefinitions.FactoryEvent{Type: factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc, Context: factorydefinitions.FactoryEventContext{DispatchID: &dispatchID}, Payload: json.RawMessage(`not-json`)}, false, ErrMalformedWorkerAssociation},
		{"blank worker", factorydefinitions.FactoryEvent{Type: factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc, Context: factorydefinitions.FactoryEventContext{DispatchID: &dispatchID}, Payload: json.RawMessage(`{}`)}, false, ErrMalformedWorkerAssociation},
		{"valid", factorydefinitions.FactoryEvent{Type: factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc, Context: factorydefinitions.FactoryEventContext{DispatchID: &dispatchID}, Payload: validPayload}, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			association, associated, err := workerAssociationFromFactoryEvent(tt.event)
			if associated != tt.associated || !errors.Is(err, tt.wantErr) {
				t.Fatalf("workerAssociationFromFactoryEvent() = (%#v, %t, %v), want associated %t error %v", association, associated, err, tt.associated, tt.wantErr)
			}
		})
	}

	state := newWorkerChildDrainState()
	children := testWorkerChild(t, New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, nil, logging.NoopLogger{}), state)
	first, added, err := children.registerAssociation(testWorkerAssociation())
	if err != nil || !added || first == nil {
		t.Fatalf("register first = (%#v, %t, %v), want child", first, added, err)
	}
	if same, added, err := children.registerAssociation(testWorkerAssociation()); err != nil || added || same != first {
		t.Fatalf("register duplicate = (%#v, %t, %v), want original child without add", same, added, err)
	}
	state.childrenByWorkerSessionID["worker-b"] = &workerChild{association: chatsessions.WorkerSessionAssociation{DispatchID: "dispatch-b", WorkerSessionID: "worker-b"}}
	if got := children.children(); len(got) != 2 || got[0].association.WorkerSessionID != "worker" || got[1].association.WorkerSessionID != "worker-b" {
		t.Fatalf("children() = %#v, want stable worker-session ordering", got)
	}
	if _, _, err := children.registerAssociation(chatsessions.WorkerSessionAssociation{DispatchID: "dispatch", WorkerSessionID: "worker-other"}); !errors.Is(err, ErrConflictingWorkerAssociation) {
		t.Fatalf("conflicting dispatch error = %v, want %v", err, ErrConflictingWorkerAssociation)
	}
	state.childrenByWorkerSessionID["worker"] = &workerChild{association: chatsessions.WorkerSessionAssociation{DispatchID: "other-dispatch", WorkerSessionID: "worker"}}
	if _, _, err := children.registerAssociation(testWorkerAssociation()); !errors.Is(err, ErrConflictingWorkerAssociation) {
		t.Fatalf("conflicting worker error = %v, want %v", err, ErrConflictingWorkerAssociation)
	}

	children.setError(nil)
	firstErr := errors.New("first")
	children.setError(firstErr)
	children.setError(errors.New("later"))
	if !errors.Is(children.error(), firstErr) {
		t.Fatalf("children error = %v, want first error", children.error())
	}
}

func TestWorkerChildParentAndErrorClassification(t *testing.T) {
	identity := events.AppendIdentity{SourceType: "worker", SourceID: "worker", SourceSequence: 1, SourceEventID: "event"}
	child := &workerChild{sequencedSources: make(map[events.AppendIdentity]struct{})}
	if _, err := child.parentForRecord(workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseDelta}, identity); !errors.Is(err, ErrWorkerChildOpeningRequired) {
		t.Fatalf("missing opening error = %v, want %v", err, ErrWorkerChildOpeningRequired)
	}
	opening := workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted}
	if parent, err := child.parentForRecord(opening, identity); err != nil || parent != "" {
		t.Fatalf("opening parent = (%q, %v), want empty parent", parent, err)
	}
	child.openingItemID, child.openingSource = "tool", identity
	if _, err := child.parentForRecord(opening, events.AppendIdentity{SourceEventID: "other"}); !errors.Is(err, ErrDuplicateWorkerChildOpening) {
		t.Fatalf("duplicate opening error = %v, want %v", err, ErrDuplicateWorkerChildOpening)
	}
	child.terminal = true
	if _, err := child.parentForRecord(workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseDelta}, events.AppendIdentity{SourceEventID: "after-terminal"}); !errors.Is(err, ErrWorkerChildAfterTerminal) {
		t.Fatalf("after terminal error = %v, want %v", err, ErrWorkerChildAfterTerminal)
	}

	classes := map[error]string{
		ErrMalformedWorkerChildRecord:  "malformed_record",
		ErrWorkerChildOpeningRequired:  "opening_required",
		ErrDuplicateWorkerChildOpening: "duplicate_opening",
		ErrWorkerChildAfterTerminal:    "after_terminal",
		errors.New("other"):            "unknown",
	}
	for err, want := range classes {
		if got := workerChildRecordErrorClass(err); got != want {
			t.Fatalf("workerChildRecordErrorClass(%v) = %q, want %q", err, got, want)
		}
		if got := isIsolatedWorkerChildRecordError(err); got != (want != "unknown") {
			t.Fatalf("isIsolatedWorkerChildRecordError(%v) = %t, want %t", err, got, want != "unknown")
		}
	}
}

func TestWorkerChildLifecycleConsumersFailClosedAndIsolateMalformedRecords(t *testing.T) {
	association := testWorkerAssociation()
	malformed := events.Record{Payload: json.RawMessage(`not-json`)}
	for _, outcome := range []events.ReadOutcome{events.ReadOutcomeGap, events.ReadOutcomeInvalidCursor} {
		eventsService := &scriptedWorkerEvents{reads: []events.ReadResult{{Outcome: outcome}}}
		service := New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, eventsService, logging.NoopLogger{})
		if err := service.drainWorkerLifecycleTail(context.Background(), "chat", newWorkerChildDrainState(), association); !errors.Is(err, ErrWorkerChildHistoryGap) {
			t.Fatalf("tail outcome %d error = %v, want %v", outcome, err, ErrWorkerChildHistoryGap)
		}
	}
	readErr := errors.New("read failed")
	service := New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, &scriptedWorkerEvents{readErr: readErr}, logging.NoopLogger{})
	if err := service.drainWorkerLifecycleTail(context.Background(), "chat", newWorkerChildDrainState(), association); !errors.Is(err, readErr) {
		t.Fatalf("tail read error = %v, want %v", err, readErr)
	}
	service = New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, &scriptedWorkerEvents{reads: []events.ReadResult{{Outcome: events.ReadOutcomeUnspecified}}}, logging.NoopLogger{})
	if err := service.drainWorkerLifecycleTail(context.Background(), "chat", newWorkerChildDrainState(), association); err == nil {
		t.Fatal("tail unexpected outcome error = nil, want failure")
	}

	state := newWorkerChildDrainState()
	state.childrenByWorkerSessionID[association.WorkerSessionID] = &workerChild{association: association, sequencedSources: make(map[events.AppendIdentity]struct{})}
	eventsService := &scriptedWorkerEvents{reads: []events.ReadResult{{Outcome: events.ReadOutcomeProgress, Records: []events.Record{malformed}, Next: events.Cursor{}}, {Outcome: events.ReadOutcomeAtHead}}}
	service = New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, eventsService, logging.NoopLogger{})
	if err := service.drainWorkerLifecycleTail(context.Background(), "chat", state, association); err != nil {
		t.Fatalf("tail malformed child error = %v, want sibling-isolated skip", err)
	}

	subscribeErr := errors.New("subscribe failed")
	service = New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, &scriptedWorkerEvents{subscribeErr: subscribeErr}, logging.NoopLogger{})
	if err := service.drainWorkerLifecycleLive(context.Background(), context.Background(), "chat", newWorkerChildDrainState(), association); !errors.Is(err, subscribeErr) {
		t.Fatalf("live subscribe error = %v, want %v", err, subscribeErr)
	}
	service = New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, &scriptedWorkerEvents{deliveries: []events.Delivery{{Kind: events.DeliveryGap}}}, logging.NoopLogger{})
	if err := service.drainWorkerLifecycleLive(context.Background(), context.Background(), "chat", newWorkerChildDrainState(), association); !errors.Is(err, ErrWorkerChildHistoryGap) {
		t.Fatalf("live gap error = %v, want %v", err, ErrWorkerChildHistoryGap)
	}
	state = newWorkerChildDrainState()
	state.childrenByWorkerSessionID[association.WorkerSessionID] = &workerChild{association: association, sequencedSources: make(map[events.AppendIdentity]struct{})}
	service = New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, &scriptedWorkerEvents{deliveries: []events.Delivery{{Kind: events.DeliveryRecord, Record: malformed}, {Kind: events.DeliveryClosed}}}, logging.NoopLogger{})
	if err := service.drainWorkerLifecycleLive(context.Background(), context.Background(), "chat", state, association); err != nil {
		t.Fatalf("live malformed child error = %v, want sibling-isolated skip", err)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestWorkerChildStartFinishAndSequencingFailuresAreExplicit(t *testing.T) {
	state := newWorkerChildDrainState()
	withoutWorkers := New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, nil, logging.NoopLogger{})
	children, err := withoutWorkers.startWorkerChildren(context.Background(), context.Background(), "chat", "factory", state)
	if err != nil || children == nil {
		t.Fatalf("start without workers = (%#v, %v), want inert children", children, err)
	}
	if err := children.finish(context.Background()); err != nil {
		t.Fatalf("finish without workers error = %v, want nil", err)
	}

	workerEvents := &scriptedWorkerEvents{}
	subscribeErr := errors.New("factory subscribe")
	service := New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{factoryEventsErr: subscribeErr}, workerEvents, logging.NoopLogger{})
	if children, err := service.startWorkerChildren(context.Background(), context.Background(), "chat", "factory", newWorkerChildDrainState()); children != nil || !errors.Is(err, subscribeErr) {
		t.Fatalf("start subscribe failure = (%#v, %v), want %v", children, err, subscribeErr)
	}
	service = New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, workerEvents, logging.NoopLogger{})
	if children, err := service.startWorkerChildren(context.Background(), context.Background(), "chat", "factory", newWorkerChildDrainState()); children != nil || err == nil {
		t.Fatalf("start nil stream = (%#v, %v), want bounded error", children, err)
	}
	service = New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{factoryEvents: &factorydefinitions.FactoryEventStream{}}, workerEvents, logging.NoopLogger{})
	children, err = service.startWorkerChildren(context.Background(), context.Background(), "chat", "factory", newWorkerChildDrainState())
	if err != nil || children == nil {
		t.Fatalf("start without factory live channel = (%#v, %v), want retained-only children", children, err)
	}

	finishFactoryErr := errors.New("factory tail")
	children = testWorkerChild(t, New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{factoryEventsErr: finishFactoryErr}, workerEvents, logging.NoopLogger{}), newWorkerChildDrainState())
	if err := children.finish(context.Background()); !errors.Is(err, finishFactoryErr) {
		t.Fatalf("finish subscribe error = %v, want %v", err, finishFactoryErr)
	}
	children = testWorkerChild(t, New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, workerEvents, logging.NoopLogger{}), newWorkerChildDrainState())
	if err := children.finish(context.Background()); err == nil {
		t.Fatal("finish nil stream error = nil, want bounded error")
	}

	association := testWorkerAssociation()
	state = newWorkerChildDrainState()
	state.childrenByWorkerSessionID[association.WorkerSessionID] = &workerChild{association: association, sequencedSources: make(map[events.AppendIdentity]struct{})}
	validOpening := workerDraftRecord(t, association.WorkerSessionID, 1, workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted, Payload: json.RawMessage(`{"status":"STARTING"}`)})
	if err := New(&bridgeSequencer{didFirst: make(chan struct{}), sequenceErr: errors.New("sequence")}, bridgeTarget{}, workerEvents, logging.NoopLogger{}).sequenceWorkerLifecycleRecord(context.Background(), "chat", state, association, validOpening); err == nil {
		t.Fatal("sequence failure error = nil, want failure")
	}
	if err := New(&bridgeSequencer{didFirst: make(chan struct{}), advanceErr: errors.New("advance")}, bridgeTarget{}, workerEvents, logging.NoopLogger{}).sequenceWorkerLifecycleRecord(context.Background(), "chat", state, association, validOpening); err == nil {
		t.Fatal("advance failure error = nil, want failure")
	}
	if err := New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, workerEvents, logging.NoopLogger{}).sequenceWorkerLifecycleRecord(context.Background(), "chat", newWorkerChildDrainState(), association, validOpening); !errors.Is(err, ErrMalformedWorkerAssociation) {
		t.Fatalf("unregistered association error = %v, want %v", err, ErrMalformedWorkerAssociation)
	}
	if err := New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, workerEvents, logging.NoopLogger{}).sequenceWorkerLifecycleRecord(context.Background(), "chat", state, association, events.Record{Payload: json.RawMessage(`not-json`)}); !errors.Is(err, ErrMalformedWorkerChildRecord) {
		t.Fatalf("malformed worker record error = %v, want %v", err, ErrMalformedWorkerChildRecord)
	}
}
