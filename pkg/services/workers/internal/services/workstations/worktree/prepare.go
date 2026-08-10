package worktree

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// PrepareFactoryGitWorktreeResult describes a successful worktree preparation.
type PrepareFactoryGitWorktreeResult = workerexecution.FactoryWorktreePreparation

// Service prepares Factory-local Git worktrees from composition-selected host
// effects. It is inert until Prepare is called.
type Service struct {
	fileSystem workerexecution.WorktreeFileSystem
	git        workerexecution.WorktreeGitCommander
}

// New constructs a worktree preparation service. Missing external effects are
// rejected instead of being resolved from process globals.
func New(
	fileSystem workerexecution.WorktreeFileSystem,
	git workerexecution.WorktreeGitCommander,
) (*Service, error) {
	if fileSystem == nil {
		return nil, fmt.Errorf("construct Worker worktree service: filesystem is required")
	}
	if git == nil {
		return nil, fmt.Errorf("construct Worker worktree service: Git commander is required")
	}
	return &Service{fileSystem: fileSystem, git: git}, nil
}

// Prepare creates or reuses the requested Factory-local Git worktree.
func (s *Service) Prepare(
	ctx context.Context,
	factoryRoot string,
	worktreeName string,
) (workerexecution.FactoryWorktreePreparation, error) {
	if s == nil || s.fileSystem == nil {
		return workerexecution.FactoryWorktreePreparation{}, fmt.Errorf("Worker worktree filesystem is required")
	}
	if s.git == nil {
		return workerexecution.FactoryWorktreePreparation{}, fmt.Errorf("Worker worktree Git commander is required")
	}
	return PrepareFactoryGitWorktree(ctx, factoryRoot, worktreeName, s.fileSystem, s.git)
}

// PrepareFactoryGitWorktree creates or reuses a git worktree checkout for the
// resolved worktree name under the factory root.
func PrepareFactoryGitWorktree(
	ctx context.Context,
	factoryRoot string,
	worktreeName string,
	fileSystem workerexecution.WorktreeFileSystem,
	git workerexecution.WorktreeGitCommander,
) (PrepareFactoryGitWorktreeResult, error) {
	if fileSystem == nil {
		return PrepareFactoryGitWorktreeResult{}, fmt.Errorf("worktree filesystem is required")
	}
	if git == nil {
		return PrepareFactoryGitWorktreeResult{}, fmt.Errorf("worktree Git commander is required")
	}

	checkoutPath, err := ResolveFactoryWorktreeCheckoutPath(factoryRoot, worktreeName, fileSystem)
	if err != nil {
		return PrepareFactoryGitWorktreeResult{}, err
	}

	if valid, err := isValidGitWorktreeCheckout(ctx, fileSystem, git, checkoutPath); err != nil {
		return PrepareFactoryGitWorktreeResult{}, err
	} else if valid {
		return PrepareFactoryGitWorktreeResult{
			CheckoutPath: checkoutPath,
			Reused:       true,
		}, nil
	}

	if _, err := fileSystem.Stat(checkoutPath); err == nil {
		return PrepareFactoryGitWorktreeResult{}, fmt.Errorf(
			"worktree checkout path %s exists but is not a valid git worktree",
			checkoutPath,
		)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return PrepareFactoryGitWorktreeResult{}, fmt.Errorf("inspect worktree checkout path: %w", err)
	}

	repoRoot, err := resolveGitRepositoryRoot(ctx, git, factoryRoot)
	if err != nil {
		return PrepareFactoryGitWorktreeResult{}, err
	}

	if err := fileSystem.MkdirAll(filepath.Dir(checkoutPath), 0o755); err != nil {
		return PrepareFactoryGitWorktreeResult{}, fmt.Errorf("create worktree parent directory: %w", err)
	}

	stdout, stderr, exitCode, runErr := git.Run(ctx, repoRoot, "worktree", "add", checkoutPath, "HEAD")
	if runErr != nil {
		return PrepareFactoryGitWorktreeResult{}, fmt.Errorf("git worktree add: %w", runErr)
	}
	if exitCode != 0 {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(stdout)
		}
		if detail == "" {
			detail = fmt.Sprintf("git worktree add exited with status %d", exitCode)
		}
		return PrepareFactoryGitWorktreeResult{}, fmt.Errorf("git worktree add failed: %s", detail)
	}

	return PrepareFactoryGitWorktreeResult{
		CheckoutPath: checkoutPath,
		Reused:       false,
	}, nil
}

// FailedWorkResultFromPreparation maps a preparation error to a failed workstation result.
func FailedWorkResultFromPreparation(
	dispatchID string,
	transitionID string,
	duration time.Duration,
	err error,
) workerexecution.WorkResult {
	message := "worktree preparation failed"
	if err != nil {
		message += ": " + err.Error()
	}
	return workerexecution.WorkResult{
		DispatchID:   dispatchID,
		TransitionID: transitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        message,
		Metrics:      workerexecution.WorkMetrics{Duration: duration},
	}
}

func resolveGitRepositoryRoot(ctx context.Context, git workerexecution.WorktreeGitCommander, fromDir string) (string, error) {
	fromDir = strings.TrimSpace(fromDir)
	if fromDir == "" {
		return "", fmt.Errorf("factory root is required")
	}

	stdout, stderr, exitCode, runErr := git.Run(ctx, fromDir, "rev-parse", "--show-toplevel")
	if runErr != nil {
		return "", fmt.Errorf("locate git repository: %w", runErr)
	}
	if exitCode != 0 {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = "factory root is not inside a git repository"
		}
		return "", fmt.Errorf("locate git repository: %s", detail)
	}
	if stdout == "" {
		return "", fmt.Errorf("locate git repository: empty repository root")
	}
	return filepath.Clean(stdout), nil
}

func isValidGitWorktreeCheckout(
	ctx context.Context,
	fileSystem workerexecution.WorktreeFileSystem,
	git workerexecution.WorktreeGitCommander,
	checkoutPath string,
) (bool, error) {
	if strings.TrimSpace(checkoutPath) == "" {
		return false, nil
	}
	if _, err := fileSystem.Stat(checkoutPath); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect worktree checkout path: %w", err)
	}

	gitMetadataPath := filepath.Join(checkoutPath, ".git")
	info, err := fileSystem.Lstat(gitMetadataPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("inspect worktree checkout .git metadata: %w", err)
	}
	if info.IsDir() {
		return false, nil
	}

	stdout, _, exitCode, runErr := git.Run(ctx, checkoutPath, "rev-parse", "--is-inside-work-tree")
	if runErr != nil {
		return false, fmt.Errorf("validate git worktree checkout: %w", runErr)
	}
	if exitCode != 0 {
		return false, nil
	}
	return strings.EqualFold(strings.TrimSpace(stdout), "true"), nil
}
