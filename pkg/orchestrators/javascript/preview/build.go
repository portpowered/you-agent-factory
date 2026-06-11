package preview

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
)

// BuildPreview resolves JavaScript orchestrator source, validates it without
// execution, and projects policy preview metadata for Factory preview surfaces.
func BuildPreview(input Request) Preview {
	return buildPreview(input)
}

// BuildFactoryPreview is an alias for BuildPreview using Factory preview semantics.
func BuildFactoryPreview(input Request) Preview {
	return buildPreview(input)
}

func buildPreview(input Request) Preview {
	sourceResolution := source.Resolve(input.Source, input.Context)
	preview := Preview{
		SourceResolution:       sourceResolution,
		SourceValidationIssues: validateSource(input, sourceResolution),
		PolicyPreview: policy.BuildPreview(policy.PreviewInput{
			Request: policy.Request{
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

func validateSource(input Request, resolution source.Resolution) []SourceValidationIssue {
	var issues []SourceValidationIssue
	issues = append(issues, validateOrchestratorConfig(input)...)

	if !resolution.Found {
		return issues
	}
	content := strings.TrimSpace(resolution.Content)
	if content == "" {
		return issues
	}

	loaded, loadIssues := validation.Load(validation.LoadRequest{
		SourceRef: resolution.SourceRef,
		Content:   content,
	})
	for _, issue := range loadIssues {
		issues = append(issues, sourceIssueFromValidation(issue))
	}
	if len(loadIssues) > 0 {
		return issues
	}

	validationResult := validation.ValidateLoaded(loaded, validation.Request{
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
	configResult := validation.Validate(validation.Request{
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
