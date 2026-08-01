package codex_test

import (
	"context"
	"errors"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	effects "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
)

func TestCodexCommandEffectClassifiesStderrExitFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		stderr   string
		wantKind providers.ExecuteFailureKind
	}{
		{
			name:     "authentication stderr",
			stderr:   `ERROR: unexpected status 401 Unauthorized {"type":"authentication_error","message":"invalid api key"}`,
			wantKind: providers.ExecuteFailureKindAuthentication,
		},
		{
			name:     "throttle stderr",
			stderr:   "ERROR: selected model is at capacity",
			wantKind: providers.ExecuteFailureKindThrottled,
		},
		{
			name:     "timeout stderr",
			stderr:   "request timed out after waiting for provider response",
			wantKind: providers.ExecuteFailureKindTimeout,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			effect := codex.NewCommandEffect(codexCommandRunnerStub{
				result: effects.CommandResult{
					ExitCode: 1,
					Stderr:   []byte(test.stderr),
				},
			})
			_, err := newCodexRoot(t, effect).Execute(t.Context(), codexFailureRequest())
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

type codexCommandRunnerStub struct {
	result effects.CommandResult
}

func (stub codexCommandRunnerStub) Run(_ context.Context, _ effects.CommandRequest) (effects.CommandResult, error) {
	return stub.result, nil
}
