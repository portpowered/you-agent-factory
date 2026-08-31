package commands_test

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
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const packageCommandRuntimeCloseTimeout = 5 * time.Second

var commandPackageRuntime *commandRuntime

// TestMain owns the one command-package application graph. Its wiring is
// shared between invocations; command-specific directories, environments,
// streams, and root inputs remain fresh at the CLI boundary below.
func TestMain(m *testing.M) {
	runtime, err := newCommandRuntime()
	if err != nil {
		fmt.Fprintf(os.Stderr, "command functional runtime: build root process: %v\n", err)
		os.Exit(1)
	}
	commandPackageRuntime = runtime

	exitCode := m.Run()
	closeContext, cancel := context.WithTimeout(context.Background(), packageCommandRuntimeCloseTimeout)
	closeErr := runtime.Close(closeContext)
	cancel()
	if closeErr != nil {
		fmt.Fprintf(os.Stderr, "command functional runtime: close root process: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

type commandRuntime struct {
	process support.ApplicationProcess

	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

func newCommandRuntime() (*commandRuntime, error) {
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		return nil, err
	}
	return &commandRuntime{process: process}, nil
}

// Close releases the package-owned application graph exactly once. Execute
// holds the same mutex for its full call, so TestMain cannot close the graph
// while a command is still using it.
func (r *commandRuntime) Close(ctx context.Context) error {
	if r == nil || r.process == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.closeOnce.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.closed = true
		r.closeErr = r.process.Close(ctx)
	})
	return r.closeErr
}

func (r *commandRuntime) execute(input root.Input) error {
	if r == nil || r.process == nil {
		return errors.New("command functional runtime is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("command functional runtime is closed")
	}
	return r.process.Execute(input)
}

// command is the small in-process command surface needed by these functional
// tests. It deliberately mirrors the existing acceptance harness contract and
// still crosses the production root.Process.Execute boundary.
type command struct {
	runtime *commandRuntime
	ctx     context.Context
	args    []string

	Dir    string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func (r *commandRuntime) CommandContext(ctx context.Context, args ...string) *command {
	return &command{runtime: r, ctx: ctx, args: append([]string(nil), args...)}
}

func (c *command) Run() error {
	if c == nil || c.runtime == nil {
		return errors.New("command functional runtime is nil")
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
	return c.runtime.execute(root.Input{
		Args:             append([]string{"you"}, c.args...),
		Env:              append([]string(nil), env...),
		Stdin:            stdin,
		Stdout:           stdout,
		Stderr:           stderr,
		Context:          ctx,
		WorkingDirectory: dir,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
}

func (c *command) CombinedOutput() ([]byte, error) {
	var output bytes.Buffer
	c.Stdout = &output
	c.Stderr = &output
	err := c.Run()
	return output.Bytes(), err
}

func (c *command) Output() ([]byte, error) {
	var stdout bytes.Buffer
	c.Stdout = &stdout
	err := c.Run()
	return stdout.Bytes(), err
}

// commandScenario supplies fresh invocation-local state without changing the
// working directory selected by a witness (which may intentionally be an
// authored Factory or unrelated directory).
type commandScenario struct {
	HomeDir string
	LogDir  string
	WorkDir string
	Env     []string
}

func newCommandScenario(t testing.TB, workingDir string) *commandScenario {
	t.Helper()
	rootDir := t.TempDir()
	scenario, err := makeCommandScenario(rootDir, workingDir)
	if err != nil {
		t.Fatalf("create command scenario: %v", err)
	}
	return scenario
}

func makeCommandScenario(rootDir, workingDir string) (*commandScenario, error) {
	homeDir := filepath.Join(rootDir, "home")
	logDir := filepath.Join(homeDir, ".you-agent-factory", "logs")
	workDir := workingDir
	if strings.TrimSpace(workDir) == "" {
		workDir = filepath.Join(rootDir, "work")
	}
	for label, path := range map[string]string{
		"home": homeDir,
		"logs": logDir,
		"work": workDir,
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return nil, fmt.Errorf("create command scenario %s directory %q: %w", label, path, err)
		}
	}
	return &commandScenario{
		HomeDir: homeDir,
		LogDir:  logDir,
		WorkDir: workDir,
		Env:     builtcliacceptance.ProcessEnvForIsolatedHome(homeDir),
	}, nil
}

func newCommandForScenario(
	t testing.TB,
	runtime *commandRuntime,
	ctx context.Context,
	workingDir string,
	args ...string,
) *command {
	t.Helper()
	scenario := newCommandScenario(t, workingDir)
	command := runtime.CommandContext(ctx, args...)
	command.Dir = scenario.WorkDir
	command.Env = scenario.Env
	return command
}

// newEphemeralCommandForScenario is the non-testing.TB variant used by the
// shared remote command helper. Its temporary state is removed immediately
// after the invocation returns, while the caller-selected working directory
// remains owned by that witness.
func newEphemeralCommandForScenario(
	runtime *commandRuntime,
	ctx context.Context,
	workingDir string,
	args ...string,
) (*command, func(), error) {
	rootDir, err := os.MkdirTemp("", "infinite-you-cli-command-")
	if err != nil {
		return nil, func() {}, fmt.Errorf("create command scenario root: %w", err)
	}
	scenario, err := makeCommandScenario(rootDir, workingDir)
	if err != nil {
		_ = os.RemoveAll(rootDir)
		return nil, func() {}, err
	}
	command := runtime.CommandContext(ctx, args...)
	command.Dir = scenario.WorkDir
	command.Env = scenario.Env
	return command, func() { _ = os.RemoveAll(rootDir) }, nil
}
