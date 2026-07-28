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
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	cursor "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/cursor"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCommandEffect_SubmitsOnlyOversizedWindowsPromptsThroughFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		operatingSystem string
		prompt          string
		wantFile        bool
	}{
		{name: "windows below limit", operatingSystem: "windows", prompt: strings.Repeat("x", cursor.CursorWindowsPromptArgumentLimit-1)},
		{name: "windows at limit", operatingSystem: "windows", prompt: strings.Repeat("x", cursor.CursorWindowsPromptArgumentLimit)},
		{name: "linux above limit", operatingSystem: "linux", prompt: strings.Repeat("x", cursor.CursorWindowsPromptArgumentLimit+1)},
		{
			name:            "windows non BMP above UTF16 limit",
			operatingSystem: "windows",
			prompt:          strings.Repeat("x", cursor.CursorWindowsPromptArgumentLimit-1) + "😀",
			wantFile:        true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			files := newPromptFileSystem()
			runner := &recordingCommandRunner{}
			effect := cursor.NewCommandEffect(runner, cursor.CommandEffectOptions{
				OperatingSystem: test.operatingSystem,
				TemporaryDir:    `C:\cursor-temp`,
				TemporaryFiles:  files,
			})
			_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
				Provider:    providers.IDCursor,
				AttemptID:   "attempt-prompt-file",
				UserMessage: test.prompt,
			}, func([]byte) error { return nil })
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if runner.calls != 1 {
				t.Fatalf("runner calls = %d, want 1", runner.calls)
			}
			gotPrompt := runner.last.Args[len(runner.last.Args)-1]
			if test.wantFile {
				if gotPrompt != "@"+files.file.path {
					t.Fatalf("prompt argument = %q, want @file", gotPrompt)
				}
				if files.created != 1 || files.file.content != test.prompt {
					t.Fatalf("temporary file = %#v, create count = %d", files.file, files.created)
				}
				if files.file.closes != 1 || files.removes != 1 {
					t.Fatalf("cleanup = (closes %d, removes %d), want once", files.file.closes, files.removes)
				}
				return
			}
			if gotPrompt != test.prompt {
				t.Fatalf("prompt argument = %q, want direct prompt", gotPrompt)
			}
			if files.created != 0 {
				t.Fatalf("temporary creates = %d, want none", files.created)
			}
		})
	}
}

func TestCommandEffect_WindowsLongPromptPreservesArgumentsAndCleansExactFileOnce(t *testing.T) {
	t.Parallel()

	files := newPromptFileSystem()
	runner := &recordingCommandRunner{}
	effect := cursor.NewCommandEffect(runner, cursor.CommandEffectOptions{
		OperatingSystem: " WINDOWS ",
		TemporaryDir:    `C:\cursor-temp`,
		TemporaryFiles:  files,
	})
	request := providers.ExecuteRequest{
		Provider:         providers.IDCursor,
		AttemptID:        "attempt-long-prompt",
		Model:            "cursor-model",
		WorkingDirectory: `C:\workspace`,
		UserMessage:      strings.Repeat("long prompt ", cursor.CursorWindowsPromptArgumentLimit),
		SkipPermissions:  true,
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDCursor,
			Kind:     providers.SessionIDKind,
			ID:       "cursor-session",
		},
	}
	_, err := effect.Execute(context.Background(), request, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantArgs := []string{
		"-f", "-p", "--model", "cursor-model", "--resume", "cursor-session",
		"--workspace", `C:\workspace`,
		"--output-format", "stream-json",
		"--stream-partial-output", "@" + files.file.path,
	}
	if !reflect.DeepEqual(runner.last.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", runner.last.Args, wantArgs)
	}
	if files.directory != `C:\cursor-temp` || files.pattern != "cursor_prompt_*.md" {
		t.Fatalf("CreateTemp(%q, %q)", files.directory, files.pattern)
	}
	if files.file.closes != 1 || files.removes != 1 || files.removedPath != files.file.path {
		t.Fatalf(
			"cleanup = (closes %d, removes %d, path %q), want once for %q",
			files.file.closes, files.removes, files.removedPath, files.file.path,
		)
	}
}

func TestCommandEffect_LongPromptFailuresFailClosedAndCleanCreatedPath(t *testing.T) {
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
			name: "write failure", configure: func(files *promptFileSystem) {
				files.file.writeErr = errors.New("write failed at private path")
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
			runner := &recordingCommandRunner{}
			effect := cursor.NewCommandEffect(runner, cursor.CommandEffectOptions{
				OperatingSystem: "windows",
				TemporaryFiles:  files,
			})
			_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
				Provider:    providers.IDCursor,
				AttemptID:   "attempt-prompt-failure",
				UserMessage: strings.Repeat("x", cursor.CursorWindowsPromptArgumentLimit+1),
			}, func([]byte) error { return nil })
			nativeErr := unwrapNativeError(err)
			if nativeErr == nil || nativeErr.Error() != test.wantMessage {
				t.Fatalf("Execute() error = %v, want safe error %q", err, test.wantMessage)
			}
			if strings.Contains(nativeErr.Error(), "private") || runner.calls != 0 {
				t.Fatalf("unsafe or runnable failure result = (calls %d, %q)", runner.calls, nativeErr)
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

func TestCommandEffect_LongPromptRequiresPlatformEffectsAfterCapabilityValidation(t *testing.T) {
	t.Parallel()

	longPrompt := strings.Repeat("x", cursor.CursorWindowsPromptArgumentLimit+1)
	t.Run("missing operating system", func(t *testing.T) {
		t.Parallel()
		runner := &recordingCommandRunner{}
		effect := cursor.NewCommandEffect(runner)
		_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
			Provider:    providers.IDCursor,
			AttemptID:   "attempt-missing-os",
			UserMessage: longPrompt,
		}, func([]byte) error { return nil })
		nativeErr := unwrapNativeError(err)
		if nativeErr == nil || nativeErr.Error() != "Cursor operating system is required to submit an oversized prompt." {
			t.Fatalf("Execute() error = %v", err)
		}
	})
	t.Run("missing temporary files", func(t *testing.T) {
		t.Parallel()
		runner := &recordingCommandRunner{}
		effect := cursor.NewCommandEffect(runner, cursor.CommandEffectOptions{OperatingSystem: "windows"})
		_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
			Provider:    providers.IDCursor,
			AttemptID:   "attempt-missing-files",
			UserMessage: longPrompt,
		}, func([]byte) error { return nil })
		nativeErr := unwrapNativeError(err)
		if nativeErr == nil || nativeErr.Error() != "Cursor temporary-file support is required to submit an oversized prompt." {
			t.Fatalf("Execute() error = %v", err)
		}
	})
	t.Run("unsupported capability before IO", func(t *testing.T) {
		t.Parallel()
		files := newPromptFileSystem()
		runner := &recordingCommandRunner{}
		effect := cursor.NewCommandEffect(runner, cursor.CommandEffectOptions{
			OperatingSystem: "windows",
			TemporaryFiles:  files,
		})
		_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
			Provider:     providers.IDCursor,
			AttemptID:    "attempt-structured-output",
			UserMessage:  longPrompt,
			OutputSchema: `{"type":"object"}`,
		}, func([]byte) error { return nil })
		if err == nil || files.created != 0 {
			t.Fatalf("Execute() error = %v, temporary creates = %d", err, files.created)
		}
	})
}

type recordingCommandRunner struct {
	calls int
	last  workers.CommandRequest
}

func (r *recordingCommandRunner) Run(
	_ context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	r.calls++
	r.last = request
	return workers.CommandResult{}, nil
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
	mu        sync.Mutex
	path      string
	content   string
	writeErr  error
	closeErr  error
	closes    int
}

func (f *promptTemporaryFile) Name() string { return f.path }

func (f *promptTemporaryFile) WriteString(value string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.content = value
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(value), nil
}

func (f *promptTemporaryFile) Close() error {
	f.mu.Lock()
	f.closes++
	err := f.closeErr
	f.mu.Unlock()
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
func (c *terminalContext) Done() <-chan struct{}         { return c.done }
func (c *terminalContext) Value(any) any                 { return nil }
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

func unwrapNativeError(err error) error {
	if err == nil {
		return nil
	}
	var failure execution.AttemptFailure
	if errors.As(err, &failure) && failure.NativeError != nil {
		return failure.NativeError
	}
	return err
}
