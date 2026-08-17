package prompting

import (
	"fmt"
	"text/template"
)

func validateStructuredResultAvailability(parsed *template.Template, present []bool) []PromptTemplateDiagnostic {
	if parsed == nil {
		return nil
	}

	validator := promptTemplateValidator{
		inputCount:                   len(present),
		structuredResultAvailability: present,
		seen:                         make(map[string]struct{}),
	}
	rootScope := &promptValidationScope{
		bindings: make(map[string]promptValidationValue),
		dot:      promptValidationValue{kind: promptValidationValueRoot},
	}
	validator.walkList(parsed.Tree.Root, rootScope)
	return validator.structuredResultDiagnostics
}

func (v *promptTemplateValidator) addStructuredResultDiagnostic(diagnostic PromptTemplateDiagnostic) {
	key := fmt.Sprintf("structured-result|%s|%s|%d", diagnostic.Path, diagnostic.Message, diagnostic.StartOffset)
	if _, exists := v.seen[key]; exists {
		return
	}
	v.seen[key] = struct{}{}
	v.structuredResultDiagnostics = append(v.structuredResultDiagnostics, diagnostic)
}

func (v *promptTemplateValidator) resolveTokenField(token promptValidationValue, field, path string) promptValidationValue {
	switch field {
	case "Name", "WorkID", "WorkTypeID", "DataType", "TraceID", "ParentID", "Project", "Payload", "PreviousOutput", "RejectionFeedback":
		return promptValidationValue{kind: promptValidationValueScalar, displayPath: path}
	case "StructuredResult":
		v.validateStructuredResultUse(token, path)
		return promptValidationValue{
			kind:            promptValidationValueStructuredResult,
			displayPath:     path,
			inputIndex:      token.inputIndex,
			inputIndexKnown: token.inputIndexKnown,
		}
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

func (v *promptTemplateValidator) validateStructuredResultUse(token promptValidationValue, path string) {
	if v.structuredResultAvailability == nil {
		return
	}

	if token.inputIndexKnown {
		if token.inputIndex >= 0 && token.inputIndex < len(v.structuredResultAvailability) && v.structuredResultAvailability[token.inputIndex] {
			return
		}
		v.addMissingStructuredResultDiagnostic(token.inputIndex, path)
		return
	}

	for index, present := range v.structuredResultAvailability {
		if !present {
			v.addMissingStructuredResultDiagnostic(index, path)
		}
	}
}

func (v *promptTemplateValidator) addMissingStructuredResultDiagnostic(inputIndex int, path string) {
	message := fmt.Sprintf(
		"structured result unavailable for input %d: %s is present only when the upstream workstation produces a schema-validated JSON result",
		inputIndex,
		path,
	)
	v.addStructuredResultDiagnostic(PromptTemplateDiagnostic{
		Kind:        PromptTemplateDiagnosticKindUnavailableVariable,
		Message:     message,
		Path:        path,
		SourceText:  path,
		StartOffset: 0,
		EndOffset:   len(path),
	})
}
