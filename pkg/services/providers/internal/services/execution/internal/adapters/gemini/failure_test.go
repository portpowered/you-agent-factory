package gemini_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	gemini "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/gemini"
	providertestdata "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/testdata"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCommandEffectNormalizesFailureCorpus(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"gemini_structured_invalid_request_precedence",
		"gemini_stderr_throttle_precedence",
		"gemini_stdout_timeout_recovery",
		"gemini_unknown_safe_excerpt",
		"gemini_noise_exit_fallback",
	} {
		entry := providertestdata.MustEntry(t, name)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runner := &staticRunner{result: workers.CommandResult{
				Stdout:   entry.FailureInput().Stdout,
				Stderr:   entry.FailureInput().Stderr,
				ExitCode: entry.FailureInput().ExitCode,
			}}
			effect := gemini.NewCommandEffect(runner)
			_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
				Provider:    providers.IDGemini,
				AttemptID:   "attempt-failure",
				UserMessage: "deterministic failure prompt",
			}, func([]byte) error { return nil })
			var failure providers.ExecuteFailure
			if !errors.As(err, &failure) {
				t.Fatalf("Execute() error = %v, want ExecuteFailure", err)
			}
			if failure.Kind != mapWorkersFailureKind(entry.ExpectedType) {
				t.Fatalf("failure kind = %q, want %q", failure.Kind, mapWorkersFailureKind(entry.ExpectedType))
			}
			if failure.Message != entry.ExpectedMessage {
				t.Fatalf("failure message = %q, want %q", failure.Message, entry.ExpectedMessage)
			}
			for _, rejected := range entry.RejectMessageContains {
				if strings.Contains(failure.Message, rejected) {
					t.Fatalf("failure message leaked %q: %q", rejected, failure.Message)
				}
			}
		})
	}
}

func TestCommandEffectDeadlineTimeoutOutranksStreamDetail(t *testing.T) {
	t.Parallel()

	runner := &staticRunner{
		result: workers.CommandResult{
			ExitCode: 1,
			Stderr:   []byte("token=customer-secret-value"),
		},
		err: context.DeadlineExceeded,
	}
	effect := gemini.NewCommandEffect(runner)
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDGemini,
		AttemptID:   "attempt-timeout",
		UserMessage: "deterministic failure prompt",
	}, func([]byte) error { return nil })
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v, want ExecuteFailure", err)
	}
	if failure.Kind != providers.ExecuteFailureKindTimeout {
		t.Fatalf("failure kind = %q, want timeout", failure.Kind)
	}
	if failure.Message != gemini.TimeoutFailureMessage {
		t.Fatalf("failure message = %q, want %q", failure.Message, gemini.TimeoutFailureMessage)
	}
	if strings.Contains(failure.Message, "token=") {
		t.Fatalf("failure message leaked sensitive facts: %q", failure.Message)
	}
}

type staticRunner struct {
	result workers.CommandResult
	err    error
}

func (r *staticRunner) Run(
	_ context.Context,
	_ workers.CommandRequest,
) (workers.CommandResult, error) {
	return r.result, r.err
}

func mapWorkersFailureKind(reason workers.WorkFailureType) providers.ExecuteFailureKind {
	switch reason {
	case workers.WorkFailureTypeAuthFailure:
		return providers.ExecuteFailureKindAuthentication
	case workers.WorkFailureTypePermanentBadRequest:
		return providers.ExecuteFailureKindInvalidRequest
	case workers.WorkFailureTypeThrottled:
		return providers.ExecuteFailureKindThrottled
	case workers.WorkFailureTypeTimeout:
		return providers.ExecuteFailureKindTimeout
	case workers.WorkFailureTypeInternalServerError:
		return providers.ExecuteFailureKindDependency
	default:
		return providers.ExecuteFailureKindUnknown
	}
}
