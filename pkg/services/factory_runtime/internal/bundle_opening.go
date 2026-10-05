package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// BundleOpening retains reusable worker and resource-opening behavior. Each
// operation owns its selected progress, worker boundary, recording and engine;
// opening a peer never replaces collaborators or state on this owner.
type BundleOpening struct {
	runtimeFactory  *RuntimeFactory
	workerService   workers.Service
	workerSessions  workersessions.Service
	workerAttempts  factory.WorkerAttemptOpener
	requestResolver *runtime.WorkstationRequestExecutor
}

func NewBundleOpening(
	runtimeFactory *RuntimeFactory,
	workerService workers.Service,
	workerSessions workersessions.Service,
	workerAttempts factory.WorkerAttemptOpener,
	requestResolver *runtime.WorkstationRequestExecutor,
) (*BundleOpening, error) {
	if runtimeFactory == nil {
		return nil, fmt.Errorf("Factory Runtime factory is required")
	}
	if workerSessions == nil {
		return nil, fmt.Errorf("worker sessions service is required")
	}
	if workerService == nil {
		return nil, fmt.Errorf("Workers service is required")
	}
	return &BundleOpening{runtimeFactory: runtimeFactory, workerService: workerService,
		workerSessions: workerSessions, workerAttempts: workerAttempts, requestResolver: requestResolver}, nil
}

// Open applies fixed opening behavior to one runtime selection. The retained
// compatibility adapter also uses this operation until its caller migration.
func (opening *BundleOpening) Open(
	ctx context.Context,
	spec runtimebuild.SessionBuildSpec,
	runtimeLogDir string,
	runtimeLogConfig factory.RuntimeLogStorageConfig,
	runtimeFileLoggingPolicy RuntimeFileLoggingPolicy,
	runtimeMetricsPolicy RuntimeMetricsPolicy,
	runtimeMetricsDir string,
	runtimeMetricsConfig factory.RuntimeMetricsStorageConfig,
	recordFlushInterval time.Duration,
	defaultSessionID string,
	runtimeMode factorydefinitions.RuntimeMode,
	runtimeScheduler scheduler.Scheduler,
	inlineDispatch bool,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	backendScopeID string,
	factoryRunnerID string,
	verbose bool,
	skipBuiltInPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	mockWorkersConfig *workers.MockWorkersConfig,
	providerSessionProgress workers.ProgressPublisher,
	dispatchCompleted func(string),
	worldStateProjector factory.WorldStateProjector,
	recordingsRuntime recordings.RuntimeScopeService,
	initialFactorySnapshot InitialFactorySnapshotFactory,
) (*factoryhost.Bundle, error) {
	loadedFactoryCfg, sessionID, initialFactory, err := resolveBundleInputs(
		spec, defaultSessionID, recordingsRuntime, initialFactorySnapshot,
	)
	if err != nil {
		return nil, err
	}
	// Progress callbacks outlive opening admission while retaining its values.
	providerSessionProgress = workersessions.RuntimeProgressPublisher(opening.workerAttempts.PublishRuntimeProgress).ForRuntime(context.WithoutCancel(ctx), spec.RuntimeInstanceID, providerSessionProgress)
	metricsSessionID := firstNonEmptySessionID(spec.MetricsSessionID, sessionID)
	workerServiceWithProgress := newRuntimeWorkersService(
		opening.workerService, providerSessionProgress, spec, sessionID,
		skipBuiltInPrerequisiteValidation, invocationSkipPermissionsOverride,
		opening.requestResolver, mockWorkersConfig,
	)
	bundle, err := opening.runtimeFactory.Build(
		ctx,
		spec.Dir,
		spec.FolderPath,
		sessionID,
		metricsSessionID,
		factoryRunnerID,
		runtimeMode,
		verbose,
		runtimeScheduler,
		inlineDispatch,
		submissionRecorder,
		dispatchRecorder,
		runtimeLogDir,
		runtimeLogConfig, runtimeFileLoggingPolicy,
		runtimeMetricsPolicy,
		runtimeMetricsDir,
		runtimeMetricsConfig,
		loadedFactoryCfg,
		spec.RuntimeInstanceID,
		strings.TrimSpace(backendScopeID),
		spec.Clock,
		spec.RecordPath,
		initialFactory,
		spec.RestoredWorldState,
		spec.SkipRestoredDispatchReconciliation,
		spec.SubmissionHooks,
		spec.CompletionPlanner,
		spec.PetriMutationRecorder,
		worldStateProjector,
		runtimeScopeWithFlush{
			RuntimeScopeService:   recordingsRuntime,
			flushInterval:         recordFlushInterval,
			resumeCanonicalEvents: cloneFactoryEvents(spec.ResumeCanonicalEvents),
		},
		workerServiceWithProgress,
		runtimeWorkerSessionBoundary{Service: opening.workerSessions, opener: opening.workerAttempts, execution: workerServiceWithProgress, clock: spec.Clock, scheduler: opening.runtimeFactory.workerAttemptScheduler, runtimeID: spec.RuntimeInstanceID},
		opening.workerAttempts,
		dispatchCompleted,
		mockWorkersConfig,
	)
	if err != nil {
		return bundle, err
	}
	setReplayEvents(bundle.Factory, spec.ReplayEvents)
	setBundleProgressPublisher(bundle, providerSessionProgress)
	return bundle, nil
}
