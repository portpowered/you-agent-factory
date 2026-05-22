package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}
	worker := &interfaces.WorkerConfig{
		Name:    "poller-script",
		Type:    interfaces.WorkerTypeScript,
		Command: "factory/scripts/poller.sh",
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{Workers: []interfaces.WorkerConfig{{Name: "poller-script"}}, Workstations: []interfaces.FactoryWorkstationConfig{poller}},
		map[string]*interfaces.WorkerConfig{"poller-script": worker},
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
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}
	worker := &interfaces.WorkerConfig{
		Name:    "poller-script",
		Type:    interfaces.WorkerTypeScript,
		Command: "factory/scripts/poller.sh",
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{Workers: []interfaces.WorkerConfig{{Name: "poller-script"}}, Workstations: []interfaces.FactoryWorkstationConfig{poller}},
		map[string]*interfaces.WorkerConfig{"poller-script": worker},
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
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}
	standard := interfaces.FactoryWorkstationConfig{
		Name:           "processor",
		Kind:           interfaces.WorkstationKindStandard,
		WorkerTypeName: "poller-script",
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{
			Workers:      []interfaces.WorkerConfig{{Name: "poller-script"}, {Name: "non-poller-script"}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller, standard},
		},
		map[string]*interfaces.WorkerConfig{
			"poller-script": {
				Name:    "poller-script",
				Type:    interfaces.WorkerTypeScript,
				Command: "factory/scripts/poller.sh",
				Args:    []string{"--mode", "watch"},
			},
			"non-poller-script": {
				Name:    "non-poller-script",
				Type:    interfaces.WorkerTypeScript,
				Command: "factory/scripts/processor.sh",
			},
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			poller.Name:   &poller,
			standard.Name: &standard,
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
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		t.TempDir(),
		&interfaces.FactoryConfig{
			Workers:      []interfaces.WorkerConfig{{Name: "poller-script"}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		map[string]*interfaces.WorkerConfig{
			"poller-script": {
				Name:    "poller-script",
				Type:    interfaces.WorkerTypeScript,
				Command: "factory/scripts/poller.sh",
			},
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			poller.Name: &poller,
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
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		t.TempDir(),
		&interfaces.FactoryConfig{
			Workers:      []interfaces.WorkerConfig{{Name: "poller-script"}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		map[string]*interfaces.WorkerConfig{
			"poller-script": {
				Name:    "poller-script",
				Type:    interfaces.WorkerTypeScript,
				Command: "factory/scripts/poller.sh",
			},
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			poller.Name: &poller,
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
	poller := interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		t.TempDir(),
		&interfaces.FactoryConfig{
			Workers:      []interfaces.WorkerConfig{{Name: "poller-script"}},
			Workstations: []interfaces.FactoryWorkstationConfig{poller},
		},
		map[string]*interfaces.WorkerConfig{
			"poller-script": {
				Name:    "poller-script",
				Type:    interfaces.WorkerTypeScript,
				Command: "factory/scripts/poller.sh",
			},
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			poller.Name: &poller,
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
	oldHandle := &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			factory: &aggregateSnapshotFactory{},
			runtimeCfg: newLoadedFactoryConfigForServiceTest(
				t,
				t.TempDir(),
				&interfaces.FactoryConfig{
					Workers:      []interfaces.WorkerConfig{{Name: "poller-script"}},
					Workstations: []interfaces.FactoryWorkstationConfig{oldPoller},
				},
				map[string]*interfaces.WorkerConfig{
					"poller-script": {
						Name:    "poller-script",
						Type:    interfaces.WorkerTypeScript,
						Command: "factory/scripts/poller.sh",
					},
				},
				map[string]*interfaces.FactoryWorkstationConfig{oldPoller.Name: &oldPoller},
			),
		},
	}
	newHandle := &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			factory: &aggregateSnapshotFactory{},
			runtimeCfg: newLoadedFactoryConfigForServiceTest(
				t,
				t.TempDir(),
				&interfaces.FactoryConfig{
					Workers:      []interfaces.WorkerConfig{{Name: "poller-script"}},
					Workstations: []interfaces.FactoryWorkstationConfig{newPoller},
				},
				map[string]*interfaces.WorkerConfig{
					"poller-script": {
						Name:    "poller-script",
						Type:    interfaces.WorkerTypeScript,
						Command: "factory/scripts/poller.sh",
					},
				},
				map[string]*interfaces.FactoryWorkstationConfig{newPoller.Name: &newPoller},
			),
		},
	}

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

	poller := interfaces.FactoryWorkstationConfig{
		Name:           workstationName,
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}
	return &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			factory: activeFactory,
			runtimeCfg: newLoadedFactoryConfigForServiceTest(
				t,
				t.TempDir(),
				&interfaces.FactoryConfig{
					Workers:      []interfaces.WorkerConfig{{Name: "poller-script"}},
					Workstations: []interfaces.FactoryWorkstationConfig{poller},
				},
				map[string]*interfaces.WorkerConfig{
					"poller-script": {
						Name:    "poller-script",
						Type:    interfaces.WorkerTypeScript,
						Command: "factory/scripts/poller.sh",
					},
				},
				map[string]*interfaces.FactoryWorkstationConfig{poller.Name: &poller},
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
