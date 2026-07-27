package script_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	declaredScriptEnvName  = "FACTORY_SCRIPT_ENV"
	declaredScriptEnvValue = "declared-value"
	undeclaredHostEnvName  = "SCRIPT_ENV_LEAK_PROBE"
	undeclaredHostEnvValue = "must-not-reach-script-command"
)

// TestScriptWorkerReceivesDeclaredEnvironmentOnly proves a root-built script
// worker passes only Factory-declared environment values to the external command
// edge and does not leak planted undeclared host environment material.
func TestScriptWorkerReceivesDeclaredEnvironmentOnly(t *testing.T) {
	t.Setenv(undeclaredHostEnvName, undeclaredHostEnvValue)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	updateScriptWorkstationEnv(t, dir, map[string]string{
		declaredScriptEnvName: declaredScriptEnvValue,
	})
	testutil.WriteSeedFile(t, dir, "task", []byte("environment-boundary-input"))

	runner := support.NewRecordingCommandRunner("script-output-ok")
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ScriptCommandRunner: runner},
		10*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work tokens = %d, want 1 successful script dispatch", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("script command calls = %d, want exactly one external command effect", runner.CallCount())
	}

	env := runner.LastRequest().Env
	if !envContains(env, declaredScriptEnvName+"="+declaredScriptEnvValue) {
		t.Fatalf("captured command env missing declared %s=%q in %v", declaredScriptEnvName, declaredScriptEnvValue, env)
	}
	if envContainsKey(env, undeclaredHostEnvName) {
		t.Fatalf("captured command env leaked undeclared host value %s in %v", undeclaredHostEnvName, env)
	}
}

// TestScriptWorkerMissingExecutableFailsActionably proves a root-built script
// worker whose external command cannot be started reports an actionable public
// failure instead of succeeding or hanging without a customer-visible outcome.
func TestScriptWorkerMissingExecutableFailsActionably(t *testing.T) {
	t.Setenv(undeclaredHostEnvName, undeclaredHostEnvValue)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("missing-executable-input"))

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ScriptCommandRunner: missingExecutableCommandRunner{}},
		10*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work tokens = %d, want 0 successful script dispatch", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work tokens = %d, want 1 actionable script failure", got)
	}
	assertScriptMissingExecutableDispatchFailure(t, events)
}

// TestScriptWorkerWorktreePassthrough proves a root-built workstation worktree
// template resolves from Work name and reaches the external provider command edge.
func TestScriptWorkerWorktreePassthrough(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "worktree_passthrough"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "my-feature-branch",
		WorkID:     "work-wt-001",
		WorkTypeID: "task",
		TraceID:    "trace-wt-test",
		Payload:    []byte("worktree test payload"),
	})

	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
Process the input task.
`)
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(
			`{"type":"stream_event","session_id":"session-worktree","event":{"type":"message_start","message":{"id":"msg-worktree","role":"assistant","content":[]}}}` + "\n" +
				`{"type":"stream_event","session_id":"session-worktree","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}` + "\n" +
				`{"type":"stream_event","session_id":"session-worktree","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Done. COMPLETE"}}}` + "\n" +
				`{"type":"stream_event","session_id":"session-worktree","event":{"type":"content_block_stop","index":0}}` + "\n" +
				`{"type":"stream_event","session_id":"session-worktree","event":{"type":"message_stop"}}` + "\n" +
				`{"type":"assistant","session_id":"session-worktree","message":{"id":"msg-worktree","role":"assistant","content":[{"type":"text","text":"Done. COMPLETE"}]}}` + "\n" +
				`{"type":"result","subtype":"success","is_error":false,"result":"Done. COMPLETE","session_id":"session-worktree"}` + "\n",
		)},
	)

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 15*time.Second)
	assertProviderWorkCompleted(t, listed)

	if runner.CallCount() != 1 {
		t.Fatalf("provider runner call count = %d, want 1", runner.CallCount())
	}
	call := runner.LastRequest()
	if call.Command != string(modelprovider.ProviderClaude) {
		t.Fatalf("command = %q, want %q", modelprovider.ProviderClaude, call.Command)
	}
	support.AssertArgsContainSequence(t, call.Args, []string{"--worktree", "my-feature-branch"})
	support.AssertArgsContainSequence(t, call.Args, []string{"--model", "test-model"})
	support.AssertArgsContainSequence(t, call.Args, []string{"--output-format", "stream-json", "--include-partial-messages"})
	if len(call.Stdin) != 0 {
		t.Fatalf("Claude prompt stayed in args, got stdin %q", string(call.Stdin))
	}
}

type missingExecutableCommandRunner struct{}

func (missingExecutableCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, exec.ErrNotFound
}

func assertScriptMissingExecutableDispatchFailure(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) == 0 {
		t.Fatal("factory events missing dispatch observations")
	}
	response := dispatches[len(dispatches)-1].Response
	if response == nil {
		t.Fatal("dispatch response missing for failed script execution")
	}
	if response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want FAILED", response.Outcome)
	}
	if response.FailureDetail == nil {
		t.Fatal("dispatch FailureDetail missing for missing executable")
	}
	if response.FailureDetail.Reason != factoryapi.WorkFailureTypeMissingExecutable {
		t.Fatalf(
			"failure reason = %q, want %q",
			response.FailureDetail.Reason,
			factoryapi.WorkFailureTypeMissingExecutable,
		)
	}
	message := strings.ToLower(response.FailureDetail.Message)
	if !strings.Contains(message, "executable") && !strings.Contains(message, "could not") {
		t.Fatalf("failure message %q does not identify missing executable", response.FailureDetail.Message)
	}
	if strings.Contains(message, undeclaredHostEnvValue) {
		t.Fatalf("failure message leaked undeclared host environment value %q", undeclaredHostEnvValue)
	}
	if response.Error != nil && strings.Contains(*response.Error, undeclaredHostEnvValue) {
		t.Fatalf("dispatch error leaked undeclared host environment value %q", undeclaredHostEnvValue)
	}
}

func updateScriptWorkstationEnv(t *testing.T, dir string, env map[string]string) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}
	workstations, ok := cfg["workstations"].([]any)
	if !ok || len(workstations) == 0 {
		t.Fatal("factory.json missing workstations")
	}
	workstation, ok := workstations[0].(map[string]any)
	if !ok {
		t.Fatal("factory.json workstation entry has unexpected shape")
	}
	workstation["env"] = env
	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func envContains(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

func envContainsKey(env []string, name string) bool {
	prefix := name + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}
