package factorysessionexecution

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

var executionTestSessionIdentity atomic.Uint64

func testSessionIDGenerator() string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", executionTestSessionIdentity.Add(1))
}

// constructorWorkflowContracts proves that constructor validation receives the
// three Factory Runtime root roles. These tests do not execute their methods.
type constructorWorkflowContracts struct {
	factory.JavaScriptWorkflowDefinitions
	factory.JavaScriptWorkflowRuntime
	factory.JavaScriptChildValues
}

func (c constructorWorkflowContracts) RunJavaScript(
	ctx context.Context,
	req factory.JavaScriptRuntimeRequest,
	hooks factory.JavaScriptRuntimeHooks,
) (factory.JavaScriptRuntimeOutcome, error) {
	return c.Run(ctx, req, hooks)
}

func (c constructorWorkflowContracts) ResumeJavaScript(
	summary factory.JavaScriptCompletedCheckpointSummary,
	records []factory.JavaScriptRuntimeRecord,
) factory.JavaScriptResumeContext {
	return c.ResumeContext(summary, records)
}

// serviceConfig keeps table-driven constructor tests compact without restoring
// a production dependency bag.
type serviceConfig struct {
	ProjectRoot       string
	ChildExecutorMode string
	Provider          workerprovider.Provider
	ProviderExecutor  workers.InvocationExecutor
	FakeScenarios     []FakeScenario
	Persistence       PersistenceChoice
	Clock             factory.Clock
	WorkerPresetIDs   map[string]struct{}
	WorkerSettings    factory.JavaScriptWorkerSettings
}

func newExecutionService(provider ExecutionProvider, config serviceConfig) (Service, error) {
	switch provider {
	case ExecutionProviderFake:
		clock := config.Clock
		if clock == nil {
			clock = durableFixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		}
		return NewFakeService(clock, config.FakeScenarios...)
	case ExecutionProviderJavaScriptRuntime:
		workflows := constructorWorkflowContracts{}
		return NewJavaScriptExecutionService(
			config.ProjectRoot,
			config.ChildExecutorMode,
			firstInvocationExecutor(config.ProviderExecutor, config.Provider),
			config.Persistence,
			config.Clock,
			testSyncWaitScheduler{},
			checkpointfixtures.CheckpointSummariesFixture{
				BuildResult:  checkpointfixtures.ResumableCheckpointSummaryResult(),
				LatestResult: checkpointfixtures.ResumableCheckpointSummaryResult(),
			},
			workflows,
			workflows,
			workflows,
			config.WorkerPresetIDs,
			config.WorkerSettings,
			mustTestRecordingWriter(),
			testSessionIDGenerator,
			nil, nil, nil,
		)
	default:
		return nil, NewValidationError("provider", "unsupported execution provider")
	}
}

func firstInvocationExecutor(executor workers.InvocationExecutor, provider workerprovider.Provider) workers.InvocationExecutor {
	if executor != nil {
		return executor
	}
	if provider == nil {
		return nil
	}
	return constructorInvocationExecutor{}
}

// constructorInvocationExecutor is an inert root-contract value. Constructor
// tests validate dependency presence only; Workers owns invocation behavior.
type constructorInvocationExecutor struct {
	workers.InvocationExecutor
}

type testSyncWaitScheduler struct{}

func (testSyncWaitScheduler) Now() time.Time { return time.Now() }

func (testSyncWaitScheduler) After(duration time.Duration) <-chan time.Time {
	return time.After(duration)
}

func TestNewJavaScriptExecutionServiceRequiresSyncWaitScheduler(t *testing.T) {
	t.Parallel()
	workflows := constructorWorkflowContracts{}
	_, err := NewJavaScriptExecutionService(
		t.TempDir(),
		ChildExecutorModeFake,
		nil,
		DisabledPersistence(),
		durableFixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		nil,
		checkpointfixtures.CheckpointSummariesFixture{},
		workflows,
		workflows,
		workflows,
		nil,
		factory.JavaScriptWorkerSettings{},
		mustTestRecordingWriter(),
		testSessionIDGenerator,
		nil, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "sync wait scheduler is required") {
		t.Fatalf("NewJavaScriptExecutionService error = %v, want missing sync wait scheduler", err)
	}
}

func TestWaitSyncCompletionUsesInjectedClockAndRecurringScheduler(t *testing.T) {
	t.Parallel()
	clock := newControlledSyncWaitClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	service := &JavaScriptRuntimeService{
		clock:     clock,
		syncWaits: clock,
		sessions: map[string]*runtimeSessionState{
			"session-1": {session: SessionReadResult{SessionID: "session-1", Status: LifecycleStatusRunning}},
		},
	}

	result := make(chan SyncStartResult, 1)
	errs := make(chan error, 1)
	go func() {
		got, err := service.waitSyncCompletion(context.Background(), "session-1", 20*time.Millisecond, false)
		result <- got
		errs <- err
	}()

	if duration := <-clock.requests; duration != 10*time.Millisecond {
		t.Fatalf("first scheduled wait = %s, want 10ms", duration)
	}
	clock.Advance(10 * time.Millisecond)
	if duration := <-clock.requests; duration != 10*time.Millisecond {
		t.Fatalf("recurring scheduled wait = %s, want 10ms", duration)
	}
	select {
	case got := <-result:
		t.Fatalf("wait completed before injected deadline: %#v", got)
	default:
	}

	clock.Advance(10 * time.Millisecond)
	if err := <-errs; err != nil {
		t.Fatalf("waitSyncCompletion: %v", err)
	}
	got := <-result
	if !got.TimedOut || got.SyncOutcome != SyncOutcomeTimedOut {
		t.Fatalf("result = %#v, want injected-clock timeout", got)
	}
}

type controlledSyncWaitClock struct {
	mu       sync.Mutex
	now      time.Time
	waiters  []chan time.Time
	requests chan time.Duration
}

func newControlledSyncWaitClock(now time.Time) *controlledSyncWaitClock {
	return &controlledSyncWaitClock{now: now, requests: make(chan time.Duration, 4)}
}

func (c *controlledSyncWaitClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *controlledSyncWaitClock) After(duration time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	waiter := make(chan time.Time, 1)
	c.waiters = append(c.waiters, waiter)
	c.requests <- duration
	return waiter
}

func (c *controlledSyncWaitClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	now := c.now
	waiters := c.waiters
	c.waiters = nil
	c.mu.Unlock()
	for _, waiter := range waiters {
		waiter <- now
	}
}
