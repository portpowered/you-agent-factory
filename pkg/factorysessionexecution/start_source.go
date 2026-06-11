package factorysessionexecution

import (
	"strings"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// StartSourceContext supplies filesystem roots for durable start source resolution.
type StartSourceContext struct {
	ProjectRoot string
}

// ResolveStartSource resolves one normalized start request through the JavaScript
// orchestrator source contract used by durable Factory Session start paths.
func ResolveStartSource(req StartRequest, ctx StartSourceContext) (ResolvedSource, error) {
	projectRoot := strings.TrimSpace(ctx.ProjectRoot)
	if projectRoot == "" {
		return ResolvedSource{}, NewValidationError("projectRoot", "projectRoot is required")
	}

	sourceCtx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		return ResolvedSource{}, NewValidationError("projectRoot", err.Error())
	}

	resolution := workflowsource.Resolve(startSourceRequest(req.Source), sourceCtx)
	if !resolution.Found {
		message := "workflow source could not be resolved"
		if len(resolution.Diagnostics) > 0 && strings.TrimSpace(resolution.Diagnostics[0].Message) != "" {
			message = resolution.Diagnostics[0].Message
		}
		return ResolvedSource{}, NewValidationError("source", message)
	}

	resolved := ResolvedSource{
		Kind:       resolution.ResolvedKind,
		SourceRef:  resolution.SourceRef,
		SourceHash: resolution.SourceHash,
		Dialect:    resolution.Dialect,
		Metadata: map[string]string{
			"project": sourceCtx.ProjectRoot,
		},
	}
	if stage := resolutionOrderForLookupStage(resolution.LookupStage); stage != "" {
		resolved.ResolutionOrder = []string{stage}
	}
	return resolved, nil
}

func startSourceRequest(source Source) workflowsource.Request {
	switch source.Kind {
	case workflowsource.KindFactoryID:
		return workflowsource.Request{
			Kind:  source.Kind,
			Value: source.FactoryID,
		}
	case workflowsource.KindFactoryInline:
		return workflowsource.Request{
			Kind:  source.Kind,
			Value: string(source.FactoryInline),
		}
	case workflowsource.KindWorkflowFile:
		return workflowsource.Request{
			Kind:  source.Kind,
			Value: source.WorkflowFile,
		}
	case workflowsource.KindWorkflowName:
		return workflowsource.Request{
			Kind:  source.Kind,
			Value: source.WorkflowName,
		}
	case workflowsource.KindInlineWorkflow:
		inline := source.InlineWorkflow
		if inline == nil {
			return workflowsource.Request{Kind: source.Kind}
		}
		return workflowsource.Request{
			Kind:         source.Kind,
			Value:        inline.InlineSource,
			InlineSource: inline.InlineSource,
		}
	default:
		return workflowsource.Request{Kind: source.Kind}
	}
}

func resolutionOrderForLookupStage(stage workflowsource.LookupStage) string {
	switch stage {
	case workflowsource.LookupStageProjectClaude, workflowsource.LookupStageExplicitSourceKind:
		return "PROJECT_CLAUDE_WORKFLOWS"
	case workflowsource.LookupStageGlobalUser:
		return "USER_YOU_AGENT_FACTORY_WORKFLOWS"
	case workflowsource.LookupStagePackageRelative:
		return "PACKAGE_RELATIVE_WORKFLOW_DIRECTORIES"
	case workflowsource.LookupStageNamedJavaScript:
		return "BUILTIN_GLOBAL_JAVASCRIPT_FACTORIES"
	case workflowsource.LookupStageExplicitFactory:
		return "EXPLICIT_FACTORY_LOOKUP"
	default:
		return ""
	}
}
