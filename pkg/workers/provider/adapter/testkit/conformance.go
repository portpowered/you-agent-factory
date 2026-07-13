// Package testkit provides reusable behavioral conformance checks for
// provider-neutral response adapters.
package testkit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

const (
	maximumDiagnosticLength = 160
	maximumToolDeltaLength  = 256
)

// FullStreamFixture supplies opaque provider-native observations and the
// provider-neutral facts a full-stream adapter must produce from them.
type FullStreamFixture struct {
	NewAdapter          func() adapter.Adapter
	Request             interfaces.ProviderInferenceRequest
	ContentAndTools     []adapter.Observation
	RetryableFailure    []adapter.Observation
	UnsafeAndRecovering []adapter.Observation
	UnterminatedFinal   []adapter.Observation
	FinalResult         workerprocess.CommandResult
	Expected            FullStreamExpected
	ForbiddenDiagnostic []string
}

// FullStreamExpected identifies stable semantic values used by conformance
// assertions without exposing the fixture's native event vocabulary.
type FullStreamExpected struct {
	ProviderSession interfaces.ProviderSessionMetadata
	ProviderRef     string
	MessageItemID   string
	ToolItemID      string
	ToolCallID      string
	MessageDeltas   []string
	FinalContent    string
	ToolOutputDelta string
	RetryAfter      int64
}

// RunFullStream runs the shared conformance contract for an adapter that
// declares and emits a native semantic stream.
func RunFullStream(t *testing.T, fixture FullStreamFixture) {
	t.Helper()
	requireFixture(t, fixture)

	t.Run("capabilities", func(t *testing.T) {
		assertFullStreamContract(t, fixture.NewAdapter(), fixture.Request)
	})
	t.Run("ordered content and correlated tools", func(t *testing.T) {
		verifyContentAndTools(t, fixture)
	})
	t.Run("authoritative final result", func(t *testing.T) {
		verifyAuthoritativeFinal(t, fixture)
	})
	t.Run("retry notification and failure facts", func(t *testing.T) {
		verifyRetryFailure(t, fixture)
	})
	t.Run("unsafe input is bounded and recovery continues", func(t *testing.T) {
		verifyUnsafeRecovery(t, fixture)
	})
	t.Run("flush processes final unterminated record", func(t *testing.T) {
		verifyUnterminatedFlush(t, fixture)
	})
}

func verifyContentAndTools(t *testing.T, fixture FullStreamFixture) {
	t.Helper()
	drafts, diagnostics := decode(t, fixture.NewAdapter(), fixture.ContentAndTools, adapter.FlushReasonCompleted)
	assertSafeDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
	assertContentAndTools(t, drafts, fixture.Expected)
}

func verifyAuthoritativeFinal(t *testing.T, fixture FullStreamFixture) {
	t.Helper()
	result, err := fixture.NewAdapter().ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: fixture.FinalResult,
		FlushReason:   adapter.FlushReasonCompleted,
	})
	if err != nil {
		t.Fatalf("ParseFinal() error = %v", err)
	}
	if result.Response.Content != fixture.Expected.FinalContent {
		t.Fatalf("final content = %q, want %q", result.Response.Content, fixture.Expected.FinalContent)
	}
	if result.Response.ProviderSession == nil || *result.Response.ProviderSession != fixture.Expected.ProviderSession {
		t.Fatalf("provider session = %#v, want %#v", result.Response.ProviderSession, fixture.Expected.ProviderSession)
	}
	if len(result.Drafts) != 0 {
		t.Fatalf("ParseFinal() duplicated streamed semantic drafts: %#v", result.Drafts)
	}
}

func verifyRetryFailure(t *testing.T, fixture FullStreamFixture) {
	t.Helper()
	providerAdapter := fixture.NewAdapter()
	drafts, diagnostics := decode(t, providerAdapter, fixture.RetryableFailure, adapter.FlushReasonTerminated)
	assertSafeDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
	assertRetryDraft(t, drafts, fixture.Expected.RetryAfter)
	parseErr := parseRetryFailure(t, providerAdapter, fixture.RetryableFailure)
	failure := providerAdapter.ClassifyFailure(context.Background(), adapter.FailureContext{
		CommandResult: workerprocess.CommandResult{ExitCode: 75},
		ParseError:    parseErr,
		FlushReason:   adapter.FlushReasonTerminated,
	})
	assertRetryFailure(t, failure, fixture.Expected, fixture.ForbiddenDiagnostic)
}

func parseRetryFailure(t *testing.T, providerAdapter adapter.Adapter, observations []adapter.Observation) error {
	t.Helper()
	_, parseErr := providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{Stdout: observationsForStream(observations, adapter.OutputStreamStdout), ExitCode: 75},
		FlushReason:   adapter.FlushReasonTerminated,
	})
	if parseErr == nil {
		t.Fatal("ParseFinal() error = nil, want retryable provider failure")
	}
	return parseErr
}

func assertRetryFailure(t *testing.T, result adapter.FailureResult, expected FullStreamExpected, forbidden []string) {
	t.Helper()
	failure := result.Failure
	if failure == nil || !failure.Retry.Retryable || failure.Family != interfaces.WorkFailureFamilyThrottle {
		t.Fatalf("ClassifyFailure() = %#v", result)
	}
	if failure.Type != interfaces.WorkFailureTypeThrottled || failure.Retry.RetryAfter == nil || int64(failure.Retry.RetryAfter.Seconds()) != expected.RetryAfter {
		t.Fatalf("normalized retry facts = %#v", failure)
	}
	if failure.ProviderSession == nil || *failure.ProviderSession != expected.ProviderSession {
		t.Fatalf("failure provider session = %#v", failure.ProviderSession)
	}
	assertSafeText(t, "failure message", failure.Message, forbidden)
}

func verifyUnsafeRecovery(t *testing.T, fixture FullStreamFixture) {
	t.Helper()
	drafts, diagnostics := decode(t, fixture.NewAdapter(), fixture.UnsafeAndRecovering, adapter.FlushReasonCompleted)
	assertSafeDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
	if len(diagnostics) == 0 {
		t.Fatal("diagnostics = nil, want a safe malformed or unknown-record diagnostic")
	}
	if findDraft(drafts, responseevents.KindMessage, responseevents.PhaseCompleted) == nil {
		t.Fatalf("later valid completed message was not decoded: %#v", drafts)
	}
}

func verifyUnterminatedFlush(t *testing.T, fixture FullStreamFixture) {
	t.Helper()
	drafts, diagnostics := decode(t, fixture.NewAdapter(), fixture.UnterminatedFinal, adapter.FlushReasonCompleted)
	assertSafeDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
	message := findDraft(drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
	if message == nil {
		t.Fatalf("flush drafts = %#v, want authoritative completed message", drafts)
	}
	if got := messageText(t, *message); got != fixture.Expected.FinalContent {
		t.Fatalf("flushed message = %q, want %q", got, fixture.Expected.FinalContent)
	}
}

func assertFullStreamContract(t *testing.T, providerAdapter adapter.Adapter, request interfaces.ProviderInferenceRequest) {
	t.Helper()
	command, err := providerAdapter.BuildCommand(context.Background(), adapter.CommandContext{Request: request})
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	if strings.TrimSpace(command.Request.Command) == "" {
		t.Fatal("BuildCommand() returned an empty command")
	}
	result, err := providerAdapter.Capabilities(context.Background(), adapter.CapabilityContext{Request: request})
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	got := result.Capabilities
	if !got.NativeStreaming || !got.MessageDeltas || !got.MessageSnapshots || !got.ToolLifecycle || !got.ToolOutputDeltas || !got.StableItemIDs || got.FinalOnly {
		t.Fatalf("full-stream capabilities = %#v", got)
	}
}

func requireFixture(t *testing.T, fixture FullStreamFixture) {
	t.Helper()
	if fixture.NewAdapter == nil {
		t.Fatal("NewAdapter is required")
	}
	if len(fixture.ContentAndTools) == 0 || len(fixture.RetryableFailure) == 0 || len(fixture.UnsafeAndRecovering) == 0 || len(fixture.UnterminatedFinal) == 0 {
		t.Fatal("all full-stream observation fixtures are required")
	}
}

func decode(t *testing.T, providerAdapter adapter.Adapter, observations []adapter.Observation, reason adapter.FlushReason) ([]responseevents.Draft, []adapter.Diagnostic) {
	t.Helper()
	decoder, err := providerAdapter.NewDecoder(context.Background(), adapter.DecoderContext{RunID: "run-conformance", DispatchID: "dispatch-conformance"})
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	var drafts []responseevents.Draft
	var diagnostics []adapter.Diagnostic
	for _, observation := range observations {
		result, observeErr := decoder.Observe(context.Background(), observation)
		if observeErr != nil {
			t.Fatalf("Observe(%q) error = %v", observation.Stream, observeErr)
		}
		drafts = append(drafts, result.Drafts...)
		diagnostics = append(diagnostics, result.Diagnostics...)
	}
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: reason})
	if err != nil {
		t.Fatalf("Flush(%q) error = %v", reason, err)
	}
	drafts = append(drafts, flushed.Drafts...)
	diagnostics = append(diagnostics, flushed.Diagnostics...)
	for index, draft := range drafts {
		if err := responseevents.ValidateDraft(draft); err != nil {
			t.Fatalf("draft[%d] is not canonical: %v", index, err)
		}
	}
	return drafts, diagnostics
}

func assertContentAndTools(t *testing.T, drafts []responseevents.Draft, expected FullStreamExpected) {
	t.Helper()
	assertDraftSequence(t, drafts, expected)
	assertMessageLifecycle(t, drafts, expected)
	assertToolLifecycle(t, drafts, expected)
}

func assertDraftSequence(t *testing.T, drafts []responseevents.Draft, expected FullStreamExpected) {
	t.Helper()
	wantKinds := []struct {
		kind  responseevents.Kind
		phase responseevents.Phase
	}{
		{responseevents.KindRun, responseevents.PhaseStarted},
		{responseevents.KindMessage, responseevents.PhaseDelta},
		{responseevents.KindMessage, responseevents.PhaseDelta},
		{responseevents.KindMessage, responseevents.PhaseCompleted},
		{responseevents.KindTool, responseevents.PhaseStarted},
		{responseevents.KindTool, responseevents.PhaseDelta},
		{responseevents.KindTool, responseevents.PhaseCompleted},
	}
	if len(drafts) != len(wantKinds) {
		t.Fatalf("draft count = %d, want %d: %#v", len(drafts), len(wantKinds), drafts)
	}
	for index, want := range wantKinds {
		got := drafts[index]
		if got.Kind != want.kind || got.Phase != want.phase {
			t.Fatalf("draft[%d] kind/phase = %s/%s, want %s/%s", index, got.Kind, got.Phase, want.kind, want.phase)
		}
		if index > 0 && got.ProviderSessionRef != expected.ProviderRef {
			t.Fatalf("draft[%d] provider session ref = %q, want %q", index, got.ProviderSessionRef, expected.ProviderRef)
		}
	}
	if drafts[0].Provenance.Delivery != responseevents.DeliverySynthesized || drafts[0].RunID == "" || drafts[0].DispatchID == "" {
		t.Fatalf("run start is not synthesized and correlated: %#v", drafts[0])
	}
}

func assertMessageLifecycle(t *testing.T, drafts []responseevents.Draft, expected FullStreamExpected) {
	t.Helper()
	for index, want := range expected.MessageDeltas {
		var payload responseevents.MessageDeltaPayload
		mustDecodePayload(t, drafts[index+1], &payload)
		if payload.TextDelta != want || drafts[index+1].ItemID != expected.MessageItemID {
			t.Fatalf("message delta[%d] = %#v item=%q", index, payload, drafts[index+1].ItemID)
		}
	}
	if got := messageText(t, drafts[3]); got != expected.FinalContent || drafts[3].ItemID != expected.MessageItemID {
		t.Fatalf("message snapshot = %q item=%q", got, drafts[3].ItemID)
	}
}

func assertToolLifecycle(t *testing.T, drafts []responseevents.Draft, expected FullStreamExpected) {
	t.Helper()
	for _, index := range []int{4, 5, 6} {
		if drafts[index].ItemID != expected.ToolItemID {
			t.Fatalf("tool draft[%d] item = %q, want %q", index, drafts[index].ItemID, expected.ToolItemID)
		}
	}
	var started, completed responseevents.ToolPayload
	mustDecodePayload(t, drafts[4], &started)
	mustDecodePayload(t, drafts[6], &completed)
	var delta responseevents.ToolDeltaPayload
	mustDecodePayload(t, drafts[5], &delta)
	if started.ToolCallID != expected.ToolCallID || delta.ToolCallID != expected.ToolCallID || completed.ToolCallID != expected.ToolCallID || delta.OutputDelta != expected.ToolOutputDelta {
		t.Fatalf("tool lifecycle is not correlated: start=%#v delta=%#v completed=%#v", started, delta, completed)
	}
	if len([]rune(delta.OutputDelta)) > maximumToolDeltaLength {
		t.Fatalf("tool output delta length = %d, want <= %d", len([]rune(delta.OutputDelta)), maximumToolDeltaLength)
	}
}

func assertRetryDraft(t *testing.T, drafts []responseevents.Draft, retryAfter int64) {
	t.Helper()
	draft := findDraft(drafts, responseevents.KindError, responseevents.PhaseUpdated)
	if draft == nil {
		t.Fatalf("retry drafts = %#v, want ERROR/UPDATED notification", drafts)
	}
	var payload responseevents.ErrorPayload
	mustDecodePayload(t, *draft, &payload)
	if !payload.Retryable || payload.RetryAfterSeconds == nil || *payload.RetryAfterSeconds != retryAfter {
		t.Fatalf("retry payload = %#v", payload)
	}
}

func assertSafeDiagnostics(t *testing.T, diagnostics []adapter.Diagnostic, forbidden []string) {
	t.Helper()
	for index, diagnostic := range diagnostics {
		if strings.TrimSpace(diagnostic.Code) == "" || strings.TrimSpace(diagnostic.Message) == "" {
			t.Fatalf("diagnostic[%d] is incomplete: %#v", index, diagnostic)
		}
		assertSafeText(t, "diagnostic code", diagnostic.Code, forbidden)
		assertSafeText(t, "diagnostic message", diagnostic.Message, forbidden)
	}
}

func assertSafeText(t *testing.T, field, value string, forbidden []string) {
	t.Helper()
	if len([]rune(value)) > maximumDiagnosticLength {
		t.Fatalf("%s length = %d, want <= %d", field, len([]rune(value)), maximumDiagnosticLength)
	}
	for _, secret := range forbidden {
		if secret != "" && strings.Contains(value, secret) {
			t.Fatalf("%s disclosed forbidden input %q", field, secret)
		}
	}
}

func findDraft(drafts []responseevents.Draft, kind responseevents.Kind, phase responseevents.Phase) *responseevents.Draft {
	for index := range drafts {
		if drafts[index].Kind == kind && drafts[index].Phase == phase {
			return &drafts[index]
		}
	}
	return nil
}

func messageText(t *testing.T, draft responseevents.Draft) string {
	t.Helper()
	var payload responseevents.MessagePayload
	mustDecodePayload(t, draft, &payload)
	if len(payload.ContentBlocks) != 1 || payload.ContentBlocks[0].Kind != responseevents.ContentBlockText {
		t.Fatalf("message payload = %#v, want one text block", payload)
	}
	return payload.ContentBlocks[0].Text
}

func mustDecodePayload(t *testing.T, draft responseevents.Draft, target any) {
	t.Helper()
	if err := json.Unmarshal(draft.Payload, target); err != nil {
		t.Fatalf("decode %s/%s payload: %v", draft.Kind, draft.Phase, err)
	}
}

func observationsForStream(observations []adapter.Observation, stream adapter.OutputStream) []byte {
	var output []byte
	for _, observation := range observations {
		if observation.Stream == stream {
			output = append(output, observation.Chunk...)
		}
	}
	return output
}
