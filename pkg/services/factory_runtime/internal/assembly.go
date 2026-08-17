package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/replayhooks"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// Assembly owns the product-policy dependencies used to assemble each
// session-owned Factory Runtime.
type Assembly struct {
	runtimeFactory        *RuntimeFactory
	workerSessionsFactory factoryruntime.WorkerSessionsFactory
	workerService         workers.Service
}

// NewAssembly constructs the inert Factory Runtime assembly service selected
// by Wire. It does not start a runtime or sidecar. workerSessionsFactory is
// the one directly injected per-session Worker Sessions construction path
// (W4 dispatch cutover); Wire owns composing it over worker_sessions/wire so
// Factory Runtime never imports that peer service's wire package directly.
func NewAssembly(
	runtimeFactory *RuntimeFactory,
	workerSessionsFactory factoryruntime.WorkerSessionsFactory,
	workerService workers.Service,
) (*Assembly, error) {
	if runtimeFactory == nil {
		return nil, fmt.Errorf("Factory Runtime factory is required")
	}
	if workerSessionsFactory == nil {
		return nil, fmt.Errorf("Worker Sessions factory is required")
	}
	if workerService == nil {
		return nil, fmt.Errorf("Workers service is required")
	}
	return &Assembly{
		runtimeFactory: runtimeFactory, workerSessionsFactory: workerSessionsFactory,
		workerService: workerService,
	}, nil
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
	providerOverride providers.Service,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	mockWorkersConfig *workers.MockWorkersConfig,
	runtimeMode factorydefinitions.RuntimeMode,
	runtimeScheduler factoryruntime.Scheduler,
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
	clock factoryruntime.Clock,
	baseLogger *zap.Logger,
	mockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	progressFactory func(string) workers.ProgressPublisher,
	completionFactory func(string) func(string),
	petriMutationRecorder factoryruntime.PetriMutationRecorder,
	worldStateProjector factoryruntime.WorldStateProjector,
	recordingsRuntime recordings.RuntimeOpening,
	initialFactorySnapshot factorydefinitions.InitialFactorySnapshotFactory,
	dir string,
	factoryRootDir string,
	executionBaseDir string,
	loadedFactory factorydefinitions.MutableLoadedFactorySource,
	runtimeInstanceID string,
	replayArtifact *factorydefinitions.ReplayArtifact,
	automationService automations.Service,
	serviceMode bool,
) (
	factoryruntime.RuntimeReplacementBuilder,
	factoryruntime.RuntimeRecord,
	factoryruntime.SessionBuildSpec,
	factoryruntime.RuntimeLifecycle,
	factoryruntime.RuntimeSidecars,
	error,
) {
	if a == nil || a.runtimeFactory == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil,
			fmt.Errorf("Factory Runtime assembly service is required")
	}
	if a.workerService == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil,
			fmt.Errorf("Workers service is required")
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
		clock,
		baseLogger,
		a.runtimeFactory,
		a.workerService,
		a.workerSessionsFactory,
		mockCommandRunnerFactory,
		progressFactory,
		completionFactory,
		petriMutationRecorder,
		worldStateProjector,
		recordingsRuntime,
		loadFactory,
		initialFactorySnapshot,
	)
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	if recordingsRuntime == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, fmt.Errorf(
			"Recordings runtime opening is required",
		)
	}
	replayProvider, replayCommandRunner, replayHooks, completionPlanner, err := recordingsRuntime.ReplayExecution(
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
	spec.ReplayEvents = cloneReplayArtifactEvents(replayArtifact)
	instance, err := builder.Build(ctx, spec)
	if err != nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, err
	}
	if instance == nil {
		return nil, nil, factoryruntime.SessionBuildSpec{}, nil, nil, fmt.Errorf(
			"default runtime instance is required",
		)
	}
	attachInvocationScheduleFactory(ctx, automationService, instance)
	lifecycle, err := instancehostwire.New(instancehost.Dependencies{Clock: clock})
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

func cloneReplayArtifactEvents(artifact *factorydefinitions.ReplayArtifact) []factorydefinitions.FactoryEvent {
	if artifact == nil || len(artifact.Events) == 0 {
		return nil
	}
	events := make([]factorydefinitions.FactoryEvent, len(artifact.Events))
	for index, event := range artifact.Events {
		events[index] = event.Clone()
	}
	return events
}
