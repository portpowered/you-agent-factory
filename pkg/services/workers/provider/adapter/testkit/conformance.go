// Package testkit provides reusable behavioral conformance checks for
// provider-neutral response adapters.
package testkit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	responseevents "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
)

const (
	maximumDiagnosticLength = 160
	maximumToolDeltaLength  = 256
)

// FullStreamFixture supplies opaque provider-native observations and the
// provider-neutral facts a full-stream adapter must produce from them.
type FullStreamFixture struct {
	NewAdapter          func() adapter.Adapter
	Request             workerexecution.ProviderInferenceRequest
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
	Capabilities    adapter.Capabilities
	ProviderSession workerexecution.ProviderSessionMetadata
	ProviderRef     string
	MessageItemID   string
	ToolItemID      string
	ToolCallID      string
	MessageDeltas   []string
	FinalContent    string
	ToolOutputDelta string
	RetryAfter      int64
}

type expectedKindPhase struct {
	kind  responseevents.Kind
	phase responseevents.Phase
}

// RunFullStream runs the shared conformance contract for an adapter that
// declares and emits a native semantic stream.
func RunFullStream(t *testing.T, fixture FullStreamFixture) {
	t.Helper()
	requireFixture(t, fixture)

	t.Run("capabilities", func(t *testing.T) {
		assertFullStreamContract(t, fixture.NewAdapter(), fixture.Request, expectedCapabilities(fixture.Expected))
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
		CommandResult: workerprocess.CommandResult{
			Stdout: observationsForStream(fixture.RetryableFailure, adapter.OutputStreamStdout), ExitCode: 75,
		},
		ParseError:  parseErr,
		FlushReason: adapter.FlushReasonTerminated,
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
	if failure == nil || !failure.Retry.Retryable || failure.Family != workerexecution.WorkFailureFamilyThrottle {
		t.Fatalf("ClassifyFailure() = %#v", result)
	}
	if failure.Type != workerexecution.WorkFailureTypeThrottled {
		t.Fatalf("normalized retry facts = %#v", failure)
	}
	if expected.RetryAfter > 0 && (failure.Retry.RetryAfter == nil || int64(failure.Retry.RetryAfter.Seconds()) != expected.RetryAfter) {
		t.Fatalf("retry-after facts = %#v", failure)
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

func assertFullStreamContract(t *testing.T, providerAdapter adapter.Adapter, request workerexecution.ProviderInferenceRequest, expected adapter.Capabilities) {
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
	if got != expected {
		t.Fatalf("full-stream capabilities = %#v, want %#v", got, expected)
	}
}

func expectedCapabilities(expected FullStreamExpected) adapter.Capabilities {
	if expected.Capabilities != (adapter.Capabilities{}) {
		return expected.Capabilities
	}
	return adapter.Capabilities{
		NativeStreaming: true, MessageDeltas: true, MessageSnapshots: true,
		ToolLifecycle: true, ToolOutputDeltas: expected.ToolOutputDelta != "", StableItemIDs: true,
	}
}

func requireFixture(t *testing.T, fixture FullStreamFixture) {
	t.Helper()
	requireNoError(t, validateFullStreamFixture(fixture))
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func validateFullStreamFixture(fixture FullStreamFixture) error {
	if fixture.NewAdapter == nil {
		return fmt.Errorf("NewAdapter is required")
	}
	if len(fixture.ContentAndTools) == 0 || len(fixture.RetryableFailure) == 0 || len(fixture.UnsafeAndRecovering) == 0 || len(fixture.UnterminatedFinal) == 0 {
		return fmt.Errorf("all full-stream observation fixtures are required")
	}
	return nil
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
	semantic := withoutOptionalRunTerminal(drafts)
	assertDraftSequence(t, semantic, expected)
	assertMessageLifecycle(t, semantic, expected)
	assertToolLifecycle(t, semantic, expected)
}

func withoutOptionalRunTerminal(drafts []responseevents.Draft) []responseevents.Draft {
	if len(drafts) == 0 {
		return drafts
	}
	last := drafts[len(drafts)-1]
	if last.Kind == responseevents.KindRun && (last.Phase == responseevents.PhaseCompleted || last.Phase == responseevents.PhaseFailed || last.Phase == responseevents.PhaseCanceled) {
		return drafts[:len(drafts)-1]
	}
	return drafts
}

func assertDraftSequence(t *testing.T, drafts []responseevents.Draft, expected FullStreamExpected) {
	t.Helper()
	wantKinds := []expectedKindPhase{{responseevents.KindRun, responseevents.PhaseStarted}}
	for range expected.MessageDeltas {
		wantKinds = append(wantKinds, expectedKindPhase{responseevents.KindMessage, responseevents.PhaseDelta})
	}
	wantKinds = append(wantKinds, expectedKindPhase{responseevents.KindMessage, responseevents.PhaseCompleted})
	if expected.ToolItemID == "" {
		assertKindsAndCorrelations(t, drafts, wantKinds, expected)
		return
	}
	if expectedCapabilities(expected).ToolLifecycle {
		wantKinds = append(wantKinds, expectedKindPhase{responseevents.KindTool, responseevents.PhaseStarted})
	}
	if expected.ToolOutputDelta != "" {
		wantKinds = append(wantKinds, expectedKindPhase{responseevents.KindTool, responseevents.PhaseDelta})
	}
	wantKinds = append(wantKinds, expectedKindPhase{responseevents.KindTool, responseevents.PhaseCompleted})
	assertKindsAndCorrelations(t, drafts, wantKinds, expected)
}

func assertKindsAndCorrelations(t *testing.T, drafts []responseevents.Draft, wantKinds []expectedKindPhase, expected FullStreamExpected) {
	t.Helper()
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
	delivery := drafts[0].Provenance.Delivery
	if (delivery != responseevents.DeliverySynthesized && delivery != responseevents.DeliveryNativeStream) || drafts[0].RunID == "" || drafts[0].DispatchID == "" {
		t.Fatalf("run start is not observable and correlated: %#v", drafts[0])
	}
}

func assertMessageLifecycle(t *testing.T, drafts []responseevents.Draft, expected FullStreamExpected) {
	t.Helper()
	var deltaIndex int
	for _, draft := range drafts {
		if draft.Kind != responseevents.KindMessage || draft.Phase != responseevents.PhaseDelta {
			continue
		}
		if deltaIndex >= len(expected.MessageDeltas) {
			t.Fatalf("unexpected message delta: %#v", draft)
		}
		var payload responseevents.MessageDeltaPayload
		mustDecodePayload(t, draft, &payload)
		if payload.TextDelta != expected.MessageDeltas[deltaIndex] || draft.ItemID != expected.MessageItemID {
			t.Fatalf("message delta[%d] = %#v item=%q", deltaIndex, payload, draft.ItemID)
		}
		deltaIndex++
	}
	if deltaIndex != len(expected.MessageDeltas) {
		t.Fatalf("message delta count = %d, want %d", deltaIndex, len(expected.MessageDeltas))
	}
	completed := findDraft(drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
	if completed == nil {
		t.Fatalf("completed message absent: %#v", drafts)
	}
	if got := messageText(t, *completed); got != expected.FinalContent || completed.ItemID != expected.MessageItemID {
		t.Fatalf("message snapshot = %q item=%q", got, completed.ItemID)
	}
}

func assertToolLifecycle(t *testing.T, drafts []responseevents.Draft, expected FullStreamExpected) {
	t.Helper()
	if expected.ToolItemID == "" {
		return
	}
	var tools []responseevents.Draft
	for _, draft := range drafts {
		if draft.Kind == responseevents.KindTool {
			tools = append(tools, draft)
		}
	}
	for index := range tools {
		if tools[index].ItemID != expected.ToolItemID {
			t.Fatalf("tool draft[%d] item = %q, want %q", index, tools[index].ItemID, expected.ToolItemID)
		}
		var payload responseevents.ToolPayload
		if tools[index].Phase != responseevents.PhaseDelta {
			mustDecodePayload(t, tools[index], &payload)
			if payload.ToolCallID != expected.ToolCallID {
				t.Fatalf("tool draft[%d] is not correlated: %#v", index, payload)
			}
		}
	}
	if expected.ToolOutputDelta != "" {
		var delta responseevents.ToolDeltaPayload
		deltaDraft := findDraft(drafts, responseevents.KindTool, responseevents.PhaseDelta)
		if deltaDraft == nil {
			t.Fatalf("tool delta absent: %#v", drafts)
		}
		mustDecodePayload(t, *deltaDraft, &delta)
		if delta.ToolCallID != expected.ToolCallID || delta.OutputDelta != expected.ToolOutputDelta {
			t.Fatalf("tool output delta = %#v", delta)
		}
		if len([]rune(delta.OutputDelta)) > maximumToolDeltaLength {
			t.Fatalf("tool output delta length = %d, want <= %d", len([]rune(delta.OutputDelta)), maximumToolDeltaLength)
		}
	}
}

func assertRetryDraft(t *testing.T, drafts []responseevents.Draft, retryAfter int64) {
	t.Helper()
	draft := findDraft(drafts, responseevents.KindError, responseevents.PhaseUpdated)
	if draft == nil {
		draft = findDraft(drafts, responseevents.KindError, responseevents.PhaseFailed)
	}
	if draft == nil {
		t.Fatalf("retry drafts = %#v, want ERROR/UPDATED notification", drafts)
	}
	var payload responseevents.ErrorPayload
	mustDecodePayload(t, *draft, &payload)
	if !payload.Retryable || (retryAfter > 0 && (payload.RetryAfterSeconds == nil || *payload.RetryAfterSeconds != retryAfter)) {
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
	if err := validateSafeText(value, forbidden); err != nil {
		t.Fatalf("%s %v", field, err)
	}
}

func validateSafeText(value string, forbidden []string) error {
	if len([]rune(value)) > maximumDiagnosticLength {
		return fmt.Errorf("length = %d, want <= %d", len([]rune(value)), maximumDiagnosticLength)
	}
	for _, secret := range forbidden {
		if secret != "" && strings.Contains(value, secret) {
			return fmt.Errorf("disclosed forbidden input %q", secret)
		}
	}
	return nil
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
	requireNoError(t, json.Unmarshal(draft.Payload, target))
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
