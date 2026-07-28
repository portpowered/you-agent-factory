package claude_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestClaudeConductorSuccessThroughRootBuildProcess proves successful Claude execution through the product graph.
func TestClaudeConductorSuccessThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderClaude,
		"claude-sonnet-4-5-20250514",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"claude conductor success"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(
			`{"type":"result","subtype":"success","is_error":false,"result":"claude functional answer COMPLETE","session_id":"claude-session-1"}` + "\n",
		),
	})

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed place tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("claude command runner calls = %d, want 1 through conductor path", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "claude" {
		t.Fatalf("command = %q, want claude (conductor-selected built-in)", request.Command)
	}
	if !containsArgPair(request.Args, "--model", "claude-sonnet-4-5-20250514") {
		t.Fatalf("args = %#v, want --model claude-sonnet-4-5-20250514", request.Args)
	}
	if !containsArgPair(request.Args, "--output-format", "stream-json") {
		t.Fatalf("args = %#v, want claude stream-json invocation", request.Args)
	}
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}
