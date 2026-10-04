package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	sourcelifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle"
	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// TestStartHostedLinearPoller_SubmitsIssuesThroughWorkersService verifies the
// Source lifecycle forwards hosted-poller inputs and output admission through
// its injected hosted-sources owner.
// Provider polling behavior with this same scenario name lives in
// automations/internal/services/hosted_sources/internal/service/supervisor_test.go.
func TestStartHostedLinearPoller_SubmitsIssuesThroughWorkersService(t *testing.T) {
	poller := hostedLinearPollerWorkstation()
	worker := hostedLinearPollerWorker()
	runtimeCfg := runtimefixtures.RuntimeConfigLookupFixture{Workers: map[string]*interfaces.FactoryWorkerConfig{worker.Name: worker}}
	submitted := &lifecycleRecordingSubmitter{}

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
	svc := &service{logger: zap.NewNop(), hostedPollers: hostedPollers}

	var sidecars sync.WaitGroup
	if err := svc.startPollersForRuntime(sourcelifecycle.RuntimeSourceConfiguration{}, context.Background(), &sidecars, &interfaces.FactoryConfig{Workstations: []interfaces.FactoryWorkstationConfig{poller}}, runtimeCfg, submitted.submit); err != nil {
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
// and WaitGroup ownership cross the lifecycle-to-hosted-owner contract.
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
	svc := &service{logger: zap.NewNop(), hostedPollers: hostedPollers}

	sidecarCtx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	if err := svc.startPollersForRuntime(
		sourcelifecycle.RuntimeSourceConfiguration{},
		sidecarCtx,
		&sidecars,
		&interfaces.FactoryConfig{Workstations: []interfaces.FactoryWorkstationConfig{hostedLinearPollerWorkstation()}},
		runtimefixtures.RuntimeConfigLookupFixture{Workers: map[string]*interfaces.FactoryWorkerConfig{"linear-poller": hostedLinearPollerWorker()}},
		func(context.Context, work.WorkRequest) error { return nil },
	); err != nil {
		t.Fatalf("StartHostedLinearPoller() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(30 * time.Second):
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
	submitted := &lifecycleRecordingSubmitter{}
	scriptPoller := interfaces.FactoryWorkstationConfig{Name: "script", Kind: interfaces.WorkstationKindPoller, WorkerTypeName: "script-worker"}
	scriptWorker := &interfaces.FactoryWorkerConfig{Name: "script-worker", Type: interfaces.WorkerTypeScript}
	hostedPoller := hostedLinearPollerWorkstation()
	hostedWorker := hostedLinearPollerWorker()
	factoryCfg := &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: scriptWorker.Name},
			{Name: hostedWorker.Name},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{scriptPoller, hostedPoller},
	}
	loaded := runtimefixtures.RuntimeConfigLookupFixture{Workers: map[string]*interfaces.FactoryWorkerConfig{scriptWorker.Name: scriptWorker, hostedWorker.Name: hostedWorker}}

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
	svc := &service{logger: zap.NewNop(), hostedPollers: hostedPollers, scriptPollers: lifecycleSubmittingScript{}}

	sidecarCtx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		sidecars.Wait()
	})
	if err := svc.ValidatePollersForRuntime(factoryCfg, loaded, submitted.submit); err != nil {
		t.Fatal(err)
	}
	if err := svc.startPollersForRuntime(sourcelifecycle.RuntimeSourceConfiguration{}, sidecarCtx, &sidecars, factoryCfg, loaded, submitted.submit); err != nil {
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

func waitForPollerSubmission(t *testing.T, submitted *lifecycleRecordingSubmitter, want int, _ time.Duration) {
	t.Helper()
	calls, _ := submitted.snapshot()
	if calls != want {
		t.Fatalf("submissions=%d, want %d", calls, want)
	}
}

type programmableHostedPollers struct {
	Start func(
		context.Context,
		*sync.WaitGroup,
		interfaces.RuntimeConfigLookup,
		interfaces.FactoryWorkstationConfig,
		*interfaces.FactoryWorkerConfig,
		automations.HostedWorkSubmitter,
	) error
	Validate func(
		interfaces.RuntimeConfigLookup,
		interfaces.FactoryWorkstationConfig,
		*interfaces.FactoryWorkerConfig,
		automations.HostedWorkSubmitter,
	) error
}

func (p programmableHostedPollers) StartLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	if p.Start == nil {
		return nil
	}
	return p.Start(ctx, sidecars, runtimeConfig, workstation, worker, submitter)
}

func (p programmableHostedPollers) ValidateLinearPoller(
	runtimeConfig interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
	submitter automations.HostedWorkSubmitter,
) error {
	if p.Validate == nil {
		return nil
	}
	return p.Validate(runtimeConfig, workstation, worker, submitter)
}

type lifecycleRecordingSubmitter struct{ submissions []work.WorkRequest }

func (r *lifecycleRecordingSubmitter) submit(_ context.Context, request work.WorkRequest) error {
	r.submissions = append(r.submissions, request)
	return nil
}
func (r *lifecycleRecordingSubmitter) snapshot() (int, []work.WorkRequest) {
	return len(r.submissions), r.submissions
}

type lifecycleSubmittingScript struct{ scriptpollers.Service }

func (lifecycleSubmittingScript) StartScriptPoller(ctx context.Context, _ *sync.WaitGroup, _ interfaces.RuntimeConfigLookup, _ interfaces.FactoryWorkstationConfig, _ *interfaces.FactoryWorkerConfig, _ scriptpollers.ScriptPollerSupervision, submit automations.WorkRequestSubmitter) {
	_ = submit(ctx, work.WorkRequest{RequestID: "script-batch-1", Works: []work.Work{{WorkID: "script:issue-new"}}})
}
