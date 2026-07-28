package cursor_test

import (
	"context"
	"errors"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	cursor "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/cursor"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCursorCommandEffectClassifiesStderrExitFailures(t *testing.T) {
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

			effect := cursor.NewCommandEffect(cursorCommandRunnerStub{
				result: workers.CommandResult{
					ExitCode: 1,
					Stderr:   []byte(test.stderr),
				},
			})
			_, err := newCursorRoot(t, effect).Execute(t.Context(), cursorFailureRequest())
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

type cursorCommandRunnerStub struct {
	result workers.CommandResult
}

func (stub cursorCommandRunnerStub) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return stub.result, nil
}
