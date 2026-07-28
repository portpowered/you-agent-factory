package prompting

import (
	"strings"
	"testing"
)

func TestBuildPromptTemplateContract_ListsSelectedInputVariablesAndUnavailablePatterns(t *testing.T) {
	contract := BuildPromptTemplateContract(1, nil)

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
	for _, wantContext := range []string{".Context.Project", ".Context.SessionID"} {
		if !hasVariablePath(contract.AvailableVariables, wantContext) {
			t.Fatalf("available variables missing %q from %#v", wantContext, contract.AvailableVariables)
		}
	}
	if !hasUnavailablePattern(contract.UnavailableAccessPatterns, ".Inputs[N]") {
		t.Fatalf("unavailable patterns = %#v, want .Inputs[N]", contract.UnavailableAccessPatterns)
	}
}

func TestValidatePromptTemplate_AcceptsContextSessionID(t *testing.T) {
	result := ValidatePromptTemplate(`you submit --session {{ .Context.SessionID }} --work follow-up`, 1, nil)

	if !result.Valid {
		t.Fatalf("Valid = false, diagnostics = %#v", result.Diagnostics)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

func TestBuildPromptTemplateContract_ListsContextSessionIDMetadata(t *testing.T) {
	contract := BuildPromptTemplateContract(0, nil)

	reference, ok := findVariableReference(contract.AvailableVariables, ".Context.SessionID")
	if !ok {
		t.Fatalf("available variables = %#v, want .Context.SessionID", contract.AvailableVariables)
	}
	if reference.Category != PromptTemplateVariableCategoryContext {
		t.Fatalf("category = %q, want %q", reference.Category, PromptTemplateVariableCategoryContext)
	}
	if reference.Example != "{{ .Context.SessionID }}" {
		t.Fatalf("example = %q, want {{ .Context.SessionID }}", reference.Example)
	}
	if reference.Description == "" {
		t.Fatal("description is empty, want non-empty guidance for session-scoped CLI usage")
	}
}

func TestValidatePromptTemplate_AcceptsSupportedInputAndContextReferences(t *testing.T) {
	result := ValidatePromptTemplate(`Project: {{ .Context.Project }}
Work: {{ (index .Inputs 0).Name }} {{ (index .Inputs 0).WorkID }} {{ (index .Inputs 0).WorkTypeID }} {{ (index .Inputs 0).DataType }} {{ (index .Inputs 0).TraceID }} {{ (index .Inputs 0).ParentID }} {{ (index .Inputs 0).Payload }} {{ (index .Inputs 0).Content }} {{ (index .Inputs 0).Project }}
Tags: {{ index (index .Inputs 0).Tags "branch" }}
Relations: {{ (index .Inputs 0).Relations }} {{ (index (index .Inputs 0).Relations 0).Type }} {{ (index (index .Inputs 0).Relations 0).TargetWorkID }} {{ (index (index .Inputs 0).Relations 0).RequiredState }}
Retry: {{ (index .Inputs 0).PreviousOutput }} {{ (index .Inputs 0).RejectionFeedback }}
History: {{ (index .Inputs 0).History }} {{ (index .Inputs 0).History.AttemptNumber }} {{ (index .Inputs 0).History.TotalVisits }} {{ (index .Inputs 0).History.FailureCount }} {{ (index .Inputs 0).History.LastError }} {{ (index .Inputs 0).History.FailureLog }}
{{ range $i, $input := .Inputs }}{{ $input.WorkID }}{{ end }}`, 1, nil)

	if !result.Valid {
		t.Fatalf("Valid = false, diagnostics = %#v", result.Diagnostics)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

func TestValidatePromptTemplate_NormalizesSyntaxErrorMessages(t *testing.T) {
	tmpl := `{{ if .Context.Project }}`
	result := ValidatePromptTemplate(tmpl, 1, nil)

	if result.Valid {
		t.Fatal("Valid = true, want false")
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one syntax error", result.Diagnostics)
	}

	diagnostic := result.Diagnostics[0]
	if diagnostic.Kind != PromptTemplateDiagnosticKindSyntaxError {
		t.Fatalf("diagnostic kind = %q, want %q", diagnostic.Kind, PromptTemplateDiagnosticKindSyntaxError)
	}
	if diagnostic.Message != "line 1: unexpected EOF" {
		t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message, "line 1: unexpected EOF")
	}
	if strings.Contains(diagnostic.Message, "template: prompt:") {
		t.Fatalf("diagnostic message = %q, must not include template parse prefix", diagnostic.Message)
	}
	if diagnostic.StartOffset != 1 || diagnostic.EndOffset != len(tmpl) {
		t.Fatalf(
			"diagnostic offsets = (%d, %d), want (1, %d)",
			diagnostic.StartOffset,
			diagnostic.EndOffset,
			len(tmpl),
		)
	}

	multilineTemplate := "Valid line\n{{ bad"
	multilineResult := ValidatePromptTemplate(multilineTemplate, 1, nil)
	if len(multilineResult.Diagnostics) != 1 {
		t.Fatalf("multiline diagnostics = %#v, want one syntax error", multilineResult.Diagnostics)
	}
	multilineDiagnostic := multilineResult.Diagnostics[0]
	if multilineDiagnostic.Message != `line 2: function "bad" not defined` {
		t.Fatalf("multiline message = %q, want line 2 function error", multilineDiagnostic.Message)
	}
	if multilineDiagnostic.StartOffset != len("Valid line\n")+1 {
		t.Fatalf("multiline start offset = %d, want %d", multilineDiagnostic.StartOffset, len("Valid line\n")+1)
	}
	if multilineDiagnostic.EndOffset != len(multilineTemplate) {
		t.Fatalf("multiline end offset = %d, want %d", multilineDiagnostic.EndOffset, len(multilineTemplate))
	}
}

func TestValidatePromptTemplate_SeparatesSyntaxErrorsFromUnavailableVariables(t *testing.T) {
	syntaxResult := ValidatePromptTemplate(`{{ if .Context.Project }}`, 1, nil)
	if syntaxResult.Valid {
		t.Fatal("syntaxResult.Valid = true, want false")
	}
	if len(syntaxResult.Diagnostics) != 1 || syntaxResult.Diagnostics[0].Kind != PromptTemplateDiagnosticKindSyntaxError {
		t.Fatalf("syntax diagnostics = %#v, want one syntax error", syntaxResult.Diagnostics)
	}
	if !strings.HasPrefix(syntaxResult.Diagnostics[0].Message, "line 1: ") {
		t.Fatalf("syntax message = %q, want line-based prefix", syntaxResult.Diagnostics[0].Message)
	}

	unavailableResult := ValidatePromptTemplate(`{{ (index .Inputs 1).Payload }}`, 1, nil)
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
{{ .Context.Env.API_KEY }}`, 1, nil)

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
			result := ValidatePromptTemplate(testCase.template, 1, nil)

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
	result := ValidatePromptTemplate(`x{{ index .Context.Env 0 }}`, 1, nil)

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
	_, ok := findVariableReference(references, want)
	return ok
}

func findVariableReference(references []PromptTemplateVariableReference, want string) (PromptTemplateVariableReference, bool) {
	for _, reference := range references {
		if reference.Path == want {
			return reference, true
		}
	}
	return PromptTemplateVariableReference{}, false
}

func TestBuildPromptTemplateContract_ListsBundledDocReferences(t *testing.T) {
	contract := BuildPromptTemplateContract(0, []string{
		"factory/docs/guide.md",
		"factory/docs/overview.md",
	})

	reference, ok := findVariableReference(contract.AvailableVariables, `.Docs["factory/docs/overview.md"]`)
	if !ok {
		t.Fatalf("available variables = %#v, want bundled doc reference", contract.AvailableVariables)
	}
	if reference.Category != PromptTemplateVariableCategoryDoc {
		t.Fatalf("category = %q, want %q", reference.Category, PromptTemplateVariableCategoryDoc)
	}
	if reference.Example != `{{ index .Docs "factory/docs/overview.md" }}` {
		t.Fatalf("example = %q, want doc index example", reference.Example)
	}
}

func TestValidatePromptTemplate_AcceptsBundledDocReference(t *testing.T) {
	docPaths := []string{"factory/docs/overview.md"}
	result := ValidatePromptTemplate(`Read this: {{ index .Docs "factory/docs/overview.md" }}`, 0, docPaths)

	if !result.Valid {
		t.Fatalf("Valid = false, diagnostics = %#v", result.Diagnostics)
	}
}

func TestValidatePromptTemplate_RejectsMissingBundledDocReference(t *testing.T) {
	result := ValidatePromptTemplate(`{{ index .Docs "factory/docs/missing.md" }}`, 0, []string{
		"factory/docs/overview.md",
	})

	if result.Valid {
		t.Fatal("Valid = true, want false")
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one unavailable-variable diagnostic", result.Diagnostics)
	}
	if result.Diagnostics[0].Kind != PromptTemplateDiagnosticKindUnavailableVariable {
		t.Fatalf("diagnostic kind = %q, want %q", result.Diagnostics[0].Kind, PromptTemplateDiagnosticKindUnavailableVariable)
	}
}

func hasUnavailablePattern(patterns []PromptTemplateUnavailableAccessPattern, want string) bool {
	for _, pattern := range patterns {
		if pattern.Path == want {
			return true
		}
	}
	return false
}
