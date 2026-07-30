package planexecute

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackagedPlanExecuteWorkspaceSetupCreatesPRDWorktreeOnRequestedBranch(t *testing.T) {
	repository := initializePlanExecuteRepository(t)
	writePlanExecutePRD(t, repository, "delivery")

	script := filepath.Join(planExecuteRepositoryRoot(t), "packages", "packaged-factories", "factories", "plan-execute", "scripts", "setup-workspace.py")
	command := exec.Command("python", script, "delivery", "feature/customer-branch")
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("setup-workspace.py error = %v\n%s", err, output)
	}

	var result struct {
		Status   string `json:"status"`
		Branch   string `json:"branch"`
		Worktree string `json:"worktree"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode setup output: %v\n%s", err, output)
	}
	if result.Status != "ready" || result.Branch != "feature/customer-branch" {
		t.Fatalf("setup result = %#v", result)
	}
	if filepath.Clean(result.Worktree) != filepath.Join(repository, ".claude", "worktrees", "delivery") {
		t.Fatalf("worktree = %q", result.Worktree)
	}
	for _, name := range []string{"prd.json", "prd.md", "progress.txt"} {
		if _, err := os.Stat(filepath.Join(result.Worktree, name)); err != nil {
			t.Fatalf("worktree artifact %s: %v", name, err)
		}
	}
	branch := runPlanExecuteGit(t, result.Worktree, "branch", "--show-current")
	if branch != "feature/customer-branch" {
		t.Fatalf("worktree branch = %q", branch)
	}
}

func TestPackagedPlanExecuteWorkspaceSetupRejectsUnsafeBranch(t *testing.T) {
	repository := initializePlanExecuteRepository(t)
	writePlanExecutePRD(t, repository, "delivery")
	script := filepath.Join(planExecuteRepositoryRoot(t), "packages", "packaged-factories", "factories", "plan-execute", "scripts", "setup-workspace.py")
	command := exec.Command("python", script, "delivery", "../escape")
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "safe branch name") {
		t.Fatalf("unsafe branch error = %v, output = %s", err, output)
	}
}

func initializePlanExecuteRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	runPlanExecuteGit(t, repository, "init", "-b", "main")
	runPlanExecuteGit(t, repository, "config", "user.email", "factory@example.test")
	runPlanExecuteGit(t, repository, "config", "user.name", "Factory Test")
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runPlanExecuteGit(t, repository, "add", "README.md")
	runPlanExecuteGit(t, repository, "commit", "-m", "fixture")
	return repository
}

func writePlanExecutePRD(t *testing.T, repository, name string) {
	t.Helper()
	directory := filepath.Join(repository, "tasks", "todo")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name+".json"), []byte(`{"stories":[{"id":"US-001","passes":false}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name+".md"), []byte("# Delivery\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runPlanExecuteGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func planExecuteRepositoryRoot(t *testing.T) string {
	t.Helper()
	root := runPlanExecuteGit(t, ".", "rev-parse", "--show-toplevel")
	absolute, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}
