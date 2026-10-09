package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/modelinvocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"go.uber.org/zap"
)

func recoveryRecordingID(recordingID string) string {
	recordingID = strings.TrimSpace(recordingID)
	if recordingID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(recordingID))
	return "sha256:" + hex.EncodeToString(digest[:])
}

// runtimeProducts is the invocation-local output of Factory Runtime assembly.
// It is consumed by canonical Start and never stored as another session graph.
type runtimeProducts struct {
	startupRecovery        *factorysessions.StartupRecovery
	process                roles.ProcessRuntime
	lifecycle              roles.LifecycleRuntime
	replayExecution        *recordingreplay.Scope
	workerSettings         *factoryruntime.JavaScriptWorkerSettings
	modelInvocation        modelinvocation.RuntimeModelInvocation
	factoryRuntime         factoryruntime.Service
	modelsScope            models.RuntimeScopeRef
	workerSessions         workersessions.ObservationService
	clock                  factoryruntime.Clock
	logger                 *zap.Logger
	diagnostics            factoryruntime.RuntimeLogDiagnostics
	directory              string
	runtimeInstanceID      string
	backendScopeID         string
	currentBoardRecordPath string
	operatorSettingsPath   string
	orderlyStop            func(context.Context) error
	closeArtifacts         func() error
	historicalReplay       *factorysessions.HistoricalReplayInspection
	skippedBoardRecordings []string
	replayMetadataWarnings []recordings.MetadataMismatchWarning
	resumeRecoveryMetadata *recordings.ResumeRecoveryMetadata
	bindRuntime            func(factoryruntime.RuntimeBinding) error
	engine                 factoryruntime.Service
	activation             *factoryruntime.RuntimeActivation
}

type workerSessionsObservationProvider interface {
	WorkerSessionsObservation() workersessions.ObservationService
}

type workerSessionsObservationForSessionProvider interface {
	WorkerSessionsObservationForSession(string) workersessions.ObservationService
}

func (r *Root) historicalReplayRuntimeProducts(
	logger *zap.Logger,
	projection recordingreplay.RecordingReplayProjection,
	liveOwner durableexecution.Service,
	closeResources func() error,
) runtimeProducts {
	if closeResources == nil {
		closeResources = func() error { return nil }
	}
	replay := r.replayBehavior.Acquire(projection, liveOwner)
	inspection := replay.Inspection()
	return runtimeProducts{
		process:          historicalReplayProcessRuntime{},
		historicalReplay: &inspection,
		logger:           logger,
		closeArtifacts:   closeResources,
		replayExecution:  replay,
	}
}

func runtimeBindingForSession(
	factoryRuntime factoryruntime.Service,
	factorySessionID string,
) func(factoryruntime.RuntimeBinding) error {
	if strings.TrimSpace(factorySessionID) == "" {
		return nil
	}
	binder, ok := factoryRuntime.(interface {
		BindRuntime(string, factoryruntime.RuntimeBinding) error
	})
	if !ok {
		return nil
	}
	return func(binding factoryruntime.RuntimeBinding) error {
		return binder.BindRuntime(factorySessionID, binding)
	}
}

func openedWorkerSessionsObservation(
	factoryRuntime factoryruntime.Service,
	startup runtimeports.RuntimeInstance,
	effectiveFactorySessionID string,
) workersessions.ObservationService {
	// The process Factory Sessions root resolves its current selected runtime,
	// which is not necessarily the runtime being opened here. Prefer the
	// session's freshly assembled runtime instance so its observation decorator
	// retains the matching Worker Sessions registry and canonical event ledger.
	observationRuntime := factoryRuntime
	if startup != nil && startup.RuntimeService() != nil {
		observationRuntime = startup.RuntimeService()
	}
	if provider, ok := observationRuntime.(workerSessionsObservationForSessionProvider); ok {
		return provider.WorkerSessionsObservationForSession(effectiveFactorySessionID)
	}
	if provider, ok := observationRuntime.(workerSessionsObservationProvider); ok {
		return provider.WorkerSessionsObservation()
	}
	return nil
}
