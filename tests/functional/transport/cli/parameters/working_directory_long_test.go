//go:build functionallong

package parameters_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLIProviderExecResolvesWorkdirAgainstFactoryRuntimeRoot proves the public
// CLI daemon resolves workstation-relative provider working directories against
// the Factory runtime root when dispatching through root.BuildProcess and
// ProviderCommandRunner edge capture.
func TestCLIProviderExecResolvesWorkdirAgainstFactoryRuntimeRoot(t *testing.T) {
	support.SkipLongFunctional(t, "slow relative-working-directory runtime-root sweep")

	factoryDir := filepath.Join(t.TempDir(), "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create factory dir: %v", err)
	}

	writeRelativeWorkingDirectoryFactoryConfig(t, factoryDir)
	support.WriteAgentConfig(t, factoryDir, "worker-a", `---
type: MODEL_WORKER
modelProvider: codex
executorProvider: script_wrap
skipPermissions: true
stopToken: COMPLETE
---
	Process the input task.
`)
	support.WriteWorkstationConfig(t, factoryDir, "process", `---
type: MODEL_WORKSTATION
---

Process {{ (index .Inputs 0).Name }} from the current working directory.
`)

	workName := "relative-working-directory-branch"
	expectedWorkDir := filepath.Join(factoryDir, ".claude", "worktrees", workName)
	if err := os.MkdirAll(expectedWorkDir, 0o755); err != nil {
		t.Fatalf("create expected work dir: %v", err)
	}

	testutil.WriteSeedRequest(t, factoryDir, work.SubmitRequest{
		Name:       workName,
		WorkID:     "work-relative-working-directory",
		WorkTypeID: "task",
		TraceID:    "trace-relative-working-directory",
		Payload:    []byte("relative working directory payload"),
	})

	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(
			`{"type":"item.completed","item":{"id":"relative-working-directory","type":"agent_message","text":"Done. COMPLETE"}}` + "\n",
		)},
	)

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, factoryDir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	for placeID, want := range map[string]int{
		"task:complete": 1,
		"task:init":     0,
		"task:failed":   0,
	} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}

	if runner.CallCount() != 1 {
		t.Fatalf("provider runner calls = %d, want 1", runner.CallCount())
	}

	req := runner.LastRequest()
	if req.Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("command = %q, want %q", req.Command, modelprovider.ProviderCodex)
	}
	support.AssertArgsContainSequence(t, req.Args, []string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "-"})
	if req.WorkDir != expectedWorkDir {
		t.Fatalf("work dir = %q, want %q", req.WorkDir, expectedWorkDir)
	}
	if string(req.Stdin) == "" {
		t.Fatal("expected Codex request prompt to be sent over stdin")
	}
}

func writeRelativeWorkingDirectoryFactoryConfig(t *testing.T, factoryDir string) {
	t.Helper()

	config := `{
  "name": "factory",
  "workTypes": [
    {
      "name": "task",
      "states": [
        { "name": "init", "type": "INITIAL" },
        { "name": "complete", "type": "TERMINAL" },
        { "name": "failed", "type": "FAILED" }
      ]
    }
  ],
  "workers": [
    { "name": "worker-a" }
  ],
  "workstations": [
    {
      "name": "process",
      "behavior": "STANDARD",
      "worker": "worker-a",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "workingDirectory": ".claude/worktrees/{{ (index .Inputs 0).Name }}",
      "worktree": "{{ (index .Inputs 0).Name }}"
    }
  ]
}
`
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(config), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}
