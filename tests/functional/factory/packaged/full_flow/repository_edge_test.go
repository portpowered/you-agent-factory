package fullflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// fullFlowGitCommander is the repository/filesystem edge for the packaged
// Full Flow. It deliberately performs the small observable Git contract in
// memory while retaining real checkout files for the production worktree
// preparer and the scenario assertions. The functional lane therefore tests
// the runtime against the injected repository effect without launching Git
// through the optimized source.
type fullFlowGitCommander struct {
	mu           sync.Mutex
	repositories map[string]*fullFlowGitRepository
	calls        []fullFlowGitCall
}

type fullFlowGitRepository struct {
	root      string
	configs   map[string]string
	branches  map[string]map[string][]byte
	commits   map[string]int
	current   string
	worktrees map[string]string
	staged    map[string]map[string]struct{}
}

type fullFlowGitCall struct {
	RepositoryRoot string
	Directory      string
	Args           []string
}

func newFullFlowGitCommander() *fullFlowGitCommander {
	return &fullFlowGitCommander{repositories: make(map[string]*fullFlowGitRepository)}
}

func (git *fullFlowGitCommander) Run(
	ctx context.Context,
	directory string,
	args ...string,
) (string, string, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", "", -1, err
	}
	if git == nil {
		return "", "", -1, fmt.Errorf("Full Flow Git edge is nil")
	}

	directory, err := filepath.Abs(directory)
	if err != nil {
		return "", "", -1, fmt.Errorf("resolve Full Flow Git directory: %w", err)
	}
	directory = filepath.Clean(directory)
	requestedArgs := append([]string(nil), args...)
	commandArgs, err := fullFlowGitCommandArgs(args)
	if err != nil {
		return "", "", -1, err
	}
	if len(commandArgs) == 0 {
		return "", "", -1, fmt.Errorf("Full Flow Git command is required")
	}

	git.mu.Lock()
	defer git.mu.Unlock()
	if commandArgs[0] == "init" {
		repository, err := git.initializeRepositoryLocked(directory, commandArgs)
		if err != nil {
			return "", err.Error(), 1, nil
		}
		git.recordLocked(repository.root, directory, requestedArgs)
		return "", "", 0, nil
	}
	repository := git.repositoryForDirectoryLocked(directory)
	if repository == nil {
		return "", fmt.Sprintf("Full Flow Git repository not found for %q", directory), 1, nil
	}
	git.recordLocked(repository.root, directory, requestedArgs)

	stdout, stderr, exitCode, runErr := git.runLocked(repository, directory, commandArgs)
	if runErr != nil {
		return stdout, stderr, exitCode, runErr
	}
	return stdout, stderr, exitCode, nil
}

func fullFlowGitCommandArgs(args []string) ([]string, error) {
	args = append([]string(nil), args...)
	for len(args) > 0 && args[0] == "-c" {
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return nil, fmt.Errorf("Full Flow Git -c option requires a value")
		}
		args = args[2:]
	}
	return args, nil
}

func (git *fullFlowGitCommander) initializeRepositoryLocked(
	root string,
	args []string,
) (*fullFlowGitRepository, error) {
	if len(args) != 3 || args[1] != "-b" || strings.TrimSpace(args[2]) == "" {
		return nil, fmt.Errorf("Full Flow Git init requires -b <branch>")
	}
	branch := args[2]
	if _, exists := git.repositories[root]; exists {
		return nil, fmt.Errorf("Full Flow Git repository %q is already initialized", root)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		return nil, fmt.Errorf("create Full Flow Git metadata: %w", err)
	}
	repository := &fullFlowGitRepository{
		root:      root,
		configs:   make(map[string]string),
		branches:  map[string]map[string][]byte{branch: {}},
		commits:   map[string]int{branch: 0},
		current:   branch,
		worktrees: make(map[string]string),
		staged:    make(map[string]map[string]struct{}),
	}
	git.repositories[root] = repository
	return repository, nil
}

func (git *fullFlowGitCommander) repositoryForDirectoryLocked(directory string) *fullFlowGitRepository {
	var best *fullFlowGitRepository
	bestRootLength := -1
	for root, repository := range git.repositories {
		if !fullFlowPathContains(root, directory) {
			continue
		}
		if len(root) > bestRootLength {
			best = repository
			bestRootLength = len(root)
		}
	}
	return best
}

func (git *fullFlowGitCommander) recordLocked(root, directory string, args []string) {
	git.calls = append(git.calls, fullFlowGitCall{
		RepositoryRoot: root,
		Directory:      directory,
		Args:           append([]string(nil), args...),
	})
}

func (git *fullFlowGitCommander) runLocked(
	repository *fullFlowGitRepository,
	directory string,
	args []string,
) (string, string, int, error) {
	switch args[0] {
	case "config":
		return repository.runConfig(args)
	case "rev-parse":
		return repository.runRevParse(directory, args)
	case "show-ref":
		return repository.runShowRef(args)
	case "worktree":
		return repository.runWorktree(directory, args)
	case "add":
		return repository.runAdd(directory, args)
	case "commit":
		return repository.runCommit(directory, args)
	case "diff":
		return repository.runDiff(directory, args)
	case "merge":
		return repository.runMerge(directory, args)
	default:
		return "", fmt.Sprintf("unsupported Full Flow Git operation %q", strings.Join(args, " ")), 1, nil
	}
}

func (repository *fullFlowGitRepository) runConfig(args []string) (string, string, int, error) {
	switch {
	case len(args) == 3 && args[1] == "--get":
		value, ok := repository.configs[args[2]]
		if !ok {
			return "", "", 1, nil
		}
		return value + "\n", "", 0, nil
	case len(args) == 3:
		repository.configs[args[1]] = args[2]
		return "", "", 0, nil
	default:
		return "", "Full Flow Git config arguments are invalid", 1, nil
	}
}

func (repository *fullFlowGitRepository) runRevParse(
	directory string,
	args []string,
) (string, string, int, error) {
	if len(args) < 2 {
		return "", "Full Flow Git rev-parse arguments are invalid", 1, nil
	}
	switch args[1] {
	case "--show-toplevel":
		if len(args) != 2 {
			return "", "Full Flow Git rev-parse arguments are invalid", 1, nil
		}
		checkout, _, ok := repository.checkoutForDirectory(directory)
		if !ok {
			return "", "directory is not inside a Full Flow Git checkout", 1, nil
		}
		return checkout + "\n", "", 0, nil
	case "--is-inside-work-tree":
		if len(args) != 2 {
			return "", "Full Flow Git rev-parse arguments are invalid", 1, nil
		}
		if _, _, ok := repository.checkoutForDirectory(directory); !ok {
			return "", "directory is not inside a Full Flow Git checkout", 1, nil
		}
		return "true\n", "", 0, nil
	case "--verify":
		if len(args) != 3 {
			return "", "Full Flow Git rev-parse --verify requires a reference", 1, nil
		}
		if _, exists := repository.branches[args[2]]; !exists {
			return "", fmt.Sprintf("Full Flow Git reference %q does not exist", args[2]), 1, nil
		}
		return args[2] + "\n", "", 0, nil
	default:
		return "", "Full Flow Git rev-parse operation is unsupported", 1, nil
	}
}

func (repository *fullFlowGitRepository) runShowRef(args []string) (string, string, int, error) {
	if len(args) != 4 || args[1] != "--verify" || args[2] != "--quiet" {
		return "", "Full Flow Git show-ref arguments are invalid", 1, nil
	}
	const prefix = "refs/heads/"
	branch, ok := strings.CutPrefix(args[3], prefix)
	if !ok || branch == "" {
		return "", "Full Flow Git show-ref reference is invalid", 1, nil
	}
	if _, exists := repository.branches[branch]; !exists {
		return "", "", 1, nil
	}
	return "", "", 0, nil
}

func (repository *fullFlowGitRepository) runWorktree(
	directory string,
	args []string,
) (string, string, int, error) {
	if len(args) < 3 || args[1] != "add" {
		if len(args) >= 3 && args[1] == "remove" && args[2] == "--force" {
			return repository.removeWorktree(args[3:])
		}
		return "", "Full Flow Git worktree arguments are invalid", 1, nil
	}
	if len(args) < 4 {
		return "", "Full Flow Git worktree add arguments are invalid", 1, nil
	}
	branch := ""
	position := 2
	if args[position] == "-b" {
		if len(args) < position+2 {
			return "", "Full Flow Git worktree branch is missing", 1, nil
		}
		branch = args[position+1]
		position += 2
	}
	if len(args) != position+2 {
		return "", "Full Flow Git worktree add arguments are invalid", 1, nil
	}
	target := filepath.Clean(args[position])
	if !filepath.IsAbs(target) {
		target = filepath.Join(repository.root, target)
	}
	target, err := filepath.Abs(target)
	if err != nil {
		return "", "", 1, fmt.Errorf("resolve Full Flow worktree target: %w", err)
	}
	base := args[position+1]
	if branch == "" {
		branch = base
		if branch == "HEAD" {
			_, checkoutBranch, ok := repository.checkoutForDirectory(directory)
			if !ok {
				return "", "directory is not inside a Full Flow Git checkout", 1, nil
			}
			branch = checkoutBranch
		}
	}
	if _, exists := repository.worktrees[target]; exists {
		return "", "Full Flow Git worktree already exists", 1, nil
	}
	if _, err := os.Stat(target); err == nil {
		return "", "Full Flow Git worktree target already exists", 1, nil
	} else if !os.IsNotExist(err) {
		return "", "", 1, fmt.Errorf("inspect Full Flow worktree target: %w", err)
	}
	baseBranch := base
	if baseBranch == "HEAD" {
		_, baseBranch, _ = repository.checkoutForDirectory(directory)
	}
	baseSnapshot, exists := repository.branches[baseBranch]
	if !exists {
		return "", fmt.Sprintf("Full Flow Git base branch %q does not exist", baseBranch), 1, nil
	}
	if _, exists := repository.branches[branch]; exists && branch != base {
		return "", fmt.Sprintf("Full Flow Git branch %q already exists", branch), 1, nil
	}
	if err := copyFullFlowRepositoryTree(repository.root, target); err != nil {
		return "", "", 1, err
	}
	if err := os.WriteFile(filepath.Join(target, ".git"), []byte("gitdir: "+filepath.Join(repository.root, ".git")+"\n"), 0o600); err != nil {
		return "", "", 1, fmt.Errorf("write Full Flow worktree metadata: %w", err)
	}
	repository.branches[branch] = cloneFullFlowSnapshot(baseSnapshot)
	repository.commits[branch] = repository.commits[baseBranch]
	repository.worktrees[target] = branch
	repository.staged[target] = make(map[string]struct{})
	return "", "", 0, nil
}

func (repository *fullFlowGitRepository) removeWorktree(args []string) (string, string, int, error) {
	if len(args) != 1 {
		return "", "Full Flow Git worktree remove arguments are invalid", 1, nil
	}
	target, err := filepath.Abs(args[0])
	if err != nil {
		return "", "", 1, fmt.Errorf("resolve Full Flow worktree removal: %w", err)
	}
	if _, exists := repository.worktrees[target]; !exists {
		return "", "Full Flow Git worktree is not registered", 1, nil
	}
	if err := os.RemoveAll(target); err != nil {
		return "", "", 1, fmt.Errorf("remove Full Flow worktree: %w", err)
	}
	delete(repository.worktrees, target)
	delete(repository.staged, target)
	return "", "", 0, nil
}

func (repository *fullFlowGitRepository) runAdd(
	directory string,
	args []string,
) (string, string, int, error) {
	if len(args) < 2 {
		return "", "Full Flow Git add requires a path", 1, nil
	}
	checkout, _, ok := repository.checkoutForDirectory(directory)
	if !ok {
		return "", "directory is not inside a Full Flow Git checkout", 1, nil
	}
	staged := repository.staged[checkout]
	if staged == nil {
		staged = make(map[string]struct{})
		repository.staged[checkout] = staged
	}
	for _, value := range args[1:] {
		if value == "." {
			if err := stageFullFlowCheckoutFiles(checkout, staged); err != nil {
				return "", "", 1, err
			}
			continue
		}
		relative, err := fullFlowRelativePath(checkout, value)
		if err != nil {
			return "", "", 1, err
		}
		staged[relative] = struct{}{}
	}
	return "", "", 0, nil
}

func (repository *fullFlowGitRepository) runCommit(
	directory string,
	args []string,
) (string, string, int, error) {
	checkout, branch, ok := repository.checkoutForDirectory(directory)
	if !ok {
		return "", "directory is not inside a Full Flow Git checkout", 1, nil
	}
	staged := repository.staged[checkout]
	if len(staged) == 0 {
		return "", "Full Flow Git commit has no staged changes", 1, nil
	}
	snapshot := cloneFullFlowSnapshot(repository.branches[branch])
	paths := make([]string, 0, len(staged))
	for path := range staged {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		absolute := filepath.Join(checkout, path)
		contents, err := os.ReadFile(absolute)
		if err != nil {
			if os.IsNotExist(err) {
				delete(snapshot, path)
				continue
			}
			return "", "", 1, fmt.Errorf("read staged Full Flow file %q: %w", absolute, err)
		}
		snapshot[path] = append([]byte(nil), contents...)
	}
	repository.branches[branch] = snapshot
	repository.commits[branch]++
	clear(staged)
	return "", "", 0, nil
}

func (repository *fullFlowGitRepository) runDiff(
	directory string,
	args []string,
) (string, string, int, error) {
	if len(args) != 3 || args[1] != "--check" || args[2] != "HEAD^" {
		return "", "Full Flow Git diff arguments are invalid", 1, nil
	}
	_, branch, ok := repository.checkoutForDirectory(directory)
	if !ok {
		return "", "directory is not inside a Full Flow Git checkout", 1, nil
	}
	if repository.commits[branch] < 2 {
		return "", "Full Flow Git diff requires a parent commit", 1, nil
	}
	return "", "", 0, nil
}

func (repository *fullFlowGitRepository) runMerge(
	directory string,
	args []string,
) (string, string, int, error) {
	if len(args) != 5 || args[1] != "--no-ff" || args[3] != "-m" {
		return "", "Full Flow Git merge arguments are invalid", 1, nil
	}
	rootCheckout, rootBranch, ok := repository.checkoutForDirectory(directory)
	if !ok || rootCheckout != repository.root {
		return "", "Full Flow Git merge must run from the repository root", 1, nil
	}
	source, exists := repository.branches[args[2]]
	if !exists {
		return "", fmt.Sprintf("Full Flow Git branch %q does not exist", args[2]), 1, nil
	}
	merged := cloneFullFlowSnapshot(repository.branches[rootBranch])
	for path, contents := range source {
		merged[path] = append([]byte(nil), contents...)
		if err := writeFullFlowCheckoutFile(repository.root, path, contents); err != nil {
			return "", "", 1, err
		}
	}
	repository.branches[rootBranch] = merged
	repository.commits[rootBranch]++
	return "", "", 0, nil
}

func (repository *fullFlowGitRepository) checkoutForDirectory(directory string) (string, string, bool) {
	checkout := repository.root
	branch := repository.current
	bestLength := len(repository.root)
	for target, candidate := range repository.worktrees {
		if !fullFlowPathContains(target, directory) || len(target) <= bestLength {
			continue
		}
		checkout = target
		branch = candidate
		bestLength = len(target)
	}
	return checkout, branch, fullFlowPathContains(checkout, directory)
}

func cloneFullFlowSnapshot(snapshot map[string][]byte) map[string][]byte {
	clone := make(map[string][]byte, len(snapshot))
	for path, contents := range snapshot {
		clone[path] = append([]byte(nil), contents...)
	}
	return clone
}

func fullFlowRelativePath(checkout, path string) (string, error) {
	absolute := path
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(checkout, path)
	}
	absolute, err := filepath.Abs(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve Full Flow staged path %q: %w", path, err)
	}
	relative, err := filepath.Rel(checkout, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Full Flow staged path %q escapes checkout", path)
	}
	return filepath.Clean(relative), nil
}

func stageFullFlowCheckoutFiles(checkout string, staged map[string]struct{}) error {
	return filepath.WalkDir(checkout, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, relativeErr := filepath.Rel(checkout, path)
		if relativeErr != nil {
			return relativeErr
		}
		if relative == ".git" {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type().IsRegular() {
			staged[relative] = struct{}{}
		}
		return nil
	})
}

func copyFullFlowRepositoryTree(source, target string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("create Full Flow worktree: %w", err)
	}
	worktreesPath := filepath.Join(source, ".claude", "worktrees")
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == target || fullFlowPathContains(target, path) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == filepath.Join(source, ".git") || path == worktreesPath {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		relative, relativeErr := filepath.Rel(source, path)
		if relativeErr != nil {
			return relativeErr
		}
		if relative == "." {
			return nil
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return os.WriteFile(destination, contents, 0o600)
	})
}

func writeFullFlowCheckoutFile(checkout, relative string, contents []byte) error {
	absolute := filepath.Join(checkout, relative)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		return fmt.Errorf("create Full Flow file parent: %w", err)
	}
	return os.WriteFile(absolute, contents, 0o600)
}

func runFullFlowGit(
	ctx context.Context,
	git *fullFlowGitCommander,
	directory string,
	args ...string,
) (string, error) {
	stdout, stderr, exitCode, err := git.Run(ctx, directory, args...)
	if err != nil {
		return stdout, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	if exitCode != 0 {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(stdout)
		}
		if detail == "" {
			detail = fmt.Sprintf("exit status %d", exitCode)
		}
		return stdout, fmt.Errorf("git %s: %s", strings.Join(args, " "), detail)
	}
	return strings.TrimSpace(stdout), nil
}

func (git *fullFlowGitCommander) callsFor(repositoryRoot string) []fullFlowGitCall {
	git.mu.Lock()
	defer git.mu.Unlock()
	calls := make([]fullFlowGitCall, 0)
	for _, call := range git.calls {
		if call.RepositoryRoot != repositoryRoot {
			continue
		}
		calls = append(calls, fullFlowGitCall{
			RepositoryRoot: call.RepositoryRoot,
			Directory:      call.Directory,
			Args:           append([]string(nil), call.Args...),
		})
	}
	return calls
}

func assertFullFlowRepositoryEdge(t testing.TB, git *fullFlowGitCommander, repositoryRoot string) {
	t.Helper()
	calls := git.callsFor(repositoryRoot)
	if len(calls) == 0 {
		t.Fatalf("Full Flow repository edge recorded no calls for %q", repositoryRoot)
	}
	want := [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "factory@example.test"},
		{"config", "user.name", "Factory Test"},
		{"add", "README.md"},
		{"commit", "-m", "fixture"},
		{"rev-parse", "--show-toplevel"},
		{"rev-parse", "--verify", "main"},
		{"config", "core.longpaths", "true"},
		{"config", "--get", "core.longpaths"},
	}
	for _, expected := range want {
		if !fullFlowGitCallExists(calls, expected) {
			t.Errorf("Full Flow repository edge did not observe %q; calls=%v", expected, calls)
		}
	}
	if countFullFlowGitCalls(calls, []string{"show-ref", "--verify", "--quiet", "refs/heads/task-a"}) == 0 ||
		countFullFlowGitCalls(calls, []string{"show-ref", "--verify", "--quiet", "refs/heads/task-b"}) == 0 {
		t.Errorf("Full Flow repository edge did not observe branch existence probes; calls=%v", calls)
	}
	if countFullFlowGitOperation(calls, "worktree", "add") < 2 {
		t.Errorf("Full Flow repository edge worktree add count = %d, want at least 2; calls=%v", countFullFlowGitOperation(calls, "worktree", "add"), calls)
	}
	if countFullFlowGitOperation(calls, "config", "core.longpaths") < 2 {
		t.Errorf("Full Flow repository edge core.longpaths writes = %d, want at least 2; calls=%v", countFullFlowGitOperation(calls, "config", "core.longpaths"), calls)
	}
	if countFullFlowGitOperation(calls, "diff", "--check") != 2 {
		t.Errorf("Full Flow repository edge diff checks = %d, want 2; calls=%v", countFullFlowGitOperation(calls, "diff", "--check"), calls)
	}
	if countFullFlowGitOperation(calls, "merge", "--no-ff") != 2 {
		t.Errorf("Full Flow repository edge merges = %d, want 2; calls=%v", countFullFlowGitOperation(calls, "merge", "--no-ff"), calls)
	}
}

func fullFlowGitCallExists(calls []fullFlowGitCall, expected []string) bool {
	return countFullFlowGitCalls(calls, expected) > 0
}

func countFullFlowGitCalls(calls []fullFlowGitCall, expected []string) int {
	count := 0
	for _, call := range calls {
		args, err := fullFlowGitCommandArgs(call.Args)
		if err == nil && strings.Join(args, "\x00") == strings.Join(expected, "\x00") {
			count++
		}
	}
	return count
}

func countFullFlowGitOperation(calls []fullFlowGitCall, operation ...string) int {
	count := 0
	for _, call := range calls {
		args, err := fullFlowGitCommandArgs(call.Args)
		if err != nil || len(args) < len(operation) {
			continue
		}
		if strings.Join(args[:len(operation)], "\x00") == strings.Join(operation, "\x00") {
			count++
		}
	}
	return count
}

// fullFlowScriptCommandRunner keeps the packaged script contract observable
// while routing setup's repository mutations through the same Git edge used
// by Worktree preparation. Cycle decisions remain the authored script's
// output contract, including its bounded invalid-input status.
type fullFlowScriptCommandRunner struct {
	git *fullFlowGitCommander
}

func newFullFlowScriptCommandRunner(git *fullFlowGitCommander) *fullFlowScriptCommandRunner {
	return &fullFlowScriptCommandRunner{git: git}
}

func (runner *fullFlowScriptCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if runner == nil || runner.git == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("Full Flow script Git edge is required")
	}
	if len(request.Args) == 0 {
		return platformprocess.CommandResult{}, fmt.Errorf("Full Flow script path is required")
	}
	script := strings.ToLower(filepath.Base(request.Args[0]))
	switch script {
	case "setup-task-worktree.py":
		return runner.runSetup(ctx, request)
	case "decide-cycle.py":
		return runner.runCycle(request)
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected Full Flow script %q", request.Args[0])
	}
}

func (runner *fullFlowScriptCommandRunner) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	result, err := runner.Run(ctx, request)
	if observer != nil {
		if len(result.Stdout) > 0 {
			observer(platformprocess.OutputStreamStdout, append([]byte(nil), result.Stdout...))
		}
		if len(result.Stderr) > 0 {
			observer(platformprocess.OutputStreamStderr, append([]byte(nil), result.Stderr...))
		}
	}
	return result, err
}

func (runner *fullFlowScriptCommandRunner) runSetup(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if len(request.Args) != 3 || strings.TrimSpace(request.Args[1]) == "" || strings.TrimSpace(request.Args[2]) == "" {
		return platformprocess.CommandResult{
			Stderr:   []byte("task and base branch are required\n"),
			ExitCode: 1,
		}, nil
	}
	task, base := request.Args[1], request.Args[2]
	root, err := runFullFlowGit(ctx, runner.git, request.WorkDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	if _, err := runFullFlowGit(ctx, runner.git, root, "rev-parse", "--verify", base); err != nil {
		return platformprocess.CommandResult{}, err
	}
	target := filepath.Join(root, ".claude", "worktrees", task)
	if _, err := os.Stat(filepath.Join(target, ".git")); os.IsNotExist(err) {
		_, _, exitCode, runErr := runner.git.Run(ctx, root, "show-ref", "--verify", "--quiet", "refs/heads/"+task)
		if runErr != nil {
			return platformprocess.CommandResult{}, runErr
		}
		if exitCode == 0 {
			if _, err := runFullFlowGit(ctx, runner.git, root, "-c", "core.longpaths=true", "worktree", "add", target, task); err != nil {
				return platformprocess.CommandResult{}, err
			}
		} else if exitCode == 1 {
			if _, err := runFullFlowGit(ctx, runner.git, root, "-c", "core.longpaths=true", "worktree", "add", "-b", task, target, base); err != nil {
				return platformprocess.CommandResult{}, err
			}
		} else {
			return platformprocess.CommandResult{}, fmt.Errorf("Full Flow Git branch probe exited with status %d", exitCode)
		}
	} else if err != nil {
		return platformprocess.CommandResult{}, err
	}
	if _, err := runFullFlowGit(ctx, runner.git, root, "config", "core.longpaths", "true"); err != nil {
		return platformprocess.CommandResult{}, err
	}
	content, err := json.Marshal(map[string]string{
		"status":   "ready",
		"task":     task,
		"base":     base,
		"worktree": target,
	})
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	return platformprocess.CommandResult{Stdout: append(content, '\n')}, nil
}

func (runner *fullFlowScriptCommandRunner) runCycle(
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if len(request.Args) != 2 {
		return platformprocess.CommandResult{Stderr: []byte("usage: decide-cycle.py <continue|complete>\n"), ExitCode: 2}, nil
	}
	decision := strings.ToLower(strings.TrimSpace(request.Args[1]))
	if decision != "continue" && decision != "complete" {
		return platformprocess.CommandResult{
			Stderr:   []byte("cycle-control payload must be exactly 'continue' or 'complete'\n"),
			ExitCode: 2,
		}, nil
	}
	return platformprocess.CommandResult{Stdout: []byte(decision + "\n")}, nil
}

var _ platformprocess.CommandRunner = (*fullFlowScriptCommandRunner)(nil)
var _ workers.WorktreeGitCommander = (*fullFlowGitCommander)(nil)
