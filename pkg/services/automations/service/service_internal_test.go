package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

func TestNilServiceUsesSafeSchedulerDefaults(t *testing.T) {
	t.Parallel()

	var svc *Service
	if svc.logger() == nil || svc.commandRunner() == nil || svc.supervisorClock() == nil || svc.pollerLogger("workstation", "worker") == nil {
		t.Fatal("nil worker service did not provide safe scheduler defaults")
	}
}

func TestSchedulerSidecarsReconcileLifecycleBeforeCanonicalWorkSubmission(t *testing.T) {
	start := time.Date(2026, time.July, 27, 15, 0, 0, 0, time.UTC)
	clock := clockwork.NewFakeClockAt(start)
	const workflowID = "workflow-reconciliation"
	workstation := interfaces.FactoryWorkstationConfig{
		Name: "scheduled-task",
		Kind: interfaces.WorkstationKindCron,
		Cron: &interfaces.CronConfig{
			Schedule:       "* * * * *",
			TriggerAtStart: true,
		},
		Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
	}
	factoryConfig := &interfaces.FactoryConfig{
		WorkTypes:    []interfaces.WorkTypeConfig{{Name: "task"}},
		Workstations: []interfaces.FactoryWorkstationConfig{workstation},
	}
	runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{
		Factory:      factoryConfig,
		FactoryPath:  t.TempDir(),
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{},
	}
	service := New(
		zap.NewNop(), clock, nil, workflowID, "", nil, nil, nil,
	)
	identity := automations.SourceIdentity{
		AutomationID: workflowID,
		SourceID:     runtimeSchedulerSourceID,
	}

	var submittedMu sync.Mutex
	var submitted []work.WorkRequest
	submitter := func(ctx context.Context, request work.WorkRequest) error {
		status, err := service.reconciler.SourceStatus(
			ctx,
			automations.SourceStatusRequest{Identity: identity},
		)
		if err != nil {
			return err
		}
		if status.Observation.State != automations.ObservedLifecycleStarting {
			t.Errorf(
				"source state at canonical Work submission = %q, want %q",
				status.Observation.State,
				automations.ObservedLifecycleStarting,
			)
		}
		submittedMu.Lock()
		submitted = append(submitted, request)
		submittedMu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	var sidecars sync.WaitGroup
	startSchedulerConcurrently(
		t, service, ctx, &sidecars, factoryConfig, runtimeConfig, submitter,
	)
	assertSchedulerState(t, service, identity, automations.ObservedLifecycleRunning)
	assertSubmittedWorkRequests(t, &submittedMu, &submitted, 1)

	cancel()
	sidecars.Wait()
	assertSchedulerState(t, service, identity, automations.ObservedLifecycleStopped)

	clock.Advance(time.Minute)
	restartCtx, restartCancel := context.WithCancel(context.Background())
	var restartedSidecars sync.WaitGroup
	if err := service.StartSchedulerSidecarsForRuntime(
		restartCtx,
		&restartedSidecars,
		runtimeConfig.FactoryDir(),
		factoryConfig,
		runtimeConfig,
		submitter,
	); err != nil {
		t.Fatalf("restart scheduler sidecars: %v", err)
	}
	assertSchedulerState(t, service, identity, automations.ObservedLifecycleRunning)
	assertSubmittedWorkRequests(t, &submittedMu, &submitted, 2)

	restartCancel()
	restartedSidecars.Wait()
}

func startSchedulerConcurrently(
	t *testing.T,
	service *Service,
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryConfig *interfaces.FactoryConfig,
	runtimeConfig runtimefixtures.RuntimeConfigLookupFixture,
	submitter automations.WorkRequestSubmitter,
) {
	t.Helper()
	startErrors := make(chan error, 2)
	var starts sync.WaitGroup
	for range 2 {
		starts.Add(1)
		go func() {
			defer starts.Done()
			startErrors <- service.StartSchedulerSidecarsForRuntime(
				ctx,
				sidecars,
				runtimeConfig.FactoryDir(),
				factoryConfig,
				runtimeConfig,
				submitter,
			)
		}()
	}
	starts.Wait()
	close(startErrors)
	for err := range startErrors {
		if err != nil {
			t.Fatalf("concurrent scheduler sidecar start: %v", err)
		}
	}
}

func TestSchedulerSidecarsReconcileDifferentRuntimeIdentitiesConcurrently(t *testing.T) {
	service := New(zap.NewNop(), clockwork.NewFakeClock(), nil, "", "", nil, nil, nil)
	factoryConfig := &interfaces.FactoryConfig{}
	directories := []string{t.TempDir(), t.TempDir()}
	contexts := make([]context.Context, len(directories))
	cancels := make([]context.CancelFunc, len(directories))
	sidecars := make([]sync.WaitGroup, len(directories))
	startErrors := make(chan error, len(directories))

	var starts sync.WaitGroup
	for i, factoryDir := range directories {
		contexts[i], cancels[i] = context.WithCancel(context.Background())
		runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{
			Factory:     factoryConfig,
			FactoryPath: factoryDir,
		}
		starts.Add(1)
		go func(index int, config runtimefixtures.RuntimeConfigLookupFixture) {
			defer starts.Done()
			startErrors <- service.StartSchedulerSidecarsForRuntime(
				contexts[index],
				&sidecars[index],
				config.FactoryDir(),
				factoryConfig,
				config,
				func(context.Context, work.WorkRequest) error { return nil },
			)
		}(i, runtimeConfig)
	}
	starts.Wait()
	close(startErrors)
	for err := range startErrors {
		if err != nil {
			t.Fatalf("start distinct scheduler source: %v", err)
		}
	}
	for _, factoryDir := range directories {
		assertSchedulerState(t, service, service.schedulerSourceIdentity(factoryDir), automations.ObservedLifecycleRunning)
	}

	for _, cancel := range cancels {
		cancel()
	}
	for i := range sidecars {
		sidecars[i].Wait()
	}
	for _, factoryDir := range directories {
		assertSchedulerState(t, service, service.schedulerSourceIdentity(factoryDir), automations.ObservedLifecycleStopped)
	}
}

func assertSchedulerState(
	t *testing.T,
	service *Service,
	identity automations.SourceIdentity,
	want automations.ObservedLifecycleState,
) {
	t.Helper()
	status, err := service.reconciler.SourceStatus(
		context.Background(),
		automations.SourceStatusRequest{Identity: identity},
	)
	if err != nil {
		t.Fatalf("scheduler source status: %v", err)
	}
	if status.Observation.State != want {
		t.Fatalf("scheduler source state = %q, want %q", status.Observation.State, want)
	}
}

func assertSubmittedWorkRequests(
	t *testing.T,
	mu *sync.Mutex,
	submitted *[]work.WorkRequest,
	want int,
) {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	if len(*submitted) != want {
		t.Fatalf("canonical Work submissions = %d, want %d", len(*submitted), want)
	}
	if len(*submitted) > 1 &&
		(*submitted)[len(*submitted)-1].Works[0].WorkID == (*submitted)[len(*submitted)-2].Works[0].WorkID {
		t.Fatal("real restarted transition repeated the prior canonical Work identity")
	}
}
