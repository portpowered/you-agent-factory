package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func cronFactoryConfig(schedule string) map[string]any {
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":     "poll-for-work",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron":     map[string]string{"schedule": schedule, "expiryWindow": "500ms"},
				"outputs":  []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	}
}

func cronFactoryConfigWithTriggerAtStart(schedule string, triggerAtStart bool) map[string]any {
	cfg := cronFactoryConfig(schedule)
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["cron"] = map[string]any{
		"schedule":       schedule,
		"expiryWindow":   "500ms",
		"triggerAtStart": triggerAtStart,
	}
	return cfg
}

func cronLoadedFactoryConfigForServiceTest(t *testing.T, factoryDir string, triggerAtStart bool) *config.LoadedFactoryConfig {
	t.Helper()

	ws := interfaces.FactoryWorkstationConfig{
		Name: "poll-for-work",
		Kind: interfaces.WorkstationKindCron,
		Cron: &interfaces.CronConfig{
			Schedule:       "* * * * *",
			TriggerAtStart: triggerAtStart,
		},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}
	return newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{
			WorkTypes:    []interfaces.WorkTypeConfig{{Name: "task"}},
			Workstations: []interfaces.FactoryWorkstationConfig{ws},
		},
		nil,
		map[string]*interfaces.FactoryWorkstationConfig{ws.Name: &ws},
	)
}

func cronFactoryConfigWithOutputState(schedule, outputState string) map[string]any {
	cfg := cronFactoryConfig(schedule)
	workTypes := cfg["workTypes"].([]map[string]any)
	task := workTypes[0]
	task["states"] = []map[string]string{
		{"name": "init", "type": "INITIAL"},
		{"name": "ready", "type": "PROCESSING"},
		{"name": "complete", "type": "TERMINAL"},
		{"name": "failed", "type": "FAILED"},
	}
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["outputs"] = []map[string]string{{"workType": "task", "state": outputState}}
	return cfg
}

func requiredInputCronFactoryConfigWithExpiry(schedule, expiryWindow string) map[string]any {
	cron := map[string]string{"schedule": schedule}
	if expiryWindow != "" {
		cron["expiryWindow"] = expiryWindow
	}
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "signal",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":     "poll-with-input",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron":     cron,
				"inputs":   []map[string]string{{"workType": "signal", "state": "init"}},
				"outputs":  []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	}
}

func TestFactoryService_ServiceModeCronScheduleConfigStartsAndStopsService(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfig("* * * * *"))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default cron runtime")
	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode factory service to stop")
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_SkipsNonCronAndTriggersOnlyCronWorkstations(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	logCore, observedLogs := observer.New(zap.InfoLevel)
	currentFactory := &aggregateSnapshotFactory{}
	replacementFactory := &aggregateSnapshotFactory{}
	validCron := interfaces.FactoryWorkstationConfig{
		Name: "valid-cron",
		Kind: interfaces.WorkstationKindCron,
		Cron: &interfaces.CronConfig{
			Schedule:       "* * * * *",
			TriggerAtStart: true,
		},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}
	manual := interfaces.FactoryWorkstationConfig{
		Name: "manual-step",
		Kind: interfaces.WorkstationKindStandard,
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "complete",
		}},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		"factory-alpha",
		&interfaces.FactoryConfig{
			WorkTypes:    []interfaces.WorkTypeConfig{{Name: "task"}},
			Workstations: []interfaces.FactoryWorkstationConfig{manual, validCron},
		},
		nil,
		map[string]*interfaces.FactoryWorkstationConfig{
			manual.Name:    &manual,
			validCron.Name: &validCron,
		},
	)
	observedRequests := make(chan interfaces.WorkRequest, 8)
	replacementFactory.submitFunc = func(_ context.Context, request interfaces.WorkRequest) error {
		select {
		case observedRequests <- request:
		default:
			t.Fatalf("cron request channel overflow")
		}
		return nil
	}
	svc := &FactoryService{
		cfg:     &FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService},
		factory: currentFactory,
		logger:  zap.New(logCore),
		clock:   fakeClock,
	}
	handle := &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			factory:    replacementFactory,
			runtimeCfg: runtimeCfg,
		},
	}
	sidecarCtx, cancelSidecars := context.WithCancel(context.Background())
	defer cancelSidecars()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	startupRequest := waitForCronWorkRequest(t, observedRequests, time.Second)
	assertCronWorkRequestNominalAt(t, startupRequest, start)
	if got := startupRequest.Works[0].Tags[cronWorkstationTag]; got != "valid-cron" {
		t.Fatalf("startup cron workstation tag = %q, want valid-cron", got)
	}

	waitForFakeClockWaiters(t, fakeClock, 1)
	assertNoCronWorkRequestQueued(t, observedRequests)
	fakeClock.Advance(time.Minute)
	scheduledRequest := waitForCronWorkRequest(t, observedRequests, time.Second)
	assertCronWorkRequestNominalAt(t, scheduledRequest, start.Add(time.Minute))
	if got := scheduledRequest.Works[0].Tags[cronWorkstationTag]; got != "valid-cron" {
		t.Fatalf("scheduled cron workstation tag = %q, want valid-cron", got)
	}
	assertNoCronWorkRequestQueued(t, observedRequests)

	if currentFactory.submitCalls != 0 {
		t.Fatalf("current runtime submit calls = %d, want 0", currentFactory.submitCalls)
	}
	if replacementFactory.submitCalls != 2 {
		t.Fatalf("replacement runtime submit calls = %d, want 2", replacementFactory.submitCalls)
	}
	assertCronWatcherRegistrationLog(t, observedLogs, "valid-cron")
	assertCronSchedulerStartedLog(t, observedLogs, 1)
}

func TestFactoryService_StartCronWatchersForRuntime_DisablesInvalidSchedulesWithoutAffectingValidCronJobs(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	logCore, observedLogs := observer.New(zap.InfoLevel)
	observedRequests := make(chan interfaces.WorkRequest, 8)
	validCron := interfaces.FactoryWorkstationConfig{
		Name: "valid-cron",
		Kind: interfaces.WorkstationKindCron,
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
		Kind: interfaces.WorkstationKindCron,
		Cron: &interfaces.CronConfig{
			Schedule:       "not-a-cron",
			TriggerAtStart: true,
		},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		"factory-alpha",
		&interfaces.FactoryConfig{
			WorkTypes:    []interfaces.WorkTypeConfig{{Name: "task"}},
			Workstations: []interfaces.FactoryWorkstationConfig{validCron, invalidCron},
		},
		nil,
		map[string]*interfaces.FactoryWorkstationConfig{
			validCron.Name:   &validCron,
			invalidCron.Name: &invalidCron,
		},
	)
	svc := &FactoryService{
		cfg:    &FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService},
		logger: zap.New(logCore),
		clock:  fakeClock,
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	svc.startCronWatchersForRuntime(
		runCtx,
		&sidecars,
		"factory-alpha",
		runtimeCfg.FactoryConfig(),
		runtimeCfg,
		func(_ context.Context, request interfaces.WorkRequest) error {
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

func TestFactoryService_ServiceModeCronSchedulerUsesFakeClockAndStopsOnCancel(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfigWithTriggerAtStart("* * * * *", false))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 8)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(nonBlockingSubmissionRecorder(observedSubmissions)),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	waitForFakeClockWaiters(t, fakeClock, 1)
	assertNoCronSubmissionQueued(t, observedSubmissions)

	fakeClock.Advance(time.Minute)
	record := waitForCronSubmission(t, observedSubmissions, time.Second)
	wantNominalAt := start.Add(time.Minute).Format(time.RFC3339Nano)
	if record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt] != wantNominalAt {
		cancelRun()
		t.Fatalf("cron nominal_at tag = %q, want %q", record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt], wantNominalAt)
	}
	if record.Request.Tags[cronWorkstationTag] != "poll-for-work" {
		cancelRun()
		t.Fatalf("cron workstation tag = %q, want poll-for-work", record.Request.Tags[cronWorkstationTag])
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode cron scheduler to stop")
	}

	fakeClock.Advance(time.Minute)
	assertNoCronSubmissionQueued(t, observedSubmissions)
}

func TestFactoryService_ServiceModeCronTriggerAtStartSubmitsOnceAndKeepsSchedule(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfigWithTriggerAtStart("* * * * *", true))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 8)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(nonBlockingSubmissionRecorder(observedSubmissions)),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	startupRecord := waitForCronSubmission(t, observedSubmissions, time.Second)
	assertCronSubmissionNominalAt(t, startupRecord, start)
	waitForCompletedDispatchConsumingWorkID(t, svc, startupRecord.Request.WorkID, time.Second)

	waitForFakeClockWaiters(t, fakeClock, 1)
	assertNoCronSubmissionQueued(t, observedSubmissions)
	fakeClock.Advance(time.Minute)
	scheduledRecord := waitForCronSubmission(t, observedSubmissions, time.Second)
	assertCronSubmissionNominalAt(t, scheduledRecord, start.Add(time.Minute))
	if scheduledRecord.Request.WorkID == startupRecord.Request.WorkID {
		cancelRun()
		t.Fatal("scheduled cron fire reused startup trigger work ID")
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode cron scheduler to stop")
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_BindsCronTriggerAtStartToReplacementRuntime(t *testing.T) {
	fakeClock := clockwork.NewFakeClock()
	currentFactory := &aggregateSnapshotFactory{}
	replacementFactory := &aggregateSnapshotFactory{}
	svc := &FactoryService{
		cfg:        &FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService},
		factory:    currentFactory,
		runtimeCfg: cronLoadedFactoryConfigForServiceTest(t, "alpha", true),
		logger:     zap.NewNop(),
		clock:      fakeClock,
	}
	handle := &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			factory:    replacementFactory,
			runtimeCfg: cronLoadedFactoryConfigForServiceTest(t, "beta", true),
		},
	}
	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	if currentFactory.submitCalls != 0 {
		t.Fatalf("current runtime submit calls = %d, want 0", currentFactory.submitCalls)
	}
	if replacementFactory.submitCalls != 1 {
		t.Fatalf("replacement runtime submit calls = %d, want 1", replacementFactory.submitCalls)
	}
	if got := replacementFactory.submissions[0].Works[0].WorkTypeID; got != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("replacement runtime submission work type = %q, want %q", got, interfaces.SystemTimeWorkTypeID)
	}
}

func TestFactoryService_CronTickSubmitsThroughEngineIngressAndAppearsInSnapshot(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 16)
	svc, runCtx, errCh, cancelRun := buildCronServiceForIngressTest(t, fakeClock, cronFactoryConfig("* * * * *"), observedSubmissions)
	defer cancelRun()

	ws := configuredCronWorkstationForServiceTest(t, svc, "poll-for-work")
	if err := svc.submitCronTick(runCtx, ws, start); err != nil {
		t.Fatalf("submitCronTick: %v", err)
	}

	record := waitForCronSubmission(t, observedSubmissions, time.Second)
	assertCronSubmissionRecord(t, record, "poll-for-work")
	assertCronDispatchAndOutput(t, svc, record.Request.WorkID, "task:init")
	stopServiceModeRun(t, cancelRun, errCh)
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

func buildCronServiceForIngressTest(
	t *testing.T,
	fakeClock *clockwork.FakeClock,
	cfg map[string]any,
	observedSubmissions chan interfaces.FactorySubmissionRecord,
) (*FactoryService, context.Context, <-chan error, context.CancelFunc) {
	t.Helper()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, cfg)
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(nonBlockingSubmissionRecorder(observedSubmissions)),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		handle := svc.currentLiveRuntime()
		if handle != nil {
			startCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			err := svc.waitForLiveRuntimeStart(startCtx, handle)
			cancel()
			if err != nil {
				t.Fatalf("wait for cron service startup: %v", err)
			}
			return svc, runCtx, errCh, cancelRun
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for cron service runtime handle")
	return svc, runCtx, errCh, cancelRun
}

func assertCronSubmissionRecord(t *testing.T, record interfaces.FactorySubmissionRecord, workstation string) {
	t.Helper()
	if record.Source != "external-submit" {
		t.Fatalf("cron submission source = %q, want external-submit", record.Source)
	}
	if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron submission work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if record.Request.TargetState != interfaces.SystemTimePendingState {
		t.Fatalf("cron submission target state = %q, want %q", record.Request.TargetState, interfaces.SystemTimePendingState)
	}
	if record.Request.Tags[cronSourceTag] != "cron" {
		t.Fatalf("cron submission source tag = %q, want cron", record.Request.Tags[cronSourceTag])
	}
	if record.Request.Tags[cronWorkstationTag] != workstation {
		t.Fatalf("cron submission workstation tag = %q, want %q", record.Request.Tags[cronWorkstationTag], workstation)
	}
}

func assertCronDispatchAndOutput(t *testing.T, svc *FactoryService, workID, outputPlace string) {
	t.Helper()
	dispatch := waitForCompletedDispatchConsumingWorkID(t, svc, workID, time.Second)
	matched := consumedTokenWithWorkID(dispatch.ConsumedTokens, workID)
	if matched == nil {
		t.Fatalf("completed cron dispatch did not retain consumed time token %q: %#v", workID, dispatch.ConsumedTokens)
	}
	if matched.Color.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron token work type = %q, want %q", matched.Color.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if matched.Color.TraceID == "" {
		t.Fatal("expected cron token to receive a trace ID")
	}
	if matched.Color.Name != cronSubmissionNamePref+"poll-for-work" {
		t.Fatalf("cron token name = %q, want %q", matched.Color.Name, cronSubmissionNamePref+"poll-for-work")
	}
	if matched.Color.Tags[cronSourceTag] != "cron" {
		t.Fatalf("cron token source tag = %q, want cron", matched.Color.Tags[cronSourceTag])
	}

	var payload map[string]string
	if err := json.Unmarshal(matched.Color.Payload, &payload); err != nil {
		t.Fatalf("cron token payload is not JSON: %v\npayload=%s", err, matched.Color.Payload)
	}
	if payload["cron_workstation"] != "poll-for-work" {
		t.Fatalf("cron payload workstation = %q, want poll-for-work", payload["cron_workstation"])
	}
	for _, key := range []string{"nominal_at", "due_at", "expires_at", "jitter", "source"} {
		if payload[key] == "" {
			t.Fatalf("expected cron payload to include %s, got %#v", key, payload)
		}
	}
	if tags := matched.Color.Tags; tags[interfaces.TimeWorkTagKeyNominalAt] == "" || tags[interfaces.TimeWorkTagKeyDueAt] == "" || tags[interfaces.TimeWorkTagKeyExpiresAt] == "" {
		t.Fatalf("expected cron timing tags, got %#v", tags)
	}

	output := waitForTokenInPlaceByParent(t, svc, outputPlace, workID, time.Second)
	if output.Color.WorkTypeID != "task" {
		t.Fatalf("cron worker-backed output work type = %q, want task", output.Color.WorkTypeID)
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

func waitForCronSubmission(t *testing.T, submissions <-chan interfaces.FactorySubmissionRecord, timeout time.Duration) interfaces.FactorySubmissionRecord {
	t.Helper()
	select {
	case record := <-submissions:
		if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
			t.Fatalf("cron submission work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
		}
		return record
	case <-time.After(timeout):
		t.Fatal("timed out waiting for cron submission")
	}
	return interfaces.FactorySubmissionRecord{}
}

func assertCronSubmissionNominalAt(t *testing.T, record interfaces.FactorySubmissionRecord, want time.Time) {
	assertCronSubmissionNominalAtForWorkstation(t, record, want, "poll-for-work")
}

func assertCronSubmissionNominalAtForWorkstation(t *testing.T, record interfaces.FactorySubmissionRecord, want time.Time, workstation string) {
	t.Helper()
	got := record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt]
	if got != want.Format(time.RFC3339Nano) {
		t.Fatalf("cron nominal_at tag = %q, want %q", got, want.Format(time.RFC3339Nano))
	}
	if record.Request.Tags[cronWorkstationTag] != workstation {
		t.Fatalf("cron workstation tag = %q, want %s", record.Request.Tags[cronWorkstationTag], workstation)
	}
}

func assertNoCronSubmissionQueued(t *testing.T, submissions <-chan interfaces.FactorySubmissionRecord) {
	t.Helper()
	select {
	case record := <-submissions:
		t.Fatalf("cron submission observed unexpectedly: %#v", record)
	default:
	}
}

func waitForCronWorkRequest(t *testing.T, requests <-chan interfaces.WorkRequest, timeout time.Duration) interfaces.WorkRequest {
	t.Helper()
	select {
	case request := <-requests:
		if len(request.Works) != 1 || request.Works[0].WorkTypeID != interfaces.SystemTimeWorkTypeID {
			t.Fatalf("cron work request works = %#v, want one internal time work item", request.Works)
		}
		return request
	case <-time.After(timeout):
		t.Fatal("timed out waiting for cron work request")
	}
	return interfaces.WorkRequest{}
}

func assertCronWorkRequestNominalAt(t *testing.T, request interfaces.WorkRequest, want time.Time) {
	t.Helper()
	if got := request.Works[0].Tags[interfaces.TimeWorkTagKeyNominalAt]; got != want.Format(time.RFC3339Nano) {
		t.Fatalf("cron nominal_at tag = %q, want %q", got, want.Format(time.RFC3339Nano))
	}
}

func assertCronWorkRequestForWorkstation(t *testing.T, request interfaces.WorkRequest, want time.Time, workstation string) {
	t.Helper()
	if request.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("cron work request type = %q, want %q", request.Type, interfaces.WorkRequestTypeFactoryRequestBatch)
	}
	assertCronWorkRequestNominalAt(t, request, want)
	if got := request.Works[0].Tags[cronWorkstationTag]; got != workstation {
		t.Fatalf("cron workstation tag = %q, want %q", got, workstation)
	}
	if got := request.Works[0].Tags[cronSourceTag]; got != "cron" {
		t.Fatalf("cron source tag = %q, want cron", got)
	}
}

func assertNoCronWorkRequestQueued(t *testing.T, requests <-chan interfaces.WorkRequest) {
	t.Helper()
	select {
	case request := <-requests:
		t.Fatalf("cron work request observed unexpectedly: %#v", request)
	default:
	}
}

func matchedTokenSnapshotTokensInPlace(t *testing.T, svc *FactoryService, placeID string) []interfaces.Token {
	t.Helper()
	snap, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	return snap.Marking.TokensInPlace(placeID)
}

func configuredCronWorkstationForServiceTest(t *testing.T, svc *FactoryService, name string) interfaces.FactoryWorkstationConfig {
	t.Helper()
	if svc == nil || svc.runtimeCfg == nil {
		t.Fatal("expected loaded service runtime config")
	}
	ws, ok := svc.runtimeCfg.Workstation(name)
	if !ok {
		t.Fatalf("expected cron workstation config %q", name)
	}
	return *ws
}

func waitForCompletedDispatchConsumingWorkID(t *testing.T, svc *FactoryService, workID string, timeout time.Duration) interfaces.CompletedDispatch {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot dispatch history: %v", err)
		}
		for _, dispatch := range snap.DispatchHistory {
			if consumedTokenWithWorkID(dispatch.ConsumedTokens, workID) != nil {
				return dispatch
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for completed dispatch consuming work %q", workID)
	return interfaces.CompletedDispatch{}
}

func consumedTokenWithWorkID(tokens []interfaces.Token, workID string) *interfaces.Token {
	for i := range tokens {
		if tokens[i].Color.WorkID == workID {
			return &tokens[i]
		}
	}
	return nil
}

func nonBlockingSubmissionRecorder(records chan<- interfaces.FactorySubmissionRecord) func(interfaces.FactorySubmissionRecord) {
	return func(record interfaces.FactorySubmissionRecord) {
		select {
		case records <- record:
		default:
		}
	}
}

func waitForTokenInPlaceByParent(t *testing.T, svc *FactoryService, placeID string, parentID string, timeout time.Duration) interfaces.Token {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot output token: %v", err)
		}
		for _, token := range snap.Marking.TokensInPlace(placeID) {
			if token.Color.ParentID == parentID {
				return token
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for token in %s with parent %q", placeID, parentID)
	return interfaces.Token{}
}

func waitForTokenInPlace(t *testing.T, svc *FactoryService, placeID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if tokens := matchedTokenSnapshotTokensInPlace(t, svc, placeID); len(tokens) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for any token in %s", placeID)
}

func TestFactoryService_CronTickTargetsInternalTimePlaceDespiteConfiguredOutputState(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 16)
	svc, runCtx, errCh, cancelRun := buildCronServiceForIngressTest(t, fakeClock, cronFactoryConfigWithOutputState("* * * * *", "ready"), observedSubmissions)
	defer cancelRun()

	ws := configuredCronWorkstationForServiceTest(t, svc, "poll-for-work")
	if err := svc.submitCronTick(runCtx, ws, start); err != nil {
		t.Fatalf("submitCronTick: %v", err)
	}

	record := waitForCronSubmission(t, observedSubmissions, time.Second)
	if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron submission work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if record.Request.TargetState != interfaces.SystemTimePendingState {
		t.Fatalf("cron submission target state = %q, want %q", record.Request.TargetState, interfaces.SystemTimePendingState)
	}
	assertCronDispatchAndOutput(t, svc, record.Request.WorkID, "task:ready")
	if tokens := matchedTokenSnapshotTokensInPlace(t, svc, "task:init"); len(tokens) != 0 {
		t.Fatalf("cron created customer token in initial state despite configured output state: %#v", tokens)
	}
	stopServiceModeRun(t, cancelRun, errCh)
}
