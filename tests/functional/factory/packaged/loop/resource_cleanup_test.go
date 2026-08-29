package loop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	loopCleanupActionRoot     = "scenario root"
	loopCleanupActionRoutes   = "provider route registrations"
	loopCleanupActionSession  = "Factory Session"
	loopCleanupActionRunner   = "controlled provider runner"
	loopCleanupActionEvent    = "Factory Event stream"
	loopCleanupActionResponse = "Factory Response Event stream"
)

// TestLoopCleanupResourceMatrix drives the package cleanup path after real
// production-composed acquisition. The matrix intentionally keeps the shared
// process alive so its listener/process/clock cleanup is checked by the same
// fixture probe as the normal loop behavior.
func TestLoopCleanupResourceMatrix(t *testing.T) {
	fixture := newLoopSharedFixtureWithExpected(t, 0)
	var (
		validationRan   bool
		blockedRan      bool
		earlyFailureRan bool
	)

	t.Run("validation-failure", func(t *testing.T) {
		validationRan = true
		runLoopValidationCleanup(t, fixture)
	})

	t.Run("dependency-blocked-cancellation", func(t *testing.T) {
		blockedRan = true
		runLoopBlockedCancellationCleanup(t, fixture)
	})

	t.Run("early-assertion-with-streams", func(t *testing.T) {
		earlyFailureRan = true
		runLoopEarlyAssertionCleanup(t, fixture)
	})

	t.Run("partial-acquisition", func(t *testing.T) {
		runLoopPartialAcquisitionCleanup(t, fixture)
	})

	sessions := 0
	for _, ran := range []bool{validationRan, blockedRan, earlyFailureRan} {
		if ran {
			sessions++
		}
	}
	fixture.lifecycle.setExpectedSessions(sessions)
}

func runLoopValidationCleanup(t *testing.T, fixture *loopSharedFixture) {
	t.Helper()
	runner := support.NewRecordingCommandRunner("validation")
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	status := postLoopInvocation(t, scenario, map[string]any{
		"request": "check dependency updates", "every": "tomorrow",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid duration status = %d, want 400", status)
	}
	assertNoLoopSubmission(t, fixture.submissions)

	if err := scenario.cleanup.run(); err != nil {
		t.Fatalf("validation cleanup: %v", err)
	}
	assertLoopScenarioResources(t, scenario, true, map[string]int{
		loopCleanupActionRunner:  1,
		loopCleanupActionSession: 1,
		loopCleanupActionRoutes:  1,
		loopCleanupActionRoot:    1,
	})
	if calls := runner.CallCount(); calls != 0 {
		t.Fatalf("validation provider calls = %d, want 0", calls)
	}
}

func runLoopBlockedCancellationCleanup(t *testing.T, fixture *loopSharedFixture) {
	t.Helper()
	timersBefore := fixture.clock.schedulerTimerRegistrations()
	stopsBefore := fixture.clock.schedulerTimerStops.Load()
	runner := newBlockingLoopRunner()
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	observer := newLoopPhaseObserver(t)
	start := fixture.clock.Now()
	schedulerBeforeInvocation := fixture.clock.schedulerTimerRegistrations()
	response := invokeLoop(t, scenario, map[string]any{
		"request": "check dependency updates", "every": "1m",
		"triggerAtStart": "true", "maxConsecutiveFailures": "0",
	})
	if response.Status != factoryapi.InvocationTerminalStatusTimedOut {
		t.Fatalf("blocked invocation status = %q, want TIMED_OUT", response.Status)
	}
	first := waitForLoopSubmission(observer, fixture.submissions, "scheduled-execution")
	assertLoopSubmission(t, first, "init", "SCHEDULED", "1", start, start)
	observer.waitForSignal("provider execution start", runner.started, func() string {
		return fmt.Sprintf("provider_calls=%d", runner.calls())
	})
	observer.waitForSchedulerTimer(
		"scheduler initial registration", fixture.clock, schedulerBeforeInvocation,
	)
	fixture.clock.Advance(time.Minute)
	skipped := waitForLoopSubmission(observer, fixture.submissions, "scheduled-execution")
	assertLoopSubmission(t, skipped, "skipped", "SKIPPED_OVERLAP", "2", start.Add(time.Minute), start.Add(time.Minute))
	if calls := runner.calls(); calls != 1 {
		t.Fatalf("blocked provider calls = %d, want 1", calls)
	}

	// The invocation deadline has already canceled the caller while the
	// controlled provider remains blocked. Scenario cleanup must release the
	// runner before terminating the public session.
	if err := scenario.cleanup.run(); err != nil {
		t.Fatalf("blocked/canceled cleanup: %v", err)
	}
	assertLoopScenarioResources(t, scenario, true, map[string]int{
		loopCleanupActionRunner:  1,
		loopCleanupActionSession: 1,
		loopCleanupActionRoutes:  1,
		loopCleanupActionRoot:    1,
	})
	if got := runner.releaseCount(); got != 1 {
		t.Fatalf("blocked runner release calls = %d, want 1", got)
	}
	waitForLoopRunnerDone(t, runner)
	registrations := fixture.clock.schedulerTimerRegistrations() - timersBefore
	stops := fixture.clock.schedulerTimerStops.Load() - stopsBefore
	if registrations == 0 || stops != registrations {
		t.Fatalf("scheduler timer cleanup registrations=%d stops=%d; want one stop per registration", registrations, stops)
	}
}

func runLoopEarlyAssertionCleanup(t *testing.T, fixture *loopSharedFixture) {
	t.Helper()
	runner := newBlockingLoopRunner()
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	eventStream := openLoopHTTPStream(t, support.SessionEventsURL(
		scenario.fixture.baseURL, scenario.sessionID,
	))
	scenario.cleanup.add(loopCleanupActionEvent, eventStream.Close)
	responseStream := openLoopHTTPStream(t, support.SessionResponseEventsURL(
		scenario.fixture.baseURL, scenario.sessionID,
	))
	scenario.cleanup.add(loopCleanupActionResponse, responseStream.Close)
	streamErr := errors.New("injected event stream cleanup failure")
	eventStream.injectedError = streamErr
	assertionErr := errors.New("injected early assertion")
	scenario.cleanup.add("early assertion", func() error { return assertionErr })

	err := scenario.cleanup.run()
	if !errors.Is(err, streamErr) || !errors.Is(err, assertionErr) {
		t.Fatalf("early cleanup error = %v, want stream and assertion failures", err)
	}
	assertLoopScenarioResources(t, scenario, true, map[string]int{
		"early assertion":         1,
		loopCleanupActionResponse: 1,
		loopCleanupActionEvent:    1,
		loopCleanupActionRunner:   1,
		loopCleanupActionSession:  1,
		loopCleanupActionRoutes:   1,
		loopCleanupActionRoot:     1,
	})
	assertLoopHTTPStreamClosed(t, eventStream)
	assertLoopHTTPStreamClosed(t, responseStream)
	if got := runner.releaseCount(); got != 1 {
		t.Fatalf("early-failure runner release calls = %d, want 1", got)
	}
}

func runLoopPartialAcquisitionCleanup(t *testing.T, fixture *loopSharedFixture) {
	t.Helper()
	runner := support.NewRecordingCommandRunner("partial")
	scenario := fixture.newScenario(t, runner)
	setupErr := errors.New("injected setup assertion")
	scenario.cleanup.add("setup assertion", func() error { return setupErr })
	if err := scenario.cleanup.run(); !errors.Is(err, setupErr) {
		t.Fatalf("partial cleanup error = %v, want setup assertion", err)
	}
	assertLoopScenarioResources(t, scenario, false, map[string]int{
		"setup assertion":       1,
		loopCleanupActionRoutes: 1,
		loopCleanupActionRoot:   1,
	})
	if scenario.sessionID != "" || scenario.lifecycleTracked {
		t.Fatalf("partial scenario acquired a session: %#v", scenario)
	}
	if calls := runner.CallCount(); calls != 0 {
		t.Fatalf("partial provider calls = %d, want 0", calls)
	}
}

func assertLoopScenarioResources(
	t *testing.T,
	scenario *loopScenario,
	sessionAcquired bool,
	wantActions map[string]int,
) {
	t.Helper()
	for name, want := range wantActions {
		if got := scenario.cleanup.actionCalls(name); got != want {
			t.Errorf("cleanup action %q calls = %d, want %d", name, got, want)
		}
	}
	if got := scenario.fixture.provider.registeredCount(); got != 0 {
		t.Errorf("provider routes after scenario cleanup = %d, want 0", got)
	}
	if got, want := scenario.fixture.provider.unregisterCount(), scenario.routeUnregisterBefore+1; got != want {
		t.Errorf("provider route unregister attempts = %d, want %d", got, want)
	}
	if !scenario.rootRemoved {
		t.Errorf("scenario root removed = false, want true")
	}
	if sessionAcquired && !scenario.sessionAbsent {
		t.Errorf("Factory Session %q remains publicly present after cleanup", scenario.sessionID)
	}
	if sessionAcquired {
		ctx, cancel := context.WithTimeout(context.Background(), loopSessionAbsentBudget)
		defer cancel()
		absent, err := observeLoopSessionAbsent(ctx, scenario.fixture.baseURL, scenario.sessionID)
		if err != nil {
			t.Errorf("public Factory Session absence probe: %v", err)
		} else if !absent {
			t.Errorf("public Factory Session %q absence probe = false, want true", scenario.sessionID)
		}
	}
}

func waitForLoopRunnerDone(t *testing.T, runner *blockingLoopRunner) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), loopSessionStoppedBudget)
	defer cancel()
	select {
	case <-runner.done:
	case <-ctx.Done():
		t.Fatalf("blocked provider goroutine remained after cleanup: %v", ctx.Err())
	}
}

type loopHTTPStream struct {
	cancel        context.CancelFunc
	body          io.ReadCloser
	done          chan struct{}
	closeOnce     sync.Once
	closeCalls    atomic.Uint32
	injectedError error
	closeError    error
}

func openLoopHTTPStream(t testing.TB, endpoint string) *loopHTTPStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		t.Fatalf("build cleanup stream request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		cancel()
		t.Fatalf("open cleanup stream: %v", err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		cancel()
		t.Fatalf("cleanup stream status/content type = %d/%q, want 200/event-stream: %s", response.StatusCode, response.Header.Get("Content-Type"), strings.TrimSpace(string(body)))
	}
	stream := &loopHTTPStream{cancel: cancel, body: response.Body, done: make(chan struct{})}
	go func() {
		_, _ = io.Copy(io.Discard, response.Body)
		close(stream.done)
	}()
	return stream
}

func (stream *loopHTTPStream) Close() error {
	if stream == nil {
		return nil
	}
	stream.closeOnce.Do(func() {
		stream.closeCalls.Add(1)
		if stream.cancel != nil {
			stream.cancel()
		}
		if stream.body != nil {
			stream.closeError = stream.body.Close()
		}
		closeTimer := time.NewTimer(loopStreamCloseBudget)
		defer closeTimer.Stop()
		select {
		case <-stream.done:
		case <-closeTimer.C:
			stream.closeError = errors.Join(stream.closeError, fmt.Errorf("stream reader did not stop"))
		}
		stream.closeError = errors.Join(stream.closeError, stream.injectedError)
	})
	return stream.closeError
}

func assertLoopHTTPStreamClosed(t *testing.T, stream *loopHTTPStream) {
	t.Helper()
	if got := stream.closeCalls.Load(); got != 1 {
		t.Errorf("cleanup stream close calls = %d, want 1", got)
	}
	select {
	case <-stream.done:
	default:
		t.Errorf("cleanup stream reader remains active")
	}
}
