package factory

import workflowvalidation "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/javascript/validation"

type (
	WorkflowValidationIssue        = workflowvalidation.Issue
	WorkflowValidationResult       = workflowvalidation.Result
	WorkflowValidationRequest      = workflowvalidation.Request
	WorkflowValidationLoadRequest  = workflowvalidation.LoadRequest
	WorkflowValidationLoadedSource = workflowvalidation.LoadedSource
	WorkflowSourceReader           = workflowvalidation.SourceReader
)

const (
	WorkflowValidationFormatJavaScript = workflowvalidation.FormatJavaScript
	WorkflowValidationFormatTypeScript = workflowvalidation.FormatTypeScript

	WorkflowValidationCodeSyntaxError          = workflowvalidation.CodeSyntaxError
	WorkflowValidationCodeUnsupportedGlobal    = workflowvalidation.CodeUnsupportedGlobal
	WorkflowValidationCodeUnsupportedPrimitive = workflowvalidation.CodeUnsupportedPrimitive
	WorkflowValidationCodeForbiddenHostAccess  = workflowvalidation.CodeForbiddenHostAccess
	WorkflowValidationCodeInvalidMetadata      = workflowvalidation.CodeInvalidMetadata
	WorkflowValidationCodeInvalidArgsSchema    = workflowvalidation.CodeInvalidArgsSchema
	WorkflowValidationCodeSourceUnreadable     = workflowvalidation.CodeSourceUnreadable
	WorkflowValidationCodeUnsupportedLoader    = workflowvalidation.CodeUnsupportedLoader
	WorkflowValidationCodeSourceHashMismatch   = workflowvalidation.CodeSourceHashMismatch
)
