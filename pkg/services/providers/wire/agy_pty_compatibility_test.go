package wire

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const legacyAgyPTYModel = "gemini-3.6-flash-high"

// TestAgyLegacyPTYCompatibilityThroughProvidersWire preserves package-level
// evidence for the public Providers PTY compatibility seam. Root composition
// intentionally selects the canonical command-runner effect whenever that edge
// is present; the root wire test owns that selection proof.
func TestAgyLegacyPTYCompatibilityThroughProvidersWire(t *testing.T) {
	t.Parallel()

	t.Run("success cleans output and preserves argv", func(t *testing.T) {
		executable := writeLegacyAgyExecutable(t)
		host := &legacyAgyPTYHost{
			stdout: []byte("\x1b[32mlegacy PTY response COMPLETE\x1b[0m\n"),
		}
		service := newLegacyAgyProvidersService(t, host, executable)
		factoryRoot := t.TempDir()
		prompt := "preserve this prompt as one argv element"

		result, err := service.Execute(context.Background(), legacyAgyRequest(factoryRoot, prompt))
		if err != nil {
			t.Fatalf("Providers.Execute() error = %v", err)
		}
		if result.Content != "legacy PTY response COMPLETE" {
			t.Fatalf("Providers.Execute() content = %q, want cleaned response", result.Content)
		}

		launch := host.lastLaunch()
		wantArgv := []string{executable, "chat", "--headless", "--model", legacyAgyPTYModel, prompt}
		if !reflect.DeepEqual(launch.Argv, wantArgv) {
			t.Fatalf("PTY argv = %#v, want %#v", launch.Argv, wantArgv)
		}
		if launch.Executable != executable {
			t.Fatalf("PTY executable = %q, want %q", launch.Executable, executable)
		}
		if launch.WorkDir != factoryRoot {
			t.Fatalf("PTY workdir = %q, want %q", launch.WorkDir, factoryRoot)
		}
		if host.startCount() != 1 {
			t.Fatalf("PTY starts = %d, want 1", host.startCount())
		}
	})

	t.Run("native auth failure is declared", func(t *testing.T) {
		executable := writeLegacyAgyExecutable(t)
		host := &legacyAgyPTYHost{
			stdout:   []byte("authentication failed: invalid api key"),
			exitCode: 1,
		}
		service := newLegacyAgyProvidersService(t, host, executable)

		_, err := service.Execute(context.Background(), legacyAgyRequest(t.TempDir(), "authenticate"))
		var failure providers.ExecuteFailure
		if !errors.As(err, &failure) {
			t.Fatalf("Providers.Execute() error = %v, want declared failure", err)
		}
		if failure.Kind != providers.ExecuteFailureKindAuthentication {
			t.Fatalf("failure kind = %q, want authentication", failure.Kind)
		}
		if failure.Message != "Agy authentication failed." {
			t.Fatalf("failure message = %q, want canonical auth message", failure.Message)
		}
		if host.startCount() != 1 {
			t.Fatalf("PTY starts = %d, want 1", host.startCount())
		}
	})

	t.Run("deadline terminates native process", func(t *testing.T) {
		executable := writeLegacyAgyExecutable(t)
		host := &legacyAgyPTYHost{waitForTerminate: true}
		service := newLegacyAgyProvidersService(t, host, executable)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		_, err := service.Execute(ctx, legacyAgyRequest(t.TempDir(), "wait for deadline"))
		var failure providers.ExecuteFailure
		if !errors.As(err, &failure) {
			t.Fatalf("Providers.Execute() error = %v, want declared timeout", err)
		}
		if failure.Kind != providers.ExecuteFailureKindTimeout {
			t.Fatalf("failure kind = %q, want timeout", failure.Kind)
		}
		if host.startCount() != 1 {
			t.Fatalf("PTY starts = %d, want 1", host.startCount())
		}
	})
}

func newLegacyAgyProvidersService(
	t *testing.T,
	host *legacyAgyPTYHost,
	executable string,
) providers.Service {
	t.Helper()

	clock := platformclock.NewDeterministic(
		time.Date(2026, time.July, 28, 0, 0, 0, 0, time.UTC),
		time.Millisecond,
	)
	clock.SetTick(1)
	allocator, err := NewAgyPTYAllocator(host, clock, platformclock.Real{})
	if err != nil {
		t.Fatalf("NewAgyPTYAllocator() error = %v", err)
	}
	service, err := NewService(IdentityCatalogProbe,
		platformclock.Real{}, logging.NoopLogger{}, nil, nil, nil,
		NewAgyPTYEffect(allocator, legacyAgyExecutableLocator{path: executable}, platformfilesystem.Local{}, clock, AgyPTYPolicy{}),
		nil,
		nil)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func legacyAgyRequest(workingDirectory, prompt string) providers.ExecuteRequest {
	return providers.ExecuteRequest{
		Provider:           providers.IDAntigravity,
		AttemptID:          "agy-legacy-pty-functional",
		Model:              legacyAgyPTYModel,
		UserMessage:        prompt,
		WorkingDirectory:   workingDirectory,
		SkipPermissions:    true,
		ProcessEnvironment: []string{"PATH=/usr/bin"},
	}
}

func writeLegacyAgyExecutable(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "agy")
	if err := os.WriteFile(path, []byte("agy fixture executable\n"), 0o755); err != nil {
		t.Fatalf("write Agy fixture executable: %v", err)
	}
	return path
}

type legacyAgyExecutableLocator struct {
	path string
}

func (locator legacyAgyExecutableLocator) LookPath(file string) (string, error) {
	if file == "agy" {
		return locator.path, nil
	}
	return "", fmt.Errorf("executable %q not found", file)
}

type legacyAgyPTYHost struct {
	mu               sync.Mutex
	stdout           []byte
	exitCode         int
	waitForTerminate bool
	starts           int
	launches         []platformpty.ProcessLaunch
}

func (host *legacyAgyPTYHost) Allocate(context.Context) (platformpty.Allocation, error) {
	return legacyAgyPTYAllocation{}, nil
}

func (host *legacyAgyPTYHost) Start(
	launch platformpty.ProcessLaunch,
	_ platformpty.Allocation,
) (platformpty.Process, io.ReadCloser, error) {
	host.mu.Lock()
	host.starts++
	host.launches = append(host.launches, platformpty.ProcessLaunch{
		Executable: launch.Executable,
		Argv:       append([]string(nil), launch.Argv...),
		WorkDir:    launch.WorkDir,
		Env:        append([]string(nil), launch.Env...),
	})
	stdout := append([]byte(nil), host.stdout...)
	exitCode := host.exitCode
	waitForTerminate := host.waitForTerminate
	host.mu.Unlock()

	return newLegacyAgyPTYProcess(exitCode, waitForTerminate), io.NopCloser(bytes.NewReader(stdout)), nil
}

func (host *legacyAgyPTYHost) startCount() int {
	host.mu.Lock()
	defer host.mu.Unlock()
	return host.starts
}

func (host *legacyAgyPTYHost) lastLaunch() platformpty.ProcessLaunch {
	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.launches) == 0 {
		return platformpty.ProcessLaunch{}
	}
	launch := host.launches[len(host.launches)-1]
	launch.Argv = append([]string(nil), launch.Argv...)
	return launch
}

type legacyAgyPTYAllocation struct{}

func (legacyAgyPTYAllocation) Kind() platformpty.Kind { return platformpty.KindPOSIX }
func (legacyAgyPTYAllocation) Close() error           { return nil }

type legacyAgyPTYProcess struct {
	exitCode         int
	waitForTerminate bool
	done             chan struct{}
	once             sync.Once
}

func newLegacyAgyPTYProcess(exitCode int, waitForTerminate bool) *legacyAgyPTYProcess {
	return &legacyAgyPTYProcess{
		exitCode:         exitCode,
		waitForTerminate: waitForTerminate,
		done:             make(chan struct{}),
	}
}

func (process *legacyAgyPTYProcess) Wait() error {
	if process.waitForTerminate {
		<-process.done
	}
	return nil
}

func (process *legacyAgyPTYProcess) Terminate() error {
	process.once.Do(func() { close(process.done) })
	return nil
}

func (*legacyAgyPTYProcess) Close()   {}
func (*legacyAgyPTYProcess) PID() int { return 0 }

func (process *legacyAgyPTYProcess) ExitCode() int {
	return process.exitCode
}

var _ platformprocess.ExecutableLocator = legacyAgyExecutableLocator{}
var _ platformpty.Host = (*legacyAgyPTYHost)(nil)
var _ platformpty.Process = (*legacyAgyPTYProcess)(nil)

func TestNewServicePreservesSelectedClockForCodexAndClaude(t *testing.T) {
	t.Parallel()
	for _, id := range []providers.ID{providers.IDCodex, providers.IDClaude} {
		t.Run(id.String(), func(t *testing.T) {
			t.Parallel()
			clock := platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond)
			runner := &clockCommandRunner{clock: clock}
			peer := &clockCommandRunner{clock: clock}
			codexRunner, claudeRunner := runner, peer
			if id == providers.IDClaude {
				codexRunner, claudeRunner = peer, runner
			}
			root, err := NewService(IdentityCatalogProbe,
				platformclock.Real{}, logging.NoopLogger{}, nil, nil, nil,
				nil,
				NewCodexEffect(codexRunner, clock),
				NewClaudeEffect(claudeRunner, clock))
			if err != nil {
				t.Fatal(err)
			}
			if runner.calls != 0 {
				t.Fatal("construction started a provider command")
			}
			if peer.calls != 0 {
				t.Fatal("construction started a peer command")
			}
			result, err := root.Execute(t.Context(), providers.ExecuteRequest{AttemptID: "clock-attempt", Provider: id, UserMessage: "clock proof", WorkingDirectory: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			if result.Content != "selected clock result" || result.Diagnostics == nil || result.Diagnostics.DurationMillis != 37 {
				t.Fatalf("result = %#v, want content and supplied 37ms duration", result)
			}
			if runner.calls != 1 {
				t.Fatalf("command calls = %d, want one", runner.calls)
			}
			if peer.calls != 0 {
				t.Fatalf("peer command calls = %d, want isolated effects", peer.calls)
			}
		})
	}
}

type clockCommandRunner struct {
	clock *platformclock.Deterministic
	calls int
}

func (runner *clockCommandRunner) Run(_ context.Context, request CommandRequest) (CommandResult, error) {
	runner.calls++
	runner.clock.SetTick(37)
	output := "selected clock result"
	switch request.Command {
	case "codex":
		output = `{"type":"thread.started","thread_id":"clock-thread"}` + "\n" +
			`{"type":"item.completed","item":{"id":"clock-message","type":"agent_message","text":"selected clock result"}}` + "\n" +
			`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}` + "\n"
	case "claude":
		output = `{"type":"result","subtype":"success","is_error":false,"result":"selected clock result","session_id":"clock-session"}` + "\n"
	}
	return CommandResult{Stdout: []byte(output)}, nil
}
