package workstation_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/workers/executor"
)

type dispatchCapturingExecutor struct {
	dispatch interfaces.WorkstationExecutionRequest
	called   bool
	result   interfaces.WorkResult
}

func (m *dispatchCapturingExecutor) Execute(_ context.Context, d interfaces.WorkstationExecutionRequest) (interfaces.WorkResult, error) {
	m.dispatch = d
	m.called = true
	return m.result, nil
}

func newCodexWorktreeTestWorkstationExecutor(
	runtimeConfig runtimefixtures.RuntimeConfigLookupFixture,
	inner *dispatchCapturingExecutor,
) *executor.WorkstationExecutor {
	return &executor.WorkstationExecutor{
		RuntimeConfig: runtimeConfig,
		Executor:      inner,
		Renderer:      &executor.DefaultPromptRenderer{},
	}
}

func TestWorkstationExecutor_CodexWorktreePreparation_SetsMaterializedWorkingDirectory(t *testing.T) {
	repoRoot := initGitRepositoryForWorkstationExecutorTest(t)
	factoryRoot := filepath.Join(repoRoot, "factory")
	if err := os.MkdirAll(factoryRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(factory): %v", err)
	}

	mock := &dispatchCapturingExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"}}
	we := newCodexWorktreeTestWorkstationExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		FactoryPath: factoryRoot,
		Workers: map[string]*interfaces.WorkerConfig{
			"codex-worker": {
				Type:          interfaces.WorkerTypeModel,
				Body:          "system",
				ModelProvider: string(interfaces.ModelProviderCodex),
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"process": {
				Type:           interfaces.WorkstationTypeModel,
				WorkerTypeName: "codex-worker",
				Runner:         interfaces.RunnerIDCodex,
				PromptTemplate: "Process {{ (index .Inputs 0).WorkID }}",
				Worktree:       `{{ index (index .Inputs 0).Tags "branch" }}`,
			},
		},
	}, mock)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-codex-worktree",
		TransitionID:    "t-codex-worktree",
		WorkerType:      "codex-worker",
		WorkstationName: "process",
		InputTokens: executor.InputTokens(interfaces.Token{
			ID: "tok-1",
			Color: interfaces.TokenColor{
				WorkID: "work-1",
				Tags:   map[string]string{"branch": "feature-a"},
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s (%s)", result.Outcome, interfaces.OutcomeAccepted, result.Error)
	}
	if !mock.called {
		t.Fatal("executor was not called")
	}

	wantCheckout := filepath.Join(factoryRoot, ".worktrees", "feature-a")
	if mock.dispatch.WorkingDirectory != wantCheckout {
		t.Fatalf("working directory = %q, want %q", mock.dispatch.WorkingDirectory, wantCheckout)
	}
	if mock.dispatch.Worktree != "feature-a" {
		t.Fatalf("worktree = %q, want feature-a", mock.dispatch.Worktree)
	}
	if mock.dispatch.RunnerID != interfaces.RunnerIDCodex {
		t.Fatalf("runner = %q, want codex", mock.dispatch.RunnerID)
	}
}

func TestWorkstationExecutor_LegacyClaudeWorktree_SkipsCodexFactoryPreparation(t *testing.T) {
	dir := t.TempDir()
	factoryRoot := filepath.Join(dir, "factory")
	if err := os.MkdirAll(factoryRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(factory): %v", err)
	}

	mock := &dispatchCapturingExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"}}
	we := newCodexWorktreeTestWorkstationExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		FactoryPath: factoryRoot,
		Workers: map[string]*interfaces.WorkerConfig{
			"claude-worker": {
				Type:          interfaces.WorkerTypeModel,
				Body:          "system",
				ModelProvider: string(interfaces.ModelProviderClaude),
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"process": {
				Type:           interfaces.WorkstationTypeModel,
				WorkerTypeName: "claude-worker",
				PromptTemplate: "Process {{ (index .Inputs 0).WorkID }}",
				Worktree:       `{{ index (index .Inputs 0).Tags "branch" }}`,
			},
		},
	}, mock)

	result, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-claude-worktree",
		TransitionID:    "t-claude-worktree",
		WorkerType:      "claude-worker",
		WorkstationName: "process",
		InputTokens: executor.InputTokens(interfaces.Token{
			ID:    "tok-1",
			Color: interfaces.TokenColor{WorkID: "work-1", Tags: map[string]string{"branch": "my-feature-branch"}},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s (%s)", result.Outcome, interfaces.OutcomeAccepted, result.Error)
	}
	if !mock.called {
		t.Fatal("executor was not called")
	}
	if mock.dispatch.WorkingDirectory != "" {
		t.Fatalf("working directory = %q, want empty for Claude --worktree passthrough", mock.dispatch.WorkingDirectory)
	}
	if mock.dispatch.Worktree != "my-feature-branch" {
		t.Fatalf("worktree = %q, want my-feature-branch", mock.dispatch.Worktree)
	}
	if mock.dispatch.RunnerID != interfaces.RunnerIDCodex {
		t.Fatalf("runner = %q, want default codex runner id", mock.dispatch.RunnerID)
	}
	if _, err := os.Stat(filepath.Join(factoryRoot, ".worktrees", "my-feature-branch")); !os.IsNotExist(err) {
		t.Fatal("expected factory-managed worktree not to be created for legacy Claude provider")
	}
}

func TestWorkstationExecutor_CodexWorktreePreparation_SkipsWhenWorkingDirectoryAuthored(t *testing.T) {
	repoRoot := initGitRepositoryForWorkstationExecutorTest(t)
	factoryRoot := filepath.Join(repoRoot, "factory")
	if err := os.MkdirAll(factoryRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(factory): %v", err)
	}

	mock := &dispatchCapturingExecutor{result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted, Output: "done"}}
	we := newCodexWorktreeTestWorkstationExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		FactoryPath:     factoryRoot,
		RuntimeBasePath: repoRoot,
		Workers: map[string]*interfaces.WorkerConfig{
			"codex-worker": {
				Type:          interfaces.WorkerTypeModel,
				Body:          "system",
				ModelProvider: string(interfaces.ModelProviderCodex),
			},
		},
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"process": {
				Type:             interfaces.WorkstationTypeModel,
				WorkerTypeName:   "codex-worker",
				Runner:           interfaces.RunnerIDCodex,
				PromptTemplate:   "Process {{ (index .Inputs 0).WorkID }}",
				WorkingDirectory: `/repo/{{ index (index .Inputs 0).Tags "branch" }}`,
				Worktree:         `{{ index (index .Inputs 0).Tags "branch" }}`,
			},
		},
	}, mock)

	_, err := we.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-codex-conflict",
		TransitionID:    "t-codex-conflict",
		WorkerType:      "codex-worker",
		WorkstationName: "process",
		InputTokens: executor.InputTokens(interfaces.Token{
			ID:    "tok-1",
			Color: interfaces.TokenColor{WorkID: "work-1", Tags: map[string]string{"branch": "feature-a"}},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Fatal("executor was not called")
	}
	wantWorkingDirectory := filepath.Join(repoRoot, "repo", "feature-a")
	if mock.dispatch.WorkingDirectory != wantWorkingDirectory {
		t.Fatalf("working directory = %q, want authored resolution %q", mock.dispatch.WorkingDirectory, wantWorkingDirectory)
	}
	if _, err := os.Stat(filepath.Join(factoryRoot, ".worktrees", "feature-a")); err == nil {
		t.Fatal("expected factory-managed worktree not to be created when workingDirectory is authored")
	}
}

func initGitRepositoryForWorkstationExecutorTest(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH")
	}

	repoRoot := t.TempDir()
	runGitForWorkstationExecutorTest(t, repoRoot, "init")
	runGitForWorkstationExecutorTest(t, repoRoot, "config", "user.email", "worktree-test@example.com")
	runGitForWorkstationExecutorTest(t, repoRoot, "config", "user.name", "worktree test")
	runGitForWorkstationExecutorTest(t, repoRoot, "commit", "--allow-empty", "-m", "init")
	return repoRoot
}

func runGitForWorkstationExecutorTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, output)
	}
}
