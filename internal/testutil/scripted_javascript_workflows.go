package testutil

import (
	"context"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// ScriptedJavaScriptWorkflowDefinitions is a transport-edge fake. Tests set
// only the root-contract operations their adapter invokes; all Factory Runtime
// implementation behavior remains owner-local.
type ScriptedJavaScriptWorkflowDefinitions struct {
	factoryruntime.JavaScriptWorkflowDefinitions

	DefaultSourceContextFunc func(string) (factoryruntime.WorkflowSourceContext, error)
	BuildPreviewFunc         func(factoryruntime.WorkflowPreviewRequest) factoryruntime.WorkflowPreview
	PreviewWorkflowFunc      func(context.Context, factoryruntime.WorkflowPreviewInput) (factoryruntime.WorkflowPreview, error)
	ResolveSourceFunc        func(
		factoryruntime.WorkflowSourceRequest,
		factoryruntime.WorkflowSourceContext,
	) factoryruntime.WorkflowSourceResolution
}

func (service ScriptedJavaScriptWorkflowDefinitions) PreviewWorkflow(
	ctx context.Context,
	input factoryruntime.WorkflowPreviewInput,
) (factoryruntime.WorkflowPreview, error) {
	if service.PreviewWorkflowFunc != nil {
		return service.PreviewWorkflowFunc(ctx, input)
	}
	context, err := service.DefaultSourceContext(input.ProjectRoot)
	if err != nil {
		return factoryruntime.WorkflowPreview{}, err
	}
	return service.BuildPreview(factoryruntime.WorkflowPreviewRequest{
		Source: input.Source, Context: context, Metadata: input.Metadata,
		ArgsSchema: input.ArgsSchema, FactoryDefaultPolicy: input.FactoryDefaultPolicy,
		RequestedPolicy: input.RequestedPolicy, RequestedRunner: input.RequestedRunner,
		RequestedModel: input.RequestedModel, RequestedProfile: input.RequestedProfile,
		TimeoutMillis: input.TimeoutMillis,
	}), nil
}

func (service ScriptedJavaScriptWorkflowDefinitions) DefaultSourceContext(
	root string,
) (factoryruntime.WorkflowSourceContext, error) {
	return service.DefaultSourceContextFunc(root)
}

func (service ScriptedJavaScriptWorkflowDefinitions) BuildPreview(
	request factoryruntime.WorkflowPreviewRequest,
) factoryruntime.WorkflowPreview {
	return service.BuildPreviewFunc(request)
}

func (service ScriptedJavaScriptWorkflowDefinitions) ResolveSource(
	request factoryruntime.WorkflowSourceRequest,
	context factoryruntime.WorkflowSourceContext,
) factoryruntime.WorkflowSourceResolution {
	return service.ResolveSourceFunc(request, context)
}
