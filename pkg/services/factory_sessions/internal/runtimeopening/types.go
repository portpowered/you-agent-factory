package runtimeopening

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type runtimeProducts struct {
	application roles.OpenedApplicationRuntime
	invocation  roles.OpenedInvocationRuntime
	execution   roles.OpenedExecutionRuntime
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
	startup factoryruntime.HostedInstance,
	lifecycle roles.LifecycleRuntime,
	process roles.ProcessRuntime,
	reader roles.RuntimeReader,
	projections recordings.ProjectionService,
	directory string,
	runtimeInstanceID string,
	backendScopeID string,
	closeResources func() error,
) runtimeProducts {
	workerPrompts, _ := workerService.(workers.PromptTemplates)
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
		FactorySessions: factorySessionGateway, Work: workService,
		Models: modelsBind.Root, ModelsScope: modelsBind.Scope,
		Workers: workerService, ProviderSessions: providerSessions,
		WorkerPrompts: workerPrompts, Logger: resources.Logger,
	}
	return runtimeProducts{
		application: roles.OpenedApplicationRuntime{
			Process: process, HTTP: httpServices,
			Visualization: roles.RuntimeVisualizationServices{
				Reader: reader, Projections: projections,
			},
			Resources: resources,
		},
		invocation: roles.OpenedInvocationRuntime{
			Workers: workerService, Sessions: factorySessionGateway,
			Invoker: sessionInvocation, InputResolver: inputResolver,
			Execution: factorySessionGateway, Lifecycle: lifecycle,
			ModelsScope:    modelsBind.Scope,
			CloseArtifacts: closeResources,
		},
		execution: roles.OpenedExecutionRuntime{
			Execution: factorySessionGateway, WorkflowPreview: workflowPreview,
			Resources: resources,
		},
	}
}
