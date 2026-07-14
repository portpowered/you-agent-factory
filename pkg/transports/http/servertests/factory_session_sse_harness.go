package apiserver_test

import (
	"bufio"
	"context"
	"encoding/json"
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

var (
	errFactorySessionSSEHarnessTimeout   = errors.New("factory session SSE harness timeout elapsed")
	errFactorySessionSSEStreamClosed     = errors.New("factory session SSE stream closed")
	errFactorySessionSSEKeepaliveMissing = errors.New("factory session SSE keepalive not observed before timeout")
)

// FactorySessionSSEKeepaliveSignal records one authored keepalive observation on
// an idle session event stream.
type FactorySessionSSEKeepaliveSignal struct {
	ConnectionKeepAlive bool
	SSEComment          string
	OpenConnectionIdle  bool
}

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

// AssertConnectionKeepAlive fails when the stream response omits Connection keep-alive.
func (s *FactorySessionSSEStream) AssertConnectionKeepAlive() {
	s.t.Helper()
	if got := s.Response.Header.Get("Connection"); got != "keep-alive" {
		s.t.Fatalf("Connection header = %q, want keep-alive", got)
	}
}

// TryWaitForKeepalive waits for authored idle keepalive traffic within timeout.
// It accepts either SSE comment/heartbeat frames or an open idle connection that
// does not close before the timeout elapses.
func (s *FactorySessionSSEStream) TryWaitForKeepalive(timeout time.Duration) (FactorySessionSSEKeepaliveSignal, error) {
	s.t.Helper()
	if timeout <= 0 {
		timeout = s.timeout
	}
	signal := FactorySessionSSEKeepaliveSignal{
		ConnectionKeepAlive: s.Response.Header.Get("Connection") == "keep-alive",
	}
	if !signal.ConnectionKeepAlive {
		return signal, fmt.Errorf("%w: missing Connection keep-alive header", errFactorySessionSSEKeepaliveMissing)
	}

	if err := s.setReadDeadline(time.Now().Add(timeout)); err == nil {
		defer s.clearReadDeadline()
		frame, ok, readErr := tryReadNextSSEFrame(s.reader)
		if readErr != nil {
			if isFactorySessionSSEHarnessReadTimeout(readErr) {
				signal.OpenConnectionIdle = true
				return signal, nil
			}
			return signal, readErr
		}
		if !ok {
			return signal, errFactorySessionSSEStreamClosed
		}
		return factorySessionSSEKeepaliveSignalFromFrame(signal, frame)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	<-timer.C

	if err := s.setReadDeadline(time.Now().Add(25 * time.Millisecond)); err == nil {
		defer s.clearReadDeadline()
		_, ok, readErr := tryReadNextSSEFrame(s.reader)
		if readErr != nil && !isFactorySessionSSEHarnessReadTimeout(readErr) {
			return signal, readErr
		}
		if !ok && readErr == nil {
			return signal, errFactorySessionSSEStreamClosed
		}
	}

	signal.OpenConnectionIdle = true
	return signal, nil
}

func factorySessionSSEKeepaliveSignalFromFrame(
	signal FactorySessionSSEKeepaliveSignal,
	frame factorySessionSSEFrame,
) (FactorySessionSSEKeepaliveSignal, error) {
	switch frame.kind {
	case factorySessionSSEFrameComment:
		signal.SSEComment = frame.comment
		return signal, nil
	case factorySessionSSEFrameOther:
		signal.SSEComment = frame.comment
		return signal, nil
	case factorySessionSSEFrameData:
		return signal, fmt.Errorf("idle keepalive read factory event id %q", frame.event.Id)
	default:
		return signal, fmt.Errorf("unexpected idle keepalive frame kind %d", frame.kind)
	}
}

func (s *FactorySessionSSEStream) setReadDeadline(deadline time.Time) error {
	if s.Response == nil || s.Response.Body == nil {
		return errors.New("session SSE stream body is unavailable")
	}
	conn, ok := s.Response.Body.(interface {
		SetReadDeadline(time.Time) error
	})
	if !ok {
		return errors.New("session SSE stream body does not support read deadlines")
	}
	return conn.SetReadDeadline(deadline)
}

func (s *FactorySessionSSEStream) clearReadDeadline() {
	_ = s.setReadDeadline(time.Time{})
}

func isFactorySessionSSEHarnessReadTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr interface{ Timeout() bool }
	return errors.As(err, &netErr) && netErr.Timeout()
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

// GetSessionEvents issues GET /factory-sessions/{session_id}/events with an optional
// raw query string and Accept header, failing closed when the request exceeds the
// harness timeout.
func (h *FactorySessionSSEHarness) GetSessionEvents(
	serverURL, sessionID, query, accept string,
) *http.Response {
	h.t.Helper()

	path := "/factory-sessions/" + sessionID + "/events"
	if query != "" {
		path += "?" + query
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+path, nil)
	if err != nil {
		h.t.Fatalf("new session events request: %v", err)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

// ProbeRecovery issues the JSON reconnect probe for one session event stream and
// decodes the structured recovery outcome.
func (h *FactorySessionSSEHarness) ProbeRecovery(
	serverURL, sessionID, query string,
) (factoryapi.FactorySessionEventStreamRecovery, *http.Response) {
	h.t.Helper()

	resp := h.GetSessionEvents(serverURL, sessionID, query, "application/json")
	var recovery factoryapi.FactorySessionEventStreamRecovery
	if err := json.NewDecoder(resp.Body).Decode(&recovery); err != nil {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		h.t.Fatalf("decode recovery probe response: %v: %s", err, string(body))
	}
	return recovery, resp
}

type factorySessionSSEFrameKind int

const (
	factorySessionSSEFrameComment factorySessionSSEFrameKind = iota
	factorySessionSSEFrameData
	factorySessionSSEFrameOther
)

type factorySessionSSEFrame struct {
	kind    factorySessionSSEFrameKind
	comment string
	event   factoryapi.FactoryEvent
}

func tryReadNextSSEFrame(reader *bufio.Reader) (factorySessionSSEFrame, bool, error) {
	var dataLine string
	var commentLine string
	var eventName string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if dataLine == "" && commentLine == "" && eventName == "" {
				return factorySessionSSEFrame{}, false, nil
			}
			return factorySessionSSEFrame{}, false, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		switch {
		case strings.HasPrefix(line, ":"):
			commentLine = strings.TrimSpace(strings.TrimPrefix(line, ":"))
		case strings.HasPrefix(line, "data: "):
			dataLine = strings.TrimPrefix(line, "data: ")
		case strings.HasPrefix(line, "event: "):
			eventName = strings.TrimPrefix(line, "event: ")
		}
	}
	if dataLine != "" {
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(dataLine), &event); err != nil {
			return factorySessionSSEFrame{
				kind:    factorySessionSSEFrameOther,
				comment: firstNonEmptyFactorySessionSSEString(commentLine, eventName, dataLine),
			}, true, nil
		}
		return factorySessionSSEFrame{kind: factorySessionSSEFrameData, event: event}, true, nil
	}
	if commentLine != "" {
		return factorySessionSSEFrame{kind: factorySessionSSEFrameComment, comment: commentLine}, true, nil
	}
	if eventName != "" {
		return factorySessionSSEFrame{kind: factorySessionSSEFrameOther, comment: eventName}, true, nil
	}
	return factorySessionSSEFrame{}, false, nil
}

func firstNonEmptyFactorySessionSSEString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
