package opencode_test

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

// TestOpenCodeConductorSuccessThroughRootBuildProcess proves successful OpenCode execution through the product graph.
func TestOpenCodeConductorSuccessThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderOpenCode,
		"openai/gpt-5",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"opencode conductor success"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(
			`{"type":"step_start","sessionID":"session-functional"}` + "\n" +
				`{"type":"text","sessionID":"session-functional","part":{"id":"message-1","text":"opencode functional answer COMPLETE","time":{"end":1}}}` + "\n",
		),
	})

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("opencode command runner calls = %d, want 1 through conductor path", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "opencode" {
		t.Fatalf("command = %q, want opencode", request.Command)
	}
}

// TestOpenCodeCommandCancellationThroughRootBuildProcessIsCanonical proves cancellation returns the canonical outcome.
func TestOpenCodeCommandCancellationThroughRootBuildProcessIsCanonical(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderOpenCode,
		"openai/gpt-5",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"opencode command cancel"}`))

	runner := &commandCancellationRunner{}
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1; listed=%#v", got, listed)
	}
	if runner.calls != 1 {
		t.Fatalf("opencode command runner calls = %d, want 1", runner.calls)
	}
	if runner.lastRequest.Command != "opencode" {
		t.Fatalf("command = %q, want opencode", runner.lastRequest.Command)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal factory events: %v", err)
	}
	payload := string(encoded)
	if !strings.Contains(payload, "provider invocation was canceled") {
		t.Fatalf("factory events missing canonical cancellation outcome: %s", payload)
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
