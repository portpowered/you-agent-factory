package runtimeopening

import (
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
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
		application: roles.OpenedApplicationRuntime{
			Process:          historicalReplayProcessRuntime{},
			HistoricalReplay: &inspection,
			Resources: roles.RuntimeResources{
				Logger: logger,
				Close:  closeResources,
			},
		},
		invocation: roles.OpenedInvocationRuntime{
			Execution:      replay,
			CloseArtifacts: closeResources,
		},
		execution: roles.OpenedExecutionRuntime{
			Execution: replay,
			Resources: roles.RuntimeResources{Logger: logger, Close: closeResources},
		},
	}
}

func assembleRuntimeProducts(
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
	var bindRuntime func(factoryruntime.RuntimeBinding) error
	factorySessionID := ""
	if len(factorySessionIDs) > 0 {
		factorySessionID = strings.TrimSpace(factorySessionIDs[0])
	}
	if factorySessionID != "" {
		if binder, ok := factoryRuntime.(interface {
			BindRuntime(string, factoryruntime.RuntimeBinding) error
		}); ok {
			bindRuntime = func(binding factoryruntime.RuntimeBinding) error {
				return binder.BindRuntime(factorySessionID, binding)
			}
		}
	}
	workerPrompts, _ := workerService.(workers.PromptTemplates)
	liveControl, _ := factorySessionGateway.(factorysessions.LiveControlService)
	var workerSessions workersessions.ObservationService
	if provider, ok := factoryRuntime.(workerSessionsObservationProvider); ok {
		workerSessions = provider.WorkerSessionsObservation()
	}
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
