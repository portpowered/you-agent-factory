package cli

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

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
