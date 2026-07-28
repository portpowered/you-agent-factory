// Package prompting is a transitional compile shim that re-exports prompt
// template contracts from the published Workers root contract.
package prompting

import (
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstationprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
)

var NewFactoryDocsLoader = workstationprompting.NewFactoryDocsLoader

type (
	PromptTemplateVariableCategory         = workers.PromptTemplateVariableCategory
	PromptTemplateVariableReference        = workers.PromptTemplateVariableReference
	PromptTemplateUnavailableAccessPattern = workers.PromptTemplateUnavailableAccessPattern
	PromptTemplateContract                 = workers.PromptTemplateContract
	PromptTemplateDiagnosticKind           = workers.PromptTemplateDiagnosticKind
	PromptTemplateDiagnostic               = workers.PromptTemplateDiagnostic
	PromptTemplateValidationResult         = workers.PromptTemplateValidationResult
	PromptTemplates                        = workers.PromptTemplates
)

const (
	PromptTemplateVariableCategoryRoot      = workers.PromptTemplateVariableCategoryRoot
	PromptTemplateVariableCategoryInput     = workers.PromptTemplateVariableCategoryInput
	PromptTemplateVariableCategoryHistory   = workers.PromptTemplateVariableCategoryHistory
	PromptTemplateVariableCategoryContext   = workers.PromptTemplateVariableCategoryContext
	PromptTemplateVariableCategoryMapAccess = workers.PromptTemplateVariableCategoryMapAccess
	PromptTemplateVariableCategoryDoc       = workers.PromptTemplateVariableCategoryDoc
	PromptTemplateDiagnosticKindSyntaxError         = workers.PromptTemplateDiagnosticKindSyntaxError
	PromptTemplateDiagnosticKindInvalidVariable     = workers.PromptTemplateDiagnosticKindInvalidVariable
	PromptTemplateDiagnosticKindUnavailableVariable = workers.PromptTemplateDiagnosticKindUnavailableVariable
)
