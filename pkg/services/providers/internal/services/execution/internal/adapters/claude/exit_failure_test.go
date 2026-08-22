package claude_test

import (
	"context"
	"errors"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	claude "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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

			effect := claude.NewCommandEffect(claudeCommandRunnerStub{
				result: workers.CommandResult{
					ExitCode: 1,
					Stderr:   []byte(test.stderr),
				},
			}, platformclock.Real{})
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
	result workers.CommandResult
}

func (stub claudeCommandRunnerStub) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return stub.result, nil
}
