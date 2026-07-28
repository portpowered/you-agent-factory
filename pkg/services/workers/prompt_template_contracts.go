package workers

type PromptTemplateVariableCategory string

const (
	PromptTemplateVariableCategoryRoot      PromptTemplateVariableCategory = "ROOT"
	PromptTemplateVariableCategoryInput     PromptTemplateVariableCategory = "INPUT"
	PromptTemplateVariableCategoryHistory   PromptTemplateVariableCategory = "HISTORY"
	PromptTemplateVariableCategoryContext   PromptTemplateVariableCategory = "CONTEXT"
	PromptTemplateVariableCategoryMapAccess PromptTemplateVariableCategory = "MAP_ACCESS"
	PromptTemplateVariableCategoryDoc       PromptTemplateVariableCategory = "DOC"
)

type PromptTemplateVariableReference struct {
	Category    PromptTemplateVariableCategory
	Description string
	Example     string
	Path        string
}

type PromptTemplateUnavailableAccessPattern struct {
	Example string
	Path    string
	Reason  string
}

type PromptTemplateContract struct {
	AvailableVariables        []PromptTemplateVariableReference
	InputCount                int
	UnavailableAccessPatterns []PromptTemplateUnavailableAccessPattern
}

type PromptTemplateDiagnosticKind string

const (
	PromptTemplateDiagnosticKindSyntaxError         PromptTemplateDiagnosticKind = "SYNTAX_ERROR"
	PromptTemplateDiagnosticKindInvalidVariable     PromptTemplateDiagnosticKind = "INVALID_VARIABLE"
	PromptTemplateDiagnosticKindUnavailableVariable PromptTemplateDiagnosticKind = "UNAVAILABLE_VARIABLE"
)

type PromptTemplateDiagnostic struct {
	EndOffset   int
	Kind        PromptTemplateDiagnosticKind
	Message     string
	Path        string
	SourceText  string
	StartOffset int
}

type PromptTemplateValidationResult struct {
	Diagnostics []PromptTemplateDiagnostic
	Valid       bool
}

// PromptTemplates is the public Workers role for editor-facing prompt
// contracts and validation.
type PromptTemplates interface {
	BuildPromptTemplateContract(int, []string) PromptTemplateContract
	ValidatePromptTemplate(string, int, []string) PromptTemplateValidationResult
}
