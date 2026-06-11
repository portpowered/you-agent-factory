// Package workflowpreview is a transitional compatibility shim for JavaScript
// orchestrator Factory preview preparation.
//
// Deprecated: use pkg/orchestrators/javascript/preview and POST /factories/preview
// for Factory preview semantics. This shim is not the final package boundary.
package workflowpreview

import jspreview "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/preview"

type (
	Preview               = jspreview.Preview
	Request               = jspreview.Request
	ResultConstraints     = jspreview.ResultConstraints
	SourceValidationIssue = jspreview.SourceValidationIssue
)

var (
	BuildPreview            = jspreview.BuildPreview
	DefaultResultConstraints = jspreview.DefaultResultConstraints
)

// BuildFactoryPreview is an alias for BuildPreview using Factory preview semantics.
var BuildFactoryPreview = jspreview.BuildFactoryPreview
