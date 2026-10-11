package claude_test

import (
	"context"
	"errors"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	claude "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
)

func TestClaudeRejectedOutputObservedBeforeFailure(t *testing.T) {
	t.Parallel()
	var progress []providers.ExecuteProgress
	returned := false
	runner := providerservice.CommandRunner{RunStreaming: func(_ context.Context, _ providerservice.CommandRequest, observe providerservice.OutputChunkObserver) (providerservice.CommandResult, error) {
		if err := observe(providerservice.OutputStreamStdout, []byte("{\"type\":\"stream_event\",\"session_id\":\"mock-claude-session\",\"event\":{\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"PRIVATE ordinary output\"}}}\n{\"type\":\"result\",\"subtype\":\"error\",\"is_error\":true,\"result\":\"rejected\",\"session_id\":\"mock-claude-session\"}\n")); err != nil {
			t.Fatal(err)
		}
		if err := observe(providerservice.OutputStreamStderr, []byte("PRIVATE ordinary stderr")); err != nil {
			t.Fatal(err)
		}
		return providerservice.CommandResult{ExitCode: 42}, nil
	}}
	effect := claude.NewCommandEffect(runner, platformclock.Real{})
	registration := claude.NewRegistration(effect)
	result, err := registration.Attempt(t.Context(), providers.ExecuteRequest{
		Provider: providers.IDClaude, AttemptID: "rejected-attempt", UserMessage: "ordinary",
		ProgressObserver: func(fact providers.ExecuteProgress) {
			if returned {
				t.Fatal("progress after attempt returned")
			}
			progress = append(progress, fact)
		},
	})
	returned = true
	if err == nil || result.Content != "" {
		t.Fatalf("rejection returned accepted content: %q %v", result.Content, err)
	}
	var output []string
	for _, fact := range progress {
		if fact.Detail == "PRIVATE ordinary output" || fact.Detail == "PRIVATE ordinary stderr" {
			output = append(output, fact.Detail)
		}
		if fact.Phase == "run.completed" || fact.Phase == "message.completed" {
			t.Fatalf("false completion: %+v", fact)
		}
	}
	if len(output) != 2 || output[0] != "PRIVATE ordinary output" || output[1] != "PRIVATE ordinary stderr" {
		t.Fatalf("ordered once-only output: %#v", output)
	}
}

func TestClaudeCommandEffectClassifiesStderrExitFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		stderr   string
		wantKind providers.ExecuteFailureKind
	}{
		{
			name:     "authentication stderr",
			stderr:   "Error: authentication_error: invalid api key",
			wantKind: providers.ExecuteFailureKindAuthentication,
		},
		{
			name:     "throttle stderr",
			stderr:   "Error: rate limit exceeded, too many requests",
			wantKind: providers.ExecuteFailureKindThrottled,
		},
		{
			name:     "timeout stderr",
			stderr:   "request timed out after waiting for provider response",
			wantKind: providers.ExecuteFailureKindTimeout,
		},
		{
			// Verified against the real installed claude CLI:
			// `echo hi | claude --resume <fake-uuid> --verbose --output-format
			// stream-json --include-partial-messages -p "hi"` produces this
			// exact stderr text on exit code 1.
			name:     "stale session stderr",
			stderr:   "No conversation found with session ID: 00000000-0000-0000-0000-000000000000",
			wantKind: providers.ExecuteFailureKindSessionNotFound,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			effect := claude.NewCommandEffect((claudeCommandRunnerStub{
				result: providerservice.CommandResult{
					ExitCode: 1,
					Stderr:   []byte(test.stderr),
				},
			}).commandEffect(), platformclock.Real{})
			_, err := newClaudeRoot(t, effect).Execute(t.Context(), claudeFailureRequest())
			var failure providers.ExecuteFailure
			if !errors.As(err, &failure) {
				t.Fatalf("Execute() error = %v, want providers.ExecuteFailure", err)
			}
			if failure.Kind != test.wantKind {
				t.Fatalf("failure kind = %q, want %q", failure.Kind, test.wantKind)
			}
		})
	}
}

type claudeCommandRunnerStub struct {
	result providerservice.CommandResult
}

func (stub claudeCommandRunnerStub) Run(_ context.Context, _ providerservice.CommandRequest) (providerservice.CommandResult, error) {
	return stub.result, nil
}

// RunStreaming supplies the buffered fixture's completed stdout chunk.
func (stub claudeCommandRunnerStub) RunStreaming(ctx context.Context, request providerservice.CommandRequest, observe providerservice.OutputChunkObserver) (providerservice.CommandResult, error) {
	result, err := stub.Run(ctx, request)
	if len(result.Stdout) > 0 && observe != nil {
		if observeErr := observe(providerservice.OutputStreamStdout, result.Stdout); err == nil {
			err = observeErr
		}
	}
	return result, err
}

func (stub claudeCommandRunnerStub) commandEffect() providerservice.CommandRunner {
	return providerservice.CommandRunner{Run: stub.Run, RunStreaming: stub.RunStreaming}
}
