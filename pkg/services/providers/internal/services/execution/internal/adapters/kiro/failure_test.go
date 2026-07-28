package kiro_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	kiro "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/kiro"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	providertestdata "github.com/portpowered/infinite-you/pkg/services/workers/provider/testdata"
)

func knownKiroMessage(reason workers.WorkFailureType) string {
	switch reason {
	case workers.WorkFailureTypeAuthFailure:
		return "Kiro authentication failed. Sign in again and retry."
	case workers.WorkFailureTypePermanentBadRequest:
		return "Kiro rejected the request as invalid."
	case workers.WorkFailureTypeThrottled:
		return "Kiro is temporarily unavailable due to usage or capacity limits."
	case workers.WorkFailureTypeTimeout:
		return kiro.TimeoutFailureMessage
	case workers.WorkFailureTypeInternalServerError:
		return "Kiro encountered a temporary service error."
	default:
		return ""
	}
}

func TestCommandEffectNormalizesFailureCorpus(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"kiro_structured_authentication_error",
		"kiro_structured_invalid_request_stdout",
		"kiro_text_authentication_stdout",
		"kiro_structured_throttle_precedes_text",
		"kiro_text_capacity_error",
		"kiro_text_timeout_malformed_structured",
		"kiro_structured_service_unavailable",
		"kiro_unknown_stderr_excerpt_precedes_stdout",
		"kiro_unknown_stdout_excerpt_after_unsafe_stderr",
		"kiro_unknown_noise_only_exit_fallback",
	} {
		entry := providertestdata.MustEntry(t, name)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runner := &staticRunner{result: workers.CommandResult{
				Stdout:   entry.FailureInput().Stdout,
				Stderr:   entry.FailureInput().Stderr,
				ExitCode: entry.FailureInput().ExitCode,
			}}
			effect := kiro.NewCommandEffect(runner)
			_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
				Provider:    providers.IDKiro,
				AttemptID:   "attempt-failure",
				UserMessage: "deterministic failure prompt",
			}, func([]byte) error { return nil })
			var failure providers.ExecuteFailure
			if !errors.As(err, &failure) {
				t.Fatalf("Execute() error = %v, want ExecuteFailure", err)
			}
			wantMessage := knownKiroMessage(entry.ExpectedType)
			if wantMessage == "" {
				switch name {
				case "kiro_unknown_stderr_excerpt_precedes_stdout":
					wantMessage = "Kiro error: model registry handshake failed"
				case "kiro_unknown_stdout_excerpt_after_unsafe_stderr":
					wantMessage = "Kiro error: plugin bridge failed"
				case "kiro_unknown_noise_only_exit_fallback":
					wantMessage = "kiro-cli exited with code 11"
				}
			}
			if failure.Kind != mapWorkersFailureKind(entry.ExpectedType) {
				t.Fatalf("failure kind = %q, want %q", failure.Kind, mapWorkersFailureKind(entry.ExpectedType))
			}
			if failure.Message != wantMessage {
				t.Fatalf("failure message = %q, want %q", failure.Message, wantMessage)
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
	effect := kiro.NewCommandEffect(runner)
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDKiro,
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
	if failure.Message != kiro.TimeoutFailureMessage {
		t.Fatalf("failure message = %q, want %q", failure.Message, kiro.TimeoutFailureMessage)
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
