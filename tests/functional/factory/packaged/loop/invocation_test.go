package loop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPackagedLoop(t *testing.T) {
	fixture := newLoopSharedFixture(t)
	primaryRan := false
	t.Run("TestPackagedLoopUsesInvocationDurationAndSkipsOverlap", func(t *testing.T) {
		primaryRan = true
		testPackagedLoopUsesInvocationDurationAndSkipsOverlap(t, fixture)
	})
	invalidRan := false
	t.Run("TestPackagedLoopRejectsInvalidDurationBeforeWorkAdmission", func(t *testing.T) {
		invalidRan = true
		testPackagedLoopRejectsInvalidDurationBeforeWorkAdmission(t, fixture)
	})
	boundaryCases := 0
	t.Run("TestPackagedLoopDurationBoundaries", func(t *testing.T) {
		boundaryCases = testPackagedLoopDurationBoundaries(t, fixture)
	})
	expected := boundaryCases
	if primaryRan {
		expected++
	}
	if invalidRan {
		expected++
	}
	fixture.lifecycle.setExpectedSessions(expected)
}

func testPackagedLoopUsesInvocationDurationAndSkipsOverlap(t *testing.T, fixture *loopSharedFixture) {
	observer := newLoopPhaseObserver(t)
	start := fixture.clock.Now()
	schedulerBeforeInvocation := fixture.clock.schedulerTimerRegistrations()
	runner := newBlockingLoopRunner()
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)

	response := invokeLoop(t, scenario, map[string]any{
		"request": "check dependency updates", "every": "1m",
		"triggerAtStart": "true", "maxConsecutiveFailures": "0",
	})
	if response.Status != factoryapi.InvocationTerminalStatusTimedOut {
		t.Fatalf("invocation status = %q, want TIMED_OUT for long-lived controller", response.Status)
	}

	first := waitForLoopSubmission(observer, fixture.submissions, "scheduled-execution")
	assertLoopSubmission(t, first, "init", "SCHEDULED", "1", start, start)
	observer.waitForSignal(
		"provider execution start",
		runner.started,
		func() string { return fmt.Sprintf("provider_calls=%d", runner.calls()) },
	)

	observer.waitForSchedulerTimer("scheduler initial registration", fixture.clock, schedulerBeforeInvocation)
	schedulerBeforeAdvance := fixture.clock.schedulerTimerRegistrations()
	fixture.clock.Advance(time.Minute)
	skipped := waitForLoopSubmission(observer, fixture.submissions, "scheduled-execution")
	assertLoopSubmission(t, skipped, "skipped", "SKIPPED_OVERLAP", "2", start.Add(time.Minute), start.Add(time.Minute))
	if runner.calls() != 1 {
		t.Fatalf("overlapping executor calls = %d, want 1", runner.calls())
	}

	runner.Release()
	waitForLoopDispatchCompletion(observer, scenario)
	observer.waitForSchedulerTimer("scheduler re-arm", fixture.clock, schedulerBeforeAdvance)
	fixture.clock.Advance(time.Minute)
	recovered := waitForLoopSubmission(observer, fixture.submissions, "scheduled-execution")
	assertLoopSubmission(t, recovered, "init", "SCHEDULED", "3", start.Add(2*time.Minute), start.Add(2*time.Minute))
}

func testPackagedLoopRejectsInvalidDurationBeforeWorkAdmission(t *testing.T, fixture *loopSharedFixture) {
	scenario := fixture.newScenario(t, support.NewRecordingCommandRunner("unused"))
	scenario.open(t)

	status := postLoopInvocation(t, scenario, map[string]any{"request": "check", "every": "tomorrow"}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid duration status = %d, want 400", status)
	}
	select {
	case record := <-fixture.submissions:
		t.Fatalf("invalid duration admitted Work = %#v", record)
	default:
	}
}

func testPackagedLoopDurationBoundaries(t *testing.T, fixture *loopSharedFixture) int {
	cases := []struct {
		name     string
		every    string
		accepted bool
	}{
		{name: "minimum_1s", every: "1s", accepted: true},
		{name: "maximum_168h", every: "168h", accepted: true},
		{name: "empty", every: "", accepted: false},
		{name: "below_minimum_999ms", every: "999ms", accepted: false},
		{name: "above_maximum_168h1s", every: "168h1s", accepted: false},
		{name: "malformed_tomorrow", every: "tomorrow", accepted: false},
	}
	runCases := 0
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			runCases++
			scenario := fixture.newScenario(t, support.NewRecordingCommandRunner("duration boundary"))
			scenario.open(t)
			args := map[string]any{
				"request":                "check dependency updates",
				"every":                  testCase.every,
				"triggerAtStart":         "true",
				"maxConsecutiveFailures": "0",
			}
			if !testCase.accepted {
				status := postLoopInvocation(t, scenario, args, nil)
				if status != http.StatusBadRequest {
					t.Fatalf("duration %q status = %d, want 400", testCase.every, status)
				}
				assertNoLoopSubmission(t, fixture.submissions)
				return
			}

			start := fixture.clock.Now()
			timeoutMillis := int64(20)
			status := postLoopInvocation(t, scenario, args, nil, &timeoutMillis)
			if status != http.StatusOK {
				t.Fatalf("duration %q status = %d, want 200", testCase.every, status)
			}
			observer := newLoopPhaseObserver(t)
			submission := waitForLoopSubmission(observer, fixture.submissions, "scheduled-execution")
			assertLoopSubmission(t, submission, "init", "SCHEDULED", "1", start, start)
		})
	}
	return runCases
}

func assertNoLoopSubmission(t *testing.T, submissions <-chan work.FactorySubmissionRecord) {
	t.Helper()
	select {
	case record := <-submissions:
		t.Fatalf("duration validation admitted Work = %#v", record)
	default:
	}
}

type blockingLoopRunner struct {
	started      chan struct{}
	release      chan struct{}
	done         chan struct{}
	startOnce    sync.Once
	releaseOnce  sync.Once
	doneOnce     sync.Once
	mu           sync.Mutex
	count        int
	releaseCalls atomic.Uint32
}

// loopSchedulerClock keeps the fake clock's scheduler-specific timer
// registration observable. FakeClock.BlockUntilContext only reports the total
// waiter count, so an unrelated server timer could otherwise release an
// advance before the loop scheduler re-arms.
type loopSchedulerClock struct {
	*clockwork.FakeClock
	schedulerTimers     chan struct{}
	schedulerTimerCount atomic.Uint64
	schedulerTimerStops atomic.Uint64
}

type loopSchedulerTimer struct {
	clockwork.Timer
	stopCount *atomic.Uint64
}

func (timer *loopSchedulerTimer) Stop() bool {
	if timer == nil {
		return false
	}
	if timer.stopCount != nil {
		timer.stopCount.Add(1)
	}
	return timer.Timer.Stop()
}

var _ clockwork.Clock = (*loopSchedulerClock)(nil)

func newLoopSchedulerClockAt(start time.Time) *loopSchedulerClock {
	return &loopSchedulerClock{
		FakeClock:       clockwork.NewFakeClockAt(start),
		schedulerTimers: make(chan struct{}, 8),
	}
}

func (clock *loopSchedulerClock) AfterFunc(duration time.Duration, callback func()) clockwork.Timer {
	timer := clock.FakeClock.AfterFunc(duration, callback)
	if loopSchedulerTimerCaller() {
		clock.schedulerTimerCount.Add(1)
		select {
		case clock.schedulerTimers <- struct{}{}:
		default:
		}
		return &loopSchedulerTimer{Timer: timer, stopCount: &clock.schedulerTimerStops}
	}
	return timer
}

func (clock *loopSchedulerClock) schedulerTimerRegistrations() uint64 {
	return clock.schedulerTimerCount.Load()
}

func loopSchedulerTimerCaller() bool {
	pcs := make([]uintptr, 8)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, "gocron/v2.(*scheduler).selectStart") ||
			strings.Contains(frame.Function, "gocron/v2.(*scheduler).selectExecJobsOutForRescheduling") {
			return true
		}
		if !more {
			return false
		}
	}
}

func newBlockingLoopRunner() *blockingLoopRunner {
	return &blockingLoopRunner{
		started: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (runner *blockingLoopRunner) Release() {
	if runner == nil {
		return
	}
	runner.releaseCalls.Add(1)
	runner.releaseOnce.Do(func() { close(runner.release) })
}

func (runner *blockingLoopRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	defer runner.doneOnce.Do(func() { close(runner.done) })
	runner.mu.Lock()
	runner.count++
	runner.mu.Unlock()
	runner.startOnce.Do(func() { close(runner.started) })
	select {
	case <-runner.release:
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("scheduled execution complete")}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func (runner *blockingLoopRunner) calls() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.count
}

func (runner *blockingLoopRunner) releaseCount() uint32 {
	if runner == nil {
		return 0
	}
	return runner.releaseCalls.Load()
}

func invokeLoop(t *testing.T, scenario *loopScenario, args map[string]any) factoryapi.InvocationResponse {
	t.Helper()
	timeoutMillis := int64(20)
	var response factoryapi.InvocationResponse
	status := postLoopInvocation(t, scenario, args, func(decoded factoryapi.InvocationResponse) { response = decoded }, &timeoutMillis)
	if status != http.StatusOK {
		t.Fatalf("loop invocation status = %d, want 200", status)
	}
	return response
}

func postLoopInvocation(
	t *testing.T,
	scenario *loopScenario,
	args map[string]any,
	decode func(factoryapi.InvocationResponse),
	timeout ...*int64,
) int {
	t.Helper()
	requestID := fmt.Sprintf("packaged-loop-%d", time.Now().UnixNano())
	request := factoryapi.InvocationRequest{RequestId: &requestID, Args: &args}
	if len(timeout) > 0 {
		request.TimeoutMillis = timeout[0]
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal loop invocation: %v", err)
	}
	endpoint := scenario.fixture.baseURL + "/factory-sessions/" + scenario.sessionID + "/invocations"
	ctx, cancel := context.WithTimeout(t.Context(), loopInvocationRequestBudget)
	defer cancel()
	requestWithContext, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("build loop invocation request: %v", err)
	}
	requestWithContext.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(requestWithContext)
	if err != nil {
		t.Fatalf("POST loop invocation: %v", err)
	}
	defer response.Body.Close()
	if decode != nil && response.StatusCode == http.StatusOK {
		var decoded factoryapi.InvocationResponse
		if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
			t.Fatalf("decode loop invocation: %v", err)
		}
		decode(decoded)
	}
	return response.StatusCode
}

func assertLoopSubmission(
	t *testing.T,
	record work.FactorySubmissionRecord,
	wantState, wantOutcome, wantSequence string,
	wantNominal, wantActual time.Time,
) {
	t.Helper()
	request := record.Request
	if request.TargetState != wantState {
		t.Fatalf("scheduled state = %q, want %q", request.TargetState, wantState)
	}
	if request.Tags["agent_factory.time.trigger_outcome"] != wantOutcome || request.Tags["agent_factory.time.sequence"] != wantSequence {
		t.Fatalf("scheduled outcome tags = %#v", request.Tags)
	}
	if request.Tags[factorydefinitions.TimeWorkTagKeyNominalAt] != wantNominal.Format(time.RFC3339Nano) ||
		request.Tags["agent_factory.time.actual_at"] != wantActual.Format(time.RFC3339Nano) {
		t.Fatalf("scheduled timing tags = %#v", request.Tags)
	}
}

type loopPhaseObserver struct {
	t     testing.TB
	phase string
	last  string
}

func newLoopPhaseObserver(t testing.TB) *loopPhaseObserver {
	t.Helper()
	return &loopPhaseObserver{t: t}
}

func (observer *loopPhaseObserver) begin(phase, last string) {
	observer.phase = phase
	observer.last = last
}

func (observer *loopPhaseObserver) fail(budget time.Duration, err error) {
	observer.t.Fatalf(
		"LOOP-CONTENTION-01 phase %q timed out after %s; last observation=%s; cause=%v",
		observer.phase,
		budget,
		observer.last,
		err,
	)
}

func (observer *loopPhaseObserver) phaseContext(
	phase, last string, budget time.Duration,
) (context.Context, context.CancelFunc) {
	observer.begin(phase, last)
	return context.WithTimeout(observer.t.Context(), budget)
}

func (observer *loopPhaseObserver) waitForSignal(
	phase string,
	signal <-chan struct{},
	last func() string,
) {
	observer.t.Helper()
	ctx, cancel := observer.phaseContext(phase, last(), loopProviderPhaseBudget)
	defer cancel()
	select {
	case <-signal:
		observer.last = "signal received"
	case <-ctx.Done():
		observer.fail(loopProviderPhaseBudget, ctx.Err())
	}
}

func (observer *loopPhaseObserver) waitForSchedulerTimer(
	phase string,
	clock *loopSchedulerClock,
	after uint64,
) {
	observer.t.Helper()
	ctx, cancel := observer.phaseContext(
		phase,
		fmt.Sprintf("scheduler_timer_registrations=%d want>%d", clock.schedulerTimerRegistrations(), after),
		loopSchedulerPhaseBudget,
	)
	defer cancel()
	for {
		registrations := clock.schedulerTimerRegistrations()
		observer.last = fmt.Sprintf("scheduler_timer_registrations=%d want>%d", registrations, after)
		if registrations > after {
			return
		}
		select {
		case <-clock.schedulerTimers:
		case <-ctx.Done():
			observer.fail(loopSchedulerPhaseBudget, ctx.Err())
		}
	}
}

func waitForLoopSubmission(
	observer *loopPhaseObserver,
	submissions <-chan work.FactorySubmissionRecord,
	workType string,
) work.FactorySubmissionRecord {
	observer.t.Helper()
	ctx, cancel := observer.phaseContext(
		"public Work submission "+workType,
		"no matching Work submission",
		loopWorkPhaseBudget,
	)
	defer cancel()
	for {
		select {
		case record := <-submissions:
			observer.last = describeLoopSubmission(record)
			if record.Request.WorkTypeID == workType {
				return record
			}
		case <-ctx.Done():
			observer.fail(loopWorkPhaseBudget, ctx.Err())
		}
	}
}

func describeLoopSubmission(record work.FactorySubmissionRecord) string {
	return fmt.Sprintf(
		"work_type=%q target_state=%q outcome=%q sequence=%q",
		record.Request.WorkTypeID,
		record.Request.TargetState,
		record.Request.Tags["agent_factory.time.trigger_outcome"],
		record.Request.Tags["agent_factory.time.sequence"],
	)
}

func waitForLoopDispatchCompletion(observer *loopPhaseObserver, scenario *loopScenario) {
	observer.t.Helper()
	observer.begin("public dispatch completion", "factory_events=0 dispatches=0 completed=false")
	_, err := support.WaitForObservation(
		loopDispatchPhaseBudget,
		func() ([]factoryapi.FactoryEvent, error) {
			events := support.GetFactoryEventsForSessionAt(observer.t, scenario.fixture.baseURL, scenario.sessionID)
			dispatches := support.ObserveDispatchEvents(observer.t, events)
			completed := len(dispatches) > 0 && dispatches[len(dispatches)-1].Response != nil
			observer.last = fmt.Sprintf(
				"factory_events=%d dispatches=%d completed=%t",
				len(events), len(dispatches), completed,
			)
			return events, nil
		},
		func(events []factoryapi.FactoryEvent) bool {
			dispatches := support.ObserveDispatchEvents(observer.t, events)
			return len(dispatches) > 0 && dispatches[len(dispatches)-1].Response != nil
		},
	)
	if err != nil {
		observer.fail(loopDispatchPhaseBudget, fmt.Errorf("public dispatch observation: %w", err))
	}
}
