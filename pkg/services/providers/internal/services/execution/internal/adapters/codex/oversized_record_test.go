package codex_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
)

// TestCodexRootSkipsOversizedMidStreamRecordAndRecoversFinalDecision pins the
// root-cause fix for the rsm-4 review-lane kill: `codex exec --json` emits one
// tool result as one JSONL record carrying the full aggregated command output,
// so a single fetched CI log can exceed the 1 MiB record inspection limit
// mid-stream. The oversized record must be skipped -- with a truncation
// diagnostic -- while the later final agent decision is still recovered.
func TestCodexRootSkipsOversizedMidStreamRecordAndRecoversFinalDecision(t *testing.T) {
	t.Parallel()

	finalDecision := "<CONTINUE>\n\nHold: CI is red-terminal; waiting on checks."
	stream := []byte(
		`{"type":"thread.started","thread_id":"thread-oversized-recovery"}` + "\n" +
			`{"type":"item.completed","item":{"id":"tool-log-fetch","type":"command_execution",` +
			`"command":"gh run view --log-failed","aggregated_output":"` +
			strings.Repeat("x", (1<<20)+128) + `"}}` + "\n" +
			`{"type":"item.completed","item":{"id":"reason-after","type":"reasoning","text":"post-skip reasoning"}}` + "\n" +
			`{"type":"item.completed","item":{"id":"message-final","type":"agent_message","text":"` +
			`<CONTINUE>\n\nHold: CI is red-terminal; waiting on checks.` + `"}}` + "\n",
	)
	effect := codex.EffectFunc(func(
		_ context.Context,
		_ execution.ContinuationRequest,
		observe func([]byte) error,
	) (codex.EffectResult, error) {
		for _, chunk := range splitEvery(stream, 64<<10) {
			if err := observe(chunk); err != nil {
				return codex.EffectResult{}, err
			}
		}
		return codex.EffectResult{}, nil
	})

	result, err := newCodexRoot(t, effect).Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDCodex,
		AttemptID:   "attempt-oversized-recovery",
		UserMessage: "review the pull request",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want recovered final decision after skipped record", err)
	}
	if result.Content != finalDecision {
		t.Fatalf("Execute() Content = %q, want the recovered final agent decision", result.Content)
	}
	if result.SessionRef == nil || result.SessionRef.ID != "thread-oversized-recovery" {
		t.Fatalf("Execute() SessionRef = %#v, want the observed thread", result.SessionRef)
	}
	if result.Diagnostics == nil {
		t.Fatal("Execute() Diagnostics = nil, want truncation diagnostics")
	}
	assertSkippedRecordMetadata(t, result.Diagnostics.Metadata)
	assertSkippedRecordProgress(t, result.Diagnostics.Progress)
}

func assertSkippedRecordMetadata(t *testing.T, metadata map[string]string) {
	t.Helper()
	if metadata["inspection_records_skipped"] != "1" ||
		metadata["inspection_limit_category"] != "record" ||
		metadata["inspection_limit_configured"] != "1048576" ||
		metadata["inspection_limit_observed"] == "" ||
		metadata["inspection_limit_line"] != "2" {
		t.Fatalf("skip metadata = %#v, want bounded skipped-record facts", metadata)
	}
	if metadata["completion_evidence"] != "agent_message" {
		t.Fatalf("completion_evidence = %q, want agent_message", metadata["completion_evidence"])
	}
}

func assertSkippedRecordProgress(t *testing.T, progress []providers.ExecuteProgress) {
	t.Helper()
	if got := countPhase(progress, "message.completed"); got != 1 {
		t.Fatalf("completed message facts = %d, want the post-skip final message decoded", got)
	}
	if !hasSkippedRecordDiagnostic(progress) {
		t.Fatalf("progress = %#v, want a record_skipped diagnostic fact", progress)
	}
	for _, fact := range progress {
		if strings.Contains(fact.Detail, strings.Repeat("x", 128)) {
			t.Fatalf("progress retained oversized record content: %q", fact.Detail[:200])
		}
	}
}

// TestCodexRootStillFailsWhenSkippedRecordHidesFinalDecision pins the terminal
// half of the contract: skipping an oversized record is only safe because the
// execution still fails -- with the pre-existing record-limit dependency
// classification -- when the stream ends without a recoverable final decision.
func TestCodexRootStillFailsWhenSkippedRecordHidesFinalDecision(t *testing.T) {
	t.Parallel()

	stream := []byte(
		`{"type":"thread.started","thread_id":"thread-skip-terminal"}` + "\n" +
			`{"type":"item.completed","item":{"id":"message-oversized","type":"agent_message","text":"` +
			strings.Repeat("x", (1<<20)+64) + `"}}` + "\n",
	)
	effect := codex.EffectFunc(func(
		_ context.Context,
		_ execution.ContinuationRequest,
		observe func([]byte) error,
	) (codex.EffectResult, error) {
		for _, chunk := range splitEvery(stream, 64<<10) {
			if err := observe(chunk); err != nil {
				return codex.EffectResult{}, err
			}
		}
		return codex.EffectResult{}, nil
	})

	result, err := newCodexRoot(t, effect).Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDCodex,
		AttemptID:   "attempt-skip-terminal",
		UserMessage: "review the pull request",
	})
	assertCodexFailure(t, result, err, providers.ExecuteFailureKindDependency, "")
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %#v, want ExecuteFailure", err)
	}
	if !strings.Contains(failure.Message, "record limit") ||
		!strings.Contains(failure.Message, "thread-skip-terminal") {
		t.Fatalf("failure message = %q, want safe record-limit context", failure.Message)
	}
	if failure.Diagnostics == nil ||
		failure.Diagnostics.Metadata["inspection_records_skipped"] != "1" ||
		failure.Diagnostics.Metadata["inspection_limit_category"] != "record" {
		t.Fatalf("failure diagnostics = %#v, want skipped-record facts", failure.Diagnostics)
	}
}

func hasSkippedRecordDiagnostic(progress []providers.ExecuteProgress) bool {
	for _, fact := range progress {
		if fact.Phase == "diagnostic" && fact.Metadata["code"] == "record_skipped" &&
			fact.Metadata["record_bytes"] != "" {
			return true
		}
	}
	return false
}
