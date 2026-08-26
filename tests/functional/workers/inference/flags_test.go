package inference_test

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	flagsTestWorktreeName = "flags-feature-branch"
	flagsTestModel        = "claude-flags-test-model"
)

// TestProviderPermissionWorktreeAndModelFlagsMapToCommand proves that when a
// worker/workstation configuration supplies skip-permissions policy, a resolved
// worktree name, and an explicit model for a selected provider, root.BuildProcess
// dispatch maps those settings onto the provider-process command args for that
// provider instead of silently dropping them or routing through a different edge.
func TestProviderPermissionWorktreeAndModelFlagsMapToCommand(t *testing.T) {
	t.Parallel()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "worktree_passthrough"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       flagsTestWorktreeName,
		WorkID:     "work-flags-map",
		WorkTypeID: "task",
		TraceID:    "trace-flags-map",
		Payload:    []byte(`{"title":"provider permission worktree model flags"}`),
	})

	workerConfig := strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderClaude, flagsTestModel),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	)
	support.WriteAgentConfig(t, dir, "worker-a", workerConfig)

	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(claudeFlagsSuccessStdout)},
	)

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want exactly one provider-process edge", runner.CallCount())
	}

	call := runner.LastRequest()
	if call.Command != string(modelprovider.ProviderClaude) {
		t.Fatalf("command = %q, want selected provider %q", call.Command, modelprovider.ProviderClaude)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--dangerously-skip-permissions"})
	support.AssertArgsContainSequence(t, call.Args, []string{"--worktree", flagsTestWorktreeName})
	support.AssertArgsContainSequence(t, call.Args, []string{"--model", flagsTestModel})
}

// TestUnsupportedProviderFlagReturnsCapabilityError proves that when a
// worker/workstation configuration requests a provider flag the selected
// provider does not support, root.BuildProcess rejects the dispatch with a
// capability error before starting a provider command instead of launching a
// live provider-process edge for that work.
func TestUnsupportedProviderFlagReturnsCapabilityError(t *testing.T) {
	t.Parallel()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))

	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderClaude,
		"claude-test-model",
	))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
outputSchema: '{}'
---
Test workstation.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"unsupported provider flag"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("provider must not run"),
	})

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work tokens = %d, want 1 unsupported-capability failure; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work tokens = %d, want 0", got)
	}
	if runner.CallCount() != 0 {
		t.Fatalf(
			"provider command runner calls = %d, want no provider-process edge before capability rejection",
			runner.CallCount(),
		)
	}
}

const claudeFlagsSuccessStdout = `{"type":"stream_event","session_id":"session-flags-map","event":{"type":"message_start","message":{"id":"msg-flags-map","role":"assistant","content":[]}}}` + "\n" +
	`{"type":"stream_event","session_id":"session-flags-map","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}` + "\n" +
	`{"type":"stream_event","session_id":"session-flags-map","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Done. COMPLETE"}}}` + "\n" +
	`{"type":"stream_event","session_id":"session-flags-map","event":{"type":"content_block_stop","index":0}}` + "\n" +
	`{"type":"stream_event","session_id":"session-flags-map","event":{"type":"message_stop"}}` + "\n" +
	`{"type":"assistant","session_id":"session-flags-map","message":{"id":"msg-flags-map","role":"assistant","content":[{"type":"text","text":"Done. COMPLETE"}]}}` + "\n" +
	`{"type":"result","subtype":"success","is_error":false,"result":"Done. COMPLETE","session_id":"session-flags-map"}` + "\n"
