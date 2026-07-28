// Package prompting is a transitional compile shim that re-exports prompting
// helpers from the private workstations destination. Peers should resolve
// through workers/wire; baseline deletion of this path is owned by DEL-WRK.
package prompting

import (
	private "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
)

type (
	PromptRenderer                       = private.PromptRenderer
	TokenData                            = private.TokenData
	PromptData                           = private.PromptData
	PromptHistory                        = private.PromptHistory
	PromptContext                        = private.PromptContext
	DefaultPromptRenderer                = private.DefaultPromptRenderer
	ResolvedFields                       = private.ResolvedFields
	PromptTemplateVariableCategory       = private.PromptTemplateVariableCategory
	PromptTemplateVariableReference      = private.PromptTemplateVariableReference
	PromptTemplateUnavailableAccessPattern = private.PromptTemplateUnavailableAccessPattern
	PromptTemplateContract               = private.PromptTemplateContract
	PromptTemplateDiagnosticKind         = private.PromptTemplateDiagnosticKind
	PromptTemplateDiagnostic             = private.PromptTemplateDiagnostic
	PromptTemplateValidationResult       = private.PromptTemplateValidationResult
	Service                              = private.Service
)

const (
	PromptTemplateVariableCategoryRoot      = private.PromptTemplateVariableCategoryRoot
	PromptTemplateVariableCategoryInput     = private.PromptTemplateVariableCategoryInput
	PromptTemplateVariableCategoryHistory   = private.PromptTemplateVariableCategoryHistory
	PromptTemplateVariableCategoryContext   = private.PromptTemplateVariableCategoryContext
	PromptTemplateVariableCategoryMapAccess = private.PromptTemplateVariableCategoryMapAccess
	PromptTemplateVariableCategoryDoc       = private.PromptTemplateVariableCategoryDoc
)

var (
	NewDefaultPromptRenderer          = private.NewDefaultPromptRenderer
	BuildPromptData                   = private.BuildPromptData
	BuildPromptDataWithFactoryDocs    = private.BuildPromptDataWithFactoryDocs
	ResolveTemplateFields             = private.ResolveTemplateFields
	ApplyResolvedFields               = private.ApplyResolvedFields
	NormalizeFactoryBundledDocTargetPaths = private.NormalizeFactoryBundledDocTargetPaths
	NewFactoryDocsLoader              = private.NewFactoryDocsLoader
	NewService                        = private.NewService
	BuildPromptTemplateContract       = private.BuildPromptTemplateContract
	ValidatePromptTemplate            = private.ValidatePromptTemplate
)
