package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type recordingGitCommander struct {
	calls []gitCall
	next  []gitResponse
}

type gitCall struct {
	dir  string
	args []string
}

type gitResponse struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

func (r *recordingGitCommander) Run(_ context.Context, dir string, args ...string) (string, string, int, error) {
	call := gitCall{dir: dir, args: append([]string(nil), args...)}
	r.calls = append(r.calls, call)
	if len(r.next) == 0 {
		return "", "unexpected git call", 1, nil
	}
	resp := r.next[0]
	r.next = r.next[1:]
	return resp.stdout, resp.stderr, resp.exitCode, resp.err
}

func TestPrepareFactoryGitWorktree_CreatesWorktreeWhenMissing(t *testing.T) {
	repoRoot := initGitRepository(t)
	factoryRoot := filepath.Join(repoRoot, "factory")
	if err := os.MkdirAll(factoryRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(factory): %v", err)
	}

	result, err := PrepareFactoryGitWorktree(context.Background(), factoryRoot, "feature-a", ExecGitCommander{})
	if err != nil {
		t.Fatalf("PrepareFactoryGitWorktree() error = %v", err)
	}
	if result.Reused {
		t.Fatal("Reused = true, want false for newly created worktree")
	}

	want := filepath.Join(factoryRoot, ".worktrees", "feature-a")
	if result.CheckoutPath != want {
		t.Fatalf("CheckoutPath = %q, want %q", result.CheckoutPath, want)
	}
	if _, err := os.Stat(result.CheckoutPath); err != nil {
		t.Fatalf("checkout path missing: %v", err)
	}

	valid, err := isValidGitWorktreeCheckout(context.Background(), ExecGitCommander{}, result.CheckoutPath)
	if err != nil {
		t.Fatalf("isValidGitWorktreeCheckout() error = %v", err)
	}
	if !valid {
		t.Fatal("expected created checkout to be a valid git worktree")
	}
}

func TestPrepareFactoryGitWorktree_ReusesExistingValidWorktree(t *testing.T) {
	repoRoot := initGitRepository(t)
	factoryRoot := filepath.Join(repoRoot, "factory")
	if err := os.MkdirAll(factoryRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(factory): %v", err)
	}

	git := ExecGitCommander{}
	first, err := PrepareFactoryGitWorktree(context.Background(), factoryRoot, "feature-a", git)
	if err != nil {
		t.Fatalf("first PrepareFactoryGitWorktree() error = %v", err)
	}

	second, err := PrepareFactoryGitWorktree(context.Background(), factoryRoot, "feature-a", git)
	if err != nil {
		t.Fatalf("second PrepareFactoryGitWorktree() error = %v", err)
	}
	if !second.Reused {
		t.Fatal("Reused = false, want true for existing valid worktree")
	}
	if second.CheckoutPath != first.CheckoutPath {
		t.Fatalf("CheckoutPath = %q, want %q", second.CheckoutPath, first.CheckoutPath)
	}
}

func TestPrepareFactoryGitWorktree_UsesExistingWorktreesParent(t *testing.T) {
	repoRoot := initGitRepository(t)
	factoryRoot := filepath.Join(repoRoot, "factory")
	existingParent := filepath.Join(factoryRoot, "worktrees")
	if err := os.MkdirAll(existingParent, 0o755); err != nil {
		t.Fatalf("MkdirAll(existing parent): %v", err)
	}

	result, err := PrepareFactoryGitWorktree(context.Background(), factoryRoot, "feature-a", ExecGitCommander{})
	if err != nil {
		t.Fatalf("PrepareFactoryGitWorktree() error = %v", err)
	}

	want := filepath.Join(existingParent, "feature-a")
	if result.CheckoutPath != want {
		t.Fatalf("CheckoutPath = %q, want %q", result.CheckoutPath, want)
	}
}

func TestPrepareFactoryGitWorktree_ReturnsFailureWhenNotGitRepository(t *testing.T) {
	factoryRoot := t.TempDir()
	git := &recordingGitCommander{
		next: []gitResponse{
			{exitCode: 128, stderr: "not a git repository"},
		},
	}

	_, err := PrepareFactoryGitWorktree(context.Background(), factoryRoot, "feature-a", git)
	if err == nil {
		t.Fatal("PrepareFactoryGitWorktree() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "locate git repository") {
		t.Fatalf("error = %v, want locate git repository failure", err)
	}
}

func TestPrepareFactoryGitWorktree_ReturnsFailureWhenGitUnavailable(t *testing.T) {
	factoryRoot := t.TempDir()
	git := &recordingGitCommander{
		next: []gitResponse{
			{err: errors.New("exec: \"git\": executable file not found in $PATH")},
		},
	}

	_, err := PrepareFactoryGitWorktree(context.Background(), factoryRoot, "feature-a", git)
	if err == nil {
		t.Fatal("PrepareFactoryGitWorktree() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "validate git worktree checkout") &&
		!strings.Contains(err.Error(), "locate git repository") &&
		!strings.Contains(err.Error(), "git worktree add") {
		t.Fatalf("error = %v, want git execution failure", err)
	}
}

func TestPrepareFactoryGitWorktree_ReturnsFailureWhenWorktreeAddFails(t *testing.T) {
	repoRoot := initGitRepository(t)
	factoryRoot := filepath.Join(repoRoot, "factory")
	if err := os.MkdirAll(factoryRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(factory): %v", err)
	}

	checkoutPath, err := ResolveFactoryWorktreeCheckoutPath(factoryRoot, "feature-a")
	if err != nil {
		t.Fatalf("ResolveFactoryWorktreeCheckoutPath() error = %v", err)
	}

	git := &recordingGitCommander{
		next: []gitResponse{
			{stdout: repoRoot}, // rev-parse --show-toplevel
			{exitCode: 1, stderr: "fatal: invalid worktree path"},
		},
	}

	_, err = PrepareFactoryGitWorktree(context.Background(), factoryRoot, "feature-a", git)
	if err == nil {
		t.Fatal("PrepareFactoryGitWorktree() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "git worktree add failed") {
		t.Fatalf("error = %v, want git worktree add failure", err)
	}
	if _, statErr := os.Stat(checkoutPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("checkout path %q should not exist after failed add", checkoutPath)
	}
}

func TestPrepareFactoryGitWorktree_ReturnsFailureWhenPathExistsButIsNotWorktree(t *testing.T) {
	repoRoot := initGitRepository(t)
	factoryRoot := filepath.Join(repoRoot, "factory")
	if err := os.MkdirAll(factoryRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(factory): %v", err)
	}

	checkoutPath, err := ResolveFactoryWorktreeCheckoutPath(factoryRoot, "feature-a")
	if err != nil {
		t.Fatalf("ResolveFactoryWorktreeCheckoutPath() error = %v", err)
	}
	if err := os.MkdirAll(checkoutPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(checkout): %v", err)
	}
	if err := os.WriteFile(filepath.Join(checkoutPath, "README"), []byte("not a worktree"), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	_, err = PrepareFactoryGitWorktree(context.Background(), factoryRoot, "feature-a", ExecGitCommander{})
	if err == nil {
		t.Fatal("PrepareFactoryGitWorktree() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "not a valid git worktree") {
		t.Fatalf("error = %v, want invalid existing checkout failure", err)
	}
}

func TestFailedWorkResultFromPreparation_UsesFailedOutcome(t *testing.T) {
	start := time.Now().Add(-time.Second)
	result := FailedWorkResultFromPreparation("dispatch-1", "transition-1", start, errors.New("git unavailable"))

	if result.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("Outcome = %q, want %q", result.Outcome, interfaces.OutcomeFailed)
	}
	if result.DispatchID != "dispatch-1" || result.TransitionID != "transition-1" {
		t.Fatalf("result = %#v, want dispatch and transition ids preserved", result)
	}
	if !strings.Contains(result.Error, "worktree preparation failed: git unavailable") {
		t.Fatalf("Error = %q, want preparation failure message", result.Error)
	}
	if result.Metrics.Duration <= 0 {
		t.Fatalf("Metrics.Duration = %v, want positive duration", result.Metrics.Duration)
	}
}

func initGitRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH")
	}

	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init")
	runGit(t, repoRoot, "config", "user.email", "worktree-test@example.com")
	runGit(t, repoRoot, "config", "user.name", "worktree test")
	runGit(t, repoRoot, "commit", "--allow-empty", "-m", "init")
	return repoRoot
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, output)
	}
}
