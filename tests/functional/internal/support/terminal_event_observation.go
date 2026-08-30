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
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	terminalFactoryEventObservationBufferSize     = 1
	terminalFactoryEventObservationScannerSize    = 4 * 1024 * 1024
	terminalFactoryEventObservationCleanupTimeout = 2 * time.Second
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
func OpenSessionTerminalFactoryEventObservation(
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
func OpenDefaultSessionTerminalFactoryEventObservation(
	t testing.TB,
	baseURL string,
) *TerminalFactoryEventObservation {
	t.Helper()
	return OpenSessionTerminalFactoryEventObservation(t, baseURL, defaultFactorySessionID())
}

func openTerminalFactoryEventObservation(
	baseURL string,
	sessionID string,
) (*TerminalFactoryEventObservation, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is empty")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is empty")
	}

	ctx, cancel := context.WithCancel(context.Background())
	endpoint := SessionEventsURL(baseURL, sessionID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("build request for %s: %w", endpoint, err)
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("GET %s: %w", endpoint, err)
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		cancel()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024))
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
		defer response.Body.Close()
		cancel()
		return nil, fmt.Errorf(
			"GET %s content type = %q, want text/event-stream",
			endpoint,
			response.Header.Get("Content-Type"),
		)
	}
	retainedRaw := strings.TrimSpace(response.Header.Get(
		factorysessionshttp.SessionEventStreamRetainedCountHeader,
	))
	if retainedRaw == "" {
		defer response.Body.Close()
		cancel()
		return nil, fmt.Errorf(
			"GET %s omitted %s",
			endpoint,
			factorysessionshttp.SessionEventStreamRetainedCountHeader,
		)
	}
	retainedCount, err := strconv.Atoi(retainedRaw)
	if err != nil || retainedCount < 0 {
		defer response.Body.Close()
		cancel()
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
		done:              make(chan struct{}),
		terminal:          make(chan factoryapi.FactoryEvent, terminalFactoryEventObservationBufferSize),
		errs:              make(chan error, terminalFactoryEventObservationBufferSize),
		retainedRemaining: retainedCount,
	}
	go observation.read(response.Body)
	return observation, nil
}

func defaultFactorySessionID() string {
	return factorysessions.DefaultSessionID
}

func (observation *TerminalFactoryEventObservation) read(stream io.ReadCloser) {
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
			observation.signalError(fmt.Errorf("decode Factory Event SSE payload: %w", err))
			return false
		}
		return observation.accept(event)
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
				observation.signalError(fmt.Errorf("Factory Event SSE data frame exceeds %d bytes", terminalFactoryEventObservationScannerSize))
				return
			}
			dataLines = append(dataLines, data)
		case strings.HasPrefix(line, "event:"):
			observation.signalError(fmt.Errorf("Factory Event SSE emitted named event line %q", line))
			return
		case strings.HasPrefix(line, "id:") || strings.HasPrefix(line, "retry:"):
			// The canonical endpoint currently emits data-only records. These
			// standard SSE fields are harmless if an intermediary adds them.
		default:
			observation.signalError(fmt.Errorf("Factory Event SSE emitted unsupported line %q", line))
			return
		}
	}
	if len(dataLines) > 0 && !flush() {
		return
	}
	if err := scanner.Err(); err != nil {
		if observation.isStopping() {
			return
		}
		observation.signalError(fmt.Errorf("read Factory Event SSE: %w", err))
		return
	}
	observation.streamClosed()
}

func (observation *TerminalFactoryEventObservation) accept(event factoryapi.FactoryEvent) bool {
	observation.mu.Lock()
	if observation.closed || observation.terminalEvent != nil || observation.terminalErr != nil {
		observation.mu.Unlock()
		return false
	}
	if observation.retainedRemaining > 0 {
		if strings.TrimSpace(event.Id) == "" {
			observation.mu.Unlock()
			observation.signalError(errors.New("retained Factory Event has an empty id"))
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
	if !observation.belongsToSession(event) {
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

func (observation *TerminalFactoryEventObservation) belongsToSession(event factoryapi.FactoryEvent) bool {
	if event.Context.SessionId == nil {
		return true
	}
	eventSessionID := strings.TrimSpace(*event.Context.SessionId)
	if eventSessionID == observation.sessionID {
		return true
	}
	// The live default-session route may publish its stable selector in event
	// context while the session read route returns the resolved runtime UUID.
	return observation.sessionID != factorysessions.DefaultSessionID &&
		eventSessionID == factorysessions.DefaultSessionID
}

func (observation *TerminalFactoryEventObservation) signalError(err error) {
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

func (observation *TerminalFactoryEventObservation) streamClosed() {
	observation.mu.Lock()
	if observation.closed || observation.terminalEvent != nil || observation.terminalErr != nil {
		observation.mu.Unlock()
		return
	}
	remaining := observation.retainedRemaining
	observation.mu.Unlock()
	if remaining > 0 {
		observation.signalError(fmt.Errorf(
			"Factory Event SSE closed before retained cursor boundary (%d events remaining)",
			remaining,
		))
		return
	}
	observation.signalError(errTerminalFactoryEventObservationClosed)
}

func (observation *TerminalFactoryEventObservation) isStopping() bool {
	observation.mu.Lock()
	defer observation.mu.Unlock()
	return observation.closed || observation.terminalEvent != nil || observation.terminalErr != nil
}

func (observation *TerminalFactoryEventObservation) snapshot() (factoryapi.FactoryEvent, error, bool) {
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

func (observation *TerminalFactoryEventObservation) wait(timeout time.Duration) (factoryapi.FactoryEvent, error) {
	if observation == nil {
		return factoryapi.FactoryEvent{}, errors.New("terminal Factory Event observation is nil")
	}
	if timeout <= 0 {
		return factoryapi.FactoryEvent{}, fmt.Errorf("wait timeout must be positive")
	}
	if event, err, complete := observation.snapshot(); complete {
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
		event, err, _ := observation.snapshot()
		if err == nil {
			return factoryapi.FactoryEvent{}, errTerminalFactoryEventObservationClosed
		}
		return event, err
	case <-timer.C:
		if event, err, complete := observation.snapshot(); complete {
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
func (observation *TerminalFactoryEventObservation) Wait(timeout time.Duration) factoryapi.FactoryEvent {
	if observation == nil || observation.t == nil {
		panic("TerminalFactoryEventObservation.Wait requires an observation opened by a test")
	}
	observation.t.Helper()
	event, err := observation.wait(timeout)
	if err != nil {
		observation.t.Fatalf("wait for terminal Factory Event: %v", err)
	}
	return event
}

// Close cancels the stream and waits for its reader to release the response
// body. It is safe for cleanup to call more than once.
func (observation *TerminalFactoryEventObservation) Close() {
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
	})
	select {
	case <-observation.done:
	case <-time.After(terminalFactoryEventObservationCleanupTimeout):
	}
}
