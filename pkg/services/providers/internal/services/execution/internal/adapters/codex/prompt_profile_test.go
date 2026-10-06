package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

type profileFiles struct {
	name                                     string
	content                                  string
	closed                                   bool
	created                                  int
	removed                                  int
	createErr, writeErr, closeErr, removeErr error
	short                                    bool
	projectConfig                            bool
	root                                     string
	baseConfig                               string
	statErr                                  error
}

func (f *profileFiles) CreateTemp(dir, pattern string) (platformfilesystem.TemporaryFile, error) {
	f.created++
	if pattern != "you-prompt-*.config.toml" || filepath.Dir(f.name) != dir {
		return nil, fs.ErrInvalid
	}
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f, nil
}
func (f *profileFiles) Name() string { return f.name }
func (f *profileFiles) WriteString(s string) (int, error) {
	f.content = s
	if f.short {
		return len(s) - 1, nil
	}
	return len(s), f.writeErr
}
func (f *profileFiles) Close() error { f.closed = true; return f.closeErr }
func (f *profileFiles) Remove(name string) error {
	if name != f.name {
		return fs.ErrInvalid
	}
	f.removed++
	return f.removeErr
}
func (f *profileFiles) ReadFile(path string) ([]byte, error) {
	if path == filepath.Join(filepath.Dir(f.name), "config.toml") && f.baseConfig != "" {
		return []byte(f.baseConfig), nil
	}
	if f.root != "" && path == filepath.Join(filepath.Dir(f.root), ".codex", "config.toml") {
		return []byte("developer_instructions=\"unrelated ancestor\""), nil
	}
	if f.projectConfig {
		return []byte("developer_instructions=\"other\""), nil
	}
	return nil, fs.ErrNotExist
}

func (f *profileFiles) Stat(path string) (fs.FileInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	if f.root != "" && path == filepath.Join(f.root, ".git") {
		return nil, nil
	}
	return nil, fs.ErrNotExist
}

func (f *profileFiles) EvalSymlinks(path string) (string, error) { return path, nil }

func TestPromptProfileProjectRootPreservesPrecedence(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"git-root", "custom-root", "cd", "root-stat-error"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			files, request := profileFixture(t)
			files.root = request.WorkingDirectory
			switch scenario {
			case "custom-root":
				files.baseConfig = "project_root_markers=[\".custom\"]"
			case "cd":
				request.Args = append(request.Args, "--cd", filepath.Dir(request.WorkingDirectory))
			case "root-stat-error":
				files.statErr = fs.ErrPermission
			}
			command, err := buildCommand(request)
			if err != nil {
				t.Fatal(err)
			}
			cleanup, err := (promptPreparation{files: files}).prepare(&command, request.ExecuteRequest)
			if scenario == "git-root" {
				if err != nil {
					t.Fatal("unrelated ancestor prevented bounded delivery: ", err)
				}
				cleanup()
			} else {
				var rejected *platformprocess.CommandStartError
				if !errors.As(err, &rejected) || files.created != 0 {
					t.Fatalf("incompatible root launched: %v", err)
				}
			}
		})
	}
}

type profileRunner struct {
	run func(providerservice.CommandRequest) (providerservice.CommandResult, error)
}

func (r profileRunner) Run(_ context.Context, c providerservice.CommandRequest) (providerservice.CommandResult, error) {
	return r.run(c)
}
func (r profileRunner) RunStreaming(_ context.Context, c providerservice.CommandRequest, _ providerservice.OutputChunkObserver) (providerservice.CommandResult, error) {
	return r.run(c)
}

func profileFixture(t *testing.T) (*profileFiles, execution.ContinuationRequest) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "codex-home")
	return &profileFiles{name: filepath.Join(home, "you-prompt-abc.config.toml")}, execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{SystemPrompt: strings.Repeat("large instructions ", 2200), UserMessage: "  user café 😀\r\n\t ",
			WorkingDirectory: root, ProcessEnvironment: []string{"CODEX_HOME=" + home}, Args: []string{"--model", "kept-model"},
		},
		ResumeSession: &providers.SessionRef{ID: "resume-id"},
	}
}

func TestPromptProfileComposedBoundariesPreserveBytesAndOptions(t *testing.T) {
	t.Parallel()
	for _, length := range []int{32766, 32767, 32768} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			t.Parallel()
			files, request := profileFixture(t)
			prefix := "  café 😀\"\\\r\n\t"
			request.SystemPrompt = prefix
			command, err := buildCommand(request)
			if err != nil {
				t.Fatal(err)
			}
			request.SystemPrompt += strings.Repeat("a", length-platformprocess.ComposedCommandLineLength(command.Command, command.Args))
			command, err = buildCommand(request)
			if err != nil {
				t.Fatal(err)
			}
			original := append([]string(nil), command.Args...)
			if got := platformprocess.ComposedCommandLineLength(command.Command, command.Args); got != length {
				t.Fatalf("length=%d want %d", got, length)
			}
			cleanup, err := (promptPreparation{files: files}).prepare(&command, request.ExecuteRequest)
			if err != nil {
				t.Fatal(err)
			}
			if length < 32767 {
				if files.created != 0 || !reflect.DeepEqual(command.Args, original) {
					t.Fatal("below-bound command changed")
				}
			} else {
				if !files.closed || files.created != 1 || platformprocess.ComposedCommandLineLength(command.Command, command.Args) >= 32767 {
					t.Fatal("profile did not close before bounded delivery")
				}
				var decoded string
				value := strings.TrimSuffix(strings.TrimPrefix(files.content, "developer_instructions="), "\n")
				if err := json.Unmarshal([]byte(value), &decoded); err != nil || decoded != request.SystemPrompt {
					t.Fatalf("decoded developer bytes changed: %v", err)
				}
				if !reflect.DeepEqual(command.Args[len(command.Args)-5:], original[len(original)-5:]) {
					t.Fatal("options or resume changed")
				}
			}
			if string(command.Stdin) != request.UserMessage {
				t.Fatal("user bytes changed")
			}
			cleanup()
			if files.removed != files.created {
				t.Fatal("own profile was not removed")
			}
		})
	}
}

func TestPromptProfileLifecycleDoesNotLaunchIncompleteFiles(t *testing.T) {
	t.Parallel()
	for _, fault := range []string{"create", "write", "short-write", "close", "project-config", "selected-profile", "fixed-args"} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			files, request := profileFixture(t)
			cause := errors.New("injected private-file fault")
			switch fault {
			case "create":
				files.createErr = cause
			case "write":
				files.writeErr = cause
			case "short-write":
				files.short = true
			case "close":
				files.closeErr = cause
			case "project-config":
				files.projectConfig = true
			case "selected-profile":
				request.Args = append(request.Args, "--profile=existing")
			case "fixed-args":
				request.Args = append(request.Args, strings.Repeat("x", 32767))
			}
			runner := profileRunner{run: func(providerservice.CommandRequest) (providerservice.CommandResult, error) {
				t.Error("incomplete/incompatible profile launched")
				return providerservice.CommandResult{}, nil
			}}
			_, err := NewCommandEffect(providerservice.CommandRunner{Run: runner.Run, RunStreaming: runner.RunStreaming}, platformclock.Real{}, files, nil).Execute(t.Context(), request, func([]byte) error { return nil })
			var failure providers.ExecuteFailure
			if !errors.As(err, &failure) || failure.Diagnostics == nil || failure.Diagnostics.Metadata["work-failure-type"] != "command_line_too_long" {
				t.Fatalf("failure=%v", err)
			}
			if !strings.Contains(failure.Message, "32767") || strings.Contains(failure.Message, request.SystemPrompt) {
				t.Fatal("failure missing safe size")
			}
			if files.created > 0 && fault != "create" && (files.removed != 1 || !files.closed) {
				t.Fatal("failed preparation retained profile")
			}
		})
	}
}

func TestPromptProfileCleanupAfterEveryRunnerOutcome(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"success", "exit", "error", "timeout", "cancel", "remove-failure"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			files, request := profileFixture(t)
			if outcome == "remove-failure" {
				files.removeErr = errors.New("cannot remove")
			}
			calls := 0
			runner := profileRunner{run: func(command providerservice.CommandRequest) (providerservice.CommandResult, error) {
				calls++
				if !files.closed || files.content == "" || files.removed != 0 || string(command.Stdin) != request.UserMessage {
					t.Fatal("launch before complete file or changed stdin")
				}
				switch outcome {
				case "exit":
					return providerservice.CommandResult{ExitCode: 1}, nil
				case "error":
					return providerservice.CommandResult{}, errors.New("runner error")
				case "timeout":
					return providerservice.CommandResult{}, context.DeadlineExceeded
				case "cancel":
					return providerservice.CommandResult{}, context.Canceled
				}
				return providerservice.CommandResult{}, nil
			}}
			_, err := NewCommandEffect(providerservice.CommandRunner{Run: runner.Run, RunStreaming: runner.RunStreaming}, platformclock.Real{}, files, nil).Execute(t.Context(), request, func([]byte) error { return nil })
			wantError := outcome != "success" && outcome != "remove-failure"
			if (err != nil) != wantError || calls != 1 || files.removed != 1 {
				t.Fatalf("outcome=%v calls=%d removed=%d", err, calls, files.removed)
			}
		})
	}
}
