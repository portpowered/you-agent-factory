package workers

import (
	"reflect"
	"strings"
	"testing"
	"time"

	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestPromptRenderer_BasicInterpolation(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{{
		ID: "tok-1",
		Color: interfaces.TokenColor{
			WorkID:     "work-123",
			WorkTypeID: "code-changes",
			TraceID:    "trace-abc",
			Tags:       map[string]string{"language": "go"},
			Payload:    []byte("Implement the feature"),
		},
	}}

	tmpl := "Work {{ (index .Inputs 0).WorkID }} ({{ (index .Inputs 0).WorkTypeID }}): {{ (index .Inputs 0).Payload }}\nLanguage: {{ index (index .Inputs 0).Tags \"language\" }}"

	result, err := renderer.Render(tmpl, tokens, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "work-123") {
		t.Errorf("expected work ID in output, got: %s", result)
	}
	if !strings.Contains(result, "Implement the feature") {
		t.Errorf("expected payload in output, got: %s", result)
	}
	if !strings.Contains(result, "Language: go") {
		t.Errorf("expected tag interpolation, got: %s", result)
	}
}

func TestPromptData_ExposesOnlyCanonicalTemplateRoots(t *testing.T) {
	dataType := reflect.TypeOf(PromptData{})
	fields := make([]string, 0, dataType.NumField())
	for i := 0; i < dataType.NumField(); i++ {
		fields = append(fields, dataType.Field(i).Name)
	}

	want := []string{"Inputs", "Context"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("PromptData fields = %v, want %v", fields, want)
	}
}

func TestPromptRenderer_TopLevelTokenAliasFailsWhileInputsRender(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{{
		ID: "tok-1",
		Color: interfaces.TokenColor{
			WorkID:  "work-123",
			Payload: []byte("Implement the feature"),
		},
	}}

	legacyTemplate := "Work {{ ." + "WorkID }}"
	if _, err := renderer.Render(legacyTemplate, tokens, nil); err == nil {
		t.Fatal("expected top-level WorkID alias to fail")
	}

	result, err := renderer.Render("Work {{ (index .Inputs 0).WorkID }}: {{ (index .Inputs 0).Payload }}", tokens, nil)
	if err != nil {
		t.Fatalf("expected canonical Inputs render to succeed: %v", err)
	}
	if result != "Work work-123: Implement the feature" {
		t.Fatalf("canonical Inputs render = %q, want %q", result, "Work work-123: Implement the feature")
	}
}

func TestPromptRenderer_RetryAwarePrompt(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{{
		ID: "tok-2",
		Color: interfaces.TokenColor{
			WorkID:  "work-456",
			Payload: []byte("Write a design document"),
			Tags: map[string]string{
				"_last_output":        "Previous draft content",
				"_rejection_feedback": "Missing error handling section",
			},
		},
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{"tr-design": 2},
			ConsecutiveFailures: map[string]int{},
			LastError:           "",
			FailureLog: []interfaces.FailureRecord{
				{TransitionID: "tr-design", Timestamp: time.Now(), Error: "timeout", Attempt: 1},
			},
		},
	}}

	tmpl := `{{ (index .Inputs 0).Payload }}
{{ if gt (index .Inputs 0).History.AttemptNumber 1 -}}
This is attempt {{ (index .Inputs 0).History.AttemptNumber }}. Previous output: {{ (index .Inputs 0).PreviousOutput }}
Reviewer feedback: {{ (index .Inputs 0).RejectionFeedback }}
{{ end -}}`

	result, err := renderer.Render(tmpl, tokens, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// AttemptNumber = TotalVisits(2) + 1 = 3
	if !strings.Contains(result, "attempt 3") {
		t.Errorf("expected attempt 3 in output, got: %s", result)
	}
	if !strings.Contains(result, "Previous draft content") {
		t.Errorf("expected previous output, got: %s", result)
	}
	if !strings.Contains(result, "Missing error handling section") {
		t.Errorf("expected rejection feedback, got: %s", result)
	}
}

func TestPromptRenderer_ContextFields(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{{
		ID: "tok-3",
		Color: interfaces.TokenColor{
			WorkID: "work-789",
			Tags:   map[string]string{},
		},
	}}

	wfCtx := &factory_context.FactoryContext{
		WorkDirectory: "/workspace/project",
		ArtifactDir:   "/workspace/artifacts",
		EnvVars:       map[string]string{"GOPRIVATE": "github.com/portpowered/*"},
	}

	tmpl := `WorkDir: {{ .Context.WorkDir }}
ArtifactDir: {{ .Context.ArtifactDir }}
GOPRIVATE: {{ index .Context.Env "GOPRIVATE" }}`

	result, err := renderer.Render(tmpl, tokens, wfCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "/workspace/project") {
		t.Errorf("expected work dir, got: %s", result)
	}
	if !strings.Contains(result, "/workspace/artifacts") {
		t.Errorf("expected artifact dir, got: %s", result)
	}
	if !strings.Contains(result, "github.com/portpowered/*") {
		t.Errorf("expected GOPRIVATE env var, got: %s", result)
	}
}

func TestPromptRenderer_ContextProjectPrefersExplicitContextOverTokenTag(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{{
		ID: "tok-project",
		Color: interfaces.TokenColor{
			WorkID:   "work-project",
			DataType: interfaces.DataTypeWork,
			Tags: map[string]string{
				factory_context.ProjectTagKey: "token-project",
			},
		},
	}}
	wfCtx := &factory_context.FactoryContext{ProjectID: "context-project"}

	result, err := renderer.Render(
		`Context={{ .Context.Project }} Token={{ (index .Inputs 0).Project }}`,
		tokens,
		wfCtx,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Context=context-project Token=token-project" {
		t.Fatalf("project rendering = %q, want explicit context with per-token project preserved", result)
	}
}

func TestPromptRenderer_ContextProjectFallsBackToFirstWorkInputProjectTag(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{
		{
			ID: "resource-slot",
			Color: interfaces.TokenColor{
				WorkID:   "slot-1",
				DataType: interfaces.DataTypeResource,
				Tags: map[string]string{
					factory_context.ProjectTagKey: "resource-project",
				},
			},
		},
		{
			ID: "tok-first-work",
			Color: interfaces.TokenColor{
				WorkID:   "work-first",
				DataType: interfaces.DataTypeWork,
				Tags: map[string]string{
					factory_context.ProjectTagKey: "first-work-project",
				},
			},
		},
		{
			ID: "tok-second-work",
			Color: interfaces.TokenColor{
				WorkID:   "work-second",
				DataType: interfaces.DataTypeWork,
				Tags: map[string]string{
					factory_context.ProjectTagKey: "second-work-project",
				},
			},
		},
	}

	result, err := renderer.Render(`{{ .Context.Project }}`, tokens, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "first-work-project" {
		t.Fatalf("context project fallback = %q, want first non-resource input project", result)
	}
}

func TestPromptRenderer_ContextProjectIgnoresResourceOnlyProjectTag(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{{
		ID: "resource-slot",
		Color: interfaces.TokenColor{
			WorkID:   "slot-1",
			DataType: interfaces.DataTypeResource,
			Tags: map[string]string{
				factory_context.ProjectTagKey: "resource-project",
			},
		},
	}}

	result, err := renderer.Render(`{{ .Context.Project }}`, tokens, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != factory_context.DefaultProjectID {
		t.Fatalf("resource-only context project = %q, want %q", result, factory_context.DefaultProjectID)
	}
}

func TestPromptRenderer_MissingOptionalFields(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	// Empty token — no tags, no history, no payload.
	tokens := []interfaces.Token{{
		ID:    "tok-empty",
		Color: interfaces.TokenColor{WorkID: "work-empty"},
	}}

	tmpl := `ID: {{ (index .Inputs 0).WorkID }}
Previous: {{ (index .Inputs 0).PreviousOutput }}
Feedback: {{ (index .Inputs 0).RejectionFeedback }}
Error: {{ (index .Inputs 0).History.LastError }}
Attempt: {{ (index .Inputs 0).History.AttemptNumber }}`

	result, err := renderer.Render(tmpl, tokens, nil)
	if err != nil {
		t.Fatalf("expected no error with missing optional fields, got: %v", err)
	}

	if !strings.Contains(result, "ID: work-empty") {
		t.Errorf("expected work ID, got: %s", result)
	}
	// AttemptNumber with no visits = 0 + 1 = 1
	if !strings.Contains(result, "Attempt: 1") {
		t.Errorf("expected attempt 1 for first run, got: %s", result)
	}
}

func TestPromptRenderer_NoTokens(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tmpl := "Static prompt with no token data"

	result, err := renderer.Render(tmpl, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Static prompt with no token data" {
		t.Errorf("expected static prompt, got: %s", result)
	}
}

func TestPromptRenderer_InvalidTemplate(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tmpl := "{{ .Invalid {{ broken }}"
	_, err := renderer.Render(tmpl, nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid template syntax")
	}
}

func TestPromptRenderer_MultipleInputTokens_PerVariableContext(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{
		{
			ID: "tok-prd",
			Color: interfaces.TokenColor{
				WorkID:     "work-prd",
				WorkTypeID: "prd",
				Payload:    []byte("Build the login page"),
				Tags:       map[string]string{"priority": "high"},
			},
		},
		{
			ID: "tok-review",
			Color: interfaces.TokenColor{
				WorkID:     "work-review",
				WorkTypeID: "review",
				Payload:    []byte("Review feedback: add tests"),
				Tags:       map[string]string{"reviewer": "alice"},
			},
		},
	}

	// Template accesses per-token data via .Inputs
	tmpl := `PRD: {{ (index .Inputs 0).Payload }}
Review: {{ (index .Inputs 1).Payload }}
PRD Priority: {{ index (index .Inputs 0).Tags "priority" }}
Reviewer: {{ index (index .Inputs 1).Tags "reviewer" }}`

	result, err := renderer.Render(tmpl, tokens, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "PRD: Build the login page") {
		t.Errorf("expected first token payload, got: %s", result)
	}
	if !strings.Contains(result, "Review: Review feedback: add tests") {
		t.Errorf("expected second token payload, got: %s", result)
	}
	if !strings.Contains(result, "PRD Priority: high") {
		t.Errorf("expected first token tags, got: %s", result)
	}
	if !strings.Contains(result, "Reviewer: alice") {
		t.Errorf("expected second token tags, got: %s", result)
	}
}

func TestPromptRenderer_MultipleInputTokens_DistinctContexts(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{
		{
			ID: "tok-a",
			Color: interfaces.TokenColor{
				WorkID:     "work-a",
				WorkTypeID: "type-a",
				TraceID:    "trace-a",
				ParentID:   "parent-a",
				Payload:    []byte("payload-a"),
				Tags:       map[string]string{"key": "val-a"},
			},
			History: interfaces.TokenHistory{
				TotalVisits: map[string]int{"tr-1": 1},
				LastError:   "error-a",
			},
		},
		{
			ID: "tok-b",
			Color: interfaces.TokenColor{
				WorkID:     "work-b",
				WorkTypeID: "type-b",
				TraceID:    "trace-b",
				ParentID:   "parent-b",
				Payload:    []byte("payload-b"),
				Tags:       map[string]string{"key": "val-b"},
			},
			History: interfaces.TokenHistory{
				TotalVisits: map[string]int{"tr-2": 3},
				LastError:   "error-b",
			},
		},
	}

	// Verify that Inputs[0] and Inputs[1] carry distinct per-token data
	tmpl := `A: {{ (index .Inputs 0).WorkID }} {{ (index .Inputs 0).WorkTypeID }} {{ (index .Inputs 0).TraceID }} {{ (index .Inputs 0).History.AttemptNumber }}
B: {{ (index .Inputs 1).WorkID }} {{ (index .Inputs 1).WorkTypeID }} {{ (index .Inputs 1).TraceID }} {{ (index .Inputs 1).History.AttemptNumber }}`

	result, err := renderer.Render(tmpl, tokens, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "A: work-a type-a trace-a 2") {
		t.Errorf("expected distinct context for token A, got: %s", result)
	}
	if !strings.Contains(result, "B: work-b type-b trace-b 4") {
		t.Errorf("expected distinct context for token B, got: %s", result)
	}
}

// TestPromptRenderer_ResourceToken_FirstInList verifies that when a resource token
// appears before work tokens in the input list, both tokens remain explicitly
// addressable by position through .Inputs.
func TestPromptRenderer_ResourceToken_FirstInList(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	// Simulate a dispatch where the resource token appears first.
	tokens := []interfaces.Token{
		{
			ID: "agent-slot:resource:0",
			Color: interfaces.TokenColor{
				WorkID:     "agent-slot:0",
				WorkTypeID: "agent-slot",
				DataType:   interfaces.DataTypeResource,
				Payload:    nil,
			},
		},
		{
			ID: "tok-story",
			Color: interfaces.TokenColor{
				WorkID:     "story-abc",
				WorkTypeID: "story",
				DataType:   interfaces.DataTypeWork,
				Payload:    []byte("Implement the login feature"),
			},
		},
	}

	tmpl := "Resource {{ (index .Inputs 0).WorkID }} ({{ (index .Inputs 0).DataType }}); Story {{ (index .Inputs 1).WorkID }}: {{ (index .Inputs 1).Payload }}"

	result, err := renderer.Render(tmpl, tokens, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Resource agent-slot:0 (resource)") {
		t.Errorf("expected resource token through Inputs[0], got: %s", result)
	}
	if !strings.Contains(result, "Story story-abc: Implement the login feature") {
		t.Errorf("expected work token through Inputs[1], got: %s", result)
	}
}

// TestPromptRenderer_ResourceToken_DataTypeAccessible verifies that resource tokens
// are still available via .Inputs and that their DataType field is accessible.
func TestPromptRenderer_ResourceToken_DataTypeAccessible(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{
		{
			ID: "agent-slot:resource:0",
			Color: interfaces.TokenColor{
				WorkID:   "agent-slot:0",
				DataType: interfaces.DataTypeResource,
			},
		},
		{
			ID: "tok-work",
			Color: interfaces.TokenColor{
				WorkID:   "work-xyz",
				DataType: interfaces.DataTypeWork,
				Payload:  []byte("do the thing"),
			},
		},
	}

	// Template accesses resource token via .Inputs and checks DataType.
	tmpl := `Input0ID: {{ (index .Inputs 0).WorkID }}
Input0Type: {{ (index .Inputs 0).DataType }}
Input1Type: {{ (index .Inputs 1).DataType }}
Count: {{ len .Inputs }}`

	result, err := renderer.Render(tmpl, tokens, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both tokens should appear in .Inputs.
	if !strings.Contains(result, "Count: 2") {
		t.Errorf("expected 2 tokens in Inputs, got: %s", result)
	}
	if !strings.Contains(result, "Input0ID: agent-slot:0") {
		t.Errorf("expected resource WorkID through Inputs[0], got: %s", result)
	}
	// DataType fields should be populated.
	if !strings.Contains(result, "Input0Type: resource") {
		t.Errorf("expected resource DataType for Inputs[0], got: %s", result)
	}
	if !strings.Contains(result, "Input1Type: work") {
		t.Errorf("expected work DataType for Inputs[1], got: %s", result)
	}
}

// TestPromptRenderer_AllResourceTokens verifies that resource-only dispatch data
// remains available through the canonical .Inputs root.
func TestPromptRenderer_AllResourceTokens(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{
		{
			ID: "gpu:resource:0",
			Color: interfaces.TokenColor{
				WorkID:   "gpu:0",
				DataType: interfaces.DataTypeResource,
			},
		},
	}

	tmpl := "WorkID={{ (index .Inputs 0).WorkID }} Payload={{ (index .Inputs 0).Payload }} Type={{ (index .Inputs 0).DataType }}"

	result, err := renderer.Render(tmpl, tokens, nil)
	if err != nil {
		t.Fatalf("expected graceful render with all-resource tokens, got error: %v", err)
	}

	if !strings.Contains(result, "WorkID=gpu:0") {
		t.Errorf("expected resource WorkID through Inputs[0], got: %s", result)
	}
	if !strings.Contains(result, "Type=resource") {
		t.Errorf("expected resource DataType through Inputs[0], got: %s", result)
	}
}

// TestPromptRenderer_NoTemplateSkipsResourcePayloads verifies that the no-template
// payload fallback only includes work token payloads, not resource token payloads.
func TestPromptRenderer_NoTemplateSkipsResourcePayloads(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{
		{
			ID: "agent-slot:resource:0",
			Color: interfaces.TokenColor{
				WorkID:   "agent-slot:0",
				DataType: interfaces.DataTypeResource,
				Payload:  []byte("should be ignored"),
			},
		},
		{
			ID: "tok-work",
			Color: interfaces.TokenColor{
				WorkID:   "work-123",
				DataType: interfaces.DataTypeWork,
				Payload:  []byte("real story content"),
			},
		},
	}

	// Empty template → falls back to getTokenPayloads
	result, err := renderer.Render("", tokens, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "real story content") {
		t.Errorf("expected work token payload in result, got: %s", result)
	}
	if strings.Contains(result, "should be ignored") {
		t.Errorf("resource token payload must not appear in no-template fallback, got: %s", result)
	}
}

func TestPromptRenderer_SingleToken_InputsSlicePopulated(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []interfaces.Token{{
		ID: "tok-single",
		Color: interfaces.TokenColor{
			WorkID:  "work-single",
			Payload: []byte("single payload"),
		},
	}}

	// Even with one token, .Inputs should be populated.
	tmpl := `Input0: {{ (index .Inputs 0).WorkID }} {{ (index .Inputs 0).Payload }}
Count: {{ len .Inputs }}`

	result, err := renderer.Render(tmpl, tokens, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Input0: work-single single payload") {
		t.Errorf("expected Inputs[0] fields, got: %s", result)
	}
	if !strings.Contains(result, "Count: 1") {
		t.Errorf("expected Inputs length 1, got: %s", result)
	}
}
