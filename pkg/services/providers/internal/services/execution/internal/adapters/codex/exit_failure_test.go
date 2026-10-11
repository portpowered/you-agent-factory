package codex_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
)

func TestCodexRejectedOutputObservedBeforeFailure(t *testing.T) {
	t.Parallel()
	var progress []providers.ExecuteProgress
	returned := false
	runner := providerservice.CommandRunner{RunStreaming: func(_ context.Context, _ providerservice.CommandRequest, observe providerservice.OutputChunkObserver) (providerservice.CommandResult, error) {
		if err := observe(providerservice.OutputStreamStdout, []byte("{\"type\":\"item.updated\",\"item\":{\"id\":\"partial\",\"type\":\"agent_message\",\"text\":\"PRIVATE ordinary output\"}}\n{\"type\":\"turn.failed\",\"error\":{\"message\":\"rejected\"}}\n")); err != nil {
			t.Fatal(err)
		}
		if err := observe(providerservice.OutputStreamStderr, []byte("PRIVATE ordinary stderr")); err != nil {
			t.Fatal(err)
		}
		return providerservice.CommandResult{ExitCode: 42}, nil
	}}
	effect := codex.NewCommandEffect(runner, platformclock.Real{}, nil, nil)
	registration := codex.NewRegistration(effect)
	result, err := registration.Attempt(t.Context(), providers.ExecuteRequest{
		Provider: providers.IDCodex, AttemptID: "rejected-attempt", UserMessage: "ordinary",
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
		{
			// Verified against the real installed codex CLI:
			// `echo hi | codex exec --json resume <fake-uuid> -` produces
			// this exact stderr text on exit code 1.
			name:     "stale session stderr",
			stderr:   "Error: thread/resume: thread/resume failed: no rollout found for thread id 00000000-0000-0000-0000-000000000000 (code -32600)",
			wantKind: providers.ExecuteFailureKindSessionNotFound,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			effect := codex.NewCommandEffect((codexCommandRunnerStub{
				result: providerservice.CommandResult{
					ExitCode: 1,
					Stderr:   []byte(test.stderr),
				},
			}).commandEffect(), platformclock.Real{}, nil, nil)
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

func TestCodexCommandEffectClassifiesUntrustedWorkingDirectoryAsTerminalWithSafeDiagnostic(t *testing.T) {
	t.Parallel()

	workingDirectory := `C:\isolated\factory\with spaces`
	effect := codex.NewCommandEffect((codexCommandRunnerStub{
		result: providerservice.CommandResult{
			ExitCode: 1,
			Stderr:   []byte("Not inside a trusted directory and --skip-git-repo-check was not specified."),
		},
	}).commandEffect(), platformclock.Real{}, nil, nil)
	request := codexFailureRequest()
	request.WorkingDirectory = workingDirectory
	_, err := newCodexRoot(t, effect).Execute(t.Context(), request)

	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v, want providers.ExecuteFailure", err)
	}
	if failure.Kind != providers.ExecuteFailureKindInvalidRequest {
		t.Fatalf("failure kind = %q, want %q", failure.Kind, providers.ExecuteFailureKindInvalidRequest)
	}
	for _, required := range []string{
		workingDirectory,
		"Codex requires a trusted working directory",
		"suitable trusted Git repository",
	} {
		if !strings.Contains(failure.Message, required) {
			t.Errorf("failure message = %q, want it to contain %q", failure.Message, required)
		}
	}
	if strings.Contains(failure.Message, "--skip-git-repo-check") {
		t.Fatalf("failure message echoed native provider output: %q", failure.Message)
	}
	if failure.Diagnostics == nil || failure.Diagnostics.Metadata[providers.ExecuteDiagnosticMetadataSafeFailureMessage] != "true" {
		t.Fatalf("failure diagnostics = %#v, want safe-message marker", failure.Diagnostics)
	}
}

func TestCodexCommandEffectMarksUnknownTurnFailedFromExitOutput(t *testing.T) {
	t.Parallel()

	const providerDetail = "future turn failure credential=secret"
	effect := codex.NewCommandEffect((codexCommandRunnerStub{
		result: providerservice.CommandResult{
			ExitCode: 1,
			Stderr:   []byte(`{"type":"turn.failed","error":{"message":"` + providerDetail + `"}}`),
		},
	}).commandEffect(), platformclock.Real{}, nil, nil)
	_, err := newCodexRoot(t, effect).Execute(t.Context(), codexFailureRequest())

	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v, want providers.ExecuteFailure", err)
	}
	if failure.Kind != providers.ExecuteFailureKindUnknown {
		t.Fatalf("failure kind = %q, want unknown", failure.Kind)
	}
	if failure.Diagnostics == nil ||
		failure.Diagnostics.Metadata[providers.ExecuteDiagnosticMetadataUnrecognizedProviderRefusal] != "true" {
		t.Fatalf("failure diagnostics = %#v, want unrecognized-refusal marker", failure.Diagnostics)
	}
	if strings.Contains(err.Error(), providerDetail) {
		t.Fatalf("failure error leaked provider detail: %v", err)
	}
}

type codexCommandRunnerStub struct {
	result providerservice.CommandResult
}

func (stub codexCommandRunnerStub) Run(_ context.Context, _ providerservice.CommandRequest) (providerservice.CommandResult, error) {
	return stub.result, nil
}

func TestCodexCommandEffectClassifiesServerOverloadedExitOutputAsThrottled(t *testing.T) {
	t.Parallel()

	effect := codex.NewCommandEffect((codexCommandRunnerStub{
		result: providerservice.CommandResult{
			ExitCode: 1,
			Stdout:   []byte(`{"type":"item.completed","item":{"type":"reasoning"}}` + "\n"),
			Stderr:   []byte(`codex_error_info=server_overloaded`),
		},
	}).commandEffect(), platformclock.Real{}, nil, nil)
	_, err := newCodexRoot(t, effect).Execute(t.Context(), codexFailureRequest())

	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Execute() error = %v, want providers.ExecuteFailure", err)
	}
	if failure.Kind != providers.ExecuteFailureKindThrottled {
		t.Fatalf("failure kind = %q, want throttled", failure.Kind)
	}
}

// RunStreaming supplies the buffered fixture's completed stdout chunk.
func (stub codexCommandRunnerStub) RunStreaming(ctx context.Context, request providerservice.CommandRequest, observe providerservice.OutputChunkObserver) (providerservice.CommandResult, error) {
	result, err := stub.Run(ctx, request)
	if len(result.Stdout) > 0 && observe != nil {
		if observeErr := observe(providerservice.OutputStreamStdout, result.Stdout); err == nil {
			err = observeErr
		}
	}
	return result, err
}

func (stub codexCommandRunnerStub) commandEffect() providerservice.CommandRunner {
	return providerservice.CommandRunner{Run: stub.Run, RunStreaming: stub.RunStreaming}
}
