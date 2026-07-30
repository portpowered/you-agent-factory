package builtcliacceptance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

const (
	defaultMaxLogTailBytes = 8192
)

// Harness constructs isolated root processes for hermetic acceptance scenarios.
type Harness struct {
	RepoRoot string
}

// Session is one hermetic acceptance run with isolated home and log directories.
type Session struct {
	harness   *Harness
	HomeDir   string
	LogDir    string
	WorkDir   string
	ServerURL string
}

// RunResult captures process output from a root-process invocation.
type RunResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// Command is an in-process analogue of one CLI command invocation. It exists
// so functional tests can exercise the production root process without
// compiling or spawning the you executable.
type Command struct {
	harness *Harness
	ctx     context.Context
	args    []string

	Dir    string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
	err        error
	pipeWriter *io.PipeWriter
}

// StdoutPipe returns a reader connected to command standard output.
func (c *Command) StdoutPipe() (io.ReadCloser, error) {
	if c.Stdout != nil {
		return nil, errors.New("stdout already configured")
	}
	reader, writer := io.Pipe()
	c.Stdout = writer
	c.pipeWriter = writer
	return reader, nil
}

// Start begins an asynchronous root-process invocation.
func (c *Command) Start() error {
	if c == nil {
		return errors.New("command is nil")
	}
	if c.done != nil {
		return errors.New("command already started")
	}
	parent := c.ctx
	if parent == nil {
		parent = context.Background()
	}
	c.ctx, c.cancel = context.WithCancel(parent)
	c.done = make(chan struct{})
	go func() {
		err := c.Run()
		if c.pipeWriter != nil {
			_ = c.pipeWriter.CloseWithError(err)
		}
		c.mu.Lock()
		c.err = err
		c.mu.Unlock()
		close(c.done)
	}()
	return nil
}

// Cancel requests graceful command shutdown through its invocation context.
func (c *Command) Cancel() {
	if c != nil && c.cancel != nil {
		c.cancel()
	}
}

// Wait joins a command started with Start.
func (c *Command) Wait() error {
	if c == nil || c.done == nil {
		return errors.New("command was not started")
	}
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// CommandContext prepares one invocation against the harness root process.
func (h *Harness) CommandContext(ctx context.Context, args ...string) *Command {
	return &Command{harness: h, ctx: ctx, args: append([]string(nil), args...)}
}

// Command prepares one invocation with a background context.
func (h *Harness) Command(args ...string) *Command {
	return h.CommandContext(context.Background(), args...)
}

// Run executes the command through root.BuildProcess.
func (c *Command) Run() error {
	if c == nil || c.harness == nil {
		return errors.New("command root process harness is nil")
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	dir := c.Dir
	if strings.TrimSpace(dir) == "" {
		dir, _ = os.Getwd()
	}
	env := c.Env
	if env == nil {
		env = os.Environ()
	}
	stdin := c.Stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	stdout := c.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := c.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	stdinIsTTY := false
	stdoutIsTTY := false
	process, err := root.BuildProcess(ctx, serviceedges.Edges{})
	if err != nil {
		return fmt.Errorf("build root process: %w", err)
	}
	defer func() { _ = process.Close(context.Background()) }()
	return process.Execute(root.Input{
		Args: append([]string{"you"}, c.args...), Env: env, Stdin: stdin,
		Stdout: stdout, Stderr: stderr, Context: ctx, WorkingDirectory: dir,
		StdinIsTTY: &stdinIsTTY, StdoutIsTTY: &stdoutIsTTY,
	})
}

// CombinedOutput executes the command and returns both output streams.
func (c *Command) CombinedOutput() ([]byte, error) {
	var output bytes.Buffer
	c.Stdout = &output
	c.Stderr = &output
	err := c.Run()
	return output.Bytes(), err
}

// Output executes the command and returns standard output.
func (c *Command) Output() ([]byte, error) {
	var stdout bytes.Buffer
	c.Stdout = &stdout
	err := c.Run()
	return stdout.Bytes(), err
}

// ScenarioFailure records enough diagnostics to debug a scenario mismatch.
type ScenarioFailure struct {
	Scenario        string
	Phase           string
	Message         string
	ExitCode        int
	StdoutTail      string
	StderrTail      string
	HomeDir         string
	LogDir          string
	ProcessBoundary string
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

// NewHarness prepares isolated production root-process invocations.
func NewHarness(t testing.TB, repoRoot string) *Harness {
	t.Helper()
	return &Harness{RepoRoot: repoRoot}
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
// a pre-running listener. The root process starts and stops its own API process.
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
	var stdout, stderr bytes.Buffer
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	command := s.harness.CommandContext(ctx, args...)
	command.Dir, command.Env, command.Stdin = s.WorkDir, env, stdin
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	exitCode := 0
	if err != nil {
		exitCode = 1
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
		Phase:           phase,
		Message:         err.Error(),
		ExitCode:        result.ExitCode,
		StdoutTail:      tailBytes([]byte(result.Stdout), defaultMaxLogTailBytes),
		StderrTail:      tailBytes([]byte(result.Stderr), defaultMaxLogTailBytes),
		HomeDir:         s.HomeDir,
		LogDir:          s.LogDir,
		ProcessBoundary: "root.BuildProcess",
	}
}

func tailBytes(data []byte, max int) string {
	if len(data) > max {
		data = data[len(data)-max:]
	}
	return string(data)
}
