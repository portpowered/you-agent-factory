package support

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// The observer is repository-wide functional-test support imported only by
// external test packages. Its entrypoints and private operations are
// function-valued so the production-only deadcode inventory does not mistake
// test-only reachability for an application API; the runtime behavior remains
// the same typed, session-scoped observer described below.

const (
	terminalFactoryEventObservationBufferSize     = 1
	terminalFactoryEventObservationScannerSize    = 4 * 1024 * 1024
	terminalFactoryEventObservationCleanupTimeout = 2 * time.Second
	terminalFactoryEventObservationHeaderTimeout  = 5 * time.Second
)

var errTerminalFactoryEventObservationClosed = errors.New(
	"terminal Factory Event observation closed before RUN_RESPONSE",
)

// TerminalFactoryEventObservation follows one session-scoped canonical Factory
// Event stream until the first post-replay RUN_RESPONSE event. The retained
// count published by the stream is the initial cursor boundary: events in that
// prefix are acknowledged into cursor and never treated as the new terminal
// result. This keeps the helper independent of status polling and quiet-time
// guesses while preserving the stream's replay/live ordering.
type TerminalFactoryEventObservation struct {
	t         testing.TB
	sessionID string
	endpoint  string
	stream    io.ReadCloser
	cancel    context.CancelFunc
	closeHTTP func()
	Wait      func(time.Duration) factoryapi.FactoryEvent
	Close     func()
	wait      func(time.Duration) (factoryapi.FactoryEvent, error)
	done      chan struct{}
	terminal  chan factoryapi.FactoryEvent
	errs      chan error

	closeOnce sync.Once
	doneOnce  sync.Once

	mu sync.Mutex

	closed            bool
	terminalEvent     *factoryapi.FactoryEvent
	terminalErr       error
	terminalSignaled  bool
	errorSignaled     bool
	terminalSignals   int
	retainedRemaining int
	cursor            FactoryEventReadCursor
}

// OpenSessionTerminalFactoryEventObservation opens the public canonical event
// stream for sessionID and starts draining it. The caller receives the first
// RUN_RESPONSE observed after the retained-history prefix.
var OpenSessionTerminalFactoryEventObservation = func(
	t testing.TB,
	baseURL string,
	sessionID string,
) *TerminalFactoryEventObservation {
	t.Helper()

	observation, err := openTerminalFactoryEventObservation(baseURL, sessionID)
	if err != nil {
		t.Fatalf("open terminal Factory Event observation: %v", err)
	}
	observation.t = t
	t.Cleanup(observation.Close)
	return observation
}

// OpenDefaultSessionTerminalFactoryEventObservation opens the canonical event
// stream for the default Factory Session.
var OpenDefaultSessionTerminalFactoryEventObservation = func(
	t testing.TB,
	baseURL string,
) *TerminalFactoryEventObservation {
	t.Helper()
	return OpenSessionTerminalFactoryEventObservation(t, baseURL, defaultFactorySessionID())
}

var openTerminalFactoryEventObservation = func(
	baseURL string,
	sessionID string,
	clients ...*http.Client,
) (*TerminalFactoryEventObservation, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is empty")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is empty")
	}

	client := (*http.Client)(nil)
	if len(clients) > 0 {
		client = clients[0]
	}
	if client == nil {
		transport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, fmt.Errorf("default HTTP transport does not support response-header timeout")
		}
		boundedTransport := transport.Clone()
		boundedTransport.ResponseHeaderTimeout = terminalFactoryEventObservationHeaderTimeout
		client = &http.Client{Transport: boundedTransport}
	}
	closeHTTP := client.CloseIdleConnections
	ctx, cancel := context.WithCancel(context.Background())
	cleanup := func() { cancel(); closeHTTP() }
	endpoint := SessionEventsURL(baseURL, sessionID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("build request for %s: %w", endpoint, err)
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := client.Do(request)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("GET %s: %w", endpoint, err)
	}
	closeResponse := func() { _ = response.Body.Close(); cleanup() }
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024))
		closeResponse()
		return nil, fmt.Errorf(
			"GET %s status = %d body = %q, want %d",
			endpoint,
			response.StatusCode,
			strings.TrimSpace(string(body)),
			http.StatusOK,
		)
	}
	contentType, _, contentTypeErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if contentTypeErr != nil || !strings.EqualFold(contentType, "text/event-stream") {
		closeResponse()
		return nil, fmt.Errorf(
			"GET %s content type = %q, want text/event-stream",
			endpoint,
			response.Header.Get("Content-Type"),
		)
	}
	retainedRaw := strings.TrimSpace(response.Header.Get(
		factorysessions.SessionEventStreamRetainedCountHeader,
	))
	if retainedRaw == "" {
		closeResponse()
		return nil, fmt.Errorf(
			"GET %s omitted %s",
			endpoint,
			factorysessions.SessionEventStreamRetainedCountHeader,
		)
	}
	retainedCount, err := strconv.Atoi(retainedRaw)
	if err != nil || retainedCount < 0 {
		closeResponse()
		return nil, fmt.Errorf(
			"GET %s retained event count = %q, want a non-negative integer",
			endpoint,
			retainedRaw,
		)
	}

	observation := &TerminalFactoryEventObservation{
		t:                 nil,
		sessionID:         sessionID,
		endpoint:          endpoint,
		stream:            response.Body,
		cancel:            cancel,
		closeHTTP:         closeHTTP,
		done:              make(chan struct{}),
		terminal:          make(chan factoryapi.FactoryEvent, terminalFactoryEventObservationBufferSize),
		errs:              make(chan error, terminalFactoryEventObservationBufferSize),
		retainedRemaining: retainedCount,
	}
	observation.wait = func(timeout time.Duration) (factoryapi.FactoryEvent, error) {
		return terminalFactoryEventObservationWait(observation, timeout)
	}
	observation.Wait = func(timeout time.Duration) factoryapi.FactoryEvent {
		if observation.t == nil {
			panic("TerminalFactoryEventObservation.Wait requires an observation opened by a test")
		}
		observation.t.Helper()
		event, err := observation.wait(timeout)
		if err != nil {
			observation.t.Fatalf("wait for terminal Factory Event: %v", err)
		}
		return event
	}
	observation.Close = func() { terminalFactoryEventObservationClose(observation) }
	go terminalFactoryEventObservationRead(observation, response.Body)
	return observation, nil
}

var defaultFactorySessionID = func() string {
	return factorysessions.DefaultSessionID
}

var terminalFactoryEventObservationRead = func(observation *TerminalFactoryEventObservation, stream io.ReadCloser) {
	defer observation.doneOnce.Do(func() { close(observation.done) })
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 64*1024), terminalFactoryEventObservationScannerSize)
	var dataLines []string
	dataBytes := 0
	flush := func() bool {
		if len(dataLines) == 0 {
			return true
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = nil
		dataBytes = 0
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			terminalFactoryEventObservationSignalError(observation, fmt.Errorf("decode Factory Event SSE payload: %w", err))
			return false
		}
		return terminalFactoryEventObservationAccept(observation, event)
	}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if !flush() {
				return
			}
		case strings.HasPrefix(line, ":"):
			// SSE comments are keep-alive records and carry no Factory Event.
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			dataBytes += len(data)
			if dataBytes > terminalFactoryEventObservationScannerSize {
				terminalFactoryEventObservationSignalError(observation, fmt.Errorf("Factory Event SSE data frame exceeds %d bytes", terminalFactoryEventObservationScannerSize))
				return
			}
			dataLines = append(dataLines, data)
		case strings.HasPrefix(line, "event:"):
			terminalFactoryEventObservationSignalError(observation, fmt.Errorf("Factory Event SSE emitted named event line %q", line))
			return
		case strings.HasPrefix(line, "id:") || strings.HasPrefix(line, "retry:"):
			// The canonical endpoint currently emits data-only records. These
			// standard SSE fields are harmless if an intermediary adds them.
		default:
			terminalFactoryEventObservationSignalError(observation, fmt.Errorf("Factory Event SSE emitted unsupported line %q", line))
			return
		}
	}
	if len(dataLines) > 0 && !flush() {
		return
	}
	if err := scanner.Err(); err != nil {
		if terminalFactoryEventObservationIsStopping(observation) {
			return
		}
		terminalFactoryEventObservationSignalError(observation, fmt.Errorf("read Factory Event SSE: %w", err))
		return
	}
	terminalFactoryEventObservationStreamClosed(observation)
}

var terminalFactoryEventObservationAccept = func(observation *TerminalFactoryEventObservation, event factoryapi.FactoryEvent) bool {
	observation.mu.Lock()
	if observation.closed || observation.terminalEvent != nil || observation.terminalErr != nil {
		observation.mu.Unlock()
		return false
	}
	if observation.retainedRemaining > 0 {
		if strings.TrimSpace(event.Id) == "" {
			observation.mu.Unlock()
			terminalFactoryEventObservationSignalError(observation, errors.New("retained Factory Event has an empty id"))
			return false
		}
		sequence := ReconnectSequenceForFactoryEvent(event)
		observation.cursor = FactoryEventReadCursor{
			AfterEventID: event.Id,
			AfterSequence: func() *int {
				value := sequence
				return &value
			}(),
		}
		observation.retainedRemaining--
		observation.mu.Unlock()
		return true
	}
	if !terminalFactoryEventObservationBelongsToSession(observation, event) {
		observation.mu.Unlock()
		return true
	}
	if event.Type != factoryapi.FactoryEventTypeRunResponse {
		observation.mu.Unlock()
		return true
	}

	eventCopy := event
	observation.terminalEvent = &eventCopy
	if !observation.terminalSignaled {
		observation.terminalSignaled = true
		observation.terminalSignals++
		select {
		case observation.terminal <- eventCopy:
		default:
		}
	}
	cancel := observation.cancel
	observation.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return false
}

var terminalFactoryEventObservationBelongsToSession = func(observation *TerminalFactoryEventObservation, event factoryapi.FactoryEvent) bool {
	if event.Context.SessionId == nil {
		return true
	}
	eventSessionID := strings.TrimSpace(*event.Context.SessionId)
	if observation.sessionID == factorysessions.DefaultSessionID {
		// The default compatibility route is already session-scoped by the
		// handler. Its events may carry the resolved runtime UUID rather than
		// the ~default selector, so the route itself is the authority here.
		return true
	}
	if eventSessionID == observation.sessionID {
		return true
	}
	// The live default-session route may publish its stable selector in event
	// context while the session read route returns the resolved runtime UUID.
	return eventSessionID == factorysessions.DefaultSessionID
}

var terminalFactoryEventObservationSignalError = func(observation *TerminalFactoryEventObservation, err error) {
	if err == nil {
		return
	}
	observation.mu.Lock()
	if observation.closed || observation.terminalEvent != nil || observation.terminalErr != nil {
		observation.mu.Unlock()
		return
	}
	observation.terminalErr = err
	if !observation.errorSignaled {
		observation.errorSignaled = true
		select {
		case observation.errs <- err:
		default:
		}
	}
	cancel := observation.cancel
	observation.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

var terminalFactoryEventObservationStreamClosed = func(observation *TerminalFactoryEventObservation) {
	observation.mu.Lock()
	if observation.closed || observation.terminalEvent != nil || observation.terminalErr != nil {
		observation.mu.Unlock()
		return
	}
	remaining := observation.retainedRemaining
	observation.mu.Unlock()
	if remaining > 0 {
		terminalFactoryEventObservationSignalError(observation, fmt.Errorf(
			"Factory Event SSE closed before retained cursor boundary (%d events remaining)",
			remaining,
		))
		return
	}
	terminalFactoryEventObservationSignalError(observation, errTerminalFactoryEventObservationClosed)
}

var terminalFactoryEventObservationIsStopping = func(observation *TerminalFactoryEventObservation) bool {
	observation.mu.Lock()
	defer observation.mu.Unlock()
	return observation.closed || observation.terminalEvent != nil || observation.terminalErr != nil
}

var terminalFactoryEventObservationSnapshot = func(observation *TerminalFactoryEventObservation) (factoryapi.FactoryEvent, error, bool) {
	observation.mu.Lock()
	defer observation.mu.Unlock()
	if observation.terminalEvent != nil {
		return *observation.terminalEvent, nil, true
	}
	if observation.terminalErr != nil {
		return factoryapi.FactoryEvent{}, observation.terminalErr, true
	}
	if observation.closed {
		return factoryapi.FactoryEvent{}, errTerminalFactoryEventObservationClosed, true
	}
	return factoryapi.FactoryEvent{}, nil, false
}

var terminalFactoryEventObservationWait = func(observation *TerminalFactoryEventObservation, timeout time.Duration) (factoryapi.FactoryEvent, error) {
	if observation == nil {
		return factoryapi.FactoryEvent{}, errors.New("terminal Factory Event observation is nil")
	}
	if timeout <= 0 {
		return factoryapi.FactoryEvent{}, fmt.Errorf("wait timeout must be positive")
	}
	if event, err, complete := terminalFactoryEventObservationSnapshot(observation); complete {
		return event, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case event := <-observation.terminal:
		return event, nil
	case err := <-observation.errs:
		return factoryapi.FactoryEvent{}, err
	case <-observation.done:
		event, err, _ := terminalFactoryEventObservationSnapshot(observation)
		if err == nil {
			return factoryapi.FactoryEvent{}, errTerminalFactoryEventObservationClosed
		}
		return event, err
	case <-timer.C:
		if event, err, complete := terminalFactoryEventObservationSnapshot(observation); complete {
			return event, err
		}
		return factoryapi.FactoryEvent{}, fmt.Errorf(
			"timed out waiting for post-cursor RUN_RESPONSE within %s",
			timeout,
		)
	}
}

// Wait returns the first post-cursor RUN_RESPONSE and fails the owning test if
// the stream reports malformed data, closes early, or misses the deadline.
var terminalFactoryEventObservationClose = func(observation *TerminalFactoryEventObservation) {
	if observation == nil {
		return
	}
	observation.closeOnce.Do(func() {
		observation.mu.Lock()
		observation.closed = true
		cancel := observation.cancel
		stream := observation.stream
		observation.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if stream != nil {
			_ = stream.Close()
		}
		if observation.closeHTTP != nil {
			observation.closeHTTP()
		}
	})
	select {
	case <-observation.done:
	case <-time.After(terminalFactoryEventObservationCleanupTimeout):
	}
}
