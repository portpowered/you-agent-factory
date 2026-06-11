package workflowruntime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/workcontent"
)

func TestRun_SimpleFinalWorkflow_ProjectsStructuredPrimaryResult(t *testing.T) {
	source := readFixture(t, "simple-final.workflow.js")
	args := marshalArgs(t, map[string]any{
		"subject": "workflows",
		"count":   3,
		"prefix":  "you",
	})

	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "simple-final.workflow.js",
		SessionID: "session-simple-final",
		Args:      args,
		Metadata: map[string]string{
			"name":        "simple-final",
			"description": "returns a structured final value",
		},
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}

	first := runSuccessful(t, req)
	second := runSuccessful(t, req)
	if string(first.Value.JSON) != string(second.Value.JSON) {
		t.Fatalf("value drift across runs: first=%s second=%s", first.Value.JSON, second.Value.JSON)
	}

	projected := projectPrimaryJSON(t, req.SessionID, first.Value)
	want := map[string]any{
		"label":       "simple-final",
		"description": "returns a structured final value",
		"subject":     "workflows",
		"repeat":      float64(3),
		"echo":        "you:workflows",
	}
	assertProjectedFields(t, projected, want)
}

func TestRun_WorkflowFinal_ProjectsStructuredPrimaryResult(t *testing.T) {
	source := readFixture(t, "workflow-final.workflow.js")
	args := marshalArgs(t, map[string]any{
		"subject": "workflows",
		"count":   3,
		"prefix":  "you",
	})

	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "workflow-final.workflow.js",
		SessionID: "session-workflow-final",
		Args:      args,
		Metadata: map[string]string{
			"name":        "workflow-final",
			"description": "completes through workflow.final",
		},
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}

	outcome := runSuccessful(t, req)
	projected := projectPrimaryJSON(t, req.SessionID, outcome.Value)
	want := map[string]any{
		"label":       "workflow-final",
		"description": "completes through workflow.final",
		"subject":     "workflows",
		"repeat":      float64(3),
		"echo":        "you:workflows",
		"mechanism":   "workflow.final",
	}
	assertProjectedFields(t, projected, want)
}

func TestRun_WorkflowFinalAndReturn_PrefersWorkflowFinal(t *testing.T) {
	source := readFixture(t, "workflow-final-and-return.workflow.js")
	args := marshalArgs(t, map[string]any{
		"subject": "workflows",
	})

	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "workflow-final-and-return.workflow.js",
		SessionID: "session-workflow-final-and-return",
		Args:      args,
		Metadata: map[string]string{
			"name": "workflow-final-and-return",
		},
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}

	outcome := runSuccessful(t, req)
	projected := projectPrimaryJSON(t, req.SessionID, outcome.Value)
	assertProjectedFields(t, projected, map[string]any{
		"label":       "workflow-final-and-return",
		"mechanism":   "workflow.final",
		"subject":     "workflows",
	})
	if projected["mechanism"] == "return" {
		t.Fatalf("projected mechanism = return, want workflow.final precedence")
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(content)
}

func marshalArgs(t *testing.T, args map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return raw
}

func runSuccessful(t *testing.T, req workflowruntime.Request) workflowruntime.Outcome {
	t.Helper()
	outcome, err := workflowruntime.Run(t.Context(), req, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	validation := workflowresult.ValidateTypedValue(outcome.Value)
	if validation.HasIssues() {
		t.Fatalf("result validation = %#v", validation.Issues)
	}
	return outcome
}

func projectPrimaryJSON(t *testing.T, sessionID string, value workflowresult.TypedValue) map[string]any {
	t.Helper()
	parts, projection := workflowresult.ProjectPrimaryResult(sessionID, value, nil)
	if projection.HasIssues() {
		t.Fatalf("primary projection validation = %#v", projection.Issues)
	}
	if len(parts) != 1 || parts[0].Type != interfaces.WorkContentPartTypeJSON {
		t.Fatalf("parts = %#v", parts)
	}
	var projected map[string]any
	if err := json.Unmarshal(parts[0].JSON, &projected); err != nil {
		t.Fatalf("unmarshal projected json: %v", err)
	}
	generated := workcontent.GeneratedPtrFromParts(parts)
	if generated == nil {
		t.Fatal("expected generated primary result content")
	}
	return projected
}

func assertProjectedFields(t *testing.T, projected map[string]any, want map[string]any) {
	t.Helper()
	for key, wantValue := range want {
		if projected[key] != wantValue {
			t.Fatalf("projected[%q] = %#v, want %#v", key, projected[key], wantValue)
		}
	}
}
