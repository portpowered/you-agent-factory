package workflowruntime_test

import (
	"strings"
	"testing"

	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime/callbehavior"
)

func TestCallBehavior_WorkflowFinalInventoryMatchesExecution(t *testing.T) {
	record := callBehaviorRecord(t, "workflow.final")
	assertCallBehaviorRecordDoesNotExposeHostContext(t, record)

	if record.Return == nil || record.Return.SyncType != "undefined" {
		t.Fatalf("workflow.final return = %#v, want sync undefined", record.Return)
	}
	if len(record.Parameters) != 1 {
		t.Fatalf("workflow.final parameters = %#v, want one optional value parameter", record.Parameters)
	}
	if record.Parameters[0].Required {
		t.Fatalf("workflow.final value parameter required = true, want optional per inventory")
	}
	if record.Determinism == "" {
		t.Fatal("workflow.final missing determinism note")
	}

	t.Run("records terminal value from optional argument", func(t *testing.T) {
		outcome := runInlineWorkflow(t, "workflow-final-terminal", `
workflow.final({ label: "terminal", mechanism: "workflow.final", count: 2 });
return { label: "returned", mechanism: "return", count: 1 };
`)
		projected := projectPrimaryJSON(t, "session-workflow-final-terminal", outcome.Value)
		assertProjectedFields(t, projected, map[string]any{
			"label":     "terminal",
			"mechanism": "workflow.final",
			"count":     float64(2),
		})
	})

	t.Run("final wins over returned workflow value", func(t *testing.T) {
		outcome := runInlineWorkflow(t, "workflow-final-precedence", readFixture(t, "workflow-final-and-return.workflow.js"))
		projected := projectPrimaryJSON(t, "session-workflow-final-precedence", outcome.Value)
		assertProjectedFields(t, projected, map[string]any{
			"mechanism": "workflow.final",
		})
		if projected["mechanism"] == "return" {
			t.Fatalf("projected mechanism = return, want workflow.final precedence from inventory")
		}
	})
}

func TestCallBehavior_WorkflowCheckpointInventoryMatchesExecution(t *testing.T) {
	record := callBehaviorRecord(t, "workflow.checkpoint")
	assertCallBehaviorRecordDoesNotExposeHostContext(t, record)

	if len(record.Parameters) != 1 || !record.Parameters[0].Required || record.Parameters[0].Type != "object" {
		t.Fatalf("workflow.checkpoint parameters = %#v, want one required object", record.Parameters)
	}
	if record.Return == nil || record.Return.SyncType != "undefined" {
		t.Fatalf("workflow.checkpoint return = %#v, want sync undefined", record.Return)
	}
	if len(record.EmittedRecords) != 1 || record.EmittedRecords[0] != "checkpoint" {
		t.Fatalf("workflow.checkpoint emittedRecords = %v, want [checkpoint]", record.EmittedRecords)
	}
	if record.ResumeNotes == "" {
		t.Fatal("workflow.checkpoint missing resume notes")
	}

	t.Run("emits checkpoint record with label and state", func(t *testing.T) {
		outcome := runInlineWorkflow(t, "workflow-checkpoint-success", `
workflow.checkpoint({ label: "after-step", state: { step: 2, tag: "alpha" } });
return { ok: true };
`)
		checkpoint := findCheckpointRecord(t, outcome.Records, "after-step")
		if checkpoint.State["step"] != float64(2) || checkpoint.State["tag"] != "alpha" {
			t.Fatalf("checkpoint state = %#v, want step=2 tag=alpha", checkpoint.State)
		}
	})

	for _, errCase := range record.Errors {
		t.Run(errCase.Condition, func(t *testing.T) {
			source := checkpointErrorSource(errCase.Condition)
			if source == "" {
				t.Fatalf("missing inline source for inventory error condition %q", errCase.Condition)
			}
			outcome := runInlineWorkflowFailure(t, "workflow-checkpoint-"+errCase.Condition, source)
			if outcome.Failure.Code != workflowruntime.CodeScriptError {
				t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodeScriptError)
			}
			if !strings.Contains(outcome.Failure.Message, errCase.Message) {
				t.Fatalf("failure message = %q, want inventory message %q", outcome.Failure.Message, errCase.Message)
			}
			if hasCheckpointRecords(outcome.Records) {
				t.Fatalf("records = %#v, want no checkpoint record after %q", outcome.Records, errCase.Condition)
			}
		})
	}
}

func TestCallBehavior_WorkflowResumeStateInventoryMatchesExecution(t *testing.T) {
	record := callBehaviorRecord(t, "workflow.resumeState")
	assertCallBehaviorRecordDoesNotExposeHostContext(t, record)

	if len(record.Parameters) != 0 {
		t.Fatalf("workflow.resumeState parameters = %#v, want zero parameters", record.Parameters)
	}
	if record.Return == nil || record.Return.SyncType != "object-or-undefined" {
		t.Fatalf("workflow.resumeState return = %#v, want object-or-undefined", record.Return)
	}
	if record.ResumeNotes == "" {
		t.Fatal("workflow.resumeState missing resume notes")
	}

	t.Run("returns undefined when resume state absent", func(t *testing.T) {
		outcome := runInlineWorkflow(t, "workflow-resume-state-absent", `
const resumed = workflow.resumeState();
return { hasResumeState: resumed !== undefined };
`)
		projected := projectPrimaryJSON(t, "session-workflow-resume-state-absent", outcome.Value)
		if projected["hasResumeState"] != false {
			t.Fatalf("projected hasResumeState = %#v, want false when absent", projected["hasResumeState"])
		}
	})

	t.Run("returns bound checkpoint state on resumed session", func(t *testing.T) {
		req := workflowruntime.Request{
			Source: `
const resumed = workflow.resumeState();
return {
  step: resumed ? resumed.step : null,
  firstLabel: resumed ? resumed.firstLabel : null,
};
`,
			SourceRef: "inline-resume-state-rehydrated",
			SessionID: "session-workflow-resume-state-rehydrated",
			Args:      marshalArgs(t, map[string]any{}),
			Metadata:  map[string]string{"name": "resume-state-rehydrated"},
			Policy:    workflowpolicy.DefaultEffectivePolicy(),
			Resume: &workflowruntime.ResumeContext{
				CheckpointState: map[string]any{
					"step":       float64(1),
					"firstLabel": "step-one",
				},
			},
		}
		outcome := runSuccessful(t, req)
		projected := projectPrimaryJSON(t, req.SessionID, outcome.Value)
		assertProjectedFields(t, projected, map[string]any{
			"step":       float64(1),
			"firstLabel": "step-one",
		})
	})

	for _, errCase := range record.Errors {
		t.Run(errCase.Condition, func(t *testing.T) {
			source := resumeStateErrorSource(errCase.Condition)
			if source == "" {
				t.Fatalf("missing inline source for inventory error condition %q", errCase.Condition)
			}
			outcome := runInlineWorkflowFailure(t, "workflow-resume-state-"+errCase.Condition, source)
			if outcome.Failure.Code != workflowruntime.CodeScriptError {
				t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodeScriptError)
			}
			if !strings.Contains(outcome.Failure.Message, errCase.Message) {
				t.Fatalf("failure message = %q, want inventory message %q", outcome.Failure.Message, errCase.Message)
			}
		})
	}
}

func callBehaviorRecord(t *testing.T, path string) callbehavior.CallBehaviorRecord {
	t.Helper()
	for _, record := range callbehavior.ProjectInstalledCallBehavior().Records {
		if record.Path == path {
			return record
		}
	}
	t.Fatalf("call-behavior record %q not found", path)
	return callbehavior.CallBehaviorRecord{}
}

func assertCallBehaviorRecordDoesNotExposeHostContext(t *testing.T, record callbehavior.CallBehaviorRecord) {
	t.Helper()
	for _, forbidden := range callbehavior.ForbiddenRootGlobals {
		if record.Path == forbidden || strings.HasPrefix(record.Path, forbidden+".") {
			t.Fatalf("record path %q exposes forbidden host context %q", record.Path, forbidden)
		}
	}
}

func runInlineWorkflow(t *testing.T, name, source string) workflowruntime.Outcome {
	t.Helper()
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: name + ".workflow.js",
		SessionID: "session-" + name,
		Args:      marshalArgs(t, map[string]any{}),
		Metadata:  map[string]string{"name": name},
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}
	return runSuccessful(t, req)
}

func runInlineWorkflowFailure(t *testing.T, name, source string) workflowruntime.Outcome {
	t.Helper()
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: name + ".workflow.js",
		SessionID: "session-" + name,
		Args:      marshalArgs(t, map[string]any{}),
		Metadata:  map[string]string{"name": name},
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}
	return runExecutionFailure(t, req)
}

func checkpointErrorSource(condition string) string {
	switch condition {
	case "missing-or-non-object-argument":
		return `const spec = 1; workflow.checkpoint(spec); return { ok: true };`
	case "missing-label":
		return `const spec = { state: { step: 1 } }; workflow.checkpoint(spec); return { ok: true };`
	case "non-json-state":
		return `workflow.checkpoint({ label: "bad-state", state: () => {} }); return { ok: true };`
	default:
		return ""
	}
}

func resumeStateErrorSource(condition string) string {
	switch condition {
	case "arguments-provided":
		return `workflow.resumeState.apply(workflow, [{}]); return { ok: true };`
	default:
		return ""
	}
}

func findCheckpointRecord(t *testing.T, records []workflowruntime.RuntimeRecord, label string) *workflowruntime.CheckpointRecord {
	t.Helper()
	for _, record := range records {
		if record.Kind == workflowruntime.RecordKindCheckpoint && record.Checkpoint != nil && record.Checkpoint.Label == label {
			return record.Checkpoint
		}
	}
	t.Fatalf("checkpoint record with label %q not found in %#v", label, records)
	return nil
}

func hasCheckpointRecords(records []workflowruntime.RuntimeRecord) bool {
	for _, record := range records {
		if record.Kind == workflowruntime.RecordKindCheckpoint {
			return true
		}
	}
	return false
}
