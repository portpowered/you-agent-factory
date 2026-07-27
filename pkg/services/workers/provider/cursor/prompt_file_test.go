package cursor_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	cursorpkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/cursor"
)

func TestBuildCommand_SubmitsOnlyOversizedWindowsPromptsThroughFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		operatingSystem string
		prompt          string
		wantFile        bool
	}{
		{name: "windows below limit", operatingSystem: "windows", prompt: strings.Repeat("x", cursorpkg.CursorWindowsPromptArgumentLimit-1)},
		{name: "windows at limit", operatingSystem: "windows", prompt: strings.Repeat("x", cursorpkg.CursorWindowsPromptArgumentLimit)},
		{name: "linux above limit", operatingSystem: "linux", prompt: strings.Repeat("x", cursorpkg.CursorWindowsPromptArgumentLimit+1)},
		{
			name:            "windows non BMP above UTF16 limit",
			operatingSystem: "windows",
			prompt:          strings.Repeat("x", cursorpkg.CursorWindowsPromptArgumentLimit-1) + "😀",
			wantFile:        true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			files := newPromptFileSystem()
			built, err := cursorAdapter(test.operatingSystem, files).BuildCommand(
				context.Background(),
				commandContext(test.prompt),
			)
			if err != nil {
				t.Fatalf("BuildCommand() error = %v", err)
			}
			gotPrompt := built.Request.Args[len(built.Request.Args)-1]
			if test.wantFile {
				if gotPrompt != "@"+files.file.path {
					t.Fatalf("prompt argument = %q, want @file", gotPrompt)
				}
				if files.created != 1 || files.file.content != test.prompt {
					t.Fatalf("temporary file = %#v, create count = %d", files.file, files.created)
				}
				built.Cleanup()
				return
			}
			if gotPrompt != test.prompt {
				t.Fatalf("prompt argument = %q, want direct prompt", gotPrompt)
			}
			if files.created != 0 || built.Cleanup != nil {
				t.Fatalf("temporary effects = %d, cleanup = %v; want none", files.created, built.Cleanup != nil)
			}
		})
	}
}

func TestBuildCommand_WindowsLongPromptPreservesArgumentsAndCleansExactFileOnce(t *testing.T) {
	t.Parallel()

	files := newPromptFileSystem()
	request := workerexecution.ProviderInferenceRequest{
		Model:            "cursor-model",
		SessionID:        "cursor-session",
		WorkingDirectory: `C:\workspace`,
		UserMessage:      strings.Repeat("long prompt ", cursorpkg.CursorWindowsPromptArgumentLimit),
	}
	built, err := cursorAdapter(" WINDOWS ", files).BuildCommand(context.Background(), adapter.CommandContext{
		Request: request, SkipPermissions: true,
	})
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	wantArgs := []string{
		"-f", "-p", "--model", "cursor-model", "--resume", "cursor-session",
		"--workspace", `C:\workspace`,
		"--output-format", cursorpkg.CursorOutputFormatStreamJSON,
		"--stream-partial-output", "@" + files.file.path,
	}
	if !reflect.DeepEqual(built.Request.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", built.Request.Args, wantArgs)
	}
	if files.directory != `C:\cursor-temp` || files.pattern != "cursor_prompt_*.md" {
		t.Fatalf("CreateTemp(%q, %q)", files.directory, files.pattern)
	}
	if files.file.closes != 1 || files.removes != 0 {
		t.Fatalf("before cleanup: closes = %d, removes = %d", files.file.closes, files.removes)
	}

	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			built.Cleanup()
		}()
	}
	wait.Wait()
	if files.file.closes != 1 || files.removes != 1 || files.removedPath != files.file.path {
		t.Fatalf(
			"after racing cleanup: closes = %d, removes = %d, path = %q",
			files.file.closes, files.removes, files.removedPath,
		)
	}
}

func TestBuildCommand_LongPromptFailuresFailClosedAndCleanCreatedPath(t *testing.T) {
	t.Parallel()

	createFailure := errors.New("private C:\\operator\\prompt path")
	tests := []struct {
		name        string
		configure   func(*promptFileSystem)
		wantMessage string
		wantClose   int
		wantRemove  int
	}{
		{
			name: "create failure", configure: func(files *promptFileSystem) {
				files.createErr = createFailure
				files.returnFileWithCreateError = false
			},
			wantMessage: "Cursor could not create a temporary file for the oversized prompt.",
		},
		{
			name: "create failure with path", configure: func(files *promptFileSystem) {
				files.createErr = createFailure
				files.returnFileWithCreateError = true
			},
			wantMessage: "Cursor could not create a temporary file for the oversized prompt.",
			wantClose:   1, wantRemove: 1,
		},
		{
			name: "write failure", configure: func(files *promptFileSystem) {
				files.file.writeErr = errors.New("write failed at private path")
			},
			wantMessage: "Cursor could not write the oversized prompt to a temporary file.",
			wantClose:   1, wantRemove: 1,
		},
		{
			name: "short write", configure: func(files *promptFileSystem) {
				files.file.shortWrite = true
			},
			wantMessage: "Cursor could not write the oversized prompt to a temporary file.",
			wantClose:   1, wantRemove: 1,
		},
		{
			name: "close failure", configure: func(files *promptFileSystem) {
				files.file.closeErr = errors.New("close failed at private path")
			},
			wantMessage: "Cursor could not close the temporary file for the oversized prompt.",
			wantClose:   1, wantRemove: 1,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			files := newPromptFileSystem()
			test.configure(files)
			built, err := cursorAdapter("windows", files).BuildCommand(
				context.Background(),
				commandContext(strings.Repeat("x", cursorpkg.CursorWindowsPromptArgumentLimit+1)),
			)
			if err == nil || err.Error() != test.wantMessage {
				t.Fatalf("BuildCommand() = (%#v, %v), want safe error %q", built, err, test.wantMessage)
			}
			if strings.Contains(err.Error(), "private") || len(built.Request.Args) != 0 {
				t.Fatalf("unsafe or runnable failure result = (%#v, %q)", built, err)
			}
			if files.file.closes != test.wantClose || files.removes != test.wantRemove {
				t.Fatalf(
					"cleanup = (closes %d, removes %d), want (%d, %d)",
					files.file.closes, files.removes, test.wantClose, test.wantRemove,
				)
			}
		})
	}
}

func TestBuildCommand_LongPromptRequiresPlatformEffectsAfterCapabilityValidation(t *testing.T) {
	t.Parallel()

	longPrompt := strings.Repeat("x", cursorpkg.CursorWindowsPromptArgumentLimit+1)
	t.Run("missing operating system", func(t *testing.T) {
		t.Parallel()
		_, err := cursorpkg.NewAdapter().BuildCommand(context.Background(), commandContext(longPrompt))
		if err == nil || err.Error() != "Cursor operating system is required to submit an oversized prompt." {
			t.Fatalf("BuildCommand() error = %v", err)
		}
	})
	t.Run("missing temporary files", func(t *testing.T) {
		t.Parallel()
		_, err := cursorAdapter("windows", nil).BuildCommand(context.Background(), commandContext(longPrompt))
		if err == nil || err.Error() != "Cursor temporary-file support is required to submit an oversized prompt." {
			t.Fatalf("BuildCommand() error = %v", err)
		}
	})
	t.Run("unsupported capability before IO", func(t *testing.T) {
		t.Parallel()
		files := newPromptFileSystem()
		input := commandContext(longPrompt)
		input.Request.RequiredOptionalCapabilities = []workerexecution.RunnerOptionalCapability{
			workerexecution.RunnerOptionalCapabilityStructuredOutput,
		}
		_, err := cursorAdapter("windows", files).BuildCommand(context.Background(), input)
		if err == nil || files.created != 0 {
			t.Fatalf("BuildCommand() error = %v, temporary creates = %d", err, files.created)
		}
	})
}

func TestBuildCommand_CancellationDuringSetupCleansBeforeReturning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr error
	}{
		{name: "cancellation", wantErr: context.Canceled},
		{name: "deadline", wantErr: context.DeadlineExceeded},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := newTerminalContext()
			files := newPromptFileSystem()
			files.file.closeHook = func() { ctx.terminate(test.wantErr) }
			_, err := cursorAdapter("windows", files).BuildCommand(
				ctx,
				commandContext(strings.Repeat("x", cursorpkg.CursorWindowsPromptArgumentLimit+1)),
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("BuildCommand() error = %v, want %v", err, test.wantErr)
			}
			if files.file.closes != 1 || files.removes != 1 {
				t.Fatalf("cleanup = (closes %d, removes %d), want once", files.file.closes, files.removes)
			}
		})
	}
}

func TestBuildCommand_CleanupCoversCommandTerminalPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		run     func(*terminalContext) (workerprocess.CommandResult, error)
		outcome adapter.CommandOutcome
	}{
		{
			name: "success",
			run: func(*terminalContext) (workerprocess.CommandResult, error) {
				return workerprocess.CommandResult{}, nil
			},
			outcome: adapter.CommandOutcomeCompleted,
		},
		{
			name: "provider failure",
			run: func(*terminalContext) (workerprocess.CommandResult, error) {
				return workerprocess.CommandResult{ExitCode: 1}, errors.New("provider failed")
			},
			outcome: adapter.CommandOutcomeProcessFailed,
		},
		{
			name: "cancellation",
			run: func(ctx *terminalContext) (workerprocess.CommandResult, error) {
				ctx.terminate(context.Canceled)
				return workerprocess.CommandResult{}, context.Canceled
			},
			outcome: adapter.CommandOutcomeCanceled,
		},
		{
			name: "timeout",
			run: func(ctx *terminalContext) (workerprocess.CommandResult, error) {
				ctx.terminate(context.DeadlineExceeded)
				return workerprocess.CommandResult{}, context.DeadlineExceeded
			},
			outcome: adapter.CommandOutcomeCanceled,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			files := newPromptFileSystem()
			ctx := newTerminalContext()
			providerAdapter := lifecycleCursorAdapter{
				Adapter: cursorAdapter("windows", files),
			}
			registry, err := adapter.NewRegistry(providerAdapter)
			if err != nil {
				t.Fatalf("NewRegistry() error = %v", err)
			}
			result, err := adapter.Execute(ctx, registry, lifecycleRunner{
				run: func() (workerprocess.CommandResult, error) {
					return test.run(ctx)
				},
			}, adapter.ExecuteInput{
				Provider: providerAdapter.Identity(),
				Command: commandContext(
					strings.Repeat("x", cursorpkg.CursorWindowsPromptArgumentLimit+1),
				),
			})
			if err != nil {
				t.Fatalf("Execute() lifecycle error = %v", err)
			}
			if result.Outcome != test.outcome {
				t.Fatalf("outcome = %q, want %q", result.Outcome, test.outcome)
			}
			if files.file.closes != 1 || files.removes != 1 {
				t.Fatalf(
					"cleanup = (closes %d, removes %d), want once",
					files.file.closes, files.removes,
				)
			}
		})
	}
}

func cursorAdapter(operatingSystem string, files platformfilesystem.TemporaryFileSystem) *cursorpkg.Adapter {
	return cursorpkg.NewAdapter(cursorpkg.AdapterDependencies{
		OperatingSystem: operatingSystem,
		TemporaryDir:    `C:\cursor-temp`,
		TemporaryFiles:  files,
	})
}

func commandContext(prompt string) adapter.CommandContext {
	return adapter.CommandContext{Request: workerexecution.ProviderInferenceRequest{UserMessage: prompt}}
}

type promptFileSystem struct {
	mu                        sync.Mutex
	file                      *promptTemporaryFile
	createErr                 error
	returnFileWithCreateError bool
	created                   int
	directory                 string
	pattern                   string
	removes                   int
	removedPath               string
}

func newPromptFileSystem() *promptFileSystem {
	files := &promptFileSystem{}
	files.file = &promptTemporaryFile{path: `C:\cursor-temp\cursor_prompt_fixture.md`}
	return files
}

func (f *promptFileSystem) CreateTemp(directory, pattern string) (platformfilesystem.TemporaryFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created++
	f.directory = directory
	f.pattern = pattern
	if f.createErr != nil && !f.returnFileWithCreateError {
		return nil, f.createErr
	}
	return f.file, f.createErr
}

func (f *promptFileSystem) Remove(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes++
	f.removedPath = path
	return nil
}

type promptTemporaryFile struct {
	mu         sync.Mutex
	path       string
	content    string
	writeErr   error
	shortWrite bool
	closeErr   error
	closeHook  func()
	closes     int
}

func (f *promptTemporaryFile) Name() string { return f.path }

func (f *promptTemporaryFile) WriteString(value string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.content = value
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.shortWrite {
		return len(value) - 1, nil
	}
	return len(value), nil
}

func (f *promptTemporaryFile) Close() error {
	f.mu.Lock()
	f.closes++
	hook := f.closeHook
	err := f.closeErr
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	return err
}

var _ platformfilesystem.TemporaryFileSystem = (*promptFileSystem)(nil)
var _ platformfilesystem.TemporaryFile = (*promptTemporaryFile)(nil)

type terminalContext struct {
	mu   sync.Mutex
	done chan struct{}
	err  error
}

func newTerminalContext() *terminalContext {
	return &terminalContext{done: make(chan struct{})}
}

func (c *terminalContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *terminalContext) Done() <-chan struct{}       { return c.done }
func (c *terminalContext) Value(any) any               { return nil }
func (c *terminalContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *terminalContext) terminate(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return
	}
	c.err = err
	close(c.done)
}

var _ context.Context = (*terminalContext)(nil)

type lifecycleCursorAdapter struct {
	*cursorpkg.Adapter
}

func (lifecycleCursorAdapter) Identity() adapter.Identity {
	return adapter.Identity("cursor-lifecycle-test")
}

func (lifecycleCursorAdapter) NewDecoder(
	context.Context,
	adapter.DecoderContext,
) (adapter.Decoder, error) {
	return lifecycleDecoder{}, nil
}

func (lifecycleCursorAdapter) ParseFinal(
	context.Context,
	adapter.FinalParseContext,
) (adapter.FinalParseResult, error) {
	return adapter.FinalParseResult{}, nil
}

func (lifecycleCursorAdapter) Capabilities(
	context.Context,
	adapter.CapabilityContext,
) (adapter.CapabilityResult, error) {
	return adapter.CapabilityResult{}, nil
}

func (lifecycleCursorAdapter) ClassifyFailure(
	context.Context,
	adapter.FailureContext,
) adapter.FailureResult {
	return adapter.FailureResult{}
}

type lifecycleDecoder struct{}

func (lifecycleDecoder) Observe(
	context.Context,
	adapter.Observation,
) (adapter.DecodeResult, error) {
	return adapter.DecodeResult{}, nil
}

func (lifecycleDecoder) Flush(
	context.Context,
	adapter.FlushContext,
) (adapter.DecodeResult, error) {
	return adapter.DecodeResult{}, nil
}

type lifecycleRunner struct {
	run func() (workerprocess.CommandResult, error)
}

func (r lifecycleRunner) Run(
	context.Context,
	workerprocess.CommandRequest,
	func(adapter.Observation) error,
) (workerprocess.CommandResult, error) {
	return r.run()
}

var _ adapter.Adapter = lifecycleCursorAdapter{}
var _ adapter.Decoder = lifecycleDecoder{}
var _ adapter.StreamingCommandRunner = lifecycleRunner{}
