package prompting

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type PromptTemplateVariableCategory = workers.PromptTemplateVariableCategory

const (
	PromptTemplateVariableCategoryRoot      = workers.PromptTemplateVariableCategoryRoot
	PromptTemplateVariableCategoryInput     = workers.PromptTemplateVariableCategoryInput
	PromptTemplateVariableCategoryHistory   = workers.PromptTemplateVariableCategoryHistory
	PromptTemplateVariableCategoryContext   = workers.PromptTemplateVariableCategoryContext
	PromptTemplateVariableCategoryMapAccess = workers.PromptTemplateVariableCategoryMapAccess
	PromptTemplateVariableCategoryDoc       = workers.PromptTemplateVariableCategoryDoc
)

type PromptTemplateVariableReference = workers.PromptTemplateVariableReference
type PromptTemplateUnavailableAccessPattern = workers.PromptTemplateUnavailableAccessPattern
type PromptTemplateContract = workers.PromptTemplateContract
type PromptTemplateDiagnosticKind = workers.PromptTemplateDiagnosticKind

const (
	PromptTemplateDiagnosticKindSyntaxError         = workers.PromptTemplateDiagnosticKindSyntaxError
	PromptTemplateDiagnosticKindInvalidVariable     = workers.PromptTemplateDiagnosticKindInvalidVariable
	PromptTemplateDiagnosticKindUnavailableVariable = workers.PromptTemplateDiagnosticKindUnavailableVariable
)

type PromptTemplateDiagnostic = workers.PromptTemplateDiagnostic
type PromptTemplateValidationResult = workers.PromptTemplateValidationResult

// Service exposes prompting operations through the Workers root contract.
type Service struct{}

func NewService() workers.PromptTemplates { return Service{} }

func (Service) BuildPromptTemplateContract(inputCount int, docPaths []string) PromptTemplateContract {
	return BuildPromptTemplateContract(inputCount, docPaths)
}

func (Service) ValidatePromptTemplate(
	template string,
	inputCount int,
	docPaths []string,
) PromptTemplateValidationResult {
	return ValidatePromptTemplate(template, inputCount, docPaths)
}

func BuildPromptTemplateContract(inputCount int, docPaths []string) PromptTemplateContract {
	normalizedDocPaths := NormalizeFactoryBundledDocTargetPaths(docPaths)
	references := []PromptTemplateVariableReference{
		{
			Category:    PromptTemplateVariableCategoryRoot,
			Description: "All input tokens consumed by the selected workstation, addressed intentionally by position.",
			Example:     "{{ (index .Inputs 0).Payload }}",
			Path:        ".Inputs",
		},
		{
			Category:    PromptTemplateVariableCategoryContext,
			Description: "Execution working directory resolved for the selected workstation run.",
			Example:     "{{ .Context.WorkDir }}",
			Path:        ".Context.WorkDir",
		},
		{
			Category:    PromptTemplateVariableCategoryContext,
			Description: "Artifact output directory available during execution.",
			Example:     "{{ .Context.ArtifactDir }}",
			Path:        ".Context.ArtifactDir",
		},
		{
			Category:    PromptTemplateVariableCategoryContext,
			Description: "Resolved project context for the active run.",
			Example:     "{{ .Context.Project }}",
			Path:        ".Context.Project",
		},
		{
			Category:    PromptTemplateVariableCategoryContext,
			Description: "Active factory session id for session-scoped CLI commands such as you submit --session.",
			Example:     "{{ .Context.SessionID }}",
			Path:        ".Context.SessionID",
		},
		{
			Category:    PromptTemplateVariableCategoryMapAccess,
			Description: "Environment variables exposed to the execution context. Access keys with index.",
			Example:     "{{ index .Context.Env \"API_KEY\" }}",
			Path:        ".Context.Env[\"KEY\"]",
		},
	}

	for inputIndex := 0; inputIndex < inputCount; inputIndex++ {
		references = append(references, inputVariableReferences(inputIndex)...)
	}
	references = append(references, bundledDocVariableReferences(normalizedDocPaths)...)

	unavailablePatterns := []PromptTemplateUnavailableAccessPattern{
		{
			Example: unavailableInputExample(inputCount),
			Path:    ".Inputs[N]",
			Reason:  unavailableInputReason(inputCount),
		},
		{
			Example: "{{ (index .Inputs 0).Tags.branch }}",
			Path:    ".Inputs[N].Tags.<key>",
			Reason:  "Tag map keys are not addressable as struct fields. Use index with a quoted key, for example {{ index (index .Inputs 0).Tags \"branch\" }}.",
		},
		{
			Example: "{{ .Context.Env.API_KEY }}",
			Path:    ".Context.Env.<key>",
			Reason:  "Environment map keys are not addressable as struct fields. Use index with a quoted key, for example {{ index .Context.Env \"API_KEY\" }}.",
		},
		{
			Example: `{{ .Docs.overview }}`,
			Path:    ".Docs.<path>",
			Reason:  "Bundled doc target paths are not addressable as struct fields. Use index with a quoted target path, for example {{ index .Docs \"factory/docs/overview.md\" }}.",
		},
	}
	if len(normalizedDocPaths) == 0 {
		unavailablePatterns = append(unavailablePatterns, PromptTemplateUnavailableAccessPattern{
			Example: `{{ index .Docs "factory/docs/overview.md" }}`,
			Path:    `.Docs["factory/docs/<name>"]`,
			Reason:  "The current factory does not bundle any documentation under factory/docs/** in this editing context.",
		})
	}

	return PromptTemplateContract{
		AvailableVariables:        references,
		InputCount:                inputCount,
		UnavailableAccessPatterns: unavailablePatterns,
	}
}

func ValidatePromptTemplate(tmpl string, inputCount int, docPaths []string) PromptTemplateValidationResult {
	parsed, err := template.New("prompt").Parse(tmpl)
	if err != nil {
		return PromptTemplateValidationResult{
			Diagnostics: []PromptTemplateDiagnostic{promptTemplateSyntaxDiagnostic(err, tmpl)},
			Valid:       false,
		}
	}

	validator := promptTemplateValidator{
		docPaths:   bundledDocTargetPathSet(NormalizeFactoryBundledDocTargetPaths(docPaths)),
		inputCount: inputCount,
		seen:       make(map[string]struct{}),
	}
	rootScope := &promptValidationScope{
		bindings: make(map[string]promptValidationValue),
		dot:      promptValidationValue{kind: promptValidationValueRoot},
	}
	validator.walkList(parsed.Tree.Root, rootScope)
	validator.addRuntimeExecutionDiagnostic(parsed, tmpl, NormalizeFactoryBundledDocTargetPaths(docPaths))

	return PromptTemplateValidationResult{
		Diagnostics: validator.diagnostics,
		Valid:       len(validator.diagnostics) == 0,
	}
}

func inputVariableReferences(inputIndex int) []PromptTemplateVariableReference {
	references := inputDirectVariableReferences(inputIndex)
	references = append(references, inputRelationVariableReferences(inputIndex)...)
	references = append(references,
		PromptTemplateVariableReference{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Previous output captured for input %d retries.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).PreviousOutput }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].PreviousOutput", inputIndex),
		},
		PromptTemplateVariableReference{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Reviewer or rejection feedback recorded for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).RejectionFeedback }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].RejectionFeedback", inputIndex),
		},
	)
	references = append(references, inputHistoryVariableReferences(inputIndex)...)

	return references
}

func inputDirectVariableReferences(inputIndex int) []PromptTemplateVariableReference {
	return []PromptTemplateVariableReference{
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Human-readable work name for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).Name }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].Name", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Stable work identifier for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).WorkID }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].WorkID", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Work type identifier for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).WorkTypeID }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].WorkTypeID", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Data type identifier for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).DataType }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].DataType", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Trace identifier for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).TraceID }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].TraceID", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Parent work identifier for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).ParentID }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].ParentID", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Payload content for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).Payload }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].Payload", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Structured content parts for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).Content }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].Content", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Project resolved for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).Project }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].Project", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Tag metadata for input %d. Access keys with index.", inputIndex),
			Example:     fmt.Sprintf("{{ index (index .Inputs %d).Tags \"branch\" }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].Tags[\"KEY\"]", inputIndex),
		},
	}
}

func inputRelationVariableReferences(inputIndex int) []PromptTemplateVariableReference {
	return []PromptTemplateVariableReference{
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("Relation records attached to input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).Relations }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].Relations", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("First relation type attached to input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index (index .Inputs %d).Relations 0).Type }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].Relations[0].Type", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("First related target work identifier attached to input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index (index .Inputs %d).Relations 0).TargetWorkID }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].Relations[0].TargetWorkID", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryInput,
			Description: fmt.Sprintf("First required related state attached to input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index (index .Inputs %d).Relations 0).RequiredState }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].Relations[0].RequiredState", inputIndex),
		},
	}
}

func inputHistoryVariableReferences(inputIndex int) []PromptTemplateVariableReference {
	return []PromptTemplateVariableReference{
		{
			Category:    PromptTemplateVariableCategoryHistory,
			Description: fmt.Sprintf("Retry and failure history for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).History }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].History", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryHistory,
			Description: fmt.Sprintf("Current attempt number for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).History.AttemptNumber }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].History.AttemptNumber", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryHistory,
			Description: fmt.Sprintf("Most recent failure message for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).History.LastError }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].History.LastError", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryHistory,
			Description: fmt.Sprintf("Total historical failure count for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).History.FailureCount }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].History.FailureCount", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryHistory,
			Description: fmt.Sprintf("Total visit count across transitions for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).History.TotalVisits }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].History.TotalVisits", inputIndex),
		},
		{
			Category:    PromptTemplateVariableCategoryHistory,
			Description: fmt.Sprintf("Failure records captured for input %d.", inputIndex),
			Example:     fmt.Sprintf("{{ (index .Inputs %d).History.FailureLog }}", inputIndex),
			Path:        fmt.Sprintf(".Inputs[%d].History.FailureLog", inputIndex),
		},
	}
}

func unavailableInputExample(inputCount int) string {
	if inputCount == 0 {
		return "{{ (index .Inputs 0).Payload }}"
	}
	return fmt.Sprintf("{{ (index .Inputs %d).Payload }}", inputCount)
}

func unavailableInputReason(inputCount int) string {
	if inputCount == 0 {
		return "The selected workstation does not consume any authored work inputs, so direct .Inputs indexing is unavailable in this editing context."
	}
	return fmt.Sprintf("The selected workstation consumes %d input(s), so only .Inputs indexes 0 through %d are available in this editing context.", inputCount, inputCount-1)
}

type promptValidationValueKind int

const (
	promptValidationValueUnknown promptValidationValueKind = iota
	promptValidationValueRoot
	promptValidationValueInputsSlice
	promptValidationValueToken
	promptValidationValueHistory
	promptValidationValueContext
	promptValidationValueTagsMap
	promptValidationValueEnvMap
	promptValidationValueDocsMap
	promptValidationValueRelationsSlice
	promptValidationValueRelation
	promptValidationValueFailureLog
	promptValidationValueContent
	promptValidationValueScalar
)

type promptValidationValue struct {
	displayPath string
	kind        promptValidationValueKind
}

type promptValidationScope struct {
	bindings map[string]promptValidationValue
	dot      promptValidationValue
	parent   *promptValidationScope
}

func (s *promptValidationScope) lookup(name string) (promptValidationValue, bool) {
	for current := s; current != nil; current = current.parent {
		if value, ok := current.bindings[name]; ok {
			return value, true
		}
	}
	return promptValidationValue{}, false
}

type promptTemplateValidator struct {
	diagnostics []PromptTemplateDiagnostic
	docPaths    map[string]struct{}
	inputCount  int
	seen        map[string]struct{}
}

func (v *promptTemplateValidator) addRuntimeExecutionDiagnostic(parsed *template.Template, tmpl string, docPaths []string) {
	if parsed == nil {
		return
	}

	var rendered bytes.Buffer
	if err := parsed.Execute(&rendered, buildPromptValidationData(v.inputCount, docPaths)); err != nil {
		diagnostic, ok := promptTemplateExecutionDiagnostic(err, tmpl)
		if !ok {
			return
		}
		if v.hasDiagnosticForSource(diagnostic.SourceText) {
			return
		}
		v.addDiagnostic(diagnostic)
	}
}

func (v *promptTemplateValidator) walkList(list *parse.ListNode, scope *promptValidationScope) {
	if list == nil {
		return
	}
	for _, node := range list.Nodes {
		v.walkNode(node, scope)
	}
}

func (v *promptTemplateValidator) walkNode(node parse.Node, scope *promptValidationScope) {
	switch typed := node.(type) {
	case *parse.ActionNode:
		v.resolvePipe(typed.Pipe, scope)
	case *parse.IfNode:
		v.resolvePipe(typed.Pipe, scope)
		v.walkList(typed.List, scope.child(scope.dot))
		v.walkList(typed.ElseList, scope.child(scope.dot))
	case *parse.RangeNode:
		sequenceValue := v.resolvePipe(typed.Pipe, scope)
		rangeScope := scope.child(rangeElementValue(sequenceValue))
		bindRangeDecls(rangeScope, typed.Pipe, sequenceValue)
		v.walkList(typed.List, rangeScope)
		v.walkList(typed.ElseList, scope.child(scope.dot))
	case *parse.WithNode:
		value := v.resolvePipe(typed.Pipe, scope)
		withScope := scope.child(value)
		v.walkList(typed.List, withScope)
		v.walkList(typed.ElseList, scope.child(scope.dot))
	case *parse.TemplateNode:
		if typed.Pipe != nil {
			v.resolvePipe(typed.Pipe, scope)
		}
	}
}

func (v *promptTemplateValidator) resolvePipe(pipe *parse.PipeNode, scope *promptValidationScope) promptValidationValue {
	if pipe == nil {
		return promptValidationValue{kind: promptValidationValueUnknown}
	}
	var result promptValidationValue
	for _, cmd := range pipe.Cmds {
		result = v.resolveCommand(cmd, scope)
	}
	return result
}

func (v *promptTemplateValidator) resolveCommand(cmd *parse.CommandNode, scope *promptValidationScope) promptValidationValue {
	if cmd == nil || len(cmd.Args) == 0 {
		return promptValidationValue{kind: promptValidationValueUnknown}
	}
	if ident, ok := cmd.Args[0].(*parse.IdentifierNode); ok {
		if ident.Ident == "index" {
			return v.resolveIndexCommand(cmd, scope)
		}
		for _, arg := range cmd.Args[1:] {
			v.resolveArgument(arg, scope)
		}
		return promptValidationValue{kind: promptValidationValueScalar}
	}
	value := v.resolveArgument(cmd.Args[0], scope)
	for _, arg := range cmd.Args[1:] {
		v.resolveArgument(arg, scope)
	}
	return value
}

func (v *promptTemplateValidator) resolveIndexCommand(cmd *parse.CommandNode, scope *promptValidationScope) promptValidationValue {
	if len(cmd.Args) < 3 {
		return promptValidationValue{kind: promptValidationValueUnknown}
	}
	current := v.resolveArgument(cmd.Args[1], scope)
	for _, arg := range cmd.Args[2:] {
		switch current.kind {
		case promptValidationValueInputsSlice:
			index, ok := literalInteger(arg)
			if !ok {
				current = promptValidationValue{kind: promptValidationValueToken, displayPath: ".Inputs[*]"}
				continue
			}
			path := fmt.Sprintf(".Inputs[%d]", index)
			source := fmt.Sprintf("(index .Inputs %d)", index)
			if index < 0 || index >= v.inputCount {
				v.addDiagnostic(PromptTemplateDiagnostic{
					Kind:        PromptTemplateDiagnosticKindUnavailableVariable,
					Message:     unavailableInputReason(v.inputCount),
					Path:        path,
					SourceText:  source,
					StartOffset: int(arg.Position()),
					EndOffset:   int(arg.Position()) + len(strconv.Itoa(index)) - 1,
				})
				current = promptValidationValue{kind: promptValidationValueUnknown, displayPath: path}
				continue
			}
			current = promptValidationValue{kind: promptValidationValueToken, displayPath: path}
		case promptValidationValueTagsMap:
			current = promptValidationValue{kind: promptValidationValueScalar, displayPath: current.displayPath}
		case promptValidationValueEnvMap:
			current = promptValidationValue{kind: promptValidationValueScalar, displayPath: current.displayPath}
		case promptValidationValueDocsMap:
			key, ok := literalString(arg)
			if !ok {
				current = promptValidationValue{kind: promptValidationValueScalar, displayPath: current.displayPath + "[*]"}
				continue
			}
			path := fmt.Sprintf(`.Docs[%q]`, key)
			source := fmt.Sprintf(`index .Docs %q`, key)
			if _, available := v.docPaths[key]; !available {
				v.addDiagnostic(PromptTemplateDiagnostic{
					Kind:        PromptTemplateDiagnosticKindUnavailableVariable,
					Message:     unavailableBundledDocReason(key),
					Path:        path,
					SourceText:  source,
					StartOffset: int(arg.Position()),
					EndOffset:   int(arg.Position()) + len(strconv.Quote(key)) - 1,
				})
				current = promptValidationValue{kind: promptValidationValueUnknown, displayPath: path}
				continue
			}
			current = promptValidationValue{kind: promptValidationValueScalar, displayPath: path}
		case promptValidationValueRelationsSlice:
			current = promptValidationValue{kind: promptValidationValueRelation, displayPath: current.displayPath + "[*]"}
		default:
			current = promptValidationValue{kind: promptValidationValueUnknown, displayPath: current.displayPath}
		}
	}
	return current
}

func (v *promptTemplateValidator) resolveArgument(arg parse.Node, scope *promptValidationScope) promptValidationValue {
	switch typed := arg.(type) {
	case *parse.DotNode:
		return scope.dot
	case *parse.FieldNode:
		return v.resolveFieldChain(scope.dot, typed.Ident, typed.Position(), "."+strings.Join(typed.Ident, "."))
	case *parse.VariableNode:
		return v.resolveVariableNode(typed, scope)
	case *parse.ChainNode:
		base := v.resolveArgument(typed.Node, scope)
		return v.resolveFieldChain(base, typed.Field, typed.Position(), chainDisplay(base.displayPath, typed.Field))
	case *parse.PipeNode:
		return v.resolvePipe(typed, scope)
	case *parse.CommandNode:
		return v.resolveCommand(typed, scope)
	case *parse.StringNode, *parse.NumberNode, *parse.BoolNode, *parse.NilNode:
		return promptValidationValue{kind: promptValidationValueScalar}
	default:
		return promptValidationValue{kind: promptValidationValueUnknown}
	}
}

func (v *promptTemplateValidator) resolveVariableNode(node *parse.VariableNode, scope *promptValidationScope) promptValidationValue {
	if len(node.Ident) == 0 {
		return promptValidationValue{kind: promptValidationValueUnknown}
	}
	baseName := node.Ident[0]
	if baseName == "$" {
		return v.resolveFieldChain(promptValidationValue{kind: promptValidationValueRoot, displayPath: "$"}, node.Ident[1:], node.Position(), "$."+strings.Join(node.Ident[1:], "."))
	}
	base, ok := scope.lookup(baseName)
	if !ok {
		v.addDiagnostic(PromptTemplateDiagnostic{
			Kind:        PromptTemplateDiagnosticKindInvalidVariable,
			Message:     fmt.Sprintf("Template variable %s is not defined in this scope.", baseName),
			Path:        baseName,
			SourceText:  strings.Join(node.Ident, "."),
			StartOffset: int(node.Position()),
			EndOffset:   int(node.Position()) + len(strings.Join(node.Ident, ".")) - 1,
		})
		return promptValidationValue{kind: promptValidationValueUnknown, displayPath: baseName}
	}
	if len(node.Ident) == 1 {
		return base
	}
	return v.resolveFieldChain(base, node.Ident[1:], node.Position(), strings.Join(node.Ident, "."))
}

func (v *promptTemplateValidator) resolveFieldChain(base promptValidationValue, fields []string, pos parse.Pos, display string) promptValidationValue {
	current := base
	if current.displayPath == "" {
		current.displayPath = display
	}
	for _, field := range fields {
		nextPath := fieldPath(current.displayPath, field)
		next, ok := v.resolveNextFieldValue(current, field, nextPath, pos)
		if !ok {
			return promptValidationValue{kind: promptValidationValueUnknown, displayPath: nextPath}
		}
		current = next
	}
	return current
}

func (v *promptTemplateValidator) resolveNextFieldValue(
	current promptValidationValue,
	field string,
	nextPath string,
	pos parse.Pos,
) (promptValidationValue, bool) {
	switch current.kind {
	case promptValidationValueRoot:
		return v.resolveRootField(field, nextPath, pos)
	case promptValidationValueToken:
		return v.resolveNamedField(field, nextPath, pos, "input token", resolveTokenField)
	case promptValidationValueHistory:
		return v.resolveNamedField(field, nextPath, pos, "prompt history", resolveHistoryField)
	case promptValidationValueContext:
		return v.resolveNamedField(field, nextPath, pos, "prompt context", resolveContextField)
	case promptValidationValueRelation:
		return v.resolveNamedField(field, nextPath, pos, "relation", resolveRelationField)
	case promptValidationValueTagsMap, promptValidationValueEnvMap, promptValidationValueDocsMap:
		v.addCollectionAccessDiagnostic(pos, nextPath, fmt.Sprintf("%s is a map. Access keys with index and a quoted key instead of dot notation.", current.displayPath))
		return promptValidationValue{}, false
	case promptValidationValueInputsSlice, promptValidationValueRelationsSlice, promptValidationValueFailureLog, promptValidationValueContent:
		v.addCollectionAccessDiagnostic(pos, nextPath, fmt.Sprintf("%s is a collection. Index or range it before reading %s.", current.displayPath, field))
		return promptValidationValue{}, false
	default:
		if current.kind != promptValidationValueUnknown {
			v.addUnknownFieldDiagnostic(pos, nextPath, field, "scalar value")
		}
		return promptValidationValue{}, false
	}
}

func (v *promptTemplateValidator) resolveRootField(field, nextPath string, pos parse.Pos) (promptValidationValue, bool) {
	switch field {
	case "Inputs":
		return promptValidationValue{kind: promptValidationValueInputsSlice, displayPath: ".Inputs"}, true
	case "Context":
		return promptValidationValue{kind: promptValidationValueContext, displayPath: ".Context"}, true
	case "Docs":
		return promptValidationValue{kind: promptValidationValueDocsMap, displayPath: ".Docs"}, true
	default:
		v.addUnknownFieldDiagnostic(pos, nextPath, field, "prompt root")
		return promptValidationValue{}, false
	}
}

func (v *promptTemplateValidator) resolveNamedField(
	field string,
	nextPath string,
	pos parse.Pos,
	subject string,
	resolve func(string, string) promptValidationValue,
) (promptValidationValue, bool) {
	next := resolve(field, nextPath)
	if next.kind != promptValidationValueUnknown {
		return next, true
	}
	v.addUnknownFieldDiagnostic(pos, nextPath, field, subject)
	return promptValidationValue{}, false
}

func (v *promptTemplateValidator) addCollectionAccessDiagnostic(pos parse.Pos, nextPath, message string) {
	v.addDiagnostic(PromptTemplateDiagnostic{
		Kind:        PromptTemplateDiagnosticKindInvalidVariable,
		Message:     message,
		Path:        nextPath,
		SourceText:  nextPath,
		StartOffset: int(pos),
		EndOffset:   int(pos) + len(nextPath) - 1,
	})
}

func (v *promptTemplateValidator) addUnknownFieldDiagnostic(pos parse.Pos, path, field, subject string) {
	v.addDiagnostic(PromptTemplateDiagnostic{
		Kind:        PromptTemplateDiagnosticKindInvalidVariable,
		Message:     fmt.Sprintf("%q is not an available field on %s.", field, subject),
		Path:        path,
		SourceText:  path,
		StartOffset: int(pos),
		EndOffset:   int(pos) + len(path) - 1,
	})
}

func (v *promptTemplateValidator) addDiagnostic(diagnostic PromptTemplateDiagnostic) {
	key := fmt.Sprintf("%s|%s|%s|%d", diagnostic.Kind, diagnostic.Path, diagnostic.Message, diagnostic.StartOffset)
	if _, exists := v.seen[key]; exists {
		return
	}
	v.seen[key] = struct{}{}
	v.diagnostics = append(v.diagnostics, diagnostic)
}

func (v *promptTemplateValidator) hasDiagnosticForSource(sourceText string) bool {
	if sourceText == "" {
		return false
	}
	normalized := normalizeDiagnosticSourceText(sourceText)
	for _, diagnostic := range v.diagnostics {
		if normalizeDiagnosticSourceText(diagnostic.SourceText) == normalized {
			return true
		}
	}
	return false
}

func (s *promptValidationScope) child(dot promptValidationValue) *promptValidationScope {
	return &promptValidationScope{
		bindings: make(map[string]promptValidationValue),
		dot:      dot,
		parent:   s,
	}
}

func bindRangeDecls(scope *promptValidationScope, pipe *parse.PipeNode, sequence promptValidationValue) {
	if pipe == nil || len(pipe.Decl) == 0 {
		return
	}
	element := rangeElementValue(sequence)
	switch len(pipe.Decl) {
	case 1:
		scope.bindings[pipe.Decl[0].Ident[0]] = element
	default:
		scope.bindings[pipe.Decl[0].Ident[0]] = promptValidationValue{kind: promptValidationValueScalar, displayPath: pipe.Decl[0].Ident[0]}
		scope.bindings[pipe.Decl[len(pipe.Decl)-1].Ident[0]] = element
	}
}

func rangeElementValue(sequence promptValidationValue) promptValidationValue {
	switch sequence.kind {
	case promptValidationValueInputsSlice:
		return promptValidationValue{kind: promptValidationValueToken, displayPath: ".Inputs[*]"}
	case promptValidationValueRelationsSlice:
		return promptValidationValue{kind: promptValidationValueRelation, displayPath: ".Relations[*]"}
	default:
		return promptValidationValue{kind: promptValidationValueUnknown, displayPath: sequence.displayPath}
	}
}

func resolveTokenField(field, path string) promptValidationValue {
	switch field {
	case "Name", "WorkID", "WorkTypeID", "DataType", "TraceID", "ParentID", "Project", "Payload", "PreviousOutput", "RejectionFeedback":
		return promptValidationValue{kind: promptValidationValueScalar, displayPath: path}
	case "Tags":
		return promptValidationValue{kind: promptValidationValueTagsMap, displayPath: path}
	case "Relations":
		return promptValidationValue{kind: promptValidationValueRelationsSlice, displayPath: path}
	case "Content":
		return promptValidationValue{kind: promptValidationValueContent, displayPath: path}
	case "History":
		return promptValidationValue{kind: promptValidationValueHistory, displayPath: path}
	default:
		return promptValidationValue{kind: promptValidationValueUnknown, displayPath: path}
	}
}

func resolveHistoryField(field, path string) promptValidationValue {
	switch field {
	case "LastError", "FailureCount", "TotalVisits", "AttemptNumber":
		return promptValidationValue{kind: promptValidationValueScalar, displayPath: path}
	case "FailureLog":
		return promptValidationValue{kind: promptValidationValueFailureLog, displayPath: path}
	default:
		return promptValidationValue{kind: promptValidationValueUnknown, displayPath: path}
	}
}

func resolveContextField(field, path string) promptValidationValue {
	switch field {
	case "WorkDir", "ArtifactDir", "Project", "SessionID":
		return promptValidationValue{kind: promptValidationValueScalar, displayPath: path}
	case "Env":
		return promptValidationValue{kind: promptValidationValueEnvMap, displayPath: path}
	default:
		return promptValidationValue{kind: promptValidationValueUnknown, displayPath: path}
	}
}

func resolveRelationField(field, path string) promptValidationValue {
	switch field {
	case "Type", "TargetWorkID", "RequiredState":
		return promptValidationValue{kind: promptValidationValueScalar, displayPath: path}
	default:
		return promptValidationValue{kind: promptValidationValueUnknown, displayPath: path}
	}
}

func literalInteger(node parse.Node) (int, bool) {
	number, ok := node.(*parse.NumberNode)
	if !ok || !number.IsInt {
		return 0, false
	}
	return int(number.Int64), true
}

func literalString(node parse.Node) (string, bool) {
	text, ok := node.(*parse.StringNode)
	if !ok {
		return "", false
	}
	return text.Text, true
}

func fieldPath(base, field string) string {
	if base == "" {
		return field
	}
	if strings.HasSuffix(base, "]") {
		return base + "." + field
	}
	return base + "." + field
}

func chainDisplay(base string, fields []string) string {
	if base == "" {
		return "." + strings.Join(fields, ".")
	}
	return fieldPath(base, strings.Join(fields, "."))
}

func normalizeDiagnosticSourceText(sourceText string) string {
	return strings.Trim(sourceText, "() ")
}

func promptTemplateSyntaxDiagnostic(err error, tmpl string) PromptTemplateDiagnostic {
	line, humanMessage := parseGoTemplateParseError(err.Error())
	message := humanMessage
	if line > 0 {
		message = fmt.Sprintf("line %d: %s", line, humanMessage)
	}

	diagnostic := PromptTemplateDiagnostic{
		Kind:    PromptTemplateDiagnosticKindSyntaxError,
		Message: message,
	}
	if line > 0 {
		if startOffset, endOffset, ok := promptTemplateLineInclusiveByteOffsets(tmpl, line); ok {
			diagnostic.StartOffset = startOffset
			diagnostic.EndOffset = endOffset
		}
	}
	return diagnostic
}

func parseGoTemplateParseError(text string) (line int, humanMessage string) {
	const prefix = "template: prompt:"
	if !strings.HasPrefix(text, prefix) {
		return 0, text
	}

	rest := strings.TrimPrefix(text, prefix)
	linePart, afterLine, found := strings.Cut(rest, ":")
	if !found {
		return 0, rest
	}

	parsedLine, err := strconv.Atoi(linePart)
	if err != nil {
		return 0, rest
	}

	if columnPart, afterColumn, foundColumn := strings.Cut(afterLine, ":"); foundColumn {
		if _, columnErr := strconv.Atoi(columnPart); columnErr == nil {
			return parsedLine, strings.TrimSpace(afterColumn)
		}
	}

	return parsedLine, strings.TrimSpace(afterLine)
}

func promptTemplateLineInclusiveByteOffsets(tmpl string, lineNumber int) (startOffset int, endOffset int, ok bool) {
	if lineNumber < 1 {
		return 0, 0, false
	}

	currentLine := 1
	lineStart := 0
	for byteIndex := 0; byteIndex <= len(tmpl); byteIndex++ {
		if currentLine == lineNumber {
			lineStart = byteIndex
			break
		}
		if byteIndex >= len(tmpl) {
			return 0, 0, false
		}
		if tmpl[byteIndex] == '\n' {
			currentLine++
		}
	}

	lineEnd := lineStart
	for lineEnd < len(tmpl) && tmpl[lineEnd] != '\n' {
		lineEnd++
	}

	if lineEnd < lineStart {
		return 0, 0, false
	}
	if lineEnd == lineStart {
		return lineStart + 1, lineStart + 1, true
	}

	return lineStart + 1, lineEnd, true
}

func promptTemplateExecutionDiagnostic(err error, tmpl string) (PromptTemplateDiagnostic, bool) {
	text := err.Error()
	sourceText := executionSourceText(text)
	reason := executionReason(text)
	if !strings.Contains(text, "error calling index:") {
		return PromptTemplateDiagnostic{}, false
	}
	path := executionPath(sourceText)
	kind := PromptTemplateDiagnosticKindInvalidVariable
	if strings.Contains(reason, "index out of range") && strings.Contains(sourceText, ".Inputs") {
		kind = PromptTemplateDiagnosticKindUnavailableVariable
	}

	message := "Template execution would fail for this reference."
	if reason != "" {
		message = fmt.Sprintf("Template execution would fail: %s.", reason)
	}

	startOffset := 0
	endOffset := 0
	if sourceText != "" {
		if index := strings.Index(tmpl, sourceText); index >= 0 {
			startOffset = index + 1
			endOffset = index + len(sourceText)
		} else {
			startOffset = 1
			endOffset = len(sourceText)
		}
	}
	if path == "" {
		path = sourceText
	}

	return PromptTemplateDiagnostic{
		Kind:        kind,
		Message:     message,
		Path:        path,
		SourceText:  sourceText,
		StartOffset: startOffset,
		EndOffset:   endOffset,
	}, true
}

func executionSourceText(text string) string {
	start := strings.Index(text, "at <")
	if start == -1 {
		return ""
	}
	start += len("at <")
	end := strings.Index(text[start:], ">:")
	if end == -1 {
		return ""
	}
	return text[start : start+end]
}

func executionReason(text string) string {
	last := strings.LastIndex(text, ": ")
	if last == -1 || last+2 >= len(text) {
		return text
	}
	return text[last+2:]
}

func executionPath(sourceText string) string {
	switch {
	case strings.HasPrefix(sourceText, "index .Context.Env "):
		return ".Context.Env"
	case strings.Contains(sourceText, ".Relations"):
		return ".Relations"
	case strings.Contains(sourceText, "index .Inputs "):
		return ".Inputs"
	default:
		return sourceText
	}
}
