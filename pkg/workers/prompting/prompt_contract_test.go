package prompting

import (
	"strings"
	"testing"
)

func TestBuildPromptTemplateContract_ListsSelectedInputVariablesAndUnavailablePatterns(t *testing.T) {
	contract := BuildPromptTemplateContract(1)

	if contract.InputCount != 1 {
		t.Fatalf("InputCount = %d, want 1", contract.InputCount)
	}
	for _, want := range []string{
		".Inputs[0].Name",
		".Inputs[0].WorkID",
		".Inputs[0].WorkTypeID",
		".Inputs[0].DataType",
		".Inputs[0].TraceID",
		".Inputs[0].ParentID",
		".Inputs[0].Project",
		".Inputs[0].Payload",
		".Inputs[0].Content",
		".Inputs[0].Tags[\"KEY\"]",
		".Inputs[0].Relations",
		".Inputs[0].Relations[0].Type",
		".Inputs[0].Relations[0].TargetWorkID",
		".Inputs[0].Relations[0].RequiredState",
		".Inputs[0].PreviousOutput",
		".Inputs[0].RejectionFeedback",
		".Inputs[0].History",
		".Inputs[0].History.AttemptNumber",
		".Inputs[0].History.TotalVisits",
		".Inputs[0].History.FailureCount",
		".Inputs[0].History.LastError",
		".Inputs[0].History.FailureLog",
	} {
		if !hasVariablePath(contract.AvailableVariables, want) {
			t.Fatalf("available variables missing %q from %#v", want, contract.AvailableVariables)
		}
	}
	if !hasVariablePath(contract.AvailableVariables, ".Context.Project") {
		t.Fatalf("available variables = %#v, want .Context.Project", contract.AvailableVariables)
	}
	if !hasUnavailablePattern(contract.UnavailableAccessPatterns, ".Inputs[N]") {
		t.Fatalf("unavailable patterns = %#v, want .Inputs[N]", contract.UnavailableAccessPatterns)
	}
}

func TestValidatePromptTemplate_AcceptsSupportedInputAndContextReferences(t *testing.T) {
	result := ValidatePromptTemplate(`Project: {{ .Context.Project }}
Work: {{ (index .Inputs 0).Name }} {{ (index .Inputs 0).WorkID }} {{ (index .Inputs 0).WorkTypeID }} {{ (index .Inputs 0).DataType }} {{ (index .Inputs 0).TraceID }} {{ (index .Inputs 0).ParentID }} {{ (index .Inputs 0).Payload }} {{ (index .Inputs 0).Content }} {{ (index .Inputs 0).Project }}
Tags: {{ index (index .Inputs 0).Tags "branch" }}
Relations: {{ (index .Inputs 0).Relations }} {{ (index (index .Inputs 0).Relations 0).Type }} {{ (index (index .Inputs 0).Relations 0).TargetWorkID }} {{ (index (index .Inputs 0).Relations 0).RequiredState }}
Retry: {{ (index .Inputs 0).PreviousOutput }} {{ (index .Inputs 0).RejectionFeedback }}
History: {{ (index .Inputs 0).History }} {{ (index .Inputs 0).History.AttemptNumber }} {{ (index .Inputs 0).History.TotalVisits }} {{ (index .Inputs 0).History.FailureCount }} {{ (index .Inputs 0).History.LastError }} {{ (index .Inputs 0).History.FailureLog }}
{{ range $i, $input := .Inputs }}{{ $input.WorkID }}{{ end }}`, 1)

	if !result.Valid {
		t.Fatalf("Valid = false, diagnostics = %#v", result.Diagnostics)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

func TestValidatePromptTemplate_SeparatesSyntaxErrorsFromUnavailableVariables(t *testing.T) {
	syntaxResult := ValidatePromptTemplate(`{{ if .Context.Project }}`, 1)
	if syntaxResult.Valid {
		t.Fatal("syntaxResult.Valid = true, want false")
	}
	if len(syntaxResult.Diagnostics) != 1 || syntaxResult.Diagnostics[0].Kind != PromptTemplateDiagnosticKindSyntaxError {
		t.Fatalf("syntax diagnostics = %#v, want one syntax error", syntaxResult.Diagnostics)
	}

	unavailableResult := ValidatePromptTemplate(`{{ (index .Inputs 1).Payload }}`, 1)
	if unavailableResult.Valid {
		t.Fatal("unavailableResult.Valid = true, want false")
	}
	if len(unavailableResult.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one unavailable-variable diagnostic", unavailableResult.Diagnostics)
	}
	diagnostic := unavailableResult.Diagnostics[0]
	if diagnostic.Kind != PromptTemplateDiagnosticKindUnavailableVariable {
		t.Fatalf("diagnostic kind = %q, want %q", diagnostic.Kind, PromptTemplateDiagnosticKindUnavailableVariable)
	}
	if diagnostic.Path != ".Inputs[1]" {
		t.Fatalf("diagnostic path = %q, want .Inputs[1]", diagnostic.Path)
	}
}

func TestValidatePromptTemplate_FlagsInvalidFieldsAndMapDotAccess(t *testing.T) {
	result := ValidatePromptTemplate(`{{ (index .Inputs 0).Unknown }}
{{ (index .Inputs 0).Tags.branch }}
{{ .Context.Env.API_KEY }}`, 1)

	if result.Valid {
		t.Fatal("Valid = true, want false")
	}
	if len(result.Diagnostics) != 3 {
		t.Fatalf("diagnostics = %#v, want 3", result.Diagnostics)
	}
	if result.Diagnostics[0].Kind != PromptTemplateDiagnosticKindInvalidVariable {
		t.Fatalf("first diagnostic kind = %q, want %q", result.Diagnostics[0].Kind, PromptTemplateDiagnosticKindInvalidVariable)
	}
}

func TestValidatePromptTemplate_RejectsTemplatesThatRuntimeWouldFailToExecute(t *testing.T) {
	testCases := []struct {
		name               string
		template           string
		wantKind           PromptTemplateDiagnosticKind
		wantSourceFragment string
		wantMessage        string
	}{
		{
			name:               "env map index requires string key",
			template:           `{{ index .Context.Env 0 }}`,
			wantKind:           PromptTemplateDiagnosticKindInvalidVariable,
			wantSourceFragment: `index .Context.Env 0`,
			wantMessage:        "value has type int; should be string",
		},
		{
			name:               "inputs slice index rejects string keys",
			template:           `{{ (index .Inputs "0").Payload }}`,
			wantKind:           PromptTemplateDiagnosticKindInvalidVariable,
			wantSourceFragment: `index .Inputs "0"`,
			wantMessage:        "cannot index slice/array with type string",
		},
		{
			name:               "relations slice index rejects string keys",
			template:           `{{ (index (index .Inputs 0).Relations "x").Type }}`,
			wantKind:           PromptTemplateDiagnosticKindInvalidVariable,
			wantSourceFragment: `index (index .Inputs 0).Relations "x"`,
			wantMessage:        "cannot index slice/array with type string",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := ValidatePromptTemplate(testCase.template, 1)

			if result.Valid {
				t.Fatalf("Valid = true, diagnostics = %#v", result.Diagnostics)
			}
			if len(result.Diagnostics) != 1 {
				t.Fatalf("diagnostics = %#v, want 1", result.Diagnostics)
			}

			diagnostic := result.Diagnostics[0]
			if diagnostic.Kind != testCase.wantKind {
				t.Fatalf("diagnostic kind = %q, want %q", diagnostic.Kind, testCase.wantKind)
			}
			if diagnostic.SourceText != testCase.wantSourceFragment {
				t.Fatalf("diagnostic source = %q, want %q", diagnostic.SourceText, testCase.wantSourceFragment)
			}
			if diagnostic.Message == "" || !containsText(diagnostic.Message, testCase.wantMessage) {
				t.Fatalf("diagnostic message = %q, want substring %q", diagnostic.Message, testCase.wantMessage)
			}
			if diagnostic.StartOffset < 0 || diagnostic.EndOffset < diagnostic.StartOffset {
				t.Fatalf("diagnostic offsets = (%d, %d), want ordered offsets", diagnostic.StartOffset, diagnostic.EndOffset)
			}
		})
	}
}

func TestValidatePromptTemplate_UsesOneBasedOffsetsForRuntimeExecutionDiagnostics(t *testing.T) {
	result := ValidatePromptTemplate(`x{{ index .Context.Env 0 }}`, 1)

	if result.Valid {
		t.Fatalf("Valid = true, diagnostics = %#v", result.Diagnostics)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want 1", result.Diagnostics)
	}

	diagnostic := result.Diagnostics[0]
	if diagnostic.SourceText != `index .Context.Env 0` {
		t.Fatalf("diagnostic source = %q, want %q", diagnostic.SourceText, `index .Context.Env 0`)
	}
	if diagnostic.StartOffset != 5 || diagnostic.EndOffset != 24 {
		t.Fatalf(
			"diagnostic offsets = (%d, %d), want (5, 24)",
			diagnostic.StartOffset,
			diagnostic.EndOffset,
		)
	}
}

func containsText(have, want string) bool {
	return strings.Contains(have, want)
}

func hasVariablePath(references []PromptTemplateVariableReference, want string) bool {
	for _, reference := range references {
		if reference.Path == want {
			return true
		}
	}
	return false
}

func hasUnavailablePattern(patterns []PromptTemplateUnavailableAccessPattern, want string) bool {
	for _, pattern := range patterns {
		if pattern.Path == want {
			return true
		}
	}
	return false
}
