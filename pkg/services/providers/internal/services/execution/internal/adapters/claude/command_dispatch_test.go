package claude_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	claude "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCommandEffectRendersProviderNeutralReasoningEffort(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner()
	effect := claude.NewCommandEffect(workers.AdaptCommandRunner(runner), platformclock.Real{})
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}

	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{ExecuteRequest: providers.ExecuteRequest{
		Provider:        providers.IDClaude,
		AttemptID:       "claude-xhigh-dispatch",
		Model:           "claude-model",
		ReasoningEffort: "xhigh",
		UserMessage:     "perform work",
	}}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{
		"-p",
		"--model", "claude-model",
		"--effort", "xhigh",
		"--verbose",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"perform work",
	}
	if got := runner.LastRequest().Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("command args = %#v, want %#v", got, want)
	}
}

func TestCommandEffectRendersResumeSessionFlag(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner()
	effect := claude.NewCommandEffect(workers.AdaptCommandRunner(runner), platformclock.Real{})
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}

	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:    providers.IDClaude,
			AttemptID:   "claude-resume-dispatch",
			Model:       "claude-model",
			UserMessage: "continue the prior turn",
		},
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDClaude,
			Kind:     providers.SessionIDKind,
			ID:       "session-previous",
		},
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{
		"-p",
		"--model", "claude-model",
		"--resume", "session-previous",
		"--verbose",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"continue the prior turn",
	}
	if got := runner.LastRequest().Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("command args = %#v, want %#v - a continued attempt must resume the exact referenced session instead of starting a fresh one", got, want)
	}
}

func TestCommandEffectRejectsUnsupportedReasoningEffort(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		effort string
		want   string
	}{
		{name: "provider unsupported", effort: "minimal", want: `does not support reasoning effort "minimal"`},
		{name: "globally unsupported", effort: "extreme", want: `unsupported reasoning effort "extreme"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := testutil.NewProviderCommandRunner()
			effect := claude.NewCommandEffect(workers.AdaptCommandRunner(runner), platformclock.Real{})
			_, err := effect.Execute(context.Background(), execution.ContinuationRequest{ExecuteRequest: providers.ExecuteRequest{
				Provider:        providers.IDClaude,
				AttemptID:       "claude-invalid-effort-dispatch",
				ReasoningEffort: test.effort,
				UserMessage:     "perform work",
			}}, func([]byte) error { return nil })
			var failure execution.AttemptFailure
			if !errors.As(err, &failure) ||
				failure.NativeError == nil ||
				!strings.Contains(failure.NativeError.Error(), test.want) {
				t.Fatalf("Execute() error = %v, want %q", err, test.want)
			}
			if got := runner.Requests(); len(got) != 0 {
				t.Fatalf("runner requests = %#v, want none", got)
			}
		})
	}
}

// failingStartCommandRunner reports a subprocess that never started, which is
// what the host returns when the process loader rejects the command line.
type failingStartCommandRunner struct{ err error }

func (r failingStartCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, r.err
}

func TestCommandEffectDeclaresAnOversizedCommandLineSpawnFailure(t *testing.T) {
	t.Parallel()

	startErr := &platformprocess.CommandStartError{
		Command:           "claude",
		ArgsCount:         9,
		CommandLineLength: 32932,
		CommandLineLimit:  platformprocess.WindowsCommandLineLimit,
		Cause:             errors.New("The filename or extension is too long."),
	}
	effect := claude.NewCommandEffect(
		workers.AdaptCommandRunner(failingStartCommandRunner{err: startErr}),
		platformclock.Real{},
	)
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}

	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:    providers.IDClaude,
			AttemptID:   "claude-oversized-command-line",
			Model:       "claude-model",
			UserMessage: strings.Repeat("u", 9819),
		},
	}, func([]byte) error { return nil })

	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %#v, want a declared providers.ExecuteFailure", err)
	}
	if failure.Kind != providers.ExecuteFailureKindMisconfigured {
		t.Fatalf("failure kind = %q, want %q", failure.Kind, providers.ExecuteFailureKindMisconfigured)
	}
	if failure.Diagnostics == nil || failure.Diagnostics.Metadata["work-failure-type"] != "command_line_too_long" {
		t.Fatalf("failure diagnostics = %#v, want work-failure-type command_line_too_long", failure.Diagnostics)
	}
	if !strings.Contains(failure.Message, "32932") || !strings.Contains(failure.Message, "32767") {
		t.Fatalf("failure message = %q, want it to report the measured size against the host limit", failure.Message)
	}
}

// claudeFactorySystemPromptSize is the system-prompt size that reproduces the
// reported plan-stage boundary: with it inlined, a 9,152-character payload
// composed a 32,265-character command line and dispatched, while a
// 9,819-character payload composed 32,932 characters and was rejected by the
// 32,767-character process-loader limit.
const claudeFactorySystemPromptSize = 23000

func TestCommandEffectKeepsAnOversizedPromptOffTheCommandLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		payloadSize int
		wantInline  bool
	}{
		{name: "previously passing boundary stays in argv", payloadSize: 9152, wantInline: true},
		{name: "previously fatal boundary moves to stdin", payloadSize: 9819},
		{name: "previously fatal larger payload moves to stdin", payloadSize: 12088},
		{name: "very large payload moves to stdin", payloadSize: 50000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			prompt := strings.Repeat("u", tc.payloadSize)
			runner := testutil.NewProviderCommandRunner()
			effect := claude.NewCommandEffect(workers.AdaptCommandRunner(runner), platformclock.Real{})
			_, err := effect.Execute(context.Background(), execution.ContinuationRequest{
				ExecuteRequest: providers.ExecuteRequest{
					Provider:     providers.IDClaude,
					AttemptID:    "claude-oversized-payload",
					Model:        "claude-model",
					SystemPrompt: strings.Repeat("s", claudeFactorySystemPromptSize),
					UserMessage:  prompt,
				},
			}, func([]byte) error { return nil })
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			req := runner.LastRequest()
			length := platformprocess.ComposedCommandLineLength(req.Command, req.Args)
			if length >= platformprocess.WindowsCommandLineLimit {
				t.Fatalf("composed command line = %d characters, want below the %d-character process-loader limit",
					length, platformprocess.WindowsCommandLineLimit)
			}
			lastArg := req.Args[len(req.Args)-1]
			if tc.wantInline {
				if lastArg != prompt {
					t.Fatalf("last argument length = %d, want the %d-character prompt inline", len(lastArg), len(prompt))
				}
				if len(req.Stdin) != 0 {
					t.Fatalf("stdin = %d bytes, want the prompt to stay in argv at this size", len(req.Stdin))
				}
				return
			}
			if string(req.Stdin) != prompt {
				t.Fatalf("stdin = %d bytes, want the %d-character prompt", len(req.Stdin), len(prompt))
			}
			if lastArg == prompt {
				t.Fatal("prompt is still the last argument, want it delivered on stdin")
			}
			if lastArg != "--include-partial-messages" {
				t.Fatalf("last argument = %q, want the flags to end the command line", lastArg)
			}
		})
	}
}

func TestCommandEffectPreservesFlagsWhenThePromptMovesToStdin(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner()
	effect := claude.NewCommandEffect(workers.AdaptCommandRunner(runner), platformclock.Real{})
	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:        providers.IDClaude,
			AttemptID:       "claude-flags-preserved",
			Model:           "claude-model",
			ReasoningEffort: "high",
			SkipPermissions: true,
			UserMessage:     strings.Repeat("u", 40000),
		},
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := []string{
		"-p",
		"--dangerously-skip-permissions",
		"--model", "claude-model",
		"--effort", "high",
		"--verbose",
		"--output-format", "stream-json",
		"--include-partial-messages",
	}
	if got := runner.LastRequest().Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("command args = %#v, want %#v", got, want)
	}
}
