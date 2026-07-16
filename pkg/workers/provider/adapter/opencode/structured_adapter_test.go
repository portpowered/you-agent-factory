package opencode_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/work"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter/opencode"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter/testkit"
)

const (
	privatePrompt = "private prompt"
	privateSecret = "sk-opencode-secret"
)

func TestStructuredAdapterSharedConformance(t *testing.T) {
	session := workerexecution.ProviderSessionMetadata{Provider: "opencode", Kind: "session_id", ID: "session-42"}
	testkit.RunFullStream(t, testkit.FullStreamFixture{
		NewAdapter: func() adapter.Adapter { return newStructuredAdapter(t) },
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{DispatchID: "dispatch-conformance"},
			Model:    "openai/gpt-5", UserMessage: privatePrompt,
		},
		ContentAndTools: openCodeObservations(
			`{"type":"step_start","sessionID":"session-42"}`,
			`{"type":"text","sessionID":"session-42","part":{"id":"message-7","text":"Hello world","time":{"end":1}}}`,
			`{"type":"tool_use","sessionID":"session-42","part":{"id":"tool-item-9","callID":"call-9","tool":"weather","state":{"status":"completed"}}}`,
		),
		RetryableFailure: openCodeObservations(
			`{"type":"error","sessionID":"session-42","error":{"name":"RateLimitError","data":{"status":429}}}`,
		),
		UnsafeAndRecovering: openCodeObservations(
			`{"type":"step_start","sessionID":"session-42"}`,
			`{"type":`+privatePrompt+`,"token":"`+privateSecret+`"}`,
			`{"type":"future_shape","prompt":"`+privatePrompt+`","token":"`+privateSecret+`"}`,
			`{"type":"text","sessionID":"session-42","part":{"id":"message-7","text":"Hello world","time":{"end":1}}}`,
		),
		UnterminatedFinal: []adapter.Observation{
			{Stream: adapter.OutputStreamStdout, Chunk: []byte("{\"type\":\"step_start\",\"sessionID\":\"session-42\"}\n")},
			{Stream: adapter.OutputStreamStdout, Chunk: []byte(`{"type":"text","sessionID":"session-42","part":{"id":"message-7","text":"Hello world","time":{"end":1}}}`)},
		},
		FinalResult: workerprocess.CommandResult{Stdout: []byte(`{"type":"text","sessionID":"session-42","part":{"id":"message-7","text":"Hello world","time":{"end":1}}}`)},
		Expected: testkit.FullStreamExpected{
			Capabilities:    adapter.Capabilities{NativeStreaming: true, MessageSnapshots: true, StableItemIDs: true},
			ProviderSession: session, ProviderRef: "session-42", MessageItemID: "message-7",
			ToolItemID: "tool-item-9", ToolCallID: "call-9",
			FinalContent: "Hello world",
		},
		ForbiddenDiagnostic: []string{privatePrompt, privateSecret},
	})
}

func openCodeObservations(records ...string) []adapter.Observation {
	observations := make([]adapter.Observation, 0, len(records))
	for _, record := range records {
		observations = append(observations, adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: []byte(record + "\n")})
	}
	return observations
}

func TestStructuredAdapterExecutesNegotiatedJSONModeAndEmitsCanonicalLifecycle(t *testing.T) {
	fixture := readFixture(t, "testdata/structured-success.jsonl")
	providerAdapter := newStructuredAdapter(t)
	registry, err := adapter.NewRegistry(providerAdapter)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runner := &fixtureStreamingRunner{stdout: fixture, chunks: splitFixtureChunks(fixture)}
	request := workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-open-42", TransitionID: "transition-open-42"},
		Model:    "openai/gpt-5", OpenCodeAgent: "implementer", SessionID: "ses_open_42",
		WorkingDirectory: "/workspace", UserMessage: privatePrompt,
	}

	result, err := adapter.Execute(context.Background(), registry, runner, adapter.ExecuteInput{
		Provider: "opencode", Command: adapter.CommandContext{Request: request, SkipPermissions: true},
		Decoder: adapter.DecoderContext{RunID: "run-open-42", DispatchID: "dispatch-open-42"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantArgs := []string{
		"run", "--format", "json", "--model", "openai/gpt-5", "--agent", "implementer",
		"--session", "ses_open_42", "--dir", "/workspace", "--dangerously-skip-permissions", privatePrompt,
	}
	if runner.request.Command != "/resolved/opencode" || !reflect.DeepEqual(runner.request.Args, wantArgs) {
		t.Fatalf("command = %q %#v, want resolved structured command %#v", runner.request.Command, runner.request.Args, wantArgs)
	}
	wantCapabilities := adapter.Capabilities{NativeStreaming: true, MessageSnapshots: true, StableItemIDs: true}
	if result.Capabilities != wantCapabilities {
		t.Fatalf("capabilities = %#v, want %#v", result.Capabilities, wantCapabilities)
	}
	if result.Response.Content != "Hello world" || result.Response.ProviderSession == nil ||
		result.Response.ProviderSession.ID != "ses_open_42" {
		t.Fatalf("response = %#v", result.Response)
	}
	if result.Failure != nil || len(result.Diagnostics) != 0 {
		t.Fatalf("failure = %#v, diagnostics = %#v", result.Failure, result.Diagnostics)
	}
	assertStructuredDrafts(t, result.Drafts)
}

func TestStructuredDecoderBoundsUnsafeDiagnosticsAndRecovers(t *testing.T) {
	decoder, err := newStructuredAdapter(t).NewDecoder(context.Background(), adapter.DecoderContext{
		RunID: "run-safe", DispatchID: "dispatch-safe",
	})
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	unsafe := []byte(
		`{"type":` + privatePrompt + `,"token":"` + privateSecret + `"}` + "\n" +
			`{"type":"future_shape","prompt":"` + privatePrompt + `","token":"` + privateSecret + `"}` + "\n" +
			`{"type":"text","sessionID":"ses_safe","part":{"id":"part_safe","text":"recovered","time":{"end":1}}}` + "\n",
	)
	decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: unsafe})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if len(decoded.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want malformed and unknown diagnostics", decoded.Diagnostics)
	}
	for _, diagnostic := range decoded.Diagnostics {
		if len(diagnostic.Message) > 160 || strings.Contains(diagnostic.Message, privatePrompt) || strings.Contains(diagnostic.Message, privateSecret) {
			t.Fatalf("unsafe diagnostic = %#v", diagnostic)
		}
	}
	if findDraft(decoded.Drafts, responseevents.KindMessage, responseevents.PhaseCompleted) == nil {
		t.Fatalf("later valid record was not decoded: %#v", decoded.Drafts)
	}
}

func TestStructuredDecoderFlushesUnterminatedSnapshotAndTerminal(t *testing.T) {
	decoder, err := newStructuredAdapter(t).NewDecoder(context.Background(), adapter.DecoderContext{
		RunID: "run-flush", DispatchID: "dispatch-flush",
	})
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	record := []byte(`{"type":"text","sessionID":"ses_flush","part":{"id":"part_flush","text":"final snapshot","time":{"end":1}}}`)
	decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: record})
	if err != nil || len(decoded.Drafts) != 0 {
		t.Fatalf("Observe() = %#v, %v; want buffered record", decoded, err)
	}
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: adapter.FlushReasonCompleted})
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if len(flushed.Drafts) != 3 {
		t.Fatalf("flush drafts = %#v, want run start, snapshot, terminal", flushed.Drafts)
	}
	message := findDraft(flushed.Drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
	terminal := findDraft(flushed.Drafts, responseevents.KindRun, responseevents.PhaseCompleted)
	if message == nil || terminal == nil || message.ProviderSessionRef != "ses_flush" {
		t.Fatalf("flush drafts = %#v", flushed.Drafts)
	}
}

func TestStructuredFinalUsesSnapshotWithoutDuplicatingPrecedingDeltas(t *testing.T) {
	stdout := []byte(
		`{"type":"text","sessionID":"ses_snapshot","part":{"id":"part_snapshot","text":"Hello "}}` + "\n" +
			`{"type":"text","sessionID":"ses_snapshot","part":{"id":"part_snapshot","text":"Hello world","time":{"end":1}}}`,
	)
	result, err := newStructuredAdapter(t).ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{Stdout: stdout}, FlushReason: adapter.FlushReasonCompleted,
	})
	if err != nil {
		t.Fatalf("ParseFinal() error = %v", err)
	}
	if result.Response.Content != "Hello world" || len(result.Drafts) != 0 {
		t.Fatalf("final result = %#v, want one authoritative snapshot without semantic drafts", result)
	}
}

func TestOpenCodeAuthoritativeFinalResultsPreserveContentBeyondPublicationLimit(t *testing.T) {
	// Cross both the 256 KiB event publication limit and the structured
	// decoder's 1 MiB observation-record limit. Authoritative parsing must not
	// inherit either limit.
	content := strings.Repeat("x", 2*1024*1024) + "complete-tail"
	tests := []struct {
		name            string
		providerAdapter adapter.Adapter
		stdout          []byte
		wantDraft       bool
	}{
		{
			name:            "structured",
			providerAdapter: newStructuredAdapter(t),
			stdout: []byte(fmt.Sprintf(
				`{"type":"text","sessionID":"ses_large","part":{"id":"part_large","text":%q,"time":{"end":1}}}`,
				content,
			)),
		},
		{
			name: "final only",
			providerAdapter: mustNegotiatedAdapter(t, opencode.Decision{
				Installation: installation(), Version: "0.9.0", Mode: opencode.ModeFinalOnly,
			}, nil),
			stdout:    []byte(content),
			wantDraft: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{
				CommandResult: workerprocess.CommandResult{Stdout: tc.stdout}, FlushReason: adapter.FlushReasonCompleted,
			})
			if err != nil {
				t.Fatalf("ParseFinal() error = %v", err)
			}
			if result.Response.Content != content {
				t.Fatalf("authoritative content length = %d, want %d with complete tail", len(result.Response.Content), len(content))
			}
			message := findDraft(result.Drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
			if tc.wantDraft {
				assertPublishedMessageIsBounded(t, message, content)
			} else if message != nil {
				t.Fatalf("structured final parser emitted duplicate message draft: %#v", message)
			}
		})
	}
}

func TestStructuredFinalCombinesDistinctCompletedTextPartsInProviderOrder(t *testing.T) {
	stdout := []byte(
		`{"type":"text","sessionID":"ses_parts","part":{"id":"part_before","text":"before ","time":{"end":1}}}` + "\n" +
			`{"type":"tool_use","sessionID":"ses_parts","part":{"id":"tool_middle","callID":"call_middle","tool":"lookup","state":{"status":"completed"}}}` + "\n" +
			`{"type":"text","sessionID":"ses_parts","part":{"id":"part_after","text":"after","time":{"end":2}}}` + "\n" +
			`{"type":"text","sessionID":"ses_parts","part":{"id":"part_after","text":"after","time":{"end":2}}}`,
	)
	result, err := newStructuredAdapter(t).ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{Stdout: stdout}, FlushReason: adapter.FlushReasonCompleted,
	})
	if err != nil {
		t.Fatalf("ParseFinal() error = %v", err)
	}
	if result.Response.Content != "before after" {
		t.Fatalf("authoritative content = %q, want provider-ordered distinct text parts", result.Response.Content)
	}
}

func assertPublishedMessageIsBounded(t *testing.T, draft *responseevents.Draft, authoritative string) {
	t.Helper()
	if draft == nil {
		t.Fatal("final-only result did not publish an authoritative message snapshot")
	}
	var payload responseevents.MessagePayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("decode message payload: %v", err)
	}
	if len(payload.ContentBlocks) != 1 {
		t.Fatalf("message payload = %#v", payload)
	}
	published := payload.ContentBlocks[0].Text
	if len(published) >= len(authoritative) || !strings.HasPrefix(authoritative, published) || strings.Contains(published, "complete-tail") {
		t.Fatalf("published snapshot length = %d, authoritative length = %d", len(published), len(authoritative))
	}
}

func TestStructuredExplicitFailureWinsOverZeroExitAndNeverLeaksNativeBody(t *testing.T) {
	stdout := readFixture(t, "testdata/structured-error.jsonl")
	providerAdapter := newStructuredAdapter(t)
	_, parseErr := providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{Stdout: stdout, ExitCode: 0},
		FlushReason:   adapter.FlushReasonCompleted,
	})
	if parseErr == nil {
		t.Fatal("ParseFinal() error = nil, want explicit structured failure")
	}
	assertSafeFailureText(t, parseErr.Error())
	classified := providerAdapter.ClassifyFailure(context.Background(), adapter.FailureContext{
		CommandResult: workerprocess.CommandResult{Stdout: stdout, ExitCode: 0},
		ParseError:    parseErr, FlushReason: adapter.FlushReasonCompleted,
	})
	assertAuthenticationFailure(t, classified)

	decoder, err := providerAdapter.NewDecoder(context.Background(), adapter.DecoderContext{})
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: stdout})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	errorDraft := findDraft(decoded.Drafts, responseevents.KindError, responseevents.PhaseFailed)
	if errorDraft == nil {
		t.Fatal("explicit failure did not emit an error draft")
	}
	if strings.Contains(string(errorDraft.Payload), privatePrompt) || strings.Contains(string(errorDraft.Payload), privateSecret) {
		t.Fatalf("unsafe or missing error draft = %#v", errorDraft)
	}
}

func assertSafeFailureText(t *testing.T, value string) {
	t.Helper()
	for _, forbidden := range []string{privatePrompt, privateSecret, "responseBody"} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("unsafe failure text = %q", value)
		}
	}
}

func assertAuthenticationFailure(t *testing.T, classified adapter.FailureResult) {
	t.Helper()
	failure := classified.Failure
	if failure == nil {
		t.Fatal("ClassifyFailure() returned no failure")
	}
	if failure.Type != workerexecution.WorkFailureTypeAuthFailure || failure.Family != workerexecution.WorkFailureFamilyTerminal || failure.Retry.Retryable {
		t.Fatalf("ClassifyFailure() = %#v", classified)
	}
	if failure.ProviderSession == nil || failure.ProviderSession.ID != "ses_error_42" {
		t.Fatalf("failure provider session = %#v", failure.ProviderSession)
	}
}

func TestStructuredFinalRejectsMissingAuthoritativeResponse(t *testing.T) {
	_, err := newStructuredAdapter(t).ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{Stdout: []byte(`{"type":"step_finish","sessionID":"ses_empty","part":{"tokens":{}}}`)},
		FlushReason:   adapter.FlushReasonCompleted,
	})
	if err == nil || strings.Contains(err.Error(), "ses_empty") {
		t.Fatalf("ParseFinal() error = %v, want bounded missing-response failure", err)
	}
	classified := newStructuredAdapter(t).ClassifyFailure(context.Background(), adapter.FailureContext{ParseError: err})
	if classified.Failure == nil || classified.Failure.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("ClassifyFailure() = %#v", classified)
	}
}

func TestOpenCodeTerminalClassificationCoversSupportedStatusAndNameSignals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		nativeName  string
		status      int
		failureType workerexecution.WorkFailureType
	}{
		{name: "bad request status", nativeName: "APIError", status: 400, failureType: workerexecution.WorkFailureTypePermanentBadRequest},
		{name: "throttle status", nativeName: "APIError", status: 429, failureType: workerexecution.WorkFailureTypeThrottled},
		{name: "timeout status", nativeName: "APIError", status: 504, failureType: workerexecution.WorkFailureTypeTimeout},
		{name: "server status", nativeName: "APIError", status: 503, failureType: workerexecution.WorkFailureTypeInternalServerError},
		{name: "auth name", nativeName: "UnauthorizedError", failureType: workerexecution.WorkFailureTypeAuthFailure},
		{name: "invalid name", nativeName: "InvalidRequest", failureType: workerexecution.WorkFailureTypePermanentBadRequest},
		{name: "capacity name", nativeName: "CapacityError", failureType: workerexecution.WorkFailureTypeThrottled},
		{name: "deadline name", nativeName: "DeadlineExceeded", failureType: workerexecution.WorkFailureTypeTimeout},
		{name: "server name", nativeName: "ServerError", failureType: workerexecution.WorkFailureTypeInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout := []byte(fmt.Sprintf(`{"type":"error","error":{"name":%q,"data":{"statusCode":%d}}}`, tc.nativeName, tc.status))
			providerAdapter := newStructuredAdapter(t)
			_, parseErr := providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{CommandResult: workerprocess.CommandResult{Stdout: stdout}})
			if parseErr == nil {
				t.Fatal("ParseFinal() error = nil")
			}
			classified := providerAdapter.ClassifyFailure(context.Background(), adapter.FailureContext{CommandResult: workerprocess.CommandResult{Stdout: stdout}, ParseError: parseErr})
			if classified.Failure == nil || classified.Failure.Type != tc.failureType {
				t.Fatalf("ClassifyFailure() = %#v, want %s", classified, tc.failureType)
			}
		})
	}
}

func TestNegotiatedAdapterRejectsInvalidConstruction(t *testing.T) {
	t.Parallel()
	tests := []opencode.Decision{
		{Installation: installation(), Version: "1.0.0", Mode: opencode.Mode("invalid")},
		{Version: "1.0.0", Mode: opencode.ModeFinalOnly},
	}
	for _, decision := range tests {
		if _, err := opencode.NewNegotiatedAdapter(decision, nil); err == nil {
			t.Fatalf("NewNegotiatedAdapter(%#v) error = nil", decision)
		}
	}
}

func newStructuredAdapter(t *testing.T) *opencode.StructuredAdapter {
	t.Helper()
	providerAdapter, err := opencode.NewStructuredAdapter(opencode.Decision{
		Installation: opencode.Installation{Executable: "/resolved/opencode", Fingerprint: "fixture"},
		Version:      "1.2.3", Mode: opencode.ModeStructured,
	})
	if err != nil {
		t.Fatalf("NewStructuredAdapter() error = %v", err)
	}
	return providerAdapter
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return contents
}

func splitFixtureChunks(fixture []byte) []adapter.Observation {
	middle := len(fixture) / 2
	return []adapter.Observation{
		{Stream: adapter.OutputStreamStdout, Chunk: fixture[:7]},
		{Stream: adapter.OutputStreamStdout, Chunk: fixture[7:middle]},
		{Stream: adapter.OutputStreamStdout, Chunk: fixture[middle:]},
	}
}

func assertStructuredDrafts(t *testing.T, drafts []responseevents.Draft) {
	t.Helper()
	want := []struct {
		kind  responseevents.Kind
		phase responseevents.Phase
	}{
		{responseevents.KindRun, responseevents.PhaseStarted},
		{responseevents.KindMessage, responseevents.PhaseCompleted},
		{responseevents.KindTool, responseevents.PhaseCompleted},
		{responseevents.KindUsage, responseevents.PhaseUpdated},
		{responseevents.KindRun, responseevents.PhaseCompleted},
	}
	if len(drafts) != len(want) {
		t.Fatalf("drafts = %#v, want %d ordered drafts", drafts, len(want))
	}
	for index, draft := range drafts {
		assertStructuredDraft(t, index, draft, want[index].kind, want[index].phase, index == len(drafts)-1)
	}
	if drafts[1].ItemID != "part_text_1" || drafts[1].ParentItemID != "msg_1" || drafts[1].ProviderSessionRef != "ses_open_42" {
		t.Fatalf("message correlation = %#v", drafts[1])
	}
	if drafts[2].ItemID != "part_tool_1" || drafts[2].ParentItemID != "msg_1" {
		t.Fatalf("tool correlation = %#v", drafts[2])
	}
	if strings.Contains(string(drafts[2].Payload), "PRIVATE.md") || strings.Contains(string(drafts[2].Payload), "private result") {
		t.Fatalf("tool payload exposed native input or output: %s", drafts[2].Payload)
	}
}

func assertStructuredDraft(t *testing.T, index int, draft responseevents.Draft, kind responseevents.Kind, phase responseevents.Phase, terminal bool) {
	t.Helper()
	if err := responseevents.ValidateDraft(draft); err != nil {
		t.Fatalf("draft[%d] is invalid: %v", index, err)
	}
	if draft.Kind != kind || draft.Phase != phase {
		t.Fatalf("draft[%d] = %s/%s, want %s/%s", index, draft.Kind, draft.Phase, kind, phase)
	}
	if draft.RunID != "run-open-42" || draft.DispatchID != "dispatch-open-42" {
		t.Fatalf("draft[%d] correlation = %#v", index, draft)
	}
	if draft.Provenance.Provider != "opencode" {
		t.Fatalf("draft[%d] provenance = %#v", index, draft.Provenance)
	}
	if terminal {
		assertTerminalProvenance(t, draft.Provenance)
		return
	}
	if draft.Provenance.Fidelity != responseevents.FidelityNormalized {
		t.Fatalf("draft[%d] provenance = %#v", index, draft.Provenance)
	}
}

func assertTerminalProvenance(t *testing.T, provenance responseevents.Provenance) {
	t.Helper()
	if provenance.Delivery != responseevents.DeliverySynthesized || provenance.Fidelity != responseevents.FidelityLifecycleOnly {
		t.Fatalf("terminal provenance = %#v", provenance)
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

type fixtureStreamingRunner struct {
	stdout  []byte
	chunks  []adapter.Observation
	request workerprocess.CommandRequest
}

func (r *fixtureStreamingRunner) Run(ctx context.Context, request workerprocess.CommandRequest, observe func(adapter.Observation) error) (workerprocess.CommandResult, error) {
	r.request = request
	for _, chunk := range r.chunks {
		if err := ctx.Err(); err != nil {
			return workerprocess.CommandResult{}, err
		}
		if err := observe(chunk); err != nil {
			return workerprocess.CommandResult{}, err
		}
	}
	return workerprocess.CommandResult{Stdout: r.stdout}, nil
}

var _ adapter.StreamingCommandRunner = (*fixtureStreamingRunner)(nil)
