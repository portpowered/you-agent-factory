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
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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
	secondTransition := watchTransitionEvent(t, "move-2", 4, "work-1", "processing", "done", true)
	firstStream := &finiteWatchEventStream{events: []factoryapi.FactoryEvent{metadata, request, firstTransition}}
	secondStream := &finiteWatchEventStream{events: []factoryapi.FactoryEvent{firstTransition, secondTransition}}
	var cursors []*watchEventCursor
	var openCalls int
	var output bytes.Buffer
	err := watchWithRetry(
		WatchConfig{Context: context.Background(), SessionID: "session-reconnect", Output: &output},
		watchEventOpenFunc(func(_ context.Context, cursor *watchEventCursor) (watchEventStream, error) {
			openCalls++
			if cursor != nil {
				copy := *cursor
				cursors = append(cursors, &copy)
			} else {
				cursors = append(cursors, nil)
			}
			switch openCalls {
			case 1:
				return firstStream, nil
			case 2:
				return secondStream, nil
			default:
				return nil, errors.New("unexpected extra reconnect")
			}
		}),
		watchRetryPolicy{maxAttempts: 2, wait: func(context.Context, time.Duration) error { return nil }},
	)
	if err != nil {
		t.Fatalf("watchWithRetry() error = %v", err)
	}
	if openCalls != 2 || len(cursors) != 2 || cursors[0] != nil {
		t.Fatalf("open calls/cursors = %d/%#v, want initial open and one cursor reconnect", openCalls, cursors)
	}
	if cursors[1] == nil || cursors[1].EventID != "move-1" || cursors[1].Sequence != 3 {
		t.Fatalf("reconnect cursor = %#v, want move-1 at sequence 3", cursors[1])
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output lines = %d, want duplicate-free transitions: %q", len(lines), output.String())
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

type errorWatchEventStream struct {
	err error
}

func (stream *errorWatchEventStream) Next(context.Context) (factoryapi.FactoryEvent, error) {
	return factoryapi.FactoryEvent{}, stream.err
}

func (stream *errorWatchEventStream) Close() error { return nil }
