package codex

import (
	"os/exec"
	"strings"
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
)

// TestCodexUntrustedWorkingDirectoryFailsOnceWithActionableDiagnostic proves an untrusted directory fails once with remediation guidance.
func TestCodexUntrustedWorkingDirectoryFailsOnceWithActionableDiagnostic(t *testing.T) {
	dir := scaffoldCodexWorkingDirectoryFactory(t)
	runner := support.NewShapedProviderCommandRunner(codexUntrustedWorkingDirectoryCommandResult())
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want one terminal failure; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:init"); got != 0 {
		t.Fatalf("init place tokens = %d, want zero after terminal refusal; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 0 {
		t.Fatalf("complete place tokens = %d, want zero after refusal; listed=%#v", got, listed)
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("provider command calls = %d, want one terminal Codex refusal", got)
	}
	requests := runner.Requests()
	if len(requests) != 1 || requests[0].Command != string(modelprovider.ProviderCodex) || requests[0].WorkDir != dir {
		t.Fatalf("Codex command request = %#v, want one request in isolated directory %q", requests, dir)
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
		if dispatch.Response.Error == nil {
			t.Error("process response error = nil, want actionable Codex trust diagnostic")
		} else {
			for _, required := range []string{
				dir,
				"Codex requires a trusted working directory",
				"suitable trusted Git repository",
			} {
				if !strings.Contains(*dispatch.Response.Error, required) {
					t.Errorf("process response error = %q, want it to contain %q", *dispatch.Response.Error, required)
				}
			}
		}
		if dispatch.Response.FailureDetail == nil || dispatch.Response.FailureDetail.Reason != factoryapi.WorkFailureTypePermanentBadRequest {
			t.Errorf("process failure detail = %#v, want permanent bad request", dispatch.Response.FailureDetail)
		}
		processFailures++
	}
	if processFailures != 1 {
		t.Fatalf("failed process dispatches = %d, want one without circuit-breaker retries", processFailures)
	}
}

// TestCodexUnrecognizedRefusalFailsOnceWithNeutralDiagnostic proves an unknown Codex refusal fails once without misclassification.
func TestCodexUnrecognizedRefusalFailsOnceWithNeutralDiagnostic(t *testing.T) {
	dir := scaffoldCodexWorkingDirectoryFactory(t)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 77,
		// A structured provider error is deliberately unknown to this adapter.
		// The adapter marks it as a provider-declared refusal; raw command text
		// remains an ordinary unknown failure and keeps the existing contract.
		Stderr: []byte(`{"type":"error","message":"future refusal: credential=secret"}`),
	})
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want one terminal failure; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:init"); got != 0 {
		t.Fatalf("init place tokens = %d, want zero after terminal refusal; listed=%#v", got, listed)
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("provider command calls = %d, want one unrecognized refusal", got)
	}

	processFailures := 0
	for _, dispatch := range support.ObserveDispatchEvents(t, events) {
		if dispatch.Request.TransitionId != "process" || dispatch.Response == nil {
			continue
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeFailed {
			t.Errorf("process response outcome = %q, want FAILED", dispatch.Response.Outcome)
		}
		if dispatch.Response.Error == nil {
			t.Error("process response error = nil, want neutral provider refusal diagnostic")
		} else {
			if !strings.Contains(*dispatch.Response.Error, "provider rejected the execution request") {
				t.Errorf("process response error = %q, want neutral refusal diagnostic", *dispatch.Response.Error)
			}
			for _, forbidden := range []string{"future refusal", "credential=secret", "codex exited"} {
				if strings.Contains(*dispatch.Response.Error, forbidden) {
					t.Errorf("process response error = %q, must not expose %q", *dispatch.Response.Error, forbidden)
				}
			}
		}
		if dispatch.Response.FailureDetail == nil || dispatch.Response.FailureDetail.Reason != factoryapi.WorkFailureTypePermanentBadRequest {
			t.Errorf("process failure detail = %#v, want permanent bad request", dispatch.Response.FailureDetail)
		}
		processFailures++
	}
	if processFailures != 1 {
		t.Fatalf("failed process dispatches = %d, want one without authored retry or circuit-breaker retries", processFailures)
	}
}

func scaffoldCodexWorkingDirectoryFactory(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "codex_working_directory",
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
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"codex working directory diagnostic"}`))
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"gpt-5-codex",
	))
	return dir
}

func initTrustedGitRepository(t *testing.T, dir string) {
	t.Helper()
	command := exec.Command("git", "-C", dir, "init")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init in trusted fixture directory failed: %v; output=%s", err, output)
	}
}

func codexUntrustedWorkingDirectoryCommandResult() platformprocess.CommandResult {
	return platformprocess.CommandResult{
		ExitCode: codexUntrustedWorkingDirectoryExitCode,
		Stderr:   []byte(codexUntrustedWorkingDirectoryStderr),
	}
}
