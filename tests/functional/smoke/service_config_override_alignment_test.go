package smoke

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestServiceConfigOverrideAlignment_ServiceHarnessScriptCommandRunner(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("script harness alignment"))

	runner := support.NewRecordingCommandRunner("script alignment output")
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
	)

	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		HasTokenInPlace("task:done").
		HasNoTokenInPlace("task:failed")
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("script command runner calls = %d, want 1", got)
	}
}

func TestServiceConfigOverrideAlignment_ServiceHarnessSharesScriptAndProviderCommandRunner(t *testing.T) {
	cfg := twoStageServicePipelineConfig()
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["workingDirectory"] = "/tmp/script-command-smoke"
	workstations[0]["env"] = map[string]any{"SCRIPT_ENV": "script-value"}
	workstations[1]["workingDirectory"] = "/tmp/provider-command-smoke"
	workstations[1]["env"] = map[string]any{"PROVIDER_ENV": "provider-value"}

	dir := support.ScaffoldFactory(t, cfg)
	support.SetWorkingDirectory(t, dir)
	support.WriteWorkstationConfig(t, dir, "step-two", `---
type: MODEL_WORKSTATION
---
Provider received {{ (index .Inputs 0).Payload }}.
`)
	support.WriteAgentConfig(t, dir, "worker-a", `---
type: SCRIPT_WORKER
command: script-tool
args:
  - "{{ (index .Inputs 0).WorkID }}"
  - "{{ (index .Inputs 0).Payload }}"
---
`)
	support.WriteAgentConfig(t, dir, "worker-b", support.BuildModelWorkerConfig(workers.ModelProviderCodex, "gpt-5-codex"))
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "mixed-command-smoke-work",
		WorkTypeID: "task",
		TraceID:    "trace-mixed-command-smoke",
		Payload:    []byte("script-input"),
	})

	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("script-output")},
		workers.CommandResult{
			Stdout: []byte("provider-output COMPLETE"),
			Stderr: []byte(`{"event":"session.created","session_id":"sess_mixed_command"}`),
		},
	)
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
		testutil.WithProviderCommandRunner(runner),
	)

	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:failed")
	if got := support.PlaceTokenCount(*harness.Marking(), "task:complete"); got != 1 {
		t.Fatalf("completed token count = %d, want 1", got)
	}

	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("shared command runner request count = %d, want 2", len(requests))
	}

	scriptReq := requests[0]
	if scriptReq.Command != "script-tool" {
		t.Fatalf("script command = %q, want %q", scriptReq.Command, "script-tool")
	}
	if !reflect.DeepEqual(scriptReq.Args, []string{"mixed-command-smoke-work", "script-input"}) {
		t.Fatalf("script args = %v, want rendered work ID and payload args", scriptReq.Args)
	}
	if scriptReq.WorkDir != support.ResolvedRuntimePath(dir, "/tmp/script-command-smoke") {
		t.Fatalf("script work dir = %q, want %q", scriptReq.WorkDir, support.ResolvedRuntimePath(dir, "/tmp/script-command-smoke"))
	}
	if !containsEnv(scriptReq.Env, "SCRIPT_ENV=script-value") {
		t.Fatalf("script env missing SCRIPT_ENV in %v", scriptReq.Env)
	}
	if len(scriptReq.Stdin) != 0 {
		t.Fatalf("script stdin = %q, want empty stdin", string(scriptReq.Stdin))
	}
	if !containsString(scriptReq.Execution.WorkIDs, "mixed-command-smoke-work") {
		t.Fatalf("script execution work IDs = %v, want mixed-command-smoke-work", scriptReq.Execution.WorkIDs)
	}

	providerReq := requests[1]
	if providerReq.Command != string(workers.ModelProviderCodex) {
		t.Fatalf("provider command = %q, want %q", providerReq.Command, workers.ModelProviderCodex)
	}
	assertArgsContainSequence(t, providerReq.Args, []string{"exec"})
	assertArgsContainSequence(t, providerReq.Args, []string{"--model", "gpt-5-codex"})
	if providerReq.Args[len(providerReq.Args)-1] != "-" {
		t.Fatalf("provider prompt placeholder = %q, want -", providerReq.Args[len(providerReq.Args)-1])
	}
	if !strings.Contains(string(providerReq.Stdin), "script-output") {
		t.Fatalf("provider stdin = %q, want it to include script output", string(providerReq.Stdin))
	}
	if providerReq.WorkDir != support.ResolvedRuntimePath(dir, "/tmp/provider-command-smoke") {
		t.Fatalf("provider work dir = %q, want %q", providerReq.WorkDir, support.ResolvedRuntimePath(dir, "/tmp/provider-command-smoke"))
	}
	if !containsEnv(providerReq.Env, "PROVIDER_ENV=provider-value") {
		t.Fatalf("provider env missing PROVIDER_ENV in %v", providerReq.Env)
	}
	if !containsString(providerReq.Execution.WorkIDs, "mixed-command-smoke-work") {
		t.Fatalf("provider execution work IDs = %v, want mixed-command-smoke-work", providerReq.Execution.WorkIDs)
	}
}

func assertArgsContainSequence(t *testing.T, args []string, want []string) {
	t.Helper()

	if len(want) == 0 {
		return
	}
	for start := 0; start+len(want) <= len(args); start++ {
		if reflect.DeepEqual(args[start:start+len(want)], want) {
			return
		}
	}
	t.Fatalf("args = %v, want contiguous sequence %v", args, want)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
