package service_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"

	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestStartHostedLinearPoller_SubmitsIssuesThroughWorkersService verifies the
// Automations owner forwards hosted-poller inputs and output admission through
// its injected Workers-owned root role. Provider polling behavior with this
// same scenario name lives in hosted_logic/supervisor_test.go.
func TestStartHostedLinearPoller_SubmitsIssuesThroughWorkersService(t *testing.T) {
	poller := hostedLinearPollerWorkstation()
	worker := hostedLinearPollerWorker()
	runtimeCfg := runtimefixtures.RuntimeConfigLookupFixture{}
	submitted := &recordingSubmitter{}

	var startCalls atomic.Int32
	hostedPollers := programmableHostedPollers{
		Start: func(
			ctx context.Context,
			sidecars *sync.WaitGroup,
			gotRuntimeCfg interfaces.RuntimeConfigLookup,
			gotPoller interfaces.FactoryWorkstationConfig,
			gotWorker *interfaces.FactoryWorkerConfig,
			submitter automations.HostedWorkSubmitter,
		) error {
			startCalls.Add(1)
			if ctx == nil || sidecars == nil || gotRuntimeCfg == nil {
				t.Fatal("hosted-poller invocation omitted lifecycle or runtime inputs")
			}
			if gotPoller.Name != poller.Name || gotWorker != worker {
				t.Fatalf("hosted-poller inputs = (%q, %p), want (%q, %p)", gotPoller.Name, gotWorker, poller.Name, worker)
			}
			return submitter(ctx, work.WorkRequest{
				RequestID: "hosted-batch-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{WorkID: "linear:issue-new"}},
			})
		},
	}
	svc := newAutomationService(automationFixture{HostedPollers: hostedPollers})

	var sidecars sync.WaitGroup
	if err := svc.StartHostedLinearPoller(context.Background(), &sidecars, runtimeCfg, poller, worker, submitted.submit); err != nil {
		t.Fatalf("StartHostedLinearPoller() error = %v", err)
	}

	calls, requests := submitted.snapshot()
	if startCalls.Load() != 1 {
		t.Fatalf("hosted-poller start calls = %d, want 1", startCalls.Load())
	}
	if calls != 1 {
		t.Fatalf("submit calls = %d, want 1", calls)
	}
	if got := requests[0].Works[0].WorkID; got != "linear:issue-new" {
		t.Fatalf("submitted work id = %q, want linear:issue-new", got)
	}
}

// TestStartHostedLinearPoller_StopsOnContextCancellation verifies cancellation
// and WaitGroup ownership cross the Automations-to-Workers root contract.
func TestStartHostedLinearPoller_StopsOnContextCancellation(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan error, 1)
	hostedPollers := programmableHostedPollers{
		Start: func(
			ctx context.Context,
			sidecars *sync.WaitGroup,
			_ interfaces.RuntimeConfigLookup,
			_ interfaces.FactoryWorkstationConfig,
			_ *interfaces.FactoryWorkerConfig,
			_ automations.HostedWorkSubmitter,
		) error {
			sidecars.Add(1)
			go func() {
				defer sidecars.Done()
				close(started)
				<-ctx.Done()
				stopped <- ctx.Err()
			}()
			return nil
		},
	}
	svc := newAutomationService(automationFixture{HostedPollers: hostedPollers})

	sidecarCtx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	if err := svc.StartHostedLinearPoller(
		sidecarCtx,
		&sidecars,
		runtimefixtures.RuntimeConfigLookupFixture{},
		hostedLinearPollerWorkstation(),
		hostedLinearPollerWorker(),
		func(context.Context, work.WorkRequest) error { return nil },
	); err != nil {
		t.Fatalf("StartHostedLinearPoller() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for hosted-poller role to start")
	}
	cancel()
	sidecars.Wait()

	select {
	case reason := <-stopped:
		if reason != context.Canceled {
			t.Fatalf("hosted-poller stop reason = %v, want context canceled", reason)
		}
	default:
		t.Fatal("hosted-poller role did not observe context cancellation")
	}
}

func TestStartPollersForRuntime_StartsScriptAndHostedPollers(t *testing.T) {
	workRequestJSON := []byte(`{
		"requestId":"script-batch-1",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-script","workTypeName":"task","payload":{"id":"SCRIPT-1"}}]
	}`)
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{Stdout: workRequestJSON}}},
	}
	submitted := &recordingSubmitter{}

	scriptPoller := newCanonicalScriptPollerWorkstation()
	scriptWorker := newCanonicalScriptPollerWorker()
	hostedPoller := hostedLinearPollerWorkstation()
	hostedWorker := hostedLinearPollerWorker()
	factoryCfg := &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: scriptWorker.Name},
			{Name: hostedWorker.Name},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{scriptPoller, hostedPoller},
	}
	loaded, err := factorydefinitioncomposition.NewLoadedSource(
		t.TempDir(),
		factoryCfg,
		runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				scriptWorker.Name: scriptWorker,
				hostedWorker.Name: hostedWorker,
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				scriptPoller.Name: &scriptPoller,
				hostedPoller.Name: &hostedPoller,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	var validateCalls atomic.Int32
	var startCalls atomic.Int32
	hostedPollers := programmableHostedPollers{
		Validate: func(
			_ interfaces.RuntimeConfigLookup,
			_ interfaces.FactoryWorkstationConfig,
			_ *interfaces.FactoryWorkerConfig,
			_ automations.HostedWorkSubmitter,
		) error {
			validateCalls.Add(1)
			return nil
		},
		Start: func(
			ctx context.Context,
			_ *sync.WaitGroup,
			_ interfaces.RuntimeConfigLookup,
			_ interfaces.FactoryWorkstationConfig,
			_ *interfaces.FactoryWorkerConfig,
			submitter automations.HostedWorkSubmitter,
		) error {
			startCalls.Add(1)
			return submitter(ctx, work.WorkRequest{
				RequestID: "hosted-batch-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{WorkID: "linear:issue-new"}},
			})
		},
	}
	fakeClock := clockwork.NewFakeClock()
	svc := newAutomationService(automationFixture{
		Clock:         fakeClock,
		CommandRunner: runner,
		HostedPollers: hostedPollers,
	})

	sidecarCtx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		sidecars.Wait()
	})
	if err := svc.StartPollersForRuntime(sidecarCtx, &sidecars, factoryCfg, loaded, submitted.submit); err != nil {
		t.Fatalf("StartPollersForRuntime() error = %v", err)
	}

	waitForPollerSubmission(t, submitted, 2, 2*time.Second)
	calls, _ := submitted.snapshot()
	if calls < 2 {
		t.Fatalf("submit calls = %d, want at least 2 (script + hosted)", calls)
	}
	if validateCalls.Load() != 1 || startCalls.Load() != 1 {
		t.Fatalf("hosted-poller calls = validate %d start %d, want 1 each", validateCalls.Load(), startCalls.Load())
	}
}

func hostedLinearPollerWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "linear-poller",
	}
}

func hostedLinearPollerWorker() *interfaces.FactoryWorkerConfig {
	return &interfaces.FactoryWorkerConfig{
		Name:     "linear-poller",
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
		Auth:     &interfaces.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear: &interfaces.HostedLinearWorkerConfig{
			PollInterval: "1h",
			Mapping: interfaces.HostedLinearWorkerMappingConfig{
				WorkType: "story",
				State:    "init",
			},
		},
	}
}

func waitForPollerSubmission(t *testing.T, submitted *recordingSubmitter, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		calls, _ := submitted.snapshot()
		if calls >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	calls, _ := submitted.snapshot()
	t.Fatalf("timed out waiting for %d poller submission(s); got %d", want, calls)
}
