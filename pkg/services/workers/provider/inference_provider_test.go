// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workertaxonomy "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/work"
	cursorpkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/cursor"
)

// --- NewScriptWrapProvider ---

func TestNewScriptWrapProvider_Defaults(t *testing.T) {
	t.Parallel()
	p := NewScriptWrapProviderWithDependencies(false, nil, nil, nil, nil, nil, "", nil, nil)
	if p.SkipPermissions {
		t.Error("expected SkipPermissions to default to false")
	}
}

func TestNewScriptWrapProvider_WithOptions(t *testing.T) {
	t.Parallel()
	p := NewScriptWrapProviderWithDependencies(
		true, nil, nil, nil, nil, nil, "", nil, nil)

	if !p.SkipPermissions {
		t.Error("expected SkipPermissions to be true")
	}
}

func TestBuildProviderEnv_Empty(t *testing.T) {
	t.Parallel()
	env := buildProviderEnv(os.Environ(), nil)
	if len(env) == 0 {
		t.Fatal("expected provider env to include process environment or automation defaults")
	}
	assertProviderAutomationDefaults(t, env)
}

func TestBuildProviderEnv_Merges(t *testing.T) {
	t.Parallel()
	env := buildProviderEnv(os.Environ(), map[string]string{
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
	t.Parallel()
	env := buildProviderEnv(os.Environ(), nil)

	assertProviderAutomationDefaults(t, env)
}

func TestBuildProviderEnv_UsesDeterministicPrecedenceForOverlappingKeys(t *testing.T) {
	t.Setenv("GIT_EDITOR", "vim")
	t.Setenv("GIT_SEQUENCE_EDITOR", "vim")
	t.Setenv("AGENT_FACTORY_PROVIDER_ENV_PRECEDENCE", "process")

	env := buildProviderEnv(os.Environ(), map[string]string{
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

	behavior := providerBehaviorFor(string(modelprovider.ProviderClaude), nil)
	req := behavior.BuildCommandRequest(workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderClaude),
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
	t.Parallel()
	env := buildProviderEnv(os.Environ(), map[string]string{
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
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
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
	t.Parallel()
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("provider output")},
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
		Model:         "gpt-5-codex",
		UserMessage:   "fix it",
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}

	assertProviderAutomationDefaults(t, fakeExec.request.Env)
}

func TestSupportedModelProviders_BuildCommandRequest_UsesCLICommand(t *testing.T) {
	t.Parallel()
	for _, provider := range factorycontracts.SupportedModelProviders() {
		t.Run(string(provider), func(t *testing.T) {
			behavior := providerBehaviorFor(string(provider), logging.NoopLogger{})
			req := workerexecution.ProviderInferenceRequest{
				ModelProvider: string(provider),
				UserMessage:   "run dispatch verification",
			}

			args, err := behavior.BuildArgs(context.Background(), req, false, nil)
			if err != nil {
				t.Fatalf("BuildArgs: %v", err)
			}

			commandReq := behavior.BuildCommandRequest(req, args)
			if commandReq.Command != string(provider) {
				t.Fatalf("command = %q, want %q", commandReq.Command, provider)
			}
		})
	}
}

func TestScriptWrapProvider_Infer_CommandCanObserveAutomationDefaultsInEnvironment(t *testing.T) {
	t.Parallel()
	fakeExec := &envPrintingProviderExec{}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	resp, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderClaude),
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
	t.Parallel()
	if testing.Short() {
		t.Skip("real Git integration")
	}
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

	result, err := testProviderExecRunner(t).Run(ctx, CommandRequest{
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
	t.Parallel()
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("claude output")},
	}
	provider := NewScriptWrapProviderWithDependencies(

		true, nil,
		fakeExec, nil, nil, nil, "", nil, nil)

	req := workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderClaude),
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
	if resp.ProviderSession.Provider != string(modelprovider.ProviderClaude) {
		t.Fatalf("provider session provider = %q, want %q", resp.ProviderSession.Provider, modelprovider.ProviderClaude)
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
	t.Parallel()
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("provider output")},
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)
	want := work.ExecutionMetadata{
		DispatchCreatedTick: 3,
		CurrentTick:         4,
		TraceID:             "trace-1",
		WorkIDs:             []string{"work-1", "work-2"},
		ReplayKey:           "transition-1/trace-1/work-1/work-2",
	}
	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderClaude),
		Model:         "claude-sonnet-4-5-20250514",
		UserMessage:   "fix it",
		Dispatch:      work.WorkDispatch{Execution: want},
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}

	assertExecutionMetadataEqual(t, want, fakeExec.request.Execution)
}

func TestScriptWrapProvider_Infer_LogsSafePreparedInvocationBeforeExecution(t *testing.T) {
	t.Parallel()
	const prompt = "synthetic prompt secret"
	sequence := []string{}
	logger := &preparedInvocationTestLogger{sequence: &sequence}
	runner := &preparedInvocationTestRunner{sequence: &sequence}
	provider := NewScriptWrapProviderWithDependencies(false, logger, runner, nil, nil, nil, "", nil, nil)

	ctx, cancel := context.WithDeadline(context.Background(), time.Date(2026, 7, 10, 12, 0, 0, 0, time.FixedZone("test", -7*60*60)))
	defer cancel()
	req := workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
		UserMessage:   prompt,
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-1",
			Execution: work.ExecutionMetadata{
				RequestID: "request-1", TraceID: "trace-1", WorkIDs: []string{"work-1", "work-2"},
			},
		},
	}

	if _, err := provider.Infer(ctx, req); err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if len(sequence) < 2 || sequence[0] != ProviderInvocationPrepared || sequence[1] != "runner" {
		t.Fatalf("sequence = %#v, want prepared record before runner", sequence)
	}
	if logger.preparedCount != 1 {
		t.Fatalf("prepared records = %d, want 1", logger.preparedCount)
	}
	assertPreparedInvocationFields(t, logger.preparedFields, prompt)
	if strings.Contains(logger.allValues, prompt) {
		t.Fatalf("logs contain raw prompt: %s", logger.allValues)
	}
}

func assertPreparedInvocationFields(t *testing.T, fields map[string]any, prompt string) {
	t.Helper()
	if fields["provider"] != "codex" || fields["model"] != ProviderDefaultModel {
		t.Fatalf("provider/model = %#v/%#v", fields["provider"], fields["model"])
	}
	if fields["request_id"] != "request-1" || fields["trace_id"] != "trace-1" || fields["work_id"] != "work-1" || fields["dispatch_id"] != "dispatch-1" {
		t.Fatalf("correlation fields = %#v", fields)
	}
	digest := sha256.Sum256([]byte(prompt))
	if fields["stdin_bytes"] != len(prompt) || fields["stdin_sha256"] != hex.EncodeToString(digest[:]) {
		t.Fatalf("stdin metadata = %#v", fields)
	}
	if fields["deadline"] != "2026-07-10T19:00:00Z" {
		t.Fatalf("deadline = %#v, want UTC deadline", fields["deadline"])
	}
}

func TestSanitizeProviderArgs_RedactsPromptCredentialsAndFreeFormValues(t *testing.T) {
	t.Parallel()
	args := []string{"-p", "--system-prompt", "system secret", "--model", "safe-model", "--resume", "session-secret", "user secret"}
	want := []string{"-p", "--system-prompt", RedactedProviderArgValue, "--model", "safe-model", "--resume", RedactedProviderArgValue, RedactedProviderPrompt}
	got := sanitizeProviderArgs(string(modelprovider.ProviderClaude), args)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("sanitizeProviderArgs() = %#v, want %#v", got, want)
	}
	if sha256Hex([]byte("same")) != sha256Hex([]byte("same")) || sha256Hex([]byte("same")) == sha256Hex([]byte("different")) {
		t.Fatal("stdin digest is not deterministic and input-sensitive")
	}
}

type preparedInvocationTestRunner struct {
	sequence *[]string
	result   CommandResult
	err      error
}

func (r *preparedInvocationTestRunner) Run(_ context.Context, _ CommandRequest) (CommandResult, error) {
	*r.sequence = append(*r.sequence, "runner")
	if r.result.ExitCode == 0 && len(r.result.Stdout) == 0 && len(r.result.Stderr) == 0 {
		r.result = CommandResult{Stdout: []byte("ok")}
	}
	return r.result, r.err
}

type preparedInvocationTestLogger struct {
	sequence       *[]string
	preparedCount  int
	preparedFields map[string]any
	failureCount   int
	failureFields  map[string]any
	allValues      string
}

func (l *preparedInvocationTestLogger) capture(keysAndValues ...any) {
	l.allValues += strings.TrimSpace(strings.Join(anyValues(keysAndValues), " "))
}
func (l *preparedInvocationTestLogger) Debug(_ string, fields ...any) { l.capture(fields...) }
func (l *preparedInvocationTestLogger) Warn(_ string, fields ...any)  { l.capture(fields...) }
func (l *preparedInvocationTestLogger) Error(_ string, fields ...any) {
	l.capture(fields...)
	values := logFieldMap(fields)
	if values["event_name"] == ProviderFailureNormalized {
		l.failureCount++
		l.failureFields = values
		*l.sequence = append(*l.sequence, ProviderFailureNormalized)
	}
}
func (l *preparedInvocationTestLogger) Verbose(_ string, fields ...any) { l.capture(fields...) }
func (l *preparedInvocationTestLogger) Info(_ string, fields ...any) {
	l.capture(fields...)
	values := logFieldMap(fields)
	if values["event_name"] == ProviderInvocationPrepared {
		l.preparedCount++
		l.preparedFields = values
		*l.sequence = append(*l.sequence, ProviderInvocationPrepared)
	}
}

func logFieldMap(fields []any) map[string]any {
	values := make(map[string]any, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if ok {
			values[key] = fields[i+1]
		}
	}
	return values
}

func anyValues(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, fmt.Sprint(value))
	}
	return result
}

func TestScriptWrapProvider_Infer_ClaudeWithoutSessionLeavesMetadataNil(t *testing.T) {
	t.Parallel()
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("claude output without session")},
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	resp, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderClaude),
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
	t.Parallel()
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			ExitCode: 1,
			Stderr:   []byte(`API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`),
		},
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderClaude),
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
	if providerErr.ProviderSession.Provider != string(modelprovider.ProviderClaude) {
		t.Fatalf("provider session provider = %q, want %q", providerErr.ProviderSession.Provider, modelprovider.ProviderClaude)
	}
	if providerErr.ProviderSession.ID != "claude-session-123" {
		t.Fatalf("provider session id = %q, want %q", providerErr.ProviderSession.ID, "claude-session-123")
	}
}

func TestScriptWrapProvider_Infer_CodexPayloadUsesExpectedCommandArgsStdinAndEnv(t *testing.T) {
	t.Parallel()
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			Stdout: []byte("codex output"),
			Stderr: []byte("{\"event\":\"session.created\",\"session_id\":\"sess_codex_123\"}"),
		},
	}
	provider := NewScriptWrapProviderWithDependencies(

		true,
		logging.NoopLogger{}, fakeExec, nil, nil, nil, "", nil, nil)

	req := workerexecution.ProviderInferenceRequest{
		ModelProvider:    string(modelprovider.ProviderCodex),
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
	if resp.ProviderSession.Provider != string(modelprovider.ProviderCodex) {
		t.Fatalf("provider session provider = %q, want %q", resp.ProviderSession.Provider, modelprovider.ProviderCodex)
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
	t.Parallel()
	fakeExec := &recordingProviderExec{result: CommandResult{Stdout: []byte("claude output")}}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderClaude),
		Model:         "claude-sonnet",
		UserMessage:   "inspect",
		InputTokens: InputTokens(factoryruntime.RuntimeToken{
			ID: "token-1",
			Color: factoryruntime.RuntimeTokenColor{
				Content: []work.WorkContentPart{
					{Type: work.WorkContentPartTypeText, Text: "caption"},
					{Type: work.WorkContentPartTypeImage, File: "fixtures/mockup.png"},
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
	if providerErr.Type != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, workerexecution.WorkFailureTypePermanentBadRequest)
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

func TestScriptWrapProvider_Infer_NonCodexPayloadUsesExpectedCommandRequestAndNoStdin(t *testing.T) {
	t.Parallel()
	for _, tc := range nonCodexInferencePayloadTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			stdout := []byte(strings.ToLower(tc.name) + " output")
			if tc.req.ModelProvider == string(modelprovider.ProviderCursor) {
				stdout = cursorpkg.SuccessStdoutJSON(strings.ToLower(tc.name)+" output", "cursor-session-from-json")
			}
			fakeExec := &recordingProviderExec{
				result: CommandResult{Stdout: stdout},
			}
			provider := newScriptWrapProviderForTest(t, fakeExec, tc.req.ModelProvider)

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
	req         workerexecution.ProviderInferenceRequest
	wantArgs    []string
	wantWorkDir string
	wantEnv     string
}

func nonCodexInferencePayloadTestCases() []nonCodexInferencePayloadTestCase {
	return []nonCodexInferencePayloadTestCase{
		{
			name: "Cursor",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCursor),
				Model:         "gpt-5",
				SessionID:     "cursor-session-123",
				UserMessage:   "run the tests",
				EnvVars: map[string]string{
					"AGENT_FACTORY_CURSOR_ENV": "enabled",
				},
			},
			wantArgs: []string{"-p", "--model", "gpt-5", "--resume", "cursor-session-123", "--output-format", "stream-json", "--stream-partial-output", "run the tests"},
			wantEnv:  "AGENT_FACTORY_CURSOR_ENV=enabled",
		},
		{
			name: "OpenCode",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider:    string(modelprovider.ProviderOpenCode),
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
	t.Parallel()
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			Stdout: []byte("codex diagnostic output"),
			Stderr: []byte("codex diagnostic stderr"),
		},
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	resp, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider:    string(modelprovider.ProviderCodex),
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
	if diag.Command != string(modelprovider.ProviderCodex) {
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
	t.Parallel()
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("provider output")},
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)
	inputToken := factoryruntime.RuntimeToken{
		ID: "token-1",
		Color: factoryruntime.RuntimeTokenColor{
			WorkID:     "work-1",
			WorkTypeID: "task",
		},
	}

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			WorkerType:      "worker-a",
			WorkstationName: "review",
			InputTokens:     InputTokens(inputToken),
			InputBindings:   map[string][]string{"task": {"token-1"}},
		},
		WorkerType:       "worker-a",
		WorkstationType:  "review",
		ModelProvider:    string(modelprovider.ProviderCodex),
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
	t.Parallel()
	rawSecret := "provider-secret-value"
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("provider diagnostic output")},
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	resp, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderClaude),
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

	fullDiagnostics := withInferenceResponseDiagnostics(workDiagnosticsForInferenceRequest(workerexecution.ProviderInferenceRequest{
		ModelProvider:    string(modelprovider.ProviderClaude),
		Model:            "claude-sonnet-4",
		WorkerType:       workertaxonomy.WorkerTypeModel,
		WorkstationType:  "review",
		WorkingDirectory: "C:\\repo",
	}), resp, 2)
	if fullDiagnostics.Provider.Provider != string(modelprovider.ProviderClaude) {
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
	t.Parallel()
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: []byte("codex output without session")},
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	resp, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
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
	t.Parallel()
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			Stdout:   []byte("partial output"),
			Stderr:   []byte("rate limited"),
			ExitCode: 1,
		},
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderClaude),
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
	if diag.Command != string(modelprovider.ProviderClaude) {
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
}

// portos:func-length-exception owner=agent-factory reason=legacy-codex-provider-error-table review=2026-07-19 removal=split-command-output-fixtures-and-shared-contract-assertions-before-next-provider-normalization-change
