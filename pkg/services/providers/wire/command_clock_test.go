package wire

import (
	"context"
	"encoding/json"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"testing"
	"time"
)

func TestNewServicePreservesSelectedClockForCodexAndClaude(t *testing.T) {
	t.Parallel()
	for _, id := range []providers.ID{providers.IDCodex, providers.IDClaude} {
		t.Run(id.String(), func(t *testing.T) {
			t.Parallel()
			clock := platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond)
			runner := &clockCommandRunner{clock: clock}
			peer := &clockCommandRunner{clock: clock}
			codexRunner, claudeRunner := runner, peer
			if id == providers.IDClaude {
				codexRunner, claudeRunner = peer, runner
			}
			root, err := newTestProvidersService(IdentityCatalogProbe,
				platformclock.Real{}, logging.NoopLogger{}, nil, nil, nil,
				nil,
				NewCodexEffect((codexRunner).commandEffect(), clock, nil, nil),
				NewClaudeEffect((claudeRunner).commandEffect(), clock),
				Configuration{})
			if err != nil {
				t.Fatal(err)
			}
			if runner.calls != 0 {
				t.Fatal("construction started a provider command")
			}
			if peer.calls != 0 {
				t.Fatal("construction started a peer command")
			}
			result, err := root.Execute(t.Context(), providers.ExecuteRequest{AttemptID: "clock-attempt", Provider: id, UserMessage: "clock proof", WorkingDirectory: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			if result.Content != "selected clock result" || result.Diagnostics == nil || result.Diagnostics.DurationMillis != 37 {
				t.Fatalf("result = %#v, want content and supplied 37ms duration", result)
			}
			if runner.calls != 1 {
				t.Fatalf("command calls = %d, want one", runner.calls)
			}
			if peer.calls != 0 {
				t.Fatalf("peer command calls = %d, want isolated effects", peer.calls)
			}
		})
	}
}

type clockCommandRunner struct {
	clock *platformclock.Deterministic
	calls int
}

func (runner *clockCommandRunner) Run(_ context.Context, request CommandRequest) (CommandResult, error) {
	runner.calls++
	runner.clock.SetTick(37)
	output := "selected clock result"
	switch request.Command {
	case "agy":
		data, _ := json.Marshal(map[string]any{"event": "result", "result": map[string]any{"conversation_id": "clock-agy", "status": "SUCCESS", "response": output, "duration_seconds": 0.037, "num_turns": 1, "usage": map[string]int{"input_tokens": 0, "output_tokens": 0, "thinking_tokens": 0, "cache_read_tokens": 0, "total_tokens": 0}}})
		output = string(data)
	case "codex":
		output = `{"type":"thread.started","thread_id":"clock-thread"}` + "\n" +
			`{"type":"item.completed","item":{"id":"clock-message","type":"agent_message","text":"selected clock result"}}` + "\n" +
			`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}` + "\n"
	case "claude":
		output = `{"type":"result","subtype":"success","is_error":false,"result":"selected clock result","session_id":"clock-session"}` + "\n"
	}
	return CommandResult{Stdout: []byte(output)}, nil
}

// RunStreaming supplies the buffered fixture's completed stdout chunk.
func (runner *clockCommandRunner) RunStreaming(ctx context.Context, request CommandRequest, observe OutputChunkObserver) (CommandResult, error) {
	result, err := runner.Run(ctx, request)
	if len(result.Stdout) > 0 && observe != nil {
		if observeErr := observe(OutputStreamStdout, result.Stdout); err == nil {
			err = observeErr
		}
	}
	return result, err
}

func (runner *clockCommandRunner) commandEffect() CommandRunner {
	return CommandRunner{Run: runner.Run, RunStreaming: runner.RunStreaming}
}
