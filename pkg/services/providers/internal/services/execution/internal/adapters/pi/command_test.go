package pi_test

import (
	"context"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	pi "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/pi"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCommandEffectBuildsJsonModeInvocation(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	effect := pi.NewCommandEffect(runner)
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:     providers.IDPi,
		AttemptID:    "attempt-command",
		UserMessage:  "hello",
		SystemPrompt: "system",
		Model:        "anthropic/claude-sonnet-4",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDPi,
			Kind:     providers.SessionIDKind,
			ID:       "pi-session-command",
		},
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.request.Command != "pi" {
		t.Fatalf("command = %q, want pi", runner.request.Command)
	}
	wantArgs := []string{
		"--print", "--mode", "json", "--approve",
		"--model", "anthropic/claude-sonnet-4",
		"--session", "pi-session-command",
		"--system-prompt", "system",
		"hello",
	}
	if len(runner.request.Args) != len(wantArgs) {
		t.Fatalf("args = %#v, want %#v", runner.request.Args, wantArgs)
	}
	for index := range wantArgs {
		if runner.request.Args[index] != wantArgs[index] {
			t.Fatalf("args[%d] = %q, want %q", index, runner.request.Args[index], wantArgs[index])
		}
	}
}

type recordingRunner struct {
	request workers.CommandRequest
}

func (runner *recordingRunner) Run(
	_ context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	runner.request = request
	return workers.CommandResult{
		Stdout: []byte(`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"ok"}],"stopReason":"stop"}}` + "\n"),
	}, nil
}
