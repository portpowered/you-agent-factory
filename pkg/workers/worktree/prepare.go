package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// PrepareFactoryGitWorktreeResult describes a successful worktree preparation.
type PrepareFactoryGitWorktreeResult struct {
	CheckoutPath string
	Reused       bool
}

// PrepareFactoryGitWorktree creates or reuses a git worktree checkout for the
// resolved worktree name under the factory root.
func PrepareFactoryGitWorktree(
	ctx context.Context,
	factoryRoot string,
	worktreeName string,
	git GitCommander,
) (PrepareFactoryGitWorktreeResult, error) {
	if git == nil {
		git = ExecGitCommander{}
	}

	checkoutPath, err := ResolveFactoryWorktreeCheckoutPath(factoryRoot, worktreeName)
	if err != nil {
		return PrepareFactoryGitWorktreeResult{}, err
	}

	if valid, err := isValidGitWorktreeCheckout(ctx, git, checkoutPath); err != nil {
		return PrepareFactoryGitWorktreeResult{}, err
	} else if valid {
		return PrepareFactoryGitWorktreeResult{
			CheckoutPath: checkoutPath,
			Reused:       true,
		}, nil
	}

	if _, err := os.Stat(checkoutPath); err == nil {
		return PrepareFactoryGitWorktreeResult{}, fmt.Errorf(
			"worktree checkout path %s exists but is not a valid git worktree",
			checkoutPath,
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return PrepareFactoryGitWorktreeResult{}, fmt.Errorf("inspect worktree checkout path: %w", err)
	}

	repoRoot, err := resolveGitRepositoryRoot(ctx, git, factoryRoot)
	if err != nil {
		return PrepareFactoryGitWorktreeResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(checkoutPath), 0o755); err != nil {
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
	start time.Time,
	err error,
) interfaces.WorkResult {
	message := "worktree preparation failed"
	if err != nil {
		message += ": " + err.Error()
	}
	return interfaces.WorkResult{
		DispatchID:   dispatchID,
		TransitionID: transitionID,
		Outcome:      interfaces.OutcomeFailed,
		Error:        message,
		Metrics:      interfaces.WorkMetrics{Duration: time.Since(start)},
	}
}

func resolveGitRepositoryRoot(ctx context.Context, git GitCommander, fromDir string) (string, error) {
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

func isValidGitWorktreeCheckout(ctx context.Context, git GitCommander, checkoutPath string) (bool, error) {
	if strings.TrimSpace(checkoutPath) == "" {
		return false, nil
	}
	if _, err := os.Stat(checkoutPath); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect worktree checkout path: %w", err)
	}

	gitMetadataPath := filepath.Join(checkoutPath, ".git")
	info, err := os.Lstat(gitMetadataPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
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
