package opencode_test

import (
	"context"
	"errors"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	opencode "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/opencode"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestOpenCodeCommandEffectClassifiesStderrExitFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		stderr   string
		wantKind providers.ExecuteFailureKind
	}{
		{
			name:     "authentication stderr",
			stderr:   `ERROR: unexpected status 401 Unauthorized login required`,
			wantKind: providers.ExecuteFailureKindAuthentication,
		},
		{
			name:     "bad request stderr",
			stderr:   "invalid request: model not found",
			wantKind: providers.ExecuteFailureKindInvalidRequest,
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
		{
			name:     "server stderr",
			stderr:   "internal server error while contacting provider",
			wantKind: providers.ExecuteFailureKindDependency,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			effect := opencode.NewCommandEffect(openCodeCommandRunnerStub{
				result: workers.CommandResult{
					ExitCode: 1,
					Stderr:   []byte(test.stderr),
				},
			}, opencode.CommandEffectOptions{Mode: opencode.ModeStructured})
			_, err := newOpenCodeRoot(t, effect, opencode.ModeStructured).Execute(
				t.Context(),
				openCodeFailureRequest(),
			)
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

type openCodeCommandRunnerStub struct {
	result workers.CommandResult
}

func (stub openCodeCommandRunnerStub) Run(
	_ context.Context,
	_ workers.CommandRequest,
) (workers.CommandResult, error) {
	return stub.result, nil
}
