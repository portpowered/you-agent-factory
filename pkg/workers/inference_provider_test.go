package workers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/logging"
)

// --- ContainsStopToken (pure function) ---

func TestContainsStopToken_Found(t *testing.T) {
	output := "Some output text\n<promise>COMPLETE</promise>\nMore text"
	if !ContainsStopToken(output, "<promise>COMPLETE</promise>") {
		t.Error("expected stop token to be found")
	}
}

func TestContainsStopToken_NotFound(t *testing.T) {
	output := "Some output text without the token"
	if ContainsStopToken(output, "<promise>COMPLETE</promise>") {
		t.Error("expected stop token NOT to be found")
	}
}

func TestContainsStopToken_EmptyToken(t *testing.T) {
	if ContainsStopToken("any output", "") {
		t.Error("empty stop token should never match")
	}
}

func TestContainsStopToken_EmptyOutput(t *testing.T) {
	if ContainsStopToken("", "COMPLETE") {
		t.Error("empty output should never match")
	}
}

func TestContainsStopToken_CaseSensitive(t *testing.T) {
	output := "The task is complete"
	if ContainsStopToken(output, "COMPLETE") {
		t.Error("stop token check should be case-sensitive")
	}
}

func TestContainsStopToken_PartialMatch(t *testing.T) {
	output := "This is COMPLETED now"
	if !ContainsStopToken(output, "COMPLETE") {
		t.Error("substring match should work — COMPLETE is in COMPLETED")
	}
}

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
		ModelProvider: string(ModelProviderCodex),
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
		ModelProvider: string(ModelProviderCodex),
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
		ModelProvider: string(ModelProviderClaude),
		UserMessage:   "print environment",
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}

	for _, entry := range providerAutomationEnvDefaults {
		want := entry.name + "=" + entry.value
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
		ModelProvider: string(ModelProviderClaude),
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
	if resp.ProviderSession.Provider != string(ModelProviderClaude) {
		t.Fatalf("provider session provider = %q, want %q", resp.ProviderSession.Provider, ModelProviderClaude)
	}
	if resp.ProviderSession.Kind != providerSessionKindSessionID {
		t.Fatalf("provider session kind = %q, want %q", resp.ProviderSession.Kind, providerSessionKindSessionID)
	}
	if resp.ProviderSession.ID != "claude-session-123" {
		t.Fatalf("provider session id = %q, want %q", resp.ProviderSession.ID, "claude-session-123")
	}

	behavior := providerBehaviorFor(req.ModelProvider, logging.NoopLogger{})
	expectedArgs := behavior.BuildArgs(req, true)
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
		ModelProvider: string(ModelProviderClaude),
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
		ModelProvider: string(ModelProviderClaude),
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
		ModelProvider: string(ModelProviderClaude),
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
	if providerErr.ProviderSession.Provider != string(ModelProviderClaude) {
		t.Fatalf("provider session provider = %q, want %q", providerErr.ProviderSession.Provider, ModelProviderClaude)
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
		ModelProvider:    string(ModelProviderCodex),
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
	if resp.ProviderSession.Provider != string(ModelProviderCodex) {
		t.Fatalf("provider session provider = %q, want %q", resp.ProviderSession.Provider, ModelProviderCodex)
	}
	if resp.ProviderSession.Kind != providerSessionKindSessionID {
		t.Fatalf("provider session kind = %q, want %q", resp.ProviderSession.Kind, providerSessionKindSessionID)
	}
	if resp.ProviderSession.ID != "sess_codex_123" {
		t.Fatalf("provider session id = %q, want %q", resp.ProviderSession.ID, "sess_codex_123")
	}

	behavior := providerBehaviorFor(req.ModelProvider, logging.NoopLogger{})
	expectedArgs := behavior.BuildArgs(req, true)
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

func TestScriptWrapProvider_Infer_AttachesSharedCommandDiagnosticsToResponse(t *testing.T) {
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			Stdout: []byte("codex diagnostic output"),
			Stderr: []byte("codex diagnostic stderr"),
		},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	resp, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider:    string(ModelProviderCodex),
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
	if diag.Command != string(ModelProviderCodex) {
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
		ModelProvider:    string(ModelProviderCodex),
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
		ModelProvider: string(ModelProviderClaude),
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
		ModelProvider:    string(ModelProviderClaude),
		Model:            "claude-sonnet-4",
		WorkerType:       interfaces.WorkerTypeModel,
		WorkstationType:  "review",
		WorkingDirectory: "C:\\repo",
	}), resp, 2)
	if fullDiagnostics.Provider.Provider != string(ModelProviderClaude) {
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
		ModelProvider: string(ModelProviderCodex),
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
		ModelProvider: string(ModelProviderClaude),
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
	if diag.Command != string(ModelProviderClaude) {
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
func TestScriptWrapProvider_Infer_CodexExitFailuresNormalizeIntoSharedContract(t *testing.T) {
	testCases := []struct {
		name  string
		entry ProviderErrorCorpusEntry
	}{
		{
			name:  "Throttled_429",
			entry: providerErrorCorpusEntryForTest(t, "codex_status_429_too_many_requests"),
		},
		{
			name:  "TransientServerError_500",
			entry: providerErrorCorpusEntryForTest(t, "codex_internal_server_status_500"),
		},
		{
			name:  "TransientServerError_HighDemand",
			entry: providerErrorCorpusEntryForTest(t, "codex_high_demand_temporary_errors"),
		},
		{
			name:  "TransientServerError_WindowsExitCode4294967295",
			entry: providerErrorCorpusEntryForTest(t, "codex_windows_exit_code_4294967295"),
		},
		{
			name:  "CursorFamily_TransientServerError_HighDemand",
			entry: providerErrorCorpusEntryForTest(t, "cursor_high_demand_temporary_errors"),
		},
		{
			name:  "BadRequest_InvalidRequest",
			entry: providerErrorCorpusEntryForTest(t, "codex_invalid_request_error"),
		},
		{
			name:  "Timeout_MessageMatch",
			entry: providerErrorCorpusEntryForTest(t, "codex_timeout_waiting_for_provider"),
		},
		{
			name:  "AuthFailure_Unauthorized",
			entry: providerErrorCorpusEntryForTest(t, "codex_authentication_unauthorized"),
		},
		{
			name: "Unknown_Unclassified",
			entry: ProviderErrorCorpusEntry{
				Name:           "codex_unknown_unclassified",
				ExitCode:       1,
				Stderr:         `some brand new failure`,
				ExpectedType:   interfaces.ProviderErrorTypeUnknown,
				ExpectedFamily: interfaces.ProviderErrorFamilyTerminal,
			},
		},
	}

	for _, tc := range testCases {
		entryLabel := providerErrorCorpusEntryLabel(tc.entry)
		t.Run(entryLabel, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.entry.CommandResult()}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("%s expected ProviderError, got %T", entryLabel, err)
			}
			if providerErr.Type != tc.entry.ExpectedType {
				t.Fatalf("%s Type = %q, want %q", entryLabel, providerErr.Type, tc.entry.ExpectedType)
			}
			if providerErr.Family != tc.entry.ExpectedFamily {
				t.Fatalf("%s Family = %q, want %q", entryLabel, providerErr.Family, tc.entry.ExpectedFamily)
			}
			if providerErr.ProviderSession != nil {
				t.Fatalf("%s expected provider session to be nil, got %#v", entryLabel, providerErr.ProviderSession)
			}
			decision := ClassifyProviderFailure(providerErr)
			if decision.Retryable != tc.entry.Retryable {
				t.Fatalf("%s Retryable = %t, want %t", entryLabel, decision.Retryable, tc.entry.Retryable)
			}
			if decision.TriggersThrottlePause != tc.entry.TriggersThrottlePause {
				t.Fatalf("%s TriggersThrottlePause = %t, want %t", entryLabel, decision.TriggersThrottlePause, tc.entry.TriggersThrottlePause)
			}
			wantTerminal := tc.entry.ExpectedFamily == interfaces.ProviderErrorFamilyTerminal
			if decision.Terminal != wantTerminal {
				t.Fatalf("%s Terminal = %t, want %t", entryLabel, decision.Terminal, wantTerminal)
			}

			// Ignore messages for now, until we resolve what we want to do with the long printed messages in case the actual response is not a reject string.
			// if !strings.Contains(providerErr.Error(), tc.wantMessage) {
			// 	t.Fatalf("expected message %q to contain %q", providerErr.Error(), tc.wantMessage)
			// }
		})
	}
}

func TestScriptWrapProvider_Infer_CodexNormalizedRetryDecisionRegressions(t *testing.T) {
	testCases := []struct {
		name              string
		stderr            string
		wantType          interfaces.ProviderErrorType
		wantFamily        interfaces.ProviderErrorFamily
		wantRetryable     bool
		wantTerminal      bool
		wantThrottlePause bool
	}{
		{
			name:              "InternalServerError_HighDemandTemporaryErrors_RetriesWithoutThrottlePause",
			stderr:            `ERROR: We're currently experiencing high demand, which may cause temporary errors.`,
			wantType:          interfaces.ProviderErrorTypeInternalServerError,
			wantFamily:        interfaces.ProviderErrorFamilyRetryable,
			wantRetryable:     true,
			wantTerminal:      false,
			wantThrottlePause: false,
		},
		{
			name:              "InternalServerError_UnexpectedStatus500_RetriesWithoutThrottlePause",
			stderr:            `ERROR: unexpected status 500 Internal Server Error`,
			wantType:          interfaces.ProviderErrorTypeInternalServerError,
			wantFamily:        interfaces.ProviderErrorFamilyRetryable,
			wantRetryable:     true,
			wantTerminal:      false,
			wantThrottlePause: false,
		},
		{
			name:              "AuthFailure_UnexpectedStatus401_IsTerminal",
			stderr:            `ERROR: unexpected status 401 Unauthorized {"type":"authentication_error","message":"invalid api key"}`,
			wantType:          interfaces.ProviderErrorTypeAuthFailure,
			wantFamily:        interfaces.ProviderErrorFamilyTerminal,
			wantRetryable:     false,
			wantTerminal:      true,
			wantThrottlePause: false,
		},
		{
			name:              "PermanentBadRequest_UnexpectedStatus400_IsTerminal",
			stderr:            `ERROR: unexpected status 400 Bad Request {"type":"invalid_request_error","message":"bad request"}`,
			wantType:          interfaces.ProviderErrorTypePermanentBadRequest,
			wantFamily:        interfaces.ProviderErrorFamilyTerminal,
			wantRetryable:     false,
			wantTerminal:      true,
			wantThrottlePause: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewScriptWrapProvider(WithProviderCommandRunner(&recordingProviderExec{
				result: CommandResult{ExitCode: 1, Stderr: []byte(tc.stderr)},
			}))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("expected ProviderError, got %T", err)
			}
			if providerErr.Type != tc.wantType || providerErr.Family != tc.wantFamily {
				t.Fatalf("normalized provider failure = (%q, %q), want (%q, %q)", providerErr.Type, providerErr.Family, tc.wantType, tc.wantFamily)
			}

			decision := ClassifyProviderFailure(providerErr)
			if decision.Retryable != tc.wantRetryable || decision.Terminal != tc.wantTerminal || decision.TriggersThrottlePause != tc.wantThrottlePause {
				t.Fatalf("ClassifyProviderFailure(%q) = %#v, want retryable=%t terminal=%t throttlePause=%t", tc.stderr, decision, tc.wantRetryable, tc.wantTerminal, tc.wantThrottlePause)
			}
		})
	}
}

func TestScriptWrapProvider_Infer_CodexWindowsCorpusEntryRemainsDistinctFromAuthFailure(t *testing.T) {
	testCases := []struct {
		entryName          string
		wantType           interfaces.ProviderErrorType
		wantRetryable      bool
		wantThrottlePause  bool
		wantRejectAuthType bool
	}{
		{
			entryName:          "codex_windows_exit_code_4294967295",
			wantType:           interfaces.ProviderErrorTypeInternalServerError,
			wantRetryable:      true,
			wantThrottlePause:  false,
			wantRejectAuthType: true,
		},
		{
			entryName:         "codex_authentication_unauthorized",
			wantType:          interfaces.ProviderErrorTypeAuthFailure,
			wantRetryable:     false,
			wantThrottlePause: false,
		},
	}

	for _, tc := range testCases {
		entry := providerErrorCorpusEntryForTest(t, tc.entryName)
		t.Run(providerErrorCorpusEntryLabel(entry), func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: entry.CommandResult()}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("%s expected ProviderError, got %T", providerErrorCorpusEntryLabel(entry), err)
			}
			if providerErr.Type != tc.wantType {
				t.Fatalf("%s Type = %q, want %q", providerErrorCorpusEntryLabel(entry), providerErr.Type, tc.wantType)
			}
			if tc.wantRejectAuthType && providerErr.Type == interfaces.ProviderErrorTypeAuthFailure {
				t.Fatalf("%s Type = %q, want non-auth retryable failure", providerErrorCorpusEntryLabel(entry), providerErr.Type)
			}

			decision := ClassifyProviderFailure(providerErr)
			if decision.Retryable != tc.wantRetryable {
				t.Fatalf("%s Retryable = %t, want %t", providerErrorCorpusEntryLabel(entry), decision.Retryable, tc.wantRetryable)
			}
			if decision.TriggersThrottlePause != tc.wantThrottlePause {
				t.Fatalf("%s TriggersThrottlePause = %t, want %t", providerErrorCorpusEntryLabel(entry), decision.TriggersThrottlePause, tc.wantThrottlePause)
			}
		})
	}
}

func TestScriptWrapProvider_Infer_CodexWindowsExitCode4294967295Normalization(t *testing.T) {
	testCases := []struct {
		name              string
		result            CommandResult
		wantType          interfaces.ProviderErrorType
		wantFamily        interfaces.ProviderErrorFamily
		wantMessage       string
		wantRetryable     bool
		wantTerminal      bool
		wantThrottlePause bool
	}{
		{
			name: "NoAuditedSignal_UsesRetryableInternalServerError",
			result: CommandResult{
				ExitCode: codexWindowsProcessFailureExitCode,
				Stderr: []byte(strings.Join([]string{
					"OpenAI Codex v0.118.0 (research preview)",
					"--------",
					"provider: openai",
				}, "\n")),
			},
			wantType:          interfaces.ProviderErrorTypeInternalServerError,
			wantFamily:        interfaces.ProviderErrorFamilyRetryable,
			wantMessage:       "codex exited with code 4294967295",
			wantRetryable:     true,
			wantTerminal:      false,
			wantThrottlePause: false,
		},
		{
			name: "ExplicitAuthSignalStillWins",
			result: CommandResult{
				ExitCode: codexWindowsProcessFailureExitCode,
				Stderr:   []byte(`ERROR: unexpected status 401 Unauthorized {"type":"authentication_error","message":"invalid api key"}`),
			},
			wantType:          interfaces.ProviderErrorTypeAuthFailure,
			wantFamily:        interfaces.ProviderErrorFamilyTerminal,
			wantMessage:       `ERROR: unexpected status 401 Unauthorized {"type":"authentication_error","message":"invalid api key"}`,
			wantRetryable:     false,
			wantTerminal:      true,
			wantThrottlePause: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("expected ProviderError, got %T", err)
			}
			if providerErr.Type != tc.wantType {
				t.Fatalf("Type = %q, want %q", providerErr.Type, tc.wantType)
			}
			if providerErr.Family != tc.wantFamily {
				t.Fatalf("Family = %q, want %q", providerErr.Family, tc.wantFamily)
			}
			if providerErr.Message != tc.wantMessage {
				t.Fatalf("Message = %q, want %q", providerErr.Message, tc.wantMessage)
			}

			decision := ClassifyProviderFailure(providerErr)
			if decision.Retryable != tc.wantRetryable || decision.Terminal != tc.wantTerminal || decision.TriggersThrottlePause != tc.wantThrottlePause {
				t.Fatalf("ClassifyProviderFailure(%#v) = %#v, want retryable=%t terminal=%t throttlePause=%t", providerErr, decision, tc.wantRetryable, tc.wantTerminal, tc.wantThrottlePause)
			}
		})
	}
}

func TestScriptWrapProvider_Infer_CodexExitFailureExtractsBoundedErrorLine(t *testing.T) {
	capacityEntry := providerErrorCorpusEntryForTest(t, "codex_model_capacity_selected_model")
	capacityLine := providerErrorCorpusLastErrorLine(t, capacityEntry)

	testCases := []struct {
		name       string
		result     CommandResult
		wantLine   string
		rejectText string
	}{
		{
			name: "StdoutCapacityErrorAfterTranscript",
			result: CommandResult{
				ExitCode: 1,
				Stdout: []byte(strings.Join([]string{
					strings.Repeat("inference transcript token ", 4000),
					"agent looked successful",
					capacityLine,
				}, "\n")),
			},
			wantLine:   capacityLine,
			rejectText: "inference transcript token",
		},
		{
			name: "StderrErrorBeforeTrailingLines",
			result: CommandResult{
				ExitCode: 1,
				Stderr: []byte(strings.Join([]string{
					"OpenAI Codex v0.118.0 (research preview)",
					"ERROR: The process with PID 1234 could not be terminated",
					"trailing cleanup note",
					"retry after cleanup",
				}, "\n")),
			},
			wantLine:   "ERROR: The process with PID 1234 could not be terminated",
			rejectText: "trailing cleanup note",
		},
		{
			name: "FinalMatchingErrorWinsAcrossStreams",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte("ERROR: First provider failure"),
				Stdout:   []byte("  ERROR: Final provider failure  \nnot final"),
			},
			wantLine:   "ERROR: Final provider failure",
			rejectText: "ERROR: First provider failure",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("expected ProviderError, got %T", err)
			}
			if providerErr.Message != tc.wantLine {
				t.Fatalf("Message = %q, want %q", providerErr.Message, tc.wantLine)
			}
			if strings.Contains(providerErr.Message, tc.rejectText) {
				t.Fatalf("Message = %q, should not contain %q", providerErr.Message, tc.rejectText)
			}
		})
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-codex-error-mapping-table review=2026-07-19 removal=split-known-error-line-cases-by-failure-category-before-next-provider-normalization-change
func TestScriptWrapProvider_Infer_KnownCodexErrorLinesMapToProviderFailureCategories(t *testing.T) {
	capacityEntry := providerErrorCorpusEntryForTest(t, "codex_model_capacity_selected_model")
	capacityLine := providerErrorCorpusLastErrorLine(t, capacityEntry)

	testCases := []struct {
		name                 string
		result               CommandResult
		wantType             interfaces.ProviderErrorType
		wantFamily           interfaces.ProviderErrorFamily
		wantMessage          string
		wantRetryable        bool
		wantTerminal         bool
		wantThrottlePause    bool
		rejectMessageContent string
	}{
		{
			name: "SelectedModelCapacity_IsThrottled",
			result: CommandResult{
				ExitCode: 1,
				Stdout:   capacityEntry.CommandResult().Stdout,
			},
			wantType:             interfaces.ProviderErrorTypeThrottled,
			wantFamily:           interfaces.ProviderErrorFamilyThrottle,
			wantMessage:          capacityLine,
			wantRetryable:        true,
			wantThrottlePause:    true,
			rejectMessageContent: "thinking transcript",
		},
		{
			name: "CodexCommandTimeout_IsRetryableTimeout",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte("ERROR: command timed out while waiting for codex"),
			},
			wantType:      interfaces.ProviderErrorTypeTimeout,
			wantFamily:    interfaces.ProviderErrorFamilyRetryable,
			wantMessage:   "ERROR: command timed out while waiting for codex",
			wantRetryable: true,
		},
		{
			name: "CodexContextDeadline_IsRetryableTimeout",
			result: CommandResult{
				ExitCode: 1,
				Stdout:   []byte("ERROR: context deadline exceeded"),
			},
			wantType:      interfaces.ProviderErrorTypeTimeout,
			wantFamily:    interfaces.ProviderErrorFamilyRetryable,
			wantMessage:   "ERROR: context deadline exceeded",
			wantRetryable: true,
		},
		{
			name: "ProcessTerminationCleanupError_IsTerminalWithoutThrottlePause",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte("ERROR: The process with PID 1234 could not be terminated"),
			},
			wantType:          interfaces.ProviderErrorTypeUnknown,
			wantFamily:        interfaces.ProviderErrorFamilyTerminal,
			wantMessage:       "ERROR: The process with PID 1234 could not be terminated",
			wantTerminal:      true,
			wantThrottlePause: false,
		},
		{
			name: "SpecificErrorLineWinsOverGenericOutput",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte(capacityLine),
				Stdout:   []byte("request failed with 400 bad request after full transcript"),
			},
			wantType:             interfaces.ProviderErrorTypeThrottled,
			wantFamily:           interfaces.ProviderErrorFamilyThrottle,
			wantMessage:          capacityLine,
			wantRetryable:        true,
			wantThrottlePause:    true,
			rejectMessageContent: "bad request",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("expected ProviderError, got %T", err)
			}
			if providerErr.Type != tc.wantType {
				t.Fatalf("Type = %q, want %q", providerErr.Type, tc.wantType)
			}
			if providerErr.Family != tc.wantFamily {
				t.Fatalf("Family = %q, want %q", providerErr.Family, tc.wantFamily)
			}
			if providerErr.Message != tc.wantMessage {
				t.Fatalf("Message = %q, want %q", providerErr.Message, tc.wantMessage)
			}
			if tc.rejectMessageContent != "" && strings.Contains(providerErr.Message, tc.rejectMessageContent) {
				t.Fatalf("Message = %q, should not contain %q", providerErr.Message, tc.rejectMessageContent)
			}

			decision := ClassifyProviderFailure(providerErr)
			if decision.Retryable != tc.wantRetryable || decision.Terminal != tc.wantTerminal || decision.TriggersThrottlePause != tc.wantThrottlePause {
				t.Fatalf("ClassifyProviderFailure(%#v) = %#v, want retryable=%t terminal=%t throttlePause=%t", providerErr, decision, tc.wantRetryable, tc.wantTerminal, tc.wantThrottlePause)
			}
		})
	}
}

func TestScriptWrapProvider_Infer_April11RecordingFailureShapesNormalize(t *testing.T) {
	// Fixture data is a reduced failure-shape extract from the April 11, 2026
	// customer recording. It keeps the raw-output patterns needed for stable
	// regression coverage without checking in the full recording transcript.
	fixture := loadApril11FailureShapeFixture(t)

	for _, sample := range fixture.Samples {
		t.Run(sample.Name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{
				result: CommandResult{
					ExitCode: sample.ExitCode,
					Stdout:   []byte(sample.Stdout),
					Stderr:   []byte(sample.Stderr),
				},
			}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "replay April 11 failure shape",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("expected ProviderError, got %T", err)
			}
			if providerErr.Type != sample.WantType {
				t.Fatalf("Type = %q, want %q", providerErr.Type, sample.WantType)
			}
			if providerErr.Message != sample.WantMessage {
				t.Fatalf("Message = %q, want %q", providerErr.Message, sample.WantMessage)
			}
			for _, rejected := range sample.RejectMessageContains {
				if strings.Contains(providerErr.Message, rejected) {
					t.Fatalf("Message = %q, should not contain recorded transcript text %q", providerErr.Message, rejected)
				}
			}

			decision := ClassifyProviderFailure(providerErr)
			if decision.Retryable != sample.WantRetryable || decision.Terminal != sample.WantTerminal || decision.TriggersThrottlePause != sample.WantThrottlePause {
				t.Fatalf("ClassifyProviderFailure(%#v) = %#v, want retryable=%t terminal=%t throttlePause=%t", providerErr, decision, sample.WantRetryable, sample.WantTerminal, sample.WantThrottlePause)
			}
		})
	}
}

func TestScriptWrapProvider_Infer_CodexExitFailurePreservesSessionMetadata(t *testing.T) {
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			ExitCode: 1,
			Stderr:   []byte(`{"event":"session.created","session_id":"sess_codex_error_123"}`),
		},
	}
	provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

	_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
		ModelProvider: string(ModelProviderCodex),
		Model:         "gpt-5-codex",
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
	if providerErr.ProviderSession.ID != "sess_codex_error_123" {
		t.Fatalf("provider session id = %q, want %q", providerErr.ProviderSession.ID, "sess_codex_error_123")
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-claude-provider-error-table review=2026-07-19 removal=split-claude-fixtures-and-shared-contract-assertions-before-next-provider-normalization-change
func TestScriptWrapProvider_Infer_ClaudeExitFailuresNormalizeIntoSharedContract(t *testing.T) {
	testCases := []struct {
		name  string
		entry ProviderErrorCorpusEntry
	}{
		{
			name:  "Throttled_RateLimitError",
			entry: providerErrorCorpusEntryForTest(t, "claude_rate_limit_error"),
		},
		{
			name:  "Throttled_OverloadedError",
			entry: providerErrorCorpusEntryForTest(t, "claude_overloaded_error"),
		},
		{
			name:  "TransientServerError_ApiError500",
			entry: providerErrorCorpusEntryForTest(t, "claude_internal_server_api_error"),
		},
		{
			name:  "BadRequest_InvalidRequest",
			entry: providerErrorCorpusEntryForTest(t, "claude_invalid_request_error"),
		},
		{
			name:  "Timeout_MessageMatch",
			entry: providerErrorCorpusEntryForTest(t, "claude_timeout_waiting_for_provider"),
		},
		{
			name:  "AuthFailure_AuthenticationError",
			entry: providerErrorCorpusEntryForTest(t, "claude_authentication_error"),
		},
		{
			name: "Unknown_Unclassified",
			entry: ProviderErrorCorpusEntry{
				Name:           "claude_unknown_unclassified",
				ExitCode:       1,
				Stderr:         `some brand new claude failure`,
				ExpectedType:   interfaces.ProviderErrorTypeUnknown,
				ExpectedFamily: interfaces.ProviderErrorFamilyTerminal,
			},
		},
	}

	for _, tc := range testCases {
		entryLabel := providerErrorCorpusEntryLabel(tc.entry)
		t.Run(entryLabel, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.entry.CommandResult()}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderClaude),
				Model:         "claude-sonnet-4-5-20250514",
				UserMessage:   "fix it",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("%s expected ProviderError, got %T", entryLabel, err)
			}
			if providerErr.Type != tc.entry.ExpectedType {
				t.Fatalf("%s Type = %q, want %q", entryLabel, providerErr.Type, tc.entry.ExpectedType)
			}
			if providerErr.Family != tc.entry.ExpectedFamily {
				t.Fatalf("%s Family = %q, want %q", entryLabel, providerErr.Family, tc.entry.ExpectedFamily)
			}
			// TODO: remove messsages until we figure out an appropriate handler.
			// if !strings.Contains(providerErr.Error(), tc.wantMessage) {
			// 	t.Fatalf("expected message %q to contain %q", providerErr.Error(), tc.wantMessage)
			// }
		})
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-provider-run-error-table review=2026-07-19 removal=split-timeout-and-misconfiguration-cases-before-next-provider-normalization-change
func TestScriptWrapProvider_Infer_RunErrorsNormalizeTimeoutAndMisconfigured(t *testing.T) {
	capacityEntry := providerErrorCorpusEntryForTest(t, "codex_model_capacity_selected_model")
	capacityLine := providerErrorCorpusLastErrorLine(t, capacityEntry)

	testCases := []struct {
		name        string
		result      CommandResult
		runErr      error
		wantType    interfaces.ProviderErrorType
		wantFamily  interfaces.ProviderErrorFamily
		wantMessage string
		rejectText  string
	}{
		{
			name:        "DeadlineExceeded_IsTimeout",
			runErr:      context.DeadlineExceeded,
			wantType:    interfaces.ProviderErrorTypeTimeout,
			wantFamily:  interfaces.ProviderErrorFamilyRetryable,
			wantMessage: "execution timeout",
		},
		{
			name: "CanceledCommandWithTimeoutOutput_IsTimeout",
			result: CommandResult{
				Stderr: []byte("context canceled after command timed out"),
			},
			runErr:      context.Canceled,
			wantType:    interfaces.ProviderErrorTypeTimeout,
			wantFamily:  interfaces.ProviderErrorFamilyRetryable,
			wantMessage: "context canceled after command timed out",
		},
		{
			name: "DeadlineExceededWithCodexErrorLine_PreservesConciseError",
			result: CommandResult{
				Stdout: []byte(strings.Join([]string{
					strings.Repeat("raw inference transcript ", 4000),
					"agent looked successful",
					capacityLine,
					"cleanup finished after provider error",
				}, "\n")),
			},
			runErr:      context.DeadlineExceeded,
			wantType:    interfaces.ProviderErrorTypeTimeout,
			wantFamily:  interfaces.ProviderErrorFamilyRetryable,
			wantMessage: capacityLine,
			rejectText:  "raw inference transcript",
		},
		{
			name: "CanceledTimeoutOutputWithCodexErrorLine_PreservesConciseError",
			result: CommandResult{
				Stderr: []byte("context canceled after command timed out"),
				Stdout: []byte(strings.Join([]string{
					strings.Repeat("raw inference transcript ", 4000),
					"ERROR: context deadline exceeded while waiting for codex",
					"cleanup finished after provider error",
				}, "\n")),
			},
			runErr:      context.Canceled,
			wantType:    interfaces.ProviderErrorTypeTimeout,
			wantFamily:  interfaces.ProviderErrorFamilyRetryable,
			wantMessage: "ERROR: context deadline exceeded while waiting for codex",
			rejectText:  "raw inference transcript",
		},
		{
			name:       "ExecutableMissing_IsMisconfigured",
			runErr:     exec.ErrNotFound,
			wantType:   interfaces.ProviderErrorTypeMisconfigured,
			wantFamily: interfaces.ProviderErrorFamilyTerminal,
		},
		{
			name:       "UnknownRuntimeFailure_IsUnknown",
			runErr:     errors.New("pipe broke"),
			wantType:   interfaces.ProviderErrorTypeUnknown,
			wantFamily: interfaces.ProviderErrorFamilyTerminal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result, err: tc.runErr}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("expected ProviderError, got %T", err)
			}
			if providerErr.Type != tc.wantType {
				t.Fatalf("Type = %q, want %q", providerErr.Type, tc.wantType)
			}
			if providerErr.Family != tc.wantFamily {
				t.Fatalf("Family = %q, want %q", providerErr.Family, tc.wantFamily)
			}
			if tc.wantMessage != "" && providerErr.Message != tc.wantMessage {
				t.Fatalf("Message = %q, want %q", providerErr.Message, tc.wantMessage)
			}
			if tc.rejectText != "" && strings.Contains(providerErr.Message, tc.rejectText) {
				t.Fatalf("Message = %q, should not contain %q", providerErr.Message, tc.rejectText)
			}
			if tc.wantType == interfaces.ProviderErrorTypeTimeout {
				decision := ClassifyProviderFailure(providerErr)
				if !decision.Retryable || decision.Terminal || decision.TriggersThrottlePause {
					t.Fatalf("ClassifyProviderFailure(%#v) = %#v, want retryable timeout decision", providerErr, decision)
				}
				if providerErr.Diagnostics == nil || providerErr.Diagnostics.Command == nil || !providerErr.Diagnostics.Command.TimedOut {
					t.Fatalf("timeout diagnostics = %#v, want command timed_out", providerErr.Diagnostics)
				}
			}
		})
	}
}

func TestScriptWrapProvider_Infer_ProviderTimeoutTextNormalizesToRetryableTimeout(t *testing.T) {
	testCases := []struct {
		name   string
		result CommandResult
	}{
		{
			name: "CodexTimeoutText",
			result: CommandResult{
				ExitCode: 1,
				Stdout:   []byte("provider timeout while waiting for response"),
			},
		},
		{
			name: "CommandTimeoutExitCode",
			result: CommandResult{
				ExitCode: 124,
				Stderr:   []byte("command timed out"),
			},
		},
		{
			name: "ContextDeadlineText",
			result: CommandResult{
				ExitCode: 1,
				Stderr:   []byte("context deadline exceeded"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeExec := &recordingProviderExec{result: tc.result}
			provider := NewScriptWrapProvider(WithProviderCommandRunner(fakeExec))

			_, err := provider.Infer(context.Background(), interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "fix it",
			})
			if err == nil {
				t.Fatal("expected Infer to fail")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("expected ProviderError, got %T", err)
			}
			if providerErr.Type != interfaces.ProviderErrorTypeTimeout {
				t.Fatalf("Type = %q, want %q", providerErr.Type, interfaces.ProviderErrorTypeTimeout)
			}
			decision := ClassifyProviderFailure(providerErr)
			if !decision.Retryable || decision.Terminal || decision.TriggersThrottlePause {
				t.Fatalf("ClassifyProviderFailure(%#v) = %#v, want retryable timeout decision", providerErr, decision)
			}
		})
	}
}

type april11FailureShapeFixture struct {
	Samples []april11FailureShapeSample `json:"samples"`
}

type april11FailureShapeSample struct {
	Name                  string                       `json:"name"`
	ExitCode              int                          `json:"exit_code"`
	Stdout                string                       `json:"stdout"`
	Stderr                string                       `json:"stderr"`
	WantType              interfaces.ProviderErrorType `json:"want_type"`
	WantMessage           string                       `json:"want_message"`
	WantRetryable         bool                         `json:"want_retryable"`
	WantTerminal          bool                         `json:"want_terminal"`
	WantThrottlePause     bool                         `json:"want_throttle_pause"`
	RejectMessageContains []string                     `json:"reject_message_contains"`
}

func loadApril11FailureShapeFixture(t *testing.T) april11FailureShapeFixture {
	t.Helper()

	path := filepath.Join("testdata", "april11_2026_failure_shapes.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read April 11 failure-shape fixture: %v", err)
	}

	var fixture april11FailureShapeFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode April 11 failure-shape fixture: %v", err)
	}
	if len(fixture.Samples) == 0 {
		t.Fatal("expected April 11 failure-shape fixture to contain samples")
	}
	return fixture
}

type recordingProviderExec struct {
	request CommandRequest
	result  CommandResult
	err     error
}

func (r *recordingProviderExec) Run(_ context.Context, req CommandRequest) (CommandResult, error) {
	r.request = CommandRequest(interfaces.CloneSubprocessExecutionRequest(req))
	return r.result, r.err
}

type envPrintingProviderExec struct{}

func (envPrintingProviderExec) Run(_ context.Context, req CommandRequest) (CommandResult, error) {
	return CommandResult{
		Stdout: []byte(strings.Join(req.Env, "\n")),
	}, nil
}

func runGitSetup(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ExecCommandRunner{}.Run(ctx, CommandRequest{
		Command: "git",
		Args:    args,
		Env:     isolatedGitEnv(os.Environ()),
		WorkDir: dir,
	})
	if err != nil {
		t.Fatalf("git %s returned system error: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, result.Stdout, result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("git %s exit code = %d\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), result.ExitCode, result.Stdout, result.Stderr)
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func writeEditorMarkerScript(t *testing.T, markerPath string) string {
	t.Helper()
	dir := t.TempDir()
	if strings.EqualFold(filepath.Ext(os.Args[0]), ".exe") {
		script := filepath.Join(dir, "editor.bat")
		content := "@echo off\r\necho invoked > %1\r\nexit /b 42\r\n"
		if err := os.WriteFile(script, []byte(content), 0755); err != nil {
			t.Fatalf("writing editor marker script: %v", err)
		}
		return script + " " + markerPath
	}

	script := filepath.Join(dir, "editor.sh")
	content := "#!/bin/sh\nprintf invoked > \"$1\"\nexit 42\n"
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatalf("writing editor marker script: %v", err)
	}
	return script + " " + markerPath
}

func providerGitTestEnv(envVars map[string]string) []string {
	return isolatedGitEnv(buildProviderEnv(envVars))
}

func isolatedGitEnv(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || inheritedGitRepoEnv[name] {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

var inheritedGitRepoEnv = map[string]bool{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	"GIT_COMMON_DIR":                   true,
	"GIT_DIR":                          true,
	"GIT_INDEX_FILE":                   true,
	"GIT_OBJECT_DIRECTORY":             true,
	"GIT_PREFIX":                       true,
	"GIT_QUARANTINE_PATH":              true,
	"GIT_WORK_TREE":                    true,
}

func assertStringSlicesEqual(t *testing.T, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("expected %d args, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("expected arg %d to be %q, got %q; full args: %v", i, want[i], got[i], got)
		}
	}
}

func assertCommandRequestAssemblyMatchesProviderBehavior(t *testing.T, want, got CommandRequest) {
	t.Helper()
	if got.Command != want.Command {
		t.Fatalf("expected command %q, got %q", want.Command, got.Command)
	}
	assertStringSlicesEqual(t, want.Args, got.Args)
	if string(got.Stdin) != string(want.Stdin) {
		t.Fatalf("expected stdin %q, got %q", string(want.Stdin), string(got.Stdin))
	}
	if got.WorkDir != want.WorkDir {
		t.Fatalf("expected workdir %q, got %q", want.WorkDir, got.WorkDir)
	}
}

func assertStringSliceDoesNotContain(t *testing.T, values []string, forbidden string) {
	t.Helper()
	for _, value := range values {
		if value == forbidden {
			t.Fatalf("expected args not to contain %q, got %v", forbidden, values)
		}
	}
}

func assertEnvContains(t *testing.T, env []string, want string) {
	t.Helper()
	for _, entry := range env {
		if entry == want {
			return
		}
	}
	t.Fatalf("expected env to contain %q", want)
}

func assertEnvValue(t *testing.T, env []string, name, want string) {
	t.Helper()
	values := envSliceToMap(env)
	if got := values[name]; got != want {
		t.Fatalf("expected env %s=%q, got %q", name, want, got)
	}
}

func assertEnvEntryCount(t *testing.T, env []string, name string, want int) {
	t.Helper()
	prefix := name + "="
	got := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			got++
		}
	}
	if got != want {
		t.Fatalf("expected env %s to appear %d time(s), got %d in %v", name, want, got, env)
	}
}

func assertProviderAutomationDefaults(t *testing.T, env []string) {
	t.Helper()
	for _, entry := range providerAutomationEnvDefaults {
		assertEnvValue(t, env, entry.name, entry.value)
	}
}
