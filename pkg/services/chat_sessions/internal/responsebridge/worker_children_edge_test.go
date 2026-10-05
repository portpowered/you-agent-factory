package responsebridge

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
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
	first, added, err := children.registerAssociation(testWorkerAssociation(), nil)
	if err != nil || !added || first == nil {
		t.Fatalf("register first = (%#v, %t, %v), want child", first, added, err)
	}
	if same, added, err := children.registerAssociation(testWorkerAssociation(), nil); err != nil || added || same != first {
		t.Fatalf("register duplicate = (%#v, %t, %v), want original child without add", same, added, err)
	}
	state.childrenByWorkerSessionID["worker-b"] = &workerChild{association: chatsessions.WorkerSessionAssociation{DispatchID: "dispatch-b", WorkerSessionID: "worker-b"}}
	if got := children.children(); len(got) != 2 || got[0].association.WorkerSessionID != "worker" || got[1].association.WorkerSessionID != "worker-b" {
		t.Fatalf("children() = %#v, want stable worker-session ordering", got)
	}
	if _, _, err := children.registerAssociation(chatsessions.WorkerSessionAssociation{DispatchID: "dispatch", WorkerSessionID: "worker-other"}, nil); !errors.Is(err, ErrConflictingWorkerAssociation) {
		t.Fatalf("conflicting dispatch error = %v, want %v", err, ErrConflictingWorkerAssociation)
	}
	state.childrenByWorkerSessionID["worker"] = &workerChild{association: chatsessions.WorkerSessionAssociation{DispatchID: "other-dispatch", WorkerSessionID: "worker"}}
	if _, _, err := children.registerAssociation(testWorkerAssociation(), nil); !errors.Is(err, ErrConflictingWorkerAssociation) {
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
		if err := service.drainWorkerLifecycleTail(context.Background(), "chat", newWorkerChildDrainState(), association, workersessions.Topic(association.WorkerSessionID)); !errors.Is(err, ErrWorkerChildHistoryGap) {
			t.Fatalf("tail outcome %d error = %v, want %v", outcome, err, ErrWorkerChildHistoryGap)
		}
	}
	readErr := errors.New("read failed")
	service := New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, &scriptedWorkerEvents{readErr: readErr}, logging.NoopLogger{})
	if err := service.drainWorkerLifecycleTail(context.Background(), "chat", newWorkerChildDrainState(), association, workersessions.Topic(association.WorkerSessionID)); !errors.Is(err, readErr) {
		t.Fatalf("tail read error = %v, want %v", err, readErr)
	}
	service = New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, &scriptedWorkerEvents{reads: []events.ReadResult{{Outcome: events.ReadOutcomeUnspecified}}}, logging.NoopLogger{})
	if err := service.drainWorkerLifecycleTail(context.Background(), "chat", newWorkerChildDrainState(), association, workersessions.Topic(association.WorkerSessionID)); err == nil {
		t.Fatal("tail unexpected outcome error = nil, want failure")
	}

	state := newWorkerChildDrainState()
	state.childrenByWorkerSessionID[association.WorkerSessionID] = &workerChild{association: association, sequencedSources: make(map[events.AppendIdentity]struct{})}
	eventsService := &scriptedWorkerEvents{reads: []events.ReadResult{{Outcome: events.ReadOutcomeProgress, Records: []events.Record{malformed}, Next: events.Cursor{}}, {Outcome: events.ReadOutcomeAtHead}}}
	service = New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, eventsService, logging.NoopLogger{})
	if err := service.drainWorkerLifecycleTail(context.Background(), "chat", state, association, workersessions.Topic(association.WorkerSessionID)); err != nil {
		t.Fatalf("tail malformed child error = %v, want sibling-isolated skip", err)
	}

	subscribeErr := errors.New("subscribe failed")
	service = New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, &scriptedWorkerEvents{subscribeErr: subscribeErr}, logging.NoopLogger{})
	if err := service.drainWorkerLifecycleLive(context.Background(), context.Background(), "chat", newWorkerChildDrainState(), association, workersessions.Topic(association.WorkerSessionID)); !errors.Is(err, subscribeErr) {
		t.Fatalf("live subscribe error = %v, want %v", err, subscribeErr)
	}
	service = New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, &scriptedWorkerEvents{deliveries: []events.Delivery{{Kind: events.DeliveryGap}}}, logging.NoopLogger{})
	if err := service.drainWorkerLifecycleLive(context.Background(), context.Background(), "chat", newWorkerChildDrainState(), association, workersessions.Topic(association.WorkerSessionID)); !errors.Is(err, ErrWorkerChildHistoryGap) {
		t.Fatalf("live gap error = %v, want %v", err, ErrWorkerChildHistoryGap)
	}
	state = newWorkerChildDrainState()
	state.childrenByWorkerSessionID[association.WorkerSessionID] = &workerChild{association: association, sequencedSources: make(map[events.AppendIdentity]struct{})}
	service = New(&bridgeSequencer{didFirst: make(chan struct{})}, bridgeTarget{}, &scriptedWorkerEvents{deliveries: []events.Delivery{{Kind: events.DeliveryRecord, Record: malformed}, {Kind: events.DeliveryClosed}}}, logging.NoopLogger{})
	if err := service.drainWorkerLifecycleLive(context.Background(), context.Background(), "chat", state, association, workersessions.Topic(association.WorkerSessionID)); err != nil {
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

// Every fixture owns its cursor, sequencer and worker observations. The paired
// logger selections must preserve the same results and committed observations.
type bridgeRunObservation struct {
	result    factorysessions.InvocationResult
	err       error
	sequences []chatsessions.SequenceRequest
	advances  []chatsessions.AdvanceStreamHeadRequest
	detached  bool
}

func observeBridgeRun(t *testing.T, failure string, invokeErr, collaboratorErr error, logger logging.Logger) bridgeRunObservation {
	t.Helper()
	observation := bridgeRunObservation{}
	sequencer := &bridgeSequencer{didFirst: make(chan struct{})}
	ready := make(chan struct{})
	cursor := &factorysessions.ResponseEventCursor{
		NextEvents: func(ctx context.Context) ([]factorysessions.FactoryResponseEvent, error) {
			close(ready)
			if failure == "next" {
				return nil, collaboratorErr
			}
			<-ctx.Done()
			return nil, ctx.Err()
		},
		DrainEvents: func() ([]factorysessions.FactoryResponseEvent, error) {
			if failure == "drain" {
				return nil, collaboratorErr
			}
			return []factorysessions.FactoryResponseEvent{{
				FactorySessionID: "factory-1", EventID: "private-event", Sequence: 1,
				Kind: workers.KindMessage, Phase: workers.PhaseCompleted,
				Payload: json.RawMessage(`{"prompt":"private-prompt","attachment":"private-attachment","transcript":"private-transcript"}`),
			}}, nil
		},
		DetachCursor: func() { observation.detached = true },
	}
	target := bridgeTarget{cursor: cursor}
	var workerEvents events.Service
	waitForLive := true
	switch failure {
	case "subscribe":
		target.err, waitForLive = collaboratorErr, false
	case "cursor":
		target.cursor, waitForLive = nil, false
	case "worker subscribe":
		workerEvents = &scriptedWorkerEvents{}
		target.factoryEventsErr, waitForLive = collaboratorErr, false
	case "worker finish":
		workerEvents = &scriptedWorkerEvents{}
		target.factoryEventStreams = []*factorydefinitions.FactoryEventStream{{}, nil}
		target.factoryEventIndex = new(int)
	case "worker gap", "response and worker":
		workerEvents = &scriptedWorkerEvents{reads: []events.ReadResult{{Outcome: events.ReadOutcomeGap}}}
		target.factoryEvents = workerAssociationStream(t, "dispatch", "worker")
	case "sequence":
		sequencer.sequenceErr = collaboratorErr
	case "advance":
		sequencer.advanceErr = collaboratorErr
	}
	if failure == "response and worker" {
		sequencer.sequenceErr = collaboratorErr
	}
	observation.result, observation.err = New(sequencer, target, workerEvents, logger).Run(
		context.Background(), "chat-1", 7, "factory-1", nil,
		func(context.Context) (factorysessions.InvocationResult, error) {
			if waitForLive {
				<-ready
			}
			status := factorysessions.InvocationTerminalStatusCompleted
			if invokeErr != nil {
				status = factorysessions.InvocationTerminalStatusFailed
			}
			return factorysessions.InvocationResult{RequestID: "turn", Status: status}, invokeErr
		},
	)
	observation.sequences, observation.advances = sequencer.sequences, sequencer.advances
	return observation
}

func TestServiceRunSelectedLoggerPreservesOutcomesAndNoopEffects(t *testing.T) {
	t.Parallel()
	collaboratorErr := errors.New("wrapped private-collaborator-error")
	invocationErr := errors.New("wrapped private-invocation-error")
	cases := []struct {
		failure, class      string
		cause               error
		sequences, advances int
		detached            bool
	}{
		{"success", "", nil, 1, 1, true},
		{"subscribe", "response_event_subscription", collaboratorErr, 0, 0, false},
		{"cursor", "response_event_subscription", nil, 0, 0, false},
		{"worker subscribe", "worker_event_subscription", collaboratorErr, 0, 0, true},
		{"next", "response_event_bridge", collaboratorErr, 0, 0, true},
		{"drain", "response_event_bridge", collaboratorErr, 0, 0, true},
		{"sequence", "response_event_bridge", collaboratorErr, 0, 0, true},
		{"advance", "response_event_bridge", collaboratorErr, 1, 0, true},
		{"worker finish", "worker_event_bridge", nil, 1, 1, true},
		{"worker gap", "worker_event_bridge", ErrWorkerChildHistoryGap, 1, 1, true},
		{"response and worker", "response_event_bridge", collaboratorErr, 0, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.failure, func(t *testing.T) {
			t.Parallel()
			for _, invokeErr := range []error{nil, invocationErr} {
				logger := &bridgeRecordingLogger{}
				capture := observeBridgeRun(t, tc.failure, invokeErr, collaboratorErr, logger)
				noop := observeBridgeRun(t, tc.failure, invokeErr, collaboratorErr, logging.NoopLogger{})
				class, cause := tc.class, tc.cause
				if invokeErr != nil {
					class, cause = "factory_invocation", invokeErr
				}
				assertBridgeRunObservation(t, capture, class, cause, tc.sequences, tc.advances, tc.detached)
				if !reflect.DeepEqual(capture.result, noop.result) || !reflect.DeepEqual(capture.sequences, noop.sequences) ||
					!reflect.DeepEqual(capture.advances, noop.advances) || capture.detached != noop.detached ||
					(cause != nil && !errors.Is(noop.err, cause)) || (capture.err == nil) != (noop.err == nil) {
					t.Fatalf("capture/Noop effects differ: %+v / %+v", capture, noop)
				}
				calls := logger.snapshot()
				if len(calls) != 2 || calls[0].level != "debug" || calls[1].level != "info" {
					t.Fatalf("diagnostics = %+v, want start/outcome", calls)
				}
				for _, call := range calls {
					assertBridgeLogValue(t, call, "op", responseBridgeOperation)
					assertBridgeLogValue(t, call, "chat_session_id", "chat-1")
					assertBridgeLogValue(t, call, "factory_session_id", "factory-1")
				}
				assertBridgeLogValue(t, calls[1], "error_class", class)
				assertBridgeLogValue(t, calls[1], "terminal_status", string(capture.result.Status))
				assertBridgeLogPrivacy(t, calls, "private-prompt", "private-attachment", "private-transcript", "private-collaborator-error", "private-invocation-error")
			}
		})
	}
}

func assertBridgeRunObservation(t *testing.T, got bridgeRunObservation, class string, cause error, sequences, advances int, detached bool) {
	t.Helper()
	wantStatus := factorysessions.InvocationTerminalStatusCompleted
	if class == "factory_invocation" {
		wantStatus = factorysessions.InvocationTerminalStatusFailed
	}
	if got.result.RequestID != "turn" || got.result.Status != wantStatus || (got.err != nil) != (class != "") {
		t.Fatalf("result/error = (%+v, %v), want status %s and class %q", got.result, got.err, wantStatus, class)
	}
	if cause != nil && !errors.Is(got.err, cause) {
		t.Fatalf("error = %v, want cause %v", got.err, cause)
	}
	if len(got.sequences) != sequences || len(got.advances) != advances || got.detached != detached {
		t.Fatalf("sequence/advance/detach = %d/%d/%t, want %d/%d/%t", len(got.sequences), len(got.advances), got.detached, sequences, advances, detached)
	}
	if advances > 0 && got.advances[0].ExpectedVersion != 7 {
		t.Fatalf("head = %+v, want version 7", got.advances[0])
	}
}

func TestWorkerChildMalformedDiagnosticsStayPrivateAndSiblingContinues(t *testing.T) {
	t.Parallel()
	for _, live := range []bool{false, true} {
		t.Run(map[bool]string{false: "tail", true: "live"}[live], func(t *testing.T) {
			t.Parallel()
			association := testWorkerAssociation()
			malformed := workerLifecycleRecord(t, association.WorkerSessionID, 1, workers.PhaseStarted, "STARTING")
			malformed.Payload = json.RawMessage(`{"prompt":"private-prompt","attachment":"private-attachment","transcript":"private-transcript","phase":`)
			valid := workerLifecycleRecord(t, association.WorkerSessionID, 2, workers.PhaseStarted, "STARTING")
			workerEvents := &scriptedWorkerEvents{
				reads:      []events.ReadResult{{Outcome: events.ReadOutcomeProgress, Records: []events.Record{malformed, valid}}},
				deliveries: []events.Delivery{{Kind: events.DeliveryRecord, Record: malformed}, {Kind: events.DeliveryRecord, Record: valid}},
			}
			logger := &bridgeRecordingLogger{}
			sequencer := &bridgeSequencer{didFirst: make(chan struct{})}
			service := New(sequencer, bridgeTarget{}, workerEvents, logger)
			state := newWorkerChildDrainState()
			state.childrenByWorkerSessionID[association.WorkerSessionID] = &workerChild{association: association, sequencedSources: make(map[events.AppendIdentity]struct{})}
			var err error
			if live {
				err = service.drainWorkerLifecycleLive(context.Background(), context.Background(), "chat", state, association, valid.ID.Topic)
			} else {
				err = service.drainWorkerLifecycleTail(context.Background(), "chat", state, association, valid.ID.Topic)
			}
			if err != nil || len(sequencer.sequences) != 1 || len(sequencer.advances) != 1 || sequencer.sequences[0].SourceEventID != valid.SourceEventID {
				t.Fatalf("valid sibling not committed: %v / %+v / %+v", err, sequencer.sequences, sequencer.advances)
			}
			calls := logger.snapshot()
			if len(calls) != 1 || calls[0].level != "warn" {
				t.Fatalf("diagnostics = %+v, want one Warn", calls)
			}
			assertBridgeLogValue(t, calls[0], "op", responseBridgeOperation)
			assertBridgeLogValue(t, calls[0], "dispatch_id", association.DispatchID)
			assertBridgeLogValue(t, calls[0], "worker_session_id", association.WorkerSessionID)
			assertBridgeLogValue(t, calls[0], "source_sequence", uint64(1))
			assertBridgeLogValue(t, calls[0], "error_class", "malformed_record")
			assertBridgeLogPrivacy(t, calls, "private-prompt", "private-attachment", "private-transcript", "unexpected end of JSON input")
		})
	}
}
