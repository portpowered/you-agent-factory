package codex_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/work"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/codex"
)

const lifecycleFixture = `{"type":"thread.started","thread_id":"thread-codex-123","unrelated_session_id":"wrong"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item-message-1","type":"agent_message","text":"authoritative answer"}}
{"type":"turn.completed","usage":{"input_tokens":4,"output_tokens":2}}
`

func TestDecoderMapsThreadTurnAndCompletedAgentMessageExactly(t *testing.T) {
	decoder := codex.NewDecoder(adapter.DecoderContext{RunID: "run-1", DispatchID: "dispatch-1"})
	cut := len(lifecycleFixture) / 2
	var drafts []responseevents.Draft
	for _, chunk := range []string{lifecycleFixture[:cut], lifecycleFixture[cut:]} {
		decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: []byte(chunk)})
		if err != nil {
			t.Fatalf("Observe() error = %v", err)
		}
		drafts = append(drafts, decoded.Drafts...)
	}
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: adapter.FlushReasonCompleted})
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	drafts = append(drafts, flushed.Drafts...)
	if len(drafts) != 5 {
		t.Fatalf("draft count = %d, want 5: %#v", len(drafts), drafts)
	}
	wantKinds := []responseevents.Kind{responseevents.KindSession, responseevents.KindTurn, responseevents.KindMessage, responseevents.KindUsage, responseevents.KindTurn}
	assertDraftKindsAndSession(t, drafts, wantKinds, "thread-codex-123")
	message := drafts[2]
	if message.ItemID != "item-message-1" || message.Phase != responseevents.PhaseCompleted || message.TurnID != drafts[1].TurnID {
		t.Fatalf("message correlation = %#v", message)
	}
	var payload responseevents.MessagePayload
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.ContentBlocks) != 1 || payload.ContentBlocks[0].Text != "authoritative answer" {
		t.Fatalf("message payload = %#v", payload)
	}
	var usage responseevents.UsagePayload
	if err := json.Unmarshal(drafts[3].Payload, &usage); err != nil {
		t.Fatal(err)
	}
	if usage.InputTokens != 4 || usage.OutputTokens != 2 {
		t.Fatalf("usage payload = %#v", usage)
	}
}

func assertDraftKindsAndSession(t *testing.T, drafts []responseevents.Draft, wantKinds []responseevents.Kind, wantSession string) {
	t.Helper()
	for index, draft := range drafts {
		if err := responseevents.ValidateDraft(draft); err != nil {
			t.Fatalf("draft[%d] invalid: %v", index, err)
		}
		if draft.Kind != wantKinds[index] || draft.ProviderSessionRef != wantSession {
			t.Fatalf("draft[%d] = %#v", index, draft)
		}
	}
}

func TestParseFinalOutputIsIndependentAndUsesOnlyThreadID(t *testing.T) {
	parsed, err := codex.ParseFinalOutput([]byte(lifecycleFixture))
	if err != nil {
		t.Fatalf("ParseFinalOutput() error = %v", err)
	}
	if parsed.Content != "authoritative answer" {
		t.Fatalf("content = %q", parsed.Content)
	}
	if parsed.ProviderSession == nil || parsed.ProviderSession.ID != "thread-codex-123" || parsed.ProviderSession.Kind != codex.ProviderSessionKindSessionID {
		t.Fatalf("provider session = %#v", parsed.ProviderSession)
	}
	if strings.Contains(parsed.Content, "thread.started") {
		t.Fatalf("content contains streamed observation: %q", parsed.Content)
	}
}

func TestDecoderPreservesEverySupportedItemSemanticAndIdentity(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "supported_items.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := codex.NewDecoder(adapter.DecoderContext{RunID: "run-items", DispatchID: "dispatch-items"})
	decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: fixture})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if len(decoded.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", decoded.Diagnostics)
	}
	if len(decoded.Drafts) != 22 {
		t.Fatalf("draft count = %d, want 22", len(decoded.Drafts))
	}
	for index, draft := range decoded.Drafts {
		if err := responseevents.ValidateDraft(draft); err != nil {
			t.Fatalf("draft[%d] invalid: %v", index, err)
		}
		if draft.ProviderSessionRef != "thread-items-123" {
			t.Fatalf("draft[%d] provider session = %q", index, draft.ProviderSessionRef)
		}
	}

	byID := draftsByItemID(decoded.Drafts)
	if got, want := itemIDOrder(decoded.Drafts), []string{"reason-1", "reason-1", "command-1", "command-1", "files-1", "mcp-1", "mcp-1", "collab-1", "collab-1", "web-1", "web-1", "error-1", "error-1", "error-1", "plan-1", "plan-1", "plan-1", "message-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("item order = %#v, want %#v", got, want)
	}
	assertItemLifecycle(t, byID["reason-1"], responseevents.KindReasoning, responseevents.PhaseStarted, responseevents.PhaseCompleted)
	assertItemLifecycle(t, byID["command-1"], responseevents.KindTool, responseevents.PhaseStarted, responseevents.PhaseCompleted)
	assertItemLifecycle(t, byID["mcp-1"], responseevents.KindTool, responseevents.PhaseStarted, responseevents.PhaseCompleted)
	assertItemLifecycle(t, byID["collab-1"], responseevents.KindTool, responseevents.PhaseStarted, responseevents.PhaseCompleted)
	assertItemLifecycle(t, byID["web-1"], responseevents.KindTool, responseevents.PhaseStarted, responseevents.PhaseCompleted)
	assertItemLifecycle(t, byID["error-1"], responseevents.KindError, responseevents.PhaseUpdated, responseevents.PhaseUpdated, responseevents.PhaseUpdated)
	assertItemLifecycle(t, byID["plan-1"], responseevents.KindPlan, responseevents.PhaseUpdated, responseevents.PhaseUpdated, responseevents.PhaseUpdated)
	assertItemLifecycle(t, byID["message-1"], responseevents.KindMessage, responseevents.PhaseCompleted)
	assertItemLifecycle(t, byID["files-1"], responseevents.KindFileChange, responseevents.PhaseUpdated)

	assertItemPayloads(t, byID)
}

func TestDecoderClassifiesOnlyExactNestedItemTypes(t *testing.T) {
	decoder := codex.NewDecoder(adapter.DecoderContext{DispatchID: "dispatch-exact"})
	decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"near-match\",\"type\":\"command_execution_result\",\"command\":\"must not run\"}}\n")})
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Drafts) != 0 || len(decoded.Diagnostics) != 1 || decoded.Diagnostics[0].Code != "codex_unknown_item" {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestDecoderMapsErrorItemAsBoundedNonTerminalObservation(t *testing.T) {
	message := strings.Repeat("recoverable ", 200)
	record := `{"type":"item.completed","item":{"id":"error-bounded","type":"error","message":` + mustMarshalString(t, message) + `}}` + "\n"
	decoder := codex.NewDecoder(adapter.DecoderContext{DispatchID: "dispatch-error-item"})
	decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: []byte(record)})
	if err != nil || len(decoded.Diagnostics) != 0 || len(decoded.Drafts) != 1 {
		t.Fatalf("Observe() = %#v, %v", decoded, err)
	}
	draft := decoded.Drafts[0]
	if draft.Kind != responseevents.KindError || draft.Phase != responseevents.PhaseUpdated || draft.ItemID != "error-bounded" {
		t.Fatalf("error item draft = %#v", draft)
	}
	var payload responseevents.ErrorPayload
	decodePayload(t, draft, &payload)
	if payload.Code != "codex_item_error" || len(payload.Message) > 1027 || !strings.HasSuffix(payload.Message, "...") {
		t.Fatalf("bounded error item payload = %#v", payload)
	}
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: adapter.FlushReasonCompleted})
	if err != nil || len(flushed.Drafts) != 0 {
		t.Fatalf("Flush() promoted non-terminal item = %#v, %v", flushed, err)
	}
}

func TestDecoderDoesNotExposeUnknownDiscriminatorValues(t *testing.T) {
	t.Parallel()
	const secret = "private prompt sk-codex-secret"
	decoder := codex.NewDecoder(adapter.DecoderContext{DispatchID: "dispatch-redaction"})
	decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: []byte(
		`{"type":"` + secret + `"}` + "\n" +
			`{"type":"item.completed","item":{"id":"future-1","type":"` + secret + `"}}` + "\n",
	)})
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want unknown event and item diagnostics", decoded.Diagnostics)
	}
	for _, diagnostic := range decoded.Diagnostics {
		if strings.Contains(diagnostic.Code+diagnostic.Message, secret) {
			t.Fatalf("diagnostic exposed unknown discriminator: %#v", diagnostic)
		}
	}
}

func TestDecoderMapsFullUsageAndSelectsOneTypedTerminalFactAtFlush(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-terminal-1"}`,
		`{"type":"turn.started"}`,
		`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":25,"reasoning_output_tokens":5}}`,
		`{"type":"turn.failed","error":{"message":"You've hit your usage limit"}}`,
		`{"type":"error","message":"unexpected status 500 from upstream"}`,
	}, "\n") + "\n"
	decoder := codex.NewDecoder(adapter.DecoderContext{RunID: "run-terminal", DispatchID: "dispatch-terminal"})
	decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: []byte(stream)})
	if err != nil {
		t.Fatal(err)
	}
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: adapter.FlushReasonCompleted})
	if err != nil {
		t.Fatal(err)
	}
	decoded.Drafts = append(decoded.Drafts, flushed.Drafts...)
	if len(decoded.Diagnostics) != 0 || len(decoded.Drafts) != 5 {
		t.Fatalf("decoded = %#v", decoded)
	}
	var usage responseevents.UsagePayload
	decodePayload(t, decoded.Drafts[2], &usage)
	if usage.InputTokens != 100 || usage.CachedInputTokens != 40 || usage.OutputTokens != 25 || usage.ReasoningOutputTokens != 5 {
		t.Fatalf("usage = %#v", usage)
	}
	draft := decoded.Drafts[4]
	if draft.Kind != responseevents.KindError || draft.Phase != responseevents.PhaseFailed || draft.ProviderSessionRef != "thread-terminal-1" {
		t.Fatalf("failure draft = %#v", draft)
	}
	var payload responseevents.ErrorPayload
	decodePayload(t, draft, &payload)
	if payload.Code != "codex_error" || !payload.Retryable || strings.Contains(payload.Message, "upstream") {
		t.Fatalf("failure payload = %#v", payload)
	}
}

func TestParseTerminalFailureUsesExactTypedRecordsAndRecognizedPrecedence(t *testing.T) {
	stream := strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-failure-1"}`,
		`{"type":"turn.failed","error":{"message":"unexpected status 429"}}`,
		`{"type":"error","message":"cleanup detail that must not override"}`,
	}, "\n") + "\n"
	failure, ok := codex.ParseTerminalFailure([]byte(stream))
	if !ok {
		t.Fatal("ParseTerminalFailure() did not find typed failure")
	}
	if failure.Type != workerexecution.WorkFailureTypeThrottled || !failure.Retryable || failure.NativeEventType != "turn.failed" {
		t.Fatalf("failure = %#v", failure)
	}
	if failure.ProviderSession == nil || failure.ProviderSession.ID != "thread-failure-1" || failure.Message != "Codex is temporarily unavailable due to usage or capacity limits." {
		t.Fatalf("failure identity/message = %#v", failure)
	}
}

func TestParseTerminalFailurePreservesCodexFailureCategoriesAndRetryPolicy(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		wantType  workerexecution.WorkFailureType
		retryable bool
	}{
		{name: "authentication", message: "unexpected status 401", wantType: workerexecution.WorkFailureTypeAuthFailure},
		{name: "bad request", message: "unexpected status 400", wantType: workerexecution.WorkFailureTypePermanentBadRequest},
		{name: "throttled", message: "You've hit your usage limit", wantType: workerexecution.WorkFailureTypeThrottled, retryable: true},
		{name: "server", message: "unexpected status 503", wantType: workerexecution.WorkFailureTypeInternalServerError, retryable: true},
		{name: "timeout", message: "command timed out", wantType: workerexecution.WorkFailureTypeTimeout, retryable: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record, err := json.Marshal(map[string]any{"type": "turn.failed", "error": map[string]string{"message": tc.message}})
			if err != nil {
				t.Fatal(err)
			}
			failure, ok := codex.ParseTerminalFailure(append(record, '\n'))
			if !ok || failure.Type != tc.wantType || failure.Retryable != tc.retryable {
				t.Fatalf("failure = %#v", failure)
			}
			if strings.Contains(failure.Message, tc.message) {
				t.Fatalf("failure message exposed native text: %q", failure.Message)
			}
		})
	}
}

func TestResponseAdapterSnapshotStreamConformance(t *testing.T) {
	t.Parallel()
	const (
		threadID = "thread-conformance-1"
		prompt   = "private prompt must not escape"
		secret   = "sk-codex-secret"
	)
	content := codexObservations(
		`{"type":"thread.started","thread_id":"`+threadID+`"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.started","item":{"id":"command-1","type":"command_execution","command":"go test ./...","status":"in_progress"}}`,
		`{"type":"item.updated","item":{"id":"command-1","type":"command_execution","aggregated_output":"ok","status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"command-1","type":"command_execution","command":"go test ./...","aggregated_output":"ok","status":"completed","exit_code":0}}`,
		`{"type":"item.completed","item":{"id":"message-1","type":"agent_message","text":"safe final"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":4,"output_tokens":2}}`,
	)
	session := workerexecution.ProviderSessionMetadata{Provider: "codex", Kind: codex.ProviderSessionKindSessionID, ID: threadID}
	runResponseAdapterConformance(t, responseAdapterFixture{
		NewAdapter: func() adapter.Adapter { return codex.NewResponseAdapter() },
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch:      work.WorkDispatch{DispatchID: "dispatch-conformance"},
			ModelProvider: "codex", Model: "gpt-test", UserMessage: prompt,
		},
		Content: content,
		TerminalFailure: codexObservations(
			`{"type":"thread.started","thread_id":"`+threadID+`"}`,
			`{"type":"turn.failed","error":{"message":"unexpected status 429"}}`,
		),
		UnsafeAndRecovering: codexObservations(
			`{"type":"thread.started","thread_id":"`+threadID+`"}`,
			`{"type":`+prompt+`,"token":"`+secret+`"}`,
			`{"type":"future.event","prompt":"`+prompt+`","token":"`+secret+`"}`,
			`{"type":"item.completed","item":{"id":"future-1","type":"future_item","text":"`+prompt+`"}}`,
			`{"type":"item.completed","item":{"id":"message-1","type":"agent_message","text":"safe final"}}`,
		),
		UnterminatedFinal: []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: []byte(
			`{"type":"thread.started","thread_id":"` + threadID + "\"}\n" +
				`{"type":"item.completed","item":{"id":"message-1","type":"agent_message","text":"safe final"}}`,
		)}},
		FinalResult: workerprocess.CommandResult{Stdout: observationsStdout(content)},
		Expected: responseAdapterExpected{
			Capabilities: adapter.Capabilities{
				NativeStreaming: true, MessageSnapshots: true, ReasoningSummaries: true,
				ToolLifecycle: true, ToolOutputDeltas: true, StableItemIDs: true,
			},
			Drafts: []draftExpectation{
				{Kind: responseevents.KindSession, Phase: responseevents.PhaseStarted, ProviderRef: threadID},
				{Kind: responseevents.KindTurn, Phase: responseevents.PhaseStarted, ProviderRef: threadID},
				{Kind: responseevents.KindTool, Phase: responseevents.PhaseStarted, ItemID: "command-1", ProviderRef: threadID},
				{Kind: responseevents.KindTool, Phase: responseevents.PhaseDelta, ItemID: "command-1", ProviderRef: threadID},
				{Kind: responseevents.KindTool, Phase: responseevents.PhaseCompleted, ItemID: "command-1", ProviderRef: threadID},
				{Kind: responseevents.KindMessage, Phase: responseevents.PhaseCompleted, ItemID: "message-1", ProviderRef: threadID},
				{Kind: responseevents.KindUsage, Phase: responseevents.PhaseUpdated, ProviderRef: threadID},
				{Kind: responseevents.KindTurn, Phase: responseevents.PhaseCompleted, ProviderRef: threadID},
			},
			ProviderSession: session, FinalContent: "safe final",
			FailureFamily: workerexecution.WorkFailureFamilyThrottle, FailureType: workerexecution.WorkFailureTypeThrottled, FailureRetryable: true,
		},
		ForbiddenDiagnostic: []string{prompt, secret},
	})
}

type responseAdapterFixture struct {
	NewAdapter          func() adapter.Adapter
	Request             workerexecution.ProviderInferenceRequest
	Content             []adapter.Observation
	TerminalFailure     []adapter.Observation
	UnsafeAndRecovering []adapter.Observation
	UnterminatedFinal   []adapter.Observation
	FinalResult         workerprocess.CommandResult
	Expected            responseAdapterExpected
	ForbiddenDiagnostic []string
}

type responseAdapterExpected struct {
	Capabilities     adapter.Capabilities
	Drafts           []draftExpectation
	ProviderSession  workerexecution.ProviderSessionMetadata
	FinalContent     string
	FailureFamily    workerexecution.WorkFailureFamily
	FailureType      workerexecution.WorkFailureType
	FailureRetryable bool
}

type draftExpectation struct {
	Kind        responseevents.Kind
	Phase       responseevents.Phase
	ItemID      string
	ProviderRef string
}

func runResponseAdapterConformance(t *testing.T, fixture responseAdapterFixture) {
	t.Helper()
	t.Run("declared capabilities and command", func(t *testing.T) { verifyAdapterCapabilities(t, fixture) })
	t.Run("ordered canonical snapshots", func(t *testing.T) { verifyAdapterDrafts(t, fixture) })
	t.Run("authoritative final result", func(t *testing.T) { verifyAdapterFinal(t, fixture) })
	t.Run("unsafe input recovers", func(t *testing.T) { verifyAdapterRecovery(t, fixture) })
	t.Run("flush processes final unterminated record", func(t *testing.T) { verifyAdapterFlush(t, fixture) })
	t.Run("terminal failure reconciliation", func(t *testing.T) { verifyAdapterFailure(t, fixture) })
}

func verifyAdapterCapabilities(t *testing.T, fixture responseAdapterFixture) {
	t.Helper()
	providerAdapter := fixture.NewAdapter()
	command, err := providerAdapter.BuildCommand(context.Background(), adapter.CommandContext{Request: fixture.Request})
	if err != nil || command.Request.Command != "codex" || !reflect.DeepEqual(command.Request.Args, []string{"exec", "--json", "--model", "gpt-test", "-"}) || string(command.Request.Stdin) != fixture.Request.UserMessage {
		t.Fatalf("BuildCommand() = %#v, %v", command, err)
	}
	result, err := providerAdapter.Capabilities(context.Background(), adapter.CapabilityContext{Request: fixture.Request})
	if err != nil || !reflect.DeepEqual(result.Capabilities, fixture.Expected.Capabilities) {
		t.Fatalf("Capabilities() = %#v, %v", result.Capabilities, err)
	}
}

func TestResponseAdapterBuildCommandPreservesOrderedImagesAndExecutionControls(t *testing.T) {
	workspace := t.TempDir()
	first := filepath.Join(workspace, "first.png")
	second := filepath.Join(workspace, "second.png")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("png"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	request := workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-command"},
		ModelProvider: string(modelprovider.Codex), Model: "gpt-test",
		UserMessage: "private prompt", WorkingDirectory: workspace,
		EnvVars: map[string]string{"CUSTOM_CODEX_ENV": "set", "GIT_EDITOR": "vim"},
		InputTokens: []any{factorytoken.Token{ID: "token-images", Color: factorytoken.Color{Content: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeImage, File: first},
			{Type: work.WorkContentPartTypeImage, File: second},
		}}}},
	}
	built, err := codex.NewResponseAdapter().BuildCommand(context.Background(), adapter.CommandContext{Request: request, SkipPermissions: true})
	if err != nil {
		t.Fatal(err)
	}
	if built.Cleanup != nil {
		defer built.Cleanup()
	}
	wantArgs := []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "--model", "gpt-test", "-i", first, "-i", second, "-"}
	if !reflect.DeepEqual(built.Request.Args, wantArgs) || string(built.Request.Stdin) != request.UserMessage || built.Request.WorkDir != workspace {
		t.Fatalf("command = %#v, want args %#v", built.Request, wantArgs)
	}
	assertEnvironmentValue(t, built.Request.Env, "CUSTOM_CODEX_ENV", "set")
	assertEnvironmentValue(t, built.Request.Env, "GIT_EDITOR", "true")
}

func assertEnvironmentValue(t *testing.T, environment []string, name, want string) {
	t.Helper()
	prefix := name + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			if got := strings.TrimPrefix(entry, prefix); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
			return
		}
	}
	t.Fatalf("environment does not contain %s", name)
}

func verifyAdapterDrafts(t *testing.T, fixture responseAdapterFixture) {
	t.Helper()
	drafts, diagnostics := decodeAdapter(t, fixture.NewAdapter(), fixture.Content, adapter.FlushReasonCompleted)
	assertSafeAdapterDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
	if len(drafts) != len(fixture.Expected.Drafts) {
		t.Fatalf("draft count = %d, want %d", len(drafts), len(fixture.Expected.Drafts))
	}
	for index, expected := range fixture.Expected.Drafts {
		got := drafts[index]
		if got.Kind != expected.Kind || got.Phase != expected.Phase || got.ItemID != expected.ItemID || got.ProviderSessionRef != expected.ProviderRef {
			t.Fatalf("draft[%d] = %#v, want %#v", index, got, expected)
		}
	}
}

func verifyAdapterFinal(t *testing.T, fixture responseAdapterFixture) {
	t.Helper()
	result, err := fixture.NewAdapter().ParseFinal(context.Background(), adapter.FinalParseContext{CommandResult: fixture.FinalResult})
	if err != nil || result.Response.Content != fixture.Expected.FinalContent || result.Response.ProviderSession == nil || *result.Response.ProviderSession != fixture.Expected.ProviderSession || len(result.Drafts) != 0 {
		t.Fatalf("ParseFinal() = %#v, %v", result, err)
	}
}

func verifyAdapterRecovery(t *testing.T, fixture responseAdapterFixture) {
	t.Helper()
	drafts, diagnostics := decodeAdapter(t, fixture.NewAdapter(), fixture.UnsafeAndRecovering, adapter.FlushReasonCompleted)
	assertSafeAdapterDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
	if len(diagnostics) == 0 || completedMessage(drafts) == nil {
		t.Fatalf("recovery drafts/diagnostics = %#v / %#v", drafts, diagnostics)
	}
}

func verifyAdapterFlush(t *testing.T, fixture responseAdapterFixture) {
	t.Helper()
	drafts, diagnostics := decodeAdapter(t, fixture.NewAdapter(), fixture.UnterminatedFinal, adapter.FlushReasonCompleted)
	assertSafeAdapterDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
	message := completedMessage(drafts)
	if message == nil || messagePayloadText(t, *message) != fixture.Expected.FinalContent {
		t.Fatalf("unterminated final drafts = %#v", drafts)
	}
}

func verifyAdapterFailure(t *testing.T, fixture responseAdapterFixture) {
	t.Helper()
	providerAdapter := fixture.NewAdapter()
	stdout := observationsStdout(fixture.TerminalFailure)
	_, parseErr := providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{CommandResult: workerprocess.CommandResult{Stdout: stdout}})
	result := providerAdapter.ClassifyFailure(context.Background(), adapter.FailureContext{CommandResult: workerprocess.CommandResult{Stdout: stdout}, ParseError: parseErr})
	failure := result.Failure
	if parseErr == nil || failure == nil || failure.Family != fixture.Expected.FailureFamily || failure.Type != fixture.Expected.FailureType || failure.Retry.Retryable != fixture.Expected.FailureRetryable {
		t.Fatalf("ClassifyFailure() = %#v, parse error %v", result, parseErr)
	}
	if failure.ProviderSession == nil || *failure.ProviderSession != fixture.Expected.ProviderSession {
		t.Fatalf("failure provider session = %#v", failure.ProviderSession)
	}
	assertSafeAdapterText(t, failure.Message, fixture.ForbiddenDiagnostic)
}

func decodeAdapter(t *testing.T, providerAdapter adapter.Adapter, observations []adapter.Observation, reason adapter.FlushReason) ([]responseevents.Draft, []adapter.Diagnostic) {
	t.Helper()
	decoder, err := providerAdapter.NewDecoder(context.Background(), adapter.DecoderContext{RunID: "run-conformance", DispatchID: "dispatch-conformance"})
	if err != nil {
		t.Fatal(err)
	}
	var drafts []responseevents.Draft
	var diagnostics []adapter.Diagnostic
	for _, observation := range observations {
		result, observeErr := decoder.Observe(context.Background(), observation)
		if observeErr != nil {
			t.Fatal(observeErr)
		}
		drafts = append(drafts, result.Drafts...)
		diagnostics = append(diagnostics, result.Diagnostics...)
	}
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: reason})
	if err != nil {
		t.Fatal(err)
	}
	return append(drafts, flushed.Drafts...), append(diagnostics, flushed.Diagnostics...)
}

func assertSafeAdapterDiagnostics(t *testing.T, diagnostics []adapter.Diagnostic, forbidden []string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "" || diagnostic.Message == "" || len(diagnostic.Message) > 160 {
			t.Fatalf("invalid diagnostic = %#v", diagnostic)
		}
		assertSafeAdapterText(t, diagnostic.Code+diagnostic.Message, forbidden)
	}
}

func assertSafeAdapterText(t *testing.T, value string, forbidden []string) {
	t.Helper()
	for _, secret := range forbidden {
		if strings.Contains(value, secret) {
			t.Fatalf("unsafe adapter text %q contains %q", value, secret)
		}
	}
}

func completedMessage(drafts []responseevents.Draft) *responseevents.Draft {
	for index := range drafts {
		if drafts[index].Kind == responseevents.KindMessage && drafts[index].Phase == responseevents.PhaseCompleted {
			return &drafts[index]
		}
	}
	return nil
}

func messagePayloadText(t *testing.T, draft responseevents.Draft) string {
	t.Helper()
	var payload responseevents.MessagePayload
	decodePayload(t, draft, &payload)
	if len(payload.ContentBlocks) != 1 {
		t.Fatalf("message payload = %#v", payload)
	}
	return payload.ContentBlocks[0].Text
}

func TestDecoderBoundsOversizedRecordsAndContinuesWithUnterminatedCompletion(t *testing.T) {
	const prompt = "private prompt must not escape"
	oversized := `{"type":"future.record","payload":"` + strings.Repeat("x", 1024*1024+64) + prompt + `"}`
	stream := strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-recovery"}`,
		`{"type":` + prompt + `}`,
		`{"type":"future.event","prompt":"` + prompt + `"}`,
		`{"type":"item.completed","item":{"id":"future-1","type":"future_item","text":"` + prompt + `"}}`,
		oversized,
	}, "\n") + "\n" + `{"type":"item.completed","item":{"id":"message-recovery","type":"agent_message","text":"recovered"}}`
	decoder := codex.NewDecoder(adapter.DecoderContext{DispatchID: "dispatch-recovery"})
	decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: []byte(stream)})
	if err != nil {
		t.Fatal(err)
	}
	assertRecoveryDiagnostics(t, decoded.Diagnostics, prompt)
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: adapter.FlushReasonCompleted})
	assertRecoveredFlush(t, flushed, err)

	parsed, err := codex.ParseFinalOutput([]byte(stream))
	assertRecoveredFinal(t, parsed, err)
	if _, err := codex.ParseFinalOutput([]byte(oversized)); err == nil || strings.Contains(err.Error(), prompt) {
		t.Fatalf("missing-completion error = %v", err)
	}
}

func assertRecoveryDiagnostics(t *testing.T, diagnostics []adapter.Diagnostic, forbidden string) {
	t.Helper()
	if len(diagnostics) != 4 {
		t.Fatalf("diagnostics = %#v, want malformed, unknown event, unknown item, and oversized record", diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if len(diagnostic.Message) > 160 || strings.Contains(diagnostic.Message, forbidden) || strings.Contains(diagnostic.Message, strings.Repeat("x", 32)) {
			t.Fatalf("unsafe diagnostic = %#v", diagnostic)
		}
	}
}

func assertRecoveredFlush(t *testing.T, flushed adapter.DecodeResult, err error) {
	t.Helper()
	if err != nil || len(flushed.Drafts) != 1 || flushed.Drafts[0].ItemID != "message-recovery" {
		t.Fatalf("Flush() = %#v, %v", flushed, err)
	}
}

func assertRecoveredFinal(t *testing.T, parsed codex.FinalResult, err error) {
	t.Helper()
	if err != nil || parsed.Content != "recovered" || parsed.ProviderSession == nil || parsed.ProviderSession.ID != "thread-recovery" {
		t.Fatalf("ParseFinalOutput() = %#v, %v", parsed, err)
	}
}

func TestDecoderIgnoresAdditiveFieldsAcrossKnownEventAndItemFixtures(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "supported_items.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	fixture = append(fixture, []byte("{\"type\":\"turn.failed\",\"error\":{\"message\":\"unexpected status 429\"}}\n{\"type\":\"error\",\"message\":\"unexpected status 500\"}\n")...)
	augmented := addUnknownFields(t, fixture)

	want := decodeFixture(t, fixture)
	got := decodeFixture(t, augmented)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("additive fields changed decoded drafts\ngot:  %#v\nwant: %#v", got, want)
	}
}

func decodeFixture(t *testing.T, fixture []byte) []responseevents.Draft {
	t.Helper()
	decoder := codex.NewDecoder(adapter.DecoderContext{RunID: "run-additive", DispatchID: "dispatch-additive"})
	decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: fixture})
	if err != nil {
		t.Fatal(err)
	}
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: adapter.FlushReasonCompleted})
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Diagnostics)+len(flushed.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v %#v", decoded.Diagnostics, flushed.Diagnostics)
	}
	return append(decoded.Drafts, flushed.Drafts...)
}

func addUnknownFields(t *testing.T, fixture []byte) []byte {
	t.Helper()
	var output []byte
	for _, line := range strings.Split(strings.TrimSpace(string(fixture)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		record["future_outer"] = map[string]any{"ignored": true}
		if item, ok := record["item"].(map[string]any); ok {
			item["future_nested"] = []any{"ignored", 42}
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		output = append(output, encoded...)
		output = append(output, '\n')
	}
	return output
}

func codexObservations(records ...string) []adapter.Observation {
	stream := strings.Join(records, "\n") + "\n"
	cut := len(stream) / 2
	return []adapter.Observation{
		{Stream: adapter.OutputStreamStdout, Chunk: []byte(stream[:cut])},
		{Stream: adapter.OutputStreamStdout, Chunk: []byte(stream[cut:])},
	}
}

func observationsStdout(observations []adapter.Observation) []byte {
	var output []byte
	for _, observation := range observations {
		if observation.Stream == adapter.OutputStreamStdout {
			output = append(output, observation.Chunk...)
		}
	}
	return output
}

func draftsByItemID(drafts []responseevents.Draft) map[string][]responseevents.Draft {
	result := make(map[string][]responseevents.Draft)
	for _, draft := range drafts {
		if draft.ItemID != "" {
			result[draft.ItemID] = append(result[draft.ItemID], draft)
		}
	}
	return result
}

func itemIDOrder(drafts []responseevents.Draft) []string {
	var result []string
	for _, draft := range drafts {
		if draft.ItemID != "" {
			result = append(result, draft.ItemID)
		}
	}
	return result
}

func assertItemLifecycle(t *testing.T, drafts []responseevents.Draft, kind responseevents.Kind, phases ...responseevents.Phase) {
	t.Helper()
	got := make([]responseevents.Phase, len(drafts))
	completedSnapshots := 0
	for index, draft := range drafts {
		if draft.Kind != kind {
			t.Fatalf("item %q draft[%d] kind = %q, want %q", draft.ItemID, index, draft.Kind, kind)
		}
		got[index] = draft.Phase
		if draft.Provenance.NativeEventType == "item.completed" {
			completedSnapshots++
			if draft.Provenance.Representation != responseevents.RepresentationSnapshot {
				t.Fatalf("item %q completion representation = %q", draft.ItemID, draft.Provenance.Representation)
			}
		}
	}
	if !reflect.DeepEqual(got, phases) {
		t.Fatalf("phases = %#v, want %#v", got, phases)
	}
	if completedSnapshots != 1 {
		t.Fatalf("completed snapshots = %d, want 1", completedSnapshots)
	}
}

func assertItemPayloads(t *testing.T, byID map[string][]responseevents.Draft) {
	t.Helper()
	assertReasoningCommandAndFilePayloads(t, byID)
	assertToolAndPlanPayloads(t, byID)
	assertErrorItemPayload(t, byID["error-1"])
}

func assertErrorItemPayload(t *testing.T, drafts []responseevents.Draft) {
	t.Helper()
	if got, want := []string{drafts[0].Provenance.NativeEventType, drafts[1].Provenance.NativeEventType, drafts[2].Provenance.NativeEventType}, []string{"item.started", "item.updated", "item.completed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("error item provenance = %#v, want %#v", got, want)
	}
	var payload responseevents.ErrorPayload
	decodePayload(t, drafts[2], &payload)
	if payload.Code != "codex_item_error" || payload.Message != "The recoverable error was recorded." || payload.Retryable {
		t.Fatalf("error item payload = %#v", payload)
	}
}

func assertReasoningCommandAndFilePayloads(t *testing.T, byID map[string][]responseevents.Draft) {
	t.Helper()
	var reasoning responseevents.ReasoningPayload
	decodePayload(t, byID["reason-1"][1], &reasoning)
	if reasoning.Summary != "The provider adapter owns native decoding." {
		t.Fatalf("reasoning = %#v", reasoning)
	}

	var command responseevents.ToolPayload
	decodePayload(t, byID["command-1"][1], &command)
	if command.ToolCallID != "command-1" || command.ToolName != "command_execution" || !strings.Contains(string(command.ResultSummary), `"exitCode":0`) {
		t.Fatalf("command = %#v", command)
	}

	var files responseevents.FileChangePayload
	decodePayload(t, byID["files-1"][0], &files)
	if files.Path != "pkg/workers/provider/codex/items.go" || files.Operation != "update" || !strings.Contains(files.Summary, "adapter_test.go") {
		t.Fatalf("file change = %#v", files)
	}
}

func assertToolAndPlanPayloads(t *testing.T, byID map[string][]responseevents.Draft) {
	t.Helper()
	var mcp responseevents.ToolPayload
	decodePayload(t, byID["mcp-1"][1], &mcp)
	if mcp.ToolName != "mcp:docs/lookup" || len(mcp.ArgumentsSummary) != 0 || strings.Contains(string(mcp.ResultSummary), "not projected") {
		t.Fatalf("mcp = %#v", mcp)
	}

	var collaboration responseevents.ToolPayload
	decodePayload(t, byID["collab-1"][1], &collaboration)
	if collaboration.ToolName != "collaboration:spawn_agent" || strings.Contains(string(collaboration.ResultSummary), "not projected") {
		t.Fatalf("collaboration = %#v", collaboration)
	}

	var plan responseevents.PlanPayload
	decodePayload(t, byID["plan-1"][2], &plan)
	if len(plan.Steps) != 2 || plan.Steps[0].ID != "" || plan.Steps[0].Status != "completed" || plan.Steps[1].Status != "completed" {
		t.Fatalf("plan = %#v", plan)
	}
}

func decodePayload(t *testing.T, draft responseevents.Draft, target any) {
	t.Helper()
	if err := json.Unmarshal(draft.Payload, target); err != nil {
		t.Fatalf("decode %q payload: %v", draft.ItemID, err)
	}
}

func mustMarshalString(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
