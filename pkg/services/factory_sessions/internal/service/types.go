package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/modelinvocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
	process                roles.ProcessRuntime
	lifecycle              roles.LifecycleRuntime
	sessions               roles.SessionGateway
	liveControl            factorysessions.LiveControlService
	execution              durableexecution.Service
	workerSettings         *factoryruntime.JavaScriptWorkerSettings
	inputResolver          roles.InvocationInputResolver
	modelInvocation        modelinvocation.RuntimeModelInvocation
	factoryRuntime         factoryruntime.Service
	factoryDefinitions     factorydefinitions.Service
	workflowPreview        factoryruntime.WorkflowPreviewOperation
	work                   work.Service
	models                 models.Service
	modelsScope            models.RuntimeScopeRef
	workers                workers.Service
	providerSessions       providersessions.Service
	workerSessions         workersessions.ObservationService
	workerPrompts          workers.PromptTemplates
	reader                 roles.RuntimeReader
	projections            recordings.ProjectionService
	recordings             recordings.Service
	clock                  factoryruntime.Clock
	logger                 *zap.Logger
	diagnostics            factoryruntime.RuntimeLogDiagnostics
	directory              string
	runtimeInstanceID      string
	backendScopeID         string
	operatorSettingsPath   string
	orderlyStop            func(context.Context) error
	closeArtifacts         func() error
	historicalReplay       *factorysessions.HistoricalReplayInspection
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

func historicalReplayRuntimeProducts(
	logger *zap.Logger,
	projection recordingreplay.RecordingReplayProjection,
	liveOwner durableexecution.Service,
	closeResources func() error,
) runtimeProducts {
	if closeResources == nil {
		closeResources = func() error { return nil }
	}
	replay := recordingreplay.NewService(projection, liveOwner)
	inspection := replay.Inspection()
	return runtimeProducts{
		process:          historicalReplayProcessRuntime{},
		historicalReplay: &inspection,
		logger:           logger,
		closeArtifacts:   closeResources,
		execution:        replay,
	}
}

func assembleRuntimeProducts(
	ctx context.Context,
	factoryDefinitions factorydefinitions.Service,
	factorySessionGateway roles.SessionGateway,
	sessionInvocation roles.SessionInvoker,
	factoryRuntime factoryruntime.Service,
	factoryWorkflows factoryruntime.JavaScriptWorkflowDefinitions,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
	workService work.Service,
	workerService workers.Service,
	modelsBind modelsRuntimeBind,
	providerSessions providersessions.Service,
	startup runtimeports.RuntimeInstance,
	lifecycle roles.LifecycleRuntime,
	process roles.ProcessRuntime,
	reader roles.RuntimeReader,
	projections recordings.ProjectionService,
	directory string,
	runtimeInstanceID string,
	backendScopeID string,
	closeResources func() error,
	factorySessionIDs ...string,
) runtimeProducts {
	if ctx == nil {
		ctx = context.Background()
	}
	factorySessionID := ""
	if len(factorySessionIDs) > 0 {
		factorySessionID = strings.TrimSpace(factorySessionIDs[0])
	}
	bindRuntime := runtimeBindingForSession(factoryRuntime, factorySessionID)
	effectiveFactorySessionID := resolveOpenedFactorySessionID(ctx, factorySessionGateway, factorySessionID)
	workerPrompts, _ := workerService.(workers.PromptTemplates)
	liveControl, _ := factorySessionGateway.(factorysessions.LiveControlService)
	workerSessions := openedWorkerSessionsObservation(factoryRuntime, startup, effectiveFactorySessionID)
	inputResolver, _ := sessionInvocation.(roles.InvocationInputResolver)
	modelInvocation := modelinvocation.RuntimeModelInvocation{
		FactorySessionID: effectiveFactorySessionID, Scope: modelsBind.Scope,
		RuntimeID: runtimeInstanceID, GenerationID: startup.StreamGeneration(),
		FactoryDirectory: directory, WorkingDirectory: directory,
	}

	return runtimeProducts{
		bindRuntime: bindRuntime,
		process:     process, lifecycle: lifecycle,
		sessions: factorySessionGateway, liveControl: liveControl, execution: factorySessionGateway,
		inputResolver: inputResolver, modelInvocation: modelInvocation,
		factoryRuntime: factoryRuntime, factoryDefinitions: factoryDefinitions,
		workflowPreview: workflowPreview, work: workService,
		models: modelsBind.Root, modelsScope: modelsBind.Scope,
		workers: workerService, providerSessions: providerSessions,
		workerSessions: workerSessions, workerPrompts: workerPrompts,
		reader: reader, projections: projections,
		logger: startup.RuntimeLogger(), diagnostics: startup.RuntimeDiagnostics(),
		directory: directory, runtimeInstanceID: runtimeInstanceID, backendScopeID: backendScopeID,
		closeArtifacts: closeResources,
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

func resolveOpenedFactorySessionID(
	ctx context.Context,
	factorySessionGateway roles.SessionGateway,
	factorySessionID string,
) string {
	effectiveID := factorySessionID
	if factorySessionGateway == nil || factorySessionID == "" {
		return effectiveID
	}
	projection, err := factorySessionGateway.GetFactorySession(ctx, factorySessionID)
	if err != nil {
		return effectiveID
	}
	if resolved := strings.TrimSpace(projection.Context.FactorySessionID); resolved != "" {
		return resolved
	}
	return effectiveID
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
