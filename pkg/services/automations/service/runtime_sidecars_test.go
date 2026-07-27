package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestStartSchedulerSidecarsForRuntime_AttachesCronAndScriptPollerSupervision(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	factoryDir := t.TempDir()

	cronWS := cronWorkstationConfigForTest("scheduled-task")
	cronWS.Cron.TriggerAtStart = true
	scriptPoller := newCanonicalScriptPollerWorkstation()
	scriptWorker := newCanonicalScriptPollerWorker()

	factoryCfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{Name: "task"}},
		Workers:   []interfaces.FactoryWorkerConfig{{Name: scriptWorker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			cronWS,
			scriptPoller,
		},
	}
	loaded, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		factoryCfg,
		runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				scriptWorker.Name: scriptWorker,
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				cronWS.Name:       &cronWS,
				scriptPoller.Name: &scriptPoller,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	workRequestJSON := []byte(`{
		"requestId":"script-batch-1",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-script","workTypeName":"task","payload":{"id":"SCRIPT-1"}}]
	}`)
	runner := &pollerSequenceCommandRunner{
		outcomes: []pollerRunOutcome{{result: workers.CommandResult{Stdout: workRequestJSON}}},
	}
	submitted := &recordingSubmitter{}

	svc := newAutomationService(automationFixture{
		Logger:        zap.NewNop(),
		Clock:         fakeClock,
		CommandRunner: runner,
	})

	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sidecars sync.WaitGroup
	svc.StartSchedulerSidecarsForRuntime(
		sidecarCtx,
		&sidecars,
		factoryDir,
		factoryCfg,
		loaded,
		submitted.submit,
	)

	waitForPollerSubmission(t, submitted, 1, 2*time.Second)

	var cronRequest work.WorkRequest
	var foundCron bool
	_, submissions := submitted.snapshot()
	for _, request := range submissions {
		if len(request.Works) > 0 && request.Works[0].Tags[interfaces.TimeWorkTagKeyCronWorkstation] == cronWS.Name {
			cronRequest = request
			foundCron = true
			break
		}
	}
	if !foundCron {
		t.Fatal("expected cron trigger-at-start submission")
	}
	assertCronWorkRequestForWorkstation(t, cronRequest, start, cronWS.Name)

	cancel()
	sidecars.Wait()
}

func TestStartSchedulerSidecarsForRuntime_CronCadenceSubmitsScheduledTicks(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	factoryDir := t.TempDir()

	cronWS := cronWorkstationConfigForTest("scheduled-task")
	cronWS.Cron.TriggerAtStart = true

	factoryCfg := &interfaces.FactoryConfig{
		WorkTypes:    []interfaces.WorkTypeConfig{{Name: "task"}},
		Workstations: []interfaces.FactoryWorkstationConfig{cronWS},
	}
	loaded, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		factoryCfg,
		runtimefixtures.RuntimeDefinitionLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				cronWS.Name: &cronWS,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	observedRequests := make(chan work.WorkRequest, 8)
	svc := newAutomationService(automationFixture{
		Logger: zap.NewNop(),
		Clock:  fakeClock,
	})

	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sidecars sync.WaitGroup
	svc.StartSchedulerSidecarsForRuntime(
		sidecarCtx,
		&sidecars,
		factoryDir,
		factoryCfg,
		loaded,
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
		cancel()
		sidecars.Wait()
	})

	startupRequest := waitForCronWorkRequest(t, observedRequests, time.Second)
	assertCronWorkRequestForWorkstation(t, startupRequest, start, cronWS.Name)

	waitForFakeClockWaiters(t, fakeClock, 1)
	assertNoCronWorkRequestQueued(t, observedRequests)
	fakeClock.Advance(time.Minute)
	scheduledRequest := waitForCronWorkRequest(t, observedRequests, time.Second)
	assertCronWorkRequestForWorkstation(t, scheduledRequest, start.Add(time.Minute), cronWS.Name)
}

func TestStartSchedulerSidecarsForRuntime_ReportsHostedStartFailureWithoutSubmission(t *testing.T) {
	startErr := errors.New("hosted poller unavailable")
	hostedPollers := programmableHostedPollers{
		Start: func(
			context.Context,
			*sync.WaitGroup,
			interfaces.RuntimeConfigLookup,
			interfaces.FactoryWorkstationConfig,
			*interfaces.FactoryWorkerConfig,
			automations.HostedWorkSubmitter,
		) error {
			return startErr
		},
	}
	poller := hostedLinearPollerWorkstation()
	worker := hostedLinearPollerWorker()
	factoryConfig := &interfaces.FactoryConfig{
		Workers:      []interfaces.FactoryWorkerConfig{*worker},
		Workstations: []interfaces.FactoryWorkstationConfig{poller},
	}
	runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{
		Factory:      factoryConfig,
		FactoryPath:  t.TempDir(),
		Workers:      map[string]*interfaces.FactoryWorkerConfig{worker.Name: worker},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{poller.Name: &poller},
	}
	service := newAutomationService(automationFixture{
		WorkflowID:    "hosted-start-failure",
		HostedPollers: hostedPollers,
	})
	submitted := &recordingSubmitter{}

	ctx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	err := service.StartSchedulerSidecarsForRuntime(
		ctx,
		&sidecars,
		runtimeConfig.FactoryDir(),
		factoryConfig,
		runtimeConfig,
		submitted.submit,
	)
	if !errors.Is(err, automations.ErrSupervisionFailed) || !errors.Is(err, startErr) {
		t.Fatalf("scheduler start error = %v, want typed supervision failure wrapping %v", err, startErr)
	}
	if calls, _ := submitted.snapshot(); calls != 0 {
		t.Fatalf("canonical Work submissions after failed start = %d, want 0", calls)
	}

	cancel()
	sidecars.Wait()
}
