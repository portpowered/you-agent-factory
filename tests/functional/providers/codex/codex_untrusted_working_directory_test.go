package codex

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
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

func assertCodexSharedActionableRefusal(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	workID, _, events := runCodexSharedRefusal(
		t,
		fixture,
		fixture.actionableFactoryDir,
		codexSharedActionableWorkName,
		"shared actionable Codex refusal",
	)
	assertCodexSharedRefusalDispatch(
		t,
		events,
		workID,
		[]string{
			fixture.actionableFactoryDir,
			"Codex requires a trusted working directory",
			"suitable trusted Git repository",
		},
		nil,
	)
}

func assertCodexSharedNeutralRefusal(t *testing.T, fixture *codexSharedProcessFixture) {
	t.Helper()
	workID, _, events := runCodexSharedRefusal(
		t,
		fixture,
		fixture.neutralFactoryDir,
		codexSharedNeutralWorkName,
		"shared neutral Codex refusal",
	)
	assertCodexSharedRefusalDispatch(
		t,
		events,
		workID,
		[]string{"provider rejected the execution request"},
		[]string{
			fixture.neutralFactoryDir,
			"future refusal",
			"credential=secret",
			"codex exited",
		},
	)
}

func runCodexSharedRefusal(
	t *testing.T,
	fixture *codexSharedProcessFixture,
	factoryDir, workName, title string,
) (string, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()
	sessionID := fixture.openSession(t, factoryDir)
	name := workName
	submitted := support.SubmitSessionWorkAt(t, fixture.baseURL, sessionID, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": title},
	})
	if submitted.SessionId == nil || *submitted.SessionId != sessionID {
		t.Fatalf("refusal Work session ID = %#v, want %q", submitted.SessionId, sessionID)
	}
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" || strings.TrimSpace(submitted.RequestId) == "" {
		t.Fatalf("refusal Work identity = work:%q request:%q, want both identities", workID, submitted.RequestId)
	}
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, sessionID, codexSharedFixtureTimeout)

	listed := listCodexSessionWork(t, fixture.baseURL, sessionID)
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("shared refusal failed Work count = %d, want one; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:init"); got != 0 {
		t.Fatalf("shared refusal init Work count = %d, want zero; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 0 {
		t.Fatalf("shared refusal complete Work count = %d, want zero; listed=%#v", got, listed)
	}
	requests := fixture.commandRunner.RequestsForWorkDir(factoryDir)
	if len(requests) != 1 || requests[0].Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("shared refusal Codex requests for %q = %#v, want one Codex request", factoryDir, requests)
	}
	events := support.GetFactoryEventsForSessionAt(t, fixture.baseURL, sessionID)
	support.AssertSingleWorkRequestEvent(t, events, submitted.RequestId, workID, "task")
	return workID, listed, events
}

func assertCodexSharedRefusalDispatch(
	t testing.TB,
	events []factoryapi.FactoryEvent,
	workID string,
	required, forbidden []string,
) {
	t.Helper()
	processFailures := 0
	for _, dispatch := range support.ObserveDispatchEvents(t, events) {
		if dispatch.Request.TransitionId != "process" ||
			!support.DispatchObservationIncludesWork(dispatch, workID) ||
			dispatch.Response == nil {
			continue
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeFailed {
			t.Errorf("shared refusal process response outcome = %q, want FAILED", dispatch.Response.Outcome)
		}
		if dispatch.Response.Error == nil {
			t.Error("shared refusal process response error = nil, want provider refusal diagnostic")
		} else {
			for _, needle := range required {
				if !strings.Contains(*dispatch.Response.Error, needle) {
					t.Errorf("shared refusal process error = %q, want it to contain %q", *dispatch.Response.Error, needle)
				}
			}
			for _, needle := range forbidden {
				if strings.Contains(*dispatch.Response.Error, needle) {
					t.Errorf("shared refusal process error = %q, must not expose %q", *dispatch.Response.Error, needle)
				}
			}
		}
		if dispatch.Response.FailureDetail == nil ||
			dispatch.Response.FailureDetail.Reason != factoryapi.WorkFailureTypePermanentBadRequest {
			t.Errorf("shared refusal process failure detail = %#v, want permanent bad request", dispatch.Response.FailureDetail)
		}
		processFailures++
	}
	if processFailures != 1 {
		t.Fatalf("shared refusal failed process dispatches = %d, want one without retries", processFailures)
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

func codexNeutralRefusalCommandResult() platformprocess.CommandResult {
	// A structured provider error is deliberately unknown to this adapter. The
	// adapter must retain the provider-declared refusal class while exposing a
	// neutral public message instead of this raw credential-like fixture text.
	return platformprocess.CommandResult{
		ExitCode: 77,
		Stderr:   []byte(`{"type":"error","message":"future refusal: credential=secret"}`),
	}
}
