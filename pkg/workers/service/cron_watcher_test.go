package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
)

func cronWorkstationConfigForTest(name string) interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name: name,
		Kind: workertaxonomy.WorkstationKindCron,
		Cron: &interfaces.CronConfig{Schedule: "* * * * *"},
		Outputs: []interfaces.IOConfig{
			{WorkTypeName: "task", StateName: "init"},
		},
	}
}

func newCronLoadedRuntimeConfig(
	t *testing.T,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	workstationConfigs map[string]*interfaces.FactoryWorkstationConfig,
) *config.LoadedFactoryConfig {
	t.Helper()

	loaded, err := config.NewLoadedFactoryConfig(factoryDir, factoryCfg, runtimefixtures.RuntimeDefinitionLookupFixture{
		Workstations: workstationConfigs,
	})
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

type cronRuntimeWorkstationLookup struct {
	Workstations map[string]*interfaces.FactoryWorkstationConfig
}

func (c cronRuntimeWorkstationLookup) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	if c.Workstations == nil {
		return nil, false
	}
	ws, ok := c.Workstations[name]
	return ws, ok
}

func TestSubmitCronTick_TimeoutFailureIsClassifiedAndBounded(t *testing.T) {
	logCore, observedLogs := observer.New(zap.InfoLevel)
	submitCalls := 0
	submitter := func(ctx context.Context, _ work.WorkRequest) error {
		submitCalls++
		<-ctx.Done()
		return ctx.Err()
	}
	runtimeCfg := newCronLoadedRuntimeConfig(t, "", &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:   "poll-for-work",
			Limits: interfaces.WorkstationLimits{MaxExecutionTime: "1ms"},
		}},
	}, map[string]*interfaces.FactoryWorkstationConfig{
		"poll-for-work": {
			Name:   "poll-for-work",
			Limits: interfaces.WorkstationLimits{MaxExecutionTime: "1ms"},
		},
	})
	svc := workersservice.New(workersservice.Config{Logger: zap.New(logCore)})

	err := svc.SubmitCronTick(
		context.Background(),
		runtimeCfg,
		"",
		submitter,
		cronWorkstationConfigForTest("poll-for-work"),
		time.Now(),
	)
	if err == nil {
		t.Fatal("expected timed-out cron tick to fail after bounded retries")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cron tick error = %v, want deadline exceeded classification", err)
	}
	if submitCalls != workersservice.CronMaxRetries+1 {
		t.Fatalf("cron submit attempts = %d, want %d", submitCalls, workersservice.CronMaxRetries+1)
	}
	if observedLogs.FilterMessage("cron watcher trigger retrying").Len() != workersservice.CronMaxRetries {
		t.Fatalf("retry log count = %d, want %d", observedLogs.FilterMessage("cron watcher trigger retrying").Len(), workersservice.CronMaxRetries)
	}
	if observedLogs.FilterMessage("cron watcher trigger exhausted").Len() != 1 {
		t.Fatal("expected exhausted timeout log after bounded cron retries")
	}

	failure := workersservice.ClassifyCronTriggerFailure(err)
	if !failure.Retryable || failure.Family != workerexecution.WorkFailureFamilyRetryable || failure.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("cron timeout classification = %#v, want retryable timeout", failure)
	}
}

func TestCronExecutionTimeout_UsesCanonicalWorkstationLimit(t *testing.T) {
	ws := cronWorkstationConfigForTest("poll-for-work")
	ws.Limits = interfaces.WorkstationLimits{MaxExecutionTime: "75ms"}
	runtimeCfg := newCronLoadedRuntimeConfig(t, "", &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{ws},
	}, map[string]*interfaces.FactoryWorkstationConfig{ws.Name: &ws})
	svc := workersservice.New(workersservice.Config{})

	timeout, err := svc.CronExecutionTimeout(runtimeCfg, cronWorkstationConfigForTest("poll-for-work"))
	if err != nil {
		t.Fatalf("CronExecutionTimeout: %v", err)
	}
	if timeout != 75*time.Millisecond {
		t.Fatalf("timeout = %v, want %v", timeout, 75*time.Millisecond)
	}
}

func TestCronExecutionTimeout_ReturnsCanonicalLimitError(t *testing.T) {
	runtimeCfg := cronRuntimeWorkstationLookup{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"poll-for-work": {
				Name:   "poll-for-work",
				Limits: interfaces.WorkstationLimits{MaxExecutionTime: "not-a-duration"},
			},
		},
	}
	svc := workersservice.New(workersservice.Config{})

	_, err := svc.CronExecutionTimeout(runtimeCfg, cronWorkstationConfigForTest("poll-for-work"))
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), `cron workstation "poll-for-work": invalid workstation limits.maxExecutionTime "not-a-duration": time: invalid duration "not-a-duration"`; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestSubmitCronTick_RetryableFailureRetriesBeforeSuccess(t *testing.T) {
	retryErr := errors.New("temporary submission ingress failure")
	attempt := 0
	submitCalls := 0
	submitter := func(_ context.Context, _ work.WorkRequest) error {
		attempt++
		submitCalls++
		if attempt <= workersservice.CronMaxRetries {
			return retryErr
		}
		return nil
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := workersservice.New(workersservice.Config{Logger: zap.New(logCore)})

	if err := svc.SubmitCronTick(
		context.Background(),
		nil,
		"",
		submitter,
		cronWorkstationConfigForTest("poll-for-work"),
		time.Now(),
	); err != nil {
		t.Fatalf("cron tick should succeed after retryable failures: %v", err)
	}
	if submitCalls != workersservice.CronMaxRetries+1 {
		t.Fatalf("cron submit attempts = %d, want %d", submitCalls, workersservice.CronMaxRetries+1)
	}
	if observedLogs.FilterMessage("cron watcher trigger retrying").Len() != workersservice.CronMaxRetries {
		t.Fatalf("retry log count = %d, want %d", observedLogs.FilterMessage("cron watcher trigger retrying").Len(), workersservice.CronMaxRetries)
	}
	if observedLogs.FilterMessage("cron watcher trigger exhausted").Len() != 0 {
		t.Fatal("cron retry success should not log exhaustion")
	}
}

func TestStartCronWatchersForRuntime_DisablesInvalidSchedulesWithoutAffectingValidCronJobs(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	logCore, observedLogs := observer.New(zap.InfoLevel)
	observedRequests := make(chan work.WorkRequest, 8)
	validCron := interfaces.FactoryWorkstationConfig{
		Name: "valid-cron",
		Kind: workertaxonomy.WorkstationKindCron,
		Cron: &interfaces.CronConfig{
			Schedule:       "* * * * *",
			TriggerAtStart: true,
		},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}
	invalidCron := interfaces.FactoryWorkstationConfig{
		Name: "invalid-cron",
		Kind: workertaxonomy.WorkstationKindCron,
		Cron: &interfaces.CronConfig{
			Schedule:       "not-a-cron",
			TriggerAtStart: true,
		},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}
	runtimeCfg := newCronLoadedRuntimeConfig(
		t,
		"factory-alpha",
		&interfaces.FactoryConfig{
			WorkTypes:    []interfaces.WorkTypeConfig{{Name: "task"}},
			Workstations: []interfaces.FactoryWorkstationConfig{validCron, invalidCron},
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			validCron.Name:   &validCron,
			invalidCron.Name: &invalidCron,
		},
	)
	svc := workersservice.New(workersservice.Config{
		Logger: zap.New(logCore),
		Clock:  fakeClock,
	})

	runCtx, cancelRun := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	svc.StartCronWatchersForRuntime(
		runCtx,
		&sidecars,
		"factory-alpha",
		runtimeCfg.FactoryConfig(),
		runtimeCfg,
		func(_ context.Context, request work.WorkRequest) error {
			select {
			case observedRequests <- request:
			default:
				t.Fatalf("cron request channel overflow")
			}
			return nil
		},
	)
	t.Cleanup(func() {
		cancelRun()
		sidecars.Wait()
	})

	startupRequest := waitForCronWorkRequest(t, observedRequests, time.Second)
	assertCronWorkRequestForWorkstation(t, startupRequest, start, "valid-cron")

	waitForFakeClockWaiters(t, fakeClock, 1)
	assertNoCronWorkRequestQueued(t, observedRequests)
	fakeClock.Advance(time.Minute)
	scheduledRequest := waitForCronWorkRequest(t, observedRequests, time.Second)
	assertCronWorkRequestForWorkstation(t, scheduledRequest, start.Add(time.Minute), "valid-cron")
	assertNoCronWorkRequestQueued(t, observedRequests)

	cancelRun()
	sidecars.Wait()
	assertCronWatcherRegistrationLog(t, observedLogs, "valid-cron")
	assertCronWatcherDisabledLog(t, observedLogs, "invalid-cron")
	assertCronSchedulerStartedLog(t, observedLogs, 1)
	if observedLogs.FilterMessage("cron watcher trigger retrying").Len() != 0 {
		t.Fatalf("retry log count = %d, want 0", observedLogs.FilterMessage("cron watcher trigger retrying").Len())
	}
	if observedLogs.FilterMessage("cron watcher trigger exhausted").Len() != 0 {
		t.Fatalf("exhausted log count = %d, want 0", observedLogs.FilterMessage("cron watcher trigger exhausted").Len())
	}
	stopped := observedLogs.FilterMessage("cron scheduler stopped").All()
	if len(stopped) != 1 {
		t.Fatalf("cron scheduler stopped log count = %d, want 1", len(stopped))
	}
}

func waitForCronWorkRequest(t *testing.T, requests <-chan work.WorkRequest, timeout time.Duration) work.WorkRequest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(timeout):
		t.Fatal("timed out waiting for cron work request")
		return work.WorkRequest{}
	}
}

func assertNoCronWorkRequestQueued(t *testing.T, requests <-chan work.WorkRequest) {
	t.Helper()
	select {
	case request := <-requests:
		t.Fatalf("unexpected cron work request: %#v", request)
	default:
	}
}

func assertCronWorkRequestForWorkstation(t *testing.T, request work.WorkRequest, want time.Time, workstation string) {
	t.Helper()
	assertCronWorkRequestNominalAt(t, request, want)
	if got := request.Works[0].Tags[interfaces.TimeWorkTagKeyCronWorkstation]; got != workstation {
		t.Fatalf("cron workstation tag = %q, want %q", got, workstation)
	}
}

func assertCronWorkRequestNominalAt(t *testing.T, request work.WorkRequest, want time.Time) {
	t.Helper()
	got := request.Works[0].Tags[interfaces.TimeWorkTagKeyNominalAt]
	wantTag := want.Format(time.RFC3339Nano)
	if got != wantTag {
		t.Fatalf("cron nominal_at tag = %q, want %q", got, wantTag)
	}
}

func waitForFakeClockWaiters(t *testing.T, fakeClock *clockwork.FakeClock, waiters int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fakeClock.BlockUntilContext(ctx, waiters); err != nil {
		t.Fatalf("timed out waiting for %d fake-clock waiter(s): %v", waiters, err)
	}
}

func assertCronWatcherRegistrationLog(t *testing.T, observedLogs *observer.ObservedLogs, workstation string) {
	t.Helper()
	registered := observedLogs.FilterMessage("cron watcher registered").All()
	if len(registered) != 1 {
		t.Fatalf("registered cron watcher count = %d, want 1", len(registered))
	}
	if got := registered[0].ContextMap()["workstation"]; got != workstation {
		t.Fatalf("registered cron watcher workstation = %#v, want %s", got, workstation)
	}
}

func assertCronWatcherDisabledLog(t *testing.T, observedLogs *observer.ObservedLogs, workstation string) {
	t.Helper()
	disabled := observedLogs.FilterMessage("cron watcher disabled").All()
	if len(disabled) != 1 {
		t.Fatalf("disabled cron watcher count = %d, want 1", len(disabled))
	}
	if got := disabled[0].ContextMap()["workstation"]; got != workstation {
		t.Fatalf("disabled cron watcher workstation = %#v, want %s", got, workstation)
	}
}

func assertCronSchedulerStartedLog(t *testing.T, observedLogs *observer.ObservedLogs, jobs int64) {
	t.Helper()
	started := observedLogs.FilterMessage("cron scheduler started").All()
	if len(started) != 1 {
		t.Fatalf("cron scheduler started log count = %d, want 1", len(started))
	}
	if got := started[0].ContextMap()["jobs"]; got != jobs {
		t.Fatalf("cron scheduler started jobs = %#v, want %d", got, jobs)
	}
}
