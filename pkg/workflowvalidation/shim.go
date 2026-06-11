// Package workflowvalidation is a transitional compatibility shim for JavaScript
// orchestrator source validation.
//
// Deprecated: use pkg/orchestrators/javascript/validation. This shim delegates to
// orchestrator ownership and is not the final package boundary.
package workflowvalidation

import jsvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"

type (
	Issue        = jsvalidation.Issue
	LoadRequest  = jsvalidation.LoadRequest
	LoadedSource = jsvalidation.LoadedSource
	Request      = jsvalidation.Request
	Result       = jsvalidation.Result
)

const (
	CodeForbiddenHostAccess   = jsvalidation.CodeForbiddenHostAccess
	CodeInvalidArgsSchema     = jsvalidation.CodeInvalidArgsSchema
	CodeInvalidMetadata       = jsvalidation.CodeInvalidMetadata
	CodeSourceHashMismatch    = jsvalidation.CodeSourceHashMismatch
	CodeSourceUnreadable      = jsvalidation.CodeSourceUnreadable
	CodeSyntaxError           = jsvalidation.CodeSyntaxError
	CodeUnsupportedLoader     = jsvalidation.CodeUnsupportedLoader
	CodeUnsupportedPrimitive  = jsvalidation.CodeUnsupportedPrimitive
	FormatJavaScript          = jsvalidation.FormatJavaScript
	FormatTypeScript          = jsvalidation.FormatTypeScript
)

var (
	FileSourceReader = jsvalidation.FileSourceReader
	Load             = jsvalidation.Load
	SourceHash       = jsvalidation.SourceHash
	Validate         = jsvalidation.Validate
	ValidateLoaded   = jsvalidation.ValidateLoaded
)
