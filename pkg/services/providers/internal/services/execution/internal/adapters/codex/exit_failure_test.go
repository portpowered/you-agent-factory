package codex_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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

			effect := codex.NewCommandEffect(codexCommandRunnerStub{
				result: workers.CommandResult{
					ExitCode: 1,
					Stderr:   []byte(test.stderr),
				},
			}, platformclock.Real{})
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
	effect := codex.NewCommandEffect(codexCommandRunnerStub{
		result: workers.CommandResult{
			ExitCode: 1,
			Stderr:   []byte("Not inside a trusted directory and --skip-git-repo-check was not specified."),
		},
	}, platformclock.Real{})
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
	effect := codex.NewCommandEffect(codexCommandRunnerStub{
		result: workers.CommandResult{
			ExitCode: 1,
			Stderr:   []byte(`{"type":"turn.failed","error":{"message":"` + providerDetail + `"}}`),
		},
	}, platformclock.Real{})
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
	result workers.CommandResult
}

func (stub codexCommandRunnerStub) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return stub.result, nil
}
