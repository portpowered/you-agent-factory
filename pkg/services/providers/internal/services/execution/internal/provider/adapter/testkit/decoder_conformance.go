package testkit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/adapter"
	responseevents "github.com/portpowered/infinite-you/pkg/services/workers"
)

// DecoderConformanceFixture supplies provider-native records for the shared
// semantic decoder contract. It is intentionally independent of command and
// final-result parsing so provider migrations can adopt it incrementally.
type DecoderConformanceFixture struct {
	NewDecoder          func(adapter.DecoderContext) adapter.Decoder
	Lifecycle           []adapter.Observation
	UnsafeAndRecovering []adapter.Observation
	UnterminatedFinal   []adapter.Observation
	Expected            DecoderConformanceExpected
	ForbiddenDiagnostic []string
}

// DecoderConformanceExpected names the stable neutral correlations asserted
// across provider-specific native fixtures.
type DecoderConformanceExpected struct {
	ProviderRef   string
	MessageItemID string
	ToolItemID    string
	ToolCallID    string
	FinalContent  string
}

// RunDecoderConformance verifies the neutral invariants shared by structured
// provider decoders: valid drafts, stable tool identity, safe diagnostics,
// recovery after additive input, and final unterminated record flushing.
func RunDecoderConformance(t *testing.T, fixture DecoderConformanceFixture) {
	t.Helper()
	if fixture.NewDecoder == nil || len(fixture.Lifecycle) == 0 || len(fixture.UnsafeAndRecovering) == 0 || len(fixture.UnterminatedFinal) == 0 {
		t.Fatal("decoder conformance requires a decoder and all observation fixtures")
	}

	t.Run("valid lifecycle and stable item identity", func(t *testing.T) {
		drafts, diagnostics := decodeProviderDecoder(t, fixture, fixture.Lifecycle)
		assertSafeDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
		assertValidDecoderDrafts(t, drafts)
		assertStableDecoderTool(t, drafts, fixture.Expected)
		assertDecoderTerminalOutcome(t, drafts, fixture.Expected)
	})
	t.Run("unknown additive input is safe and recoverable", func(t *testing.T) {
		drafts, diagnostics := decodeProviderDecoder(t, fixture, fixture.UnsafeAndRecovering)
		assertSafeDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
		if len(diagnostics) == 0 {
			t.Fatal("diagnostics = nil, want bounded diagnostic for unknown input")
		}
		assertValidDecoderDrafts(t, drafts)
		assertDecoderTerminalOutcome(t, drafts, fixture.Expected)
	})
	t.Run("flush processes final unterminated record", func(t *testing.T) {
		drafts, diagnostics := decodeProviderDecoder(t, fixture, fixture.UnterminatedFinal)
		assertSafeDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
		assertValidDecoderDrafts(t, drafts)
		assertDecoderTerminalOutcome(t, drafts, fixture.Expected)
	})
}

func decodeProviderDecoder(t *testing.T, fixture DecoderConformanceFixture, observations []adapter.Observation) ([]responseevents.Draft, []adapter.Diagnostic) {
	t.Helper()
	decoder := fixture.NewDecoder(adapter.DecoderContext{RunID: "run-decoder-conformance", DispatchID: "dispatch-decoder-conformance"})
	var drafts []responseevents.Draft
	var diagnostics []adapter.Diagnostic
	for _, observation := range observations {
		result, err := decoder.Observe(context.Background(), observation)
		if err != nil {
			t.Fatalf("Observe() error = %v", err)
		}
		drafts = append(drafts, result.Drafts...)
		diagnostics = append(diagnostics, result.Diagnostics...)
	}
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: adapter.FlushReasonCompleted})
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	return append(drafts, flushed.Drafts...), append(diagnostics, flushed.Diagnostics...)
}

func assertValidDecoderDrafts(t *testing.T, drafts []responseevents.Draft) {
	t.Helper()
	for index, draft := range drafts {
		if err := responseevents.ValidateDraft(draft); err != nil {
			t.Fatalf("draft[%d] %s/%s is invalid: %v", index, draft.Kind, draft.Phase, err)
		}
		if draft.Kind == responseevents.KindReasoning {
			t.Fatalf("draft[%d] inferred unsupported reasoning: %#v", index, draft)
		}
	}
}

func assertStableDecoderTool(t *testing.T, drafts []responseevents.Draft, expected DecoderConformanceExpected) {
	t.Helper()
	started := findDraft(drafts, responseevents.KindTool, responseevents.PhaseStarted)
	if started == nil || started.ItemID != expected.ToolItemID || started.ProviderSessionRef != expected.ProviderRef {
		t.Fatalf("started tool = %#v, want item %q and provider ref %q", started, expected.ToolItemID, expected.ProviderRef)
	}
	var startedPayload responseevents.ToolPayload
	mustDecodePayload(t, *started, &startedPayload)
	if startedPayload.ToolCallID != expected.ToolCallID {
		t.Fatalf("started tool call = %q, want %q", startedPayload.ToolCallID, expected.ToolCallID)
	}
	for _, phase := range []responseevents.Phase{responseevents.PhaseCompleted, responseevents.PhaseFailed, responseevents.PhaseCanceled} {
		terminal := findDraft(drafts, responseevents.KindTool, phase)
		if terminal == nil || terminal.ItemID != started.ItemID {
			continue
		}
		var terminalPayload responseevents.ToolPayload
		mustDecodePayload(t, *terminal, &terminalPayload)
		if terminalPayload.ToolCallID == startedPayload.ToolCallID && terminalPayload.ToolName == startedPayload.ToolName {
			return
		}
	}
	t.Fatalf("drafts do not contain a terminal tool event correlated to %#v", *started)
}

func assertDecoderTerminalOutcome(t *testing.T, drafts []responseevents.Draft, expected DecoderConformanceExpected) {
	t.Helper()
	message := findDraft(drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
	if message == nil {
		if findDraft(drafts, responseevents.KindError, responseevents.PhaseFailed) != nil || findDraft(drafts, responseevents.KindRun, responseevents.PhaseCanceled) != nil {
			return
		}
		t.Fatalf("drafts have neither authoritative message completion nor explicit failure: %#v", drafts)
	}
	if message.ItemID != expected.MessageItemID || message.ProviderSessionRef != expected.ProviderRef {
		t.Fatalf("completed message = %#v, want item %q and provider ref %q", *message, expected.MessageItemID, expected.ProviderRef)
	}
	var payload responseevents.MessagePayload
	mustDecodePayload(t, *message, &payload)
	if len(payload.ContentBlocks) != 1 || strings.TrimSpace(string(payload.ContentBlocks[0].Kind)) == "" || payload.ContentBlocks[0].Text != expected.FinalContent {
		encoded, _ := json.Marshal(payload)
		t.Fatalf("completed message payload = %s, want final content %q", encoded, expected.FinalContent)
	}
}
