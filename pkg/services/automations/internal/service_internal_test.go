package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	cronwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron/wire"
	filesystemwatcherswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers/wire"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
	service := newAutomationServiceForTest(
		zap.NewNop(), clock, nil, workflowID, "", nil, nil,
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

func TestSchedulerSourceObservationAttachesBeforeStartEffectInitialization(t *testing.T) {
	service := newAutomationServiceForTest(zap.NewNop(), clockwork.NewFakeClock(), nil, "", "", nil, nil)
	identity := automations.SourceIdentity{
		AutomationID: "workflow-start-barrier",
		SourceID:     runtimeSchedulerSourceID,
	}
	factoryConfig := &interfaces.FactoryConfig{}
	runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{
		Factory:     factoryConfig,
		FactoryPath: t.TempDir(),
	}
	var sidecars sync.WaitGroup
	source := &schedulerSource{}
	source.configure(schedulerSourceConfig{
		sidecars:      &sidecars,
		factoryDir:    runtimeConfig.FactoryDir(),
		factoryConfig: factoryConfig,
		runtimeConfig: runtimeConfig,
		submitter:     func(context.Context, work.WorkRequest) error { return nil },
	})
	effect := reconciliation.WaitEffect{
		Desired: automations.DesiredLifecycleRunning,
		Observation: automations.SourceObservation{
			Identity:   identity,
			InstanceID: "instance-start-barrier",
			State:      automations.ObservedLifecycleStarting,
		},
	}

	cancelledCtx, cancelWait := context.WithCancel(context.Background())
	cancelWait()
	if _, err := source.observe(cancelledCtx, effect); !errors.Is(err, context.Canceled) {
		t.Fatalf("observe before start effect = %v, want attached wait cancellation", err)
	}

	ctx, cancelSource := context.WithCancel(context.Background())
	if err := source.start(ctx, service, identity); err != nil {
		t.Fatalf("start scheduler source: %v", err)
	}
	observation, err := source.observe(context.Background(), effect)
	if err != nil {
		t.Fatalf("observe initialized scheduler source: %v", err)
	}
	if observation.State != automations.ObservedLifecycleRunning {
		t.Fatalf("initialized observation = %q, want %q",
			observation.State, automations.ObservedLifecycleRunning)
	}
	cancelSource()
	sidecars.Wait()
}

func TestProductionRootUsesScriptPollersOwner(t *testing.T) {
	t.Parallel()

	service := newAutomationServiceForTest(
		zap.NewNop(), clockwork.NewFakeClock(), nil, "workflow-script-pollers", "", nil, nil,
	)
	if service.scriptPollers == nil {
		t.Fatal("expected script pollers owner on production Automations root")
	}
}

func TestProductionRootScriptPollerCursorThroughCompositionPath(t *testing.T) {
	t.Parallel()

	const workflowID = "workflow-script-poller-cursor"
	factoryDir := t.TempDir()
	stdout := []byte(`{
		"requestId":"linear-issue-batch-cursor",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-cursor","workTypeName":"task","payload":{"id":"ISSUE-CURSOR"}}],
		"cursor":"opaque-cursor-root",
		"checkpoint":"checkpoint-root"
	}`)
	runner := &internalScriptPollerRunner{
		outcomes: []internalScriptPollerOutcome{{result: workers.CommandResult{Stdout: stdout}}},
	}
	submitted := &internalScriptPollerSubmitter{}
	service := newAutomationServiceForTest(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		runner,
		workflowID,
		"",
		nil,
		nil,
	)
	poller := internalCanonicalScriptPollerWorkstation()
	worker := internalCanonicalScriptPollerWorker()
	runtimeCfg := internalScriptPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)

	err := service.RunScriptPoller(
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

	instanceID := testScriptPollerInstanceID(workflowID, poller.Name)
	cursor, err := service.Root().GetCursor(
		context.Background(),
		automations.GetCursorRequest{InstanceID: instanceID},
	)
	if err != nil {
		t.Fatalf("Root.GetCursor: %v", err)
	}
	if cursor.AutomationID != workflowID ||
		cursor.InstanceID != instanceID ||
		string(cursor.Cursor) != "opaque-cursor-root" ||
		cursor.Checkpoint != "checkpoint-root" {
		t.Fatalf("Root.GetCursor = %+v, want committed opaque recovery facts for %q", cursor, instanceID)
	}
}

type internalScriptPollerSubmitter struct {
	calls int
}

func (s *internalScriptPollerSubmitter) submit(context.Context, work.WorkRequest) error {
	s.calls++
	return nil
}

type internalScriptPollerOutcome struct {
	result workers.CommandResult
	err    error
}

type internalScriptPollerRunner struct {
	outcomes []internalScriptPollerOutcome
}

func (r *internalScriptPollerRunner) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	if len(r.outcomes) == 0 {
		return workers.CommandResult{}, errors.New("no scripted outcomes")
	}
	outcome := r.outcomes[0]
	r.outcomes = r.outcomes[1:]
	return outcome.result, outcome.err
}

func internalCanonicalScriptPollerWorkstation() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name:           "linear-ingress",
		Kind:           interfaces.WorkstationKindPoller,
		WorkerTypeName: "poller-script",
	}
}

func internalCanonicalScriptPollerWorker() *interfaces.FactoryWorkerConfig {
	return &interfaces.FactoryWorkerConfig{
		Name:    "poller-script",
		Type:    interfaces.WorkerTypeScript,
		Command: "factory/scripts/poller.sh",
	}
}

func internalScriptPollerLoadedRuntimeConfig(
	t *testing.T,
	factoryDir string,
	poller interfaces.FactoryWorkstationConfig,
	worker *interfaces.FactoryWorkerConfig,
) interfaces.MutableLoadedFactorySource {
	t.Helper()

	factoryCfg := &interfaces.FactoryConfig{
		Workers:      []interfaces.FactoryWorkerConfig{{Name: worker.Name}},
		Workstations: []interfaces.FactoryWorkstationConfig{poller},
	}
	loaded, err := factorydefinitioncomposition.NewLoadedSource(
		factoryDir,
		factoryCfg,
		runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				worker.Name: worker,
			},
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				poller.Name: &poller,
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

func TestProductionRootUsesSchedulerReconciliationOwner(t *testing.T) {
	t.Parallel()

	const workflowID = "workflow-production-root"
	service, identity, sidecars, cancel := startProductionRootScheduler(t, workflowID)
	defer cancel()
	root := service.Root()
	observation := assertProductionRootStatus(t, root, identity)
	assertProductionRootReconcile(t, root, observation)
	assertProductionRootRepeatedStart(t, root, observation)
	assertProductionRootInstanceReads(t, root, observation)
	stopProductionRootScheduler(t, root, identity, sidecars)
}

func startProductionRootScheduler(
	t *testing.T,
	workflowID string,
) (*Service, automations.SourceIdentity, *sync.WaitGroup, context.CancelFunc) {
	t.Helper()
	service := newAutomationServiceForTest(
		zap.NewNop(), clockwork.NewFakeClock(), nil, workflowID, "", nil, nil,
	)
	identity := automations.SourceIdentity{
		AutomationID: workflowID,
		SourceID:     runtimeSchedulerSourceID,
	}
	factoryConfig := &interfaces.FactoryConfig{}
	runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{
		Factory:     factoryConfig,
		FactoryPath: t.TempDir(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	sidecars := &sync.WaitGroup{}
	if err := service.StartSchedulerSidecarsForRuntime(
		ctx,
		sidecars,
		runtimeConfig.FactoryDir(),
		factoryConfig,
		runtimeConfig,
		func(context.Context, work.WorkRequest) error { return nil },
	); err != nil {
		t.Fatalf("StartSchedulerSidecarsForRuntime: %v", err)
	}
	return service, identity, sidecars, cancel
}

func assertProductionRootStatus(
	t *testing.T,
	root automations.Root,
	identity automations.SourceIdentity,
) automations.SourceObservation {
	t.Helper()
	status, err := root.SourceStatus(
		context.Background(),
		automations.SourceStatusRequest{Identity: identity},
	)
	if err != nil {
		t.Fatalf("Root.SourceStatus: %v", err)
	}
	if status.Observation.State != automations.ObservedLifecycleRunning {
		t.Fatalf("Root.SourceStatus state = %q, want %q",
			status.Observation.State, automations.ObservedLifecycleRunning)
	}
	return status.Observation
}

func assertProductionRootReconcile(
	t *testing.T,
	root automations.Root,
	observation automations.SourceObservation,
) {
	t.Helper()
	reconciled, err := root.Reconcile(
		context.Background(),
		automations.ReconcileRequest{
			Desired: []automations.DesiredSpec{{
				AutomationID: observation.Identity.AutomationID,
				SourceID:     observation.Identity.SourceID,
				Kind:         runtimeSchedulerSourceKind,
				State:        automations.DesiredLifecycleRunning,
			}},
			Observed: []automations.ObservedInstance{{
				AutomationID: observation.Identity.AutomationID,
				SourceID:     observation.Identity.SourceID,
				InstanceID:   observation.InstanceID,
				State:        observation.State,
			}},
		},
	)
	if err != nil {
		t.Fatalf("Root.Reconcile: %v", err)
	}
	if len(reconciled.Outcomes) != 1 ||
		reconciled.Outcomes[0].Convergence != automations.ConvergenceStatusConverged {
		t.Fatalf("Root.Reconcile outcomes = %+v, want one converged source",
			reconciled.Outcomes)
	}
}

func assertProductionRootRepeatedStart(
	t *testing.T,
	root automations.Root,
	observation automations.SourceObservation,
) {
	t.Helper()
	started, err := root.StartSource(
		context.Background(),
		automations.StartSourceRequest{
			Identity: observation.Identity,
			Kind:     runtimeSchedulerSourceKind,
		},
	)
	if err != nil {
		t.Fatalf("Root.StartSource: %v", err)
	}
	if !started.Outcome.Idempotent ||
		started.Outcome.Observation.InstanceID != observation.InstanceID {
		t.Fatalf("Root.StartSource outcome = %+v, want same idempotent instance",
			started.Outcome)
	}
}

func assertProductionRootInstanceReads(
	t *testing.T,
	root automations.Root,
	observation automations.SourceObservation,
) {
	t.Helper()
	instanceStatus, err := root.GetStatus(
		context.Background(),
		automations.GetStatusRequest{InstanceID: observation.InstanceID},
	)
	if err != nil {
		t.Fatalf("Root.GetStatus: %v", err)
	}
	if instanceStatus.AutomationID != observation.Identity.AutomationID ||
		instanceStatus.Status != automations.ObservedLifecycleRunning {
		t.Fatalf("Root.GetStatus = %+v, want workflow %q running",
			instanceStatus, observation.Identity.AutomationID)
	}
	cursor, err := root.GetCursor(
		context.Background(),
		automations.GetCursorRequest{InstanceID: observation.InstanceID},
	)
	if err != nil {
		t.Fatalf("Root.GetCursor: %v", err)
	}
	if cursor.AutomationID != observation.Identity.AutomationID ||
		cursor.InstanceID != observation.InstanceID {
		t.Fatalf("Root.GetCursor = %+v, want workflow/instance %q/%q",
			cursor, observation.Identity.AutomationID, observation.InstanceID)
	}
}

func stopProductionRootScheduler(
	t *testing.T,
	root automations.Root,
	identity automations.SourceIdentity,
	sidecars *sync.WaitGroup,
) {
	t.Helper()
	if _, err := root.StopSource(
		context.Background(),
		automations.StopSourceRequest{Identity: identity},
	); err != nil {
		t.Fatalf("Root.StopSource: %v", err)
	}
	stopped, err := root.WaitSource(
		context.Background(),
		automations.WaitSourceRequest{
			Identity: identity,
			Desired:  automations.DesiredLifecycleStopped,
		},
	)
	if err != nil {
		t.Fatalf("Root.WaitSource: %v", err)
	}
	if stopped.Outcome.Observation.State != automations.ObservedLifecycleStopped ||
		stopped.Outcome.Convergence != automations.ConvergenceStatusConverged {
		t.Fatalf("Root.WaitSource outcome = %+v, want stopped/converged", stopped.Outcome)
	}
	sidecars.Wait()
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
	service := newAutomationServiceForTest(zap.NewNop(), clockwork.NewFakeClock(), nil, "", "", nil, nil)
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

func newAutomationServiceForTest(
	logger *zap.Logger,
	clock Clock,
	commandRunner workers.CommandRunner,
	workflowID string,
	defaultFactoryDir string,
	hostedPollers automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver,
) *Service {
	var service *Service
	reconciler := reconciliationwire.NewService(reconciliation.Effects{
		Start: func(ctx context.Context, effect reconciliation.StartEffect) error {
			return service.StartSchedulerSourceEffect(ctx, effect)
		},
		Stop: func(ctx context.Context, effect reconciliation.StopEffect) error {
			return service.StopSchedulerSourceEffect(ctx, effect)
		},
		Wait: func(ctx context.Context, effect reconciliation.WaitEffect) (automations.SourceObservation, error) {
			return service.WaitSchedulerSourceEffect(ctx, effect)
		},
	})
	service = New(
		logger,
		clock,
		commandRunner,
		workflowID,
		defaultFactoryDir,
		hostedPollers,
		resolveTemplates,
		reconciler,
		scriptpollerswire.NewService(
			testPollerLogger(logger),
			testPollerClock(clock),
			commandRunner,
			resolveTemplates,
		),
		cronwire.NewService(),
		filesystemwatcherswire.NewService(testPollerClock(clock)),
	)
	return service
}

func testPollerLogger(logger *zap.Logger) *zap.Logger {
	if logger == nil {
		logger = zap.NewNop()
	}
	return logger
}

func testPollerClock(clock Clock) clockwork.Clock {
	if typed, ok := clock.(clockwork.Clock); ok && typed != nil {
		return typed
	}
	return clockwork.NewRealClock()
}

func testScriptPollerInstanceID(automationID, workstationName string) string {
	automationID = strings.TrimSpace(automationID)
	sourceID := "script-poller:" + strings.TrimSpace(workstationName)
	identity := fmt.Sprintf("%d:%s:%d:%s", len(automationID), automationID, len(sourceID), sourceID)
	sum := sha256.Sum256([]byte("automations-script-poller-instance:" + identity))
	return "script-poller-instance:" + hex.EncodeToString(sum[:16])
}
