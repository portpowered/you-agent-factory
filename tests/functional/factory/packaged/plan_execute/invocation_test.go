package planexecute

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var planExecuteWorkName = regexp.MustCompile(`tasks/todo/([^` + "`" + `]+)\.md`)

func TestPackagedPlanExecuteRunsPRDWorktreeReviewAndActualMerge(t *testing.T) {
	repository := initializePlanExecuteRepository(t)
	home := t.TempDir()
	support.InstallPackagedFactory(t, home, factorydefinitions.PackagedPlanExecuteFactoryName)
	runner := &planExecuteDeliveryRunner{repository: repository, branch: "feature/packaged-delivery"}

	args := []string{
		"you", "--json", "run", "--named", factorydefinitions.PackagedPlanExecuteFactoryName,
		"--provider", "CODEX", "--model", "gpt-5", "--branch-name", runner.branch,
		"--no-record", "--to", "Deliver the packaged flow",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = repository
	err := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner}).Execute(inputs.Input)
	if err != nil {
		t.Fatalf("plan-execute invocation error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response = %#v", response)
	}
	if content, err := os.ReadFile(filepath.Join(repository, "delivered.txt")); err != nil || string(content) != "delivered through isolated worktree\n" {
		t.Fatalf("merged delivery content = %q, error = %v", content, err)
	}
	if got := runPlanExecuteGit(t, repository, "log", "-1", "--pretty=%s"); got != "merge packaged delivery" {
		t.Fatalf("main tip = %q, want actual merge commit", got)
	}
	if calls := runner.Calls(); strings.Join(calls, ",") != "planner,executor,reviewer" {
		t.Fatalf("role calls = %v", calls)
	}
}

func TestPackagedPlanExecuteRepairsReviewConflictBeforeMerge(t *testing.T) {
	repository := initializePlanExecuteRepository(t)
	home := t.TempDir()
	support.InstallPackagedFactory(t, home, factorydefinitions.PackagedPlanExecuteFactoryName)
	runner := &planExecuteDeliveryRunner{
		repository: repository,
		branch:     "feature/conflict-repair",
		rejectOnce: true,
	}
	args := []string{
		"you", "--json", "run", "--named", factorydefinitions.PackagedPlanExecuteFactoryName,
		"--provider", "CODEX", "--model", "gpt-5", "--branch-name", runner.branch,
		"--no-record", "--to", "Repair conflict and merge",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = repository
	if err := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner}).Execute(inputs.Input); err != nil {
		t.Fatalf("plan-execute conflict invocation error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response = %#v", response)
	}
	if calls := strings.Join(runner.Calls(), ","); calls != "planner,executor,reviewer,executor,reviewer" {
		t.Fatalf("role calls = %s", calls)
	}
	if got := runPlanExecuteGit(t, repository, "diff", "--check", "HEAD^"); got != "" {
		t.Fatalf("post-merge CI diff check = %q", got)
	}
}

type planExecuteDeliveryRunner struct {
	mu         sync.Mutex
	repository string
	branch     string
	calls      []string
	rejectOnce bool
	executions int
	reviews    int
}

func (runner *planExecuteDeliveryRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	prompt := string(request.Stdin)
	switch {
	case strings.Contains(prompt, "Create both `tasks/todo/"):
		matches := planExecuteWorkName.FindStringSubmatch(prompt)
		if len(matches) != 2 {
			return platformprocess.CommandResult{}, fmt.Errorf("planner prompt omitted concrete Work name: %s", prompt)
		}
		runner.record("planner")
		writePlanExecutePRDForRunner(runner.repository, matches[1], runner.branch)
		return planExecuteCodexResult("<COMPLETE>"), nil
	case strings.Contains(prompt, "Implement only the highest"):
		runner.record("executor")
		runner.executions++
		if runner.rejectOnce && runner.executions == 2 {
			_, _ = planExecuteGit(request.WorkDir, "merge", "main")
			if err := os.WriteFile(filepath.Join(request.WorkDir, "delivered.txt"), []byte("resolved delivery\n"), 0o600); err != nil {
				return platformprocess.CommandResult{}, err
			}
			if _, err := planExecuteGit(request.WorkDir, "add", "delivered.txt"); err != nil {
				return platformprocess.CommandResult{}, err
			}
			if _, err := planExecuteGit(request.WorkDir, "commit", "-m", "repair merge conflict"); err != nil {
				return platformprocess.CommandResult{}, err
			}
			return planExecuteCodexResult("<COMPLETE>"), nil
		}
		if err := os.WriteFile(filepath.Join(request.WorkDir, "delivered.txt"), []byte("delivered through isolated worktree\n"), 0o600); err != nil {
			return platformprocess.CommandResult{}, err
		}
		if _, err := planExecuteGit(request.WorkDir, "add", "delivered.txt"); err != nil {
			return platformprocess.CommandResult{}, err
		}
		if _, err := planExecuteGit(request.WorkDir, "commit", "-m", "deliver packaged work"); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return planExecuteCodexResult("<COMPLETE>"), nil
	case strings.Contains(prompt, "Review `prd.json`"):
		runner.record("reviewer")
		runner.reviews++
		if runner.rejectOnce && runner.reviews == 1 {
			if err := os.WriteFile(filepath.Join(runner.repository, "delivered.txt"), []byte("conflicting base change\n"), 0o600); err != nil {
				return platformprocess.CommandResult{}, err
			}
			if _, err := planExecuteGit(runner.repository, "add", "delivered.txt"); err != nil {
				return platformprocess.CommandResult{}, err
			}
			if _, err := planExecuteGit(runner.repository, "commit", "-m", "conflicting base change"); err != nil {
				return platformprocess.CommandResult{}, err
			}
			return planExecuteCodexResult("<REJECTED>resolve the merge conflict"), nil
		}
		if _, err := planExecuteGit(runner.repository, "merge", "--no-ff", runner.branch, "-m", "merge packaged delivery"); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return planExecuteCodexResult("<COMPLETE>"), nil
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected plan-execute prompt: %s", prompt)
	}
}

func (runner *planExecuteDeliveryRunner) record(role string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.calls = append(runner.calls, role)
}

func (runner *planExecuteDeliveryRunner) Calls() []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]string(nil), runner.calls...)
}

func writePlanExecutePRDForRunner(repository, name, branch string) {
	directory := filepath.Join(repository, "tasks", "todo")
	_ = os.MkdirAll(directory, 0o700)
	jsonDocument := fmt.Sprintf(`{"project":"delivery","branchName":%q,"stories":[{"id":"US-001","priority":1,"passes":false,"notes":""}]}`, branch)
	_ = os.WriteFile(filepath.Join(directory, name+".json"), []byte(jsonDocument), 0o600)
	_ = os.WriteFile(filepath.Join(directory, name+".md"), []byte("# Delivery PRD\n\n## Acceptance criteria\n\n- Delivery is merged.\n"), 0o600)
}

func planExecuteCodexResult(text string) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(text)}
}

func planExecuteGit(directory string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}
