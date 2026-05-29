package providers

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCodexWorktreeWorkstationDispatch_MaterializesCheckoutAndOmitsCLIWorktreeFlag(t *testing.T) {
	cases := []struct {
		name               string
		seedClaudeWorktrees bool
		wantParentRel      string
	}{
		{
			name:          "DefaultDotWorktreesParent",
			wantParentRel: ".worktrees",
		},
		{
			name:                "ExistingClaudeWorktreesParent",
			seedClaudeWorktrees: true,
			wantParentRel:       ".claude/worktrees",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoRoot := initGitRepositoryForCodexWorktreeFunctionalTest(t)
			factoryDir := filepath.Join(repoRoot, "factory")
			if err := os.MkdirAll(factoryDir, 0o755); err != nil {
				t.Fatalf("create factory dir: %v", err)
			}
			if tc.seedClaudeWorktrees {
				if err := os.MkdirAll(filepath.Join(factoryDir, ".claude", "worktrees"), 0o755); err != nil {
					t.Fatalf("seed .claude/worktrees: %v", err)
				}
			}

			writeCodexWorktreeFactoryConfig(t, factoryDir)
			support.WriteAgentConfig(t, factoryDir, "worker-a", `---
type: MODEL_WORKER
modelProvider: codex
model: test-model
stopToken: COMPLETE
---
Process the input task.
`)
			writeCodexWorktreeWorkstationAgents(t, factoryDir)

			workName := "codex-worktree-feature"
			testutil.WriteSeedRequest(t, factoryDir, interfaces.SubmitRequest{
				Name:       workName,
				WorkID:     "work-codex-worktree",
				WorkTypeID: "task",
				TraceID:    "trace-codex-worktree",
				Payload:    []byte("codex worktree workstation payload"),
			})

			runner := testutil.NewProviderCommandRunner(
				workers.CommandResult{Stdout: []byte("Done. COMPLETE")},
			)
			h := testutil.NewServiceTestHarness(t, factoryDir,
				testutil.WithProviderCommandRunner(runner),
				testutil.WithFullWorkerPoolAndScriptWrap(),
			)

			h.RunUntilComplete(t, 15*time.Second)

			h.Assert().
				HasTokenInPlace("task:complete").
				HasNoTokenInPlace("task:init").
				HasNoTokenInPlace("task:failed").
				TokenCount(1)

			if runner.CallCount() != 1 {
				t.Fatalf("provider runner call count = %d, want 1", runner.CallCount())
			}

			wantCheckout := filepath.Join(factoryDir, filepath.FromSlash(tc.wantParentRel), workName)
			call := runner.LastRequest()
			if call.Command != string(interfaces.ModelProviderCodex) {
				t.Fatalf("command = %q, want %q", call.Command, interfaces.ModelProviderCodex)
			}
			if call.WorkDir != wantCheckout {
				t.Fatalf("work dir = %q, want materialized checkout %q", call.WorkDir, wantCheckout)
			}
			assertArgsDoNotContain(t, call.Args, "--worktree")
			support.AssertArgsContainSequence(t, call.Args, []string{"exec", "--model", "test-model", "-"})

			if _, err := os.Stat(wantCheckout); err != nil {
				t.Fatalf("materialized checkout missing at %s: %v", wantCheckout, err)
			}

			events, err := h.GetFactoryEvents(context.Background())
			if err != nil {
				t.Fatalf("GetFactoryEvents: %v", err)
			}
			request := requireInferenceRequestEvent(t, events)
			if request.Worktree != workName {
				t.Fatalf("inference request worktree = %q, want %q", request.Worktree, workName)
			}
			if request.WorkingDirectory != wantCheckout {
				t.Fatalf("inference request workingDirectory = %q, want %q", request.WorkingDirectory, wantCheckout)
			}
		})
	}
}

func writeCodexWorktreeWorkstationAgents(t *testing.T, factoryDir string) {
	t.Helper()

	path := filepath.Join(factoryDir, "workstations", "process", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKSTATION\n---\nProcess {{ (index .Inputs 0).Name }}.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func writeCodexWorktreeFactoryConfig(t *testing.T, factoryDir string) {
	t.Helper()

	config := `{
  "name": "codex_worktree_workstation",
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
      "worker": "worker-a",
      "inputs": [{ "workType": "task", "state": "init" }],
      "outputs": [{ "workType": "task", "state": "complete" }],
      "onFailure": [{ "workType": "task", "state": "failed" }],
      "worktree": "{{ (index .Inputs 0).Name }}"
    }
  ]
}
`
	path := filepath.Join(factoryDir, "factory.json")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func initGitRepositoryForCodexWorktreeFunctionalTest(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH")
	}

	repoRoot := t.TempDir()
	runGitForCodexWorktreeFunctionalTest(t, repoRoot, "init")
	runGitForCodexWorktreeFunctionalTest(t, repoRoot, "config", "user.email", "codex-worktree-functional@example.com")
	runGitForCodexWorktreeFunctionalTest(t, repoRoot, "config", "user.name", "codex worktree functional")
	runGitForCodexWorktreeFunctionalTest(t, repoRoot, "commit", "--allow-empty", "-m", "init")
	return repoRoot
}

func runGitForCodexWorktreeFunctionalTest(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, output)
	}
}

func assertArgsDoNotContain(t *testing.T, args []string, forbidden ...string) {
	t.Helper()

	for _, arg := range args {
		for _, item := range forbidden {
			if arg == item {
				t.Fatalf("args = %#v, want to omit %q", args, item)
			}
		}
	}
}

func requireInferenceRequestEvent(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.InferenceRequestEventPayload {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceRequest {
			continue
		}
		payload, err := event.Payload.AsInferenceRequestEventPayload()
		if err != nil {
			t.Fatalf("decode inference request payload: %v", err)
		}
		return payload
	}
	t.Fatalf("events missing %s: %v", factoryapi.FactoryEventTypeInferenceRequest, eventTypes(events))
	return factoryapi.InferenceRequestEventPayload{}
}

func eventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}
