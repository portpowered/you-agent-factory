package workers

import (
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
)

type PromptRenderer = workerprompting.PromptRenderer
type TokenData = workerprompting.TokenData
type PromptData = workerprompting.PromptData
type PromptHistory = workerprompting.PromptHistory
type PromptContext = workerprompting.PromptContext
type DefaultPromptRenderer = workerprompting.DefaultPromptRenderer
type ResolvedFields = workerprompting.ResolvedFields
type PromptTemplateVariableCategory = workerprompting.PromptTemplateVariableCategory
type PromptTemplateVariableReference = workerprompting.PromptTemplateVariableReference
type PromptTemplateUnavailableAccessPattern = workerprompting.PromptTemplateUnavailableAccessPattern
type PromptTemplateContract = workerprompting.PromptTemplateContract
type PromptTemplateDiagnosticKind = workerprompting.PromptTemplateDiagnosticKind
type PromptTemplateDiagnostic = workerprompting.PromptTemplateDiagnostic
type PromptTemplateValidationResult = workerprompting.PromptTemplateValidationResult

const (
	PromptTemplateDiagnosticKindSyntaxError         = workerprompting.PromptTemplateDiagnosticKindSyntaxError
	PromptTemplateDiagnosticKindInvalidVariable     = workerprompting.PromptTemplateDiagnosticKindInvalidVariable
	PromptTemplateDiagnosticKindUnavailableVariable = workerprompting.PromptTemplateDiagnosticKindUnavailableVariable
)

func ResolveTemplateFields(
	workingDirTemplate string,
	envTemplates map[string]string,
	tokens []interfaces.Token,
	wfCtx *factory_context.FactoryContext,
	worktreeTemplate string,
) (*ResolvedFields, error) {
	return workerprompting.ResolveTemplateFields(workingDirTemplate, envTemplates, tokens, wfCtx, worktreeTemplate)
}

func applyResolvedFields(base *factory_context.FactoryContext, resolved *ResolvedFields) *factory_context.FactoryContext {
	return workerprompting.ApplyResolvedFields(base, resolved)
}
