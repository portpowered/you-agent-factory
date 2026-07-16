package factorysessionsse

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	defaultFactorySessionSSEHarnessTimeout   = 5 * time.Second
	factorySessionSSEBackendScopeHeader      = "X-Factory-Session-Backend-Scope-Id"
	factorySessionSSELogicalSessionKeyHeader = "X-Factory-Session-Logical-Session-Key-Id"
	factorySessionSSEFactorySessionHeader    = "X-Factory-Session-Factory-Session-Id"
	factorySessionSSEStreamGenerationHeader  = "X-Factory-Session-Stream-Generation-Id"
)

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

// FactorySessionSSEStreamIdentity preserves the four independent identities
// returned by a successful session event stream handshake.
type FactorySessionSSEStreamIdentity struct {
	BackendScopeID      string
	LogicalSessionKeyID string
	FactorySessionID    string
	StreamGenerationID  string
}

// FactorySessionSSECheckpoint identifies the last FactoryEvent acknowledged by
// a session stream consumer. AfterEventID takes precedence when both fields are
// supplied, matching the public API contract.
type FactorySessionSSECheckpoint struct {
	AfterEventID  string
	AfterSequence *int
}

// FactorySessionSSEFrame is one complete SSE protocol frame. FactoryEvent is
// populated only when Data contains a valid generated FactoryEvent contract.
type FactorySessionSSEFrame struct {
	ID           string
	Event        string
	Data         string
	Comment      string
	FactoryEvent *factoryapi.FactoryEvent

	kind factorySessionSSEFrameKind
}

// FactorySessionSSEParseError reports invalid FactoryEvent JSON together with
// the complete protocol frame that caused the failure.
type FactorySessionSSEParseError struct {
	Frame FactorySessionSSEFrame
	Err   error
}

func (e *FactorySessionSSEParseError) Error() string {
	return fmt.Sprintf("decode FactoryEvent from SSE frame: %v", e.Err)
}

func (e *FactorySessionSSEParseError) Unwrap() error { return e.Err }

type factorySessionSSEReadResult struct {
	frame FactorySessionSSEFrame
	ok    bool
	err   error
}

// FactorySessionSSEStream is one open session event SSE connection.
type FactorySessionSSEStream struct {
	t         *testing.T
	timeout   time.Duration
	Response  *http.Response
	Identity  FactorySessionSSEStreamIdentity
	reader    *bufio.Reader
	cancel    context.CancelFunc
	pending   <-chan factorySessionSSEReadResult
	lastFrame FactorySessionSSEFrame
	hasFrame  bool
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
	req.Header.Set("Accept", "text/event-stream")
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
		Identity: factorySessionSSEIdentityFromHeader(resp.Header),
		reader:   bufio.NewReader(resp.Body),
		cancel:   cancel,
	}
}

// OpenFromCheckpoint resumes a session event stream after the supplied
// acknowledged FactoryEvent checkpoint.
func (h *FactorySessionSSEHarness) OpenFromCheckpoint(
	serverURL, sessionID string,
	checkpoint FactorySessionSSECheckpoint,
) *FactorySessionSSEStream {
	h.t.Helper()

	query := url.Values{}
	if checkpoint.AfterEventID != "" {
		query.Set("after_event_id", checkpoint.AfterEventID)
	}
	if checkpoint.AfterSequence != nil {
		query.Set("after_sequence", fmt.Sprint(*checkpoint.AfterSequence))
	}
	return h.Open(serverURL, sessionID, query.Encode())
}

func factorySessionSSEIdentityFromHeader(header http.Header) FactorySessionSSEStreamIdentity {
	return FactorySessionSSEStreamIdentity{
		BackendScopeID:      header.Get(factorySessionSSEBackendScopeHeader),
		LogicalSessionKeyID: header.Get(factorySessionSSELogicalSessionKeyHeader),
		FactorySessionID:    header.Get(factorySessionSSEFactorySessionHeader),
		StreamGenerationID:  header.Get(factorySessionSSEStreamGenerationHeader),
	}
}

// LastValidFrame returns the most recent completely parsed frame on this
// connection. A malformed data frame does not replace the last valid frame.
func (s *FactorySessionSSEStream) LastValidFrame() (FactorySessionSSEFrame, bool) {
	if s == nil || !s.hasFrame {
		return FactorySessionSSEFrame{}, false
	}
	return s.lastFrame, true
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
	frame FactorySessionSSEFrame,
) (FactorySessionSSEKeepaliveSignal, error) {
	switch frame.kind {
	case factorySessionSSEFrameComment:
		signal.SSEComment = frame.Comment
		return signal, nil
	case factorySessionSSEFrameOther:
		signal.SSEComment = firstNonEmptyFactorySessionSSEString(frame.Comment, frame.Event, frame.Data)
		return signal, nil
	case factorySessionSSEFrameData:
		return signal, fmt.Errorf("idle keepalive read factory event id %q", frame.FactoryEvent.Id)
	default:
		return signal, fmt.Errorf("unexpected idle keepalive frame kind %d", frame.kind)
	}
}

func firstNonEmptyFactorySessionSSEString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return factoryapi.FactoryEvent{}, fmt.Errorf("%w after %s", errFactorySessionSSEHarnessTimeout, timeout)
		}
		frame, err := s.TryReadNextFrame(remaining)
		if err != nil {
			return factoryapi.FactoryEvent{}, err
		}
		if frame.FactoryEvent != nil {
			return *frame.FactoryEvent, nil
		}
	}
}

// TryReadNextFrame reads one complete SSE frame within timeout. Protocol-only
// and comment-only frames are returned without being treated as FactoryEvents.
func (s *FactorySessionSSEStream) TryReadNextFrame(timeout time.Duration) (FactorySessionSSEFrame, error) {
	s.t.Helper()
	if timeout <= 0 {
		timeout = s.timeout
	}
	if s.pending == nil {
		done := make(chan factorySessionSSEReadResult, 1)
		s.pending = done
		go func() {
			frame, ok, err := tryReadNextSSEFrame(s.reader)
			done <- factorySessionSSEReadResult{frame: frame, ok: ok, err: err}
		}()
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-s.pending:
		s.pending = nil
		if result.err != nil {
			return result.frame, result.err
		}
		if !result.ok {
			return FactorySessionSSEFrame{}, io.EOF
		}
		s.lastFrame = result.frame
		s.hasFrame = true
		return result.frame, nil
	case <-timer.C:
		return FactorySessionSSEFrame{}, fmt.Errorf("%w after %s", errFactorySessionSSEHarnessTimeout, timeout)
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

func tryReadNextSSEFrame(reader *bufio.Reader) (FactorySessionSSEFrame, bool, error) {
	var frame FactorySessionSSEFrame
	var dataLines []string
	var commentLines []string
	var hasIDField bool
	var hasEventField bool
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if !hasIDField && !hasEventField && len(dataLines) == 0 && len(commentLines) == 0 {
				return FactorySessionSSEFrame{}, false, nil
			}
			return frame, false, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, ":") {
			commentLines = append(commentLines, strings.TrimPrefix(strings.TrimPrefix(line, ":"), " "))
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			field, value = line, ""
		} else {
			value = strings.TrimPrefix(value, " ")
		}
		switch field {
		case "id":
			hasIDField = true
			frame.ID = value
		case "event":
			hasEventField = true
			frame.Event = value
		case "data":
			dataLines = append(dataLines, value)
		}
	}
	frame.Data = strings.Join(dataLines, "\n")
	frame.Comment = strings.Join(commentLines, "\n")
	if len(dataLines) > 0 {
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(frame.Data), &event); err != nil {
			frame.kind = factorySessionSSEFrameOther
			return frame, true, &FactorySessionSSEParseError{Frame: frame, Err: err}
		}
		frame.kind = factorySessionSSEFrameData
		frame.FactoryEvent = &event
		return frame, true, nil
	}
	if len(commentLines) > 0 {
		frame.kind = factorySessionSSEFrameComment
		return frame, true, nil
	}
	if hasIDField || hasEventField {
		frame.kind = factorySessionSSEFrameOther
		return frame, true, nil
	}
	return FactorySessionSSEFrame{}, false, nil
}
