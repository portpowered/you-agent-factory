package agy_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/agy"
)

func TestAdapterParseFinalTimeoutEmitsOnePartialMessage(t *testing.T) {
	t.Parallel()

	providerAdapter := agy.NewAdapter(t.TempDir())
	result, err := providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{
			ExitCode: 124,
			Stdout:   []byte("partial answer before timeout"),
		},
		CommandError: agypty.ErrSessionTimedOut,
		FlushReason:  adapter.FlushReasonCanceled,
		RunID:        "run-partial-timeout",
		DispatchID:   "dispatch-partial-timeout",
	})
	if err == nil {
		t.Fatal("ParseFinal() error = nil, want timeout failure")
	}
	if result.Response.Content != "" {
		t.Fatalf("response content = %q, want empty on timeout", result.Response.Content)
	}
	if len(result.Drafts) != 3 {
		t.Fatalf("drafts = %#v, want partial message plus timeout error and failed run", result.Drafts)
	}
	assertPartialTimeoutMessageDraft(t, result.Drafts[0], "partial answer before timeout")
	assertTimeoutTerminalDrafts(t, result.Drafts[1:], "", false)
}

func TestAdapterExecuteTimeoutEmitsPartialMessageBeforeFailure(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &failureStubAllocator{result: agypty.SessionResult{
		ExitCode: 124, TimedOut: true, CleanedText: "partial answer before timeout",
	}}
	providerAdapter := agy.NewAdapterWithDependencies(
		factoryRoot, mock, "agy", agypty.SessionConfig{}, executableDependencies(nil),
	)
	registry, err := adapter.NewRegistry(providerAdapter)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runner, err := providerAdapter.PTYRunner()
	if err != nil {
		t.Fatalf("PTYRunner() error = %v", err)
	}
	result, executeErr := adapter.Execute(context.Background(), registry, runner, adapter.ExecuteInput{
		Provider: providerAdapter.Identity(),
		Command: adapter.CommandContext{Request: workerexecution.ProviderInferenceRequest{
			Dispatch:         work.WorkDispatch{DispatchID: "dispatch-agy-timeout"},
			WorkingDirectory: ".",
			UserMessage:      "plan the goal",
		}},
		Decoder: adapter.DecoderContext{RunID: "run-agy-timeout", DispatchID: "dispatch-agy-timeout"},
	})
	if executeErr == nil {
		t.Fatal("Execute() error = nil, want timeout failure")
	}
	if result.Response.Content != "" {
		t.Fatalf("response content = %q, want no successful final output on timeout", result.Response.Content)
	}
	if result.Failure == nil {
		t.Fatal("failure = nil, want classified timeout failure")
	}
	assertFailureFacts(t, adapter.FailureResult{Failure: result.Failure},
		workerexecution.WorkFailureTypeTimeout, "Agy request timed out.", true)

	partialDrafts := filterMessageDrafts(result.Drafts)
	if len(partialDrafts) != 1 {
		t.Fatalf("partial message drafts = %#v, want exactly one", partialDrafts)
	}
	assertPartialTimeoutMessageDraft(t, partialDrafts[0], "partial answer before timeout")
	assertTimeoutTerminalDrafts(t, result.Drafts[1:], "", false)
}

func TestAdapterParseFinalTimeoutBoundsPartialContent(t *testing.T) {
	t.Parallel()

	oversized := strings.Repeat("x", (256*1024)+64)
	providerAdapter := agy.NewAdapter(t.TempDir())
	result, err := providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{
			ExitCode: 124,
			Stdout:   []byte(oversized),
		},
		CommandError: agypty.ErrSessionTimedOut,
		FlushReason:  adapter.FlushReasonCanceled,
		RunID:        "run-bound-timeout",
		DispatchID:   "dispatch-bound-timeout",
	})
	if err == nil {
		t.Fatal("ParseFinal() error = nil, want timeout failure")
	}
	if len(result.Drafts) != 3 {
		t.Fatalf("drafts = %d, want partial message plus timeout error and failed run", len(result.Drafts))
	}
	var payload factorysessions.ResponseEventMessage
	if err := json.Unmarshal(result.Drafts[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal partial payload: %v", err)
	}
	if len(payload.ContentBlocks) != 1 {
		t.Fatalf("content blocks = %#v", payload.ContentBlocks)
	}
	got := payload.ContentBlocks[0].Text
	if len(got) > 256*1024 {
		t.Fatalf("partial text length = %d, want <= %d", len(got), 256*1024)
	}
	if !strings.HasPrefix(oversized, got) {
		t.Fatalf("partial text = %q..., want prefix of oversized capture", got[:min(32, len(got))])
	}
}

func TestAdapterParseFinalTimeoutSkipsPartialForUnusableCapture(t *testing.T) {
	t.Parallel()

	providerAdapter := agy.NewAdapter(t.TempDir())
	cases := []struct {
		name   string
		stdout []byte
	}{
		{name: "empty stdout", stdout: nil},
		{name: "whitespace only", stdout: []byte("   \n\t")},
		{name: "invalid utf8", stdout: []byte{0xff, 0xfe, 0xfd}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{
				CommandResult: workerprocess.CommandResult{ExitCode: 124, Stdout: tc.stdout},
				CommandError:  agypty.ErrSessionTimedOut,
				FlushReason:   adapter.FlushReasonCanceled,
				RunID:         "run-empty-timeout",
				DispatchID:    "dispatch-empty-timeout",
			})
			if err == nil {
				t.Fatal("ParseFinal() error = nil, want timeout failure")
			}
			if len(result.Drafts) != 2 {
				t.Fatalf("drafts = %#v, want timeout error and failed run without partial message", result.Drafts)
			}
			assertTimeoutTerminalDrafts(t, result.Drafts, "", false)
		})
	}
}

func assertPartialTimeoutMessageDraft(t *testing.T, draft factorysessions.ResponseEventDraft, wantText string) {
	t.Helper()
	if err := factorysessions.ValidateResponseEventDraft(draft); err != nil {
		t.Fatalf("partial draft invalid: %v", err)
	}
	if draft.Kind != factorysessions.ResponseEventKindMessage || draft.Phase != factorysessions.ResponseEventPhaseCompleted {
		t.Fatalf("draft = %#v, want completed MESSAGE", draft)
	}
	if draft.Provenance.NativeEventType != "timeout_partial_response" {
		t.Fatalf("native event type = %q, want timeout_partial_response", draft.Provenance.NativeEventType)
	}
	var payload factorysessions.ResponseEventMessage
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if !payload.Partial {
		t.Fatal("payload.partial = false, want true")
	}
	if payload.Role != "assistant" {
		t.Fatalf("payload.role = %q, want assistant", payload.Role)
	}
	if len(payload.ContentBlocks) != 1 || payload.ContentBlocks[0].Text != wantText {
		t.Fatalf("payload content = %#v, want %q", payload.ContentBlocks, wantText)
	}
	if agypty.ContainsTerminalEscapeOrControl(wantText) {
		t.Fatalf("wantText still contains terminal escape or control bytes: %q", wantText)
	}
}

func filterMessageDrafts(drafts []factorysessions.ResponseEventDraft) []factorysessions.ResponseEventDraft {
	filtered := make([]factorysessions.ResponseEventDraft, 0, len(drafts))
	for _, draft := range drafts {
		if draft.Kind != factorysessions.ResponseEventKindMessage {
			continue
		}
		var payload factorysessions.ResponseEventMessage
		if err := json.Unmarshal(draft.Payload, &payload); err != nil || !payload.Partial {
			continue
		}
		filtered = append(filtered, draft)
	}
	return filtered
}
