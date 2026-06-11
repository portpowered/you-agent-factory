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
	args, err := json.Marshal(map[string]any{
		"subject": "workflows",
		"count":   3,
		"prefix":  "you",
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

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

	first, err := workflowruntime.Run(t.Context(), req, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !first.OK {
		t.Fatalf("Run() failure = %#v", first.Failure)
	}

	second, err := workflowruntime.Run(t.Context(), req, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("Run() second pass error = %v", err)
	}
	if !second.OK {
		t.Fatalf("Run() second pass failure = %#v", second.Failure)
	}
	if string(first.Value.JSON) != string(second.Value.JSON) {
		t.Fatalf("value drift across runs: first=%s second=%s", first.Value.JSON, second.Value.JSON)
	}

	validation := workflowresult.ValidateTypedValue(first.Value)
	if validation.HasIssues() {
		t.Fatalf("result validation = %#v", validation.Issues)
	}

	parts, projection := workflowresult.ProjectPrimaryResult(req.SessionID, first.Value, nil)
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
	want := map[string]any{
		"label":       "simple-final",
		"description": "returns a structured final value",
		"subject":     "workflows",
		"repeat":      float64(3),
		"echo":        "you:workflows",
	}
	for key, wantValue := range want {
		if projected[key] != wantValue {
			t.Fatalf("projected[%q] = %#v, want %#v", key, projected[key], wantValue)
		}
	}

	generated := workcontent.GeneratedPtrFromParts(parts)
	if generated == nil {
		t.Fatal("expected generated primary result content")
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
