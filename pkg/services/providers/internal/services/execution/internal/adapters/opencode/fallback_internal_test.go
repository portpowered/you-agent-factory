package opencode

import (
	"context"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type singleRunRunner struct {
	result workers.CommandResult
	err    error
}

func (runner *singleRunRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return runner.result, runner.err
}

func TestRunAttemptCapturesUnsupportedFormatRejection(t *testing.T) {
	t.Parallel()

	rejection := []byte("error: unknown option '--format'\n")
	effect := commandEffect{
		runner: &singleRunRunner{
			result: workers.CommandResult{Stderr: rejection, ExitCode: 2},
		},
		mode: ModeStructured,
	}
	outcome := runAttempt(
		context.Background(),
		providers.ExecuteRequest{Provider: providers.IDOpenCode, AttemptID: "attempt-1"},
		effect,
		ModeStructured,
	)
	if !plansStructuredFallback(context.Background(), providers.ExecuteRequest{}, outcome, false) {
		t.Fatalf("outcome = %#v", outcome)
	}
}

type sequenceRunner struct {
	attempts []workers.CommandResult
}

func (runner *sequenceRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	if len(runner.attempts) == 0 {
		return workers.CommandResult{}, nil
	}
	result := runner.attempts[0]
	runner.attempts = runner.attempts[1:]
	return result, nil
}

func (runner *sequenceRunner) RunStreaming(
	ctx context.Context,
	request workers.CommandRequest,
	observe workers.OutputChunkObserver,
) (workers.CommandResult, error) {
	result, err := runner.Run(ctx, request)
	if observe != nil && len(result.Stdout) > 0 {
		observe(workers.OutputStreamStdout, result.Stdout)
	}
	return result, err
}

func TestRunAttemptCapturesStreamingUnsupportedFormatRejection(t *testing.T) {
	t.Parallel()

	rejection := []byte("error: unknown option '--format'\n")
	effect := commandEffect{
		runner: &sequenceRunner{
			attempts: []workers.CommandResult{{Stderr: rejection, ExitCode: 2}},
		},
		mode: ModeStructured,
	}
	outcome := runAttempt(
		context.Background(),
		providers.ExecuteRequest{Provider: providers.IDOpenCode, AttemptID: "attempt-1"},
		effect,
		ModeStructured,
	)
	if !plansStructuredFallback(context.Background(), providers.ExecuteRequest{}, outcome, false) {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestNewAttemptFallsBackThroughNegotiatingEffect(t *testing.T) {
	t.Parallel()

	rejection := []byte("error: unknown option '--format'\n")
	runner := &sequenceRunner{
		attempts: []workers.CommandResult{
			{Stderr: rejection, ExitCode: 2},
			{Stdout: []byte("fallback answer")},
		},
	}
	request := providers.ExecuteRequest{Provider: providers.IDOpenCode, AttemptID: "attempt-fallback"}
	attempt := newAttempt(newNegotiatingEffect(runner), ModeStructured, RegistrationOptions{})
	result, err := attempt(context.Background(), request)
	if err != nil {
		t.Fatalf("attempt() error = %v (%T)", err, err)
	}
	if result.Content != "fallback answer" {
		t.Fatalf("Content = %q", result.Content)
	}
}
