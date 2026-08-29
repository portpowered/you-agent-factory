package shell_completion_test

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

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const completionProcessCloseTimeout = 5 * time.Second

var completionProcesses *completionProcessFixture

// TestMain owns the one immutable root-built process used by every completion
// scenario. Each invocation still owns fresh streams, environments, homes,
// and working roots.
func TestMain(m *testing.M) {
	ledger := newCompletionLifecycleLedger()
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build shell-completion functional process: %v\n", err)
		os.Exit(1)
	}
	ledger.processStarted()
	completionProcesses = &completionProcessFixture{process: process, ledger: ledger}

	exitCode := m.Run()
	closeContext, cancel := context.WithTimeout(context.Background(), completionProcessCloseTimeout)
	closeErr := process.Close(closeContext)
	cancel()
	ledger.processClosed()
	if closeErr != nil {
		fmt.Fprintf(os.Stderr, "close shell-completion functional process: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	if ledgerErr := ledger.assertClean(); ledgerErr != nil {
		fmt.Fprintf(os.Stderr, "shell-completion lifecycle ledger: %v\n", ledgerErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	fmt.Fprintf(os.Stderr, "shell-completion lifecycle ledger: %s\n", ledger.summary())
	os.Exit(exitCode)
}

type completionProcessFixture struct {
	process support.ApplicationProcess
	ledger  *completionLifecycleLedger
}

func completionProcess(t testing.TB) *completionProcessFixture {
	t.Helper()
	if completionProcesses == nil {
		t.Fatal("shell-completion functional process was not initialized by TestMain")
	}
	return completionProcesses
}

type completionInvocation struct {
	workingDirectory string
	environment      []string
}

func (fixture *completionProcessFixture) invocation(t testing.TB, withFactory bool) completionInvocation {
	t.Helper()
	workingDirectory := fixture.tempDir(t)
	if withFactory {
		writeShellCompletionFactory(t, workingDirectory)
		if err := os.WriteFile(filepath.Join(workingDirectory, shellFileName), []byte("{}"), 0o600); err != nil {
			t.Fatalf("write shell completion file: %v", err)
		}
	}

	homeDirectory := fixture.tempDir(t)
	return completionInvocation{
		workingDirectory: workingDirectory,
		environment:      fixture.freshEnvironment(t, homeDirectory),
	}
}

func (fixture *completionProcessFixture) tempDir(t testing.TB) string {
	t.Helper()
	directory, err := os.MkdirTemp("", "functional-shell-completion-")
	if err != nil {
		t.Fatalf("create shell-completion temporary root: %v", err)
	}
	if err := fixture.ledger.registerRoot(directory); err != nil {
		_ = os.RemoveAll(directory)
		t.Fatalf("register shell-completion temporary root %q: %v", directory, err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Errorf("remove shell-completion temporary root %q: %v", directory, err)
			return
		}
		if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				t.Errorf("shell-completion temporary root %q still exists after cleanup", directory)
			} else {
				t.Errorf("stat removed shell-completion temporary root %q: %v", directory, err)
			}
			return
		}
		fixture.ledger.rootRemoved(directory)
	})
	return directory
}

func (fixture *completionProcessFixture) freshEnvironment(t testing.TB, home string) []string {
	t.Helper()
	environment := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		switch {
		case strings.EqualFold(name, "HOME"),
			strings.EqualFold(name, "USERPROFILE"),
			strings.EqualFold(name, "HOMEDRIVE"),
			strings.EqualFold(name, "HOMEPATH"),
			strings.EqualFold(name, "XDG_CACHE_HOME"),
			strings.EqualFold(name, "INFINITE_YOU_OMNIVOICE_CACHE_DIR"):
			continue
		default:
			environment = append(environment, entry)
		}
	}
	return append(environment,
		"HOME="+home,
		"USERPROFILE="+home,
		"XDG_CACHE_HOME="+filepath.Join(home, "cache"),
		"INFINITE_YOU_OMNIVOICE_CACHE_DIR="+filepath.Join(home, "omnivoice-cache"),
	)
}

type completionCommandResult struct {
	stdout string
	stderr string
	err    error
}

func executeCompletionCommand(t testing.TB, invocation completionInvocation, args ...string) completionCommandResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := executeCompletionCommandInto(
		t,
		invocation,
		&stdout,
		&stderr,
		args...,
	)
	return completionCommandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func executeCompletionCommandInto(
	t testing.TB,
	invocation completionInvocation,
	stdout, stderr io.Writer,
	args ...string,
) error {
	t.Helper()
	fixture := completionProcess(t)
	finishInvocation := fixture.ledger.beginInvocation()
	defer finishInvocation()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := support.FakeInputs(ctx, append([]string{"you"}, args...))
	inputs.Input.Env = append([]string(nil), invocation.environment...)
	inputs.Input.WorkingDirectory = invocation.workingDirectory
	inputs.Input.Stdout = stdout
	inputs.Input.Stderr = stderr
	return fixture.process.Execute(inputs.Input)
}

type completionLifecycleLedger struct {
	mu                 sync.Mutex
	processStarts      int
	processCloses      int
	activeInvocations  int
	activeOutputBuffer int
	roots              map[string]bool
}

func newCompletionLifecycleLedger() *completionLifecycleLedger {
	return &completionLifecycleLedger{roots: make(map[string]bool)}
}

func (ledger *completionLifecycleLedger) processStarted() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processStarts++
}

func (ledger *completionLifecycleLedger) processClosed() {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.processCloses++
}

func (ledger *completionLifecycleLedger) beginInvocation() func() {
	ledger.mu.Lock()
	ledger.activeInvocations++
	ledger.activeOutputBuffer++
	ledger.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			ledger.mu.Lock()
			ledger.activeInvocations--
			ledger.activeOutputBuffer--
			ledger.mu.Unlock()
		})
	}
}

func (ledger *completionLifecycleLedger) registerRoot(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("normalize shell-completion root %q: %w", path, err)
	}
	absolute = filepath.Clean(absolute)
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if _, exists := ledger.roots[absolute]; exists {
		return fmt.Errorf("shell-completion root %q registered twice", absolute)
	}
	ledger.roots[absolute] = false
	return nil
}

func (ledger *completionLifecycleLedger) rootRemoved(path string) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if _, exists := ledger.roots[filepath.Clean(absolute)]; exists {
		ledger.roots[filepath.Clean(absolute)] = true
	}
}

func (ledger *completionLifecycleLedger) assertClean() error {
	ledger.mu.Lock()
	processStarts := ledger.processStarts
	processCloses := ledger.processCloses
	activeInvocations := ledger.activeInvocations
	activeOutputBuffer := ledger.activeOutputBuffer
	roots := make(map[string]bool, len(ledger.roots))
	for path, removed := range ledger.roots {
		roots[path] = removed
	}
	ledger.mu.Unlock()

	var problems []error
	if processStarts != 1 {
		problems = append(problems, fmt.Errorf("process starts = %d, want 1", processStarts))
	}
	if processCloses != 1 {
		problems = append(problems, fmt.Errorf("process closes = %d, want 1", processCloses))
	}
	if activeInvocations != 0 {
		problems = append(problems, fmt.Errorf("active invocations = %d, want 0", activeInvocations))
	}
	if activeOutputBuffer != 0 {
		problems = append(problems, fmt.Errorf("active output buffers = %d, want 0", activeOutputBuffer))
	}
	for path, removed := range roots {
		if !removed {
			problems = append(problems, fmt.Errorf("shell-completion temporary root %q was not removed", path))
		}
	}
	return errors.Join(problems...)
}

func (ledger *completionLifecycleLedger) summary() string {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	removed := 0
	for _, rootRemoved := range ledger.roots {
		if rootRemoved {
			removed++
		}
	}
	return fmt.Sprintf(
		"process_starts=%d process_closes=%d active_invocations=%d active_output_buffers=%d active_sessions=0 active_routes=0 roots_removed=%d/%d",
		ledger.processStarts, ledger.processCloses, ledger.activeInvocations,
		ledger.activeOutputBuffer, removed, len(ledger.roots),
	)
}
