package workflowpreview

import (
	"strings"

	workflowsource "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/javascript/source"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/javascript/validation"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
)

// BuildPreview resolves workflow source, validates it without execution, and projects policy preview metadata.
func BuildPreview(input Request) Preview {
	sourceResolution := workflowsource.Resolve(input.Source, input.Context)
	preview := Preview{
		SourceResolution:       sourceResolution,
		SourceValidationIssues: validateSource(input, sourceResolution),
		PolicyPreview: workflowpolicy.BuildPreview(workflowpolicy.PreviewInput{
			Request: workflowpolicy.Request{
				Requested:      input.RequestedPolicy,
				FactoryDefault: input.FactoryDefaultPolicy,
				DeploymentCap:  input.DeploymentCap,
			},
			RequestedRunner:  input.RequestedRunner,
			RequestedModel:   input.RequestedModel,
			RequestedProfile: input.RequestedProfile,
			TimeoutMillis:    input.TimeoutMillis,
		}),
		ResultConstraints: DefaultResultConstraints(),
	}
	preview.Valid = !preview.HasBlockingIssues()
	return preview
}

func validateSource(input Request, resolution workflowsource.Resolution) []SourceValidationIssue {
	var issues []SourceValidationIssue
	issues = append(issues, validateOrchestratorConfig(input)...)

	if !resolution.Found {
		return issues
	}
	content := strings.TrimSpace(resolution.Content)
	if content == "" {
		return issues
	}

	loaded, loadIssues := workflowvalidation.Load(workflowvalidation.LoadRequest{
		SourceRef: resolution.SourceRef,
		Content:   content,
	})
	for _, issue := range loadIssues {
		issues = append(issues, sourceIssueFromValidation(issue))
	}
	if len(loadIssues) > 0 {
		return issues
	}

	validationResult := workflowvalidation.ValidateLoaded(loaded, workflowvalidation.Request{
		ConfigPath: "orchestrator.javascript",
		Metadata:   input.Metadata,
		ArgsSchema: input.ArgsSchema,
	})
	for _, issue := range validationResult.Issues {
		issues = append(issues, sourceIssueFromValidation(issue))
	}
	return issues
}

func validateOrchestratorConfig(input Request) []SourceValidationIssue {
	configResult := workflowvalidation.Validate(workflowvalidation.Request{
		ConfigPath: "orchestrator.javascript",
		Metadata:   input.Metadata,
		ArgsSchema: input.ArgsSchema,
	})
	out := make([]SourceValidationIssue, 0, len(configResult.Issues))
	for _, issue := range configResult.Issues {
		out = append(out, sourceIssueFromValidation(issue))
	}
	return out
}
