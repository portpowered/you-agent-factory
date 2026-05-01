package functional_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
)

func TestRootFactoryTemplate_CopiedFactoryRendersScriptAndScriptWrapCommandRequests(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, rootFactoryDir(t))
	setWorkingDirectory(t, dir)
	removeCopiedRootFactoryRuntimeDirs(t, dir)

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "root-factory-template-branch",
		WorkID:     "root-plan-work",
		WorkTypeID: "plan",
		TraceID:    "trace-root-factory-template",
		Payload:    []byte("root factory plan payload"),
	})

	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("workspace ready from script")},
		workers.CommandResult{Stdout: []byte("process accepted <COMPLETE>")},
		workers.CommandResult{Stdout: []byte("review accepted <COMPLETE>")},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
		testutil.WithProviderCommandRunner(runner),
	)

	h.RunUntilComplete(t, 60*time.Second)

	h.Assert().
		PlaceTokenCount("plan:complete", 1).
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("plan:init").
		HasNoTokenInPlace("plan:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:in-review").
		HasNoTokenInPlace("task:failed")

	requests := runner.Requests()
	if len(requests) != 3 {
		t.Fatalf("root factory command request count = %d, want 3", len(requests))
	}
	assertRootFactoryWorkspaceSetupRequest(t, requests[0])
	assertRootFactoryProcessRequest(t, dir, requests[1])
	assertRootFactoryReviewRequest(t, dir, requests[2])
}

func rootFactoryDir(t *testing.T) string {
	t.Helper()
	return testutil.MustRepoPath(t, "factory")
}

func removeCopiedRootFactoryRuntimeDirs(t *testing.T, dir string) {
	t.Helper()

	for _, name := range []string{"inputs", "logs"} {
		if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
			t.Fatalf("remove copied root factory %s dir: %v", name, err)
		}
	}
}

func assertRootFactoryWorkspaceSetupRequest(t *testing.T, req workers.CommandRequest) {
	t.Helper()

	if req.Command != "python" {
		t.Fatalf("workspace setup command = %q, want python", req.Command)
	}
	assertCommandArgs(t, req, []string{
		"factory/scripts/setup-workspace.py",
		"root-factory-template-branch",
	})
	if req.WorkDir != "" {
		t.Fatalf("workspace setup work dir = %q, want empty", req.WorkDir)
	}
	token := firstCommandRequestInputToken(t, req)
	if token.PlaceID != "plan:init" {
		t.Fatalf("workspace setup input place = %q, want plan:init", token.PlaceID)
	}
	if token.Color.Name != "root-factory-template-branch" {
		t.Fatalf("workspace setup input name = %q, want root-factory-template-branch", token.Color.Name)
	}
	if token.Color.Payload == nil || string(token.Color.Payload) != "root factory plan payload" {
		t.Fatalf("workspace setup input payload = %q, want root factory plan payload", string(token.Color.Payload))
	}
}

func assertRootFactoryProcessRequest(t *testing.T, dir string, req workers.CommandRequest) {
	t.Helper()

	assertRootFactoryProcessorRequest(t, req, rootFactoryProcessorWant{
		workstation:   "process",
		workDir:       filepath.Join(dir, ".claude", "worktrees", "root-factory-template-branch"),
		stdinContains: "You are an autonomous coding agent working on a software project.",
		inputPlace:    "task:init",
		inputName:     "root-factory-template-branch",
	})
}

func assertRootFactoryReviewRequest(t *testing.T, dir string, req workers.CommandRequest) {
	t.Helper()

	assertRootFactoryProcessorRequest(t, req, rootFactoryProcessorWant{
		workstation:   "review",
		workDir:       filepath.Join(dir, ".claude", "worktrees", "root-factory-template-branch"),
		stdinContains: "You are a code reviewer agent.",
		inputPlace:    "task:in-review",
		inputName:     "root-factory-template-branch",
	})
}

type rootFactoryProcessorWant struct {
	workstation   string
	workDir       string
	stdinContains string
	inputPlace    string
	inputName     string
}

func assertRootFactoryProcessorRequest(t *testing.T, req workers.CommandRequest, want rootFactoryProcessorWant) {
	t.Helper()

	if req.Command != string(workers.ModelProviderCodex) {
		t.Fatalf("%s command = %q, want codex", want.workstation, req.Command)
	}
	assertCommandArgs(t, req, []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "-"})
	if !strings.Contains(string(req.Stdin), want.stdinContains) {
		t.Fatalf("%s stdin = %q, want to contain %q", want.workstation, string(req.Stdin), want.stdinContains)
	}
	if req.WorkDir != want.workDir {
		t.Fatalf("%s work dir = %q, want %q", want.workstation, req.WorkDir, want.workDir)
	}
	if req.WorkstationName != want.workstation {
		t.Fatalf("%s dispatch workstation = %q, want %q", want.workstation, req.WorkstationName, want.workstation)
	}

	token := firstCommandRequestInputToken(t, req)
	if token.PlaceID != want.inputPlace {
		t.Fatalf("%s input place = %q, want %q", want.workstation, token.PlaceID, want.inputPlace)
	}
	if token.Color.Name != want.inputName {
		t.Fatalf("%s input name = %q, want %q", want.workstation, token.Color.Name, want.inputName)
	}
}
