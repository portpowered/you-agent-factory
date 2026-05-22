package service

import (
	"context"
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

func TestValidateScriptPollerOutput_RejectsNonEmptyStdout(t *testing.T) {
	err := validateScriptPollerOutput([]byte("submitted work\n"))
	if err == nil {
		t.Fatal("expected malformed stdout error")
	}
	if !strings.Contains(err.Error(), "malformed stdout") {
		t.Fatalf("error = %v, want malformed stdout", err)
	}
}
