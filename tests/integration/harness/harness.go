package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const (
	traceCommand = "strace"
	buildPackage = "./cmd/factory"
)

// ErrAuditUnavailable means the host cannot enforce the source-tree read
// boundary required by the installed-binary integration tier.
var ErrAuditUnavailable = errors.New("source-tree read auditing is unavailable")

// Harness owns one integration run. Its binary is built lazily at most once;
// each invocation still gets a new customer-like process environment.
type Harness struct {
	repoRoot string
	root     string
	tracer   string

	buildOnce sync.Once
	binary    string
	buildErr  error

	mu     sync.Mutex
	closed bool
}

// InvocationEnvironment describes the isolated locations given to one
// process invocation. All paths are outside the repository's canonical root.
type InvocationEnvironment struct {
	WorkingDirectory string
	HomeDirectory    string
	UserProfile      string
	RuntimeDirectory string
}

// Invocation is single-use. Files such as a copied Factory fixture can be
// written into its working directory before Run is called.
type Invocation struct {
	harness *Harness
	root    string
	env     InvocationEnvironment

	mu   sync.Mutex
	used bool
}

// Result contains the process verdict and captured output. A non-zero
// ExitCode is a valid result for callers that are checking a CLI rejection.
type Result struct {
	ExitCode     int
	Stdout       string
	Stderr       string
	AuditLogPath string
}

// SourceTreeReadError identifies the canonical path that crossed the source
// tree boundary.
type SourceTreeReadError struct {
	Path      string
	TraceLine string
}

func (e *SourceTreeReadError) Error() string {
	return fmt.Sprintf("source-tree read detected at %s (trace: %s)", e.Path, e.TraceLine)
}

// New creates a harness and fails closed unless Linux strace is available.
func New(repoRoot string) (*Harness, error) {
	canonicalRoot, err := canonicalPath(repoRoot, "")
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	if info, err := os.Stat(canonicalRoot); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("path is not a directory")
		}
		return nil, fmt.Errorf("repository root: %w", err)
	}

	tracer, err := findTracer()
	if err != nil {
		return nil, err
	}
	root, err := makeExternalTempRoot(canonicalRoot)
	if err != nil {
		return nil, err
	}

	return &Harness{repoRoot: canonicalRoot, root: root, tracer: tracer}, nil
}

func findTracer() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("%w: supported enforcement uses Linux strace (GOOS=%s)", ErrAuditUnavailable, runtime.GOOS)
	}
	tracer, err := exec.LookPath(traceCommand)
	if err != nil {
		return "", fmt.Errorf("%w: %s is not on PATH: %v", ErrAuditUnavailable, traceCommand, err)
	}
	return tracer, nil
}

func makeExternalTempRoot(repoRoot string) (string, error) {
	for range 3 {
		root, err := os.MkdirTemp("", "you-integration-")
		if err != nil {
			return "", fmt.Errorf("create integration temp root: %w", err)
		}
		canonicalRoot, canonicalErr := canonicalPath(root, "")
		if canonicalErr == nil && !pathWithin(repoRoot, canonicalRoot) {
			return canonicalRoot, nil
		}
		_ = os.RemoveAll(root)
	}
	return "", fmt.Errorf("%w: temporary integration root is reachable from repository %s", ErrAuditUnavailable, repoRoot)
}

// Build builds cmd/factory once and returns the shared executable path.
func (h *Harness) Build(ctx context.Context) (string, error) {
	h.buildOnce.Do(func() {
		h.binary = filepath.Join(h.root, "you"+executableSuffix())
		cmd := exec.CommandContext(ctx, "go", "build", "-o", h.binary, buildPackage)
		cmd.Dir = h.repoRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			h.buildErr = fmt.Errorf("build %s: %w\n%s", buildPackage, err, strings.TrimSpace(string(output)))
		}
	})
	if h.buildErr != nil {
		return "", h.buildErr
	}
	return h.binary, nil
}

// NewInvocation creates fresh HOME, USERPROFILE, working, and runtime paths.
func (h *Harness) NewInvocation() (*Invocation, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, errors.New("integration harness is closed")
	}

	root, err := os.MkdirTemp(h.root, "invocation-")
	if err != nil {
		return nil, fmt.Errorf("create invocation root: %w", err)
	}
	paths := map[string]string{
		"working directory": filepath.Join(root, "work"),
		"home directory":    filepath.Join(root, "home"),
		"user profile":      filepath.Join(root, "userprofile"),
		"runtime directory": filepath.Join(root, "runtime"),
	}
	for label, path := range paths {
		if err := os.Mkdir(path, 0o700); err != nil {
			_ = os.RemoveAll(root)
			return nil, fmt.Errorf("create %s: %w", label, err)
		}
		if err := h.requireExternalPath(path); err != nil {
			_ = os.RemoveAll(root)
			return nil, err
		}
	}

	return &Invocation{
		harness: h,
		root:    root,
		env: InvocationEnvironment{
			WorkingDirectory: paths["working directory"],
			HomeDirectory:    paths["home directory"],
			UserProfile:      paths["user profile"],
			RuntimeDirectory: paths["runtime directory"],
		},
	}, nil
}

func (h *Harness) requireExternalPath(path string) error {
	canonical, err := canonicalPath(path, "")
	if err != nil {
		return fmt.Errorf("resolve isolated path %s: %w", path, err)
	}
	if pathWithin(h.repoRoot, canonical) {
		return fmt.Errorf("%w: isolated path %s is inside repository %s", ErrAuditUnavailable, canonical, h.repoRoot)
	}
	return nil
}

// Environment returns the paths that may be populated before Run.
func (i *Invocation) Environment() InvocationEnvironment {
	return i.env
}

// Run executes the shared binary under strace -f, which follows forked and
// exec'd descendants. A process exit status is returned as a verdict; setup,
// tracing, and source-boundary errors are returned as Go errors.
func (i *Invocation) Run(ctx context.Context, args ...string) (Result, error) {
	i.mu.Lock()
	if i.used {
		i.mu.Unlock()
		return Result{}, errors.New("integration invocation is single-use")
	}
	i.used = true
	i.mu.Unlock()

	if len(args) == 0 {
		return Result{}, errors.New("integration invocation requires a command")
	}
	binary, err := i.harness.Build(ctx)
	if err != nil {
		return Result{}, err
	}
	auditLog := filepath.Join(i.env.RuntimeDirectory, "file-access.strace")
	cmdArgs := []string{"-ff", "-yy", "-e", "trace=%file", "-o", auditLog, binary}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, i.harness.tracer, cmdArgs...)
	cmd.Dir = i.env.WorkingDirectory
	cmd.Env = isolatedEnvironment(i.env)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	result := Result{
		ExitCode:     exitCode(cmd, runErr),
		Stdout:       stdout.String(),
		Stderr:       stderr.String(),
		AuditLogPath: auditLog,
	}
	if err := i.checkAudit(auditLog, result); err != nil {
		return result, err
	}
	if runErr == nil {
		return result, nil
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return result, nil
	}
	return result, fmt.Errorf("run installed binary: %w", runErr)
}

func (i *Invocation) checkAudit(auditLog string, result Result) error {
	logs, err := auditLogFiles(auditLog)
	if err != nil {
		return fmt.Errorf("%w: find strace output: %v; stderr=%q", ErrAuditUnavailable, err, strings.TrimSpace(result.Stderr))
	}
	sawFileAccess := false
	for _, logPath := range logs {
		data, err := os.ReadFile(logPath)
		if err != nil {
			return fmt.Errorf("%w: read strace output %s: %v; stderr=%q", ErrAuditUnavailable, logPath, err, strings.TrimSpace(result.Stderr))
		}
		violation, err := auditTrace(i.harness.repoRoot, i.env.WorkingDirectory, data)
		if errors.Is(err, errNoFileAccessEvents) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%w: %v; stderr=%q", ErrAuditUnavailable, err, strings.TrimSpace(result.Stderr))
		}
		sawFileAccess = true
		if violation != nil {
			return violation
		}
	}
	if !sawFileAccess {
		return fmt.Errorf("%w: %v; stderr=%q", ErrAuditUnavailable, errNoFileAccessEvents, strings.TrimSpace(result.Stderr))
	}
	return nil
}

func auditLogFiles(prefix string) ([]string, error) {
	directory := filepath.Dir(prefix)
	filePrefix := filepath.Base(prefix) + "."
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), filePrefix) {
			continue
		}
		paths = append(paths, filepath.Join(directory, entry.Name()))
	}
	if len(paths) == 0 {
		return nil, errors.New("strace produced no per-process output files")
	}
	return paths, nil
}

func exitCode(cmd *exec.Cmd, runErr error) int {
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// Close releases all harness state, including any invocation directories.
func (h *Harness) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	h.mu.Unlock()
	return os.RemoveAll(h.root)
}

// Close removes this invocation's files while leaving the shared binary for
// other invocations in the same harness run.
func (i *Invocation) Close() error {
	return os.RemoveAll(i.root)
}

func isolatedEnvironment(env InvocationEnvironment) []string {
	overrides := map[string]string{
		"HOME":            env.HomeDirectory,
		"USERPROFILE":     env.UserProfile,
		"HOMEDRIVE":       filepath.VolumeName(env.UserProfile),
		"HOMEPATH":        string(os.PathSeparator),
		"PWD":             env.WorkingDirectory,
		"OLDPWD":          env.WorkingDirectory,
		"TMPDIR":          env.RuntimeDirectory,
		"TMP":             env.RuntimeDirectory,
		"TEMP":            env.RuntimeDirectory,
		"XDG_CONFIG_HOME": filepath.Join(env.HomeDirectory, ".config"),
		"XDG_CACHE_HOME":  filepath.Join(env.HomeDirectory, ".cache"),
		"XDG_DATA_HOME":   filepath.Join(env.HomeDirectory, ".local", "share"),
		"APPDATA":         filepath.Join(env.UserProfile, "AppData", "Roaming"),
		"LOCALAPPDATA":    filepath.Join(env.UserProfile, "AppData", "Local"),
	}

	result := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, replaced := overrides[strings.ToUpper(key)]; replaced {
			continue
		}
		result = append(result, entry)
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func executableSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
