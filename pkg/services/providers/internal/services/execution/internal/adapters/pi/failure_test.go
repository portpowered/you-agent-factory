package pi_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	pi "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/pi"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCommandEffectMapsRetryFailureFromStdout(t *testing.T) {
	t.Parallel()

	runner := &staticRunner{result: workers.CommandResult{
		ExitCode: 1,
		Stdout: []byte(strings.Join([]string{
			`{"type":"session","id":"pi-session-retry"}`,
			`{"type":"auto_retry_start","attempt":2,"retryDelayMs":2000,"errorStatus":429}`,
		}, "\n") + "\n"),
	}}
	effect := pi.NewCommandEffect(runner)
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDPi,
		AttemptID:   "attempt-retry",
		UserMessage: "deterministic failure prompt",
	}, func([]byte) error { return nil })
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v, want ExecuteFailure", err)
	}
	if failure.Kind != providers.ExecuteFailureKindThrottled {
		t.Fatalf("failure kind = %q, want throttled", failure.Kind)
	}
	if failure.Message != "Pi reported a retryable provider API failure." {
		t.Fatalf("failure message = %q", failure.Message)
	}
	if failure.SessionRef == nil || failure.SessionRef.ID != "pi-session-retry" {
		t.Fatalf("SessionRef = %#v", failure.SessionRef)
	}
}

func TestCommandEffectMapsTerminalAssistantFailure(t *testing.T) {
	t.Parallel()

	runner := &staticRunner{result: workers.CommandResult{
		ExitCode: 1,
		Stdout: []byte(strings.Join([]string{
			`{"type":"session","id":"pi-session-production"}`,
			`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"rate limited"}}`,
		}, "\n") + "\n"),
	}}
	effect := pi.NewCommandEffect(runner)
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDPi,
		AttemptID:   "attempt-terminal",
		UserMessage: "deterministic failure prompt",
	}, func([]byte) error { return nil })
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v, want ExecuteFailure", err)
	}
	if failure.Message != "rate limited" {
		t.Fatalf("failure message = %q", failure.Message)
	}
}

func TestCommandEffectDeadlineTimeout(t *testing.T) {
	t.Parallel()

	runner := &staticRunner{
		result: workers.CommandResult{ExitCode: 1},
		err:    context.DeadlineExceeded,
	}
	effect := pi.NewCommandEffect(runner)
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDPi,
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
}

type staticRunner struct {
	result workers.CommandResult
	err    error
}

func (runner *staticRunner) Run(
	_ context.Context,
	_ workers.CommandRequest,
) (workers.CommandResult, error) {
	return runner.result, runner.err
}
