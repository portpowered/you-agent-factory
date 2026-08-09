package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const watchRetryModelRequestEventID = "factory-event/model-request/dispatch-retry/model-request/1"

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
	if first.EventID != "move-1" || second.EventID != "move-between-follow" || third.EventID != "move-later" ||
		first.Sequence >= second.Sequence || second.Sequence >= third.Sequence || first.Terminal || second.Terminal || third.Terminal {
		t.Fatalf("follow transitions = %#v, %#v, %#v, want ordered non-terminal transitions", first, second, third)
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
