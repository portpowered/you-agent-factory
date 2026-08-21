package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	// This is the sanitized stderr emitted by Codex 0.149.0 when the command
	// starts outside a trusted Git directory without changing trust settings.
	codexUntrustedWorkingDirectoryStderr   = "Not inside a trusted directory and --skip-git-repo-check was not specified."
	codexUntrustedWorkingDirectoryExitCode = 1
	codexGenericProviderFailureMessage     = "provider execution failed"
)

// TestCodexUntrustedWorkingDirectoryCharacterization records the pre-fix
// behavior: Codex's deterministic trust refusal is retried three times and
// then reaches the configured process loop breaker as a generic failure.
func TestCodexUntrustedWorkingDirectoryCharacterization(t *testing.T) {
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "codex_untrusted_working_directory",
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":   "process",
			"worker": "processor",
			"inputs": []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs": []any{map[string]any{
				"workType": "task",
				"state":    "complete",
			}},
			"onFailure": []any{map[string]any{
				"workType": "task",
				"state":    "init",
			}},
			"limits": map[string]any{"maxRetries": 3},
		}},
	})
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"codex trusted working directory refusal"}`))
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"gpt-5-codex",
	))

	runner := support.NewShapedProviderCommandRunner(
		codexUntrustedWorkingDirectoryCommandResult(),
		codexUntrustedWorkingDirectoryCommandResult(),
		codexUntrustedWorkingDirectoryCommandResult(),
	)
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want one after loop breaker; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:init"); got != 0 {
		t.Fatalf("init place tokens = %d, want zero after loop breaker; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 0 {
		t.Fatalf("complete place tokens = %d, want zero after refusal; listed=%#v", got, listed)
	}
	if got := runner.CallCount(); got != 3 {
		t.Fatalf("provider command calls = %d, want three equivalent Codex refusals before the circuit breaker", got)
	}
	for index, request := range runner.Requests() {
		if request.Command != string(modelprovider.ProviderCodex) {
			t.Fatalf("provider request %d command = %q, want codex", index+1, request.Command)
		}
		if request.WorkDir != dir {
			t.Fatalf("provider request %d working directory = %q, want isolated non-Git directory %q", index+1, request.WorkDir, dir)
		}
	}

	dispatches := support.ObserveDispatchEvents(t, events)
	processFailures := 0
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != "process" || dispatch.Response == nil {
			continue
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeFailed {
			t.Errorf("process response outcome = %q, want FAILED", dispatch.Response.Outcome)
		}
		if dispatch.Response.Error == nil || *dispatch.Response.Error != codexGenericProviderFailureMessage {
			actual := "<nil>"
			if dispatch.Response.Error != nil {
				actual = *dispatch.Response.Error
			}
			t.Errorf("process response error = %q, want generic provider failure", actual)
		}
		processFailures++
	}
	if processFailures != 3 {
		t.Fatalf("failed process dispatches = %d, want three before circuit-breaker exhaustion", processFailures)
	}
	t.Logf(
		"before reproduction: stderr=%q exit_code=%d working_directory=%q public_failure=%q provider_attempts=%d circuit_breaker=tripped",
		codexUntrustedWorkingDirectoryStderr,
		codexUntrustedWorkingDirectoryExitCode,
		dir,
		codexGenericProviderFailureMessage,
		runner.CallCount(),
	)
}

func codexUntrustedWorkingDirectoryCommandResult() platformprocess.CommandResult {
	return platformprocess.CommandResult{
		ExitCode: codexUntrustedWorkingDirectoryExitCode,
		Stderr:   []byte(codexUntrustedWorkingDirectoryStderr),
	}
}
