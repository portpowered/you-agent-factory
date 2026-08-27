package mock

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// testServiceConfigOverrideAlignmentCustomerProcess
// proves customer-process script and provider dispatches route through one shared
// replaced command-runner edge with correct per-workstation env and work dirs.
func testServiceConfigOverrideAlignmentCustomerProcess(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	dir := scaffoldSharedCommandRunnerFactory(t)
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("script-output")},
		platformprocess.CommandResult{
			// Protocol-valid Codex JSONL; plain text is a retryable decode fault.
			Stdout: support.CodexSuccessStdout("provider-output COMPLETE"),
			Stderr: []byte(`{"event":"session.created","session_id":"sess_mixed_command"}`),
		},
	)
	fixture.useCommandRunners(runner, runner)
	session := fixture.openSession(t, dir)
	_, _ = session.terminalObservations(t, 20*time.Second)
	defer session.closeAndAssertGone(t)

	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("shared command runner request count = %d, want 2", len(requests))
	}

	assertSharedCommandRunnerScriptRequest(t, dir, requests[0])
	assertSharedCommandRunnerProviderRequest(t, dir, requests[1])
}
