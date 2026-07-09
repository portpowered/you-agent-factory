// Package workflowpreview is a Batch 001 compatibility shim for the legacy root
// workflow preview import path.
//
// Deprecated: canonical ownership for JavaScript workflow preview lives in
// github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview. Core
// runtime and API code must import pkg/orchestrators/javascript/preview directly.
package workflowpreview

import target "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"

type (
	Request               = target.Request
	SourceValidationIssue = target.SourceValidationIssue
	ResultConstraints     = target.ResultConstraints
	Preview               = target.Preview
)
