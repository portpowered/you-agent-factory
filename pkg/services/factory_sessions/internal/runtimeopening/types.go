package runtimeopening

import (
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

type runtimeProducts struct {
	application roles.OpenedApplicationRuntime
	invocation  roles.OpenedInvocationRuntime
	execution   roles.OpenedExecutionRuntime
	bindRuntime func(factoryruntime.RuntimeBinding) error
	startup     runtimeports.RuntimeInstance
	replacement runtimeports.RuntimeReplacementBuilder
	buildSpec   factoryruntime.SessionBuildSpec
	lifecycle   runtimeports.RuntimeLifecycle
	sidecars    runtimeports.RuntimeSidecarService
}

type workerSessionsObservationProvider interface {
	WorkerSessionsObservation() workersessions.ObservationService
}

type detachedOperationsProvider interface {
	DetachedOperations() factorysessions.DetachedService
}

func detachedOperationsFromAssembly(assembly roles.RuntimeAssembly) factorysessions.DetachedService {
	provider, ok := assembly.(detachedOperationsProvider)
	if !ok {
		return nil
	}
	return provider.DetachedOperations()
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
	httpServices := roles.RuntimeHTTPServices{
		FactoryRuntime: factoryRuntime, FactoryDefinitions: factoryDefinitions,
		WorkflowPreview: workflowPreview,
		FactorySessions: factorySessionGateway, LiveControl: factorySessionGateway,
		Work:   workService,
		Models: modelsBind.Root, ModelsScope: modelsBind.Scope,
		ModelInvoker: NewRuntimeModelInvoker(RuntimeModelInvokerConfig{
			Models: modelsBind.Root, Scope: modelsBind.Scope,
			Sessions: factorySessionGateway, Workers: workerService,
			RuntimeID: runtimeInstanceID, GenerationID: startup.StreamGeneration(),
			FactoryDirectory: directory, WorkingDirectory: directory,
		}),
		Workers: workerService, ProviderSessions: providerSessions,
		WorkerSessions: workerSessions,
		WorkerPrompts:  workerPrompts, Logger: resources.Logger,
	}
	return runtimeProducts{
		bindRuntime: bindRuntime,
		application: roles.OpenedApplicationRuntime{
			Process: process, HTTP: httpServices,
			Visualization: roles.RuntimeVisualizationServices{
				Reader: reader, Projections: projections,
			},
			Resources: resources,
		},
		invocation: roles.OpenedInvocationRuntime{
			Workers: workerService, Sessions: factorySessionGateway,
			ModelInvoker: NewRuntimeModelInvoker(RuntimeModelInvokerConfig{
				Models: modelsBind.Root, Scope: modelsBind.Scope,
				Sessions: factorySessionGateway, Workers: workerService,
				RuntimeID: runtimeInstanceID, GenerationID: startup.StreamGeneration(),
				FactoryDirectory: directory, WorkingDirectory: directory,
			}),
			Invoker: sessionInvocation, InputResolver: inputResolver,
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
