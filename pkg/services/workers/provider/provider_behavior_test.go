// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	workertaxonomy "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/services/workers/diagnostics"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type cursorTemporaryFileSystemFake struct {
	file      *cursorTemporaryFileFake
	createErr error
	events    []string
}

func (f *cursorTemporaryFileSystemFake) CreateTemp(dir, pattern string) (platformfilesystem.TemporaryFile, error) {
	f.events = append(f.events, "create:"+dir+":"+pattern)
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.file == nil {
		f.file = &cursorTemporaryFileFake{name: filepath.Join(dir, "cursor_prompt_deterministic.md"), owner: f}
	}
	return f.file, nil
}

func (f *cursorTemporaryFileSystemFake) Remove(path string) error {
	f.events = append(f.events, "remove:"+path)
	return nil
}

type cursorTemporaryFileFake struct {
	name     string
	content  strings.Builder
	writeErr error
	closeErr error
	owner    *cursorTemporaryFileSystemFake
}

func (f *cursorTemporaryFileFake) Name() string { return f.name }
func (f *cursorTemporaryFileFake) WriteString(value string) (int, error) {
	f.owner.events = append(f.owner.events, "write")
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.content.WriteString(value)
}
func (f *cursorTemporaryFileFake) Close() error {
	f.owner.events = append(f.owner.events, "close")
	return f.closeErr
}

type cursorLifecycleRunner struct {
	events *[]string
	result CommandResult
	err    error
}

func (r cursorLifecycleRunner) Run(context.Context, CommandRequest) (CommandResult, error) {
	*r.events = append(*r.events, "run")
	return r.result, r.err
}

func TestClaudeProviderBehavior_BuildArgs(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		req             workerexecution.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "BasicPrompt",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderClaude),
				UserMessage:   "hello",
			},
			want: []string{"-p", "hello"},
		},
		{
			name: "WithSkipPermissions",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderClaude),
				UserMessage:   "hello",
			},
			skipPermissions: true,
			want:            []string{"-p", "--dangerously-skip-permissions", "hello"},
		},
		{
			name: "WithSystemPromptAndModel",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderClaude),
				UserMessage:   "do stuff",
				SystemPrompt:  "You are helpful",
				Model:         "claude-sonnet-4-5-20250514",
			},
			want: []string{"-p", "--system-prompt", "You are helpful", "--model", "claude-sonnet-4-5-20250514", "do stuff"},
		},
		{
			name: "WithResumeSessionID",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderClaude),
				UserMessage:   "do stuff",
				SessionID:     "claude-session-123",
			},
			want: []string{"-p", "--resume", "claude-session-123", "do stuff"},
		},
	}

	behavior := claudeProviderBehavior{logger: logging.NoopLogger{}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := behavior.BuildArgs(context.Background(), tc.req, tc.skipPermissions, nil)
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			assertStringSlicesEqual(t, tc.want, args)
		})
	}
}

func TestCodexProviderBehavior_BuildArgs(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		req             workerexecution.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "BasicPrompt",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				UserMessage:   "fix the bug",
			},
			want: []string{"exec", "-"},
		},
		{
			name: "WithSkipPermissionsAndModel",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "hello",
			},
			skipPermissions: true,
			want:            []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "--model", "gpt-5-codex", "-"},
		},
		{
			name: "WithWorkingDirectoryRetainsStdinPlaceholderOnly",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider:    string(modelprovider.ProviderCodex),
				WorkingDirectory: "C:\\worktree",
				Model:            "gpt-5-codex",
				UserMessage:      "line 1\nline 2",
			},
			want: []string{"exec", "--model", "gpt-5-codex", "-"},
		},
	}

	behavior := codexProviderBehavior{logger: logging.NoopLogger{}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := behavior.BuildArgs(context.Background(), tc.req, tc.skipPermissions, nil)
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			assertStringSlicesEqual(t, tc.want, args)
		})
	}
}

func TestCodexProviderBehavior_BuildArgs_RejectsUnsupportedOptionalCapabilities(t *testing.T) {
	t.Parallel()
	behavior := codexProviderBehavior{logger: logging.NoopLogger{}}
	_, err := behavior.BuildArgs(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
		UserMessage:   "summarize the workspace",
		Worktree:      "feature-worktree",
		RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{
			workerexecution.RunnerOptionalCapabilityWorktree,
		},
	}, false, nil)
	if err == nil || err.Error() != "worktree selection is not supported by the codex runner in v1" {
		t.Fatalf("BuildArgs error = %v, want worktree rejection", err)
	}
}

func TestCodexProviderBehavior_BuildArgs_AllowsWorktreeMetadataWithPreparedWorkingDirectory(t *testing.T) {
	t.Parallel()
	behavior := codexProviderBehavior{logger: logging.NoopLogger{}}
	args, err := behavior.BuildArgs(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider:    string(modelprovider.ProviderCodex),
		UserMessage:      "summarize the workspace",
		Worktree:         "feature-worktree",
		WorkingDirectory: "/tmp/factory/.worktrees/feature-worktree",
		RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{
			workerexecution.RunnerOptionalCapabilityWorkingDirectory,
		},
	}, false, nil)
	if err != nil {
		t.Fatalf("BuildArgs() error = %v", err)
	}
	if len(args) == 0 || args[len(args)-1] != "-" {
		t.Fatalf("args = %#v, want stdin placeholder", args)
	}
	for _, arg := range args {
		if arg == "--worktree" {
			t.Fatalf("args = %#v, want no --worktree passthrough", args)
		}
	}
}

func TestCodexProviderBehavior_BuildArgs_MaterializesLocalFileURLWithoutCopy(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	imagePath := filepath.Join(workspace, "img.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	rawURL, err := work.FilesystemPathToContentURL(imagePath)
	if err != nil {
		t.Fatalf("content url: %v", err)
	}

	behavior := codexProviderBehavior{logger: logging.NoopLogger{}}
	cache := newDispatchContentCache()
	defer cache.release()

	args, err := behavior.BuildArgs(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
		Model:         "gpt-5-codex",
		UserMessage:   "inspect",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID: "token-1",
			Color: factoryruntime.RuntimeTokenColor{
				Content: []work.WorkContentPart{
					{Type: work.WorkContentPartTypeImage, URL: rawURL},
				},
			},
		}),
	}, false, &ProviderBuildContext{
		ContentCache: cache,
		ContentMaterializer: work.ContentMaterializeFunc(func(
			context.Context,
			string,
		) (string, work.ContentCleanup, error) {
			return imagePath, func() {}, nil
		}),
	})
	if err != nil {
		t.Fatalf("BuildArgs() error = %v", err)
	}
	want := []string{"exec", "--model", "gpt-5-codex", "-i", imagePath, "-"}
	assertStringSlicesEqual(t, want, args)
}

func TestGeminiProviderBehavior_BuildArgs(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		req             workerexecution.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "BasicPrompt",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderGemini),
				UserMessage:   "summarize the workspace",
			},
			want: []string{"--prompt", "summarize the workspace"},
		},
		{
			name: "WithModelAndSkipPermissions",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderGemini),
				Model:         "gemini-2.5-flash",
				UserMessage:   "run the tests",
			},
			skipPermissions: true,
			want:            []string{"--prompt", "run the tests", "--model", "gemini-2.5-flash", "--approval-mode", "yolo", "--sandbox", "false"},
		},
	}

	behavior := geminiProviderBehavior{logger: logging.NoopLogger{}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := behavior.BuildArgs(context.Background(), tc.req, tc.skipPermissions, nil)
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			assertStringSlicesEqual(t, tc.want, args)
		})
	}
}

func TestGeminiProviderBehavior_BuildArgs_RejectsUnsupportedOptionalCapabilities(t *testing.T) {
	t.Parallel()
	behavior := geminiProviderBehavior{logger: logging.NoopLogger{}}
	_, err := behavior.BuildArgs(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderGemini),
		UserMessage:   "summarize the workspace",
		RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{
			workerexecution.RunnerOptionalCapabilityStructuredOutput,
		},
	}, false, nil)
	if err == nil || err.Error() != "structured output is not supported by the gemini runner in v1" {
		t.Fatalf("BuildArgs error = %v, want structured output rejection", err)
	}
}

func TestKiroProviderBehavior_BuildArgs(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		req             workerexecution.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "BasicPrompt",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderKiro),
				UserMessage:   "summarize the workspace",
			},
			want: []string{"chat", "--no-interactive", "summarize the workspace"},
		},
		{
			name: "ComposedContextAndUserPrompt",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderKiro),
				SystemPrompt:  "You are a careful reviewer.",
				UserMessage:   "run the tests",
			},
			want: []string{
				"chat",
				"--no-interactive",
				"System instructions:\nYou are a careful reviewer.\n\nUser request:\nrun the tests",
			},
		},
		{
			name: "EmptyPrompt",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderKiro),
			},
			want: []string{"chat", "--no-interactive"},
		},
		{
			name: "TrustedTools",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderKiro),
				UserMessage:   "run the tests",
			},
			skipPermissions: true,
			want:            []string{"chat", "--no-interactive", "--trust-all-tools", "run the tests"},
		},
		{
			name: "ResumeSession",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderKiro),
				UserMessage:   "continue the review",
				SessionID:     "kiro-session-123",
			},
			want: []string{"chat", "--no-interactive", "--resume-id", "kiro-session-123", "continue the review"},
		},
	}

	behavior := providerBehaviorFor(string(modelprovider.ProviderKiro), logging.NoopLogger{})
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := behavior.BuildArgs(context.Background(), tc.req, tc.skipPermissions, nil)
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			assertStringSlicesEqual(t, tc.want, args)
		})
	}
}

func TestKiroProviderBehavior_BuildArgs_RejectsUnsupportedOptionalCapabilities(t *testing.T) {
	t.Parallel()
	behavior := kiroProviderBehavior{logger: logging.NoopLogger{}}
	_, err := behavior.BuildArgs(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderKiro),
		UserMessage:   "summarize the workspace",
		RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{
			workerexecution.RunnerOptionalCapabilityStructuredOutput,
		},
	}, false, nil)
	if err == nil || err.Error() != "structured output is not supported by the kiro runner in v1" {
		t.Fatalf("BuildArgs error = %v, want structured output rejection", err)
	}
}

func TestCursorProviderBehavior_BuildArgs(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		req             workerexecution.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "BasicPrompt",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCursor),
				UserMessage:   "summarize the workspace",
			},
			want: []string{"-p", "--output-format", "stream-json", "--stream-partial-output", "summarize the workspace"},
		},
		{
			name: "WithModelSessionAndForce",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCursor),
				Model:         "gpt-5",
				SessionID:     "cursor-session-123",
				UserMessage:   "run the tests",
			},
			skipPermissions: true,
			want:            []string{"-f", "-p", "--model", "gpt-5", "--resume", "cursor-session-123", "--output-format", "stream-json", "--stream-partial-output", "run the tests"},
		},
		{
			name: "WithWorkspace",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider:    string(modelprovider.ProviderCursor),
				WorkingDirectory: "/tmp/project",
				UserMessage:      "inspect the repo",
			},
			want: []string{"-p", "--workspace", "/tmp/project", "--output-format", "stream-json", "--stream-partial-output", "inspect the repo"},
		},
	}

	behavior := cursorProviderBehavior{logger: logging.NoopLogger{}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := behavior.BuildArgs(context.Background(), tc.req, tc.skipPermissions, nil)
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			assertStringSlicesEqual(t, tc.want, args)
		})
	}
}

func TestCursorProviderBehavior_BuildArgs_WindowsLongPromptUsesArgumentFile(t *testing.T) {
	t.Parallel()
	tempDir := `C:\deterministic-temp`
	temporaryFiles := &cursorTemporaryFileSystemFake{}
	buildCtx := &ProviderBuildContext{operatingSystem: "windows", tempDir: tempDir, temporaryFiles: temporaryFiles}
	prompt := strings.Repeat("long cursor instruction ", cursorWindowsPromptArgumentLimit)
	req := workerexecution.ProviderInferenceRequest{
		ModelProvider:    string(modelprovider.ProviderCursor),
		SystemPrompt:     "system guidance",
		UserMessage:      prompt,
		Model:            "composer-2.5",
		SessionID:        "cursor-session-123",
		WorkingDirectory: `C:\workspace`,
		Worktree:         "feature-worktree",
	}

	args, err := (cursorProviderBehavior{logger: logging.NoopLogger{}}).BuildArgs(context.Background(), req, true, buildCtx)
	if err != nil {
		t.Fatalf("BuildArgs returned error: %v", err)
	}
	wantPrefix := []string{"-f", "-p", "--model", "composer-2.5", "--resume", "cursor-session-123", "--workspace", `C:\workspace`, "--worktree", "feature-worktree", "--output-format", "stream-json", "--stream-partial-output"}
	if len(args) != len(wantPrefix)+1 {
		t.Fatalf("args = %#v, want prefix plus prompt-file argument", args)
	}
	assertStringSlicesEqual(t, wantPrefix, args[:len(wantPrefix)])
	promptArg := args[len(args)-1]
	if !strings.HasPrefix(promptArg, "@") {
		t.Fatalf("prompt argument = %q, want @file reference", promptArg)
	}
	promptPath := strings.TrimPrefix(promptArg, "@")
	wantPrompt := buildKiroPrompt(req)
	if gotPrompt := temporaryFiles.file.content.String(); gotPrompt != wantPrompt {
		t.Fatalf("prompt file content differs: got %d bytes, want %d", len(gotPrompt), len(wantPrompt))
	}
	buildCtx.release()
	wantEvents := []string{
		"create:" + tempDir + ":cursor_prompt_*.md",
		"write",
		"close",
		"remove:" + promptPath,
	}
	assertStringSlicesEqual(t, wantEvents, temporaryFiles.events)
	if promptPath != filepath.Join(tempDir, "cursor_prompt_deterministic.md") {
		t.Fatalf("prompt path = %q, want injected temporary-file path", promptPath)
	}
}

func TestCursorProviderBehavior_BuildArgs_LongPromptFailsClosedWithoutTemporaryFileSystem(t *testing.T) {
	t.Parallel()
	_, err := (cursorProviderBehavior{logger: logging.NoopLogger{}}).BuildArgs(
		t.Context(),
		workerexecution.ProviderInferenceRequest{
			ModelProvider: string(modelprovider.ProviderCursor),
			UserMessage:   strings.Repeat("x", cursorWindowsPromptArgumentLimit+1),
		},
		false,
		&ProviderBuildContext{operatingSystem: "windows", tempDir: `C:\temp`},
	)
	if err == nil || !strings.Contains(err.Error(), "temporary filesystem is required") {
		t.Fatalf("BuildArgs error = %v, want missing injected temporary filesystem", err)
	}
}

func TestCursorProviderBehavior_BuildArgs_PreservesTemporaryFileCreationDiagnostic(t *testing.T) {
	t.Parallel()
	createErr := errors.New("temporary volume unavailable")
	temporaryFiles := &cursorTemporaryFileSystemFake{createErr: createErr}
	_, err := (cursorProviderBehavior{logger: logging.NoopLogger{}}).BuildArgs(
		t.Context(),
		workerexecution.ProviderInferenceRequest{ModelProvider: string(modelprovider.ProviderCursor), UserMessage: strings.Repeat("x", cursorWindowsPromptArgumentLimit+1)},
		false,
		&ProviderBuildContext{operatingSystem: "windows", tempDir: `C:\temp`, temporaryFiles: temporaryFiles},
	)
	if !errors.Is(err, createErr) || !strings.Contains(err.Error(), "failed to write temporary prompt file") {
		t.Fatalf("BuildArgs error = %v, want wrapped injected creation diagnostic", err)
	}
	assertStringSlicesEqual(t, []string{"create:C:\\temp:cursor_prompt_*.md"}, temporaryFiles.events)
}

func TestCursorProviderBehavior_BuildArgs_RemovesPartialTemporaryFileAfterWriteOrCloseFailure(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name     string
		writeErr error
		closeErr error
		want     []string
	}{
		{
			name: "write failure", writeErr: errors.New("write failed"),
			want: []string{"create:C:\\temp:cursor_prompt_*.md", "write", "close", "remove:" + filepath.Join(`C:\temp`, "cursor_prompt_deterministic.md")},
		},
		{
			name: "close failure", closeErr: errors.New("close failed"),
			want: []string{"create:C:\\temp:cursor_prompt_*.md", "write", "close", "remove:" + filepath.Join(`C:\temp`, "cursor_prompt_deterministic.md")},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			temporaryFiles := &cursorTemporaryFileSystemFake{}
			temporaryFiles.file = &cursorTemporaryFileFake{
				name: filepath.Join(`C:\temp`, "cursor_prompt_deterministic.md"), writeErr: testCase.writeErr,
				closeErr: testCase.closeErr, owner: temporaryFiles,
			}
			_, err := (cursorProviderBehavior{logger: logging.NoopLogger{}}).BuildArgs(
				t.Context(),
				workerexecution.ProviderInferenceRequest{ModelProvider: string(modelprovider.ProviderCursor), UserMessage: strings.Repeat("x", cursorWindowsPromptArgumentLimit+1)},
				false,
				&ProviderBuildContext{operatingSystem: "windows", tempDir: `C:\temp`, temporaryFiles: temporaryFiles},
			)
			if err == nil {
				t.Fatal("BuildArgs error = nil, want injected temporary-file failure")
			}
			assertStringSlicesEqual(t, testCase.want, temporaryFiles.events)
		})
	}
}

func TestScriptWrapProvider_CursorLongPromptCleansRequestFileAfterCommandFailure(t *testing.T) {
	t.Parallel()
	temporaryFiles := &cursorTemporaryFileSystemFake{}
	runner := cursorLifecycleRunner{events: &temporaryFiles.events, err: errors.New("command failed")}
	provider := NewScriptWrapProviderWithDependencies(false, nil, runner, nil, nil, nil, "", nil, nil)
	provider.operatingSystem = "windows"
	provider.temporaryFiles = temporaryFiles

	_, err := provider.Infer(t.Context(), workerexecution.ProviderInferenceRequest{
		ModelProvider:    string(modelprovider.ProviderCursor),
		WorkingDirectory: `C:\workspace`,
		UserMessage:      strings.Repeat("x", cursorWindowsPromptArgumentLimit+1),
	})
	if err == nil || err.Error() != "provider error: unknown" {
		t.Fatalf("Infer error = %v, want existing normalized provider diagnostic", err)
	}
	wantPath := filepath.Join(`C:\workspace`, "cursor_prompt_deterministic.md")
	wantEvents := []string{
		"create:C:\\workspace:cursor_prompt_*.md", "write", "close", "run", "remove:" + wantPath,
	}
	assertStringSlicesEqual(t, wantEvents, temporaryFiles.events)
}

func TestCursorProviderBehavior_BuildArgs_DoesNotWrapShortOrNonWindowsPrompts(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		operatingSystem string
		prompt          string
	}{
		{name: "ShortWindowsPrompt", operatingSystem: "windows", prompt: "inspect the repository"},
		{name: "LongLinuxPrompt", operatingSystem: "linux", prompt: strings.Repeat("x", cursorWindowsPromptArgumentLimit+1)},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := (cursorProviderBehavior{logger: logging.NoopLogger{}}).BuildArgs(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCursor), UserMessage: tc.prompt,
			}, false, &ProviderBuildContext{operatingSystem: tc.operatingSystem, tempDir: t.TempDir()})
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			if got := args[len(args)-1]; got != tc.prompt {
				t.Fatalf("prompt argument = %q, want positional prompt", got)
			}
		})
	}
}

func TestCursorProviderBehavior_BuildArgs_LongPromptRequiresOperatingSystem(t *testing.T) {
	t.Parallel()
	_, err := (cursorProviderBehavior{logger: logging.NoopLogger{}}).BuildArgs(
		context.Background(),
		workerexecution.ProviderInferenceRequest{
			ModelProvider: string(modelprovider.ProviderCursor),
			UserMessage:   strings.Repeat("x", cursorWindowsPromptArgumentLimit+1),
		},
		false,
		&ProviderBuildContext{},
	)
	if err == nil || !strings.Contains(err.Error(), "operating system is required") {
		t.Fatalf("BuildArgs error = %v, want missing operating-system decision", err)
	}
}

func TestCursorProviderBehavior_BuildArgs_RejectsUnsupportedOptionalCapabilities(t *testing.T) {
	t.Parallel()
	behavior := cursorProviderBehavior{logger: logging.NoopLogger{}}
	_, err := behavior.BuildArgs(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCursor),
		UserMessage:   "summarize the workspace",
		RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{
			workerexecution.RunnerOptionalCapabilityStructuredOutput,
		},
	}, false, nil)
	if err == nil || err.Error() != "structured output is not supported by the cursor-cli runner in v1" {
		t.Fatalf("BuildArgs error = %v, want structured output rejection", err)
	}
}

func TestOpenCodeProviderBehavior_BuildArgs(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		req             workerexecution.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "BasicPrompt",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderOpenCode),
				UserMessage:   "summarize the workspace",
			},
			want: []string{"run", "summarize the workspace"},
		},
		{
			name: "WithModelSessionWorkingDirectoryAndSkipPermissions",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider:    string(modelprovider.ProviderOpenCode),
				Model:            "openai/gpt-5",
				SessionID:        "opencode-session-123",
				WorkingDirectory: "/tmp/project",
				UserMessage:      "run the tests",
			},
			skipPermissions: true,
			want:            []string{"run", "--model", "openai/gpt-5", "--session", "opencode-session-123", "--dir", "/tmp/project", "--dangerously-skip-permissions", "run the tests"},
		},
		{
			name: "WithOpenCodeAgent",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderOpenCode),
				OpenCodeAgent: "implementer",
				UserMessage:   "summarize the workspace",
			},
			want: []string{"run", "--agent", "implementer", "summarize the workspace"},
		},
		{
			name: "WithOpenCodeAgentModelSessionWorkingDirectoryAndSkipPermissions",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider:    string(modelprovider.ProviderOpenCode),
				Model:            "openai/gpt-5",
				OpenCodeAgent:    "implementer",
				SessionID:        "opencode-session-123",
				WorkingDirectory: "/tmp/project",
				UserMessage:      "run the tests",
			},
			skipPermissions: true,
			want:            []string{"run", "--model", "openai/gpt-5", "--agent", "implementer", "--session", "opencode-session-123", "--dir", "/tmp/project", "--dangerously-skip-permissions", "run the tests"},
		},
	}

	behavior := openCodeProviderBehavior{logger: logging.NoopLogger{}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := behavior.BuildArgs(context.Background(), tc.req, tc.skipPermissions, nil)
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			assertStringSlicesEqual(t, tc.want, args)
		})
	}
}

func TestOpenCodeProviderBehavior_BuildArgs_AcceptsNegotiatedStructuredCapability(t *testing.T) {
	t.Parallel()
	behavior := openCodeProviderBehavior{logger: logging.NoopLogger{}}
	args, err := behavior.BuildArgs(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderOpenCode),
		UserMessage:   "summarize the workspace",
		RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{
			workerexecution.RunnerOptionalCapabilityStructuredOutput,
		},
	}, false, nil)
	if err != nil {
		t.Fatalf("BuildArgs error = %v", err)
	}
	if len(args) != 2 || args[0] != "run" || args[1] != "summarize the workspace" {
		t.Fatalf("BuildArgs() = %#v", args)
	}
}

func TestNonCodexProviderBehavior_BuildCommandRequest(t *testing.T) {
	t.Parallel()
	for _, tc := range nonCodexCommandRequestTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			request := providerBehaviorFor(tc.req.ModelProvider, logging.NoopLogger{}).BuildCommandRequest(tc.req, tc.args)
			if request.Command != tc.req.ModelProvider {
				t.Fatalf("command = %q, want %q", request.Command, tc.req.ModelProvider)
			}
			assertStringSlicesEqual(t, tc.args, request.Args)
			if len(request.Stdin) != 0 {
				t.Fatalf("expected non-codex request to avoid stdin, got %q", string(request.Stdin))
			}
			if request.WorkDir != tc.req.WorkingDirectory {
				t.Fatalf("workdir = %q, want %q", request.WorkDir, tc.req.WorkingDirectory)
			}
			assertEnvContains(t, request.Env, tc.wantEnv)
			if len(request.InputTokens) != len(tc.req.InputTokens) {
				t.Fatalf("input token count = %d, want %d", len(request.InputTokens), len(tc.req.InputTokens))
			}
			if len(request.InputTokens) > 0 && &request.InputTokens[0] == &tc.req.InputTokens[0] {
				t.Fatal("expected command request input tokens to be cloned")
			}
		})
	}
}

func TestFailureBaseline_AbsentDefault_BuildCommandRequestUsesEmptyProviderCommandWhenModelProviderUnset(t *testing.T) {
	t.Parallel()
	req := workerexecution.ProviderInferenceRequest{
		UserMessage: "plan the goal",
	}
	request := providerBehaviorFor("", logging.NoopLogger{}).BuildCommandRequest(req, []string{"-p", "plan the goal"})
	if request.Command != "" {
		t.Fatalf("command = %q, want empty provider command when modelProvider is unset", request.Command)
	}
}

type nonCodexCommandRequestTestCase struct {
	name    string
	req     workerexecution.ProviderInferenceRequest
	args    []string
	wantEnv string
}

func nonCodexCommandRequestTestCases() []nonCodexCommandRequestTestCase {
	token := factoryruntime.RuntimeToken{
		ID: "token-1",
		Color: factoryruntime.RuntimeTokenColor{
			Content: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "hello"},
			},
		},
	}

	return []nonCodexCommandRequestTestCase{
		{
			name: "Claude",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderClaude),
				UserMessage:   "review this",
				EnvVars: map[string]string{
					"AGENT_FACTORY_PROVIDER": "claude",
				},
				InputTokens: InputTokens(token),
			},
			args:    []string{"-p", "review this"},
			wantEnv: "AGENT_FACTORY_PROVIDER=claude",
		},
		{
			name: "Gemini",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderGemini),
				UserMessage:   "review this",
				EnvVars: map[string]string{
					"AGENT_FACTORY_PROVIDER": "gemini",
				},
				InputTokens: InputTokens(token),
			},
			args:    []string{"--prompt", "review this"},
			wantEnv: "AGENT_FACTORY_PROVIDER=gemini",
		},
		{
			name: "Kiro",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderKiro),
				UserMessage:   "review this",
				EnvVars: map[string]string{
					"AGENT_FACTORY_PROVIDER": "kiro",
				},
				InputTokens: InputTokens(token),
			},
			args:    []string{"chat", "--no-interactive", "review this"},
			wantEnv: "AGENT_FACTORY_PROVIDER=kiro",
		},
		{
			name: "Cursor",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCursor),
				UserMessage:   "review this",
				EnvVars: map[string]string{
					"AGENT_FACTORY_PROVIDER": "cursor",
				},
				InputTokens: InputTokens(token),
			},
			args:    []string{"--print", "review this", "--output-format", "text"},
			wantEnv: "AGENT_FACTORY_PROVIDER=cursor",
		},
		{
			name: "OpenCode",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider:    string(modelprovider.ProviderOpenCode),
				UserMessage:      "review this",
				WorkingDirectory: "/tmp/project",
				EnvVars: map[string]string{
					"AGENT_FACTORY_PROVIDER": "opencode",
				},
				InputTokens: InputTokens(token),
			},
			args:    []string{"run", "review this"},
			wantEnv: "AGENT_FACTORY_PROVIDER=opencode",
		},
	}
}

func TestScriptWrapProvider_Infer_KiroKnownFailuresUseCanonicalParserAndPolicy(t *testing.T) {
	t.Parallel()
	fixtureNames := []string{
		"kiro_structured_authentication_error",
		"kiro_structured_invalid_request_stdout",
		"kiro_text_authentication_stdout",
		"kiro_structured_throttle_precedes_text",
		"kiro_text_capacity_error",
		"kiro_text_timeout_malformed_structured",
		"kiro_structured_service_unavailable",
	}

	for _, name := range fixtureNames {
		entry := providerErrorCorpusEntryForTest(t, name)
		t.Run(providerErrorCorpusEntryLabel(entry), func(t *testing.T) {
			provider := NewScriptWrapProviderWithDependencies(false, nil, &recordingProviderExec{result: entry.CommandResult()}, nil, nil, nil, "", nil, nil)
			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderKiro),
				UserMessage:   "private prompt that must stay out of normalized failures",
			})
			assertNormalizedProviderFailure(t, err, normalizedProviderFailureExpectation{
				wantType: entry.ExpectedType, wantFamily: entry.ExpectedFamily,
				wantMessage:   knownKiroFailure(entry.ExpectedType).Message,
				rejectTexts:   append(entry.RejectMessageContains, "private prompt"),
				wantRetryable: entry.Retryable, wantTerminal: !entry.Retryable,
				wantThrottlePause: entry.TriggersThrottlePause,
			})
		})
	}
}

func TestScriptWrapProvider_Infer_KiroUnknownFailuresUseBoundedParserMessages(t *testing.T) {
	t.Parallel()
	testCases := map[string]string{
		"kiro_unknown_stderr_excerpt_precedes_stdout":     "Kiro error: model registry handshake failed",
		"kiro_unknown_stdout_excerpt_after_unsafe_stderr": "Kiro error: plugin bridge failed",
		"kiro_unknown_noise_only_exit_fallback":           "kiro-cli exited with code 11",
	}

	for name, wantMessage := range testCases {
		entry := providerErrorCorpusEntryForTest(t, name)
		t.Run(providerErrorCorpusEntryLabel(entry), func(t *testing.T) {
			provider := NewScriptWrapProviderWithDependencies(false, nil, &recordingProviderExec{result: entry.CommandResult()}, nil, nil, nil, "", nil, nil)
			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderKiro),
				UserMessage:   "private prompt that must stay out of normalized failures",
			})
			assertNormalizedProviderFailure(t, err, normalizedProviderFailureExpectation{
				wantType: workerexecution.WorkFailureTypeUnknown, wantFamily: workerexecution.WorkFailureFamilyTerminal,
				wantMessage: wantMessage, rejectTexts: append(entry.RejectMessageContains, "private prompt"),
				wantTerminal: true,
			})
		})
	}
}

func providerOwnedCursorAndCodexFailureTestCases() []exitFailureInferenceTestCase {
	return []exitFailureInferenceTestCase{
		{
			name:        "CursorUsesItsOwnErrorExtraction",
			provider:    string(modelprovider.ProviderCursor),
			result:      CommandResult{ExitCode: 1, Stderr: []byte("noise before\nERROR: unexpected status 500 from cursor upstream")},
			wantMessage: "ERROR: unexpected status 500 from cursor upstream",
			wantType:    workerexecution.WorkFailureTypeInternalServerError,
		},
		{
			name:        "CursorDoesNotInheritCodexHighDemandClassification",
			provider:    string(modelprovider.ProviderCursor),
			result:      CommandResult{ExitCode: 1, Stderr: []byte(codexHighDemandTemporaryErrorsNeedle)},
			wantMessage: codexHighDemandTemporaryErrorsNeedle,
			wantType:    workerexecution.WorkFailureTypeUnknown,
		},
		{
			name:        "CodexUsesItsOwnSafeServerResult",
			provider:    string(modelprovider.ProviderCodex),
			result:      CommandResult{ExitCode: 1, Stderr: []byte("noise before\nERROR: unexpected status 500 from codex upstream")},
			wantMessage: codexServerFailureMessage,
			wantType:    workerexecution.WorkFailureTypeInternalServerError,
		},
		{
			name:        "UnsupportedProviderDoesNotFallBackToCodexParser",
			provider:    "custom-provider",
			result:      CommandResult{ExitCode: 1, Stderr: []byte(codexHighDemandTemporaryErrorsNeedle)},
			wantMessage: codexHighDemandTemporaryErrorsNeedle,
			wantType:    workerexecution.WorkFailureTypeUnknown,
		},
	}
}

func TestWorkDiagnosticsForInferenceRequest_IncludesOpenCodeAgentWhenConfigured(t *testing.T) {
	t.Parallel()
	diagnostics := workDiagnosticsForInferenceRequest(workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderOpenCode),
		Model:         "openai/gpt-5",
		OpenCodeAgent: "implementer",
		WorkerType:    workertaxonomy.WorkerTypeModel,
	})
	if got := diagnostics.Provider.RequestMetadata["opencode_agent"]; got != "implementer" {
		t.Fatalf("opencode_agent = %q, want implementer", got)
	}
}

func TestWorkDiagnosticsForInferenceRequest_OmitsOpenCodeAgentWhenUnset(t *testing.T) {
	t.Parallel()
	diagnostics := workDiagnosticsForInferenceRequest(workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderOpenCode),
		Model:         "openai/gpt-5",
		WorkerType:    workertaxonomy.WorkerTypeModel,
	})
	if _, ok := diagnostics.Provider.RequestMetadata["opencode_agent"]; ok {
		t.Fatalf("request metadata = %#v, want opencode_agent omitted", diagnostics.Provider.RequestMetadata)
	}
}

func TestWorkDiagnosticsForInferenceRequest_SafeProjectionPreservesOpenCodeAgent(t *testing.T) {
	t.Parallel()
	diagnostics := workDiagnosticsForInferenceRequest(workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderOpenCode),
		OpenCodeAgent: "implementer",
	})
	safe := workerdiagnostics.SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics)
	if got := safe.Provider.RequestMetadata["opencode_agent"]; got != "implementer" {
		t.Fatalf("safe opencode_agent = %q, want implementer", got)
	}
}

type s14ProviderCase struct {
	provider       modelprovider.Provider
	unsafeMarker   string
	unsafeReq      workerexecution.ProviderInferenceRequest
	safeReq        workerexecution.ProviderInferenceRequest
	unsafeArgCheck func(args []string) bool
	safeArgCheck   func(args []string) bool
}

func s14SkipPermissionsProviderCases() []s14ProviderCase {
	return []s14ProviderCase{
		{
			provider:     modelprovider.ProviderClaude,
			unsafeMarker: "--dangerously-skip-permissions",
			unsafeReq: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderClaude),
				UserMessage:   "run the tests",
			},
			safeReq: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderClaude),
				UserMessage:   "run the tests",
			},
			unsafeArgCheck: func(args []string) bool {
				return strings.Contains(strings.Join(args, " "), "--dangerously-skip-permissions")
			},
			safeArgCheck: func(args []string) bool {
				return !strings.Contains(strings.Join(args, " "), "--dangerously-skip-permissions")
			},
		},
		{
			provider:     modelprovider.ProviderCodex,
			unsafeMarker: "--dangerously-bypass-approvals-and-sandbox",
			unsafeReq: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				UserMessage:   "run the tests",
			},
			safeReq: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				UserMessage:   "run the tests",
			},
			unsafeArgCheck: func(args []string) bool {
				return strings.Contains(strings.Join(args, " "), "--dangerously-bypass-approvals-and-sandbox")
			},
		},
		{
			provider:     modelprovider.ProviderGemini,
			unsafeMarker: "--approval-mode",
			unsafeReq: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderGemini),
				UserMessage:   "run the tests",
			},
			safeReq: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderGemini),
				UserMessage:   "run the tests",
			},
			unsafeArgCheck: func(args []string) bool {
				joined := strings.Join(args, " ")
				return strings.Contains(joined, "--approval-mode") && strings.Contains(joined, "yolo") &&
					strings.Contains(joined, "--sandbox") && strings.Contains(joined, "false")
			},
		},
		{
			provider:     modelprovider.ProviderKiro,
			unsafeMarker: "--trust-all-tools",
			unsafeReq: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderKiro),
				UserMessage:   "run the tests",
			},
			safeReq: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderKiro),
				UserMessage:   "run the tests",
			},
			unsafeArgCheck: func(args []string) bool {
				return strings.Contains(strings.Join(args, " "), "--trust-all-tools")
			},
		},
		{
			provider:     modelprovider.ProviderCursor,
			unsafeMarker: "-f",
			unsafeReq: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCursor),
				UserMessage:   "run the tests",
			},
			safeReq: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCursor),
				UserMessage:   "run the tests",
			},
			unsafeArgCheck: func(args []string) bool {
				return len(args) > 0 && args[0] == "-f"
			},
		},
		{
			provider:     modelprovider.ProviderOpenCode,
			unsafeMarker: "--dangerously-skip-permissions",
			unsafeReq: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderOpenCode),
				UserMessage:   "run the tests",
			},
			safeReq: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderOpenCode),
				UserMessage:   "run the tests",
			},
			unsafeArgCheck: func(args []string) bool {
				return strings.Contains(strings.Join(args, " "), "--dangerously-skip-permissions")
			},
		},
	}
}

func s14ResolveSafeArgCheck(tc *s14ProviderCase) {
	if tc.safeArgCheck != nil {
		return
	}
	marker := tc.unsafeMarker
	tc.safeArgCheck = func(args []string) bool {
		return !strings.Contains(strings.Join(args, " "), marker)
	}
	if tc.provider == modelprovider.ProviderCursor {
		tc.safeArgCheck = func(args []string) bool {
			return len(args) == 0 || args[0] != "-f"
		}
	}
}

func assertS14ProviderUnsafeArgs(t *testing.T, tc s14ProviderCase) {
	t.Helper()
	behavior := providerBehaviorFor(string(tc.provider), logging.NoopLogger{})
	args, err := behavior.BuildArgs(context.Background(), tc.unsafeReq, true, nil)
	if err != nil {
		t.Fatalf("BuildArgs(skip=true): %v", err)
	}
	if !tc.unsafeArgCheck(args) {
		t.Fatalf("provider args = %#v, want unsafe marker %q", args, tc.unsafeMarker)
	}
}

func assertS14ProviderSafeArgs(t *testing.T, tc s14ProviderCase) {
	t.Helper()
	behavior := providerBehaviorFor(string(tc.provider), logging.NoopLogger{})
	args, err := behavior.BuildArgs(context.Background(), tc.safeReq, false, nil)
	if err != nil {
		t.Fatalf("BuildArgs(skip=false): %v", err)
	}
	if !tc.safeArgCheck(args) {
		t.Fatalf("provider args = %#v, want to omit unsafe marker %q", args, tc.unsafeMarker)
	}
}

func TestS14SupportedProviderUnsafeOptionPropagationEvidence(t *testing.T) {
	t.Parallel()

	for _, tc := range s14SkipPermissionsProviderCases() {
		tc := tc
		s14ResolveSafeArgCheck(&tc)
		t.Run(string(tc.provider)+"/EffectiveTrueIncludesUnsafeOption", func(t *testing.T) {
			t.Parallel()
			assertS14ProviderUnsafeArgs(t, tc)
		})
		t.Run(string(tc.provider)+"/EffectiveFalseOmitsUnsafeOption", func(t *testing.T) {
			t.Parallel()
			assertS14ProviderSafeArgs(t, tc)
		})
	}
}
