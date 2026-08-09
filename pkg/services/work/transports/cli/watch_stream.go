package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type watchEventStream interface {
	Next(context.Context) (factoryapi.FactoryEvent, error)
	Close() error
	RetainedEventCount() int
}

type watchEventOpener interface {
	Open(context.Context, *watchEventCursor) (watchEventStream, error)
}

type watchEventOpenFunc func(context.Context, *watchEventCursor) (watchEventStream, error)

func (open watchEventOpenFunc) Open(ctx context.Context, cursor *watchEventCursor) (watchEventStream, error) {
	if open == nil {
		return nil, fmt.Errorf("work watch event opener is required")
	}
	return open(ctx, cursor)
}

// NewWatch binds the CLI HTTP protocol to the Work watch operation. The
// protocol is injected by Wire so the stream still uses the process-owned
// external-effect boundary.
func NewWatch(transport clihttp.Protocol) func(WatchConfig) error {
	return func(cfg WatchConfig) error {
		cfg.HTTP = transport
		return Watch(cfg)
	}
}

// Watch consumes the selected session's canonical Factory Event SSE stream.
// It does not query Work snapshots or schedule a polling interval.
func Watch(cfg WatchConfig) error {
	if err := ValidateWatchConfig(cfg); err != nil {
		return err
	}
	if cfg.HTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	sessionID := watchSessionID(cfg)
	return watchWithSource(cfg, watchEventOpenFunc(func(ctx context.Context, cursor *watchEventCursor) (watchEventStream, error) {
		return openHTTPWatchEventStream(ctx, cfg.HTTP, cfg.Server, sessionID, cursor, cfg.Diagnostics, cfg.Verbose)
	}))
}

func watchSessionID(cfg WatchConfig) string {
	if strings.TrimSpace(cfg.SessionID) == "" {
		return sessionpath.DefaultFactorySessionID
	}
	return cfg.SessionID
}

func watchWithSource(cfg WatchConfig, opener watchEventOpener) error {
	return watchWithRetry(cfg, opener, defaultWatchRetryPolicy())
}

func watchWithRetry(cfg WatchConfig, opener watchEventOpener, retry watchRetryPolicy) error {
	if err := ValidateWatchConfig(cfg); err != nil {
		return err
	}
	if opener == nil {
		return fmt.Errorf("work watch event opener is required")
	}
	sessionID := watchSessionID(cfg)
	reducer := newWatchReducer(sessionID)
	reconnectAttempts := 0
	for {
		cursor := reducer.Cursor()
		stream, err := opener.Open(cfg.Context, cursor)
		if err != nil {
			if contextErr := cfg.Context.Err(); contextErr != nil {
				return contextErr
			}
			if !isRetryableWatchError(err) {
				return fmt.Errorf("open work watch stream for session %q: %w", sessionID, err)
			}
			if err := retryWatchReconnect(cfg, retry, sessionID, cursor, reconnectAttempts, err); err != nil {
				return err
			}
			reconnectAttempts++
			continue
		}
		if stream == nil {
			return fmt.Errorf("open work watch stream for session %q: stream is unavailable", sessionID)
		}

		closeStream, stopCloseOnCancel := watchStreamCloseOnCancel(cfg.Context, stream)
		result := consumeWatchStream(cfg, reducer, stream)
		_ = closeStream()
		stopCloseOnCancel()

		if result.completed {
			return nil
		}
		if contextErr := cfg.Context.Err(); contextErr != nil {
			return contextErr
		}
		if result.err == nil {
			return fmt.Errorf("work watch stream for session %q ended without an outcome", sessionID)
		}
		if !result.retryable {
			return fmt.Errorf("work watch stream for session %q: %w", sessionID, result.err)
		}
		cursor = reducer.Cursor()
		if err := retryWatchReconnect(cfg, retry, sessionID, cursor, reconnectAttempts, result.err); err != nil {
			return err
		}
		reconnectAttempts++
	}
}

type watchStreamResult struct {
	err       error
	completed bool
	retryable bool
}

func consumeWatchStream(cfg WatchConfig, reducer *watchReducer, stream watchEventStream) watchStreamResult {
	retainedRemaining := stream.RetainedEventCount()
	if retainedRemaining < 0 {
		return watchStreamResult{
			err: &watchProtocolError{message: fmt.Sprintf("work watch stream returned negative retained event count %d", retainedRemaining)},
		}
	}
	for {
		event, err := stream.Next(cfg.Context)
		if err != nil {
			return watchStreamResult{
				err:       formatWatchReadError(err),
				retryable: isRetryableWatchError(err),
			}
		}
		if retainedRemaining > 0 {
			retainedRemaining--
		}

		transition, emit, completed, err := reducer.Accept(event)
		if err != nil {
			return watchStreamResult{err: fmt.Errorf("reduce Work watch event %q: %w", event.Id, err)}
		}
		if emit {
			if err := RenderWatchTransition(cfg.Output, transition); err != nil {
				return watchStreamResult{err: err}
			}
		}
		if completed && retainedRemaining == 0 && !cfg.Follow {
			return watchStreamResult{completed: true}
		}
	}
}

func formatWatchReadError(err error) error {
	if errors.Is(err, io.EOF) {
		return io.EOF
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("read Work watch stream: %w", err)
}

type httpWatchEventStream struct {
	reader             *bufio.Reader
	body               io.ReadCloser
	retainedEventCount int
	closeOnce          sync.Once
}

func openHTTPWatchEventStream(
	ctx context.Context,
	transport clihttp.Protocol,
	server string,
	sessionID string,
	cursor *watchEventCursor,
	diagnostics io.Writer,
	verbose bool,
) (watchEventStream, error) {
	if transport == nil {
		return nil, fmt.Errorf("CLI HTTP protocol is required")
	}
	endpointURL, err := cliserver.RequestURL(server, sessionpath.FactoryEventsPath(sessionID))
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return nil, fmt.Errorf("parse work watch endpoint: %w", err)
	}
	if cursor != nil {
		query := endpoint.Query()
		query.Set("after_event_id", cursor.EventID)
		query.Set("after_sequence", strconv.Itoa(cursor.Sequence))
		endpoint.RawQuery = query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build work watch request: %w", err)
	}
	request.Header.Set("Accept", "text/event-stream")
	clidiag.Printf(
		diagnostics,
		verbose,
		"work watch stream open endpointPath=%s endpoint=%s session=%s",
		endpoint.Path,
		endpoint.String(),
		sessionID,
	)
	response, err := transport.Execute(request)
	if err != nil {
		return nil, fmt.Errorf("factory not reachable at %s: %w", endpoint.String(), err)
	}
	if response.HTTP == nil {
		return nil, &watchProtocolError{message: "work watch stream returned no HTTP response"}
	}
	resp := response.HTTP
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		message := ""
		if errResp, ok := clihttp.DecodeAPIError(resp); ok {
			message = errResp.Message
		}
		return nil, &watchHTTPStatusError{sessionID: sessionID, status: resp.StatusCode, message: message}
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		defer resp.Body.Close()
		return nil, &watchProtocolError{message: fmt.Sprintf("work watch stream for session %q returned content type %q", sessionID, resp.Header.Get("Content-Type"))}
	}
	retainedEventCount, err := parseWatchRetainedEventCount(resp.Header.Get(factorysessions.SessionEventStreamRetainedCountHeader), sessionID)
	if err != nil {
		defer resp.Body.Close()
		return nil, err
	}
	return &httpWatchEventStream{
		reader:             bufio.NewReader(resp.Body),
		body:               resp.Body,
		retainedEventCount: retainedEventCount,
	}, nil
}

func parseWatchRetainedEventCount(headerValue, sessionID string) (int, error) {
	value := strings.TrimSpace(headerValue)
	if value == "" {
		return 0, &watchProtocolError{message: fmt.Sprintf(
			"work watch stream for session %q is missing %s",
			sessionID,
			factorysessions.SessionEventStreamRetainedCountHeader,
		)}
	}
	count, err := strconv.Atoi(value)
	if err != nil || count < 0 {
		return 0, &watchProtocolError{message: fmt.Sprintf(
			"work watch stream for session %q returned invalid %s %q",
			sessionID,
			factorysessions.SessionEventStreamRetainedCountHeader,
			value,
		)}
	}
	return count, nil
}

func (stream *httpWatchEventStream) RetainedEventCount() int {
	if stream == nil {
		return 0
	}
	return stream.retainedEventCount
}

func (stream *httpWatchEventStream) Next(ctx context.Context) (factoryapi.FactoryEvent, error) {
	if stream == nil || stream.reader == nil {
		return factoryapi.FactoryEvent{}, fmt.Errorf("work watch HTTP stream is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.FactoryEvent{}, err
	}
	event, err := readWatchSSEEvent(stream.reader)
	if err != nil && ctx.Err() != nil {
		return factoryapi.FactoryEvent{}, ctx.Err()
	}
	return event, err
}

func (stream *httpWatchEventStream) Close() error {
	if stream == nil || stream.body == nil {
		return nil
	}
	var err error
	stream.closeOnce.Do(func() { err = stream.body.Close() })
	return err
}

func readWatchSSEEvent(reader *bufio.Reader) (factoryapi.FactoryEvent, error) {
	var data []string
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if err != nil && len(line) == 0 {
			if len(data) == 0 {
				return factoryapi.FactoryEvent{}, err
			}
		}
		if line == "" {
			if len(data) == 0 {
				if err != nil {
					return factoryapi.FactoryEvent{}, err
				}
				continue
			}
			return decodeWatchSSEEvent(data)
		}
		if strings.HasPrefix(line, ":") {
			if err != nil {
				return factoryapi.FactoryEvent{}, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			data = append(data, value)
		}
		if err != nil {
			return decodeWatchSSEEvent(data)
		}
	}
}

func decodeWatchSSEEvent(data []string) (factoryapi.FactoryEvent, error) {
	var event factoryapi.FactoryEvent
	if err := json.Unmarshal([]byte(strings.Join(data, "\n")), &event); err != nil {
		return factoryapi.FactoryEvent{}, &watchMalformedEventError{err: err}
	}
	return event, nil
}

const (
	watchMaxReconnectAttempts = 5
	watchInitialBackoff       = 100 * time.Millisecond
	watchMaximumBackoff       = 2 * time.Second
)

type watchEventCursor struct {
	EventID  string
	Sequence int
}

type watchRetryPolicy struct {
	maxAttempts  int
	initialDelay time.Duration
	maximumDelay time.Duration
	wait         func(context.Context, time.Duration) error
}

func defaultWatchRetryPolicy() watchRetryPolicy {
	return watchRetryPolicy{
		maxAttempts:  watchMaxReconnectAttempts,
		initialDelay: watchInitialBackoff,
		maximumDelay: watchMaximumBackoff,
		wait:         waitWatchReconnect,
	}
}

func (policy watchRetryPolicy) normalized() watchRetryPolicy {
	if policy.maxAttempts < 0 {
		policy.maxAttempts = 0
	}
	if policy.initialDelay < 0 {
		policy.initialDelay = 0
	}
	if policy.maximumDelay < 0 {
		policy.maximumDelay = 0
	}
	if policy.maximumDelay > 0 && policy.maximumDelay < policy.initialDelay {
		policy.maximumDelay = policy.initialDelay
	}
	if policy.wait == nil {
		policy.wait = waitWatchReconnect
	}
	return policy
}

func (policy watchRetryPolicy) delay(attempt int) time.Duration {
	policy = policy.normalized()
	if attempt <= 1 || policy.initialDelay == 0 {
		return policy.initialDelay
	}
	delay := policy.initialDelay
	for step := 1; step < attempt; step++ {
		if policy.maximumDelay > 0 && delay >= policy.maximumDelay {
			return policy.maximumDelay
		}
		delay *= 2
		if policy.maximumDelay > 0 && delay >= policy.maximumDelay {
			return policy.maximumDelay
		}
	}
	return delay
}

func waitWatchReconnect(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryWatchReconnect(
	cfg WatchConfig,
	policy watchRetryPolicy,
	sessionID string,
	cursor *watchEventCursor,
	attempts int,
	cause error,
) error {
	policy = policy.normalized()
	if attempts >= policy.maxAttempts {
		return fmt.Errorf(
			"work watch stream for session %q reconnect attempts exhausted after %d attempt(s) at %s: %w",
			sessionID, attempts, formatWatchCursor(cursor), cause,
		)
	}
	attempt := attempts + 1
	delay := policy.delay(attempt)
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"work watch reconnect session=%s attempt=%d/%d cursor=%s backoff=%s",
		sessionID,
		attempt,
		policy.maxAttempts,
		formatWatchCursor(cursor),
		delay,
	)
	return policy.wait(cfg.Context, delay)
}

func formatWatchCursor(cursor *watchEventCursor) string {
	if cursor == nil {
		return "start"
	}
	return fmt.Sprintf("eventId=%q sequence=%d", cursor.EventID, cursor.Sequence)
}

func watchStreamCloseOnCancel(ctx context.Context, stream watchEventStream) (func() error, func()) {
	var once sync.Once
	var closeErr error
	closeStream := func() error {
		once.Do(func() { closeErr = stream.Close() })
		return closeErr
	}
	stop := context.AfterFunc(ctx, func() { _ = closeStream() })
	return closeStream, func() { _ = stop() }
}

type watchHTTPStatusError struct {
	sessionID string
	status    int
	message   string
}

func (err *watchHTTPStatusError) Error() string {
	if err == nil {
		return ""
	}
	if err.message != "" {
		return fmt.Sprintf("watch work failed for session %q (%d): %s", err.sessionID, err.status, err.message)
	}
	return fmt.Sprintf("watch work failed for session %q (%d)", err.sessionID, err.status)
}

func (err *watchHTTPStatusError) retryable() bool {
	if err == nil {
		return false
	}
	return err.status == 408 || err.status == 429 || err.status >= 500
}

type watchProtocolError struct {
	message string
}

func (err *watchProtocolError) Error() string {
	if err == nil {
		return ""
	}
	return err.message
}

type watchMalformedEventError struct {
	err error
}

func (err *watchMalformedEventError) Error() string {
	if err == nil || err.err == nil {
		return "malformed canonical Factory Event SSE data"
	}
	return fmt.Sprintf("decode canonical Factory Event SSE data: %v", err.err)
}

func (err *watchMalformedEventError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.err
}

func isRetryableWatchError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var statusErr *watchHTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.retryable()
	}
	var protocolErr *watchProtocolError
	if errors.As(err, &protocolErr) {
		return false
	}
	var malformedErr *watchMalformedEventError
	if errors.As(err, &malformedErr) {
		return false
	}
	return true
}
