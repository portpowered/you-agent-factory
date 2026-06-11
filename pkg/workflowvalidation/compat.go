// Deprecated: use github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation instead.
// This package is a Batch 001 compatibility shim; core runtime and API code must import the orchestrator-owned path directly.
package workflowvalidation

import target "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"

type (
	Issue        = target.Issue
	Result       = target.Result
	LoadRequest  = target.LoadRequest
	LoadedSource = target.LoadedSource
	Request      = target.Request
	SourceReader = target.SourceReader
)

const (
	CodeSyntaxError          = target.CodeSyntaxError
	CodeUnsupportedGlobal    = target.CodeUnsupportedGlobal
	CodeUnsupportedPrimitive = target.CodeUnsupportedPrimitive
	CodeForbiddenHostAccess  = target.CodeForbiddenHostAccess
	CodeInvalidMetadata      = target.CodeInvalidMetadata
	CodeInvalidArgsSchema    = target.CodeInvalidArgsSchema
	CodeSourceUnreadable     = target.CodeSourceUnreadable
	CodeUnsupportedLoader    = target.CodeUnsupportedLoader
	CodeSourceHashMismatch   = target.CodeSourceHashMismatch
	FormatJavaScript         = target.FormatJavaScript
	FormatTypeScript         = target.FormatTypeScript
)
