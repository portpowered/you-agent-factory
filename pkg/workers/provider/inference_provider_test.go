package provider

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	cursorpkg "github.com/portpowered/infinite-you/pkg/workers/provider/cursor"
)

// --- NewScriptWrapProvider ---

func TestNewScriptWrapProvider_Defaults(t *testing.T) {
	p := NewScriptWrapProvider()
	if p.SkipPermissions {
		t.Error("expected SkipPermissions to default to false")
	}
}

func TestNewScriptWrapProvider_WithOptions(t *testing.T) {
	p := NewScriptWrapProvider(
		WithSkipPermissions(true),
	)
	if !p.SkipPermissions {
		t.Error("expected SkipPermissions to be true")
	}
}

func TestBuildProviderEnv_Empty(t *testing.T) {
	env := buildProviderEnv(nil)
	if len(env) == 0 {
		t.Fatal("expected provider env to include process environment or automation defaults")
	}
	assertProviderAutomationDefaults(t, env)
}

func TestBuildProviderEnv_Merges(t *testing.T) {
	env := buildProviderEnv(map[string]string{
		"CUSTOM_A": "val_a",
		"CUSTOM_B": "val_b",
	})
	foundA, foundB := false, false
	for _, e := range env {
		if e == "CUSTOM_A=val_a" {
			foundA = true
		}
		if e == "CUSTOM_B=val_b" {
			foundB = true
		}
	}
	if !foundA {
		t.Error("expected CUSTOM_A=val_a")
	}
	if !foundB {
		t.Error("expected CUSTOM_B=val_b")
	}
	assertProviderAutomationDefaults(t, env)
}

func TestBuildProviderEnv_IncludesAutomationDefaults(t *testing.T) {
	env := buildProviderEnv(nil)

	assertProviderAutomationDefaults(t, env)
}

func TestBuildProviderEnv_UsesDeterministicPrecedenceForOverlappingKeys(t *testing.T) {
	t.Setenv("GIT_EDITOR", "vim")
	t.Setenv("GIT_SEQUENCE_EDITOR", "vim")
	t.Setenv("AGENT_FACTORY_PROVIDER_ENV_PRECEDENCE", "process")

	env := buildProviderEnv(map[string]string{
		"AGENT_FACTORY_PROVIDER_ENV_PRECEDENCE": "provider",
		"AGENT_FACTORY_PROVIDER_ONLY":           "present",
		"GIT_EDITOR":                            "nano",
		"GIT_SEQUENCE_EDITOR":                   "nano",
	})

	assertEnvValue(t, env, "GIT_EDITOR", "true")
	assertEnvValue(t, env, "GIT_SEQUENCE_EDITOR", "true")
	assertEnvValue(t, env, "AGENT_FACTORY_PROVIDER_ENV_PRECEDENCE", "provider")
	assertEnvValue(t, env, "AGENT_FACTORY_PROVIDER_ONLY", "present")

	for _, name := range []string{
		"AGENT_FACTORY_PROVIDER_ENV_PRECEDENCE",
		"AGENT_FACTORY_PROVIDER_ONLY",
		"GIT_EDITOR",
		"GIT_SEQUENCE_EDITOR",
	} {
		assertEnvEntryCount(t, env, name, 1)
	}
}

func TestModelWorkerCommandRequest_MergesEnvironmentWithDeterministicPrecedence(t *testing.T) {
	t.Setenv("GIT_EDITOR", "vim")
	t.Setenv("GIT_SEQUENCE_EDITOR", "vim")
	t.Setenv("AGENT_FACTORY_PROVIDER_ENV_PRECEDENCE", "process")

	behavior := providerBehaviorFor(string(interfaces.ModelProviderClaude), nil)
	req := behavior.BuildCommandRequest(interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderClaude),
		UserMessage:   "fix it",
		EnvVars: map[string]string{
			"AGENT_FACTORY_PROVIDER_ENV_PRECEDENCE": "provider",
			"AGENT_FACTORY_PROVIDER_ONLY":           "present",
			"GIT_EDITOR":                            "nano",
			"GIT_SEQUENCE_EDITOR":                   "nano",
		},
	}, []string{"-p", "fix it"})

	assertEnvValue(t, req.Env, "GIT_EDITOR", "true")
	assertEnvValue(t, req.Env, "GIT_SEQUENCE_EDITOR", "true")
	assertEnvValue(t, req.Env, "AGENT_FACTORY_PROVIDER_ENV_PRECEDENCE", "provider")
	assertEnvValue(t, req.Env, "AGENT_FACTORY_PROVIDER_ONLY", "present")

	for _, name := range []string{
		"AGENT_FACTORY_PROVIDER_ENV_PRECEDENCE",
		"AGENT_FACTORY_PROVIDER_ONLY",
		"GIT_EDITOR",
		"GIT_SEQUENCE_EDITOR",
	} {
		assertEnvEntryCount(t, req.Env, name, 1)
	}
}

func TestBuildProviderEnv_PreservesExplicitLegacyPortOSEnvKeys(t *testing.T) {
	env := buildProviderEnv(map[string]string{
		"PORTOS_BRANCH": "ralph/legacy-feature",
	})

	assertEnvValue(t, env, "PORTOS_BRANCH", "ralph/legacy-feature")
	assertEnvEntryCount(t, env, "PORTOS_BRANCH", 1)
}

func TestScriptWrapProvider_Infer_CommandEnvironmentUsesAutomationDefaultsOverProviderOverrides(t *testing.T) {
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("provider output")},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCodex),
		Model:         "gpt-5-codex",
		UserMessage:   "fix it",
		EnvVars: map[string]string{
			"AGENT_FACTORY_CUSTOM_ENV": "provider",
			"GIT_TERMINAL_PROMPT":      "1",
		},
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}

	assertEnvValue(t, fakeExec.request.Env, "GIT_TERMINAL_PROMPT", "0")
	assertEnvValue(t, fakeExec.request.Env, "AGENT_FACTORY_CUSTOM_ENV", "provider")
	assertEnvEntryCount(t, fakeExec.request.Env, "GIT_TERMINAL_PROMPT", 1)
	assertEnvEntryCount(t, fakeExec.request.Env, "AGENT_FACTORY_CUSTOM_ENV", 1)
}

func TestScriptWrapProvider_Infer_CommandEnvironmentIncludesAutomationDefaults(t *testing.T) {
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("provider output")},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCodex),
		Model:         "gpt-5-codex",
		UserMessage:   "fix it",
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}

	assertProviderAutomationDefaults(t, fakeExec.request.Env)
}

func TestScriptWrapProvider_Infer_CommandCanObserveAutomationDefaultsInEnvironment(t *testing.T) {
	fakeExec := &envPrintingProviderExec{}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	resp, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderClaude),
		UserMessage:   "print environment",
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}

	for _, entry := range providerAutomationEnvDefaults {
		want := entry.Name + "=" + entry.Value
		if !strings.Contains(resp.Content, want) {
			t.Fatalf("expected provider command output to contain %q, got:\n%s", want, resp.Content)
		}
	}
}

func TestScriptWrapProvider_CommandEnvironmentPreventsGitMergeEditorPrompt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}

	repoDir := t.TempDir()
	editorMarker := filepath.Join(t.TempDir(), "editor-invoked")
	editorScript := writeEditorMarkerScript(t, editorMarker)

	runGitSetup(t, repoDir, "init", "-b", "main")
	runGitSetup(t, repoDir, "config", "gc.auto", "0")
	runGitSetup(t, repoDir, "config", "maintenance.auto", "false")
	runGitSetup(t, repoDir, "config", "user.email", "agent-factory-test@example.com")
	runGitSetup(t, repoDir, "config", "user.name", "Agent Factory Test")
	writeTestFile(t, repoDir, "base.txt", "base\n")
	runGitSetup(t, repoDir, "add", "base.txt")
	runGitSetup(t, repoDir, "commit", "-m", "base")

	runGitSetup(t, repoDir, "checkout", "-b", "feature")
	writeTestFile(t, repoDir, "feature.txt", "feature\n")
	runGitSetup(t, repoDir, "add", "feature.txt")
	runGitSetup(t, repoDir, "commit", "-m", "feature")

	runGitSetup(t, repoDir, "checkout", "main")
	writeTestFile(t, repoDir, "main.txt", "main\n")
	runGitSetup(t, repoDir, "add", "main.txt")
	runGitSetup(t, repoDir, "commit", "-m", "main")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ExecCommandRunner{}.Run(ctx, CommandRequest{
		Command: "git",
		Args:    []string{"merge", "--no-ff", "feature"},
		Env: providerGitTestEnv(map[string]string{
			"GIT_EDITOR":          editorScript,
			"GIT_SEQUENCE_EDITOR": editorScript,
			"GIT_MERGE_AUTOEDIT":  "yes",
			"EDITOR":              editorScript,
			"VISUAL":              editorScript,
		}),
		WorkDir: repoDir,
	})
	if err != nil {
		t.Fatalf("git merge returned system error: %v\nstdout:\n%s\nstderr:\n%s", err, result.Stdout, result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("git merge exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if _, err := os.Stat(editorMarker); err == nil {
		t.Fatalf("git invoked editor at %s; provider automation env should suppress merge editor prompts", editorMarker)
	} else if !os.IsNotExist(err) {
		t.Fatalf("checking editor marker: %v", err)
	}
}

func TestScriptWrapProvider_Infer_ClaudePayloadUsesExpectedCommandArgsAndEnv(t *testing.T) {
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("claude output")},
	}
	provider := NewScriptWrapProvider(
		WithProviderCommandRunner(fakeExec),
		WithSkipPermissions(true),
	)

	req := interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderClaude),
		Model:         "claude-sonnet-4-5-20250514",
		SessionID:     "claude-session-123",
		SystemPrompt:  "system prompt",
		UserMessage:   "user prompt",
		Worktree:      "C:\\repo\\worktree",
		EnvVars: map[string]string{
			"AGENT_FACTORY_CLAUDE_ENV": "enabled",
		},
	}
	resp, err := provider.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if resp.Content != "claude output" {
		t.Fatalf("expected response content %q, got %q", "claude output", resp.Content)
	}
	if resp.ProviderSession == nil {
		t.Fatal("expected provider session metadata for claude response")
	}
	if resp.ProviderSession.Provider != string(interfaces.ModelProviderClaude) {
		t.Fatalf("provider session provider = %q, want %q", resp.ProviderSession.Provider, interfaces.ModelProviderClaude)
	}
	if resp.ProviderSession.Kind != providerSessionKindSessionID {
		t.Fatalf("provider session kind = %q, want %q", resp.ProviderSession.Kind, providerSessionKindSessionID)
	}
	if resp.ProviderSession.ID != "claude-session-123" {
		t.Fatalf("provider session id = %q, want %q", resp.ProviderSession.ID, "claude-session-123")
	}

	behavior := providerBehaviorFor(req.ModelProvider, logging.NoopLogger{})
	expectedArgs, err := behavior.BuildArgs(context.Background(), req, true, nil)
	if err != nil {
		t.Fatalf("BuildArgs returned error: %v", err)
	}
	expectedRequest := behavior.BuildCommandRequest(req, expectedArgs)
	assertCommandRequestAssemblyMatchesProviderBehavior(t, expectedRequest, fakeExec.request)
	if len(fakeExec.request.Stdin) != 0 {
		t.Fatalf("expected claude request not to send stdin, got %q", string(fakeExec.request.Stdin))
	}
	if fakeExec.request.WorkDir != "" {
		t.Fatalf("expected claude request not to set command working directory, got %q", fakeExec.request.WorkDir)
	}
	assertStringSliceDoesNotContain(t, fakeExec.request.Args, "-")
	assertEnvContains(t, fakeExec.request.Env, "AGENT_FACTORY_CLAUDE_ENV=enabled")
}

func TestScriptWrapProvider_Infer_PropagatesExecutionMetadataToProviderCommand(t *testing.T) {
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("provider output")},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	want := interfaces.ExecutionMetadata{
		DispatchCreatedTick: 3,
		CurrentTick:         4,
		TraceID:             "trace-1",
		WorkIDs:             []string{"work-1", "work-2"},
		ReplayKey:           "transition-1/trace-1/work-1/work-2",
	}
	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderClaude),
		Model:         "claude-sonnet-4-5-20250514",
		UserMessage:   "fix it",
		Dispatch:      interfaces.WorkDispatch{Execution: want},
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}

	assertExecutionMetadataEqual(t, want, fakeExec.request.Execution)
}

func TestScriptWrapProvider_Infer_ClaudeWithoutSessionLeavesMetadataNil(t *testing.T) {
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("claude output without session")},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	resp, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderClaude),
		Model:         "claude-sonnet-4-5-20250514",
		UserMessage:   "fix it",
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if resp.ProviderSession != nil {
		t.Fatalf("expected provider session to be nil, got %#v", resp.ProviderSession)
	}
}

func TestScriptWrapProvider_Infer_ClaudeExitFailurePreservesConfiguredSessionMetadata(t *testing.T) {
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			ExitCode: 1,
			Stderr:   []byte(`API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`),
		},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderClaude),
		Model:         "claude-sonnet-4-5-20250514",
		SessionID:     "claude-session-123",
		UserMessage:   "fix it",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}

	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.ProviderSession == nil {
		t.Fatal("expected provider session metadata on failure")
	}
	if providerErr.ProviderSession.Provider != string(interfaces.ModelProviderClaude) {
		t.Fatalf("provider session provider = %q, want %q", providerErr.ProviderSession.Provider, interfaces.ModelProviderClaude)
	}
	if providerErr.ProviderSession.ID != "claude-session-123" {
		t.Fatalf("provider session id = %q, want %q", providerErr.ProviderSession.ID, "claude-session-123")
	}
}

func TestScriptWrapProvider_Infer_CodexPayloadUsesExpectedCommandArgsStdinAndEnv(t *testing.T) {
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			Stdout: []byte("codex output"),
			Stderr: []byte("{\"event\":\"session.created\",\"session_id\":\"sess_codex_123\"}"),
		},
	}
	provider := NewScriptWrapProvider(
		WithProviderCommandRunner(fakeExec),
		WithSkipPermissions(true),
		WithProviderLogger(logging.NoopLogger{}),
	)

	req := interfaces.ProviderInferenceRequest{
		ModelProvider:    string(interfaces.ModelProviderCodex),
		Model:            "gpt-5-codex",
		WorkingDirectory: "C:\\repo",
		UserMessage:      "line 1\nline 2",
		EnvVars: map[string]string{
			"AGENT_FACTORY_CODEX_ENV": "present",
		},
	}
	resp, err := provider.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if resp.Content != "codex output" {
		t.Fatalf("expected response content %q, got %q", "codex output", resp.Content)
	}
	if resp.ProviderSession == nil {
		t.Fatal("expected provider session metadata for codex response")
	}
	if resp.ProviderSession.Provider != string(interfaces.ModelProviderCodex) {
		t.Fatalf("provider session provider = %q, want %q", resp.ProviderSession.Provider, interfaces.ModelProviderCodex)
	}
	if resp.ProviderSession.Kind != providerSessionKindSessionID {
		t.Fatalf("provider session kind = %q, want %q", resp.ProviderSession.Kind, providerSessionKindSessionID)
	}
	if resp.ProviderSession.ID != "sess_codex_123" {
		t.Fatalf("provider session id = %q, want %q", resp.ProviderSession.ID, "sess_codex_123")
	}

	behavior := providerBehaviorFor(req.ModelProvider, logging.NoopLogger{})
	expectedArgs, err := behavior.BuildArgs(context.Background(), req, true, nil)
	if err != nil {
		t.Fatalf("BuildArgs returned error: %v", err)
	}
	expectedRequest := behavior.BuildCommandRequest(req, expectedArgs)
	assertCommandRequestAssemblyMatchesProviderBehavior(t, expectedRequest, fakeExec.request)
	if string(fakeExec.request.Stdin) != "line 1\nline 2" {
		t.Fatalf("expected codex stdin to carry the prompt, got %q", string(fakeExec.request.Stdin))
	}
	if fakeExec.request.WorkDir != "C:\\repo" {
		t.Fatalf("expected codex request workdir %q, got %q", "C:\\repo", fakeExec.request.WorkDir)
	}
	assertStringSliceDoesNotContain(t, fakeExec.request.Args, "line 1\nline 2")
	assertEnvContains(t, fakeExec.request.Env, "AGENT_FACTORY_CODEX_ENV=present")
}

func TestScriptWrapProvider_Infer_ClaudeRejectsImageContentBeforeRunner(t *testing.T) {
	fakeExec := &recordingProviderExec{result: CommandResult{Stdout: []byte("claude output")}}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderClaude),
		Model:         "claude-sonnet",
		UserMessage:   "inspect",
		InputTokens: InputTokens(interfaces.Token{
			ID: "token-1",
			Color: interfaces.TokenColor{
				Content: []interfaces.WorkContentPart{
					{Type: interfaces.WorkContentPartTypeText, Text: "caption"},
					{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/mockup.png"},
				},
			},
		}),
	})
	if err == nil {
		t.Fatal("expected claude image content to fail")
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}
	if providerErr.Type != interfaces.WorkFailureTypePermanentBadRequest {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, interfaces.WorkFailureTypePermanentBadRequest)
	}
	if !strings.Contains(providerErr.Message, `input_tokens[0].color.content[1].file`) ||
		!strings.Contains(providerErr.Message, `model provider claude`) ||
		!strings.Contains(providerErr.Message, `configure modelProvider codex`) {
		t.Fatalf("provider error message = %q", providerErr.Message)
	}
	if fakeExec.calls != 0 {
		t.Fatalf("expected runner not to be called, got %d calls", fakeExec.calls)
	}
}

func TestScriptWrapProvider_Infer_CursorParsesJSONResult(t *testing.T) {
	stdout := cursorpkg.SuccessStdoutJSON("Parsed assistant answer.", "cursor-session-abc")
	stdout = append(stdout, '\n')
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: stdout},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	resp, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCursor),
		Model:         "gpt-5",
		UserMessage:   "run the tests",
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if resp.Content != "Parsed assistant answer." {
		t.Fatalf("content = %q, want parsed result text", resp.Content)
	}
	if resp.Content == string(stdout) {
		t.Fatal("content must not be raw JSON stdout")
	}
	if resp.ProviderSession == nil {
		t.Fatal("expected provider session metadata")
	}
	if resp.ProviderSession.Provider != "cursor" {
		t.Fatalf("provider = %q, want cursor", resp.ProviderSession.Provider)
	}
	if resp.ProviderSession.ID != "cursor-session-abc" {
		t.Fatalf("session id = %q, want cursor-session-abc", resp.ProviderSession.ID)
	}
	if resp.Diagnostics == nil || resp.Diagnostics.Command == nil {
		t.Fatal("expected command diagnostics on success")
	}
	if string(resp.Diagnostics.Command.Stdout) != string(stdout) {
		t.Fatal("command diagnostics should retain raw stdout for observability")
	}
}

func TestScriptWrapProvider_Infer_CursorMalformedJSONReturnsProviderError(t *testing.T) {
	stdout := []byte(`{"type":"result"`)
	stderr := []byte("cursor stderr detail")
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: stdout, Stderr: stderr},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCursor),
		UserMessage:   "run the tests",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Type != interfaces.WorkFailureTypePermanentBadRequest {
		t.Fatalf("error type = %q, want permanent_bad_request", providerErr.Type)
	}
	if providerErr.Diagnostics == nil || providerErr.Diagnostics.Command == nil {
		t.Fatal("expected command diagnostics on parse failure")
	}
	if got := providerErr.Diagnostics.Command.Stdout; got != string(stdout) {
		t.Fatalf("command stdout = %q, want full stdout for worker-internal diagnostics", got)
	}
	if got := providerErr.Diagnostics.Command.Stderr; got != string(stderr) {
		t.Fatalf("command stderr = %q, want full stderr for worker-internal diagnostics", got)
	}
	assertCursorFailureExcerpts(t, providerErr.Diagnostics, string(stdout), string(stderr))
	assertSafeCursorFailureExcerpts(t, providerErr.Diagnostics)
}

func TestScriptWrapProvider_Infer_CursorExitFailurePreservesBoundedDiagnosticsExcerpts(t *testing.T) {
	stdout := []byte("partial json output")
	stderr := []byte("noise before\nERROR: unexpected status 500 from cursor upstream")
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: 1,
		},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCursor),
		UserMessage:   "run the tests",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Type != interfaces.WorkFailureTypeInternalServerError {
		t.Fatalf("error type = %q, want internal_server_error", providerErr.Type)
	}
	if providerErr.Message != "ERROR: unexpected status 500 from cursor upstream" {
		t.Fatalf("error message = %q", providerErr.Message)
	}
	assertCursorFailureExcerpts(t, providerErr.Diagnostics, string(stdout), string(stderr))
	assertSafeCursorFailureExcerpts(t, providerErr.Diagnostics)
}

func assertCursorFailureExcerpts(t *testing.T, diagnostics *interfaces.WorkDiagnostics, wantStdout, wantStderr string) {
	t.Helper()
	if diagnostics == nil || diagnostics.Provider == nil {
		t.Fatal("expected provider diagnostics with failure excerpts")
	}
	metadata := diagnostics.Provider.ResponseMetadata
	if got := metadata[cursorpkg.ResponseMetadataStdoutExcerpt]; got != wantStdout {
		t.Fatalf("stdout excerpt = %q, want %q", got, wantStdout)
	}
	if got := metadata[cursorpkg.ResponseMetadataStderrExcerpt]; got != wantStderr {
		t.Fatalf("stderr excerpt = %q, want %q", got, wantStderr)
	}
}

func assertSafeCursorFailureExcerpts(t *testing.T, diagnostics *interfaces.WorkDiagnostics) {
	t.Helper()
	safe := interfaces.SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics)
	if safe == nil || safe.Provider == nil {
		t.Fatal("expected safe provider diagnostics")
	}
	if safe.Provider.ResponseMetadata[cursorpkg.ResponseMetadataStdoutExcerpt] == "" {
		t.Fatal("expected safe stdout excerpt")
	}
	if safe.Provider.ResponseMetadata[cursorpkg.ResponseMetadataStderrExcerpt] == "" {
		t.Fatal("expected safe stderr excerpt")
	}
	if safe.Provider.ResponseMetadata["raw_body"] != "" {
		t.Fatal("safe diagnostics must not include unsafe metadata keys")
	}
}

func TestScriptWrapProvider_Infer_NonCodexPayloadUsesExpectedCommandRequestAndNoStdin(t *testing.T) {
	for _, tc := range nonCodexInferencePayloadTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			stdout := []byte(strings.ToLower(tc.name) + " output")
			if tc.req.ModelProvider == string(interfaces.ModelProviderCursor) {
				stdout = cursorpkg.SuccessStdoutJSON(strings.ToLower(tc.name)+" output", "cursor-session-from-json")
			}
			fakeExec := &recordingProviderExec{
				result: CommandResult{Stdout: stdout},
			}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			resp, err := provider.Infer(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("Infer returned error: %v", err)
			}
			wantContent := strings.ToLower(tc.name) + " output"
			if resp.Content != wantContent {
				t.Fatalf("response content = %q, want %q", resp.Content, wantContent)
			}
			if fakeExec.request.Command != tc.req.ModelProvider {
				t.Fatalf("command = %q, want %q", fakeExec.request.Command, tc.req.ModelProvider)
			}
			assertStringSlicesEqual(t, tc.wantArgs, fakeExec.request.Args)
			if len(fakeExec.request.Stdin) != 0 {
				t.Fatalf("expected non-codex request not to use stdin, got %q", string(fakeExec.request.Stdin))
			}
			if fakeExec.request.WorkDir != tc.wantWorkDir {
				t.Fatalf("workdir = %q, want %q", fakeExec.request.WorkDir, tc.wantWorkDir)
			}
			assertEnvContains(t, fakeExec.request.Env, tc.wantEnv)
		})
	}
}

type nonCodexInferencePayloadTestCase struct {
	name        string
	req         interfaces.ProviderInferenceRequest
	wantArgs    []string
	wantWorkDir string
	wantEnv     string
}

func nonCodexInferencePayloadTestCases() []nonCodexInferencePayloadTestCase {
	return []nonCodexInferencePayloadTestCase{
		{
			name: "Gemini",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderGemini),
				Model:         "gemini-2.5-flash",
				UserMessage:   "run the tests",
				EnvVars: map[string]string{
					"AGENT_FACTORY_GEMINI_ENV": "enabled",
				},
			},
			wantArgs: []string{"--prompt", "run the tests", "--model", "gemini-2.5-flash"},
			wantEnv:  "AGENT_FACTORY_GEMINI_ENV=enabled",
		},
		{
			name: "Kiro",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderKiro),
				SystemPrompt:  "You are a careful reviewer.",
				SessionID:     "kiro-session-123",
				UserMessage:   "run the tests",
				EnvVars: map[string]string{
					"AGENT_FACTORY_KIRO_ENV": "enabled",
				},
			},
			wantArgs: []string{
				"chat",
				"--no-interactive",
				"--resume-id",
				"kiro-session-123",
				"System instructions:\nYou are a careful reviewer.\n\nUser request:\nrun the tests",
			},
			wantEnv: "AGENT_FACTORY_KIRO_ENV=enabled",
		},
		{
			name: "Cursor",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(interfaces.ModelProviderCursor),
				Model:         "gpt-5",
				SessionID:     "cursor-session-123",
				UserMessage:   "run the tests",
				EnvVars: map[string]string{
					"AGENT_FACTORY_CURSOR_ENV": "enabled",
				},
			},
			wantArgs: []string{"-p", "--model", "gpt-5", "--resume", "cursor-session-123", "--output-format", "json", "run the tests"},
			wantEnv:  "AGENT_FACTORY_CURSOR_ENV=enabled",
		},
		{
			name: "OpenCode",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider:    string(interfaces.ModelProviderOpenCode),
				Model:            "openai/gpt-5",
				SessionID:        "opencode-session-123",
				WorkingDirectory: "/tmp/project",
				UserMessage:      "run the tests",
				EnvVars: map[string]string{
					"AGENT_FACTORY_OPENCODE_ENV": "enabled",
				},
			},
			wantArgs:    []string{"run", "--model", "openai/gpt-5", "--session", "opencode-session-123", "--dir", "/tmp/project", "run the tests"},
			wantWorkDir: "/tmp/project",
			wantEnv:     "AGENT_FACTORY_OPENCODE_ENV=enabled",
		},
	}
}

func TestScriptWrapProvider_Infer_AttachesSharedCommandDiagnosticsToResponse(t *testing.T) {
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			Stdout: []byte("codex diagnostic output"),
			Stderr: []byte("codex diagnostic stderr"),
		},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	resp, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider:    string(interfaces.ModelProviderCodex),
		Model:            "gpt-5-codex",
		WorkingDirectory: "C:\\repo",
		UserMessage:      "diagnose this",
		EnvVars: map[string]string{
			"AGENT_FACTORY_DIAG_ENV": "present",
		},
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if resp.Diagnostics == nil || resp.Diagnostics.Command == nil {
		t.Fatal("expected shared command diagnostics on provider response")
	}
	diag := resp.Diagnostics.Command
	if diag.Command != string(interfaces.ModelProviderCodex) {
		t.Fatalf("diagnostic command = %q, want codex", diag.Command)
	}
	// "--cd", "C:\\repo",
	expectedArgs := []string{"exec", "--model", "gpt-5-codex", "-"}
	assertStringSlicesEqual(t, expectedArgs, diag.Args)
	if diag.Stdin != "diagnose this" {
		t.Fatalf("diagnostic stdin = %q, want prompt", diag.Stdin)
	}
	if diag.Stdout != "codex diagnostic output" {
		t.Fatalf("diagnostic stdout = %q", diag.Stdout)
	}
	if diag.Stderr != "codex diagnostic stderr" {
		t.Fatalf("diagnostic stderr = %q", diag.Stderr)
	}
	if diag.ExitCode != 0 {
		t.Fatalf("diagnostic exit code = %d, want 0", diag.ExitCode)
	}
	if diag.WorkingDir != "C:\\repo" {
		t.Fatalf("diagnostic workdir = %q, want C:\\repo", diag.WorkingDir)
	}
	if diag.Env["AGENT_FACTORY_DIAG_ENV"] != MetadataOnlyCommandEnvValue {
		t.Fatalf("diagnostic env AGENT_FACTORY_DIAG_ENV = %q, want metadata marker", diag.Env["AGENT_FACTORY_DIAG_ENV"])
	}
}

func TestScriptWrapProvider_Infer_ConsumesCanonicalWorkDispatchInputTokens(t *testing.T) {
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("provider output")},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))
	inputToken := interfaces.Token{
		ID: "token-1",
		Color: interfaces.TokenColor{
			WorkID:     "work-1",
			WorkTypeID: "task",
		},
	}

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		Dispatch: interfaces.WorkDispatch{
			WorkerType:      "worker-a",
			WorkstationName: "review",
			InputTokens:     InputTokens(inputToken),
			InputBindings:   map[string][]string{"task": {"token-1"}},
		},
		WorkerType:       "worker-a",
		WorkstationType:  "review",
		ModelProvider:    string(interfaces.ModelProviderCodex),
		Model:            "gpt-5-codex",
		UserMessage:      "fix it",
		InputTokens:      InputTokens(inputToken),
		WorkingDirectory: "C:\\repo",
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}

	if fakeExec.request.WorkerType != "worker-a" || fakeExec.request.WorkstationName != "review" {
		t.Fatalf("command request identity = worker %q workstation %q", fakeExec.request.WorkerType, fakeExec.request.WorkstationName)
	}
	commandTokens := CommandRequestInputTokens(fakeExec.request)
	if len(commandTokens) != 1 || commandTokens[0].ID != inputToken.ID || commandTokens[0].Color.WorkID != inputToken.Color.WorkID {
		t.Fatalf("command input tokens = %#v, want %#v", commandTokens, inputToken)
	}
	if got := fakeExec.request.InputBindings["task"]; len(got) != 1 || got[0] != "token-1" {
		t.Fatalf("command input bindings = %#v", fakeExec.request.InputBindings)
	}
}

func TestScriptWrapProvider_Infer_CommandDiagnosticsRedactSensitiveEnvWithoutChangingExecution(t *testing.T) {
	rawSecret := "provider-secret-value"
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("provider diagnostic output")},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	resp, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderClaude),
		Model:         "claude-sonnet-4",
		UserMessage:   "diagnose provider env",
		EnvVars: map[string]string{
			"ANTHROPIC_API_KEY":        rawSecret,
			"PROVIDER_CONTEXT_DIR":     "C:\\repo",
			"GIT_TERMINAL_PROMPT":      "1",
			"AGENT_FACTORY_AUTH_TOKEN": "runner-token",
		},
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	assertEnvContains(t, fakeExec.request.Env, "ANTHROPIC_API_KEY="+rawSecret)
	assertEnvContains(t, fakeExec.request.Env, "AGENT_FACTORY_AUTH_TOKEN=runner-token")

	if resp.Diagnostics == nil || resp.Diagnostics.Command == nil {
		t.Fatal("expected shared command diagnostics on provider response")
	}
	diag := resp.Diagnostics.Command
	if got := diag.Env["ANTHROPIC_API_KEY"]; got != RedactedCommandEnvValue {
		t.Fatalf("diagnostic env ANTHROPIC_API_KEY = %q, want redaction marker", got)
	}
	if got := diag.Env["AGENT_FACTORY_AUTH_TOKEN"]; got != RedactedCommandEnvValue {
		t.Fatalf("diagnostic env AGENT_FACTORY_AUTH_TOKEN = %q, want redaction marker", got)
	}
	if got := diag.Env["PROVIDER_CONTEXT_DIR"]; got != MetadataOnlyCommandEnvValue {
		t.Fatalf("diagnostic env PROVIDER_CONTEXT_DIR = %q, want metadata marker", got)
	}
	if got := diag.Env["GIT_TERMINAL_PROMPT"]; got != "0" {
		t.Fatalf("diagnostic env GIT_TERMINAL_PROMPT = %q, want automation default", got)
	}
	if strings.Contains(strings.Join(mapValues(diag.Env), "\n"), rawSecret) {
		t.Fatalf("diagnostic env leaked raw provider secret: %#v", diag.Env)
	}

	metadata := resp.Diagnostics.Metadata
	if metadata["env_count"] == "" {
		t.Fatalf("diagnostic metadata missing env_count: %#v", metadata)
	}
	if !strings.Contains(metadata["env_keys"], "ANTHROPIC_API_KEY") {
		t.Fatalf("diagnostic metadata env_keys missing ANTHROPIC_API_KEY: %#v", metadata)
	}

	fullDiagnostics := withInferenceResponseDiagnostics(workDiagnosticsForInferenceRequest(interfaces.ProviderInferenceRequest{
		ModelProvider:    string(interfaces.ModelProviderClaude),
		Model:            "claude-sonnet-4",
		WorkerType:       interfaces.WorkerTypeModel,
		WorkstationType:  "review",
		WorkingDirectory: "C:\\repo",
	}), resp, 2)
	if fullDiagnostics.Provider.Provider != string(interfaces.ModelProviderClaude) {
		t.Fatalf("provider diagnostic provider = %q, want claude", fullDiagnostics.Provider.Provider)
	}
	if fullDiagnostics.Provider.Model != "claude-sonnet-4" {
		t.Fatalf("provider diagnostic model = %q, want claude-sonnet-4", fullDiagnostics.Provider.Model)
	}
	if fullDiagnostics.Provider.ResponseMetadata["retry_count"] != "2" {
		t.Fatalf("provider diagnostic retry_count = %q, want 2", fullDiagnostics.Provider.ResponseMetadata["retry_count"])
	}
	if fullDiagnostics.Command.Env["ANTHROPIC_API_KEY"] != RedactedCommandEnvValue {
		t.Fatalf("merged provider diagnostics lost command env redaction: %#v", fullDiagnostics.Command.Env)
	}
}

func TestScriptWrapProvider_Infer_CodexWithoutSessionLeavesMetadataNil(t *testing.T) {
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("codex output without session")},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	resp, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderCodex),
		Model:         "gpt-5-codex",
		UserMessage:   "fix it",
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if resp.ProviderSession != nil {
		t.Fatalf("expected provider session to be nil, got %#v", resp.ProviderSession)
	}
}

func TestScriptWrapProvider_Infer_ExitFailureIncludesExitCodeAndProcessOutput(t *testing.T) {
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			Stdout:   []byte("partial output"),
			Stderr:   []byte("rate limited"),
			ExitCode: 1,
		},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(interfaces.ModelProviderClaude),
		UserMessage:   "hello",
	})
	if err == nil {
		t.Fatal("expected Infer to fail when exec returns a non-zero exit code")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Diagnostics == nil || providerErr.Diagnostics.Command == nil {
		t.Fatal("expected shared command diagnostics on provider error")
	}
	diag := providerErr.Diagnostics.Command
	if diag.Command != string(interfaces.ModelProviderClaude) {
		t.Fatalf("diagnostic command = %q, want claude", diag.Command)
	}
	if diag.Stdout != "partial output" {
		t.Fatalf("diagnostic stdout = %q", diag.Stdout)
	}
	if diag.Stderr != "rate limited" {
		t.Fatalf("diagnostic stderr = %q", diag.Stderr)
	}
	if diag.ExitCode != 1 {
		t.Fatalf("diagnostic exit code = %d, want 1", diag.ExitCode)
	}
	// got := err.Error()
	// for _, want := range []string{
	// 	"claude exited with code 1",
	// 	"stderr: rate limited",
	// 	"stdout: partial output",
	// } {
	// 	if !strings.Contains(got, want) {
	// 		t.Fatalf("expected error %q to contain %q", got, want)
	// 	}
	// }
}

// portos:func-length-exception owner=agent-factory reason=legacy-codex-provider-error-table review=2026-07-19 removal=split-command-output-fixtures-and-shared-contract-assertions-before-next-provider-normalization-change
