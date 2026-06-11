// Deprecated: use github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview instead.
// This package is a Batch 001 compatibility shim; core runtime and API code must import the orchestrator-owned path directly.
package workflowpreview

import target "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"

type (
	Request = target.Request
	SourceValidationIssue = target.SourceValidationIssue
	ResultConstraints = target.ResultConstraints
	Preview = target.Preview
)

func BuildPreview(input Request) Preview { return target.BuildPreview(input) }

func DefaultResultConstraints() ResultConstraints { return target.DefaultResultConstraints() }

