package script_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	declaredScriptEnvName  = "FACTORY_SCRIPT_ENV"
	declaredScriptEnvValue = "declared-value"
	undeclaredHostEnvName  = "SCRIPT_ENV_LEAK_PROBE"
	undeclaredHostEnvValue = "must-not-reach-script-command"
)

func newScriptSharedEnvironmentScenarios(t *testing.T) []scriptSharedScenario {
	t.Helper()
	t.Setenv(undeclaredHostEnvName, undeclaredHostEnvValue)

	environmentDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	updateScriptWorkstationEnv(t, environmentDir, map[string]string{
		declaredScriptEnvName: declaredScriptEnvValue,
	})

	missingDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	worktreeDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "worktree_passthrough"))
	support.WriteAgentConfig(t, worktreeDir, "worker-a", `---
type: MODEL_WORKER
model: test-model
modelProvider: claude
stopToken: COMPLETE
---
Process the input task.
`)

	return []scriptSharedScenario{
		{
			name:               "DeclaredEnvironmentOnly",
			factoryDir:         environmentDir,
			workName:           "shared-script-declared-environment",
			traceID:            "shared-script-declared-environment-trace",
			workTypeName:       "task",
			terminalState:      "done",
			expectedOutput:     "script-output-ok",
			expectedOutcome:    factoryapi.WorkOutcomeAccepted,
			commandKind:        scriptSharedScriptCommand,
			expectedCommand:    "echo",
			expectedArgs:       []string{"default-output"},
			environmentPrivacy: true,
			runner: newScriptSharedCommandRunner(
				support.NewRecordingCommandRunner("script-output-ok"),
			),
			assertResult: assertScriptSharedDeclaredEnvironment,
		},
		{
			name:            "MissingExecutable",
			factoryDir:      missingDir,
			workName:        "shared-script-missing-executable",
			traceID:         "shared-script-missing-executable-trace",
			workTypeName:    "task",
			terminalState:   "failed",
			expectedOutcome: factoryapi.WorkOutcomeFailed,
			commandKind:     scriptSharedScriptCommand,
			expectedCommand: "echo",
			expectedArgs:    []string{"default-output"},
			runner:          newScriptSharedCommandRunner(missingExecutableCommandRunner{}),
			assertResult:    assertScriptSharedMissingExecutable,
		},
		{
			name:            "WorktreePassthrough",
			factoryDir:      worktreeDir,
			workName:        "my-feature-branch",
			traceID:         "shared-script-worktree-trace",
			workTypeName:    "task",
			terminalState:   "complete",
			expectedOutcome: factoryapi.WorkOutcomeAccepted,
			commandKind:     scriptSharedProviderCommand,
			expectedCommand: string(modelprovider.ProviderClaude),
			expectedArgSequences: [][]string{
				{"--worktree", "my-feature-branch"},
				{"--model", "test-model"},
				{"--output-format", "stream-json", "--include-partial-messages"},
			},
			requireEmptyStdin: true,
			runner: newScriptSharedCommandRunner(testutil.NewProviderCommandRunner(
				platformprocess.CommandResult{Stdout: []byte(sharedScriptWorktreeProviderOutput())},
			)),
		},
	}
}

func assertScriptSharedDeclaredEnvironment(
	t *testing.T,
	_ *scriptSharedSpineFixture,
	scenario scriptSharedScenario,
	_ string,
	_ factoryapi.SubmitWorkResponse,
	_ factoryapi.ListWorkResponse,
	_ []factoryapi.FactoryEvent,
) {
	t.Helper()
	requests := scenario.runner.Requests()
	if len(requests) != 1 {
		t.Fatalf("declared environment command calls = %d, want one", len(requests))
	}
	env := requests[0].Env
	if !envContains(env, declaredScriptEnvName+"="+declaredScriptEnvValue) {
		t.Fatalf("captured command env missing declared %s=%q in %v", declaredScriptEnvName, declaredScriptEnvValue, env)
	}
	if envContainsKey(env, undeclaredHostEnvName) {
		t.Fatalf("captured command env leaked undeclared host value %s in %v", undeclaredHostEnvName, env)
	}
}

func assertScriptSharedMissingExecutable(
	t *testing.T,
	_ *scriptSharedSpineFixture,
	_ scriptSharedScenario,
	_ string,
	_ factoryapi.SubmitWorkResponse,
	_ factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	assertScriptMissingExecutableDispatchFailure(t, events)
}

func sharedScriptWorktreeProviderOutput() string {
	return `{"type":"stream_event","session_id":"session-worktree","event":{"type":"message_start","message":{"id":"msg-worktree","role":"assistant","content":[]}}}` + "\n" +
		`{"type":"stream_event","session_id":"session-worktree","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}` + "\n" +
		`{"type":"stream_event","session_id":"session-worktree","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Done. COMPLETE"}}}` + "\n" +
		`{"type":"stream_event","session_id":"session-worktree","event":{"type":"content_block_stop","index":0}}` + "\n" +
		`{"type":"stream_event","session_id":"session-worktree","event":{"type":"message_stop"}}` + "\n" +
		`{"type":"assistant","session_id":"session-worktree","message":{"id":"msg-worktree","role":"assistant","content":[{"type":"text","text":"Done. COMPLETE"}]}}` + "\n" +
		`{"type":"result","subtype":"success","is_error":false,"result":"Done. COMPLETE","session_id":"session-worktree"}` + "\n"
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
