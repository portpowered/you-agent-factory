package service_test

import (
	"context"
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

func TestWorkflowIdentityForFactoryDir_UsesConfiguredWorkflowID(t *testing.T) {
	svc := newAutomationService(automationFixture{
		WorkflowID:        "workflow-override",
		DefaultFactoryDir: "/default/factory",
	})
	if got := svc.WorkflowIdentityForFactoryDir("/runtime/factory"); got != "workflow-override" {
		t.Fatalf("workflow identity = %q, want workflow-override", got)
	}
}

func TestWorkflowIdentityForFactoryDir_FallsBackToFactoryDirAndDefault(t *testing.T) {
	svc := newAutomationService(automationFixture{DefaultFactoryDir: "/default/factory"})
	if got := svc.WorkflowIdentityForFactoryDir("/runtime/factory"); got != "/runtime/factory" {
		t.Fatalf("workflow identity = %q, want /runtime/factory", got)
	}
	if got := svc.WorkflowIdentityForFactoryDir(""); got != "/default/factory" {
		t.Fatalf("empty factory dir identity = %q, want /default/factory", got)
	}
}

func TestStartPollersForRuntime_LogsDisabledPaths(t *testing.T) {
	logCore, observedLogs := observer.New(zap.WarnLevel)
	svc := newAutomationService(automationFixture{Logger: zap.New(logCore)})

	missingBinding := interfaces.FactoryWorkstationConfig{
		Name: "missing-binding",
		Kind: interfaces.WorkstationKindPoller,
	}
	missingWorker := interfaces.FactoryWorkstationConfig{
		Name:           "missing-worker",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "unknown-worker",
	}
	unsupportedHosted := interfaces.FactoryWorkstationConfig{
		Name:           "unsupported-hosted",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "github-poller",
	}
	githubWorker := &interfaces.FactoryWorkerConfig{
		Name:     "github-poller",
		Type:     interfaces.WorkerTypeHosted,
		Provider: "github",
	}

	factoryCfg := &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{{Name: githubWorker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			missingBinding,
			missingWorker,
			unsupportedHosted,
		},
	}
	runtimeCfg, err := factorydefinitioncomposition.NewLoadedSource(
		"",
		factoryCfg,
		runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				githubWorker.Name: githubWorker,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	var sidecars sync.WaitGroup
	svc.StartPollersForRuntime(
		context.Background(),
		&sidecars,
		factoryCfg,
		runtimeCfg,
		func(context.Context, work.WorkRequest) error { return nil },
	)

	if observedLogs.FilterMessage("script poller disabled").Len() != 2 {
		t.Fatalf("script poller disabled logs = %d, want 2", observedLogs.FilterMessage("script poller disabled").Len())
	}
	if observedLogs.FilterMessage("hosted poller disabled").Len() != 1 {
		t.Fatalf("hosted poller disabled logs = %d, want 1", observedLogs.FilterMessage("hosted poller disabled").Len())
	}
}

func TestScriptPollerCommandRequest_ResolvesWorkstationEnvAndWorkerTimeout(t *testing.T) {
	factoryDir := t.TempDir()
	poller := newCanonicalScriptPollerWorkstation()
	poller.Env = map[string]string{"POLLER_MODE": "watch"}
	poller.WorkingDirectory = "relative/workdir"
	worker := newCanonicalScriptPollerWorker()
	worker.Timeout = "50ms"

	runtimeCfg := newScriptPollerLoadedRuntimeConfig(
		t,
		factoryDir,
		scriptPollerRuntimeConfigOptions{poller: poller, pollerWorker: worker},
	)

	req, err := automationservice.ScriptPollerCommandRequest(runtimeCfg, poller, worker, nil)
	if err != nil {
		t.Fatalf("ScriptPollerCommandRequest: %v", err)
	}
	if len(req.Env) != 1 || req.Env[0] != "POLLER_MODE=watch" {
		t.Fatalf("poller env = %#v, want POLLER_MODE=watch", req.Env)
	}
	if !strings.Contains(req.WorkDir, filepath.Join("relative", "workdir")) {
		t.Fatalf("poller workdir = %q, want resolved relative path", req.WorkDir)
	}
}

func TestRunScriptPoller_UsesWorkerTimeout(t *testing.T) {
	factoryDir := t.TempDir()
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{waitForCancel: true}},
	}
	svc := newAutomationService(automationFixture{
		Logger:        zap.NewNop(),
		CommandRunner: runner,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	worker.Timeout = "1ms"
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(
		t,
		factoryDir,
		scriptPollerRuntimeConfigOptions{poller: poller, pollerWorker: worker},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := svc.RunScriptPoller(ctx, runner, runtimeCfg, poller, worker, func(context.Context, work.WorkRequest) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("RunScriptPoller error = %v, want timeout", err)
	}
}

func TestParseScriptPollerOutput_RejectsConflictingEnvelopeFields(t *testing.T) {
	raw := []byte(`{"request":{"requestId":"a","type":"FACTORY_REQUEST_BATCH","works":[]},"submissions":[]}`)
	_, hasOutput, err := automationservice.ParseScriptPollerOutput(raw)
	if !hasOutput {
		t.Fatal("expected conflicting envelope to count as output")
	}
	if err == nil || !strings.Contains(err.Error(), "either request or submissions") {
		t.Fatalf("error = %v, want request/submissions conflict", err)
	}
}

func TestParseScriptPollerOutput_RejectsInvalidRequestTypeAndMissingRequestID(t *testing.T) {
	invalidType := []byte(`{"requestId":"x","type":"UNSUPPORTED","works":[]}`)
	_, _, err := automationservice.ParseScriptPollerOutput(invalidType)
	if err == nil || !strings.Contains(err.Error(), "unsupported work request type") {
		t.Fatalf("invalid type error = %v", err)
	}

	missingID := []byte(`{"requestId":"","type":"FACTORY_REQUEST_BATCH","works":[{"name":"w","workTypeName":"task"}]}`)
	_, _, err = automationservice.ParseScriptPollerOutput(missingID)
	if err == nil || !strings.Contains(err.Error(), "requestId") {
		t.Fatalf("missing requestId error = %v", err)
	}
}

func TestParseScriptPollerOutput_EmptyStdoutIsNotOutput(t *testing.T) {
	_, hasOutput, err := automationservice.ParseScriptPollerOutput(nil)
	if hasOutput || err != nil {
		t.Fatalf("empty stdout = hasOutput %v err %v, want no output", hasOutput, err)
	}
}

func TestStartScriptPoller_StopReasonOnContextCancel(t *testing.T) {
	fakeClock := clockwork.NewFakeClock()
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{waitForCancel: true}},
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
	svc.StartScriptPoller(runCtx, &sidecars, runtimeCfg, poller, worker, func(context.Context, work.WorkRequest) error {
		return nil
	})
	cancelRun()
	sidecars.Wait()

	stopped := observedLogs.FilterMessage("script poller stopped").All()
	if len(stopped) != 1 {
		t.Fatalf("script poller stopped logs = %d, want 1", len(stopped))
	}
	if got := stopped[0].ContextMap()["reason"]; got != "context canceled" {
		t.Fatalf("stop reason = %#v, want context canceled", got)
	}
}

func TestStartCronWatchersForRuntime_LogsTriggerFailure(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	logCore, observedLogs := observer.New(zap.ErrorLevel)
	cronWS := cronWorkstationConfigForTest("failing-cron")
	cronWS.Cron.TriggerAtStart = true
	factoryCfg := &interfaces.FactoryConfig{
		WorkTypes:    []interfaces.WorkTypeConfig{{Name: "task"}},
		Workstations: []interfaces.FactoryWorkstationConfig{cronWS},
	}
	runtimeCfg := newCronLoadedRuntimeConfig(
		t,
		"factory-alpha",
		factoryCfg,
		map[string]*interfaces.FactoryWorkstationConfig{cronWS.Name: &cronWS},
	)
	svc := newAutomationService(automationFixture{
		Logger: zap.New(logCore),
		Clock:  fakeClock,
	})

	runCtx, cancelRun := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	svc.StartCronWatchersForRuntime(
		runCtx,
		&sidecars,
		"factory-alpha",
		factoryCfg,
		runtimeCfg,
		func(context.Context, work.WorkRequest) error {
			return errors.New("cron submit rejected")
		},
	)
	t.Cleanup(func() {
		cancelRun()
		sidecars.Wait()
	})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if observedLogs.FilterMessage("cron watcher trigger failed").Len() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected cron watcher trigger failed log")
}

func TestStartSchedulerSidecarsForRuntime_NoOpsOnNilInput(t *testing.T) {
	svc := newAutomationService(automationFixture{})
	var sidecars sync.WaitGroup
	svc.StartSchedulerSidecarsForRuntime(context.Background(), &sidecars, "", nil, nil, nil)
	svc.StartPollersForRuntime(context.Background(), nil, nil, nil, nil)
	svc.StartCronWatchersForRuntime(context.Background(), nil, "", nil, nil, nil)
	svc.StartHostedLinearPoller(context.Background(), nil, nil, interfaces.FactoryWorkstationConfig{}, nil, nil)
	svc.StartScriptPoller(context.Background(), nil, nil, interfaces.FactoryWorkstationConfig{}, nil, nil)
}

func TestService_DefaultCollaboratorsWhenConfigUnset(t *testing.T) {
	svc := newAutomationService(automationFixture{})
	factoryDir := t.TempDir()
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{}}},
	}
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
		func(context.Context, work.WorkRequest) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("RunScriptPoller with default collaborators error = %v", err)
	}
}
