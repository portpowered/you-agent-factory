package service

import (
	"context"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/replayhooks"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service/host"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"go.uber.org/zap"
)

// Assembly owns the product-policy dependencies used to assemble each
// session-owned Factory Runtime.
type Assembly struct {
	runtimeFactory *RuntimeFactory
}

// NewAssembly constructs the inert Factory Runtime assembly service selected
// by Wire. It does not start a runtime or sidecar.
func NewAssembly(runtimeFactory *RuntimeFactory) (*Assembly, error) {
	if runtimeFactory == nil {
		return nil, fmt.Errorf("Factory Runtime factory is required")
	}
	return &Assembly{runtimeFactory: runtimeFactory}, nil
}

// Assemble creates one session-owned runtime from invocation values and the
// product-policy dependencies already selected by Wire.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func (a *Assembly) Assemble(
	ctx context.Context,
	defaultWorkerModelProvider string,
	defaultWorkerModel string,
	applyOperatorDefaults bool,
	recordPath string,
	workflowID string,
	defaultSessionID string,
	workstationLoader factorydefinitions.WorkstationLoader,
	loadFactory factoryruntime.LoadedFactoryLoader,
	providerOverride workerprovider.Provider,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	mockWorkersConfig *workers.MockWorkersConfig,
	runtimeMode factorydefinitions.RuntimeMode,
	runtimeScheduler factoryruntime.Scheduler,
	workerExecutors map[string]workers.WorkerExecutor,
	workerExecutorDecorator func(string, workers.WorkerExecutor) workers.WorkerExecutor,
	inlineDispatch bool,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	runtimeLogDir string,
	runtimeLogConfig factoryruntime.RuntimeLogStorageConfig,
	runtimeFileLoggingPolicy factoryruntime.RuntimeFileLoggingPolicy,
	runtimeMetricsPolicy factoryruntime.RuntimeMetricsPolicy,
	runtimeMetricsDir string,
	runtimeMetricsConfig factoryruntime.RuntimeMetricsStorageConfig,
	recordFlushInterval time.Duration,
	backendScopeID string,
	factoryRunnerID string,
	verbose bool,
	skipRunnerPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	clock factoryruntime.Clock,
	baseLogger *zap.Logger,
	workerExecution workers.RuntimeService,
	runtimeExecutorsFactory factoryruntime.WorkersRuntimeExecutorsFactory,
	mockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	progressFactory func(string) workers.ProgressPublisher,
	completionFactory func(string) func(string),
	petriMutationRecorder factoryruntime.PetriMutationRecorder,
	worldStateProjector factoryruntime.WorldStateProjector,
	runtimeLedgerFactory factoryruntime.RuntimeLedgerFactory,
	runtimeRecorderFactory recordings.RuntimeRecorderFactory,
	initialFactorySnapshot factorydefinitions.InitialFactorySnapshotFactory,
	dir string,
	factoryRootDir string,
	executionBaseDir string,
	loadedFactory factorydefinitions.MutableLoadedFactorySource,
	runtimeInstanceID string,
	replayArtifact *factorydefinitions.ReplayArtifact,
	replayExecutionFactory recordings.ReplayExecutionFactory,
	automationService automations.RuntimeScheduler,
	serviceMode bool,
) (
	factoryruntime.ReplacementBuilder,
	factoryruntime.HostedInstance,
	factoryruntime.SessionBuildSpec,
	factoryruntime.Lifecycle,
	factoryruntime.Sidecars,
	error,
) {
	if a == nil || a.runtimeFactory == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil,
			fmt.Errorf("Factory Runtime assembly service is required")
	}
	builder, err := NewRuntimeBuild(
		defaultWorkerModelProvider,
		defaultWorkerModel,
		applyOperatorDefaults,
		recordPath,
		workflowID,
		defaultSessionID,
		workstationLoader,
		providerOverride,
		providerCommandRunner,
		scriptCommandRunner,
		mockWorkersConfig,
		runtimeMode,
		runtimeScheduler,
		workerExecutors,
		workerExecutorDecorator,
		inlineDispatch,
		submissionRecorder,
		dispatchRecorder,
		runtimeLogDir,
		runtimeLogConfig,
		runtimeFileLoggingPolicy,
		runtimeMetricsPolicy,
		runtimeMetricsDir,
		runtimeMetricsConfig,
		recordFlushInterval,
		backendScopeID,
		factoryRunnerID,
		verbose,
		skipRunnerPrerequisiteValidation,
		invocationSkipPermissionsOverride,
		clock,
		baseLogger,
		a.runtimeFactory,
		workerExecution,
		runtimeExecutorsFactory,
		mockCommandRunnerFactory,
		progressFactory,
		completionFactory,
		petriMutationRecorder,
		worldStateProjector,
		runtimeLedgerFactory,
		runtimeRecorderFactory,
		loadFactory,
		initialFactorySnapshot,
	)
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	if replayExecutionFactory == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, fmt.Errorf(
			"Recordings replay execution factory is required",
		)
	}
	replayProvider, replayCommandRunner, replayHooks, completionPlanner, err := replayExecutionFactory(
		replayArtifact,
	)
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	spec, err := builder.BuildSpec(
		ctx,
		dir,
		factoryRootDir,
		defaultSessionID,
		executionBaseDir,
		loadedFactory,
		runtimeInstanceID,
		replayProvider,
		replayCommandRunner,
		replayhooks.Adapt(replayHooks),
		completionPlanner,
		true,
	)
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	instance, err := builder.Build(ctx, spec)
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	if instance == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, fmt.Errorf(
			"default runtime instance is required",
		)
	}
	lifecycle, err := factoryhost.NewLifecycleService(clock)
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	return builder,
		instance,
		spec,
		lifecycle,
		NewRuntimeSidecars(automationService, serviceMode),
		nil
}
