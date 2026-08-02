// Package javascript provides the concrete JavaScript orchestrator capability
// selected by the application composition root.
package javascript

import (
	"context"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowpreview "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript/preview"
	workflowruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript/runtime"
	workflowsource "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript/source"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript/validation"
)

// Service implements the JavaScript orchestrator capabilities published by
// Factory Runtime.
type Service struct {
	files           factoryruntime.WorkflowSourceFileSystem
	resolveHome     factoryruntime.WorkflowHomeResolver
	resolveSymlinks factoryruntime.WorkflowSourceResolveSymlinks
}

var _ factoryruntime.JavaScriptWorkflows = (*Service)(nil)
var _ factoryruntime.WorkflowPreviewOperation = (*Service)(nil)

// New constructs the stateless JavaScript orchestrator service.
func New(
	files factoryruntime.WorkflowSourceFileSystem,
	resolveHome factoryruntime.WorkflowHomeResolver,
	resolveSymlinks factoryruntime.WorkflowSourceResolveSymlinks,
) *Service {
	return &Service{files: files, resolveHome: resolveHome, resolveSymlinks: resolveSymlinks}
}

func (s *Service) BuildPreview(request factoryruntime.WorkflowPreviewRequest) factoryruntime.WorkflowPreview {
	request.Context = workflowsource.WithDependencies(request.Context, s.files, s.resolveSymlinks)
	return workflowpreview.BuildPreview(request)
}

// PreviewWorkflow owns source-context defaults and construction of the
// internal preview request. Transports forward only decoded external fields.
func (s *Service) PreviewWorkflow(ctx context.Context, input factoryruntime.WorkflowPreviewInput) (factoryruntime.WorkflowPreview, error) {
	if ctx == nil {
		return factoryruntime.WorkflowPreview{}, fmt.Errorf("workflow preview context is required")
	}
	if err := ctx.Err(); err != nil {
		return factoryruntime.WorkflowPreview{}, err
	}
	projectRoot := strings.TrimSpace(input.ProjectRoot)
	if projectRoot == "" {
		return factoryruntime.WorkflowPreview{}, fmt.Errorf("project root is required")
	}
	sourceContext, err := s.DefaultSourceContext(projectRoot)
	if err != nil {
		return factoryruntime.WorkflowPreview{}, err
	}
	preview := s.BuildPreview(factoryruntime.WorkflowPreviewRequest{
		Source:               input.Source,
		Context:              sourceContext,
		Metadata:             input.Metadata,
		ArgsSchema:           input.ArgsSchema,
		FactoryDefaultPolicy: input.FactoryDefaultPolicy,
		RequestedPolicy:      input.RequestedPolicy,
		RequestedRunner:      input.RequestedRunner,
		RequestedModel:       input.RequestedModel,
		RequestedProfile:     input.RequestedProfile,
		TimeoutMillis:        input.TimeoutMillis,
	})
	if err := ctx.Err(); err != nil {
		return factoryruntime.WorkflowPreview{}, err
	}
	return preview, nil
}

func (s *Service) DefaultSourceContext(projectRoot string) (factoryruntime.WorkflowSourceContext, error) {
	if s == nil || s.files == nil || s.resolveHome == nil || s.resolveSymlinks == nil {
		return factoryruntime.WorkflowSourceContext{}, fmt.Errorf("JavaScript workflow source filesystem, home resolver, and symlink resolver are required")
	}
	home, err := s.resolveHome()
	if err != nil {
		return factoryruntime.WorkflowSourceContext{}, fmt.Errorf("resolve workflow source home: %w", err)
	}
	return workflowsource.DefaultContext(projectRoot, home, s.files, s.resolveSymlinks)
}

func (s *Service) ResolveSource(
	request factoryruntime.WorkflowSourceRequest,
	sourceContext factoryruntime.WorkflowSourceContext,
) factoryruntime.WorkflowSourceResolution {
	if s == nil || s.files == nil || s.resolveSymlinks == nil {
		return factoryruntime.WorkflowSourceResolution{
			RequestKind:  request.Kind,
			RequestValue: request.Value,
			Diagnostics: []factoryruntime.WorkflowSourceDiagnostic{{
				Code:    "workflow.source.dependencies",
				Message: "workflow source filesystem and symlink resolver are required",
			}},
		}
	}
	return workflowsource.Resolve(request, workflowsource.WithDependencies(sourceContext, s.files, s.resolveSymlinks))
}

func (*Service) LoadSource(
	request factoryruntime.WorkflowValidationLoadRequest,
) (factoryruntime.WorkflowValidationLoadedSource, []factoryruntime.WorkflowValidationIssue) {
	return workflowvalidation.Load(request)
}

func (*Service) ValidateArgs(schemaJSON []byte, args map[string]any) error {
	return workflowvalidation.ValidateArgs(schemaJSON, args)
}

func (*Service) ValidateLoaded(
	loaded factoryruntime.WorkflowValidationLoadedSource,
	request factoryruntime.WorkflowValidationRequest,
) factoryruntime.WorkflowValidationResult {
	return workflowvalidation.ValidateLoaded(loaded, request)
}

func (*Service) Validate(
	request factoryruntime.WorkflowValidationRequest,
) factoryruntime.WorkflowValidationResult {
	return workflowvalidation.Validate(request)
}

func (*Service) Run(
	ctx context.Context,
	request factoryruntime.JavaScriptRuntimeRequest,
	hooks factoryruntime.JavaScriptRuntimeHooks,
) (factoryruntime.JavaScriptRuntimeOutcome, error) {
	return workflowruntime.Run(ctx, request, hooks)
}

func (*Service) ResumeContext(
	summary factoryruntime.JavaScriptCompletedCheckpointSummary,
	records []factoryruntime.JavaScriptRuntimeRecord,
) factoryruntime.JavaScriptResumeContext {
	return workflowruntime.ResumeContextFromCheckpointSummary(summary, records)
}

func (*Service) TextDigest(text string) string {
	return workflowruntime.TextDigest(text)
}

func (*Service) SchemaDigest(schema map[string]any) string {
	return workflowruntime.SchemaDigest(schema)
}

func (*Service) CloneOutputMap(output map[string]any) map[string]any {
	return workflowruntime.CloneOutputMap(output)
}
