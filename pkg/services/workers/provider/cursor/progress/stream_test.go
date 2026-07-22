package progress_test

import (
	"context"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/cursor/progress"
	"testing"
)

func TestIsCommand_AcceptsNativeExecutableShapes(t *testing.T) {
	for _, command := range []string{"agent", "agent.exe", `C:\tools\agent.cmd`, "/usr/local/bin/agent"} {
		if !progress.IsCommand(command) {
			t.Fatalf("IsCommand(%q) = false, want true", command)
		}
	}
	if progress.IsCommand("agent-helper.exe") {
		t.Fatal("agent-helper.exe must not be classified as the Cursor provider command")
	}
}

func TestResponseEventStream_PublishesDiagnosticsAndValidDraftsInOrder(t *testing.T) {
	stdout := "{not json}\n" +
		"{\"type\":\"mystery\"}\n" +
		"{\"type\":\"assistant\",\"timestamp_ms\":1,\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Plan \"}]},\"session_id\":\"cursor-session-123\"}\n" +
		"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"Plan done\",\"session_id\":\"cursor-session-123\"}\n"

	var published []progress.ProgressFragment
	stream := progress.NewResponseEventStream("dispatch-stream-cursor", func(fragment progress.ProgressFragment) {
		published = append(published, fragment)
	}, nil)
	stream.Observe(context.Background(), string(adapter.OutputStreamStdout), []byte(stdout))
	stream.Observe(context.Background(), string(adapter.OutputStreamStderr), []byte("stderr planning note\n"))
	stream.Flush(context.Background(), adapter.FlushReasonCompleted)

	if len(published) != 5 {
		t.Fatalf("published fragments = %#v, want 5 ordered fragments", published)
	}

	var diagnostics []progress.ProgressFragment
	var drafts []factorysessions.ResponseEventDraft
	for _, fragment := range published {
		if fragment.DispatchID != "dispatch-stream-cursor" {
			t.Fatalf("dispatch = %q, want dispatch-stream-cursor", fragment.DispatchID)
		}
		if !fragment.HasCanonicalDraft {
			diagnostics = append(diagnostics, fragment)
			continue
		}
		drafts = append(drafts, fragment.CanonicalDraft)
	}
	if len(diagnostics) != 3 || len(drafts) != 2 {
		t.Fatalf("published fragments = %#v, want three diagnostics and two structured drafts", published)
	}
	if diagnostics[0].Payload == "" || diagnostics[1].Payload == "" {
		t.Fatalf("diagnostics = %#v, want bounded diagnostic payloads", diagnostics)
	}
	for index, wantPhase := range []factorysessions.ResponseEventPhase{factorysessions.ResponseEventPhaseDelta, factorysessions.ResponseEventPhaseCompleted} {
		draft := drafts[index]
		if draft.Kind != factorysessions.ResponseEventKindMessage || draft.Phase != wantPhase || draft.DispatchID != "dispatch-stream-cursor" {
			t.Fatalf("drafts[%d] = %#v, want MESSAGE/%s for dispatch", index, draft, wantPhase)
		}
		if draft.ProviderSessionRef != "cursor-session-123" || draft.Provenance.Provider != "cursor" {
			t.Fatalf("drafts[%d] correlation = %#v, want Cursor session", index, draft)
		}
	}
}

func TestFlushReason_MapsCompletionContext(t *testing.T) {
	testCases := []struct {
		name     string
		ctx      context.Context
		exitCode int
		err      error
		want     adapter.FlushReason
	}{
		{name: "completed", ctx: context.Background(), exitCode: 0, want: adapter.FlushReasonCompleted},
		{name: "terminated", ctx: context.Background(), exitCode: 1, want: adapter.FlushReasonTerminated},
		{name: "canceled", ctx: canceledContext(), exitCode: 0, want: adapter.FlushReasonCanceled},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := progress.FlushReason(tc.ctx, tc.exitCode, tc.err); got != tc.want {
				t.Fatalf("FlushReason() = %q, want %q", got, tc.want)
			}
		})
	}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
