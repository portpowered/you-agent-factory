package builtcliacceptance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	defaultMaxLogTailBytes = 8192
	defaultBinaryName      = "you"
)

// Harness holds a built you CLI binary for hermetic acceptance scenarios.
type Harness struct {
	BinaryPath string
	RepoRoot   string
}

// Session is one hermetic acceptance run with isolated home and log directories.
type Session struct {
	harness   *Harness
	HomeDir   string
	LogDir    string
	WorkDir   string
	ServerURL string
}

// RunResult captures process output from a built-CLI invocation.
type RunResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// ScenarioFailure records enough diagnostics to debug a scenario mismatch.
type ScenarioFailure struct {
	Scenario   string
	Phase      string
	Message    string
	ExitCode   int
	StdoutTail string
	StderrTail string
	HomeDir    string
	LogDir     string
	BinaryPath string
}

func (f *ScenarioFailure) Error() string {
	if f == nil {
		return "<nil ScenarioFailure>"
	}
	parts := []string{fmt.Sprintf("%s: %s", f.Phase, f.Message)}
	if f.Scenario != "" {
		parts = append([]string{fmt.Sprintf("scenario=%s", f.Scenario)}, parts...)
	}
	if f.ExitCode != 0 {
		parts = append(parts, fmt.Sprintf("exit_code=%d", f.ExitCode))
	}
	if strings.TrimSpace(f.StdoutTail) != "" {
		parts = append(parts, "stdout_tail="+f.StdoutTail)
	}
	if strings.TrimSpace(f.StderrTail) != "" {
		parts = append(parts, "stderr_tail="+f.StderrTail)
	}
	return strings.Join(parts, "; ")
}

// NewHarness builds ./cmd/factory into a temporary binary under t.TempDir().
func NewHarness(t testing.TB, repoRoot string) *Harness {
	t.Helper()

	binaryPath, err := BuildBinary(repoRoot, filepath.Join(t.TempDir(), binaryFileName()))
	if err != nil {
		t.Fatalf("builtcliacceptance.NewHarness: %v", err)
	}
	return &Harness{
		BinaryPath: binaryPath,
		RepoRoot:   repoRoot,
	}
}

// BuildBinary compiles ./cmd/factory from repoRoot into outputPath.
func BuildBinary(repoRoot, outputPath string) (string, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return "", errors.New("repo root is required")
	}
	if strings.TrimSpace(outputPath) == "" {
		return "", errors.New("output path is required")
	}

	build := exec.Command("go", "build", "-o", outputPath, "./cmd/factory")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build you CLI: %w\n%s", err, string(output))
	}
	return outputPath, nil
}

// NewSession allocates isolated home and log directories for one scenario.
func (h *Harness) NewSession(t testing.TB) *Session {
	t.Helper()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("builtcliacceptance.NewSession: create home dir: %v", err)
	}
	logDir := filepath.Join(root, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("builtcliacceptance.NewSession: create log dir: %v", err)
	}
	workDir := filepath.Join(root, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("builtcliacceptance.NewSession: create work dir: %v", err)
	}

	return &Session{
		harness: h,
		HomeDir: homeDir,
		LogDir:  logDir,
		WorkDir: workDir,
	}
}

// WithNoExternalServer reserves a local TCP port for --server without requiring
// a pre-running listener. The built CLI starts and stops its own API process.
func (s *Session) WithNoExternalServer(t testing.TB) *Session {
	t.Helper()

	port, err := ReserveLocalTCPPort()
	if err != nil {
		t.Fatalf("builtcliacceptance.WithNoExternalServer: %v", err)
	}
	s.ServerURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	return s
}

// RuntimeLogDirFlags returns CLI flags that keep runtime logs under the session log dir.
func (s *Session) RuntimeLogDirFlags() []string {
	return []string{"--runtime-log-dir", s.LogDir}
}

// ServerFlags returns CLI flags for the reserved no-external-server URL.
func (s *Session) ServerFlags() []string {
	if strings.TrimSpace(s.ServerURL) == "" {
		return nil
	}
	return []string{"--server", s.ServerURL}
}

// ProcessEnv returns child-process environment variables for this session.
func (s *Session) ProcessEnv() []string {
	return ProcessEnvForIsolatedHome(s.HomeDir)
}

// ProcessEnvWith returns ProcessEnv plus additional KEY=value entries.
func (s *Session) ProcessEnvWith(extra ...string) []string {
	env := s.ProcessEnv()
	if len(extra) == 0 {
		return env
	}
	return append(env, extra...)
}

// Run executes the built you binary with the session's hermetic environment.
func (s *Session) Run(ctx context.Context, args ...string) (RunResult, error) {
	return s.run(ctx, s.ProcessEnv(), nil, args...)
}

// RunWithStdin executes the built you binary with piped stdin content.
func (s *Session) RunWithStdin(ctx context.Context, stdin string, args ...string) (RunResult, error) {
	return s.run(ctx, s.ProcessEnv(), strings.NewReader(stdin), args...)
}

// RunWithEnv executes the built you binary with extra environment variables.
func (s *Session) RunWithEnv(ctx context.Context, extraEnv []string, args ...string) (RunResult, error) {
	return s.run(ctx, s.ProcessEnvWith(extraEnv...), nil, args...)
}

func (s *Session) run(ctx context.Context, env []string, stdin io.Reader, args ...string) (RunResult, error) {
	if s.harness == nil {
		return RunResult{}, errors.New("session harness is nil")
	}
	if strings.TrimSpace(s.harness.BinaryPath) == "" {
		return RunResult{}, errors.New("session binary path is empty")
	}

	cmd := exec.CommandContext(ctx, s.harness.BinaryPath, args...)
	cmd.Dir = s.WorkDir
	cmd.Env = env
	if stdin != nil {
		cmd.Stdin = stdin
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return RunResult{}, &ScenarioFailure{
				Phase:      "start_process",
				Message:    err.Error(),
				HomeDir:    s.HomeDir,
				LogDir:     s.LogDir,
				BinaryPath: s.harness.BinaryPath,
				StdoutTail: tailBytes(stdout.Bytes(), defaultMaxLogTailBytes),
				StderrTail: tailBytes(stderr.Bytes(), defaultMaxLogTailBytes),
			}
		}
	}

	result := RunResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
	if exitCode != 0 {
		return result, s.failure("run_process", fmt.Errorf("exit status %d", exitCode), result)
	}
	return result, nil
}

// RequireSuccess fails the test when Run returns a non-zero exit or start error.
func (s *Session) RequireSuccess(t testing.TB, scenario string, result RunResult, err error) RunResult {
	t.Helper()
	if err == nil {
		return result
	}

	var failure *ScenarioFailure
	if errors.As(err, &failure) {
		failure.Scenario = scenario
		t.Fatalf("%s", failure.Error())
	}
	t.Fatalf("scenario %s: %v\nstdout:\n%s\nstderr:\n%s", scenario, err, result.Stdout, result.Stderr)
	return result
}

func (s *Session) failure(phase string, err error, result RunResult) *ScenarioFailure {
	return &ScenarioFailure{
		Phase:      phase,
		Message:    err.Error(),
		ExitCode:   result.ExitCode,
		StdoutTail: tailBytes([]byte(result.Stdout), defaultMaxLogTailBytes),
		StderrTail: tailBytes([]byte(result.Stderr), defaultMaxLogTailBytes),
		HomeDir:    s.HomeDir,
		LogDir:     s.LogDir,
		BinaryPath: s.harness.BinaryPath,
	}
}

func binaryFileName() string {
	if runtime.GOOS == "windows" {
		return defaultBinaryName + ".exe"
	}
	return defaultBinaryName
}

func tailBytes(data []byte, max int) string {
	if len(data) > max {
		data = data[len(data)-max:]
	}
	return string(data)
}
