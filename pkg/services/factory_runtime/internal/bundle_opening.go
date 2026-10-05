package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// BundleOpeningOperation is the scoped resource-opening capability consumed by
// assembly. Concrete host state remains private to Runtime.
type BundleOpeningOperation func(
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
	backendScopeID string,
	factoryRunnerID string,
	verbose bool,
	skipBuiltInPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	mockWorkersConfig *workers.MockWorkersConfig,
	providerSessionProgress workers.ProgressPublisher,
	dispatchCompleted func(string),
) (factory.RuntimeRecord, error)

// runtimeResourceOpening is fixed resource-opening behavior selected at construction.
// Its concrete result stays private; consumers receive RuntimeRecord.
type runtimeResourceOpening func(
	ctx context.Context,
	baseLogger *zap.Logger,
	dir string,
	folderPath string,
	sessionID string,
	metricsSessionID string,
	runnerID string,
	runtimeMode factorydefinitions.RuntimeMode,
	verbose bool,
	runtimeScheduler scheduler.Scheduler,
	inlineDispatch bool,
	runtimeLogDir string,
	runtimeLogConfig factory.RuntimeLogStorageConfig,
	runtimeFileLoggingPolicy RuntimeFileLoggingPolicy,
	runtimeMetricsPolicy RuntimeMetricsPolicy,
	runtimeMetricsDir string,
	runtimeMetricsConfig factory.RuntimeMetricsStorageConfig,
	loadedFactoryCfg factory.LoadedConfig,
	runtimeInstanceID string,
	backendScopeID string,
	clock factory.Clock,
	recordPath string,
	initialFactory *factorydefinitions.FactorySnapshot,
	restoredWorldState *factorydefinitions.FactoryWorldState,
	skipRestoredDispatchReconciliation bool,
	submissionHooks []factory.SubmissionHook,
	completionPlanner factory.CompletionDeliveryPlanner,
	petriMutationRecorder factory.PetriMutationRecorder,
	recordingsRuntime recordings.RuntimeScopeService,
	workerService workers.Service,
	workerSessions workersessions.Service,
	workerAttempts factory.WorkerAttemptOpener,
	dispatchCompleted func(string),
	mockWorkersConfigs ...*workers.MockWorkersConfig,
) (*factoryhost.Bundle, error)

// BundleOpening retains reusable worker and resource-opening behavior. Each
// operation owns its selected progress, worker boundary, recording and engine;
// opening a peer never replaces collaborators or state on this owner.
type BundleOpening struct {
	runtimeBuild           runtimeResourceOpening
	workerAttemptScheduler platformclock.TimerSource
	workerService          workers.Service
	workerSessions         workersessions.Service
	workerAttempts         factory.WorkerAttemptOpener
	requestResolver        *runtime.WorkstationRequestExecutor
	recordingsRuntime      recordings.RuntimeScopeService
	initialFactorySnapshot InitialFactorySnapshotFactory
}

func NewBundleOpening(
	runtimeBuild runtimeResourceOpening,
	workerAttemptScheduler platformclock.TimerSource,
	workerService workers.Service,
	workerSessions workersessions.Service,
	workerAttempts factory.WorkerAttemptOpener,
	requestResolver *runtime.WorkstationRequestExecutor,
	recordingsRuntime recordings.RuntimeScopeService,
	initialFactorySnapshot InitialFactorySnapshotFactory,
) (*BundleOpening, error) {
	if runtimeBuild == nil {
		return nil, fmt.Errorf("factory runtime resource opening is required")
	}
	if workerSessions == nil {
		return nil, fmt.Errorf("worker sessions service is required")
	}
	if workerService == nil {
		return nil, fmt.Errorf("workers service is required")
	}
	return &BundleOpening{runtimeBuild: runtimeBuild, workerAttemptScheduler: workerAttemptScheduler, workerService: workerService,
		workerSessions: workerSessions, workerAttempts: workerAttempts, requestResolver: requestResolver,
		recordingsRuntime: recordingsRuntime, initialFactorySnapshot: initialFactorySnapshot}, nil
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
	backendScopeID string,
	factoryRunnerID string,
	verbose bool,
	skipBuiltInPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	mockWorkersConfig *workers.MockWorkersConfig,
	providerSessionProgress workers.ProgressPublisher,
	dispatchCompleted func(string),
) (factory.RuntimeRecord, error) {
	loadedFactoryCfg, sessionID, initialFactory, err := resolveBundleInputs(
		spec, defaultSessionID, opening.recordingsRuntime, opening.initialFactorySnapshot,
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
	bundle, err := opening.runtimeBuild(
		ctx,
		spec.BaseLogger,
		spec.Dir,
		spec.FolderPath,
		sessionID,
		metricsSessionID,
		factoryRunnerID,
		runtimeMode,
		verbose,
		runtimeScheduler,
		inlineDispatch,
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
		runtimeScopeWithFlush{
			RuntimeScopeService:   opening.recordingsRuntime,
			flushInterval:         recordFlushInterval,
			resumeCanonicalEvents: cloneFactoryEvents(spec.ResumeCanonicalEvents),
		},
		workerServiceWithProgress,
		runtimeWorkerSessionBoundary{Service: opening.workerSessions, opener: opening.workerAttempts, execution: workerServiceWithProgress, clock: spec.Clock, scheduler: opening.workerAttemptScheduler, runtimeID: spec.RuntimeInstanceID},
		opening.workerAttempts,
		dispatchCompleted,
		mockWorkersConfig,
	)
	if err != nil {
		if bundle == nil {
			return nil, err
		}
		return bundle, err
	}
	setReplayEvents(bundle.Factory, spec.ReplayEvents)
	setBundleProgressPublisher(bundle, providerSessionProgress)
	return bundle, nil
}
