package preview

import (
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
)

// Request is the shared workflow validation and policy preview contract input.
type Request struct {
	Source source.Request
	Context source.Context

	Metadata             map[string]string
	ArgsSchema           []byte
	FactoryDefaultPolicy json.RawMessage
	RequestedPolicy      map[string]any

	RequestedRunner  string
	RequestedModel   string
	RequestedProfile string
	TimeoutMillis    *int64
	DeploymentCap    int
}

// SourceValidationIssue is one workflow source validation diagnostic.
type SourceValidationIssue struct {
	Code    string
	Message string
	Path    string
	Line    int
	Column  int
}

// ResultConstraints documents structured workflow result expectations.
type ResultConstraints struct {
	RequiresStructuredCloneableJSON bool
	ArtifactURIScheme               string
	MaxEmbeddedBytes                int64
	RejectedValueKinds              []string
}

// Preview is the shared validation, source resolution, and policy preview contract.
type Preview struct {
	Valid                  bool
	SourceResolution       source.Resolution
	SourceValidationIssues []SourceValidationIssue
	PolicyPreview          policy.Preview
	ResultConstraints      ResultConstraints
}

// HasBlockingIssues reports whether preview found blocking validation or policy issues.
func (p Preview) HasBlockingIssues() bool {
	if !p.SourceResolution.Found {
		return true
	}
	if !p.SourceResolution.ArtifactRoot.Allowed && p.SourceResolution.ArtifactRoot.Requested != "" {
		return true
	}
	if len(p.SourceValidationIssues) > 0 {
		return true
	}
	if len(p.PolicyPreview.ValidationIssues) > 0 {
		return true
	}
	for _, diagnostic := range p.SourceResolution.Diagnostics {
		if diagnostic.Code == source.CodeSourceConflict {
			return true
		}
	}
	return false
}

func sourceIssueFromValidation(issue validation.Issue) SourceValidationIssue {
	return SourceValidationIssue{
		Code:    issue.Code,
		Message: issue.Message,
		Path:    issue.Path,
		Line:    issue.Line,
		Column:  issue.Column,
	}
}
