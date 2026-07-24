package provider

import (
	"context"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	geminipkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/gemini"
)

// Migrated Gemini must no longer own aggregate command/failure/timeout branches.
// Production selection stays on registry + conductor; these assertions prove the
// legacy Gemini-named aggregate ownership is gone without introducing a new
// concrete Gemini switch in shared orchestration.
func TestAggregateSurfacesOmitMigratedGeminiBranches(t *testing.T) {
	t.Parallel()

	t.Run("command_construction", func(t *testing.T) {
		t.Parallel()
		behavior := providerBehaviorFor(string(modelprovider.ProviderGemini), logging.NoopLogger{})
		args, err := behavior.BuildArgs(context.Background(), workerexecution.ProviderInferenceRequest{
			ModelProvider: string(modelprovider.ProviderGemini),
			UserMessage:   "summarize the workspace",
		}, false, nil)
		if err != nil {
			t.Fatalf("BuildArgs error = %v", err)
		}
		if len(args) > 0 && args[0] == "--prompt" {
			t.Fatalf("aggregate still owns Gemini argv: %#v", args)
		}
		command := behavior.BuildCommandRequest(workerexecution.ProviderInferenceRequest{
			ModelProvider: string(modelprovider.ProviderGemini),
			UserMessage:   "summarize the workspace",
		}, args)
		if command.Command == string(modelprovider.ProviderGemini) && containsArgPair(command.Args, "--prompt", "summarize the workspace") {
			t.Fatalf("aggregate still owns Gemini command request: %#v", command)
		}
	})

	t.Run("exit_failure", func(t *testing.T) {
		t.Parallel()
		parsed := parseProviderExitFailure(string(modelprovider.ProviderGemini), CommandResult{
			ExitCode: 1,
			Stderr:   []byte(`{"error":{"status":"UNAUTHENTICATED"}}`),
		})
		if parsed.failure.Message == "Gemini authentication failed." {
			t.Fatal("aggregate still owns Gemini exit-failure parsing")
		}
	})

	t.Run("timeout_failure", func(t *testing.T) {
		t.Parallel()
		parsed := parseProviderTimeoutFailure(string(modelprovider.ProviderGemini), CommandResult{})
		if parsed.Message == geminipkg.TimeoutFailureMessage {
			t.Fatal("aggregate still owns Gemini timeout parsing")
		}
	})
}

func TestAggregateSurfacesRetainNonGeminiProviders(t *testing.T) {
	t.Parallel()

	t.Run("kiro_command_construction", func(t *testing.T) {
		t.Parallel()
		behavior := providerBehaviorFor(string(modelprovider.ProviderKiro), logging.NoopLogger{})
		args, err := behavior.BuildArgs(context.Background(), workerexecution.ProviderInferenceRequest{
			ModelProvider: string(modelprovider.ProviderKiro),
			UserMessage:   "summarize the workspace",
		}, false, nil)
		if err != nil {
			t.Fatalf("BuildArgs error = %v", err)
		}
		wantPrefix := []string{"chat", "--no-interactive", "summarize the workspace"}
		if strings.Join(args, "\x00") != strings.Join(wantPrefix, "\x00") {
			t.Fatalf("kiro args = %#v, want %#v", args, wantPrefix)
		}
	})

	t.Run("kiro_exit_failure", func(t *testing.T) {
		t.Parallel()
		parsed := parseProviderExitFailure(string(modelprovider.ProviderKiro), CommandResult{
			ExitCode: 1,
			Stderr:   []byte("ERROR: Unauthorized"),
		})
		if parsed.failure.Reason != workerexecution.WorkFailureTypeAuthFailure {
			t.Fatalf("kiro failure reason = %q, want auth failure", parsed.failure.Reason)
		}
	})

	t.Run("kiro_timeout_failure", func(t *testing.T) {
		t.Parallel()
		parsed := parseProviderTimeoutFailure(string(modelprovider.ProviderKiro), CommandResult{})
		if parsed.Reason != workerexecution.WorkFailureTypeTimeout {
			t.Fatalf("kiro timeout reason = %q, want timeout", parsed.Reason)
		}
		if parsed.Message == "" {
			t.Fatal("kiro timeout message is empty")
		}
	})
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}
