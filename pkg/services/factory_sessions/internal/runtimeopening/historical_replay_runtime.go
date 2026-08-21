package runtimeopening

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// historicalReplayProcessRuntime completes the process lifecycle for an
// inspection-only portable recording. It intentionally starts neither a live
// Factory runtime nor worker sidecars nor an HTTP host.
type historicalReplayProcessRuntime struct{}

func (historicalReplayProcessRuntime) Start(context.Context, context.Context) error { return nil }

func (historicalReplayProcessRuntime) StartWorkers(context.Context) (factorysessions.RuntimeStop, error) {
	return func(context.Context) error { return nil }, nil
}

func (historicalReplayProcessRuntime) RunTransport(context.Context, http.Handler) error { return nil }

func (historicalReplayProcessRuntime) Stop(context.Context) error { return nil }

type portableReplayDurableOwner struct {
	durableexecution.Service
	prepareOnce sync.Once
	prepare     func(context.Context) error
	prepareErr  error
}

func (owner *portableReplayDurableOwner) HasRestorableState(
	ctx context.Context,
	sessionID string,
) (bool, error) {
	if owner == nil || owner.Service == nil {
		return false, nil
	}
	probe, ok := owner.Service.(interface {
		HasRestorableState(context.Context, string) (bool, error)
	})
	if !ok {
		return false, nil
	}
	available, err := probe.HasRestorableState(ctx, sessionID)
	if err != nil || !available {
		return available, err
	}
	owner.prepareOnce.Do(func() {
		if owner.prepare != nil {
			owner.prepareErr = owner.prepare(ctx)
		}
	})
	if owner.prepareErr != nil {
		return false, owner.prepareErr
	}
	return true, nil
}

type portableReplayRuntimeCleanup struct {
	mu       sync.Mutex
	runtime  runtimeports.RuntimeInstance
	closed   bool
	closeErr error
}

func newPortableReplayRuntimeCleanup() *portableReplayRuntimeCleanup {
	return &portableReplayRuntimeCleanup{}
}

func (cleanup *portableReplayRuntimeCleanup) Set(runtime runtimeports.RuntimeInstance) {
	if cleanup == nil || runtime == nil {
		return
	}
	cleanup.mu.Lock()
	defer cleanup.mu.Unlock()
	if cleanup.closed {
		if cleanup.closeErr == nil {
			cleanup.closeErr = runtime.CloseArtifacts()
		}
		return
	}
	cleanup.runtime = runtime
}

func (cleanup *portableReplayRuntimeCleanup) Close() error {
	if cleanup == nil {
		return nil
	}
	cleanup.mu.Lock()
	defer cleanup.mu.Unlock()
	if cleanup.closed {
		return cleanup.closeErr
	}
	cleanup.closed = true
	if cleanup.runtime != nil {
		cleanup.closeErr = cleanup.runtime.CloseArtifacts()
		cleanup.runtime = nil
	}
	return cleanup.closeErr
}

// openPortableReplayDurableOwner constructs the existing durable execution
// owner for a checkpoint-bearing replay. Runtime assembly is deferred until
// HasRestorableState confirms that the durable owner can actually resume; a
// public checkpoint summary alone must remain inspection-only.
func openPortableReplayDurableOwner(
	configured preparedRuntime,
	root RuntimeRoot,
	logger *zap.Logger,
	clockEdge factoryruntime.Clock,
	providerOverride providers.Service,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	workerService workers.Service,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	providerFromCommandRunnerFactory ProviderFromCommandRunnerFactory,
	durableExecutionFactory DurableExecutionFactory,
	factorySessionExecutionFactory FactorySessionExecutionFactory,
	providerIdentities factorysessions.ProviderIdentityResolver,
	resolveClock factoryruntime.ClockResolver,
	factoryRuntimeAssembler FactoryRuntimeAssembler,
	recordingsRuntime recordings.RuntimeOpening,
	initialFactorySnapshotFactory factorydefinitions.InitialFactorySnapshotFactory,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	automationService automations.Service,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
) (durableexecution.Service, func() error, error) {
	if durableExecutionFactory == nil {
		return nil, nil, fmt.Errorf("construct portable replay runtime: durable execution operation is required")
	}
	clock, err := clockForReplay(clockEdge, nil, nil, resolveClock)
	if err != nil {
		return nil, nil, err
	}
	durable, providerForDurable, err := constructPortableReplayDurableOwner(
		configured,
		root,
		clock,
		providerOverride,
		providerCommandRunner,
		workersMockCommandRunnerFactory,
		providerFromCommandRunnerFactory,
		durableExecutionFactory,
		factorySessionExecutionFactory,
		providerIdentities,
	)
	if err != nil {
		return nil, nil, err
	}
	if durable.Service == nil {
		return nil, nil, fmt.Errorf("construct portable replay runtime: durable execution owner is required")
	}
	cleanup := newPortableReplayRuntimeCleanup()
	owner := &portableReplayDurableOwner{
		Service: durable.Service,
		prepare: func(probeContext context.Context) error {
			runtime, err := preparePortableReplayRuntime(
				probeContext,
				configured,
				root,
				clock,
				logger,
				factoryRuntimeAssembler,
				durable.Service,
				workerService,
				providerForDurable,
				providerOverride,
				providerCommandRunner,
				scriptCommandRunner,
				workersMockCommandRunnerFactory,
				submissionRecorder,
				dispatchRecorder,
				recordingsRuntime,
				initialFactorySnapshotFactory,
				loadFactory,
				automationService,
			)
			if err != nil {
				return err
			}
			cleanup.Set(runtime)
			return nil
		},
	}
	return owner, cleanup.Close, nil
}

func preparePortableReplayRuntime(
	ctx context.Context,
	configured preparedRuntime,
	root RuntimeRoot,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	factoryRuntimeAssembler FactoryRuntimeAssembler,
	durableOwner durableexecution.Service,
	workerService workers.Service,
	providerForDurable providers.Service,
	providerOverride providers.Service,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	recordingsRuntime recordings.RuntimeOpening,
	initialFactorySnapshotFactory factorydefinitions.InitialFactorySnapshotFactory,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	automationService automations.Service,
) (runtimeports.RuntimeInstance, error) {
	runtime, err := assemblePortableReplayRuntime(
		ctx,
		configured,
		root,
		clock,
		logger,
		factoryRuntimeAssembler,
		durableOwner,
		providerOverride,
		providerCommandRunner,
		scriptCommandRunner,
		workersMockCommandRunnerFactory,
		submissionRecorder,
		dispatchRecorder,
		recordingsRuntime,
		initialFactorySnapshotFactory,
		loadFactory,
		automationService,
	)
	if err != nil {
		return nil, err
	}
	runtimeService := runtime.RuntimeService()
	var resourceLeaseAdmission factoryruntime.ResourceCapacityLeaseAdmission
	if admission, ok := runtimeService.(factoryruntime.ResourceCapacityLeaseAdmission); ok {
		resourceLeaseAdmission = admission
	}
	if err := bindDurableExecutionCapabilities(
		configured.Session.FactorySessionID,
		durableOwner,
		workerService,
		runtimeService,
		resourceLeaseAdmission,
		configured.Runtime.RuntimeInstanceID,
		runtime.StreamGeneration(),
		providerForDurable,
		configured.Workers.MockWorkers,
		providerCommandRunner,
		runtimeProgressPublisher(runtime),
		runtimeWorkerAttemptStarter(runtime),
	); err != nil {
		runtime.CloseArtifacts()
		return nil, err
	}
	return runtime, nil
}

func constructPortableReplayDurableOwner(
	configured preparedRuntime,
	root RuntimeRoot,
	clock factoryruntime.Clock,
	providerOverride providers.Service,
	providerCommandRunner workers.CommandRunner,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	providerFromCommandRunnerFactory ProviderFromCommandRunnerFactory,
	durableExecutionFactory DurableExecutionFactory,
	factorySessionExecutionFactory FactorySessionExecutionFactory,
	providerIdentities factorysessions.ProviderIdentityResolver,
) (DurableExecution, providers.Service, error) {
	providerForDurable, err := resolveDurableExecutionProvider(
		providerOverride,
		configured.Workers.MockWorkers,
		nil,
		providerCommandRunner,
		workersMockCommandRunnerFactory,
		providerFromCommandRunnerFactory,
	)
	if err != nil {
		return DurableExecution{}, nil, err
	}
	durable, err := durableExecutionFactory(
		configured.Definition,
		configured.Session,
		configured.OperatorDefaults,
		root,
		clock,
		providerForDurable,
		configured.Workers.MockWorkers,
		factorySessionExecutionFactory,
		providerIdentities,
	)
	if err != nil {
		return DurableExecution{}, nil, err
	}
	return durable, providerForDurable, nil
}

func assemblePortableReplayRuntime(
	ctx context.Context,
	configured preparedRuntime,
	root RuntimeRoot,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	factoryRuntimeAssembler FactoryRuntimeAssembler,
	durableOwner durableexecution.Service,
	providerOverride providers.Service,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	recordingsRuntime recordings.RuntimeOpening,
	initialFactorySnapshotFactory factorydefinitions.InitialFactorySnapshotFactory,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	automationService automations.Service,
) (runtimeports.RuntimeInstance, error) {
	if factoryRuntimeAssembler == nil {
		return nil, fmt.Errorf("construct portable replay runtime: Factory Runtime assembler is required")
	}
	if recordingsRuntime == nil {
		return nil, fmt.Errorf("construct portable replay runtime: Recordings runtime opening is required")
	}
	projection := recordingsRuntime.Projection()
	if projection == nil {
		return nil, fmt.Errorf("construct portable replay runtime: Recordings projection is unavailable")
	}
	mutationOwner, ok := durableOwner.(interface {
		RecordPetriTokenMutations(string, []factorydefinitions.TokenMutationRecord) error
	})
	if !ok {
		return nil, fmt.Errorf("construct portable replay runtime: durable execution owner does not record Petri mutations")
	}
	progressFactory := fanOutWorkerProgress(nil, durableOwner)
	_, runtime, _, _, _, err := factoryRuntimeAssembler.Assemble(
		ctx,
		configured.OperatorDefaults.WorkerModelProvider,
		configured.OperatorDefaults.WorkerModel,
		false,
		configured.Recordings.RecordPath,
		configured.Recordings.WorkflowID,
		configured.Session.FactorySessionID,
		nil,
		loadFactory,
		providerOverride,
		providerCommandRunner,
		scriptCommandRunner,
		configured.Workers.MockWorkers,
		configured.Runtime.Mode,
		factoryruntime.Scheduler(nil),
		false,
		submissionRecorder,
		dispatchRecorder,
		configured.Runtime.LogDirectory,
		configured.Runtime.LogConfig,
		factoryruntime.RuntimeFileLoggingPolicy(configured.Runtime.FileLoggingPolicy),
		factoryruntime.RuntimeMetricsPolicy(configured.Runtime.MetricsPolicy),
		configured.Runtime.MetricsDirectory,
		configured.Runtime.MetricsConfig,
		configured.Recordings.FlushInterval,
		configured.Session.BackendScopeID,
		configured.Workers.RunnerID,
		configured.Runtime.Verbose,
		configured.Workers.SkipBuiltInPrerequisiteValidation,
		configured.Workers.InvocationSkipPermissionsOverride,
		clock,
		logger,
		workersMockCommandRunnerFactory,
		progressFactory,
		nil,
		mutationOwner.RecordPetriTokenMutations,
		projection.ReconstructFactoryWorldState,
		recordingsRuntime,
		initialFactorySnapshotFactory,
		configured.Definition.Directory,
		root.FactoryRootDir,
		configured.Definition.ExecutionBaseDir,
		nil,
		configured.Runtime.RuntimeInstanceID,
		nil,
		nil,
		automationService,
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("construct portable replay runtime: %w", err)
	}
	if runtime == nil {
		return nil, fmt.Errorf("construct portable replay runtime: runtime instance is required")
	}
	return runtime, nil
}

func bindDurableExecutionCapabilities(
	sessionID string,
	execution durableexecution.Service,
	workerService workers.Service,
	invoker factoryruntime.Service,
	admission factoryruntime.ResourceCapacityLeaseAdmission,
	runtimeID string,
	generationID string,
	providerOverride providers.Service,
	mockWorkers *workers.MockWorkersConfig,
	commandRunner workers.CommandRunner,
	progressPublisher workers.ProgressPublisher,
	attemptStarter func(context.Context, workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error),
) error {
	setWorkerInvoker(execution, invoker)
	if err := setWorkerExecution(
		sessionID,
		execution,
		workerService,
		admission,
		runtimeID,
		generationID,
		providerOverride,
		mockWorkers,
		commandRunner,
	); err != nil {
		return err
	}
	setWorkerProgressPublisher(execution, progressPublisher)
	setWorkerAttemptStarter(execution, attemptStarter)
	return nil
}
