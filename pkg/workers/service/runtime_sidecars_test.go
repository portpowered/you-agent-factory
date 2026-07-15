package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
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
		Workers:   []workerconfig.Config{{Name: scriptWorker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			cronWS,
			scriptPoller,
		},
	}
	loaded, err := config.NewLoadedFactoryConfig(factoryDir, factoryCfg, runtimefixtures.RuntimeDefinitionLookupFixture{
		Workers: map[string]*workerconfig.Config{
			scriptWorker.Name: scriptWorker,
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			cronWS.Name:       &cronWS,
			scriptPoller.Name: &scriptPoller,
		},
	})
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

	svc := workersservice.New(workersservice.Config{
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
		workersservice.RuntimeSidecarsInput{
			FactoryDir: factoryDir,
			FactoryCfg: factoryCfg,
			RuntimeCfg: loaded,
			Submitter:  submitted.submit,
		},
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
	loaded, err := config.NewLoadedFactoryConfig(factoryDir, factoryCfg, runtimefixtures.RuntimeDefinitionLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			cronWS.Name: &cronWS,
		},
	})
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	observedRequests := make(chan work.WorkRequest, 8)
	svc := workersservice.New(workersservice.Config{
		Logger: zap.NewNop(),
		Clock:  fakeClock,
	})

	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sidecars sync.WaitGroup
	svc.StartSchedulerSidecarsForRuntime(
		sidecarCtx,
		&sidecars,
		workersservice.RuntimeSidecarsInput{
			FactoryDir: factoryDir,
			FactoryCfg: factoryCfg,
			RuntimeCfg: loaded,
			Submitter: func(_ context.Context, request work.WorkRequest) error {
				select {
				case observedRequests <- request:
				default:
					t.Fatalf("cron request channel overflow")
				}
				return nil
			},
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
