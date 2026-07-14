package apiserver_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const defaultFactorySessionSSEHarnessTimeout = 5 * time.Second

var errFactorySessionSSEHarnessTimeout = errors.New("factory session SSE harness timeout elapsed")

// FactorySessionSSEHarness opens session-scoped GET /factory-sessions/{session_id}/events
// streams and reads SSE data frames within bounded timeouts.
type FactorySessionSSEHarness struct {
	t       *testing.T
	timeout time.Duration
}

// FactorySessionSSEStream is one open session event SSE connection.
type FactorySessionSSEStream struct {
	t        *testing.T
	timeout  time.Duration
	Response *http.Response
	reader   *bufio.Reader
	cancel   context.CancelFunc
}

// NewFactorySessionSSEHarness returns a harness that fails closed when a read
// exceeds the supplied timeout. Zero timeout uses the package default.
func NewFactorySessionSSEHarness(t *testing.T, timeout time.Duration) *FactorySessionSSEHarness {
	t.Helper()
	if timeout <= 0 {
		timeout = defaultFactorySessionSSEHarnessTimeout
	}
	return &FactorySessionSSEHarness{t: t, timeout: timeout}
}

// Open starts GET /factory-sessions/{sessionID}/events with an optional raw query
// string (without leading "?").
func (h *FactorySessionSSEHarness) Open(serverURL, sessionID, query string) *FactorySessionSSEStream {
	h.t.Helper()

	path := "/factory-sessions/" + sessionID + "/events"
	if query != "" {
		path += "?" + query
	}
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+path, nil)
	if err != nil {
		h.t.Fatalf("new session SSE request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		h.t.Fatalf("GET %s: %v", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		h.t.Fatalf("GET %s status = %d, want 200: %s", path, resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		cancel()
		_ = resp.Body.Close()
		h.t.Fatalf("GET %s Content-Type = %q, want text/event-stream", path, got)
	}
	return &FactorySessionSSEStream{
		t:        h.t,
		timeout:  h.timeout,
		Response: resp,
		reader:   bufio.NewReader(resp.Body),
		cancel:   cancel,
	}
}

// Close cancels the stream request context and closes the response body.
func (s *FactorySessionSSEStream) Close() {
	if s == nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.Response != nil && s.Response.Body != nil {
		_ = s.Response.Body.Close()
	}
}

// ReadNextEvent reads the next SSE data frame within the harness timeout and
// fails the test when the timeout elapses or the stream ends early.
func (s *FactorySessionSSEStream) ReadNextEvent() factoryapi.FactoryEvent {
	s.t.Helper()
	event, err := s.TryReadNextEvent(s.timeout)
	if err != nil {
		s.t.Fatalf("read session SSE factory event: %v", err)
	}
	return event
}

// TryReadNextEvent reads one SSE data frame within timeout, returning
// errFactorySessionSSEHarnessTimeout when no frame arrives in time.
func (s *FactorySessionSSEStream) TryReadNextEvent(timeout time.Duration) (factoryapi.FactoryEvent, error) {
	s.t.Helper()
	if timeout <= 0 {
		timeout = s.timeout
	}
	type readResult struct {
		event factoryapi.FactoryEvent
		err   error
	}
	done := make(chan readResult, 1)
	go func() {
		event, ok, err := tryReadSSEFactoryEvent(s.reader)
		if err != nil {
			done <- readResult{err: err}
			return
		}
		if !ok {
			done <- readResult{err: io.EOF}
			return
		}
		done <- readResult{event: event}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-done:
		return result.event, result.err
	case <-timer.C:
		return factoryapi.FactoryEvent{}, fmt.Errorf("%w after %s", errFactorySessionSSEHarnessTimeout, timeout)
	}
}

// ReadEvents reads count SSE data frames in order, failing closed on timeout.
func (s *FactorySessionSSEStream) ReadEvents(count int) []factoryapi.FactoryEvent {
	s.t.Helper()
	events := make([]factoryapi.FactoryEvent, 0, count)
	for len(events) < count {
		events = append(events, s.ReadNextEvent())
	}
	return events
}
