package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestJitteredRetryBackoff_ClampsAndDesynchronizesControlledValues(t *testing.T) {
	offsets := []time.Duration{-5 * time.Millisecond, 5 * time.Millisecond}
	call := 0
	jitter := func(base time.Duration) time.Duration {
		value := base + offsets[call]
		call++
		return value
	}

	first := jitteredRetryBackoff(1, jitter)
	second := jitteredRetryBackoff(1, jitter)
	if first != restartBackoffMin-5*time.Millisecond {
		t.Fatalf("first jittered delay = %s, want %s", first, restartBackoffMin-5*time.Millisecond)
	}
	if second != restartBackoffMin+5*time.Millisecond {
		t.Fatalf("second jittered delay = %s, want %s", second, restartBackoffMin+5*time.Millisecond)
	}
	if first == second {
		t.Fatalf("equivalent failures received lockstep delay %s", first)
	}

	if got := jitteredRetryBackoff(1, func(time.Duration) time.Duration { return -time.Hour }); got != retryDelayFloor {
		t.Fatalf("negative jittered delay = %s, want floor %s", got, retryDelayFloor)
	}
	if got := jitteredRetryBackoff(1, func(time.Duration) time.Duration { return time.Hour }); got != restartBackoffMax {
		t.Fatalf("unbounded jittered delay = %s, want ceiling %s", got, restartBackoffMax)
	}
}

func TestDefaultRetryJitter_StaysWithinBoundedRange(t *testing.T) {
	for _, failures := range []int{1, 5, 13, 20} {
		base := restartBackoff(failures)
		minimum := clampRetryDelay(base - base*retryJitterRatio/100)
		maximum := clampRetryDelay(base + base*retryJitterRatio/100)
		for sample := 0; sample < 100; sample++ {
			got := defaultRetryJitter(base)
			if got < minimum || got > maximum {
				t.Fatalf("defaultRetryJitter(%s) = %s, want within [%s, %s]", base, got, minimum, maximum)
			}
		}
	}
}

func TestStartLinearPoller_ResetsFailureBackoffAfterSuccessfulCycle(t *testing.T) {
	fakeClock := clockwork.NewFakeClock()
	var requestCount atomic.Int32
	requestEvents := make(chan int, 4)
	logCore, observedLogs := observer.New(zap.InfoLevel)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call := requestCount.Add(1)
		requestEvents <- int(call)
		if call == 1 || call == 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":[{"message":"temporary provider failure"}]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mockLinearIssuesGraphQLResponse()))
	}))
	defer server.Close()

	factoryDir := t.TempDir()
	writeHostedLinearSecretForTest(t, factoryDir)
	pollerCfg, runtimeCfg, poller, worker := hostedLinearPollerFixtureForTest(t, factoryDir, server, nil)
	pollerCfg.Logger = zap.New(logCore)
	pollerCfg.Clock = fakeClock

	ctx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	if err := startLinearPollerWithConfig(ctx, &sidecars, pollerCfg, runtimeCfg, poller, worker, func(context.Context, work.WorkRequest) error {
		return nil
	}); err != nil {
		t.Fatalf("StartLinearPoller() error = %v", err)
	}

	waitForObservedLogCount(t, observedLogs, "hosted linear poller restarting", 1, time.Second)
	waitForProviderRequest(t, requestEvents, 1)
	firstRestart := observedLogs.FilterMessage("hosted linear poller restarting").All()[0]
	if got := intField(firstRestart.ContextMap()["consecutive_failures"]); got != 1 {
		t.Fatalf("first consecutive failure count = %d, want 1", got)
	}
	if got := durationField(firstRestart.ContextMap()["selected_delay"]); got != restartBackoffMin {
		t.Fatalf("first selected delay = %s, want %s", got, restartBackoffMin)
	}

	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(restartBackoffMin)
	waitForProviderRequest(t, requestEvents, 2)
	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(time.Hour)
	waitForProviderRequest(t, requestEvents, 3)
	waitForObservedLogCount(t, observedLogs, "hosted linear poller restarting", 2, time.Second)

	secondRestart := observedLogs.FilterMessage("hosted linear poller restarting").All()[1]
	if got := intField(secondRestart.ContextMap()["consecutive_failures"]); got != 1 {
		t.Fatalf("post-recovery consecutive failure count = %d, want reset to 1", got)
	}
	if got := durationField(secondRestart.ContextMap()["selected_delay"]); got != restartBackoffMin {
		t.Fatalf("post-recovery selected delay = %s, want %s", got, restartBackoffMin)
	}

	cancel()
	sidecars.Wait()
}

func TestStartLinearPoller_PersistentFailureRateStaysBounded(t *testing.T) {
	const failureWindow = time.Minute
	fakeClock := clockwork.NewFakeClock()
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":[{"message":"persistent provider failure"}]}`))
	}))
	defer server.Close()

	factoryDir := t.TempDir()
	writeHostedLinearSecretForTest(t, factoryDir)
	logCore, observedLogs := observer.New(zap.InfoLevel)
	pollerCfg, runtimeCfg, poller, worker := hostedLinearPollerFixtureForTest(t, factoryDir, server, nil)
	pollerCfg.Logger = zap.New(logCore)
	pollerCfg.Clock = fakeClock

	ctx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	if err := startLinearPollerWithConfig(ctx, &sidecars, pollerCfg, runtimeCfg, poller, worker, func(context.Context, work.WorkRequest) error {
		return nil
	}); err != nil {
		t.Fatalf("StartLinearPoller() error = %v", err)
	}

	waitForObservedLogCount(t, observedLogs, "hosted linear poller restarting", 1, time.Second)
	elapsed := time.Duration(0)
	for failure := 1; ; failure++ {
		delay := restartBackoff(failure)
		if elapsed+delay > failureWindow {
			break
		}
		waitForFakeClockWaiters(t, fakeClock, 1)
		fakeClock.Advance(delay)
		elapsed += delay
		waitForObservedLogCount(t, observedLogs, "hosted linear poller restarting", failure+1, time.Second)
	}
	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(failureWindow - elapsed)

	got := int(requestCount.Load())
	before := persistentFailureRequestCount(failureWindow, legacyRestartBackoff)
	after := persistentFailureRequestCount(failureWindow, restartBackoff)
	if got != after {
		t.Fatalf("persistent-failure requests = %d, want %d in %s", got, after, failureWindow)
	}
	if got > 20 {
		t.Fatalf("persistent-failure requests = %d, want no more than 20 in %s", got, failureWindow)
	}
	t.Logf(
		"persistent failure over %s: before=%d (%.2f requests/s), after=%d (%.2f requests/s)",
		failureWindow,
		before,
		float64(before)/failureWindow.Seconds(),
		got,
		float64(got)/failureWindow.Seconds(),
	)

	cancel()
	sidecars.Wait()
}

func TestStartLinearPoller_StopsOnContextCancellationDuringProviderBackoff(t *testing.T) {
	fakeClock := clockwork.NewFakeClock()
	var requestCount atomic.Int32
	logCore, observedLogs := observer.New(zap.InfoLevel)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	factoryDir := t.TempDir()
	writeHostedLinearSecretForTest(t, factoryDir)
	pollerCfg, runtimeCfg, poller, worker := hostedLinearPollerFixtureForTest(t, factoryDir, server, nil)
	pollerCfg.Logger = zap.New(logCore)
	pollerCfg.Clock = fakeClock

	ctx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	if err := startLinearPollerWithConfig(ctx, &sidecars, pollerCfg, runtimeCfg, poller, worker, func(context.Context, work.WorkRequest) error {
		return nil
	}); err != nil {
		t.Fatalf("StartLinearPoller() error = %v", err)
	}

	waitForObservedLogMessage(t, observedLogs, "hosted linear poller restarting", time.Second)
	restartEntry := observedLogs.FilterMessage("hosted linear poller restarting").All()[0]
	if got := durationField(restartEntry.ContextMap()["selected_delay"]); got != time.Hour {
		t.Fatalf("provider selected delay = %s, want 1h", got)
	}
	if got := fieldString(restartEntry.ContextMap()["delay_source"]); got != "provider" {
		t.Fatalf("provider delay source = %q, want provider", got)
	}
	waitForFakeClockWaiters(t, fakeClock, 1)
	cancel()
	sidecars.Wait()

	if got := requestCount.Load(); got != 1 {
		t.Fatalf("provider request count = %d, want no retry after provider-delay cancellation", got)
	}
	if got := observedLogs.FilterMessage("hosted linear poller restarting").Len(); got != 1 {
		t.Fatalf("restart log count = %d, want 1", got)
	}
}

func waitForProviderRequest(t *testing.T, requests <-chan int, want int) {
	t.Helper()
	// The handler channel synchronizes directly with the fake HTTP provider; the
	// timeout only guards a broken poller without adding a scheduling delay.
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for {
		select {
		case got := <-requests:
			if got == want {
				return
			}
			if got > want {
				t.Fatalf("provider request event = %d, want %d", got, want)
			}
		case <-timeout.C:
			t.Fatalf("timed out waiting for provider request %d", want)
		}
	}
}

func legacyRestartBackoff(consecutiveFailures int) time.Duration {
	const (
		legacyMinimum = 25 * time.Millisecond
		legacyMaximum = 250 * time.Millisecond
	)
	if consecutiveFailures <= 1 {
		return legacyMinimum
	}
	backoff := legacyMinimum
	for failure := 1; failure < consecutiveFailures && backoff < legacyMaximum; failure++ {
		backoff *= 2
		if backoff > legacyMaximum {
			return legacyMaximum
		}
	}
	return backoff
}

func persistentFailureRequestCount(window time.Duration, delay func(int) time.Duration) int {
	elapsed := time.Duration(0)
	requests := 1
	for failure := 1; ; failure++ {
		nextDelay := delay(failure)
		if elapsed+nextDelay > window {
			return requests
		}
		elapsed += nextDelay
		requests++
	}
}
