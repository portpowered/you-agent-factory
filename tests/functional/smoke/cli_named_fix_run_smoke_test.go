package smoke

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	for _, tc := range []struct {
		name           string
		worktreePrefix string
		additionalArgs []string
		modelProvider  string
		model          string
	}{
		{name: "default", worktreePrefix: "fix"},
		{name: "configured worktree", worktreePrefix: "customer-fix", additionalArgs: []string{"--worktree", "customer-fix"}},
		{name: "model provider flags", worktreePrefix: "flag-fix", additionalArgs: []string{
			"--worktree", "flag-fix", "--default-worker-model-provider", "codex", "--default-worker-model", "gpt-5-codex",
		}, modelProvider: "codex", model: "gpt-5-codex"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockWorkersPath := writePackagedFixMockWorkersConfig(t, packagedFixMockWorkersOptions{
				modelProvider: tc.modelProvider,
				model:         tc.model,
			})
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

func TestNamedFixRun_RealCLIRecordsRejectedReviewLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/fix lifecycle smoke")
	}

	repoRoot := materializeNamedFixFactoryInGitRepository(t)
	homeDir := t.TempDir()
	if _, err := configinit.Init(homeDir); err != nil {
		t.Fatalf("configinit.Init: %v", err)
	}
	binaryPath := buildYouCLIBinary(t)
	logPath := filepath.Join(t.TempDir(), "stages.log")
	mockWorkersPath := writePackagedFixMockWorkersConfig(t, packagedFixMockWorkersOptions{
		stageLogPath:      logPath,
		rejectFirstReview: true,
	})

	stdout, stderr := runNamedFixCLI(t, repoRoot, homeDir, binaryPath, mockWorkersPath)
	if strings.TrimSpace(stdout) != "<COMPLETE>" || stderr != "" {
		t.Fatalf("CLI output = stdout %q stderr %q, want approved completion", stdout, stderr)
	}
	assertNamedFixCreatedIsolatedWorktree(t, repoRoot, "fix")
	stageCalls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read recorded stage calls: %v", err)
	}
	if got, want := strings.Fields(string(stageCalls)), []string{"plan", "implement", "review", "implement", "review"}; !slicesEqual(got, want) {
		t.Fatalf("stage calls = %v, want %v", got, want)
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

type packagedFixMockWorkersOptions struct {
	stageLogPath      string
	rejectFirstReview bool
	modelProvider     string
	model             string
}

func writePackagedFixMockWorkersConfig(t *testing.T, opts packagedFixMockWorkersOptions) string {
	t.Helper()
	newMock := func(workerName, workstationName, stage string) factoryconfig.MockWorkerConfig {
		command, args := packagedFixMockWorkerCommand()
		env := map[string]string{
			"GO_WANT_NAMED_FIX_MOCK_WORKER": "1",
			"FIX_SMOKE_STAGE":               stage,
			"FIX_SMOKE_STAGE_LOG":           opts.stageLogPath,
		}
		if opts.rejectFirstReview && stage == "review" {
			env["FIX_SMOKE_REJECT_FIRST_REVIEW"] = "1"
			env["FIX_SMOKE_REVIEW_COUNTER"] = filepath.Join(t.TempDir(), "review.count")
		}
		return factoryconfig.MockWorkerConfig{
			WorkerName: workerName, WorkstationName: workstationName,
			ModelProvider: opts.modelProvider, Model: opts.model,
			RunType:      factoryconfig.MockWorkerRunTypeScript,
			ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: command, Args: args, Env: env},
		}
	}
	cfg := factoryconfig.MockWorkersConfig{
		MockWorkers: []factoryconfig.MockWorkerConfig{
			newMock("fix-planner", fix.PackagedPlanWorkstationName, "plan"),
			newMock("fix-implementer", fix.PackagedImplementWorkstationName, "implement"),
			newMock("fix-reviewer", fix.PackagedReviewWorkstationName, "review"),
		},
	}
	return writeMockWorkersConfigFile(t, cfg, "mock-workers-packaged-fix.json")
}

func packagedFixMockWorkerCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", `
if ($env:FIX_SMOKE_STAGE_LOG) { Add-Content -LiteralPath $env:FIX_SMOKE_STAGE_LOG -Value $env:FIX_SMOKE_STAGE }
if ($env:FIX_SMOKE_STAGE -eq 'review' -and $env:FIX_SMOKE_REJECT_FIRST_REVIEW -eq '1') {
  $count = 0
  if (Test-Path -LiteralPath $env:FIX_SMOKE_REVIEW_COUNTER) { $count = [int](Get-Content -Raw -LiteralPath $env:FIX_SMOKE_REVIEW_COUNTER) }
  Set-Content -NoNewline -LiteralPath $env:FIX_SMOKE_REVIEW_COUNTER -Value ($count + 1)
  if ($count -eq 0) { [Console]::Out.Write('needs revision'); exit 0 }
}
[Console]::Out.Write('<COMPLETE>')`}
	}
	return "/bin/sh", []string{"-c", `
if [ -n "$FIX_SMOKE_STAGE_LOG" ]; then printf '%s\n' "$FIX_SMOKE_STAGE" >> "$FIX_SMOKE_STAGE_LOG"; fi
if [ "$FIX_SMOKE_STAGE" = review ] && [ "$FIX_SMOKE_REJECT_FIRST_REVIEW" = 1 ]; then
  count=0
  if [ -f "$FIX_SMOKE_REVIEW_COUNTER" ]; then count=$(cat "$FIX_SMOKE_REVIEW_COUNTER"); fi
  printf '%s' $((count + 1)) > "$FIX_SMOKE_REVIEW_COUNTER"
  if [ "$count" -eq 0 ]; then printf 'needs revision'; exit 0; fi
fi
printf '<COMPLETE>'`}
}

func slicesEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
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
