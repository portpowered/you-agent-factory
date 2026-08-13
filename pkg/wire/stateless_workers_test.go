package wire

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestCanonicalStatelessWorkersExecuteBeforeRuntimeOpening(t *testing.T) {
	providersService, err := provideProvidersService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	modelsService, err := provideModelsService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideModelsService() error = %v", err)
	}
	worktreeLifecycle, err := provideWorkersWorktree(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideWorkersWorktree() error = %v", err)
	}
	service, err := provideStatelessWorkersService(
		providersService,
		modelsService,
		statelessCompositionCommandRunner{},
		platformfilesystem.Local{},
		platformclock.Real{},
		zap.NewNop(),
		worktreeLifecycle,
		platformfilesystem.Local{},
	)
	if err != nil {
		t.Fatalf("provideStatelessWorkersService() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			DispatchID: "dispatch-canonical",
			AttemptID:  "attempt-canonical",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "script-worker",
			RunnerID:   "script",
			Command:    "canonical-script",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "canonical-output" {
		t.Fatalf("output = %#v, want canonical-output", result.Output)
	}
}

func TestCanonicalStatelessWorkersReleasesProductionWorktreeAfterSuccess(t *testing.T) {
	git := &statelessWorktreeGit{}
	service := newProductionCleanupStatelessService(t, git, statelessCompositionCommandRunner{})

	result, err := service.Execute(context.Background(), statelessWorktreeRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	git.assertRemoved(t)
}

func TestCanonicalStatelessWorkersReleasesProductionWorktreeAfterCancellation(t *testing.T) {
	started := make(chan struct{})
	git := &statelessWorktreeGit{}
	service := newProductionCleanupStatelessService(t, git, &statelessBlockingCommandRunner{started: started})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		result workers.ExecuteResult
		err    error
	}, 1)
	go func() {
		result, err := service.Execute(ctx, statelessWorktreeRequest())
		done <- struct {
			result workers.ExecuteResult
			err    error
		}{result: result, err: err}
	}()
	<-started
	cancel()

	completed := <-done
	if completed.err != nil {
		t.Fatalf("Execute() error = %v", completed.err)
	}
	if completed.result.Outcome != workers.ExecutionOutcomeCanceled {
		t.Fatalf("outcome = %q, want CANCELED", completed.result.Outcome)
	}
	git.assertRemoved(t)
}

func TestCanonicalStatelessWorkersReleasesProductionWorktreeAfterPreStartFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	git := &statelessWorktreeGit{onAdd: cancel}
	service := newProductionCleanupStatelessService(t, git, statelessCompositionCommandRunner{})

	_, err := service.Execute(ctx, statelessWorktreeRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	git.assertRemoved(t)
}

func newProductionCleanupStatelessService(
	t *testing.T,
	git *statelessWorktreeGit,
	commandRunner factorysessionwire.ScriptCommandRunner,
) workers.Service {
	t.Helper()
	edges := serviceedges.Edges{
		WorkersWorktreeFileSystem: statelessWorktreeFileSystem{},
		WorkersWorktreeGit:        git,
	}
	providersService, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	modelsService, err := provideModelsService(edges)
	if err != nil {
		t.Fatalf("provideModelsService() error = %v", err)
	}
	worktreeLifecycle, err := provideWorkersWorktree(edges)
	if err != nil {
		t.Fatalf("provideWorkersWorktree() error = %v", err)
	}
	service, err := provideStatelessWorkersService(
		providersService,
		modelsService,
		commandRunner,
		platformfilesystem.Local{},
		platformclock.Real{},
		zap.NewNop(),
		worktreeLifecycle,
		platformfilesystem.Local{},
	)
	if err != nil {
		t.Fatalf("provideStatelessWorkersService() error = %v", err)
	}
	return service
}

func statelessWorktreeRequest() workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			DispatchID: "dispatch-worktree",
			AttemptID:  "attempt-worktree",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "script-worker",
			RunnerID:   "script",
			Command:    "cleanup-script",
			Workspace: workers.WorkspacePolicy{
				PrepareWorktree:    true,
				FactoryDirectory:   "factory-root",
				CheckoutIdentifier: "attempt-worktree",
			},
		},
	}
}

type statelessWorktreeFileSystem struct{}

func (statelessWorktreeFileSystem) Stat(string) (fs.FileInfo, error) {
	return nil, fs.ErrNotExist
}

func (statelessWorktreeFileSystem) Lstat(string) (fs.FileInfo, error) {
	return nil, fs.ErrNotExist
}

func (statelessWorktreeFileSystem) MkdirAll(string, fs.FileMode) error {
	return nil
}

type statelessWorktreeGit struct {
	mu    sync.Mutex
	calls []string
	onAdd func()
}

func (git *statelessWorktreeGit) Run(_ context.Context, _ string, args ...string) (string, string, int, error) {
	git.mu.Lock()
	git.calls = append(git.calls, strings.Join(args, " "))
	git.mu.Unlock()
	if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
		if git.onAdd != nil {
			git.onAdd()
		}
		return "", "", 0, nil
	}
	if len(args) >= 2 && args[0] == "worktree" && args[1] == "remove" {
		return "", "", 0, nil
	}
	return "repo-root", "", 0, nil
}

func (git *statelessWorktreeGit) assertRemoved(t *testing.T) {
	t.Helper()
	git.mu.Lock()
	defer git.mu.Unlock()
	if len(git.calls) == 0 || !strings.HasPrefix(git.calls[len(git.calls)-1], "worktree remove --force ") {
		t.Fatalf("Git calls = %#v, want final worktree remove", git.calls)
	}
}

type statelessBlockingCommandRunner struct {
	started chan struct{}
	once    sync.Once
}

func (runner *statelessBlockingCommandRunner) Run(ctx context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	runner.once.Do(func() { close(runner.started) })
	<-ctx.Done()
	return workers.CommandResult{}, ctx.Err()
}

type statelessCompositionCommandRunner struct{}

func (statelessCompositionCommandRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: []byte("canonical-output")}, nil
}
