package codex_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCodexConductorSuccessThroughRootBuildProcess proves successful Codex execution through the product graph.
func TestCodexConductorSuccessThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"gpt-5.3-codex-spark",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"codex conductor success"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(
			`{"type":"item.completed","item":{"id":"message-final","type":"agent_message","text":"codex functional answer COMPLETE"}}` + "\n",
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
		t.Fatalf("codex command runner calls = %d, want 1 through conductor path", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "codex" {
		t.Fatalf("command = %q, want codex (conductor-selected built-in)", request.Command)
	}
	if !containsArgPair(request.Args, "--model", "gpt-5.3-codex-spark") {
		t.Fatalf("args = %#v, want --model gpt-5.3-codex-spark", request.Args)
	}
	if !containsArg(request.Args, "exec") || !containsArg(request.Args, "--json") {
		t.Fatalf("args = %#v, want codex exec --json streaming invocation", request.Args)
	}
}

func containsArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}

// TestCodexCommandCancellationThroughRootBuildProcessIsCanonical proves cancellation returns the canonical outcome.
func TestCodexCommandCancellationThroughRootBuildProcessIsCanonical(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"gpt-5.3-codex-spark",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"codex command cancel"}`))

	runner := &commandCancellationRunner{}
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1; listed=%#v", got, listed)
	}
	if runner.calls != 1 {
		t.Fatalf("codex command runner calls = %d, want 1", runner.calls)
	}
	request := runner.lastRequest
	if request.Command != "codex" {
		t.Fatalf("command = %q, want codex (conductor-selected built-in)", request.Command)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal factory events: %v", err)
	}
	payload := string(encoded)
	if !strings.Contains(payload, "provider invocation was canceled") {
		t.Fatalf("factory events missing canonical cancellation outcome: %s", payload)
	}
	if strings.Contains(payload, "Codex command did not complete successfully") {
		t.Fatalf("factory events used Codex-local cancellation fallback: %s", payload)
	}
}

type commandCancellationRunner struct {
	calls       int
	lastRequest platformprocess.CommandRequest
}

func (r *commandCancellationRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls++
	r.lastRequest = request
	return platformprocess.CommandResult{}, context.Canceled
}
