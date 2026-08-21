package runtimeopening

import (
	"context"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// runtimeProducts is the Factory Sessions side of a completed runtime opening.
// It retains the opened role bundles, the Runtime binding publication edge, and
// the opaque Runtime service published by the activation. The hosted-instance,
// replacement-builder, session build spec, lifecycle, and sidecar handles that
// opening used to retain are Runtime-owned construction values; Sessions no
// longer holds them, so it cannot re-enter Runtime construction through them.
type runtimeProducts struct {
	application roles.OpenedApplicationRuntime
	invocation  roles.OpenedInvocationRuntime
	execution   roles.OpenedExecutionRuntime
	bindRuntime func(factoryruntime.RuntimeBinding) error
	engine      factoryruntime.Service
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
) runtimeProducts {
	inspection := recordingreplay.NewService(projection).Inspection()
	return runtimeProducts{
		application: roles.OpenedApplicationRuntime{
			Process:          historicalReplayProcessRuntime{},
			HistoricalReplay: &inspection,
			Resources: roles.RuntimeResources{
				Logger: logger,
				Close:  func() error { return nil },
			},
		},
	}
}

func assembleRuntimeProducts(
	ctx context.Context,
	factoryDefinitions factorydefinitions.Service,
	factorySessionGateway factorysessions.Service,
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
	resources := roles.RuntimeResources{
		Logger: startup.RuntimeLogger(), Close: closeResources,
		Diagnostics: startup.RuntimeDiagnostics(),
	}
	resources.Directory = directory
	resources.RuntimeInstanceID = runtimeInstanceID
	resources.BackendScopeID = backendScopeID
	modelInvoker := NewRuntimeModelInvoker(RuntimeModelInvokerConfig{
		Models: modelsBind.Root, Scope: modelsBind.Scope,
		Sessions: factorySessionGateway, Workers: workerService,
		RuntimeID: runtimeInstanceID, GenerationID: startup.StreamGeneration(),
		FactoryDirectory: directory, WorkingDirectory: directory,
	})
	return runtimeProducts{
		bindRuntime: bindRuntime,
		application: roles.OpenedApplicationRuntime{
			Process:        process,
			FactoryRuntime: factoryRuntime, FactoryDefinitions: factoryDefinitions,
			WorkflowPreview: workflowPreview,
			FactorySessions: factorySessionGateway, LiveControl: liveControl,
			Work: workService, Models: modelsBind.Root, ModelsScope: modelsBind.Scope,
			ModelInvoker: modelInvoker, Workers: workerService,
			ProviderSessions: providerSessions, WorkerSessions: workerSessions,
			WorkerPrompts: workerPrompts, Logger: resources.Logger,
			Visualization: roles.RuntimeVisualizationServices{
				Reader: reader, Projections: projections,
			},
			Resources: resources,
		},
		invocation: roles.OpenedInvocationRuntime{
			Workers: workerService, Sessions: factorySessionGateway, LiveControl: liveControl,
			ModelInvoker: modelInvoker,
			Invoker:      sessionInvocation, InputResolver: inputResolver,
			Execution: factorySessionGateway, Lifecycle: lifecycle,
			ModelsScope: modelsBind.Scope,
			RuntimeID:   runtimeInstanceID, GenerationID: startup.StreamGeneration(),
			CloseArtifacts: closeResources,
		},
		execution: roles.OpenedExecutionRuntime{
			Execution: factorySessionGateway, WorkflowPreview: workflowPreview,
			Resources: resources,
		},
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
	factorySessionGateway factorysessions.Service,
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
