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

	"github.com/portpowered/infinite-you/internal/testutil/boundedio"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	recordingshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

const (
	defaultFactorySessionSSEHarnessTimeout    = 5 * time.Second
	factorySessionSSEBackendScopeHeader       = "X-Factory-Session-Backend-Scope-Id"
	factorySessionSSELogicalSessionKeyHeader  = "X-Factory-Session-Logical-Session-Key-Id"
	factorySessionSSEFactorySessionHeader     = "X-Factory-Session-Factory-Session-Id"
	factorySessionSSEStreamGenerationHeader   = "X-Factory-Session-Stream-Generation-Id"
	factorySessionSSERetainedEventCountHeader = "X-Factory-Session-Retained-Event-Count"
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
	t                     *testing.T
	timeout               time.Duration
	doer                  HTTPDoer
	ctx                   context.Context
	ProductionWiredServer func(recordingshttp.LegacyLiveEvents) *api.Server
}

// HTTPDoer is the explicit HTTP edge used by the SSE test harness.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
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

// FactorySessionSSEOpenError preserves a typed HTTP failure returned while
// opening a session event stream.
type FactorySessionSSEOpenError struct {
	SessionID  string
	Checkpoint FactorySessionSSECheckpoint
	StatusCode int
	Response   factoryapi.ErrorResponse
}

func (e *FactorySessionSSEOpenError) Error() string {
	return fmt.Sprintf(
		"open Factory Session SSE stream for session %q checkpoint %s: status %d: %s",
		e.SessionID,
		factorySessionSSECheckpointDescription(e.Checkpoint),
		e.StatusCode,
		e.Response.Message,
	)
}

// FactorySessionSSEReadOutcome classifies bounded waiting separately from
// caller cancellation.
type FactorySessionSSEReadOutcome string

const (
	FactorySessionSSEReadOutcomeWaitingTimeout FactorySessionSSEReadOutcome = "WAITING_TIMEOUT"
	FactorySessionSSEReadOutcomeCallerCanceled FactorySessionSSEReadOutcome = "CALLER_CANCELED"
)

// FactorySessionSSEReadError records the selector, checkpoint, elapsed bound,
// and last trustworthy frame for an interrupted bounded read.
type FactorySessionSSEReadError struct {
	Outcome        FactorySessionSSEReadOutcome
	SessionID      string
	Checkpoint     FactorySessionSSECheckpoint
	ElapsedBound   time.Duration
	LastValidFrame *FactorySessionSSEFrame
	Err            error
}

func (e *FactorySessionSSEReadError) Error() string {
	lastFrame := "no valid frame observed"
	if e.LastValidFrame != nil {
		lastFrame = fmt.Sprintf("last valid frame id=%q event=%q data=%q comment=%q",
			e.LastValidFrame.ID,
			e.LastValidFrame.Event,
			e.LastValidFrame.Data,
			e.LastValidFrame.Comment,
		)
	}
	return fmt.Sprintf(
		"session SSE read %s for session %q checkpoint %s after %s: %s: %v",
		e.Outcome,
		e.SessionID,
		factorySessionSSECheckpointDescription(e.Checkpoint),
		e.ElapsedBound,
		lastFrame,
		e.Err,
	)
}

func (e *FactorySessionSSEReadError) Unwrap() error { return e.Err }

// ApplyRecovery returns the checkpoint to use for the next stream open after a
// typed JSON recovery probe. The server controls each omission independently.
func (c FactorySessionSSECheckpoint) ApplyRecovery(
	recovery factoryapi.FactorySessionEventStreamRecovery,
) FactorySessionSSECheckpoint {
	if recovery.Retry.OmitAfterEventId {
		c.AfterEventID = ""
	}
	if recovery.Retry.OmitAfterSequence {
		c.AfterSequence = nil
	}
	return c
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
	t          *testing.T
	timeout    time.Duration
	Response   *http.Response
	Identity   FactorySessionSSEStreamIdentity
	reader     *bufio.Reader
	ctx        context.Context
	cancel     context.CancelFunc
	pending    *boundedio.Pending[factorySessionSSEReadResult]
	lastFrame  FactorySessionSSEFrame
	hasFrame   bool
	sessionID  string
	checkpoint FactorySessionSSECheckpoint
}

// NewFactorySessionSSEHarness returns a harness that fails closed when a read
// exceeds the supplied timeout. Zero timeout uses the package default.
func NewFactorySessionSSEHarness(t *testing.T, timeout time.Duration, doer HTTPDoer, ctx context.Context) *FactorySessionSSEHarness {
	t.Helper()
	if timeout <= 0 {
		timeout = defaultFactorySessionSSEHarnessTimeout
	}
	if doer == nil {
		t.Fatal("Factory Session SSE HTTP doer is required")
	}
	if ctx == nil {
		t.Fatal("Factory Session SSE caller context is required")
	}
	logger := zap.NewNop()
	return &FactorySessionSSEHarness{
		t:       t,
		timeout: timeout,
		doer:    doer,
		ctx:     ctx,
		ProductionWiredServer: func(liveEvents recordingshttp.LegacyLiveEvents) *api.Server {
			return api.NewServerWithRecordings(
				recordingshttp.NewLegacyAdapterWithLive(nil, nil, liveEvents),
				factorysessionshttp.NewHandler(factorysessionshttp.Dependencies{}, logger),
				nil, nil, nil, nil, logger,
			)
		},
	}
}

// Open starts GET /factory-sessions/{sessionID}/events with an optional raw query
// string (without leading "?").
func (h *FactorySessionSSEHarness) Open(serverURL, sessionID, query string) *FactorySessionSSEStream {
	h.t.Helper()
	stream, err := h.tryOpen(h.ctx, serverURL, sessionID, query, FactorySessionSSECheckpoint{})
	if err != nil {
		h.t.Fatalf("open session SSE stream: %v", err)
	}
	return stream
}

func (h *FactorySessionSSEHarness) tryOpen(
	callerContext context.Context,
	serverURL, sessionID, query string,
	checkpoint FactorySessionSSECheckpoint,
) (*FactorySessionSSEStream, error) {
	h.t.Helper()

	path := "/factory-sessions/" + sessionID + "/events"
	if query != "" {
		path += "?" + query
	}
	ctx, cancel := boundedio.CancelScope(callerContext)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+path, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("new session SSE request for session %q: %w", sessionID, err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := h.doer.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		var errorResponse factoryapi.ErrorResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&errorResponse)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("GET %s status = %d: decode error response: %w", path, resp.StatusCode, decodeErr)
		}
		return nil, &FactorySessionSSEOpenError{
			SessionID:  sessionID,
			Checkpoint: checkpoint,
			StatusCode: resp.StatusCode,
			Response:   errorResponse,
		}
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		cancel()
		_ = resp.Body.Close()
		return nil, fmt.Errorf("GET %s Content-Type = %q, want text/event-stream", path, got)
	}
	return &FactorySessionSSEStream{
		t:          h.t,
		timeout:    h.timeout,
		Response:   resp,
		Identity:   factorySessionSSEIdentityFromHeader(resp.Header),
		reader:     bufio.NewReader(resp.Body),
		ctx:        ctx,
		cancel:     cancel,
		sessionID:  sessionID,
		checkpoint: checkpoint,
	}, nil
}

// OpenFromCheckpoint resumes a session event stream after the supplied
// acknowledged FactoryEvent checkpoint.
func (h *FactorySessionSSEHarness) OpenFromCheckpoint(
	serverURL, sessionID string,
	checkpoint FactorySessionSSECheckpoint,
) *FactorySessionSSEStream {
	h.t.Helper()

	stream, err := h.tryOpen(
		h.ctx,
		serverURL,
		sessionID,
		factorySessionSSECheckpointQuery(checkpoint),
		checkpoint,
	)
	if err != nil {
		h.t.Fatalf("open session SSE stream from checkpoint: %v", err)
	}
	return stream
}

// TryOpenFromCheckpoint opens a session stream with caller-owned cancellation
// and returns typed HTTP failures instead of failing the test.
func (h *FactorySessionSSEHarness) TryOpenFromCheckpoint(
	ctx context.Context,
	serverURL, sessionID string,
	checkpoint FactorySessionSSECheckpoint,
) (*FactorySessionSSEStream, error) {
	h.t.Helper()

	return h.tryOpen(ctx, serverURL, sessionID, factorySessionSSECheckpointQuery(checkpoint), checkpoint)
}

func factorySessionSSECheckpointQuery(checkpoint FactorySessionSSECheckpoint) string {
	query := url.Values{}
	if checkpoint.AfterEventID != "" {
		query.Set("after_event_id", checkpoint.AfterEventID)
	}
	if checkpoint.AfterSequence != nil {
		query.Set("after_sequence", fmt.Sprint(*checkpoint.AfterSequence))
	}
	return query.Encode()
}

func factorySessionSSECheckpointDescription(checkpoint FactorySessionSSECheckpoint) string {
	afterSequence := "<omitted>"
	if checkpoint.AfterSequence != nil {
		afterSequence = fmt.Sprint(*checkpoint.AfterSequence)
	}
	afterEventID := checkpoint.AfterEventID
	if afterEventID == "" {
		afterEventID = "<omitted>"
	}
	return fmt.Sprintf("after_event_id=%q after_sequence=%s", afterEventID, afterSequence)
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

	if err := s.setReadDeadline(boundedio.Deadline(timeout)); err == nil {
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

	boundedio.Wait(timeout)

	if err := s.setReadDeadline(boundedio.Deadline(25 * time.Millisecond)); err == nil {
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
	deadline := boundedio.Deadline(timeout)
	for {
		remaining := boundedio.Remaining(deadline)
		if remaining <= 0 {
			return factoryapi.FactoryEvent{}, s.readError(
				FactorySessionSSEReadOutcomeWaitingTimeout,
				timeout,
				errFactorySessionSSEHarnessTimeout,
			)
		}
		frame, err := s.TryReadNextFrame(remaining)
		if err != nil {
			var readErr *FactorySessionSSEReadError
			if errors.As(err, &readErr) {
				readErr.ElapsedBound = timeout
			}
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
	if readErr := s.callerCancellationError(timeout); readErr != nil {
		return FactorySessionSSEFrame{}, readErr
	}
	if s.pending == nil {
		s.pending = boundedio.Start(func() factorySessionSSEReadResult {
			frame, ok, err := tryReadNextSSEFrame(s.reader)
			return factorySessionSSEReadResult{frame: frame, ok: ok, err: err}
		})
	}

	result, waitErr := s.pending.Await(s.ctx, timeout)
	switch {
	case waitErr == nil:
		s.pending = nil
		if readErr := s.callerCancellationError(timeout); readErr != nil {
			return FactorySessionSSEFrame{}, readErr
		}
		if result.err != nil {
			if errors.Is(result.err, context.Canceled) {
				return FactorySessionSSEFrame{}, s.readError(
					FactorySessionSSEReadOutcomeCallerCanceled,
					timeout,
					context.Canceled,
				)
			}
			return result.frame, result.err
		}
		if !result.ok {
			return FactorySessionSSEFrame{}, io.EOF
		}
		s.lastFrame = result.frame
		s.hasFrame = true
		return result.frame, nil
	case errors.Is(waitErr, boundedio.ErrTimeout):
		return FactorySessionSSEFrame{}, s.readError(
			FactorySessionSSEReadOutcomeWaitingTimeout,
			timeout,
			errFactorySessionSSEHarnessTimeout,
		)
	default:
		return FactorySessionSSEFrame{}, s.readError(
			FactorySessionSSEReadOutcomeCallerCanceled,
			timeout,
			waitErr,
		)
	}
}

func (s *FactorySessionSSEStream) callerCancellationError(timeout time.Duration) *FactorySessionSSEReadError {
	if s.ctx == nil || s.ctx.Err() == nil {
		return nil
	}
	return s.readError(FactorySessionSSEReadOutcomeCallerCanceled, timeout, s.ctx.Err())
}

func (s *FactorySessionSSEStream) readError(
	outcome FactorySessionSSEReadOutcome,
	elapsedBound time.Duration,
	err error,
) *FactorySessionSSEReadError {
	readErr := &FactorySessionSSEReadError{
		Outcome:      outcome,
		SessionID:    s.sessionID,
		Checkpoint:   s.checkpoint,
		ElapsedBound: elapsedBound,
		Err:          err,
	}
	if s.hasFrame {
		lastFrame := s.lastFrame
		readErr.LastValidFrame = &lastFrame
	}
	return readErr
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
	ctx, cancel := boundedio.TimeoutScope(h.ctx, h.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+path, nil)
	if err != nil {
		h.t.Fatalf("new session events request: %v", err)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := h.doer.Do(req)
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

// ProbeRecoveryFromCheckpoint requests the generated JSON recovery contract
// for the same checkpoint representation used by OpenFromCheckpoint.
func (h *FactorySessionSSEHarness) ProbeRecoveryFromCheckpoint(
	serverURL, sessionID string,
	checkpoint FactorySessionSSECheckpoint,
) (factoryapi.FactorySessionEventStreamRecovery, *http.Response) {
	h.t.Helper()

	return h.ProbeRecovery(serverURL, sessionID, factorySessionSSECheckpointQuery(checkpoint))
}

type factorySessionSSEFrameKind int

const (
	factorySessionSSEFrameComment factorySessionSSEFrameKind = iota
	factorySessionSSEFrameData
	factorySessionSSEFrameOther
)

func tryReadNextSSEFrame(reader *bufio.Reader) (FactorySessionSSEFrame, bool, error) {
	rawFrame, ok, err := readFactorySessionSSERawFrame(reader)
	if err != nil || !ok {
		return rawFrame.frame, ok, err
	}
	return rawFrame.decode()
}

type factorySessionSSERawFrame struct {
	frame         FactorySessionSSEFrame
	dataLines     []string
	commentLines  []string
	hasIDField    bool
	hasEventField bool
}

func readFactorySessionSSERawFrame(reader *bufio.Reader) (factorySessionSSERawFrame, bool, error) {
	var rawFrame factorySessionSSERawFrame
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if !rawFrame.hasFields() {
				return factorySessionSSERawFrame{}, false, nil
			}
			return rawFrame, false, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return rawFrame, true, nil
		}
		rawFrame.addLine(line)
	}
}

func (f *factorySessionSSERawFrame) addLine(line string) {
	if strings.HasPrefix(line, ":") {
		f.commentLines = append(f.commentLines, strings.TrimPrefix(strings.TrimPrefix(line, ":"), " "))
		return
	}
	field, value, found := strings.Cut(line, ":")
	if !found {
		field, value = line, ""
	} else {
		value = strings.TrimPrefix(value, " ")
	}
	switch field {
	case "id":
		f.hasIDField = true
		f.frame.ID = value
	case "event":
		f.hasEventField = true
		f.frame.Event = value
	case "data":
		f.dataLines = append(f.dataLines, value)
	}
}

func (f factorySessionSSERawFrame) hasFields() bool {
	return f.hasIDField || f.hasEventField || len(f.dataLines) > 0 || len(f.commentLines) > 0
}

func (f factorySessionSSERawFrame) decode() (FactorySessionSSEFrame, bool, error) {
	f.frame.Data = strings.Join(f.dataLines, "\n")
	f.frame.Comment = strings.Join(f.commentLines, "\n")
	if len(f.dataLines) > 0 {
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(f.frame.Data), &event); err != nil {
			f.frame.kind = factorySessionSSEFrameOther
			return f.frame, true, &FactorySessionSSEParseError{Frame: f.frame, Err: err}
		}
		f.frame.kind = factorySessionSSEFrameData
		f.frame.FactoryEvent = &event
		return f.frame, true, nil
	}
	if len(f.commentLines) > 0 {
		f.frame.kind = factorySessionSSEFrameComment
		return f.frame, true, nil
	}
	if f.hasIDField || f.hasEventField {
		f.frame.kind = factorySessionSSEFrameOther
		return f.frame, true, nil
	}
	return FactorySessionSSEFrame{}, false, nil
}
