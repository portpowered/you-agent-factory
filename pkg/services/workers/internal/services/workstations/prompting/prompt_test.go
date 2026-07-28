package prompting

import (
	"github.com/portpowered/infinite-you/internal/testpath"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPromptRenderer_BasicInterpolation(t *testing.T) {
	renderer := &DefaultPromptRenderer{FactoryDocs: mustFactoryDocsLoader(t, platformfilesystem.Local{})}

	tokens := []factoryruntime.RuntimeToken{{
		ID: "tok-1",
		Color: factoryruntime.RuntimeTokenColor{
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

	want := []string{"Docs", "Inputs", "Context"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("PromptData fields = %v, want %v", fields, want)
	}
}

func TestPromptRenderer_TopLevelTokenAliasFailsWhileInputsRender(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []factoryruntime.RuntimeToken{{
		ID: "tok-1",
		Color: factoryruntime.RuntimeTokenColor{
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

	tokens := []factoryruntime.RuntimeToken{{
		ID: "tok-2",
		Color: factoryruntime.RuntimeTokenColor{
			WorkID:  "work-456",
			Payload: []byte("Write a design document"),
			Tags: map[string]string{
				"_last_output":        "Previous draft content",
				"_rejection_feedback": "Missing error handling section",
			},
		},
		History: factoryruntime.RuntimeTokenHistory{
			TotalVisits:         map[string]int{"tr-design": 2},
			ConsecutiveFailures: map[string]int{},
			LastError:           "",
			FailureLog: []factoryruntime.RuntimeTokenFailure{
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

func TestPromptRenderer_ResolvesCheckedInPlannerFactoryDocs(t *testing.T) {
	renderer := &DefaultPromptRenderer{FactoryDocs: mustFactoryDocsLoader(t, platformfilesystem.Local{})}
	factoryDir := testpath.MustRepoPathFromCaller(t, 0, "factory")
	factoryCtx := &workers.Context{FactoryDirectory: factoryDir}

	overview, err := renderer.Render(
		`{{ index .Docs "factory/docs/overview.md" }}`,
		nil,
		factoryCtx,
	)
	if err != nil {
		t.Fatalf("render overview doc: %v", err)
	}
	for _, want := range []string{
		"# Factory Overview",
		"you-agent-factory",
		"ideafy",
		"docs/temp/progress.md",
		"docs/temp/checklist.md",
		"docs/temp/meta.md",
		"factory/docs/batch-input-example.json",
		"you work list --session",
		"you session list",
	} {
		if !strings.Contains(overview, want) {
			t.Fatalf("overview doc missing %q", want)
		}
	}
	for _, absent := range []string{
		"Awesome-list",
		"awesome-list",
		"docs/internal/",
	} {
		if strings.Contains(overview, absent) {
			t.Fatalf("overview doc still contains stale marker %q", absent)
		}
	}

	batchInputs, err := renderer.Render(
		`{{ index .Docs "factory/docs/batch-inputs.md" }}`,
		nil,
		factoryCtx,
	)
	if err != nil {
		t.Fatalf("render batch-inputs doc: %v", err)
	}
	for _, want := range []string{
		"# Batch Inputs",
		"factory/docs/batch-input-example.json",
		"docs/temp/progress.md",
		"docs/temp/checklist.md",
		"docs/temp/meta.md",
		"you submit batch --dry-run factory/docs/batch-input-example.json --session",
		"## Verification",
	} {
		if !strings.Contains(batchInputs, want) {
			t.Fatalf("batch-inputs doc missing %q", want)
		}
	}
}

func TestPromptRenderer_RendersBundledDocReference(t *testing.T) {
	renderer := &DefaultPromptRenderer{FactoryDocs: mustFactoryDocsLoader(t, platformfilesystem.Local{})}
	factoryDir := t.TempDir()
	docsDir := filepath.Join(factoryDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "overview.md"), []byte("overview-content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := renderer.Render(
		`{{ index .Docs "factory/docs/overview.md" }}`,
		nil,
		&workers.Context{FactoryDirectory: factoryDir},
	)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result != "overview-content" {
		t.Fatalf("result = %q, want overview-content", result)
	}
}

func TestBuildPromptData_MapsFactoryContextSessionID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sessionID string
		want      string
	}{
		{name: "default session", sessionID: workers.DefaultSessionID, want: workers.DefaultSessionID},
		{name: "named session", sessionID: "session-beta", want: "session-beta"},
		{name: "blank session falls back to default", sessionID: "  ", want: workers.DefaultSessionID},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := BuildPromptData(nil, &workers.Context{SessionID: tc.sessionID})
			if data.Context.SessionID != tc.want {
				t.Fatalf("Context.SessionID = %q, want %q", data.Context.SessionID, tc.want)
			}
		})
	}
}

func TestPromptRenderer_ContextSessionID(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []factoryruntime.RuntimeToken{{
		ID: "tok-session",
		Color: factoryruntime.RuntimeTokenColor{
			WorkID: "work-session",
		},
	}}

	tests := []struct {
		name      string
		sessionID string
		want      string
	}{
		{name: "default session", sessionID: workers.DefaultSessionID, want: workers.DefaultSessionID},
		{name: "named session", sessionID: "session-beta", want: "session-beta"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wfCtx := &workers.Context{SessionID: tc.sessionID}
			result, err := renderer.Render(
				`you submit --session {{ .Context.SessionID }} --work follow-up`,
				tokens,
				wfCtx,
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := "you submit --session " + tc.want + " --work follow-up"
			if result != want {
				t.Fatalf("rendered prompt = %q, want %q", result, want)
			}
		})
	}
}

func TestPromptRenderer_ContextFields(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []factoryruntime.RuntimeToken{{
		ID: "tok-3",
		Color: factoryruntime.RuntimeTokenColor{
			WorkID: "work-789",
			Tags:   map[string]string{},
		},
	}}

	wfCtx := &workers.Context{
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

	tokens := []factoryruntime.RuntimeToken{{
		ID: "tok-project",
		Color: factoryruntime.RuntimeTokenColor{
			WorkID:   "work-project",
			DataType: factoryruntime.RuntimeTokenDataTypeWork,
			Tags: map[string]string{
				workers.ProjectTagKey: "token-project",
			},
		},
	}}
	wfCtx := &workers.Context{ProjectID: "context-project"}

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

	tokens := []factoryruntime.RuntimeToken{
		{
			ID: "resource-slot",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:   "slot-1",
				DataType: factoryruntime.RuntimeTokenDataTypeResource,
				Tags: map[string]string{
					workers.ProjectTagKey: "resource-project",
				},
			},
		},
		{
			ID: "tok-first-work",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:   "work-first",
				DataType: factoryruntime.RuntimeTokenDataTypeWork,
				Tags: map[string]string{
					workers.ProjectTagKey: "first-work-project",
				},
			},
		},
		{
			ID: "tok-second-work",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:   "work-second",
				DataType: factoryruntime.RuntimeTokenDataTypeWork,
				Tags: map[string]string{
					workers.ProjectTagKey: "second-work-project",
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

	tokens := []factoryruntime.RuntimeToken{{
		ID: "resource-slot",
		Color: factoryruntime.RuntimeTokenColor{
			WorkID:   "slot-1",
			DataType: factoryruntime.RuntimeTokenDataTypeResource,
			Tags: map[string]string{
				workers.ProjectTagKey: "resource-project",
			},
		},
	}}

	result, err := renderer.Render(`{{ .Context.Project }}`, tokens, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != workers.DefaultProjectID {
		t.Fatalf("resource-only context project = %q, want %q", result, workers.DefaultProjectID)
	}
}

func TestPromptRenderer_MissingOptionalFields(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	// Empty token — no tags, no history, no payload.
	tokens := []factoryruntime.RuntimeToken{{
		ID:    "tok-empty",
		Color: factoryruntime.RuntimeTokenColor{WorkID: "work-empty"},
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

	tokens := []factoryruntime.RuntimeToken{
		{
			ID: "tok-prd",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:     "work-prd",
				WorkTypeID: "prd",
				Payload:    []byte("Build the login page"),
				Tags:       map[string]string{"priority": "high"},
			},
		},
		{
			ID: "tok-review",
			Color: factoryruntime.RuntimeTokenColor{
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

	tokens := []factoryruntime.RuntimeToken{
		{
			ID: "tok-a",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:     "work-a",
				WorkTypeID: "type-a",
				TraceID:    "trace-a",
				ParentID:   "parent-a",
				Payload:    []byte("payload-a"),
				Tags:       map[string]string{"key": "val-a"},
			},
			History: factoryruntime.RuntimeTokenHistory{
				TotalVisits: map[string]int{"tr-1": 1},
				LastError:   "error-a",
			},
		},
		{
			ID: "tok-b",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:     "work-b",
				WorkTypeID: "type-b",
				TraceID:    "trace-b",
				ParentID:   "parent-b",
				Payload:    []byte("payload-b"),
				Tags:       map[string]string{"key": "val-b"},
			},
			History: factoryruntime.RuntimeTokenHistory{
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

func TestPromptRenderer_MultipleInputTokens_PreservesPerInputCanonicalContent(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	tokens := []factoryruntime.RuntimeToken{
		{
			ID: "tok-text",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID: "work-text",
				Content: []work.WorkContentPart{
					{Type: work.WorkContentPartTypeText, Text: "plan"},
				},
				Payload: []byte("plan"),
			},
		},
		{
			ID: "tok-mixed",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID: "work-mixed",
				Content: []work.WorkContentPart{
					{Type: work.WorkContentPartTypeText, Text: "caption"},
					{Type: work.WorkContentPartTypeImage, File: "fixtures/mockup.png"},
				},
				Payload: []byte("caption"),
			},
		},
	}

	tmpl := `{{ (index .Inputs 0).WorkID }}:{{ range (index .Inputs 0).Content }} [{{ .Type }}={{ .Text }}{{ .File }}]{{ end }}
{{ (index .Inputs 1).WorkID }}:{{ range (index .Inputs 1).Content }} [{{ .Type }}={{ .Text }}{{ .File }}]{{ end }}`

	result, err := renderer.Render(tmpl, tokens, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "work-text: [text=plan]") {
		t.Fatalf("first input content = %q, want preserved text-only input", result)
	}
	if !strings.Contains(result, "work-mixed: [text=caption] [image=fixtures/mockup.png]") {
		t.Fatalf("second input content = %q, want ordered mixed-content input", result)
	}
}

// TestPromptRenderer_ResourceToken_FirstInList verifies that when a resource token
// appears before work tokens in the input list, both tokens remain explicitly
// addressable by position through .Inputs.
func TestPromptRenderer_ResourceToken_FirstInList(t *testing.T) {
	renderer := &DefaultPromptRenderer{}

	// Simulate a dispatch where the resource token appears first.
	tokens := []factoryruntime.RuntimeToken{
		{
			ID: "agent-slot:resource:0",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:     "agent-slot:0",
				WorkTypeID: "agent-slot",
				DataType:   factoryruntime.RuntimeTokenDataTypeResource,
				Payload:    nil,
			},
		},
		{
			ID: "tok-story",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:     "story-abc",
				WorkTypeID: "story",
				DataType:   factoryruntime.RuntimeTokenDataTypeWork,
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

	tokens := []factoryruntime.RuntimeToken{
		{
			ID: "agent-slot:resource:0",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:   "agent-slot:0",
				DataType: factoryruntime.RuntimeTokenDataTypeResource,
			},
		},
		{
			ID: "tok-work",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:   "work-xyz",
				DataType: factoryruntime.RuntimeTokenDataTypeWork,
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

	tokens := []factoryruntime.RuntimeToken{
		{
			ID: "gpu:resource:0",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:   "gpu:0",
				DataType: factoryruntime.RuntimeTokenDataTypeResource,
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

	tokens := []factoryruntime.RuntimeToken{
		{
			ID: "agent-slot:resource:0",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:   "agent-slot:0",
				DataType: factoryruntime.RuntimeTokenDataTypeResource,
				Payload:  []byte("should be ignored"),
			},
		},
		{
			ID: "tok-work",
			Color: factoryruntime.RuntimeTokenColor{
				WorkID:   "work-123",
				DataType: factoryruntime.RuntimeTokenDataTypeWork,
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

	tokens := []factoryruntime.RuntimeToken{{
		ID: "tok-single",
		Color: factoryruntime.RuntimeTokenColor{
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
