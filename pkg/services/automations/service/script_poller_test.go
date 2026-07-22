package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	automationservice "github.com/portpowered/infinite-you/pkg/services/automations/service"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	canonicalScriptPollerWorkstationName = "linear-ingress"
	canonicalScriptPollerWorkerName      = "poller-script"
	canonicalScriptPollerCommand         = "factory/scripts/poller.sh"
)

type recordingSubmitter struct {
	mu             sync.Mutex
	calls          int
	submissions    []work.WorkRequest
	submitOverride func(context.Context, work.WorkRequest) error
}

func (r *recordingSubmitter) submit(ctx context.Context, request work.WorkRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.submissions = append(r.submissions, request)
	if r.submitOverride != nil {
		return r.submitOverride(ctx, request)
	}
	return nil
}

func (r *recordingSubmitter) snapshot() (int, []work.WorkRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, append([]work.WorkRequest(nil), r.submissions...)
}

func TestRunScriptPoller_SubmitsCanonicalWorkRequestStdout(t *testing.T) {
	factoryDir := t.TempDir()
	workRequestJSON := []byte(`{
		"requestId":"linear-issue-batch-1",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-123","workTypeName":"task","payload":{"id":"ISSUE-123"}}]
	}`)
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{Stdout: workRequestJSON}}},
	}
	submitted := &recordingSubmitter{}
	svc := newAutomationService(automationFixture{
		Logger:        zap.NewNop(),
		CommandRunner: runner,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(
		t,
		factoryDir,
		scriptPollerRuntimeConfigOptions{
			poller: poller,
		},
	)

	err := svc.RunScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		submitted.submit,
	)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("RunScriptPoller error = %v, want unexpected exit after successful submit", err)
	}
	if submitted.calls != 1 {
		t.Fatalf("submit calls = %d, want 1", submitted.calls)
	}
	if len(submitted.submissions) != 1 {
		t.Fatalf("submitted requests = %d, want 1", len(submitted.submissions))
	}
	if submitted.submissions[0].RequestID != "linear-issue-batch-1" {
		t.Fatalf("submitted request ID = %q, want linear-issue-batch-1", submitted.submissions[0].RequestID)
	}
	if submitted.submissions[0].Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("submitted request type = %q, want FACTORY_REQUEST_BATCH", submitted.submissions[0].Type)
	}
}

func TestRunScriptPoller_SubmitsSubmitStyleRecordsStdout(t *testing.T) {
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
	submitted := &recordingSubmitter{}
	svc := newAutomationService(automationFixture{
		Logger:        zap.NewNop(),
		CommandRunner: runner,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(
		t,
		factoryDir,
		scriptPollerRuntimeConfigOptions{
			poller: poller,
		},
	)

	err := svc.RunScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		submitted.submit,
	)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("RunScriptPoller error = %v, want unexpected exit after successful submit", err)
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

func TestScriptPollerCommandRequest_DefaultsEmptyWorkingDirectoryToRuntimeBaseDirectory(t *testing.T) {
	factoryDir := t.TempDir()
	runtimeBaseDir := t.TempDir()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(
		t,
		factoryDir,
		scriptPollerRuntimeConfigOptions{
			poller: newCanonicalScriptPollerWorkstation(),
		},
	)
	runtimeCfg.SetRuntimeBaseDir(runtimeBaseDir)

	req, err := automationservice.ScriptPollerCommandRequest(
		runtimeCfg,
		newCanonicalScriptPollerWorkstation(),
		newCanonicalScriptPollerWorker("--mode", "watch"),
		nil,
	)
	if err != nil {
		t.Fatalf("ScriptPollerCommandRequest: %v", err)
	}
	if req.WorkDir != runtimeBaseDir {
		t.Fatalf("poller workdir = %q, want %q", req.WorkDir, runtimeBaseDir)
	}
	if req.Command != filepath.Join(factoryDir, "scripts", "poller.sh") {
		t.Fatalf("poller command = %q, want resolved factory script path", req.Command)
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

	_, hasOutput, parseErr := automationservice.ParseScriptPollerOutput(rawEventJSON)
	if !hasOutput {
		t.Fatal("expected raw event payload to count as poller output")
	}
	if parseErr == nil || !strings.Contains(parseErr.Error(), "unsupported raw factory events") {
		t.Fatalf("parse error = %v, want unsupported raw factory events", parseErr)
	}
}

func TestParseScriptPollerOutput_RejectsMalformedStdout(t *testing.T) {
	_, hasOutput, err := automationservice.ParseScriptPollerOutput([]byte("submitted work\n"))
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

func TestRunScriptPoller_CommandFailureReturnsExecutionError(t *testing.T) {
	runErr := errors.New("shell command unavailable")
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{err: runErr}},
	}
	submitted := &recordingSubmitter{}
	svc := newAutomationService(automationFixture{
		Logger:        zap.NewNop(),
		CommandRunner: runner,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(
		t,
		t.TempDir(),
		scriptPollerRuntimeConfigOptions{poller: poller},
	)

	err := svc.RunScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		submitted.submit,
	)
	if err == nil || !strings.Contains(err.Error(), "execution failed") {
		t.Fatalf("RunScriptPoller error = %v, want execution failed", err)
	}
	if submitted.calls != 0 {
		t.Fatalf("submit calls = %d, want 0 on command failure", submitted.calls)
	}
}

func TestRunScriptPoller_NonZeroExitReturnsErrorWithoutSubmit(t *testing.T) {
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{ExitCode: 2}}},
	}
	submitted := &recordingSubmitter{}
	svc := newAutomationService(automationFixture{
		Logger:        zap.NewNop(),
		CommandRunner: runner,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(
		t,
		t.TempDir(),
		scriptPollerRuntimeConfigOptions{poller: poller},
	)

	err := svc.RunScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		submitted.submit,
	)
	if err == nil || !strings.Contains(err.Error(), "exited with code 2") {
		t.Fatalf("RunScriptPoller error = %v, want non-zero exit", err)
	}
	if submitted.calls != 0 {
		t.Fatalf("submit calls = %d, want 0 on non-zero exit", submitted.calls)
	}
}

func TestRunScriptPoller_SubmitFailureReturnsSubmitError(t *testing.T) {
	factoryDir := t.TempDir()
	workRequestJSON := []byte(`{
		"requestId":"linear-issue-batch-submit-fail",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-999","workTypeName":"task","payload":{"id":"ISSUE-999"}}]
	}`)
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{Stdout: workRequestJSON}}},
	}
	submitErr := errors.New("ingress unavailable")
	submitted := &recordingSubmitter{
		submitOverride: func(_ context.Context, _ work.WorkRequest) error {
			return submitErr
		},
	}
	svc := newAutomationService(automationFixture{
		Logger:        zap.NewNop(),
		CommandRunner: runner,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(
		t,
		factoryDir,
		scriptPollerRuntimeConfigOptions{poller: poller},
	)

	err := svc.RunScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		submitted.submit,
	)
	if err == nil || !strings.Contains(err.Error(), "submit failed") {
		t.Fatalf("RunScriptPoller error = %v, want submit failed", err)
	}
	if submitted.calls != 1 {
		t.Fatalf("submit calls = %d, want 1 before submit failure", submitted.calls)
	}
}

func TestStartScriptPoller_RestartsOnMalformedOutputWithBackoff(t *testing.T) {
	fakeClock := clockwork.NewFakeClock()
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{
			{result: workers.CommandResult{Stdout: []byte("not-json\n")}},
			{waitForCancel: true},
		},
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := newAutomationService(automationFixture{
		Logger:        zap.New(logCore),
		Clock:         fakeClock,
		CommandRunner: runner,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(
		t,
		t.TempDir(),
		scriptPollerRuntimeConfigOptions{poller: poller},
	)

	runCtx, cancelRun := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	svc.StartScriptPoller(
		runCtx,
		&sidecars,
		runtimeCfg,
		poller,
		worker,
		func(_ context.Context, _ work.WorkRequest) error { return nil },
	)
	t.Cleanup(func() {
		cancelRun()
		sidecars.Wait()
	})

	waitForPollerRunnerCalls(t, runner, 1, time.Second)
	waitForFakeClockWaiters(t, fakeClock, 1)
	fakeClock.Advance(automationservice.ScriptPollerRestartBackoffMin)
	waitForPollerRunnerCalls(t, runner, 2, time.Second)

	if observedLogs.FilterMessage("script poller restarting").Len() == 0 {
		t.Fatal("expected restart log for malformed poller output")
	}
	entry := observedLogs.FilterMessage("script poller restarting").All()[0]
	if got := entry.ContextMap()["error"]; got == nil || !strings.Contains(got.(string), "malformed stdout") {
		t.Fatalf("restart error = %#v, want malformed stdout context", got)
	}
}

func newCanonicalScriptPollerWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           canonicalScriptPollerWorkstationName,
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: canonicalScriptPollerWorkerName,
	}
}

func newCanonicalScriptPollerWorker(args ...string) *interfaces.FactoryWorkerConfig {
	return &interfaces.FactoryWorkerConfig{
		Name:    canonicalScriptPollerWorkerName,
		Type:    interfaces.WorkerTypeScript,
		Command: canonicalScriptPollerCommand,
		Args:    args,
	}
}

type scriptPollerRuntimeConfigOptions struct {
	poller       interfaces.FactoryWorkstationConfig
	pollerWorker *interfaces.FactoryWorkerConfig
}

func newScriptPollerLoadedRuntimeConfig(
	t *testing.T,
	factoryDir string,
	options scriptPollerRuntimeConfigOptions,
) interfaces.MutableLoadedFactorySource {
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
		Workers:      []interfaces.FactoryWorkerConfig{{Name: pollerWorker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{poller},
	}
	workerConfigs := map[string]*interfaces.FactoryWorkerConfig{
		pollerWorker.Name: pollerWorker,
	}
	workstationConfigs := map[string]*interfaces.FactoryWorkstationConfig{
		poller.Name: &poller,
	}

	loaded, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		factoryCfg,
		runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers:      workerConfigs,
			Workstations: workstationConfigs,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
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

func waitForPollerRunnerCalls(t *testing.T, runner *pollerSequenceCommandRunner, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if runner.callCount() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d poller runner call(s); got %d", want, runner.callCount())
}
