package workflowruntime_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

func TestRun_ProgressPrimitives_EmitsOrderedRuntimeRecords(t *testing.T) {
	source := readFixture(t, "progress-primitives.workflow.js")
	maxTokens := int64(4096)
	maxRunDurationMs := int64(120000)
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxTokens = &maxTokens
	policy.MaxRunDurationMs = &maxRunDurationMs
	policy.SandboxMode = "read-only"

	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "progress-primitives.workflow.js",
		SessionID: "session-progress-primitives",
		Args:      marshalArgs(t, map[string]any{}),
		Metadata: map[string]string{
			"name": "progress-primitives",
		},
		Policy: policy,
	}

	var artifactKinds []string
	hooks := workflowruntime.Hooks{
		OnArtifact: func(kind string, content json.RawMessage) error {
			artifactKinds = append(artifactKinds, kind)
			return nil
		},
	}

	first := runSuccessfulWithHooks(t, req, hooks)
	if len(artifactKinds) != 1 || artifactKinds[0] != "log" {
		t.Fatalf("artifact hook kinds = %#v, want [log]", artifactKinds)
	}

	second := runSuccessfulWithHooks(t, req, workflowruntime.Hooks{})

	if len(first.Records) != 7 {
		t.Fatalf("record count = %d, want 7", len(first.Records))
	}
	assertRecordSequences(t, first.Records)

	wantKinds := []string{
		workflowruntime.RecordKindPhase,
		workflowruntime.RecordKindLog,
		workflowruntime.RecordKindLog,
		workflowruntime.RecordKindPhase,
		workflowruntime.RecordKindArtifact,
		workflowruntime.RecordKindCheckpoint,
		workflowruntime.RecordKindBudget,
	}
	for i, want := range wantKinds {
		if first.Records[i].Kind != want {
			t.Fatalf("records[%d].kind = %q, want %q", i, first.Records[i].Kind, want)
		}
	}

	if first.Records[0].Phase == nil || first.Records[0].Phase.Name != "setup" {
		t.Fatalf("phase record = %#v", first.Records[0].Phase)
	}
	if first.Records[1].Log == nil || first.Records[1].Log.Message != "starting workflow" {
		t.Fatalf("log record = %#v", first.Records[1].Log)
	}
	if first.Records[1].Log.Fields["step"] != float64(1) {
		t.Fatalf("log fields = %#v, want step=1", first.Records[1].Log.Fields)
	}
	if first.Records[2].Log == nil || first.Records[2].Log.Message != "workflow step" {
		t.Fatalf("workflow.log record = %#v", first.Records[2].Log)
	}
	if first.Records[3].Phase == nil || first.Records[3].Phase.Name != "execute" {
		t.Fatalf("second phase record = %#v", first.Records[3].Phase)
	}

	artifact := first.Records[4].Artifact
	if artifact == nil {
		t.Fatal("expected artifact record")
	}
	wantURI := workflowresult.FormatArtifactURI(req.SessionID, "artifact-1")
	if artifact.URI != wantURI {
		t.Fatalf("artifact uri = %q, want %q", artifact.URI, wantURI)
	}
	if artifact.Kind != "log" || artifact.Label != "step-output" {
		t.Fatalf("artifact metadata = %#v", artifact)
	}
	if artifact.Visibility != "WORKFLOW_RUNTIME" {
		t.Fatalf("artifact visibility = %q, want WORKFLOW_RUNTIME", artifact.Visibility)
	}
	if artifact.ContentHash == "" || artifact.SizeBytes <= 0 {
		t.Fatalf("artifact content metadata = %#v", artifact)
	}

	checkpoint := first.Records[5].Checkpoint
	if checkpoint == nil {
		t.Fatal("expected checkpoint record")
	}
	if checkpoint.ID != "checkpoint-1" || checkpoint.Label != "after-artifact" {
		t.Fatalf("checkpoint record = %#v", checkpoint)
	}
	if checkpoint.State["artifactRef"] != wantURI {
		t.Fatalf("checkpoint state artifactRef = %#v, want %q", checkpoint.State["artifactRef"], wantURI)
	}
	if checkpoint.State["step"] != float64(2) {
		t.Fatalf("checkpoint state step = %#v", checkpoint.State["step"])
	}
	if strings.Contains(checkpoint.Summary, "goja") || strings.Contains(checkpoint.Summary, "Runtime") {
		t.Fatalf("checkpoint summary exposes VM internals: %q", checkpoint.Summary)
	}

	budget := first.Records[6].Budget
	if budget == nil {
		t.Fatal("expected budget record")
	}
	if budget.MaxAgents != policy.MaxAgents || budget.Concurrency != policy.Concurrency {
		t.Fatalf("budget record = %#v, want maxAgents=%d concurrency=%d", budget, policy.MaxAgents, policy.Concurrency)
	}
	if budget.MaxTokens == nil || *budget.MaxTokens != maxTokens {
		t.Fatalf("budget maxTokens = %#v, want %d", budget.MaxTokens, maxTokens)
	}
	if budget.MaxRunDurationMs == nil || *budget.MaxRunDurationMs != maxRunDurationMs {
		t.Fatalf("budget maxRunDurationMs = %#v, want %d", budget.MaxRunDurationMs, maxRunDurationMs)
	}
	if budget.SandboxMode != "read-only" {
		t.Fatalf("budget sandboxMode = %q, want read-only", budget.SandboxMode)
	}

	projected := projectPrimaryJSON(t, req.SessionID, first.Value)
	if projected["artifactRef"] != wantURI {
		t.Fatalf("projected artifactRef = %#v, want %q", projected["artifactRef"], wantURI)
	}
	budgetValue, ok := projected["budget"].(map[string]any)
	if !ok {
		t.Fatalf("projected budget = %#v, want object", projected["budget"])
	}
	if budgetValue["maxAgents"] != float64(policy.MaxAgents) {
		t.Fatalf("projected budget maxAgents = %#v", budgetValue["maxAgents"])
	}
	if budgetValue["maxTokens"] != float64(maxTokens) {
		t.Fatalf("projected budget maxTokens = %#v", budgetValue["maxTokens"])
	}

	if recordsJSON(first.Records) != recordsJSON(second.Records) {
		t.Fatalf("record drift across runs:\nfirst=%s\nsecond=%s", recordsJSON(first.Records), recordsJSON(second.Records))
	}
}

func runSuccessfulWithHooks(t *testing.T, req workflowruntime.Request, hooks workflowruntime.Hooks) workflowruntime.Outcome {
	t.Helper()
	outcome, err := workflowruntime.Run(t.Context(), req, hooks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	return outcome
}

func assertRecordSequences(t *testing.T, records []workflowruntime.RuntimeRecord) {
	t.Helper()
	for i, record := range records {
		want := i + 1
		if record.Sequence != want {
			t.Fatalf("records[%d].sequence = %d, want %d", i, record.Sequence, want)
		}
	}
}

func recordsJSON(records []workflowruntime.RuntimeRecord) string {
	raw, err := json.Marshal(records)
	if err != nil {
		return ""
	}
	return string(raw)
}
