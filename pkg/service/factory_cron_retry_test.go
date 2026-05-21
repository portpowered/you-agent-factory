package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestFactoryService_RequiredInputCronKeepsTimeWorkPendingWhenInputMissing(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 16)
	svc, runCtx, errCh, cancelRun := buildCronServiceForIngressTest(
		t,
		fakeClock,
		requiredInputCronFactoryConfigWithExpiry("* * * * *", "40ms"),
		observedSubmissions,
	)
	defer cancelRun()

	ws := configuredCronWorkstationForServiceTest(t, svc, "poll-with-input")
	if err := svc.submitCronTick(runCtx, ws, start); err != nil {
		t.Fatalf("submitCronTick: %v", err)
	}

	firstRecord := waitForCronSubmission(t, observedSubmissions, time.Second)
	if firstRecord.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("required-input cron submission work type = %q, want %q", firstRecord.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if firstRecord.Request.Tags[cronWorkstationTag] != "poll-with-input" {
		t.Fatalf("required-input cron workstation tag = %q, want poll-with-input", firstRecord.Request.Tags[cronWorkstationTag])
	}

	pendingSnap := waitForPendingCronTimeToken(t, svc, firstRecord.Request.WorkID)
	if pendingSnap.InFlightCount != 0 || len(pendingSnap.Dispatches) != 0 {
		t.Fatalf("required-input cron dispatched while input was missing: inflight=%d dispatches=%#v", pendingSnap.InFlightCount, pendingSnap.Dispatches)
	}
	if tokens := pendingSnap.Marking.TokensInPlace("task:init"); len(tokens) != 0 {
		t.Fatalf("required-input cron created task output before input existed: %#v", tokens)
	}
	stopServiceModeRun(t, cancelRun, errCh)
}

func waitForPendingCronTimeToken(
	t *testing.T,
	svc *FactoryService,
	workID string,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot pending time work: %v", err)
		}
		for _, token := range snap.Marking.TokensInPlace(interfaces.SystemTimePendingPlaceID) {
			if token.Color.WorkID == workID {
				return snap
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for required-input cron time token in %s", interfaces.SystemTimePendingPlaceID)
	return nil
}

func TestFactoryService_CronTickTimeoutFailureIsClassifiedAndBounded(t *testing.T) {
	logCore, observedLogs := observer.New(zap.InfoLevel)
	mock := &aggregateSnapshotFactory{
		submitFunc: func(ctx context.Context, _ interfaces.WorkRequest) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	svc := &FactoryService{
		factory:    mock,
		logger:     zap.New(logCore),
		runtimeCfg: newLoadedFactoryConfigForServiceTest(t, "", &interfaces.FactoryConfig{Workstations: []interfaces.FactoryWorkstationConfig{{Name: "poll-for-work", Limits: interfaces.WorkstationLimits{MaxExecutionTime: "1ms"}}}}, nil, nil),
	}

	err := svc.submitCronTick(context.Background(), cronWorkstationConfigForTest("poll-for-work"), time.Now())
	if err == nil {
		t.Fatal("expected timed-out cron tick to fail after bounded retries")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cron tick error = %v, want deadline exceeded classification", err)
	}
	if mock.submitCalls != cronMaxRetries+1 {
		t.Fatalf("cron submit attempts = %d, want %d", mock.submitCalls, cronMaxRetries+1)
	}
	if len(mock.submissions) != cronMaxRetries+1 {
		t.Fatalf("recorded cron work requests = %d, want %d", len(mock.submissions), cronMaxRetries+1)
	}
	submitted := mock.submissions[len(mock.submissions)-1]
	if submitted.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("cron submitted request type = %q, want %q", submitted.Type, interfaces.WorkRequestTypeFactoryRequestBatch)
	}
	if len(submitted.Works) != 1 || submitted.Works[0].WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron submitted works = %#v, want one internal time work item", submitted.Works)
	}
	if observedLogs.FilterMessage("cron watcher trigger retrying").Len() != cronMaxRetries {
		t.Fatalf("retry log count = %d, want %d", observedLogs.FilterMessage("cron watcher trigger retrying").Len(), cronMaxRetries)
	}
	if observedLogs.FilterMessage("cron watcher trigger exhausted").Len() != 1 {
		t.Fatal("expected exhausted timeout log after bounded cron retries")
	}

	failure := classifyCronTriggerFailure(err)
	if !failure.retryable || failure.Family != interfaces.ProviderErrorFamilyRetryable || failure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("cron timeout classification = %#v, want retryable timeout", failure)
	}
}

func TestFactoryService_CronExecutionTimeoutUsesCanonicalWorkstationLimit(t *testing.T) {
	svc := &FactoryService{}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:   "poll-for-work",
			Limits: interfaces.WorkstationLimits{MaxExecutionTime: "75ms"},
		}},
	}, nil, nil)

	timeout, err := svc.cronExecutionTimeout(runtimeCfg, cronWorkstationConfigForTest("poll-for-work"))
	if err != nil {
		t.Fatalf("cronExecutionTimeout: %v", err)
	}
	if timeout != 75*time.Millisecond {
		t.Fatalf("timeout = %v, want %v", timeout, 75*time.Millisecond)
	}
}

func TestFactoryService_CronExecutionTimeoutReturnsCanonicalLimitError(t *testing.T) {
	svc := &FactoryService{}
	runtimeCfg := serviceTestRuntimeConfig{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"poll-for-work": {
				Name:   "poll-for-work",
				Limits: interfaces.WorkstationLimits{MaxExecutionTime: "not-a-duration"},
			},
		},
	}

	_, err := svc.cronExecutionTimeout(runtimeCfg, cronWorkstationConfigForTest("poll-for-work"))
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), `cron workstation "poll-for-work": invalid workstation limits.maxExecutionTime "not-a-duration": time: invalid duration "not-a-duration"`; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestFactoryService_CronTickRetryableFailureRetriesBeforeSuccess(t *testing.T) {
	retryErr := errors.New("temporary submission ingress failure")
	mock := &aggregateSnapshotFactory{}
	attempt := 0
	mock.submitFunc = func(_ context.Context, _ interfaces.WorkRequest) error {
		attempt++
		if attempt <= cronMaxRetries {
			return retryErr
		}
		return nil
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := &FactoryService{
		factory: mock,
		logger:  zap.New(logCore),
	}

	if err := svc.submitCronTick(context.Background(), cronWorkstationConfigForTest("poll-for-work"), time.Now()); err != nil {
		t.Fatalf("cron tick should succeed after retryable failures: %v", err)
	}
	if mock.submitCalls != cronMaxRetries+1 {
		t.Fatalf("cron submit attempts = %d, want %d", mock.submitCalls, cronMaxRetries+1)
	}
	if observedLogs.FilterMessage("cron watcher trigger retrying").Len() != cronMaxRetries {
		t.Fatalf("retry log count = %d, want %d", observedLogs.FilterMessage("cron watcher trigger retrying").Len(), cronMaxRetries)
	}
	if observedLogs.FilterMessage("cron watcher trigger exhausted").Len() != 0 {
		t.Fatal("cron retry success should not log exhaustion")
	}
}

func cronWorkstationConfigForTest(name string) interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name: name,
		Kind: interfaces.WorkstationKindCron,
		Cron: &interfaces.CronConfig{Schedule: "* * * * *"},
		Outputs: []interfaces.IOConfig{
			{WorkTypeName: "task", StateName: "init"},
		},
	}
}

func TestFactoryService_BatchModeDoesNotStartCronWatchers(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfig("* * * * *"))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 1)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(func(record interfaces.FactorySubmissionRecord) {
				observedSubmissions <- record
			}),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	select {
	case record := <-observedSubmissions:
		t.Fatalf("batch-mode cron watcher submitted unexpectedly: %#v", record)
	default:
	}
}
