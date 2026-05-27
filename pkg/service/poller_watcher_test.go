package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const (
	canonicalScriptPollerWorkstationName = "linear-ingress"
	canonicalScriptPollerWorkerName      = "poller-script"
	canonicalScriptPollerCommand         = "factory/scripts/poller.sh"
)

func TestRunScriptPoller_SubmitsCanonicalWorkRequestStdoutToFactoryService(t *testing.T) {
	factoryDir := t.TempDir()
	workRequestJSON := []byte(`{
		"requestId":"linear-issue-batch-1",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-123","workTypeName":"task","payload":{"id":"ISSUE-123"}}]
	}`)
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{Stdout: workRequestJSON}}},
	}
	submitted := &aggregateSnapshotFactory{}
	svc := &FactoryService{
		cfg:    &FactoryServiceConfig{CommandRunnerOverride: runner},
		logger: zap.NewNop(),
	}
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{Workers: []interfaces.WorkerConfig{{Name: canonicalScriptPollerWorkerName}}, Workstations: []interfaces.FactoryWorkstationConfig{poller}},
		map[string]*interfaces.WorkerConfig{canonicalScriptPollerWorkerName: worker},
		map[string]*interfaces.FactoryWorkstationConfig{poller.Name: &poller},
	)

	err := svc.runScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		submitWorkRequestWithFactory(submitted),
	)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("runScriptPoller error = %v, want unexpected exit after successful submit", err)
	}
	if submitted.submitCalls != 1 {
		t.Fatalf("submit calls = %d, want 1", submitted.submitCalls)
	}
	if len(submitted.submissions) != 1 {
		t.Fatalf("submitted requests = %d, want 1", len(submitted.submissions))
	}
	if submitted.submissions[0].RequestID != "linear-issue-batch-1" {
		t.Fatalf("submitted request ID = %q, want linear-issue-batch-1", submitted.submissions[0].RequestID)
	}
	if submitted.submissions[0].Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("submitted request type = %q, want FACTORY_REQUEST_BATCH", submitted.submissions[0].Type)
	}
}

func TestRunScriptPoller_SubmitsSubmitStyleRecordsStdoutToFactoryService(t *testing.T) {
	factoryDir := t.TempDir()
	envelopeJSON := []byte(`{
		"submissions":[
			{
				"requestId":"linear-issue-batch-2",
				"workId":"linear-issue-124",
				"name":"issue-124",
				"workTypeName":"task",
				"traceId":"trace-124"
			}
		]
	}`)
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{Stdout: envelopeJSON}}},
	}
	submitted := &aggregateSnapshotFactory{}
	svc := &FactoryService{
		cfg:    &FactoryServiceConfig{CommandRunnerOverride: runner},
		logger: zap.NewNop(),
	}
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{Workers: []interfaces.WorkerConfig{{Name: canonicalScriptPollerWorkerName}}, Workstations: []interfaces.FactoryWorkstationConfig{poller}},
		map[string]*interfaces.WorkerConfig{canonicalScriptPollerWorkerName: worker},
		map[string]*interfaces.FactoryWorkstationConfig{poller.Name: &poller},
	)

	err := svc.runScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		submitWorkRequestWithFactory(submitted),
	)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("runScriptPoller error = %v, want unexpected exit after successful submit", err)
	}
	if len(submitted.submissions) != 1 {
		t.Fatalf("submitted requests = %d, want 1", len(submitted.submissions))
	}
	workRequest := submitted.submissions[0]
	if workRequest.RequestID != "linear-issue-batch-2" {
		t.Fatalf("submitted request ID = %q, want linear-issue-batch-2", workRequest.RequestID)
	}
	if workRequest.Works == nil || len(workRequest.Works) != 1 {
		t.Fatalf("submitted works = %#v, want one canonical work item", workRequest.Works)
	}
	if workRequest.Works[0].WorkID != "linear-issue-124" {
		t.Fatalf("submitted work ID = %q, want linear-issue-124", workRequest.Works[0].WorkID)
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_StartsOnlyScriptPollersAndRestartsUnexpectedExit(t *testing.T) {
	start := time.Date(2026, time.May, 22, 9, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	factoryDir := t.TempDir()
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{
			{result: workers.CommandResult{}},
			{waitForCancel: true},
		},
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := &FactoryService{
		cfg:    &FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService, CommandRunnerOverride: runner},
		logger: zap.New(logCore),
		clock:  fakeClock,
	}
	poller := newCanonicalScriptPollerWorkstation()
	standard := interfaces.FactoryWorkstationConfig{
		Name:           "processor",
		Kind:           interfaces.WorkstationKindStandard,
		WorkerTypeName: canonicalScriptPollerWorkerName,
	}
	runtimeCfg := newScriptPollerLoadedRuntimeConfigForServiceTest(
		t,
		factoryDir,
		scriptPollerRuntimeConfigOptions{
			poller:       poller,
			pollerWorker: newCanonicalScriptPollerWorker("--mode", "watch"),
			additionalWorkers: []*interfaces.WorkerConfig{
				{
					Name:    "non-poller-script",
					Type:    interfaces.WorkerTypeScript,
					Command: "factory/scripts/processor.sh",
				},
			},
			additionalWorkstations: []interfaces.FactoryWorkstationConfig{standard},
		},
	)
	handle := &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			factory:    &aggregateSnapshotFactory{},
			runtimeCfg: runtimeCfg,
		},
	}
	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	waitForPollerRunnerCalls(t, runner, 1, time.Second)
	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(pollerRestartBackoffMin)
	waitForPollerRunnerCalls(t, runner, 2, time.Second)

	reqs := runner.requests()
	if len(reqs) < 2 {
		t.Fatalf("poller runner requests = %d, want at least 2", len(reqs))
	}
	first := reqs[0]
	if first.WorkstationName != "linear-ingress" {
		t.Fatalf("poller workstation name = %q, want linear-ingress", first.WorkstationName)
	}
	if first.WorkerType != "poller-script" {
		t.Fatalf("poller worker type = %q, want poller-script", first.WorkerType)
	}
	if first.Command != filepath.Join(factoryDir, "scripts", "poller.sh") {
		t.Fatalf("poller command = %q, want resolved factory script path", first.Command)
	}
	if strings.Join(first.Args, " ") != "--mode watch" {
		t.Fatalf("poller args = %#v, want [--mode watch]", first.Args)
	}
	if observedLogs.FilterMessage("script poller restarting").Len() == 0 {
		t.Fatal("expected poller restart log after unexpected exit")
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_BatchModeDoesNotStartScriptPollers(t *testing.T) {
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{waitForCancel: true}},
	}
	svc := &FactoryService{
		cfg:    &FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeBatch, CommandRunnerOverride: runner},
		logger: zap.NewNop(),
	}
	poller := newCanonicalScriptPollerWorkstation()
	runtimeCfg := newScriptPollerLoadedRuntimeConfigForServiceTest(
		t,
		t.TempDir(),
		scriptPollerRuntimeConfigOptions{
			poller: poller,
		},
	)
	handle := &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			factory:    &aggregateSnapshotFactory{},
			runtimeCfg: runtimeCfg,
		},
	}

	if err := svc.startLiveRuntimeSidecars(context.Background(), handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	time.Sleep(50 * time.Millisecond)
	if runner.callCount() != 0 {
		t.Fatalf("poller runner calls = %d, want 0 in batch mode", runner.callCount())
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_RestartsScriptPollerOnMalformedOutput(t *testing.T) {
	fakeClock := clockwork.NewFakeClock()
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{
			{result: workers.CommandResult{Stdout: []byte("not-json\n")}},
			{waitForCancel: true},
		},
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := &FactoryService{
		cfg:    &FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService, CommandRunnerOverride: runner},
		logger: zap.New(logCore),
		clock:  fakeClock,
	}
	poller := newCanonicalScriptPollerWorkstation()
	runtimeCfg := newScriptPollerLoadedRuntimeConfigForServiceTest(
		t,
		t.TempDir(),
		scriptPollerRuntimeConfigOptions{
			poller: poller,
		},
	)
	handle := &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			factory:    &aggregateSnapshotFactory{},
			runtimeCfg: runtimeCfg,
		},
	}
	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	waitForPollerRunnerCalls(t, runner, 1, time.Second)
	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(pollerRestartBackoffMin)
	waitForPollerRunnerCalls(t, runner, 2, time.Second)

	if observedLogs.FilterMessage("script poller restarting").Len() == 0 {
		t.Fatal("expected restart log for malformed poller output")
	}
	entry := observedLogs.FilterMessage("script poller restarting").All()[0]
	if fieldString(entry.ContextMap()["error"]) == "" || !strings.Contains(fieldString(entry.ContextMap()["error"]), "malformed stdout") {
		t.Fatalf("restart error = %#v, want malformed stdout context", entry.ContextMap()["error"])
	}
}

func TestFactoryService_StopLiveRuntimeSidecars_StopsScriptPollerAndLogsLifecycle(t *testing.T) {
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{waitForCancel: true}},
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := &FactoryService{
		cfg:    &FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService, CommandRunnerOverride: runner},
		logger: zap.New(logCore),
	}
	poller := newCanonicalScriptPollerWorkstation()
	runtimeCfg := newScriptPollerLoadedRuntimeConfigForServiceTest(
		t,
		t.TempDir(),
		scriptPollerRuntimeConfigOptions{
			poller: poller,
		},
	)
	handle := &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			factory:    &aggregateSnapshotFactory{},
			runtimeCfg: runtimeCfg,
		},
	}

	if err := svc.startLiveRuntimeSidecars(context.Background(), handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}

	waitForPollerRunnerCalls(t, runner, 1, time.Second)
	svc.stopLiveRuntimeSidecars(handle)

	if observedLogs.FilterMessage("script poller started").Len() != 1 {
		t.Fatalf("script poller started log count = %d, want 1", observedLogs.FilterMessage("script poller started").Len())
	}
	stopped := observedLogs.FilterMessage("script poller stopped").All()
	if len(stopped) != 1 {
		t.Fatalf("script poller stopped log count = %d, want 1", len(stopped))
	}
	if got := fieldString(stopped[0].ContextMap()["reason"]); got != "context canceled" {
		t.Fatalf("script poller stop reason = %q, want context canceled", got)
	}
	if runner.callCount() != 1 {
		t.Fatalf("script poller runner calls after stop = %d, want 1", runner.callCount())
	}
}

func TestFactoryService_StopLiveRuntimeSidecars_StopsPriorScriptPollerBeforeReplacementStart(t *testing.T) {
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{
			{waitForCancel: true},
			{waitForCancel: true},
		},
	}
	svc := &FactoryService{
		cfg:    &FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService, CommandRunnerOverride: runner},
		logger: zap.NewNop(),
	}
	oldPoller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress-old",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}
	newPoller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress-new",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}
	oldHandle := newScriptPollerRuntimeHandleForWorkstation(t, oldPoller, &aggregateSnapshotFactory{})
	newHandle := newScriptPollerRuntimeHandleForWorkstation(t, newPoller, &aggregateSnapshotFactory{})

	if err := svc.startLiveRuntimeSidecars(context.Background(), oldHandle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars(old): %v", err)
	}
	waitForPollerRunnerCalls(t, runner, 1, time.Second)

	svc.stopLiveRuntimeSidecars(oldHandle)

	if err := svc.startLiveRuntimeSidecars(context.Background(), newHandle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars(new): %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(newHandle)
	waitForPollerRunnerCalls(t, runner, 2, time.Second)

	reqs := runner.requests()
	if len(reqs) != 2 {
		t.Fatalf("poller runner requests = %d, want 2", len(reqs))
	}
	if reqs[0].WorkstationName != oldPoller.Name {
		t.Fatalf("first poller workstation = %q, want %q", reqs[0].WorkstationName, oldPoller.Name)
	}
	if reqs[1].WorkstationName != newPoller.Name {
		t.Fatalf("replacement poller workstation = %q, want %q", reqs[1].WorkstationName, newPoller.Name)
	}
}

func TestFactoryService_StopLiveRuntimeSidecars_WaitsForScriptPollerSubmitBeforeReplacementStart(t *testing.T) {
	workRequestJSON := []byte(`{
		"requestId":"linear-issue-batch-3",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-125","workTypeName":"task","payload":{"id":"ISSUE-125"}}]
	}`)
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{
			{result: workers.CommandResult{Stdout: workRequestJSON}},
			{waitForCancel: true},
		},
	}
	submitStarted := make(chan struct{})
	releaseSubmit := make(chan struct{})
	oldFactory := &aggregateSnapshotFactory{
		submitFunc: func(context.Context, interfaces.WorkRequest) error {
			close(submitStarted)
			<-releaseSubmit
			return nil
		},
	}
	newFactory := &aggregateSnapshotFactory{}
	svc := &FactoryService{
		cfg:    &FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService, CommandRunnerOverride: runner},
		logger: zap.NewNop(),
	}
	oldHandle := newScriptPollerRuntimeHandle(t, "linear-ingress-old", oldFactory)
	newHandle := newScriptPollerRuntimeHandle(t, "linear-ingress-new", newFactory)

	if err := svc.startLiveRuntimeSidecars(context.Background(), oldHandle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars(old): %v", err)
	}
	<-submitStarted

	stopped := make(chan struct{})
	go func() {
		svc.stopLiveRuntimeSidecars(oldHandle)
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("stopLiveRuntimeSidecars(old) completed before in-flight submit drained")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseSubmit)

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stopLiveRuntimeSidecars(old) to finish")
	}

	if err := svc.startLiveRuntimeSidecars(context.Background(), newHandle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars(new): %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(newHandle)
	waitForPollerRunnerCalls(t, runner, 2, time.Second)

	if oldFactory.submitCalls != 1 {
		t.Fatalf("old runtime submit calls = %d, want 1", oldFactory.submitCalls)
	}
	if newFactory.submitCalls != 0 {
		t.Fatalf("replacement runtime submit calls before replacement poller restart = %d, want 0", newFactory.submitCalls)
	}
	reqs := runner.requests()
	if len(reqs) != 2 {
		t.Fatalf("poller runner requests = %d, want 2", len(reqs))
	}
	if reqs[0].WorkstationName != "linear-ingress-old" {
		t.Fatalf("first poller workstation = %q, want %q", reqs[0].WorkstationName, "linear-ingress-old")
	}
	if reqs[1].WorkstationName != "linear-ingress-new" {
		t.Fatalf("replacement poller workstation = %q, want %q", reqs[1].WorkstationName, "linear-ingress-new")
	}
}

func newScriptPollerRuntimeHandle(t *testing.T, workstationName string, activeFactory *aggregateSnapshotFactory) *liveRuntimeHandle {
	t.Helper()

	return newScriptPollerRuntimeHandleForWorkstation(t, interfaces.FactoryWorkstationConfig{
		Name:           workstationName,
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}, activeFactory)
}

func newScriptPollerRuntimeHandleForWorkstation(
	t *testing.T,
	poller interfaces.FactoryWorkstationConfig,
	activeFactory *aggregateSnapshotFactory,
) *liveRuntimeHandle {
	t.Helper()

	return &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			factory: activeFactory,
			runtimeCfg: newScriptPollerLoadedRuntimeConfigForServiceTest(
				t,
				t.TempDir(),
				scriptPollerRuntimeConfigOptions{
					poller: poller,
				},
			),
		},
	}
}

func TestParseScriptPollerOutput_RejectsUnsupportedRawFactoryEvents(t *testing.T) {
	rawEventJSON, err := json.Marshal(map[string]any{
		"events": []map[string]any{{
			"type": "WORK_REQUEST",
		}},
	})
	if err != nil {
		t.Fatalf("marshal raw event payload: %v", err)
	}

	_, hasOutput, parseErr := parseScriptPollerOutput(rawEventJSON)
	if !hasOutput {
		t.Fatal("expected raw event payload to count as poller output")
	}
	if parseErr == nil || !strings.Contains(parseErr.Error(), "unsupported raw factory events") {
		t.Fatalf("parse error = %v, want unsupported raw factory events", parseErr)
	}
}

func newCanonicalScriptPollerWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           canonicalScriptPollerWorkstationName,
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: canonicalScriptPollerWorkerName,
	}
}

func newCanonicalScriptPollerWorker(args ...string) *interfaces.WorkerConfig {
	return &interfaces.WorkerConfig{
		Name:    canonicalScriptPollerWorkerName,
		Type:    interfaces.WorkerTypeScript,
		Command: canonicalScriptPollerCommand,
		Args:    args,
	}
}

type scriptPollerRuntimeConfigOptions struct {
	poller                 interfaces.FactoryWorkstationConfig
	pollerWorker           *interfaces.WorkerConfig
	additionalWorkers      []*interfaces.WorkerConfig
	additionalWorkstations []interfaces.FactoryWorkstationConfig
}

func newScriptPollerLoadedRuntimeConfigForServiceTest(
	t *testing.T,
	factoryDir string,
	options scriptPollerRuntimeConfigOptions,
) *config.LoadedFactoryConfig {
	t.Helper()

	poller := options.poller
	if poller.Name == "" {
		poller = newCanonicalScriptPollerWorkstation()
	}
	pollerWorker := options.pollerWorker
	if pollerWorker == nil {
		pollerWorker = newCanonicalScriptPollerWorker()
	}

	factoryCfg := &interfaces.FactoryConfig{
		Workers:      []interfaces.WorkerConfig{{Name: pollerWorker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{poller},
	}
	workerConfigs := map[string]*interfaces.WorkerConfig{
		pollerWorker.Name: pollerWorker,
	}
	workstationConfigs := map[string]*interfaces.FactoryWorkstationConfig{
		poller.Name: &poller,
	}

	for _, worker := range options.additionalWorkers {
		if worker == nil {
			continue
		}
		factoryCfg.Workers = append(factoryCfg.Workers, interfaces.WorkerConfig{Name: worker.Name})
		workerConfigs[worker.Name] = worker
	}
	for i := range options.additionalWorkstations {
		workstation := options.additionalWorkstations[i]
		factoryCfg.Workstations = append(factoryCfg.Workstations, workstation)
		workstationCopy := workstation
		workstationConfigs[workstation.Name] = &workstationCopy
	}

	return newLoadedFactoryConfigForServiceTest(t, factoryDir, factoryCfg, workerConfigs, workstationConfigs)
}

type pollerRunOutcome struct {
	result        workers.CommandResult
	err           error
	waitForCancel bool
}

type pollerSequenceCommandRunner struct {
	mu       sync.Mutex
	calls    int
	reqs     []workers.CommandRequest
	outcomes []pollerRunOutcome
}

func (r *pollerSequenceCommandRunner) Run(ctx context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	r.reqs = append(r.reqs, req)
	index := r.calls - 1
	var outcome pollerRunOutcome
	if index < len(r.outcomes) {
		outcome = r.outcomes[index]
	} else if len(r.outcomes) > 0 {
		outcome = r.outcomes[len(r.outcomes)-1]
	}
	r.mu.Unlock()

	if outcome.waitForCancel {
		<-ctx.Done()
		return outcome.result, ctx.Err()
	}
	return outcome.result, outcome.err
}

func (r *pollerSequenceCommandRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *pollerSequenceCommandRunner) requests() []workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]workers.CommandRequest, len(r.reqs))
	copy(out, r.reqs)
	return out
}

func waitForPollerRunnerCalls(t *testing.T, runner *pollerSequenceCommandRunner, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if runner.callCount() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d poller runner call(s); got %d", want, runner.callCount())
}

func fieldString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case error:
		return typed.Error()
	default:
		return ""
	}
}

func TestParseScriptPollerOutput_RejectsMalformedStdout(t *testing.T) {
	_, hasOutput, err := parseScriptPollerOutput([]byte("submitted work\n"))
	if !hasOutput {
		t.Fatal("expected non-empty stdout to count as poller output")
	}
	if err == nil {
		t.Fatal("expected malformed stdout error")
	}
	if !strings.Contains(err.Error(), "malformed stdout") {
		t.Fatalf("error = %v, want malformed stdout", err)
	}
}

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
