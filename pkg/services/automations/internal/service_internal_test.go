package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factorydefinitioncomposition "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	cronwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron/wire"
	cursorscopeswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes/wire"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	fswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers/wire"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
	sourcelifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle"
	sourcelifecyclewire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle/wire"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestExplicitServiceConstructionDoesNotExecuteCommandRunner(t *testing.T) {
	t.Parallel()
	runner := &internalScriptPollerRunner{outcomes: []internalScriptPollerOutcome{{}}}
	service := newTestService(zap.NewNop(), clockwork.NewFakeClock(), runner, "", "", nil, nil, factorydefinitioncomposition.WorkstationExecutionPolicy{}, cronwire.NewService(), fswire.NewService())
	if service == nil || len(runner.outcomes) != 1 {
		t.Fatal("construction executed the supplied command runner")
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
	service := newTestService(
		zap.NewNop(), clock, &internalScriptPollerRunner{}, workflowID, "", nil, nil, factorydefinitioncomposition.WorkstationExecutionPolicy{},
		cronwire.NewService(),
		fswire.NewService(),
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
	service := newTestService(zap.NewNop(), clockwork.NewFakeClock(), &internalScriptPollerRunner{}, "", "", nil, nil, factorydefinitioncomposition.WorkstationExecutionPolicy{}, cronwire.NewService(), fswire.NewService())
	identity := automations.SourceIdentity{
		AutomationID: "workflow-start-barrier",
		SourceID:     runtimeSchedulerSourceID,
	}
	factoryConfig := &interfaces.FactoryConfig{}
	runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{
		Factory:     factoryConfig,
		FactoryPath: t.TempDir(),
	}
	configuration := sourcelifecycle.RuntimeSourceConfiguration{Snapshot: interfaces.RuntimeSnapshot{
		FactoryDir: runtimeConfig.FactoryDir(), EffectiveFactory: *factoryConfig,
	}, Inputs: automations.RuntimeActivationInputs{StartSchedulers: true, Submitter: func(context.Context, work.WorkRequest) error { return nil }}}
	configuration.Snapshot.Invocation.WorkflowID = identity.AutomationID
	if err := service.lifecycle.ConfigureRuntimeSource(context.Background(), configuration); err != nil {
		t.Fatal(err)
	}
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
	if _, err := service.lifecycle.Wait(cancelledCtx, effect); !errors.Is(err, context.Canceled) {
		t.Fatalf("observe before start effect = %v, want attached wait cancellation", err)
	}

	ctx, cancelSource := context.WithCancel(context.Background())
	if err := service.lifecycle.Start(ctx, sourcelifecycle.StartEffect{Kind: runtimeSchedulerSourceKind, Observation: effect.Observation}); err != nil {
		t.Fatalf("start scheduler source: %v", err)
	}
	observation, err := service.lifecycle.Wait(context.Background(), effect)
	if err != nil {
		t.Fatalf("observe initialized scheduler source: %v", err)
	}
	if observation.State != automations.ObservedLifecycleRunning {
		t.Fatalf("initialized observation = %q, want %q",
			observation.State, automations.ObservedLifecycleRunning)
	}
	cancelSource()
	if err := service.lifecycle.Stop(context.Background(), sourcelifecycle.StopEffect{Observation: effect.Observation}); err != nil {
		t.Fatal(err)
	}
}

func TestProductionRootUsesScriptPollersOwner(t *testing.T) {
	t.Parallel()

	service := newTestService(
		zap.NewNop(), clockwork.NewFakeClock(), &internalScriptPollerRunner{}, "workflow-script-pollers", "", nil, nil, factorydefinitioncomposition.WorkstationExecutionPolicy{},
		cronwire.NewService(),
		fswire.NewService(),
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
		outcomes: []internalScriptPollerOutcome{{result: platformprocess.CommandResult{Stdout: stdout}}},
	}
	submitted := &internalScriptPollerSubmitter{}
	service := newTestService(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		runner,
		workflowID,
		"",
		nil,
		nil,
		factorydefinitioncomposition.WorkstationExecutionPolicy{},
		cronwire.NewService(),
		fswire.NewService(),
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

	supervision := scriptpollers.SupervisionFor(workflowID, poller.Name)
	cursor, err := service.Root().GetCursor(
		context.Background(),
		automations.GetCursorRequest{InstanceID: supervision.InstanceID},
	)
	if err != nil {
		t.Fatalf("Root.GetCursor: %v", err)
	}
	if cursor.AutomationID != workflowID ||
		cursor.InstanceID != supervision.InstanceID ||
		string(cursor.Cursor) != "opaque-cursor-root" ||
		cursor.Checkpoint != "checkpoint-root" {
		t.Fatalf("Root.GetCursor = %+v, want committed opaque recovery facts for %q", cursor, supervision.InstanceID)
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
	result platformprocess.CommandResult
	err    error
}

type internalScriptPollerRunner struct {
	outcomes []internalScriptPollerOutcome
}

func (r *internalScriptPollerRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if len(r.outcomes) == 0 {
		return platformprocess.CommandResult{}, errors.New("no scripted outcomes")
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
	service := newTestService(
		zap.NewNop(), clockwork.NewFakeClock(), &internalScriptPollerRunner{}, workflowID, "", nil, nil, factorydefinitioncomposition.WorkstationExecutionPolicy{},
		cronwire.NewService(),
		fswire.NewService(),
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
	service := newTestService(zap.NewNop(), clockwork.NewFakeClock(), &internalScriptPollerRunner{}, "", "", nil, nil, factorydefinitioncomposition.WorkstationExecutionPolicy{}, cronwire.NewService(), fswire.NewService())
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

// F10b/j service-contract evidence: reads preserve the committed opaque facts
// and never execute the next command or admit another Work request.
func TestGetCursorPreservesOpaqueFactsWithoutCommandOrAdmission(t *testing.T) {
	t.Parallel()
	for _, durable := range []bool{false, true} {
		t.Run(fmt.Sprintf("durable=%t", durable), func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			dir := t.TempDir()
			base := ""
			if durable {
				base = dir
			}
			const cursor = "opaque:/雪\nembedded  spaces\t\"quoted\""
			const checkpoint = "checkpoint:{\"key\":\"é\"}\nopaque=value"
			output, err := json.Marshal(map[string]any{
				"requestId": "opaque-request", "type": "FACTORY_REQUEST_BATCH",
				"works":  []map[string]string{{"name": "opaque-work", "workTypeName": "task"}},
				"cursor": cursor, "checkpoint": checkpoint,
			})
			if err != nil {
				t.Fatal(err)
			}
			runner := &internalScriptPollerRunner{outcomes: []internalScriptPollerOutcome{
				{result: platformprocess.CommandResult{Stdout: output}}, {},
			}}
			service := newTestService(zap.NewNop(), clockwork.NewFakeClock(), runner, "opaque-workflow", base, nil, nil,
				factorydefinitioncomposition.WorkstationExecutionPolicy{}, cronwire.NewService(), fswire.NewService())
			poller, worker := internalCanonicalScriptPollerWorkstation(), internalCanonicalScriptPollerWorker()
			admissions := 0
			err = service.RunScriptPoller(ctx, runner, internalScriptPollerLoadedRuntimeConfig(t, dir, poller, worker), poller, worker,
				func(context.Context, work.WorkRequest) error { admissions++; return nil })
			if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
				t.Fatalf("poll cycle = %v, want terminal exit after commit", err)
			}
			request := automations.GetCursorRequest{InstanceID: scriptpollers.SupervisionFor("opaque-workflow", poller.Name).InstanceID}
			before, err := service.Root().GetCursor(ctx, request)
			if err != nil || before.AutomationID != "opaque-workflow" || before.InstanceID != request.InstanceID ||
				before.Cursor != cursor || before.Checkpoint != checkpoint {
				t.Fatalf("GetCursor = %+v, %v, want exact committed facts", before, err)
			}
			request.ExpectedCursor = "stale"
			_, err = service.Root().GetCursor(ctx, request)
			assertCursorReadError(t, err, automations.ErrorCodeConflict, automations.ErrConflict)
			request.ExpectedCursor = cursor
			after, err := service.Root().GetCursor(ctx, request)
			if err != nil || after != before {
				t.Fatalf("read after conflict = %+v, %v, want %+v", after, err, before)
			}
			if len(runner.outcomes) != 1 || admissions != 1 {
				t.Fatalf("reads caused effects: remaining commands=%d admissions=%d", len(runner.outcomes), admissions)
			}
		})
	}
}

// F10g component evidence uses the existing Root contract after a failed
// replacement. Public Work, diagnostics and peer progress need separate
// composed evidence; these direct admissions do not establish that journey.
func TestGetCursorAfterFailedReplacementPreservesPriorOpaqueFacts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	files := &cursorReplacementFaultFileSystem{}
	runner := &internalScriptPollerRunner{}
	service := newTestServiceWithCursorFileSystem(zap.NewNop(), clockwork.NewFakeClock(), runner, "opaque-replacement", dir, nil, nil,
		factorydefinitioncomposition.WorkstationExecutionPolicy{}, files, cronwire.NewService(), fswire.NewService())
	poller, worker := internalCanonicalScriptPollerWorkstation(), internalCanonicalScriptPollerWorker()
	config := internalScriptPollerLoadedRuntimeConfig(t, dir, poller, worker)
	admissions := 0
	const cursor = "prior:/雪\nembedded  spaces\t\"quoted\""
	const checkpoint = "prior:{\"key\":\"é\"}\nopaque=value"
	output, err := json.Marshal(map[string]any{
		"requestId": "prior-request", "type": "FACTORY_REQUEST_BATCH",
		"works":  []map[string]string{{"name": "prior-work", "workTypeName": "task"}},
		"cursor": cursor, "checkpoint": checkpoint,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner.outcomes = []internalScriptPollerOutcome{{result: platformprocess.CommandResult{Stdout: output}}}
	submit := func(context.Context, work.WorkRequest) error { admissions++; return nil }
	err = service.RunScriptPoller(ctx, runner, config, poller, worker, submit)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("initial poll = %v, want successful commit then terminal exit", err)
	}
	request := automations.GetCursorRequest{InstanceID: scriptpollers.SupervisionFor("opaque-replacement", poller.Name).InstanceID}
	before, err := service.Root().GetCursor(ctx, request)
	if err != nil || before.Cursor != cursor || before.Checkpoint != checkpoint {
		t.Fatalf("initial read = %+v, %v, want exact prior opaque facts", before, err)
	}
	files.err = errors.New("controlled cursor replacement failure")
	runner.outcomes = []internalScriptPollerOutcome{{result: platformprocess.CommandResult{Stdout: cursorPollerOutput("replacement")}}, {}}
	err = service.RunScriptPoller(ctx, runner, config, poller, worker, submit)
	var typed *automations.Error
	if !errors.As(err, &typed) || typed.Op != scriptpollers.CommitCursorOperation ||
		typed.Code != automations.ErrorCodeFailed || !errors.Is(err, files.err) {
		t.Fatalf("replacement poll = %v, want typed commit failure retaining error identity", err)
	}
	assertRetainedOpaqueCursorReads(t, service.Root(), request, before)
	if admissions != 2 || len(runner.outcomes) != 1 {
		t.Fatalf("reads caused effects: admissions=%d remaining commands=%d", admissions, len(runner.outcomes))
	}
}

func assertRetainedOpaqueCursorReads(t *testing.T, root automations.Root, request automations.GetCursorRequest, before automations.GetCursorResult) {
	t.Helper()
	ctx := context.Background()
	after, err := root.GetCursor(ctx, request)
	if err != nil || after != before {
		t.Fatalf("read after failed replacement = %+v, %v, want %+v", after, err, before)
	}
	request.ExpectedCursor = "cursor-replacement"
	_, err = root.GetCursor(ctx, request)
	assertCursorReadError(t, err, automations.ErrorCodeConflict, automations.ErrConflict)
	request.ExpectedCursor = before.Cursor
	after, err = root.GetCursor(ctx, request)
	if err != nil || after != before {
		t.Fatalf("read after stale conflict = %+v, %v, want %+v", after, err, before)
	}
}

type cursorReplacementFaultFileSystem struct {
	platformfilesystem.Local
	err error
}

func (f *cursorReplacementFaultFileSystem) Rename(from, to string) error {
	if f.err != nil {
		return f.err
	}
	return f.Local.Rename(from, to)
}

func newTestService(logger *zap.Logger, clock Clock, runner platformprocess.CommandRunner,
	workflowID, factoryDir string, hosted automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver, policy interfaces.WorkstationExecutionPolicyService,
	cronService cron.Service, watchers filesystemwatchers.Service) *Service {
	return newTestServiceWithCursorFileSystem(logger, clock, runner, workflowID, factoryDir,
		hosted, resolveTemplates, policy, nil, cronService, watchers)
}
func newTestServiceWithCursorFileSystem(logger *zap.Logger, clock Clock, runner platformprocess.CommandRunner,
	workflowID, factoryDir string, hosted automations.HostedPollers,
	resolveTemplates workers.TemplateFieldResolver, policy interfaces.WorkstationExecutionPolicyService,
	files cursorscopeswire.CursorPersistenceFileSystem, cronService cron.Service, watchers filesystemwatchers.Service) *Service {
	cursors := cursorscopeswire.NewService(files)
	pollers := scriptpollerswire.NewService(logger, clock, runner, resolveTemplates, policy, cursors)
	lifecycle := sourcelifecyclewire.NewService(logger, clock, pollers, cronService, watchers, hosted, cursors, policy)
	cursorBaseDir := ""
	if files != nil {
		cursorBaseDir = strings.TrimSpace(factoryDir)
	}
	return New(logger, clock, lifecycle, reconciliationwire.NewService(lifecycle), pollers,
		cronService, watchers, hosted, policy, cursors, files != nil, workflowID, factoryDir, cursorBaseDir)
}
