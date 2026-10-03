package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// throttleClock is a fake clock whose sleeper advances time instead of
// blocking, so capacity-window behavior is tested without real waiting.
type throttleClock struct {
	now    time.Time
	sleeps []time.Duration
}

func newThrottleClock() *throttleClock {
	return &throttleClock{now: time.Date(2026, time.October, 3, 12, 0, 0, 0, time.UTC)}
}

func (c *throttleClock) Now() time.Time { return c.now }

func (c *throttleClock) Sleep(_ context.Context, d time.Duration) error {
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
	return nil
}

func (c *throttleClock) slept() time.Duration {
	var total time.Duration
	for _, d := range c.sleeps {
		total += d
	}
	return total
}

func throttledProviderError() error {
	return workers.NewProviderError(workers.WorkFailureTypeThrottled, "Selected model is at capacity", nil)
}

func TestExecuteProviderWithRetryOutlastsLongCapacityEventThenSucceeds(t *testing.T) {
	t.Parallel()

	clock := newThrottleClock()
	service := &Service{clock: clock.Now, retrySleep: clock.Sleep}
	// More overloads than the old 3-attempt budget allowed, but recoverable
	// inside the 30 minute window (waits of ~30s, 1m, 2m, 4m, then 5m each).
	const overloads = 8
	attempts := 0
	result, err := service.executeProviderWithRetry(
		context.Background(),
		workers.RunnerExecutionRequest{},
		func(workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			attempts++
			if attempts <= overloads {
				return workers.RunnerExecutionResult{}, throttledProviderError()
			}
			return workers.RunnerExecutionResult{Content: "accepted"}, nil
		},
	)
	if err != nil || result.Content != "accepted" {
		t.Fatalf("executeProviderWithRetry() = (%#v, %v), want accepted after capacity recovers", result, err)
	}
	if attempts != overloads+1 {
		t.Fatalf("attempts = %d, want %d", attempts, overloads+1)
	}
	if got := clock.slept(); got > 30*time.Minute {
		t.Fatalf("slept %s, want within the 30 minute window", got)
	}
	if first := clock.sleeps[0]; first < 24*time.Second || first > 36*time.Second {
		t.Fatalf("first wait = %s, want about 30s with jitter", first)
	}
	if second := clock.sleeps[1]; second < 48*time.Second || second > 72*time.Second {
		t.Fatalf("second wait = %s, want about 60s with jitter", second)
	}
	for i, d := range clock.sleeps {
		if d > 5*time.Minute {
			t.Fatalf("wait %d = %s, want capped at 5 minutes", i, d)
		}
	}
}

func TestExecuteProviderWithRetryPersistentCapacityStopsAtWindow(t *testing.T) {
	t.Parallel()

	clock := newThrottleClock()
	service := &Service{clock: clock.Now, retrySleep: clock.Sleep}
	providerErr := throttledProviderError()
	attempts := 0
	_, err := service.executeProviderWithRetry(
		context.Background(),
		workers.RunnerExecutionRequest{},
		func(workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			attempts++
			return workers.RunnerExecutionResult{}, providerErr
		},
	)
	if !errors.Is(err, providerErr) {
		t.Fatalf("error = %v, want the throttled provider error after the window", err)
	}
	if got := clock.slept(); got != 30*time.Minute {
		t.Fatalf("slept %s, want exactly the 30 minute window", got)
	}
	if attempts <= 3 {
		t.Fatalf("attempts = %d, want more than the old 3-attempt limit", attempts)
	}
}

func TestExecuteProviderWithRetryNonThrottleFailuresKeepShortBudget(t *testing.T) {
	t.Parallel()

	clock := newThrottleClock()
	service := &Service{clock: clock.Now, retrySleep: clock.Sleep}
	providerErr := workers.NewProviderError(workers.WorkFailureTypeInternalServerError, "temporary", nil)
	attempts := 0
	_, err := service.executeProviderWithRetry(
		context.Background(),
		workers.RunnerExecutionRequest{},
		func(workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			attempts++
			return workers.RunnerExecutionResult{}, providerErr
		},
	)
	if !errors.Is(err, providerErr) || attempts != detachedProviderMaxRetries+1 {
		t.Fatalf("non-throttle = (%v, %d attempts), want original error after %d attempts", err, attempts, detachedProviderMaxRetries+1)
	}
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
	if len(clock.sleeps) != len(want) || clock.sleeps[0] != want[0] || clock.sleeps[1] != want[1] {
		t.Fatalf("waits = %v, want %v", clock.sleeps, want)
	}
}

func TestExecuteProviderWithRetryCapacityWaitHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	service := &Service{retrySleep: func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}}
	attempts := 0
	_, err := service.executeProviderWithRetry(
		ctx,
		workers.RunnerExecutionRequest{},
		func(workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
			attempts++
			return workers.RunnerExecutionResult{}, throttledProviderError()
		},
	)
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("canceled capacity wait = (%v, %d attempts), want context.Canceled after one attempt", err, attempts)
	}
}
