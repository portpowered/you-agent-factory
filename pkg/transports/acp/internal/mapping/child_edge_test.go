package mapping

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestProjectWorkerChildRejectsMalformedContentPayloads keeps every
// child-specific payload validation failure typed and parent-isolated. These
// values are published Worker vocabulary, so the assertions exercise the
// actual ACP boundary rather than inspecting implementation details.
func TestProjectWorkerChildRejectsMalformedContentPayloads(t *testing.T) {
	parent := "worker-tool-call"
	tests := []struct {
		name    string
		kind    workers.Kind
		phase   workers.Phase
		payload any
	}{
		{"run invalid JSON", workers.KindRun, workers.PhaseStarted, json.RawMessage(`not-json`)},
		{"turn invalid JSON", workers.KindTurn, workers.PhaseStarted, json.RawMessage(`not-json`)},
		{"message delta invalid block index", workers.KindMessage, workers.PhaseDelta, workers.MessageDeltaPayload{ContentBlockIndex: -1, ContentBlockKind: workers.ContentBlockText, TextDelta: "text"}},
		{"message snapshot missing role", workers.KindMessage, workers.PhaseCompleted, workers.MessagePayload{ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "text"}}}},
		{"reasoning invalid JSON", workers.KindReasoning, workers.PhaseDelta, json.RawMessage(`not-json`)},
		{"tool delta missing output", workers.KindTool, workers.PhaseDelta, workers.ToolDeltaPayload{ToolCallID: "native"}},
		{"tool snapshot missing name", workers.KindTool, workers.PhaseStarted, workers.ToolPayload{ToolCallID: "native"}},
		{"file change missing operation", workers.KindFileChange, workers.PhaseUpdated, workers.FileChangePayload{Path: "a.go"}},
		{"plan blank step", workers.KindPlan, workers.PhaseUpdated, workers.PlanPayload{Steps: []workers.PlanStep{{Description: " "}}}},
		{"progress missing label", workers.KindProgress, workers.PhaseUpdated, workers.ProgressPayload{}},
		{"error missing message", workers.KindError, workers.PhaseUpdated, workers.ErrorPayload{Code: "failed"}},
		{"stream gap invalid range", workers.KindStreamGap, workers.PhaseUpdated, workers.StreamGapPayload{FromSequence: 2, ToSequence: 1, FirstAvailableSequence: 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := json.RawMessage(nil)
			if raw, ok := tt.payload.(json.RawMessage); ok {
				payload = raw
			} else {
				payload = mustMarshal(t, tt.payload)
			}
			update, err := ProjectWorkerChild(childItemWithParent(t, parent, tt.kind, tt.phase, payload))
			requireMalformed(t, update, err)
		})
	}
}

func TestProjectWorkerChildOptionalContentRemainsParentScoped(t *testing.T) {
	parent := "worker-tool-call"
	percent := 42.5
	tests := []struct {
		name  string
		kind  workers.Kind
		phase workers.Phase
		body  any
		want  string
	}{
		{"reasoning snapshot", workers.KindReasoning, workers.PhaseCompleted, workers.ReasoningPayload{Summary: "thought"}, "thought"},
		{"tool snapshot status", workers.KindTool, workers.PhaseCompleted, workers.ToolPayload{ToolCallID: "native", ToolName: "search", Status: "DONE"}, "Tool search (DONE)"},
		{"file change summary", workers.KindFileChange, workers.PhaseUpdated, workers.FileChangePayload{Path: "a.go", Operation: "modified", Summary: "formatted"}, "File modified: a.go\nformatted"},
		{"plan status", workers.KindPlan, workers.PhaseUpdated, workers.PlanPayload{Summary: "plan", Steps: []workers.PlanStep{{Description: "ship", Status: "DONE"}}}, "plan\nship (DONE)"},
		{"progress percent", workers.KindProgress, workers.PhaseUpdated, workers.ProgressPayload{Label: "compile", Message: "halfway", PercentComplete: &percent}, "compile: halfway (42.5%)"},
		{"usage model", workers.KindUsage, workers.PhaseUpdated, workers.UsagePayload{TotalTokens: 7, Model: "model"}, "Usage: 7 total tokens (model)"},
		{"retryable error", workers.KindError, workers.PhaseUpdated, workers.ErrorPayload{Code: "retry", Message: "again", Retryable: true}, "Worker error [retry]: again (retryable)"},
		{"item gap reason", workers.KindStreamGap, workers.PhaseUpdated, workers.StreamGapPayload{AffectedItemID: "lost-item", Reason: "retention"}, "Some records for this item are unavailable: retention"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update, err := ProjectWorkerChild(childItemWithParent(t, parent, tt.kind, tt.phase, mustMarshal(t, tt.body)))
			if err != nil || update == nil || update.ToolCallUpdate == nil {
				t.Fatalf("ProjectWorkerChild() = (%#v, %v), want parent-scoped content", update, err)
			}
			if update.ToolCallUpdate.ToolCallId != acpsdk.ToolCallId(parent) {
				t.Fatalf("parent tool call = %q, want %q", update.ToolCallUpdate.ToolCallId, parent)
			}
			if got := childUpdateText(t, update.ToolCallUpdate); got != tt.want {
				t.Fatalf("content = %q, want %q", got, tt.want)
			}
		})
	}

	noOutput, err := ProjectWorkerChild(childItemWithParent(t, parent, workers.KindPlan, workers.PhaseUpdated, mustMarshal(t, workers.PlanPayload{})))
	if err != nil || noOutput != nil {
		t.Fatalf("empty plan = (%#v, %v), want declared no output", noOutput, err)
	}
}

func TestBoundChildProjectionProtectsGapAndFailureEvidence(t *testing.T) {
	parent := "worker-tool-call"
	ordinary := childTextUpdate(parent, "ordinary")
	gapItem := childItemWithParent(t, parent, workers.KindStreamGap, workers.PhaseUpdated, mustMarshal(t, workers.StreamGapPayload{
		FromSequence: 4, ToSequence: 6, FirstAvailableSequence: 7, Reason: "retention",
	}))
	failureItem := childItemWithParent(t, parent, workers.KindError, workers.PhaseUpdated, mustMarshal(t, workers.ErrorPayload{Code: "failed", Message: "message"}))

	budget := NewChildProjectionBudget(ChildProjectionLimits{MaxRecords: 2, MaxSerializedBytes: 4096})
	var err error
	budget, elision, err := BoundChildProjection(budget, childItemWithParent(t, parent, workers.KindMessage, workers.PhaseDelta, mustMarshal(t, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "ordinary"})), ordinary)
	if err != nil || elision == nil || !strings.Contains(childUpdateText(t, elision.ToolCallUpdate), "elided") {
		t.Fatalf("ordinary pressure = (%#v, %v), want explicit ordinary elision", elision, err)
	}
	budget, gap, err := BoundChildProjection(budget, gapItem, childTextUpdate(parent, "gap"))
	if err != nil || gap == nil || !strings.Contains(childUpdateText(t, gap.ToolCallUpdate), "Additional stream-gap detail was elided") {
		t.Fatalf("gap pressure = (%#v, %v), want bounded gap evidence", gap, err)
	}
	if _, repeated, err := BoundChildProjection(budget, failureItem, childTextUpdate(parent, "failure")); err != nil || repeated != nil {
		t.Fatalf("exhausted protected budget = (%#v, %v), want no duplicate evidence", repeated, err)
	}

	budget = NewChildProjectionBudget(ChildProjectionLimits{MaxRecords: 3, MaxSerializedBytes: 4096})
	budget, retained, err := BoundChildProjection(budget, failureItem, childTextUpdate(parent, "failure"))
	if err != nil || retained == nil || childUpdateText(t, retained.ToolCallUpdate) != "failure" {
		t.Fatalf("available failure evidence = (%#v, %v), want original evidence", retained, err)
	}
	budget = NewChildProjectionBudget(ChildProjectionLimits{MaxRecords: 2, MaxSerializedBytes: 4096}).record(3500)
	_, failureElision, err := BoundChildProjection(budget, failureItem, childTextUpdate(parent, "failure"))
	if err != nil || failureElision == nil || !strings.Contains(childUpdateText(t, failureElision.ToolCallUpdate), "Worker error content was elided") {
		t.Fatalf("failure pressure = (%#v, %v), want explicit failure elision", failureElision, err)
	}
}

func TestBoundChildProjectionRejectsInvalidLimitsAndLineage(t *testing.T) {
	parent := "worker-tool-call"
	item := childItemWithParent(t, parent, workers.KindMessage, workers.PhaseDelta, mustMarshal(t, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "text"}))
	update := childTextUpdate(parent, "text")

	for _, limits := range []ChildProjectionLimits{{MaxRecords: 1, MaxSerializedBytes: 1000}, {MaxRecords: 2, MaxSerializedBytes: 1}} {
		_, bounded, err := BoundChildProjection(NewChildProjectionBudget(limits), item, update)
		if bounded != nil || !errors.Is(err, ErrMalformedRecord) {
			t.Fatalf("limits %#v = (%#v, %v), want malformed", limits, bounded, err)
		}
	}

	missingParent := childTextUpdate("", "text")
	if _, bounded, err := BoundChildProjection(NewChildProjectionBudget(DefaultChildProjectionLimits()), item, missingParent); bounded != nil || !errors.Is(err, ErrMissingChildParent) {
		t.Fatalf("missing parent = (%#v, %v), want ErrMissingChildParent", bounded, err)
	}
	mismatched := childTextUpdate("other", "text")
	if _, bounded, err := BoundChildProjection(NewChildProjectionBudget(DefaultChildProjectionLimits()), item, mismatched); bounded != nil || !errors.Is(err, ErrMalformedRecord) {
		t.Fatalf("mismatched parent = (%#v, %v), want malformed", bounded, err)
	}

	limits := DefaultChildProjectionLimits()
	if limits.MaxRecords != DefaultChildProjectionMaxRecords || limits.MaxSerializedBytes != DefaultChildProjectionMaxSerializedBytes {
		t.Fatalf("default limits = %#v, want named defaults", limits)
	}
	if budget, bounded, err := BoundChildProjection(NewChildProjectionBudget(limits), item, &acpsdk.SessionUpdate{}); err != nil || bounded == nil || budget.emittedRecords != 0 {
		t.Fatalf("unbounded update = (%#v, %#v, %v), want unchanged update and budget", budget, bounded, err)
	}
}

func TestChildProjectionValidationRejectsEveryMalformedBoundaryShape(t *testing.T) {
	association := ChildAssociation{DispatchID: "dispatch", WorkerSessionID: "worker"}
	wrongOpening := workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseStarted, ItemID: "item", Payload: mustMarshal(t, workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "text"}}})}
	if update, err := ProjectChildOpening(wrongOpening, association); update != nil || !errors.Is(err, ErrMalformedRecord) {
		t.Fatalf("wrong opening = (%#v, %v), want malformed", update, err)
	}

	invalidPair := childItemWithParent(t, "parent", workers.KindMessage, workers.PhaseUpdated, mustMarshal(t, workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "text"}}}))
	if update, err := ProjectWorkerChild(invalidPair); update != nil || !errors.Is(err, ErrMalformedRecord) {
		t.Fatalf("illegal pair = (%#v, %v), want malformed", update, err)
	}
	if update, err := ProjectChildLifecycle(childSessionDraft(workers.PhaseDelta, "item", "parent", "RUNNING")); update != nil || !errors.Is(err, ErrMalformedRecord) {
		t.Fatalf("unsupported lifecycle phase = (%#v, %v), want malformed", update, err)
	}
	for _, draft := range []workers.Draft{
		{Kind: workers.KindSession, Phase: workers.PhaseUpdated, ParentItemID: "parent", Payload: json.RawMessage(`[]`)},
		childSessionDraft(workers.PhaseUpdated, "item", "parent", ""),
		childSessionDraft(workers.PhaseUpdated, "item", "parent", "COMPLETED"),
		childSessionDraft(workers.PhaseFailed, "item", "parent", "COMPLETED"),
		childSessionDraft(workers.PhaseCanceled, "item", "parent", "COMPLETED"),
	} {
		if update, err := ProjectChildLifecycle(draft); update != nil || !errors.Is(err, ErrMalformedRecord) {
			t.Fatalf("lifecycle %#v = (%#v, %v), want malformed", draft, update, err)
		}
	}

	for _, kind := range []workers.ContentBlockKind{
		workers.ContentBlockText, workers.ContentBlockReasoningSummary, workers.ContentBlockToolRequest,
		workers.ContentBlockImageRef, workers.ContentBlockResourceRef, workers.ContentBlockStructuredOutput,
	} {
		if !knownContentBlockKind(kind) {
			t.Fatalf("knownContentBlockKind(%q) = false, want true", kind)
		}
	}
	if knownContentBlockKind("UNKNOWN") {
		t.Fatal("knownContentBlockKind(UNKNOWN) = true, want false")
	}

	for _, draft := range []workers.Draft{
		{Kind: "UNKNOWN", Phase: workers.PhaseUpdated, ParentItemID: "parent", Payload: json.RawMessage(`{}`)},
		{Kind: workers.KindMessage, Phase: workers.PhaseCompleted, ParentItemID: "parent", Payload: json.RawMessage(`[]`)},
		{Kind: workers.KindTool, Phase: workers.PhaseDelta, ParentItemID: "parent", Payload: json.RawMessage(`[]`)},
		{Kind: workers.KindTool, Phase: workers.PhaseStarted, ParentItemID: "parent", Payload: json.RawMessage(`[]`)},
		{Kind: workers.KindFileChange, Phase: workers.PhaseUpdated, ParentItemID: "parent", Payload: json.RawMessage(`[]`)},
		{Kind: workers.KindPlan, Phase: workers.PhaseUpdated, ParentItemID: "parent", Payload: json.RawMessage(`[]`)},
		{Kind: workers.KindProgress, Phase: workers.PhaseUpdated, ParentItemID: "parent", Payload: json.RawMessage(`[]`)},
		{Kind: workers.KindUsage, Phase: workers.PhaseUpdated, ParentItemID: "parent", Payload: json.RawMessage(`[]`)},
		{Kind: workers.KindError, Phase: workers.PhaseUpdated, ParentItemID: "parent", Payload: json.RawMessage(`[]`)},
		{Kind: workers.KindStreamGap, Phase: workers.PhaseUpdated, ParentItemID: "parent", Payload: json.RawMessage(`[]`)},
	} {
		if update, err := projectChildRecord(draft); update != nil || !errors.Is(err, ErrMalformedRecord) {
			t.Fatalf("malformed child record %#v = (%#v, %v), want malformed", draft, update, err)
		}
	}
	if update, err := projectChildMessage(workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseCompleted, ParentItemID: "parent", Payload: mustMarshal(t, workers.MessagePayload{Role: "user", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "not customer output"}}})}); update != nil || err != nil {
		t.Fatalf("non-assistant message = (%#v, %v), want declared no output", update, err)
	}
	if update, err := projectChildMessage(workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseCompleted, ParentItemID: "parent", Payload: mustMarshal(t, workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: "UNKNOWN", Text: "text"}}})}); update != nil || !errors.Is(err, ErrMalformedRecord) {
		t.Fatalf("unknown message block = (%#v, %v), want malformed", update, err)
	}
	if got := gapNoticeText(workers.StreamGapPayload{AffectedItemID: "lost"}); got != "Some records for this item are unavailable." {
		t.Fatalf("item gap without reason = %q, want bounded default notice", got)
	}

	invalidGap := chatsessions.SequencedItem{Payload: json.RawMessage(`[]`)}
	if update, _, err := childStreamGapElision(invalidGap, "parent"); update != nil || !errors.Is(err, ErrMalformedRecord) {
		t.Fatalf("malformed gap elision = (%#v, %v), want malformed", update, err)
	}
	invalidGap.Payload = mustMarshal(t, workers.StreamGapPayload{})
	if update, _, err := childStreamGapElision(invalidGap, "parent"); update != nil || !errors.Is(err, ErrMalformedRecord) {
		t.Fatalf("invalid gap elision = (%#v, %v), want malformed", update, err)
	}

	unsafe := &acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{ToolCallId: "parent", RawInput: math.Inf(1)}}
	if _, err := serializedUpdateBytes(unsafe); !errors.Is(err, ErrMalformedRecord) {
		t.Fatalf("serializedUpdateBytes(unsafe) error = %v, want malformed", err)
	}
	item := childItemWithParent(t, "parent", workers.KindMessage, workers.PhaseDelta, mustMarshal(t, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "text"}))
	if _, update, err := BoundChildProjection(NewChildProjectionBudget(DefaultChildProjectionLimits()), item, unsafe); update != nil || !errors.Is(err, ErrMalformedRecord) {
		t.Fatalf("unsafe bounded projection = (%#v, %v), want malformed", update, err)
	}
	badGapItem := chatsessions.SequencedItem{Kind: workers.KindStreamGap, Payload: json.RawMessage(`[]`)}
	budgetAtProtectedLimit := NewChildProjectionBudget(ChildProjectionLimits{MaxRecords: 2, MaxSerializedBytes: 4096}).record(1)
	if _, update, err := boundProtectedChildProjection(budgetAtProtectedLimit, badGapItem, childTextUpdate("parent", "gap"), 200, 100, childTextUpdate("parent", "failure"), 200); update != nil || !errors.Is(err, ErrMalformedRecord) {
		t.Fatalf("bounded malformed gap = (%#v, %v), want malformed", update, err)
	}
}
