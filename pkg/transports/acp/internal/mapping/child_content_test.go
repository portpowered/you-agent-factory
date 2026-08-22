package mapping

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type childOutcome string

const (
	childOutcomeToolCall childOutcome = "TOOL_CALL"
	childOutcomeUpdate   childOutcome = "TOOL_CALL_UPDATE"
	childOutcomeNoOutput childOutcome = "NO_OUTPUT"
)

// declaredChildOutcomes is intentionally keyed by every legal pair rather
// than Kind: the SESSION opening is a tool_call, its later phases are parent
// updates, RUN/TURN are declared no-output, and every meaningful Worker
// payload remains inside its owning child tool call.
var declaredChildOutcomes = map[dispatchPair]childOutcome{
	{workers.KindSession, workers.PhaseStarted}:   childOutcomeToolCall,
	{workers.KindSession, workers.PhaseUpdated}:   childOutcomeUpdate,
	{workers.KindSession, workers.PhaseCompleted}: childOutcomeUpdate,
	{workers.KindSession, workers.PhaseFailed}:    childOutcomeUpdate,
	{workers.KindSession, workers.PhaseCanceled}:  childOutcomeUpdate,

	{workers.KindRun, workers.PhaseStarted}:   childOutcomeNoOutput,
	{workers.KindRun, workers.PhaseCompleted}: childOutcomeNoOutput,
	{workers.KindRun, workers.PhaseFailed}:    childOutcomeNoOutput,
	{workers.KindRun, workers.PhaseCanceled}:  childOutcomeNoOutput,

	{workers.KindTurn, workers.PhaseStarted}:   childOutcomeNoOutput,
	{workers.KindTurn, workers.PhaseCompleted}: childOutcomeNoOutput,
	{workers.KindTurn, workers.PhaseFailed}:    childOutcomeNoOutput,
	{workers.KindTurn, workers.PhaseCanceled}:  childOutcomeNoOutput,

	{workers.KindMessage, workers.PhaseStarted}:   childOutcomeUpdate,
	{workers.KindMessage, workers.PhaseDelta}:     childOutcomeUpdate,
	{workers.KindMessage, workers.PhaseCompleted}: childOutcomeUpdate,

	{workers.KindReasoning, workers.PhaseStarted}:   childOutcomeUpdate,
	{workers.KindReasoning, workers.PhaseDelta}:     childOutcomeUpdate,
	{workers.KindReasoning, workers.PhaseCompleted}: childOutcomeUpdate,

	{workers.KindTool, workers.PhaseStarted}:   childOutcomeUpdate,
	{workers.KindTool, workers.PhaseDelta}:     childOutcomeUpdate,
	{workers.KindTool, workers.PhaseCompleted}: childOutcomeUpdate,
	{workers.KindTool, workers.PhaseFailed}:    childOutcomeUpdate,
	{workers.KindTool, workers.PhaseCanceled}:  childOutcomeUpdate,

	{workers.KindFileChange, workers.PhaseUpdated}: childOutcomeUpdate,
	{workers.KindPlan, workers.PhaseUpdated}:       childOutcomeUpdate,
	{workers.KindProgress, workers.PhaseUpdated}:   childOutcomeUpdate,
	{workers.KindUsage, workers.PhaseUpdated}:      childOutcomeUpdate,
	{workers.KindError, workers.PhaseUpdated}:      childOutcomeUpdate,
	{workers.KindError, workers.PhaseFailed}:       childOutcomeUpdate,
	{workers.KindStreamGap, workers.PhaseUpdated}:  childOutcomeUpdate,
}

func TestProjectWorkerChild_HandlesEveryLegalKindPhasePair(t *testing.T) {
	t.Parallel()
	if len(declaredChildOutcomes) != legalPairCount() {
		t.Fatalf("declared child outcomes = %d, want one per %d legal pair", len(declaredChildOutcomes), legalPairCount())
	}

	for pair, outcome := range declaredChildOutcomes {
		pair, outcome := pair, outcome
		t.Run(string(pair.Kind)+"/"+string(pair.Phase), func(t *testing.T) {
			t.Parallel()
			item := childItem(t, pair.Kind, pair.Phase, childFixturePayload(t, pair.Kind, pair.Phase))
			update, err := ProjectWorkerChild(item)
			if err != nil {
				t.Fatalf("ProjectWorkerChild() error = %v", err)
			}
			switch outcome {
			case childOutcomeNoOutput:
				requireNoUpdate(t, update)
			case childOutcomeToolCall:
				if update == nil || update.ToolCall == nil || update.ToolCall.ToolCallId != "child-record" {
					t.Fatalf("ProjectWorkerChild() = %#v, want opening ToolCall with stored identity", update)
				}
			case childOutcomeUpdate:
				if update == nil || update.ToolCallUpdate == nil {
					t.Fatalf("ProjectWorkerChild() = %#v, want ToolCallUpdate", update)
				}
				if update.ToolCallUpdate.ToolCallId != "child-tool-call" {
					t.Fatalf("ToolCallUpdate.ToolCallId = %q, want stored parent", update.ToolCallUpdate.ToolCallId)
				}
			default:
				t.Fatalf("unexpected declared child outcome %q", outcome)
			}
		})
	}
}

func TestProjectWorkerChild_MapsContentAndNativeToolPayloadsToParent(t *testing.T) {
	t.Parallel()
	percent := 50.0
	tool := workers.ToolPayload{ToolCallID: "native-tool", ToolName: "search", Status: "RUNNING"}
	cases := []struct {
		name       string
		kind       workers.Kind
		phase      workers.Phase
		payload    any
		wantText   string
		wantRawIn  any
		wantRawOut any
	}{
		{"message", workers.KindMessage, workers.PhaseDelta, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "answer"}, "answer", nil, nil},
		{"reasoning", workers.KindReasoning, workers.PhaseDelta, workers.ReasoningPayload{SummaryDelta: "thinking"}, "thinking", nil, nil},
		{"tool", workers.KindTool, workers.PhaseStarted, tool, "Tool search (RUNNING)", tool, nil},
		{"tool delta", workers.KindTool, workers.PhaseDelta, workers.ToolDeltaPayload{ToolCallID: "native-tool", OutputDelta: "tool output"}, "tool output", nil, workers.ToolDeltaPayload{ToolCallID: "native-tool", OutputDelta: "tool output"}},
		{"file change", workers.KindFileChange, workers.PhaseUpdated, workers.FileChangePayload{Path: "a.go", Operation: "modified", Summary: "updated mapper"}, "File modified: a.go\nupdated mapper", nil, nil},
		{"plan", workers.KindPlan, workers.PhaseUpdated, workers.PlanPayload{Summary: "plan", Steps: []workers.PlanStep{{Description: "implement", Status: "in_progress"}}}, "plan\nimplement (in_progress)", nil, nil},
		{"progress", workers.KindProgress, workers.PhaseUpdated, workers.ProgressPayload{Label: "Mapping", Message: "halfway", PercentComplete: &percent}, "Mapping: halfway (50%)", nil, nil},
		{"usage", workers.KindUsage, workers.PhaseUpdated, workers.UsagePayload{TotalTokens: 9, Model: "test-model"}, "Usage: 9 total tokens (test-model)", nil, nil},
		{"error", workers.KindError, workers.PhaseUpdated, workers.ErrorPayload{Code: "upstream", Message: "retry later", Retryable: true}, "Worker error [upstream]: retry later (retryable)", nil, nil},
		{"gap", workers.KindStreamGap, workers.PhaseUpdated, workers.StreamGapPayload{FromSequence: 3, ToSequence: 4, FirstAvailableSequence: 5}, "Records from sequence 3 to 4 are unavailable; history resumes at sequence 5.", nil, nil},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			item := childItem(t, tc.kind, tc.phase, mustMarshal(t, tc.payload))
			update, err := ProjectWorkerChild(item)
			if err != nil || update == nil || update.ToolCallUpdate == nil {
				t.Fatalf("ProjectWorkerChild() = (%#v, %v), want parent update", update, err)
			}
			got := update.ToolCallUpdate
			if got.ToolCallId != "child-tool-call" {
				t.Fatalf("ToolCallId = %q, want stored parent", got.ToolCallId)
			}
			if text := childUpdateText(t, got); text != tc.wantText {
				t.Fatalf("content = %q, want %q", text, tc.wantText)
			}
			if !reflect.DeepEqual(got.RawInput, tc.wantRawIn) || !reflect.DeepEqual(got.RawOutput, tc.wantRawOut) {
				t.Fatalf("raw fields = (%#v, %#v), want (%#v, %#v)", got.RawInput, got.RawOutput, tc.wantRawIn, tc.wantRawOut)
			}
		})
	}
}

func TestProjectWorkerChildRejectsMalformedContentWithoutPartialUpdate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		kind    workers.Kind
		phase   workers.Phase
		payload json.RawMessage
	}{
		{"invalid json", workers.KindMessage, workers.PhaseDelta, json.RawMessage(`not-json`)},
		{"message missing role", workers.KindMessage, workers.PhaseCompleted, json.RawMessage(`{"contentBlocks":[{"kind":"TEXT","text":"x"}]}`)},
		{"tool delta missing output", workers.KindTool, workers.PhaseDelta, mustMarshal(t, workers.ToolDeltaPayload{ToolCallID: "tool"})},
		{"file missing path", workers.KindFileChange, workers.PhaseUpdated, mustMarshal(t, workers.FileChangePayload{Operation: "modified"})},
		{"progress missing label", workers.KindProgress, workers.PhaseUpdated, mustMarshal(t, workers.ProgressPayload{})},
		{"error missing message", workers.KindError, workers.PhaseFailed, mustMarshal(t, workers.ErrorPayload{Code: "bad"})},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			update, err := ProjectWorkerChild(childItem(t, tc.kind, tc.phase, tc.payload))
			if update != nil || !errors.Is(err, ErrMalformedRecord) {
				t.Fatalf("ProjectWorkerChild() = (%#v, %v), want no update and ErrMalformedRecord", update, err)
			}
		})
	}

	update, err := ProjectWorkerChild(chatsessions.SequencedItem{
		ItemID:                   "missing-parent",
		WorkerSessionAssociation: childAssociation(),
		Kind:                     workers.KindMessage,
		Phase:                    workers.PhaseDelta,
		Payload:                  mustMarshal(t, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "x"}),
	})
	if update != nil || !errors.Is(err, ErrMissingChildParent) {
		t.Fatalf("ProjectWorkerChild(missing parent) = (%#v, %v), want ErrMissingChildParent", update, err)
	}
}

func TestProjectWorkerChildInterleavingKeepsSiblingParentTargetsIndependent(t *testing.T) {
	t.Parallel()
	items := []chatsessions.SequencedItem{
		childOpening(t, "child-a"),
		childOpening(t, "child-b"),
		childItemWithParent(t, "child-a", workers.KindMessage, workers.PhaseDelta, mustMarshal(t, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "a-one"})),
		childItemWithParent(t, "child-b", workers.KindProgress, workers.PhaseUpdated, mustMarshal(t, workers.ProgressPayload{Label: "b-progress"})),
		childItemWithParent(t, "child-a", workers.KindError, workers.PhaseUpdated, json.RawMessage(`{"code":"broken"}`)),
		childItemWithParent(t, "child-b", workers.KindMessage, workers.PhaseDelta, mustMarshal(t, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "b-two"})),
	}
	wantParents := []string{"child-a", "child-b", "child-b"}
	var gotParents []string
	for _, item := range items {
		update, err := ProjectWorkerChild(item)
		if item.ItemID == "child-a-record" && item.Kind == workers.KindError {
			if update != nil || !errors.Is(err, ErrMalformedRecord) {
				t.Fatalf("malformed child A = (%#v, %v), want typed error only", update, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ProjectWorkerChild(%s) error = %v", item.ItemID, err)
		}
		if update != nil && update.ToolCallUpdate != nil {
			gotParents = append(gotParents, string(update.ToolCallUpdate.ToolCallId))
		}
	}
	if !reflect.DeepEqual(gotParents, wantParents) {
		t.Fatalf("parent update targets = %#v, want %#v", gotParents, wantParents)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestBoundChildProjectionReservesFailureAndTerminalEvidenceAfterRecordPressure(t *testing.T) {
	t.Parallel()
	limits := ChildProjectionLimits{MaxRecords: 4, MaxSerializedBytes: 4096}
	budget := NewChildProjectionBudget(limits)
	ordinary := childItemWithParent(t, "child-tool-call", workers.KindProgress, workers.PhaseUpdated,
		mustMarshal(t, workers.ProgressPayload{Label: "progress"}))

	for index := 0; index < 2; index++ {
		var update *acpsdk.SessionUpdate
		var err error
		budget, update, err = BoundChildProjection(budget, ordinary, childTextUpdate("child-tool-call", "progress"))
		if err != nil || childUpdateText(t, update.ToolCallUpdate) != "progress" {
			t.Fatalf("ordinary update %d = (%#v, %v), want retained content", index, update, err)
		}
	}

	var update *acpsdk.SessionUpdate
	var err error
	budget, update, err = BoundChildProjection(budget, ordinary, childTextUpdate("child-tool-call", "noisy progress"))
	if err != nil || update == nil || update.ToolCallUpdate == nil || !strings.Contains(childUpdateText(t, update.ToolCallUpdate), "content was elided") {
		t.Fatalf("record-pressure update = (%#v, %v), want explicit ordinary elision", update, err)
	}

	budget, update, err = BoundChildProjection(budget, ordinary, childTextUpdate("child-tool-call", "later progress"))
	if err != nil || update != nil {
		t.Fatalf("post-elision ordinary update = (%#v, %v), want declared no output", update, err)
	}

	failure := childItemWithParent(t, "child-tool-call", workers.KindError, workers.PhaseFailed,
		mustMarshal(t, workers.ErrorPayload{Code: "upstream", Message: "failed"}))
	budget, update, err = BoundChildProjection(budget, failure, childTextUpdate("child-tool-call", "Worker error [upstream]: failed"))
	if err != nil || update == nil || update.ToolCallUpdate == nil || !strings.Contains(childUpdateText(t, update.ToolCallUpdate), "Worker error content was elided") {
		t.Fatalf("failure after record pressure = (%#v, %v), want explicit retained failure evidence", update, err)
	}

	terminalStatus := acpsdk.ToolCallStatusFailed
	terminal := &acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
		ToolCallId: "child-tool-call", Status: &terminalStatus,
	}}
	terminalItem := childItemWithParent(t, "child-tool-call", workers.KindSession, workers.PhaseFailed,
		mustMarshal(t, workers.SessionPayload{Status: "FAILED"}))
	_, update, err = BoundChildProjection(budget, terminalItem, terminal)
	if err != nil || update != terminal || update.ToolCallUpdate.Status == nil || *update.ToolCallUpdate.Status != acpsdk.ToolCallStatusFailed {
		t.Fatalf("terminal after record pressure = (%#v, %v), want unchanged failed status", update, err)
	}
}

func TestBoundChildProjectionReplacesOversizedValueWithByteBoundedElision(t *testing.T) {
	t.Parallel()
	ordinaryNotice, ordinaryBytes := childProjectionElision("child-tool-call", childProjectionElisionOrdinary)
	_, failureBytes := childProjectionElision("child-tool-call", childProjectionElisionFailure)
	limits := ChildProjectionLimits{MaxRecords: 4, MaxSerializedBytes: ordinaryBytes + failureBytes + 1}
	item := childItemWithParent(t, "child-tool-call", workers.KindMessage, workers.PhaseDelta,
		mustMarshal(t, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: strings.Repeat("x", 2048)}))

	_, update, err := BoundChildProjection(
		NewChildProjectionBudget(limits),
		item,
		childTextUpdate("child-tool-call", strings.Repeat("x", 2048)),
	)
	if err != nil || update == nil || update.ToolCallUpdate == nil {
		t.Fatalf("BoundChildProjection() = (%#v, %v), want explicit byte elision", update, err)
	}
	if text := childUpdateText(t, update.ToolCallUpdate); !strings.Contains(text, "content was elided") || strings.Contains(text, strings.Repeat("x", 32)) {
		t.Fatalf("byte-bounded content = %q, want explicit non-source elision", text)
	}
	gotBytes, err := serializedUpdateBytes(update)
	if err != nil {
		t.Fatalf("serializedUpdateBytes() error = %v", err)
	}
	if gotBytes > limits.MaxSerializedBytes {
		t.Fatalf("elision bytes = %d, want <= %d", gotBytes, limits.MaxSerializedBytes)
	}
	if update != ordinaryNotice && childUpdateText(t, update.ToolCallUpdate) != childUpdateText(t, ordinaryNotice.ToolCallUpdate) {
		t.Fatalf("byte elision = %#v, want deterministic ordinary notice", update)
	}
}

func TestBoundChildProjectionKeepsMultipleStreamGapRanges(t *testing.T) {
	t.Parallel()
	budget := NewChildProjectionBudget(ChildProjectionLimits{MaxRecords: 8, MaxSerializedBytes: 4096})
	gaps := []workers.StreamGapPayload{
		{FromSequence: 3, ToSequence: 4, FirstAvailableSequence: 5, Reason: "first eviction"},
		{FromSequence: 8, ToSequence: 9, FirstAvailableSequence: 10, Reason: "second eviction"},
	}
	for _, gap := range gaps {
		item := childItemWithParent(t, "child-tool-call", workers.KindStreamGap, workers.PhaseUpdated, mustMarshal(t, gap))
		mapped, err := ProjectWorkerChild(item)
		if err != nil {
			t.Fatalf("ProjectWorkerChild() error = %v", err)
		}
		var bounded *acpsdk.SessionUpdate
		budget, bounded, err = BoundChildProjection(budget, item, mapped)
		if err != nil || bounded == nil || bounded.ToolCallUpdate == nil {
			t.Fatalf("BoundChildProjection() = (%#v, %v), want stream-gap update", bounded, err)
		}
		want := gapNoticeText(gap)
		if got := childUpdateText(t, bounded.ToolCallUpdate); got != want {
			t.Fatalf("stream-gap text = %q, want %q", got, want)
		}
	}
}

func childItem(t *testing.T, kind workers.Kind, phase workers.Phase, payload json.RawMessage) chatsessions.SequencedItem {
	t.Helper()
	if kind == workers.KindSession && phase == workers.PhaseStarted {
		return childOpening(t, "child-record")
	}
	return childItemWithParent(t, "child-tool-call", kind, phase, payload)
}

func childItemWithParent(t *testing.T, parent string, kind workers.Kind, phase workers.Phase, payload json.RawMessage) chatsessions.SequencedItem {
	t.Helper()
	return chatsessions.SequencedItem{
		ItemID:                   parent + "-record",
		ParentItemID:             parent,
		WorkerSessionAssociation: childAssociation(),
		Kind:                     kind,
		Phase:                    phase,
		Payload:                  payload,
	}
}

func childOpening(t *testing.T, itemID string) chatsessions.SequencedItem {
	t.Helper()
	return chatsessions.SequencedItem{
		ItemID:                   itemID,
		WorkerSessionAssociation: childAssociation(),
		Kind:                     workers.KindSession,
		Phase:                    workers.PhaseStarted,
		Payload:                  mustMarshal(t, workers.SessionPayload{Status: "STARTING"}),
	}
}

func childAssociation() *chatsessions.WorkerSessionAssociation {
	return &chatsessions.WorkerSessionAssociation{DispatchID: "dispatch-1", WorkerSessionID: "worker-1"}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func childFixturePayload(t *testing.T, kind workers.Kind, phase workers.Phase) json.RawMessage {
	t.Helper()
	switch kind {
	case workers.KindSession:
		status := "RUNNING"
		switch phase {
		case workers.PhaseStarted:
			status = "STARTING"
		case workers.PhaseCompleted:
			status = "COMPLETED"
		case workers.PhaseFailed:
			status = "FAILED"
		case workers.PhaseCanceled:
			status = "CANCELED"
		}
		return mustMarshal(t, workers.SessionPayload{Status: status})
	case workers.KindRun:
		return mustMarshal(t, workers.RunPayload{Status: "RUNNING"})
	case workers.KindTurn:
		return mustMarshal(t, workers.TurnPayload{TurnIndex: 1, Status: "RUNNING"})
	case workers.KindMessage:
		if phase == workers.PhaseDelta {
			return mustMarshal(t, workers.MessageDeltaPayload{ContentBlockKind: workers.ContentBlockText, TextDelta: "message"})
		}
		return mustMarshal(t, workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "message"}}})
	case workers.KindReasoning:
		if phase == workers.PhaseDelta {
			return mustMarshal(t, workers.ReasoningPayload{SummaryDelta: "reasoning"})
		}
		return mustMarshal(t, workers.ReasoningPayload{Summary: "reasoning"})
	case workers.KindTool:
		if phase == workers.PhaseDelta {
			return mustMarshal(t, workers.ToolDeltaPayload{ToolCallID: "native-tool", OutputDelta: "output"})
		}
		return mustMarshal(t, workers.ToolPayload{ToolCallID: "native-tool", ToolName: "search", Status: "RUNNING"})
	case workers.KindFileChange:
		return mustMarshal(t, workers.FileChangePayload{Path: "a.go", Operation: "modified"})
	case workers.KindPlan:
		return mustMarshal(t, workers.PlanPayload{Summary: "plan"})
	case workers.KindProgress:
		return mustMarshal(t, workers.ProgressPayload{Label: "progress"})
	case workers.KindUsage:
		return mustMarshal(t, workers.UsagePayload{TotalTokens: 1})
	case workers.KindError:
		return mustMarshal(t, workers.ErrorPayload{Code: "bad", Message: "failure"})
	case workers.KindStreamGap:
		return mustMarshal(t, workers.StreamGapPayload{FromSequence: 1, ToSequence: 2, FirstAvailableSequence: 3})
	default:
		t.Fatalf("no child fixture for %s/%s", kind, phase)
		return nil
	}
}

func legalPairCount() int {
	count := 0
	for _, phases := range legalPhasesByKind {
		count += len(phases)
	}
	return count
}

func childUpdateText(t *testing.T, update *acpsdk.SessionToolCallUpdate) string {
	t.Helper()
	if len(update.Content) != 1 || update.Content[0].Content == nil || update.Content[0].Content.Content.Text == nil {
		t.Fatalf("tool call content = %#v, want one text content block", update.Content)
	}
	return update.Content[0].Content.Content.Text.Text
}
