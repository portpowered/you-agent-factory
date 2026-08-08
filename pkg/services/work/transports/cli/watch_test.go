package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &fields); err != nil {
		t.Fatalf("decode rendered line: %v\n%s", err, output.String())
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
	var got watchLine
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &got); err != nil {
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

type watchTestHTTPClock struct{}

func (watchTestHTTPClock) Now() time.Time { return time.Unix(0, 0) }

type finiteWatchEventStream struct {
	events    []factoryapi.FactoryEvent
	nextCalls int
	closed    bool
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
