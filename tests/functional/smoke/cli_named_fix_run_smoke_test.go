package smoke

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/configinit"
	"github.com/portpowered/infinite-you/pkg/factory/packages/fix"
)

func TestNamedFixRun_RealCLICompletesIsolatedInvocationVariants(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/fix invocation smoke")
	}

	repoRoot := materializeNamedFixFactoryInGitRepository(t)
	homeDir := t.TempDir()
	if _, err := configinit.Init(homeDir); err != nil {
		t.Fatalf("configinit.Init: %v", err)
	}
	binaryPath := buildYouCLIBinary(t)
	mockWorkersPath := writePackagedFixMockWorkersConfig(t)

	for _, tc := range []struct {
		name           string
		worktreePrefix string
		additionalArgs []string
	}{
		{name: "default", worktreePrefix: "fix"},
		{name: "configured worktree", worktreePrefix: "customer-fix", additionalArgs: []string{"--worktree", "customer-fix"}},
		{name: "model provider flags", worktreePrefix: "flag-fix", additionalArgs: []string{
			"--worktree", "flag-fix", "--default-worker-model-provider", "codex", "--default-worker-model", "gpt-5-codex",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := runNamedFixCLI(t, repoRoot, homeDir, binaryPath, mockWorkersPath, tc.additionalArgs...)
			if strings.TrimSpace(stdout) != "<COMPLETE>" {
				t.Fatalf("stdout = %q, want approved review primary result", stdout)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty stderr", stderr)
			}
			assertNamedFixCreatedIsolatedWorktree(t, repoRoot, tc.worktreePrefix)
		})
	}
}

func materializeNamedFixFactoryInGitRepository(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	runNamedFixGit(t, repoRoot, "init")
	runNamedFixGit(t, repoRoot, "config", "user.email", "fix-smoke@example.com")
	runNamedFixGit(t, repoRoot, "config", "user.name", "fix smoke")
	runNamedFixGit(t, repoRoot, "commit", "--allow-empty", "-m", "init")
	projectRoot, err := factoryconfig.DefaultProjectNamedFactoryRoot(repoRoot)
	if err != nil {
		t.Fatalf("DefaultProjectNamedFactoryRoot: %v", err)
	}
	if _, err := factoryconfig.PersistNamedFactory(projectRoot, fix.PackagedFactoryName, fix.BuiltInFactoryJSON); err != nil {
		t.Fatalf("PersistNamedFactory(@you/fix): %v", err)
	}
	return repoRoot
}

func writePackagedFixMockWorkersConfig(t *testing.T) string {
	t.Helper()
	command, args := mockWorkerEchoCommand("<COMPLETE>")
	cfg := factoryconfig.MockWorkersConfig{
		MockWorkers: []factoryconfig.MockWorkerConfig{
			{WorkerName: "fix-planner", WorkstationName: fix.PackagedPlanWorkstationName, RunType: factoryconfig.MockWorkerRunTypeAccept},
			{WorkerName: "fix-implementer", WorkstationName: fix.PackagedImplementWorkstationName, RunType: factoryconfig.MockWorkerRunTypeScript, ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: command, Args: args}},
			{WorkerName: "fix-reviewer", WorkstationName: fix.PackagedReviewWorkstationName, RunType: factoryconfig.MockWorkerRunTypeScript, ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: command, Args: args}},
		},
	}
	return writeMockWorkersConfigFile(t, cfg, "mock-workers-packaged-fix.json")
}

func runNamedFixCLI(t *testing.T, repoRoot, homeDir, binaryPath, mockWorkersPath string, additionalArgs ...string) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"run", "--named", fix.PackagedFactoryName, "--with-mock-workers", "--no-record", "--quiet", mockWorkersPath}
	args = append(args, additionalArgs...)
	args = append(args, fmt.Sprintf("functional named fix %d", time.Now().UnixNano()))
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --named %s: %v\nstdout:\n%s\nstderr:\n%s", fix.PackagedFactoryName, err, stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String()
}

func assertNamedFixCreatedIsolatedWorktree(t *testing.T, repoRoot, prefix string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(repoRoot, "factory", "@you", "fix", ".worktrees", prefix+"-*"))
	if err != nil {
		t.Fatalf("find %s worktrees: %v", prefix, err)
	}
	if len(matches) != 1 {
		t.Fatalf("%s worktrees = %v, want one isolated checkout", prefix, matches)
	}
	if _, err := os.Stat(filepath.Join(matches[0], ".git")); err != nil {
		t.Fatalf("worktree %q is not a git checkout: %v", matches[0], err)
	}
}

func runNamedFixGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
