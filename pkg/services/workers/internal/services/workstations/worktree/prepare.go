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

const (
	gitSerializationLockRetries    = 20
	gitSerializationLockRetryDelay = 50 * time.Millisecond
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

	if err := addGitWorktree(ctx, fileSystem, git, repoRoot, checkoutPath); err != nil {
		return PrepareFactoryGitWorktreeResult{}, err
	}

	return PrepareFactoryGitWorktreeResult{
		CheckoutPath: checkoutPath,
		Reused:       false,
	}, nil
}

func addGitWorktree(
	ctx context.Context,
	fileSystem workerexecution.WorktreeFileSystem,
	git workerexecution.WorktreeGitCommander,
	repoRoot string,
	checkoutPath string,
) error {
	var lastResult gitWorktreeCommandResult
	for attempt := 0; attempt < gitSerializationLockRetries; attempt++ {
		stdout, stderr, exitCode, runErr := git.Run(ctx, repoRoot, "worktree", "add", checkoutPath, "HEAD")
		lastResult = gitWorktreeCommandResult{stdout: stdout, stderr: stderr, exitCode: exitCode, runErr: runErr}
		if runErr == nil && exitCode == 0 {
			return nil
		}
		if !isGitSerializationLockFailure(stdout, stderr, runErr) {
			return formatGitWorktreeCommandError(lastResult)
		}
		if attempt+1 == gitSerializationLockRetries {
			break
		}
		if err := waitForGitSerializationRetry(ctx); err != nil {
			return fmt.Errorf("git worktree add: %w", err)
		}
	}

	lockPath := gitSerializationLockPath(repoRoot, lastResult.stdout, lastResult.stderr)
	if lockPath != "" && gitPathExists(fileSystem, lockPath) {
		return fmt.Errorf(
			"git worktree serialization contention: resource=%s owner_liveness=indeterminate; "+
				"verify no Git or worktree setup process is still using the repository, then remove only %s and retry; "+
				"last_git_error=%s",
			lockPath,
			lockPath,
			gitWorktreeCommandDetail(lastResult),
		)
	}
	return formatGitWorktreeCommandError(lastResult)
}

type gitWorktreeCommandResult struct {
	stdout   string
	stderr   string
	exitCode int
	runErr   error
}

func formatGitWorktreeCommandError(result gitWorktreeCommandResult) error {
	if result.runErr != nil {
		return fmt.Errorf("git worktree add: %w", result.runErr)
	}
	return fmt.Errorf("git worktree add failed: %s", gitWorktreeCommandDetail(result))
}

func gitWorktreeCommandDetail(result gitWorktreeCommandResult) string {
	detail := strings.TrimSpace(result.stderr)
	if detail == "" {
		detail = strings.TrimSpace(result.stdout)
	}
	if detail == "" {
		detail = fmt.Sprintf("git worktree add exited with status %d", result.exitCode)
	}
	return detail
}

func isGitSerializationLockFailure(stdout, stderr string, runErr error) bool {
	detail := strings.ToLower(strings.Join([]string{stdout, stderr, gitWorktreeRunError(runErr)}, "\n"))
	return strings.Contains(detail, ".lock") &&
		(strings.Contains(detail, "file exists") ||
			strings.Contains(detail, "unable to create") ||
			strings.Contains(detail, "could not lock") ||
			strings.Contains(detail, "lock file"))
}

func gitWorktreeRunError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func waitForGitSerializationRetry(ctx context.Context) error {
	timer := time.NewTimer(gitSerializationLockRetryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func gitSerializationLockPath(repoRoot string, output ...string) string {
	for _, text := range output {
		for _, field := range strings.Fields(text) {
			candidate := strings.Trim(field, "\"'`.,:;()[]")
			if !strings.Contains(strings.ToLower(candidate), ".lock") {
				continue
			}
			if filepath.IsAbs(candidate) {
				return filepath.Clean(candidate)
			}
			relative := filepath.Clean(filepath.FromSlash(candidate))
			if relative == ".git" || strings.HasPrefix(relative, ".git"+string(filepath.Separator)) {
				return filepath.Join(repoRoot, relative)
			}
			return filepath.Clean(filepath.Join(repoRoot, ".git", filepath.Base(relative)))
		}
	}
	return filepath.Join(repoRoot, ".git", "config.lock")
}

func gitPathExists(fileSystem workerexecution.WorktreeFileSystem, path string) bool {
	_, err := fileSystem.Stat(path)
	return err == nil
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
