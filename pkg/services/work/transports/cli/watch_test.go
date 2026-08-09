package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestRenderWatchTransitionWritesExactEscapedNDJSONContract(t *testing.T) {
	eventTime := time.Date(2026, time.August, 8, 20, 21, 22, 123456789, time.UTC)
	var output bytes.Buffer
	err := RenderWatchTransition(&output, WatchTransition{
		SessionID:     "session/\"beta\"",
		EventID:       "factory-event/\"move\"",
		Sequence:      12,
		EventTime:     eventTime,
		WorkID:        "work\\alpha",
		WorkTypeName:  "review\nitem",
		FromState:     "in-review",
		ToState:       "to-complete",
		Source:        "operator/api",
		Terminal:      false,
		TriggerWorkID: "trigger-1",
		Reason:        "line one\nline two",
	})
	if err != nil {
		t.Fatalf("RenderWatchTransition() error = %v", err)
	}

	if !strings.HasSuffix(output.String(), "\n") || strings.Count(output.String(), "\n") != 1 {
		t.Fatalf("output = %q, want exactly one newline-terminated line", output.String())
	}
	assertWatchLineHasExactFields(t, output.Bytes())
	assertDecodedWatchLineMatchesEscapedTransition(t, output.Bytes(), eventTime)
}

func assertWatchLineHasExactFields(t *testing.T, line []byte) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(line), &fields); err != nil {
		t.Fatalf("decode rendered line: %v\n%s", err, line)
	}
	wantFields := []string{
		"schemaVersion", "sessionId", "eventId", "sequence", "eventTime",
		"workId", "workTypeName", "fromState", "toState", "source", "terminal",
		"triggerWorkId", "reason",
	}
	if len(fields) != len(wantFields) {
		t.Fatalf("field count = %d, want %d (%v)", len(fields), len(wantFields), fields)
	}
	for _, field := range wantFields {
		if _, ok := fields[field]; !ok {
			t.Fatalf("rendered line missing field %q", field)
		}
	}
}

func assertDecodedWatchLineMatchesEscapedTransition(t *testing.T, line []byte, eventTime time.Time) {
	t.Helper()
	var got watchLine
	if err := json.Unmarshal(bytes.TrimSpace(line), &got); err != nil {
		t.Fatalf("decode typed rendered line: %v", err)
	}
	if got.SchemaVersion != WatchSchemaVersion || got.SessionID != "session/\"beta\"" ||
		got.EventID != "factory-event/\"move\"" || got.Sequence != 12 ||
		!got.EventTime.Equal(eventTime) || got.WorkID != "work\\alpha" ||
		got.WorkTypeName != "review\nitem" || got.TriggerWorkID != "trigger-1" ||
		got.Reason != "line one\nline two" {
		t.Fatalf("decoded line = %#v, want escaped transition values", got)
	}
}

func TestRenderWatchTransitionOmitsAbsentOptionalFieldsAndWritesOnce(t *testing.T) {
	output := &countingWriter{}
	err := RenderWatchTransition(output, WatchTransition{
		SessionID:    "session-1",
		EventID:      "event-1",
		Sequence:     0,
		EventTime:    time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC),
		WorkID:       "work-1",
		WorkTypeName: "task",
		FromState:    "init",
		ToState:      "complete",
		Source:       "worker",
		Terminal:     true,
	})
	if err != nil {
		t.Fatalf("RenderWatchTransition() error = %v", err)
	}
	if output.calls != 1 {
		t.Fatalf("writer calls = %d, want one atomic line write", output.calls)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &fields); err != nil {
		t.Fatalf("decode rendered line: %v", err)
	}
	if _, ok := fields["triggerWorkId"]; ok {
		t.Fatal("rendered line contains absent triggerWorkId")
	}
	if _, ok := fields["reason"]; ok {
		t.Fatal("rendered line contains absent reason")
	}
}

func TestRenderWatchTransitionRejectsInvalidInputBeforeWriting(t *testing.T) {
	output := &countingWriter{}
	err := RenderWatchTransition(output, WatchTransition{
		SessionID:    "session-1",
		EventID:      "event-1",
		EventTime:    time.Now().UTC(),
		WorkID:       "work-1",
		WorkTypeName: "task",
		FromState:    "init",
		ToState:      "complete",
		Source:       "worker",
		Sequence:     -1,
	})
	if err == nil || !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("error = %v, want non-negative sequence validation", err)
	}
	if output.calls != 0 {
		t.Fatalf("writer calls = %d, want zero on validation failure", output.calls)
	}
}

func TestValidateWatchConfigDistinguishesDefaultAndEmptyExplicitSession(t *testing.T) {
	base := WatchConfig{Context: context.Background(), Output: io.Discard}
	if err := ValidateWatchConfig(base); err != nil {
		t.Fatalf("default session config error = %v", err)
	}
	base.SessionID = "session-beta"
	base.SessionIDExplicit = true
	if err := ValidateWatchConfig(base); err != nil {
		t.Fatalf("explicit session config error = %v", err)
	}
	base.SessionID = ""
	if err := ValidateWatchConfig(base); err == nil || !strings.Contains(err.Error(), "--session") {
		t.Fatalf("empty explicit session error = %v, want actionable --session error", err)
	}
}

func TestRenderWatchTransitionReportsShortWrite(t *testing.T) {
	err := RenderWatchTransition(shortWriter{}, WatchTransition{
		SessionID:    "session-1",
		EventID:      "event-1",
		EventTime:    time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC),
		WorkID:       "work-1",
		WorkTypeName: "task",
		FromState:    "init",
		ToState:      "complete",
		Source:       "worker",
	})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("error = %v, want io.ErrShortWrite", err)
	}
}

type countingWriter struct {
	bytes.Buffer
	calls int
}

func (w *countingWriter) Write(payload []byte) (int, error) {
	w.calls++
	return w.Buffer.Write(payload)
}

type shortWriter struct{}

func (shortWriter) Write(payload []byte) (int, error) {
	return len(payload) - 1, nil
}

func TestWatchReducerProjectsOrderedTransitionsAndUsesTerminalMetadata(t *testing.T) {
	terminal := factoryapi.WorkState{Name: "shipped", Type: factoryapi.WorkStateTypeTERMINAL}
	factoryEvent := watchFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory", 1,
		factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{
				Name: "task",
				States: []factoryapi.WorkState{
					{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
					{Name: "processing", Type: factoryapi.WorkStateTypePROCESSING},
					terminal,
				},
			}},
		}})
	workRequest := watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "request", 2,
		factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{
			{WorkId: watchStringPtr("work-1"), WorkTypeName: watchStringPtr("task"), State: &factoryapi.WorkState{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL}},
			{WorkId: watchStringPtr("work-2"), WorkTypeName: watchStringPtr("task"), State: &factoryapi.WorkState{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL}},
		}})
	events := []factoryapi.FactoryEvent{
		factoryEvent,
		workRequest,
		{Id: "dispatch", Type: factoryapi.FactoryEventTypeDispatchRequest, Context: watchEventContext(3)},
		watchTransitionEvent(t, "move-1", 4, "work-1", "ready", "processing", false),
		watchTransitionEvent(t, "move-2", 5, "work-2", "ready", "shipped", true),
		watchTransitionEvent(t, "move-3", 6, "work-1", "processing", "shipped", true),
	}

	reducer := newWatchReducer("session-1")
	var transitions []WatchTransition
	for _, event := range events {
		transition, emit, completed, err := reducer.Accept(event)
		if err != nil {
			t.Fatalf("Accept(%q) error = %v", event.Id, err)
		}
		if emit {
			transitions = append(transitions, transition)
		}
		if event.Id == "move-2" && completed {
			t.Fatal("reducer completed before the second Work reached terminal")
		}
	}
	if len(transitions) != 3 {
		t.Fatalf("transition count = %d, want 3", len(transitions))
	}
	for index, transition := range transitions {
		wantSequence := int64(index + 4)
		if transition.Sequence != wantSequence {
			t.Fatalf("transition[%d].Sequence = %d, want %d", index, transition.Sequence, wantSequence)
		}
	}
	if transitions[0].WorkID != "work-1" || transitions[1].WorkID != "work-2" || transitions[2].WorkID != "work-1" {
		t.Fatalf("interleaved Work IDs = %#v, want work-1, work-2, work-1", transitions)
	}
	if !transitions[1].Terminal || !transitions[2].Terminal || !reducer.Completed() {
		t.Fatalf("terminal projection = %#v, reducer completed = %t", transitions, reducer.Completed())
	}
}

func TestWatchReducerSuppressesExactDuplicatesAndRejectsOrderingConflicts(t *testing.T) {
	reducer := newWatchReducer("session-1")
	metadata := watchFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory", 1,
		factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{Name: "task", States: []factoryapi.WorkState{{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL}}}},
		}})
	if _, _, _, err := reducer.Accept(metadata); err != nil {
		t.Fatalf("Accept(metadata) error = %v", err)
	}
	transition := watchTransitionEvent(t, "move-1", 2, "work-1", "ready", "done", true)
	if _, emit, _, err := reducer.Accept(transition); err != nil || !emit {
		t.Fatalf("first transition: emit=%t error=%v, want emitted", emit, err)
	}
	if _, emit, _, err := reducer.Accept(transition); err != nil || emit {
		t.Fatalf("duplicate transition: emit=%t error=%v, want suppressed", emit, err)
	}
	conflictingID := transition
	conflictingID.Context.Sequence = 3
	if _, _, _, err := reducer.Accept(conflictingID); err == nil || !strings.Contains(err.Error(), "reused with conflicting") {
		t.Fatalf("conflicting event ID error = %v, want explicit conflict", err)
	}
	regressed := watchTransitionEvent(t, "move-2", 1, "work-1", "ready", "done", true)
	if _, _, _, err := reducer.Accept(regressed); err == nil || !strings.Contains(err.Error(), "regressed") {
		t.Fatalf("regressed sequence error = %v, want explicit ordering failure", err)
	}
}

func TestWatchReducerAlreadyTerminalRetainedCohortCompletesWithoutTransition(t *testing.T) {
	reducer := newWatchReducer("session-1")
	metadata := watchFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory", 1,
		factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{Name: "task", States: []factoryapi.WorkState{{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL}}}},
		}})
	request := watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "request", 2,
		factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{
			{WorkId: watchStringPtr("work-1"), WorkTypeName: watchStringPtr("task"), State: &factoryapi.WorkState{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL}},
		}})
	if _, _, completed, err := reducer.Accept(metadata); err != nil || completed {
		t.Fatalf("metadata: completed=%t error=%v, want incomplete", completed, err)
	}
	if _, emit, completed, err := reducer.Accept(request); err != nil || emit || !completed {
		t.Fatalf("terminal retained request: emit=%t completed=%t error=%v", emit, completed, err)
	}
}

func TestWatchFiniteStreamWritesFinalTransitionBeforeReturning(t *testing.T) {
	metadata := watchFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory", 1,
		factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{Name: "task", States: []factoryapi.WorkState{
				{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
			}}},
		}})
	request := watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "request", 2,
		factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{{
			WorkId: watchStringPtr("work-1"), WorkTypeName: watchStringPtr("task"), State: &factoryapi.WorkState{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
		}}})
	stream := &finiteWatchEventStream{events: []factoryapi.FactoryEvent{
		metadata, request, watchTransitionEvent(t, "move-1", 3, "work-1", "ready", "done", true),
	}}
	var output bytes.Buffer
	err := watchWithSource(WatchConfig{Context: context.Background(), SessionID: "session-1", Output: &output}, watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
		return stream, nil
	}))
	if err != nil {
		t.Fatalf("watchWithSource() error = %v", err)
	}
	if stream.nextCalls != 3 || !stream.closed {
		t.Fatalf("stream calls=%d closed=%t, want three events and close", stream.nextCalls, stream.closed)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("output lines = %d, want one transition line: %q", len(lines), output.String())
	}
	var got watchLine
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("decode final transition: %v", err)
	}
	if got.EventID != "move-1" || got.Sequence != 3 || !got.Terminal {
		t.Fatalf("final line = %#v, want terminal move-1 at sequence 3", got)
	}
}

func TestWatchConsumesCanonicalSSEStreamUsingDefaultSession(t *testing.T) {
	metadata := watchFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory", 1,
		factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{Name: "task", States: []factoryapi.WorkState{
				{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
			}}},
		}})
	request := watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "request", 2,
		factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{{
			WorkId: watchStringPtr("work-1"), WorkTypeName: watchStringPtr("task"),
			State: &factoryapi.WorkState{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
		}}})
	transition := watchTransitionEvent(t, "move-1", 3, "work-1", "ready", "done", true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/factory-sessions/~default/events" ||
			r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("watch request = %s %s Accept=%q, want default Factory Event SSE route", r.Method, r.URL.Path, r.Header.Get("Accept"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Factory-Session-Retained-Event-Count", "3")
		w.WriteHeader(http.StatusOK)
		for _, event := range []factoryapi.FactoryEvent{metadata, request, transition} {
			payload, err := json.Marshal(event)
			if err != nil {
				t.Errorf("encode SSE event %q: %v", event.Id, err)
				return
			}
			if _, err := io.WriteString(w, "data: "+string(payload)+"\n\n"); err != nil {
				t.Errorf("write SSE event %q: %v", event.Id, err)
				return
			}
		}
	}))
	defer server.Close()
	transport, err := clihttp.NewProtocol(server.Client(), watchTestHTTPClock{})
	if err != nil {
		t.Fatalf("build watch HTTP protocol: %v", err)
	}

	var output bytes.Buffer
	err = Watch(WatchConfig{
		Context: context.Background(),
		Server:  server.URL,
		Output:  &output,
		HTTP:    transport,
	})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	var got watchLine
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &got); err != nil {
		t.Fatalf("decode streamed transition: %v", err)
	}
	if got.SessionID != "~default" || got.EventID != "move-1" || got.Sequence != 3 || !got.Terminal {
		t.Fatalf("streamed transition = %#v, want terminal default-session move-1", got)
	}
}

func TestWatchFiniteConsumesEntireHTTPRetainedPrefixBeforeCompleting(t *testing.T) {
	metadata, requestA, _ := watchReconnectSetup(t)
	terminalA := watchTransitionEvent(t, "move-terminal", 3, "work-1", "ready", "done", true)
	requestB := watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "request-b", 4,
		factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{{
			WorkId: watchStringPtr("work-b"), WorkTypeName: watchStringPtr("task"),
			State: &factoryapi.WorkState{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
		}}})
	terminalB := watchTransitionEvent(t, "move-b", 5, "work-b", "ready", "done", true)
	events := []factoryapi.FactoryEvent{metadata, requestA, terminalA, requestB, terminalB}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Factory-Session-Retained-Event-Count", "5")
		w.WriteHeader(http.StatusOK)
		for _, event := range events {
			payload, err := json.Marshal(event)
			if err != nil {
				t.Errorf("encode retained event %q: %v", event.Id, err)
				return
			}
			if _, err := io.WriteString(w, "data: "+string(payload)+"\n\n"); err != nil {
				t.Errorf("write retained event %q: %v", event.Id, err)
				return
			}
		}
	}))
	defer server.Close()
	transport, err := clihttp.NewProtocol(server.Client(), watchTestHTTPClock{})
	if err != nil {
		t.Fatalf("build watch HTTP protocol: %v", err)
	}

	var output bytes.Buffer
	err = Watch(WatchConfig{
		Context: context.Background(),
		Server:  server.URL,
		Output:  &output,
		HTTP:    transport,
	})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output lines = %d, want both retained terminal transitions: %q", len(lines), output.String())
	}
	var first, second watchLine
	if err := decodeWatchLine(lines[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := decodeWatchLine(lines[1], &second); err != nil {
		t.Fatal(err)
	}
	if first.EventID != "move-terminal" || second.EventID != "move-b" ||
		first.Sequence >= second.Sequence || !first.Terminal || !second.Terminal {
		t.Fatalf("retained transitions = %#v, %#v, want ordered terminal A then terminal B", first, second)
	}
}

func TestWatchRejectsNegativeRetainedEventCount(t *testing.T) {
	stream := &finiteWatchEventStream{retainedEventCount: -1}
	err := watchWithSource(WatchConfig{Context: context.Background(), Output: io.Discard}, watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
		return stream, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "negative retained event count") {
		t.Fatalf("watch error = %v, want retained-count validation failure", err)
	}
}

type watchTestHTTPClock struct{}

func (watchTestHTTPClock) Now() time.Time { return time.Unix(0, 0) }

type finiteWatchEventStream struct {
	events             []factoryapi.FactoryEvent
	retainedEventCount int
	nextCalls          int
	closed             bool
}

func (stream *finiteWatchEventStream) Next(ctx context.Context) (factoryapi.FactoryEvent, error) {
	stream.nextCalls++
	if err := ctx.Err(); err != nil {
		return factoryapi.FactoryEvent{}, err
	}
	if len(stream.events) == 0 {
		return factoryapi.FactoryEvent{}, io.EOF
	}
	event := stream.events[0]
	stream.events = stream.events[1:]
	return event, nil
}

func (stream *finiteWatchEventStream) Close() error {
	stream.closed = true
	return nil
}

func (stream *finiteWatchEventStream) RetainedEventCount() int { return stream.retainedEventCount }

func TestNilHTTPWatchEventStreamHasNoRetainedEvents(t *testing.T) {
	var stream *httpWatchEventStream
	if got := stream.RetainedEventCount(); got != 0 {
		t.Fatalf("nil stream retained count = %d, want zero", got)
	}
}

func watchFactoryEvent(t *testing.T, eventType factoryapi.FactoryEventType, id string, sequence int, payload any) factoryapi.FactoryEvent {
	t.Helper()
	var union factoryapi.FactoryEvent_Payload
	var err error
	switch typed := payload.(type) {
	case factoryapi.InitialStructureRequestEventPayload:
		err = union.FromInitialStructureRequestEventPayload(typed)
	case factoryapi.WorkRequestEventPayload:
		err = union.FromWorkRequestEventPayload(typed)
	case factoryapi.WorkStateChangeEventPayload:
		err = union.FromWorkStateChangeEventPayload(typed)
	default:
		t.Fatalf("unsupported watch test payload %T", payload)
	}
	if err != nil {
		t.Fatalf("encode watch test payload: %v", err)
	}
	return factoryapi.FactoryEvent{
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          eventType,
		Id:            id,
		Context:       watchEventContext(sequence),
		Payload:       union,
	}
}

func watchTransitionEvent(t *testing.T, id string, sequence int, workID, fromState, toState string, terminal bool) factoryapi.FactoryEvent {
	t.Helper()
	return watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkStateChange, id, sequence,
		factoryapi.WorkStateChangeEventPayload{
			WorkId: workID, WorkTypeName: "task", FromState: fromState, ToState: toState,
			Source: factoryapi.WorkStateChangeSourceCLI,
			Reason: watchOptionalString(terminal, "finished"),
		})
}

func watchEventContext(sequence int) factoryapi.FactoryEventContext {
	return factoryapi.FactoryEventContext{
		EventTime: time.Date(2026, time.August, 8, 20, 0, sequence, 0, time.UTC),
		Sequence:  sequence,
	}
}

func watchStringPtr(value string) *string { return &value }
func watchOptionalString(include bool, value string) *string {
	if !include {
		return nil
	}
	return &value
}

func TestWatchFollowContinuesAfterTerminalUntilCancellation(t *testing.T) {
	metadata, request, terminal, later := watchFollowSetup(t)
	stream := &cancellableWatchEventStream{
		events:  []factoryapi.FactoryEvent{metadata, request, terminal, later},
		blocked: make(chan struct{}),
		closed:  make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- watchWithRetry(
			WatchConfig{Context: ctx, SessionID: "session-follow", Follow: true, Output: &output},
			watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
				return stream, nil
			}),
			watchRetryPolicy{maxAttempts: 0},
		)
	}()

	select {
	case <-stream.blocked:
	case <-time.After(time.Second):
		t.Fatal("follow watch did not remain attached after terminal transition")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("watch error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("follow watch did not cancel while reading")
	}
	select {
	case <-stream.closed:
	default:
		t.Fatal("cancellation did not close the active event stream")
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output lines = %d, want terminal plus later follow line: %q", len(lines), output.String())
	}
	var first, second watchLine
	if err := decodeWatchLine(lines[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := decodeWatchLine(lines[1], &second); err != nil {
		t.Fatal(err)
	}
	if first.EventID != "move-terminal" || !first.Terminal || second.EventID != "move-later" || second.Terminal {
		t.Fatalf("follow transitions = %#v, %#v, want terminal followed by non-terminal transition", first, second)
	}
}

func TestWatchCancellationInterruptsReconnectBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backoffStarted := make(chan struct{})
	var openCalls int
	done := make(chan error, 1)
	go func() {
		done <- watchWithRetry(
			WatchConfig{Context: ctx, SessionID: "session-backoff", Output: io.Discard},
			watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
				openCalls++
				return nil, errors.New("temporary transport disconnect")
			}),
			watchRetryPolicy{
				maxAttempts:  3,
				initialDelay: time.Hour,
				maximumDelay: time.Hour,
				wait: func(ctx context.Context, _ time.Duration) error {
					close(backoffStarted)
					<-ctx.Done()
					return ctx.Err()
				},
			},
		)
	}()

	select {
	case <-backoffStarted:
	case <-time.After(time.Second):
		t.Fatal("watch did not enter reconnect backoff")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("watch error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("watch did not cancel during reconnect backoff")
	}
	if openCalls != 1 {
		t.Fatalf("open calls = %d, want no open after cancellation", openCalls)
	}
}

func TestWatchReconnectsFromCursorAndSuppressesReplayOverlap(t *testing.T) {
	metadata, request, firstTransition := watchReconnectSetup(t)
	retryInitial := watchModelRequestEvent(t, "model-retry-reconnect", 4, "model-before-refresh")
	retryRefreshed := watchModelRequestEvent(t, "model-retry-reconnect", 4, "model-after-refresh")
	secondTransition := watchTransitionEvent(t, "move-2", 5, "work-1", "processing", "done", true)
	firstStream := &finiteWatchEventStream{events: []factoryapi.FactoryEvent{metadata, request, firstTransition, retryInitial}}
	secondStream := &finiteWatchEventStream{events: []factoryapi.FactoryEvent{firstTransition, retryRefreshed, secondTransition}}
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
	if cursors[1] == nil || cursors[1].EventID != "model-retry-reconnect" || cursors[1].Sequence != 4 {
		t.Fatalf("reconnect cursor = %#v, want model retry at sequence 4", cursors[1])
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

func TestWatchReconnectFailsOnRetentionGapCursor(t *testing.T) {
	metadata, request, firstTransition := watchReconnectSetup(t)
	firstStream := &finiteWatchEventStream{events: []factoryapi.FactoryEvent{metadata, request, firstTransition}}
	var openCalls int
	err := watchWithRetry(
		WatchConfig{Context: context.Background(), SessionID: "session-gap", Output: io.Discard},
		watchEventOpenFunc(func(_ context.Context, cursor *watchEventCursor) (watchEventStream, error) {
			openCalls++
			if cursor == nil {
				return firstStream, nil
			}
			return nil, &watchHTTPStatusError{
				sessionID: "session-gap",
				status:    http.StatusBadRequest,
				message:   "invalid event reconnect cursor",
			}
		}),
		watchRetryPolicy{maxAttempts: 2, wait: func(context.Context, time.Duration) error { return nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "invalid event reconnect cursor") || !strings.Contains(err.Error(), "400") {
		t.Fatalf("retention-gap error = %v, want actionable cursor failure", err)
	}
	if openCalls != 2 {
		t.Fatalf("open calls = %d, want one reconnect open", openCalls)
	}
}

func TestWatchNonRetryableSessionErrorsDoNotReconnect(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var openCalls, waits int
			err := watchWithRetry(
				WatchConfig{Context: context.Background(), SessionID: "session-status", Output: io.Discard},
				watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
					openCalls++
					return nil, &watchHTTPStatusError{sessionID: "session-status", status: status, message: http.StatusText(status)}
				}),
				watchRetryPolicy{
					maxAttempts: 3,
					wait: func(context.Context, time.Duration) error {
						waits++
						return nil
					},
				},
			)
			if err == nil || !strings.Contains(err.Error(), http.StatusText(status)) {
				t.Fatalf("status error = %v, want %s", err, http.StatusText(status))
			}
			if openCalls != 1 || waits != 0 {
				t.Fatalf("open calls/waits = %d/%d, want one non-retryable open failure", openCalls, waits)
			}
		})
	}
}

func TestWatchMalformedEventFailsWithoutReconnect(t *testing.T) {
	_, malformedErr := decodeWatchSSEEvent([]string{"{not-json"})
	if malformedErr == nil {
		t.Fatal("decodeWatchSSEEvent() unexpectedly accepted malformed JSON")
	}
	var openCalls int
	err := watchWithRetry(
		WatchConfig{Context: context.Background(), SessionID: "session-malformed", Output: io.Discard},
		watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
			openCalls++
			return &errorWatchEventStream{err: malformedErr}, nil
		}),
		watchRetryPolicy{maxAttempts: 3, wait: func(context.Context, time.Duration) error { return nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "decode canonical Factory Event SSE data") {
		t.Fatalf("malformed-event error = %v, want explicit decode failure", err)
	}
	if openCalls != 1 {
		t.Fatalf("open calls = %d, want no reconnect for malformed input", openCalls)
	}
}

func TestWatchMalformedTransitionFailsWithoutReconnect(t *testing.T) {
	metadata, _, _ := watchReconnectSetup(t)
	malformed := watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkStateChange, "malformed-transition", 2,
		factoryapi.WorkStateChangeEventPayload{
			WorkTypeName: "task",
			FromState:    "ready",
			ToState:      "done",
			Source:       factoryapi.WorkStateChangeSourceCLI,
		})
	var openCalls int
	err := watchWithRetry(
		WatchConfig{Context: context.Background(), SessionID: "session-malformed-transition", Output: io.Discard},
		watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
			openCalls++
			return &finiteWatchEventStream{events: []factoryapi.FactoryEvent{metadata, malformed}}, nil
		}),
		watchRetryPolicy{maxAttempts: 3, wait: func(context.Context, time.Duration) error { return nil }},
	)
	if err == nil || !strings.Contains(err.Error(), "workId is required") {
		t.Fatalf("malformed transition error = %v, want required workId failure", err)
	}
	if openCalls != 1 {
		t.Fatalf("open calls = %d, want no reconnect for malformed transition", openCalls)
	}
}

func TestWatchExhaustsBoundedReconnectAttempts(t *testing.T) {
	var openCalls, waits int
	err := watchWithRetry(
		WatchConfig{Context: context.Background(), SessionID: "session-exhausted", Output: io.Discard},
		watchEventOpenFunc(func(context.Context, *watchEventCursor) (watchEventStream, error) {
			openCalls++
			return &finiteWatchEventStream{}, nil
		}),
		watchRetryPolicy{
			maxAttempts: 2,
			wait: func(context.Context, time.Duration) error {
				waits++
				return nil
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "reconnect attempts exhausted") || !strings.Contains(err.Error(), "session-exhausted") {
		t.Fatalf("exhaustion error = %v, want bounded reconnect diagnostic", err)
	}
	if openCalls != 3 || waits != 2 {
		t.Fatalf("open calls/waits = %d/%d, want initial open plus two bounded retries", openCalls, waits)
	}
}

func TestOpenHTTPWatchEventStreamSendsReconnectCursor(t *testing.T) {
	requestSeen := make(chan struct{})
	var gotEventID, gotSequence string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEventID = r.URL.Query().Get("after_event_id")
		gotSequence = r.URL.Query().Get("after_sequence")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Factory-Session-Retained-Event-Count", "0")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(requestSeen)
	}))
	defer server.Close()
	transport, err := clihttp.NewProtocol(server.Client(), watchTestHTTPClock{})
	if err != nil {
		t.Fatalf("build watch HTTP protocol: %v", err)
	}
	stream, err := openHTTPWatchEventStream(
		context.Background(), transport, server.URL, "session-cursor",
		&watchEventCursor{EventID: "move/1?retry", Sequence: 17}, io.Discard, false,
	)
	if err != nil {
		t.Fatalf("openHTTPWatchEventStream() error = %v", err)
	}
	defer stream.Close()
	select {
	case <-requestSeen:
	case <-time.After(time.Second):
		t.Fatal("HTTP server did not receive reconnect request")
	}
	if gotEventID != "move/1?retry" || gotSequence != "17" {
		t.Fatalf("reconnect query = eventId=%q sequence=%q, want encoded cursor values", gotEventID, gotSequence)
	}
}

func TestOpenHTTPWatchEventStreamValidatesRetainedEventCount(t *testing.T) {
	for _, retainedHeader := range []string{"", "not-a-count", "-1"} {
		t.Run(fmt.Sprintf("header=%q", retainedHeader), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if retainedHeader != "" {
					w.Header().Set("X-Factory-Session-Retained-Event-Count", retainedHeader)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			transport, err := clihttp.NewProtocol(server.Client(), watchTestHTTPClock{})
			if err != nil {
				t.Fatalf("build watch HTTP protocol: %v", err)
			}
			stream, err := openHTTPWatchEventStream(
				context.Background(), transport, server.URL, "session-header", nil, io.Discard, false,
			)
			if stream != nil {
				_ = stream.Close()
			}
			if err == nil || !strings.Contains(err.Error(), "X-Factory-Session-Retained-Event-Count") {
				t.Fatalf("open error = %v, want retained-count handshake failure", err)
			}
		})
	}
}

func watchReconnectSetup(t *testing.T) (factoryapi.FactoryEvent, factoryapi.FactoryEvent, factoryapi.FactoryEvent) {
	t.Helper()
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
			State: &factoryapi.WorkState{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
		}}})
	transition := watchTransitionEvent(t, "move-1", 3, "work-1", "ready", "processing", false)
	return metadata, request, transition
}

func watchFollowSetup(t *testing.T) (factoryapi.FactoryEvent, factoryapi.FactoryEvent, factoryapi.FactoryEvent, factoryapi.FactoryEvent) {
	t.Helper()
	metadata := watchFactoryEvent(t, factoryapi.FactoryEventTypeInitialStructureRequest, "factory-follow", 1,
		factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{Name: "task", States: []factoryapi.WorkState{
				{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "processing", Type: factoryapi.WorkStateTypePROCESSING},
				{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
			}}},
		}})
	request := watchFactoryEvent(t, factoryapi.FactoryEventTypeWorkRequest, "request-follow", 2,
		factoryapi.WorkRequestEventPayload{Works: &[]factoryapi.Work{
			{WorkId: watchStringPtr("work-1"), WorkTypeName: watchStringPtr("task"), State: &factoryapi.WorkState{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL}},
			{WorkId: watchStringPtr("work-2"), WorkTypeName: watchStringPtr("task"), State: &factoryapi.WorkState{Name: "ready", Type: factoryapi.WorkStateTypeINITIAL}},
		}})
	terminal := watchTransitionEvent(t, "move-terminal", 3, "work-1", "ready", "done", true)
	later := watchTransitionEvent(t, "move-later", 4, "work-2", "ready", "processing", false)
	return metadata, request, terminal, later
}

func decodeWatchLine(line string, destination *watchLine) error {
	return json.Unmarshal([]byte(line), destination)
}

type cancellableWatchEventStream struct {
	events    []factoryapi.FactoryEvent
	blocked   chan struct{}
	closed    chan struct{}
	blockOnce sync.Once
	closeOnce sync.Once
}

func (stream *cancellableWatchEventStream) Next(context.Context) (factoryapi.FactoryEvent, error) {
	if len(stream.events) > 0 {
		event := stream.events[0]
		stream.events = stream.events[1:]
		return event, nil
	}
	stream.blockOnce.Do(func() { close(stream.blocked) })
	<-stream.closed
	return factoryapi.FactoryEvent{}, io.EOF
}

func (stream *cancellableWatchEventStream) Close() error {
	stream.closeOnce.Do(func() { close(stream.closed) })
	return nil
}

func (stream *cancellableWatchEventStream) RetainedEventCount() int { return 0 }

type errorWatchEventStream struct {
	err error
}

func (stream *errorWatchEventStream) Next(context.Context) (factoryapi.FactoryEvent, error) {
	return factoryapi.FactoryEvent{}, stream.err
}

func (stream *errorWatchEventStream) Close() error { return nil }

func (stream *errorWatchEventStream) RetainedEventCount() int { return 0 }
