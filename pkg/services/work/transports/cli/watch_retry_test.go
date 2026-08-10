package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	watchRetryModelRequestEventID  = "factory-event/model-request/dispatch-retry/model-request/1"
	watchRetryModelResponseEventID = "factory-event/model-response/2cf2a099-909b-4446-8e8d-1453054e093c/model-request/1"
)

func TestWatchReducerDefersWorkRequestsWithoutAuthoritativeState(t *testing.T) {
	metadata := watchFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory", 1,
		factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{
				Name: "task",
				States: []factoryapi.WorkState{
					{Name: "to-complete", Type: factoryapi.WorkStateTypePROCESSING},
					{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
				},
			}},
		}})
	request := watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "request", 2,
		factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{
			{WorkId: watchStringPtr("work-1"), WorkTypeName: watchStringPtr("task")},
			{WorkId: watchStringPtr("system-time"), WorkTypeName: watchStringPtr("__system_time"), State: &factoryapi.WorkState{
				Name: "pending", Type: factoryapi.WorkStateTypePROCESSING,
			}},
		}})
	terminal := watchTransitionEvent(t, "move-terminal", 3, "work-1", "to-complete", "complete", true)
	reducer := newWatchReducer("session-1")
	if _, _, completed, err := reducer.Accept(metadata); err != nil || completed {
		t.Fatalf("metadata: completed=%t error=%v, want incomplete", completed, err)
	}
	if _, _, completed, err := reducer.Accept(request); err != nil || completed {
		t.Fatalf("state-less Work request: completed=%t error=%v, want incomplete", completed, err)
	}
	if _, emit, completed, err := reducer.Accept(terminal); err != nil || !emit || !completed {
		t.Fatalf("terminal transition: emit=%t completed=%t error=%v, want emitted completion", emit, completed, err)
	}
}

func TestWatchReducerAcceptsRefreshedModelRequestRetryWithoutProjectingWork(t *testing.T) {
	metadata, request, firstTransition := watchRetrySetup(t)
	retryInitial := watchModelRequestEvent(t, watchRetryModelRequestEventID, 4, "gpt-5-codex")
	betweenTransition := watchTransitionEvent(t, "move-between", 5, "work-1", "processing", "review", false)
	retryRefreshed := watchModelRequestEvent(t, watchRetryModelRequestEventID, 6, "gpt-5-codex")
	secondTransition := watchTransitionEvent(t, "move-2", 7, "work-1", "review", "done", true)
	events := []factoryapi.FactoryEvent{
		metadata, request, firstTransition, retryInitial, betweenTransition,
		retryRefreshed, retryRefreshed, secondTransition,
	}
	assertModelRequestRetryShape(t, retryInitial, retryRefreshed)

	reducer := newWatchReducer("session-retry")
	var transitions []WatchTransition
	var previousCursor *watchEventCursor
	for _, event := range events {
		transition, emit, _, err := reducer.Accept(event)
		if err != nil {
			t.Fatalf("Accept(%q) error = %v", event.Id, err)
		}
		if emit {
			transitions = append(transitions, transition)
		}
		cursor := reducer.Cursor()
		if previousCursor != nil && cursor.Sequence < previousCursor.Sequence {
			t.Fatalf("cursor regressed from %#v to %#v after %q", previousCursor, cursor, event.Id)
		}
		previousCursor = cursor
	}

	if len(transitions) != 3 || transitions[0].EventID != "move-1" || transitions[1].EventID != "move-between" || transitions[2].EventID != "move-2" {
		t.Fatalf("transitions = %#v, want exactly move-1, move-between, then move-2", transitions)
	}
	if cursor := reducer.Cursor(); cursor == nil || cursor.EventID != "move-2" || cursor.Sequence != 7 {
		t.Fatalf("final cursor = %#v, want move-2 at sequence 7", cursor)
	}
	if !reducer.Completed() {
		t.Fatal("reducer did not complete after the terminal Work transition")
	}
}

func TestWatchReducerRejectsNonIncreasingConflictingModelRequestRetry(t *testing.T) {
	reducer := newWatchReducer("session-retry-conflict")
	initial := watchModelRequestEvent(t, watchRetryModelRequestEventID, 4, "gpt-5-codex")
	if _, _, _, err := reducer.Accept(initial); err != nil {
		t.Fatalf("Accept(initial) error = %v", err)
	}

	for _, test := range []struct {
		name     string
		sequence int
	}{
		{name: "same sequence", sequence: 4},
		{name: "backward sequence", sequence: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			conflicting := watchModelRequestEvent(t, watchRetryModelRequestEventID, test.sequence, "gpt-5-codex-mini")
			if _, _, _, err := reducer.Accept(conflicting); err == nil || !strings.Contains(err.Error(), "non-increasing canonical sequence") {
				t.Fatalf("Accept(conflicting retry) error = %v, want non-increasing sequence failure", err)
			}
			if cursor := reducer.Cursor(); cursor == nil || cursor.EventID != initial.Id || cursor.Sequence != initial.Context.Sequence {
				t.Fatalf("cursor after rejected retry = %#v, want initial model request at sequence %d", cursor, initial.Context.Sequence)
			}
		})
	}
}

func TestWatchReducerAppliesOrderedPolicyToEveryNonProjectingEvent(t *testing.T) {
	for _, eventType := range currentNonProjectingWatchEventTypes() {
		t.Run(string(eventType), func(t *testing.T) {
			assertOrderedNonProjectingEventPolicy(t, eventType)
		})
	}
}

func TestWatchReducerTreatsUnknownEventTypesAsStrict(t *testing.T) {
	const eventType = factoryapi.FactoryEventType("FUTURE_PROJECTION_EVENT")
	const eventID = "factory-event/future-projection/1"
	reducer := newWatchReducer("session-future-event")
	initial := watchNonProjectingEvent(t, eventType, eventID, 4, "initial")
	refreshed := watchNonProjectingEvent(t, eventType, eventID, 6, "refreshed")
	assertWatchAcceptsNoTransition(t, reducer, initial, eventID, 4)
	if _, _, _, err := reducer.Accept(refreshed); err == nil {
		t.Fatal("Accept(refreshed unknown event) error = nil, want strict conflict")
	}
	assertWatchCursor(t, reducer, eventID, 4)
}

func currentNonProjectingWatchEventTypes() []factoryapi.FactoryEventType {
	return []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeAgentRunResponse,
		factoryapi.FactoryEventTypeArtifactCreated,
		factoryapi.FactoryEventTypeDispatchInterrupted,
		factoryapi.FactoryEventTypeDispatchQueued,
		factoryapi.FactoryEventTypeDispatchReconciled,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchResponse,
		factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation,
		factoryapi.FactoryEventTypeFactoryStateResponse,
		factoryapi.FactoryEventTypeInferenceRequest,
		factoryapi.FactoryEventTypeInferenceResponse,
		factoryapi.FactoryEventTypeJavaScriptCheckpointRef,
		factoryapi.FactoryEventTypeJavaScriptPhaseChange,
		factoryapi.FactoryEventTypeModelRequest,
		factoryapi.FactoryEventTypeModelResponse,
		factoryapi.FactoryEventTypeOrchestratorCheckpointWritten,
		factoryapi.FactoryEventTypeOrchestratorPhaseChanged,
		factoryapi.FactoryEventTypeRelationshipChangeRequest,
		factoryapi.FactoryEventTypeRunResponse,
		factoryapi.FactoryEventTypeScriptRequest,
		factoryapi.FactoryEventTypeScriptResponse,
		factoryapi.FactoryEventTypeSessionCompleted,
		factoryapi.FactoryEventTypeSessionLifecycleControl,
		factoryapi.FactoryEventTypeSessionPaused,
		factoryapi.FactoryEventTypeSessionResultUpdated,
		factoryapi.FactoryEventTypeSessionResumed,
		factoryapi.FactoryEventTypeSessionStarted,
	}
}

func assertOrderedNonProjectingEventPolicy(t *testing.T, eventType factoryapi.FactoryEventType) {
	t.Helper()
	eventID := watchNonProjectingEventID(eventType)
	reducer := newWatchReducer("session-enrichment-policy")
	initial := watchNonProjectingEvent(t, eventType, eventID, 4, "initial")
	refreshed := watchNonProjectingEvent(t, eventType, eventID, 6, "refreshed")
	assertWatchAcceptsNoTransition(t, reducer, initial, eventID, 4)
	assertWatchAcceptsNoTransition(t, reducer, refreshed, eventID, 6)
	assertWatchAcceptsNoTransition(t, reducer, refreshed, eventID, 6)
	conflicting := watchNonProjectingEvent(t, eventType, eventID, 6, "conflicting")
	assertWatchRejectsNonIncreasing(t, reducer, conflicting)
	backward := watchNonProjectingEvent(t, eventType, eventID, 5, "backward")
	assertWatchRejectsNonIncreasing(t, reducer, backward)
	assertWatchCursor(t, reducer, eventID, 6)
}

func watchNonProjectingEventID(eventType factoryapi.FactoryEventType) string {
	switch eventType {
	case factoryapi.FactoryEventTypeModelRequest:
		return watchRetryModelRequestEventID
	case factoryapi.FactoryEventTypeModelResponse:
		return watchRetryModelResponseEventID
	default:
		return "factory-event/enrichment/" + strings.ToLower(strings.ReplaceAll(string(eventType), "_", "-"))
	}
}

func assertWatchAcceptsNoTransition(
	t *testing.T,
	reducer *watchReducer,
	event factoryapi.FactoryEvent,
	eventID string,
	sequence int,
) {
	t.Helper()
	_, emit, _, err := reducer.Accept(event)
	if err != nil || emit {
		t.Fatalf("Accept(%q) = emit %t, error %v; want no transition", event.Id, emit, err)
	}
	assertWatchCursor(t, reducer, eventID, sequence)
}

func assertWatchRejectsNonIncreasing(t *testing.T, reducer *watchReducer, event factoryapi.FactoryEvent) {
	t.Helper()
	if _, _, _, err := reducer.Accept(event); err == nil || !strings.Contains(err.Error(), "non-increasing canonical sequence") {
		t.Fatalf("Accept(%q) error = %v, want non-increasing sequence failure", event.Id, err)
	}
}

func assertWatchCursor(t *testing.T, reducer *watchReducer, eventID string, sequence int) {
	t.Helper()
	if cursor := reducer.Cursor(); cursor == nil || cursor.EventID != eventID || cursor.Sequence != sequence {
		t.Fatalf("cursor = %#v, want %q at sequence %d", cursor, eventID, sequence)
	}
}

func TestWatchFiniteStreamIgnoresRefreshedModelRequestRetry(t *testing.T) {
	metadata, request, firstTransition := watchRetrySetup(t)
	retryInitial := watchModelRequestEvent(t, watchRetryModelRequestEventID, 4, "gpt-5-codex")
	betweenTransition := watchTransitionEvent(t, "move-between", 5, "work-1", "processing", "review", false)
	retryRefreshed := watchModelRequestEvent(t, watchRetryModelRequestEventID, 6, "gpt-5-codex")
	terminal := watchTransitionEvent(t, "move-terminal", 7, "work-1", "review", "done", true)
	assertModelRequestRetryShape(t, retryInitial, retryRefreshed)
	stream := &finiteWatchEventStream{events: []factoryapi.FactoryEvent{
		metadata, request, firstTransition, retryInitial, betweenTransition, retryRefreshed, terminal,
	}}
	var output bytes.Buffer
	err := watchWithSource(
		WatchConfig{Context: context.Background(), SessionID: "session-finite-retry", Output: &output},
		watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
			return stream, nil
		}),
	)
	if err != nil {
		t.Fatalf("watchWithSource() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("output lines = %d, want three Work transition lines: %q", len(lines), output.String())
	}
	var first, second, third watchLine
	if err := decodeWatchLine(lines[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := decodeWatchLine(lines[1], &second); err != nil {
		t.Fatal(err)
	}
	if err := decodeWatchLine(lines[2], &third); err != nil {
		t.Fatal(err)
	}
	if first.EventID != "move-1" || second.EventID != "move-between" || third.EventID != "move-terminal" ||
		first.Sequence >= second.Sequence || second.Sequence >= third.Sequence || !third.Terminal {
		t.Fatalf("finite transitions = %#v, %#v, %#v, want ordered move-1, move-between, then terminal move", first, second, third)
	}
}

func TestWatchFiniteStreamIgnoresRefreshedModelResponseRetry(t *testing.T) {
	metadata, request, firstTransition := watchRetrySetup(t)
	retryInitial := watchModelResponseEvent(t, watchRetryModelResponseEventID, 4, "gpt-5-codex")
	betweenTransition := watchTransitionEvent(t, "move-between-response", 5, "work-1", "processing", "review", false)
	retryRefreshed := watchModelResponseEvent(t, watchRetryModelResponseEventID, 6, "gpt-5-codex-mini")
	terminal := watchTransitionEvent(t, "move-terminal-response", 7, "work-1", "review", "done", true)
	stream := &finiteWatchEventStream{events: []factoryapi.FactoryEvent{
		metadata, request, firstTransition, retryInitial, betweenTransition, retryRefreshed, terminal,
	}}
	var output bytes.Buffer
	err := watchWithSource(
		WatchConfig{Context: context.Background(), SessionID: "session-finite-response-retry", Output: &output},
		watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
			return stream, nil
		}),
	)
	if err != nil {
		t.Fatalf("watchWithSource() error = %v", err)
	}
	assertRetryStreamOutput(t, output.String(), "move-between-response", "move-terminal-response")
}

func TestWatchFollowIgnoresRefreshedModelRequestRetry(t *testing.T) {
	metadata, request, firstTransition, later := watchRetryFollowSetup(t)
	retryInitial := watchModelRequestEvent(t, watchRetryModelRequestEventID, 4, "gpt-5-codex")
	betweenTransition := watchTransitionEvent(t, "move-between-follow", 5, "work-1", "processing", "review", false)
	retryRefreshed := watchModelRequestEvent(t, watchRetryModelRequestEventID, 6, "gpt-5-codex")
	later.Context = watchEventContext(7)
	assertModelRequestRetryShape(t, retryInitial, retryRefreshed)
	stream := &cancellableWatchEventStream{
		events:  []factoryapi.FactoryEvent{metadata, request, firstTransition, retryInitial, betweenTransition, retryRefreshed, later},
		blocked: make(chan struct{}),
		closed:  make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- watchWithRetry(
			WatchConfig{Context: ctx, SessionID: "session-follow-retry", Follow: true, Output: &output},
			watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
				return stream, nil
			}),
			watchRetryPolicy{maxAttempts: 0},
		)
	}()

	waitForFollowStream(t, stream)
	cancel()
	waitForFollowCancellation(t, done)
	assertFollowRetryOutput(t, output.String())
}

func TestWatchFollowIgnoresRefreshedModelResponseRetry(t *testing.T) {
	metadata, request, firstTransition, later := watchRetryFollowSetup(t)
	retryInitial := watchModelResponseEvent(t, watchRetryModelResponseEventID, 4, "gpt-5-codex")
	betweenTransition := watchTransitionEvent(t, "move-between-response-follow", 5, "work-1", "processing", "review", false)
	retryRefreshed := watchModelResponseEvent(t, watchRetryModelResponseEventID, 6, "gpt-5-codex-mini")
	later.Context = watchEventContext(7)
	stream := &cancellableWatchEventStream{
		events:  []factoryapi.FactoryEvent{metadata, request, firstTransition, retryInitial, betweenTransition, retryRefreshed, later},
		blocked: make(chan struct{}),
		closed:  make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- watchWithRetry(
			WatchConfig{Context: ctx, SessionID: "session-follow-response-retry", Follow: true, Output: &output},
			watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
				return stream, nil
			}),
			watchRetryPolicy{maxAttempts: 0},
		)
	}()

	waitForFollowStream(t, stream)
	cancel()
	waitForFollowCancellation(t, done)
	assertFollowRetryOutputWithEvents(t, output.String(), "move-between-response-follow")
}

func waitForFollowStream(t *testing.T, stream *cancellableWatchEventStream) {
	t.Helper()
	select {
	case <-stream.blocked:
	case <-time.After(time.Second):
		t.Fatal("follow watch did not remain attached after retry enrichment")
	}
}

func waitForFollowCancellation(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("watch error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("follow watch did not cancel while reading")
	}
}

func assertFollowRetryOutput(t *testing.T, output string) {
	assertFollowRetryOutputWithEvents(t, output, "move-between-follow")
}

func assertFollowRetryOutputWithEvents(t *testing.T, output, betweenEventID string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Fatalf("output lines = %d, want three Work transition lines: %q", len(lines), output)
	}
	var first, second, third watchLine
	if err := decodeWatchLine(lines[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := decodeWatchLine(lines[1], &second); err != nil {
		t.Fatal(err)
	}
	if err := decodeWatchLine(lines[2], &third); err != nil {
		t.Fatal(err)
	}
	if first.EventID != "move-1" || second.EventID != betweenEventID || third.EventID != "move-later" ||
		first.Sequence >= second.Sequence || second.Sequence >= third.Sequence || first.Terminal || second.Terminal || third.Terminal {
		t.Fatalf("follow transitions = %#v, %#v, %#v, want ordered non-terminal transitions", first, second, third)
	}
}

func assertRetryStreamOutput(t *testing.T, output, betweenEventID, terminalEventID string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Fatalf("output lines = %d, want three Work transition lines: %q", len(lines), output)
	}
	var first, second, third watchLine
	if err := decodeWatchLine(lines[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := decodeWatchLine(lines[1], &second); err != nil {
		t.Fatal(err)
	}
	if err := decodeWatchLine(lines[2], &third); err != nil {
		t.Fatal(err)
	}
	if first.EventID != "move-1" || second.EventID != betweenEventID || third.EventID != terminalEventID ||
		first.Sequence >= second.Sequence || second.Sequence >= third.Sequence || !third.Terminal {
		t.Fatalf("finite transitions = %#v, %#v, %#v, want ordered transitions through terminal", first, second, third)
	}
}

func TestWatchReconnectsFromLaterRetryCursorAndEmitsSubsequentTransition(t *testing.T) {
	metadata, request, firstTransition := watchReconnectSetup(t)
	retryInitial := watchModelRequestEvent(t, watchRetryModelRequestEventID, 4, "gpt-5-codex")
	interveningResponse := watchModelResponseEvent(t, "factory-event/model-response/dispatch-retry/model-request/1", 5, "gpt-5-codex")
	retryRefreshed := watchModelRequestEvent(t, watchRetryModelRequestEventID, 6, "gpt-5-codex")
	secondTransition := watchTransitionEvent(t, "move-2", 7, "work-1", "processing", "done", true)
	firstStream := &finiteWatchEventStream{events: []factoryapi.FactoryEvent{
		metadata, request, firstTransition, retryInitial, interveningResponse, retryRefreshed,
	}}
	secondStream := &finiteWatchEventStream{events: []factoryapi.FactoryEvent{secondTransition}}
	var cursors []*watchEventCursor
	var openCalls int
	var output bytes.Buffer
	err := watchWithRetry(
		WatchConfig{Context: context.Background(), SessionID: "session-reconnect", Output: &output},
		reconnectSequencedOpenFunc(t, firstStream, secondStream, &cursors, &openCalls),
		watchRetryPolicy{maxAttempts: 2, wait: func(context.Context, time.Duration) error { return nil }},
	)
	if err != nil {
		t.Fatalf("watchWithRetry() error = %v", err)
	}
	assertReconnectCursorSequence(t, openCalls, cursors)
	assertReconnectOrderedTransitions(t, output.String())
}

// reconnectSequencedOpenFunc returns a watchEventOpenFunc that serves first
// on the initial open and second on the first reconnect, recording every
// cursor it was opened with.
func reconnectSequencedOpenFunc(
	t *testing.T,
	first *finiteWatchEventStream,
	second *finiteWatchEventStream,
	cursors *[]*watchEventCursor,
	openCalls *int,
) watchEventOpenFunc {
	t.Helper()
	return func(_ context.Context, cursor *watchEventCursor) (watchEventStream, error) {
		*openCalls++
		if cursor != nil {
			copy := *cursor
			*cursors = append(*cursors, &copy)
		} else {
			*cursors = append(*cursors, nil)
		}
		switch *openCalls {
		case 1:
			return first, nil
		case 2:
			return second, nil
		default:
			return nil, errors.New("unexpected extra reconnect")
		}
	}
}

func assertReconnectCursorSequence(t *testing.T, openCalls int, cursors []*watchEventCursor) {
	t.Helper()
	if openCalls != 2 || len(cursors) != 2 || cursors[0] != nil {
		t.Fatalf("open calls/cursors = %d/%#v, want initial open and one cursor reconnect", openCalls, cursors)
	}
	if cursors[1] == nil || cursors[1].EventID != watchRetryModelRequestEventID || cursors[1].Sequence != 6 {
		t.Fatalf("reconnect cursor = %#v, want refreshed model retry at sequence 6", cursors[1])
	}
}

func assertReconnectOrderedTransitions(t *testing.T, output string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("output lines = %d, want duplicate-free transitions: %q", len(lines), output)
	}
	var first, second watchLine
	if err := decodeWatchLine(lines[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := decodeWatchLine(lines[1], &second); err != nil {
		t.Fatal(err)
	}
	if first.EventID != "move-1" || second.EventID != "move-2" || first.Sequence >= second.Sequence || !second.Terminal {
		t.Fatalf("transitions = %#v, %#v, want ordered move-1 then terminal move-2", first, second)
	}
}

func watchRetrySetup(t *testing.T) (factoryapi.FactoryEvent, factoryapi.FactoryEvent, factoryapi.FactoryEvent) {
	t.Helper()
	metadata := watchFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory-retry", 1,
		factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{Name: "task", States: []factoryapi.WorkState{
				{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "processing", Type: factoryapi.WorkStateTypePROCESSING},
				{Name: "review", Type: factoryapi.WorkStateTypePROCESSING},
				{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
			}}},
		}})
	request := watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "request-retry", 2,
		factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{
			{WorkId: watchStringPtr("work-1"), WorkTypeName: watchStringPtr("task"), State: &factoryapi.WorkState{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL}},
		}})
	transition := watchTransitionEvent(t, "move-1", 3, "work-1", "ready", "processing", false)
	return metadata, request, transition
}

func watchRetryFollowSetup(t *testing.T) (factoryapi.FactoryEvent, factoryapi.FactoryEvent, factoryapi.FactoryEvent, factoryapi.FactoryEvent) {
	t.Helper()
	metadata, _, first := watchRetrySetup(t)
	request := watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "request-follow-retry", 2,
		factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{
			{WorkId: watchStringPtr("work-1"), WorkTypeName: watchStringPtr("task"), State: &factoryapi.WorkState{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL}},
			{WorkId: watchStringPtr("work-2"), WorkTypeName: watchStringPtr("task"), State: &factoryapi.WorkState{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL}},
		}})
	later := watchTransitionEvent(t, "move-later", 7, "work-2", "ready", "processing", false)
	return metadata, request, first, later
}

func watchModelRequestEvent(t *testing.T, id string, sequence int, model string) factoryapi.FactoryEvent {
	t.Helper()
	payload := factoryapi.ModelRequestEventPayload{
		ModelRequestId:   "dispatch-retry/model-request/1",
		Attempt:          1,
		Operation:        "GENERATE",
		Worker:           "worker-retry",
		Model:            model,
		ProviderLocality: "CLOUD",
		WorkingDirectory: watchStringPtr("/factory/worktrees/work-1"),
		Worktree:         watchStringPtr("/factory/worktrees/work-1"),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromModelRequestEventPayload(payload); err != nil {
		t.Fatalf("encode model request event: %v", err)
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeModelRequest,
		Id:            id,
		Context:       watchModelEventContext(sequence),
		Payload:       union,
	}
}

func watchModelResponseEvent(t *testing.T, id string, sequence int, model string) factoryapi.FactoryEvent {
	t.Helper()
	payload := factoryapi.ModelResponseEventPayload{
		ModelRequestId:   "dispatch-retry/model-request/1",
		Attempt:          1,
		Operation:        "GENERATE",
		Worker:           "worker-retry",
		Model:            model,
		ProviderLocality: "CLOUD",
		Outcome:          factoryapi.InferenceOutcomeFailed,
		DurationMillis:   12,
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromModelResponseEventPayload(payload); err != nil {
		t.Fatalf("encode model response event: %v", err)
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeModelResponse,
		Id:            id,
		Context:       watchModelEventContext(sequence),
		Payload:       union,
	}
}

func watchNonProjectingEvent(
	t *testing.T,
	eventType factoryapi.FactoryEventType,
	id string,
	sequence int,
	revision string,
) factoryapi.FactoryEvent {
	t.Helper()
	switch eventType {
	case factoryapi.FactoryEventTypeModelRequest:
		return watchModelRequestEvent(t, id, sequence, revision)
	case factoryapi.FactoryEventTypeModelResponse:
		return watchModelResponseEvent(t, id, sequence, revision)
	default:
		context := watchModelEventContext(sequence)
		context.CurrentChainingTraceId = watchStringPtr(revision)
		return factoryapi.FactoryEvent{
			SchemaVersion: factoryapi.AgentFactoryEventV1,
			Type:          eventType,
			Id:            id,
			Context:       context,
		}
	}
}

func watchModelEventContext(sequence int) factoryapi.FactoryEventContext {
	return factoryapi.FactoryEventContext{
		EventTime:  watchEventContext(sequence).EventTime,
		Sequence:   sequence,
		Tick:       11,
		DispatchId: watchStringPtr("dispatch-retry"),
		RequestId:  watchStringPtr("request-retry"),
		TraceIds:   watchStringSlicePtr("trace-retry"),
		WorkIds:    watchStringSlicePtr("work-1"),
	}
}

func assertModelRequestRetryShape(t *testing.T, initial, refreshed factoryapi.FactoryEvent) {
	t.Helper()
	initialPayload, err := initial.Payload.AsModelRequestEventPayload()
	if err != nil {
		t.Fatalf("decode initial model request: %v", err)
	}
	refreshedPayload, err := refreshed.Payload.AsModelRequestEventPayload()
	if err != nil {
		t.Fatalf("decode refreshed model request: %v", err)
	}
	if initial.Id != refreshed.Id || initialPayload.ModelRequestId != refreshedPayload.ModelRequestId ||
		initialPayload.Attempt != refreshedPayload.Attempt {
		t.Fatalf("retry identities = %#v / %#v, want same event/request identity and attempt", initial, refreshed)
	}
	if refreshed.Context.Sequence <= initial.Context.Sequence || !refreshed.Context.EventTime.After(initial.Context.EventTime) {
		t.Fatalf("retry canonical position = %d/%s then %d/%s, want later sequence and event time", initial.Context.Sequence, initial.Context.EventTime, refreshed.Context.Sequence, refreshed.Context.EventTime)
	}
}

func watchStringSlicePtr(values ...string) *[]string {
	return &values
}

func TestRenderWatchTransitionPreservesNativeStructuredResultValues(t *testing.T) {
	base := WatchTransition{
		SessionID:               "session-1",
		EventID:                 "event-1",
		Sequence:                1,
		EventTime:               time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC),
		WorkID:                  "work-1",
		WorkTypeName:            "task",
		FromState:               "init",
		ToState:                 "complete",
		Source:                  "worker",
		Terminal:                true,
		StructuredResultPresent: true,
	}
	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "object", value: map[string]any{"z": float64(2), "a": "first"}, want: `{"a":"first","z":2}`},
		{name: "explicit null", value: json.RawMessage("null"), want: "null"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			transition := base
			transition.EventID = "event-" + test.name
			transition.StructuredResult = test.value
			if err := RenderWatchTransition(&output, transition); err != nil {
				t.Fatalf("RenderWatchTransition() error = %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &fields); err != nil {
				t.Fatalf("decode line: %v", err)
			}
			if got := string(fields["structuredResult"]); got != test.want {
				t.Fatalf("structuredResult = %s, want %s", got, test.want)
			}
		})
	}
}

func TestDecodeWatchSSEEventPreservesExplicitStructuredResultNull(t *testing.T) {
	event := watchFactoryEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "dispatch-response", 3,
		factoryapi.DispatchResponseEventPayload{
			Outcome:          factoryapi.WorkOutcomeAccepted,
			StructuredResult: json.RawMessage("null"),
		})
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("encode SSE event: %v", err)
	}
	decoded, err := decodeWatchSSEEvent([]string{string(payload)})
	if err != nil {
		t.Fatalf("decode SSE event: %v", err)
	}
	response, err := decoded.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode dispatch response payload: %v", err)
	}
	value, ok := response.StructuredResult.(string)
	if !ok || value != watchStructuredResultNullMarker {
		t.Fatalf("structuredResult = %#v (marker=%t), want internal explicit-null marker", response.StructuredResult, ok)
	}
}

func TestWatchReducerAttachesDispatchStructuredResultToOneFollowingTransition(t *testing.T) {
	reducer := newWatchReducer("session-1")
	metadata := watchFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory", 1,
		factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{Name: "task", States: []factoryapi.WorkState{
				{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "processing", Type: factoryapi.WorkStateTypePROCESSING},
				{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
			}}},
		}})
	request := watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "request", 2,
		factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{{
			WorkId: watchStringPtr("work-1"), WorkTypeName: watchStringPtr("task"),
		}}})
	structured := map[string]any{"decision": "accept", "score": float64(2)}
	response := watchFactoryEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "dispatch-response", 3,
		factoryapi.DispatchResponseEventPayload{
			Outcome: factoryapi.WorkOutcomeAccepted,
			OutputWork: &[]factoryapi.Work{{
				WorkId:           watchStringPtr("work-1"),
				WorkTypeName:     watchStringPtr("task"),
				StructuredResult: structured,
			}},
		})
	firstTransition := watchTransitionEvent(t, "move-1", 4, "work-1", "ready", "processing", false)
	secondTransition := watchTransitionEvent(t, "move-2", 5, "work-1", "processing", "done", true)

	for _, event := range []factoryapi.FactoryEvent{metadata, request, response} {
		if _, emit, _, err := reducer.Accept(event); err != nil || emit {
			t.Fatalf("Accept(%q): emit=%t error=%v, want no output line", event.Id, emit, err)
		}
	}
	transition, emit, _, err := reducer.Accept(firstTransition)
	if err != nil || !emit {
		t.Fatalf("first transition: emit=%t error=%v, want output line", emit, err)
	}
	if !transition.StructuredResultPresent || transition.StructuredResult == nil {
		t.Fatalf("first transition structuredResult = %#v (present=%t), want native result", transition.StructuredResult, transition.StructuredResultPresent)
	}
	if !reflect.DeepEqual(transition.StructuredResult, structured) {
		t.Fatalf("first transition structuredResult = %#v, want %#v", transition.StructuredResult, structured)
	}
	transition, emit, _, err = reducer.Accept(secondTransition)
	if err != nil || !emit {
		t.Fatalf("second transition: emit=%t error=%v, want output line", emit, err)
	}
	if transition.StructuredResultPresent {
		t.Fatalf("second transition structuredResult = %#v, want omitted after first handoff", transition.StructuredResult)
	}
}

func TestWatchReducerOmitsStructuredResultFromFailedDispatchTransition(t *testing.T) {
	reducer := newWatchReducer("session-1")
	metadata := watchFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory", 1,
		factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{Name: "task", States: []factoryapi.WorkState{
				{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
			}}},
		}})
	request := watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "request", 2,
		factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{{
			WorkId: watchStringPtr("work-1"), WorkTypeName: watchStringPtr("task"),
		}}})
	accepted := watchFactoryEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "dispatch-accepted", 3,
		factoryapi.DispatchResponseEventPayload{
			Outcome: factoryapi.WorkOutcomeAccepted,
			OutputWork: &[]factoryapi.Work{{
				WorkId:           watchStringPtr("work-1"),
				StructuredResult: map[string]any{"decision": "accept"},
			}},
		})
	failed := watchFactoryEvent(t, factoryapi.FactoryEventTypeDispatchResponse, "dispatch-failed", 4,
		factoryapi.DispatchResponseEventPayload{
			Outcome: factoryapi.WorkOutcomeFailed,
			OutputWork: &[]factoryapi.Work{{
				WorkId: watchStringPtr("work-1"),
			}},
		})
	transitionEvent := watchTransitionEvent(t, "move-failed", 5, "work-1", "ready", "failed", true)
	for _, event := range []factoryapi.FactoryEvent{metadata, request, accepted, failed} {
		if _, emit, _, err := reducer.Accept(event); err != nil || emit {
			t.Fatalf("Accept(%q): emit=%t error=%v, want no output line", event.Id, emit, err)
		}
	}
	transition, emit, _, err := reducer.Accept(transitionEvent)
	if err != nil || !emit {
		t.Fatalf("failed transition: emit=%t error=%v, want output line", emit, err)
	}
	if transition.StructuredResultPresent {
		t.Fatalf("failed transition structuredResult = %#v, want omitted", transition.StructuredResult)
	}
}
