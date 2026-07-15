package agy_test

import (
	"context"
	"encoding/json"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/agy"
)

func TestPartialTimeoutMessageSnapshotIsNotAuthoritative(t *testing.T) {
	t.Parallel()

	payload := responseevents.MessagePayload{
		Role:    "assistant",
		Partial: true,
		ContentBlocks: []responseevents.ContentBlock{{
			Kind: responseevents.ContentBlockText,
			Text: "partial answer before timeout",
		}},
	}
	if responseevents.IsAuthoritativeMessageSnapshot(payload) {
		t.Fatal("partial timeout snapshot must not be authoritative")
	}

	finalPayload := responseevents.MessagePayload{
		Role: "assistant",
		ContentBlocks: []responseevents.ContentBlock{{
			Kind: responseevents.ContentBlockText,
			Text: "final answer",
		}},
	}
	if !responseevents.IsAuthoritativeMessageSnapshot(finalPayload) {
		t.Fatal("non-partial completed snapshot should remain authoritative")
	}
}

func TestAdapterParseFinalTimeoutPartialDoesNotPopulateSuccessfulInferenceResponse(t *testing.T) {
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
		t.Fatalf("response content = %q, want empty so partial capture cannot become primary result", result.Response.Content)
	}
	assertPartialTimeoutMessageDraft(t, result.Drafts[0], "partial answer before timeout")
	assertTimeoutDraftsExcludeCompletedRun(t, result.Drafts)
}

func TestAdapterExecuteTimeoutPartialDoesNotPopulateSuccessfulInferenceResponse(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &failureStubAllocator{result: agypty.SessionResult{
		ExitCode: 124, TimedOut: true, CleanedText: "partial answer before timeout",
	}}
	providerAdapter := agy.NewAdapter(factoryRoot, agy.WithAllocator(mock), agy.WithExecutable("agy"))
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
		t.Fatalf("response content = %q, want empty so partial capture cannot become primary result", result.Response.Content)
	}
	if result.Failure == nil || result.Failure.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("failure = %#v, want classified timeout failure", result.Failure)
	}
	assertTimeoutDraftsExcludeCompletedRun(t, result.Drafts)
}

func assertTimeoutDraftsExcludeCompletedRun(t *testing.T, drafts []responseevents.Draft) {
	t.Helper()

	for _, draft := range drafts {
		if draft.Kind != responseevents.KindRun {
			continue
		}
		if draft.Phase == responseevents.PhaseCompleted {
			t.Fatalf("drafts include completed run success signal: %#v", drafts)
		}
		if draft.Phase == responseevents.PhaseFailed {
			var payload responseevents.RunPayload
			if err := json.Unmarshal(draft.Payload, &payload); err != nil {
				t.Fatalf("unmarshal failed run payload: %v", err)
			}
			if payload.Status != "failed" {
				t.Fatalf("failed run status = %q, want failed", payload.Status)
			}
		}
	}
}
