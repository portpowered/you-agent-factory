package provider

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

func TestClaudeProviderBehavior_BuildArgs(t *testing.T) {
	testCases := []struct {
		name            string
		req             interfaces.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "BasicPrompt",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderClaude),
				UserMessage:   "hello",
			},
			want: []string{"-p", "hello"},
		},
		{
			name: "WithSkipPermissions",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderClaude),
				UserMessage:   "hello",
			},
			skipPermissions: true,
			want:            []string{"-p", "--dangerously-skip-permissions", "hello"},
		},
		{
			name: "WithSystemPromptAndModel",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderClaude),
				UserMessage:   "do stuff",
				SystemPrompt:  "You are helpful",
				Model:         "claude-sonnet-4-5-20250514",
			},
			want: []string{"-p", "--system-prompt", "You are helpful", "--model", "claude-sonnet-4-5-20250514", "do stuff"},
		},
		{
			name: "WithResumeSessionID",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderClaude),
				UserMessage:   "do stuff",
				SessionID:     "claude-session-123",
			},
			want: []string{"-p", "--resume", "claude-session-123", "do stuff"},
		},
	}

	behavior := claudeProviderBehavior{logger: logging.NoopLogger{}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := behavior.BuildArgs(tc.req, tc.skipPermissions)
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			assertStringSlicesEqual(t, tc.want, args)
		})
	}
}

func TestCodexProviderBehavior_BuildArgs(t *testing.T) {
	testCases := []struct {
		name            string
		req             interfaces.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "BasicPrompt",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				UserMessage:   "fix the bug",
			},
			want: []string{"exec", "-"},
		},
		{
			name: "WithSkipPermissionsAndModel",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCodex),
				Model:         "gpt-5-codex",
				UserMessage:   "hello",
			},
			skipPermissions: true,
			want:            []string{"exec", "--dangerously-bypass-approvals-and-sandbox", "--model", "gpt-5-codex", "-"},
		},
		{
			name: "WithWorkingDirectoryRetainsStdinPlaceholderOnly",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider:    string(ModelProviderCodex),
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
			args, err := behavior.BuildArgs(tc.req, tc.skipPermissions)
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			assertStringSlicesEqual(t, tc.want, args)
		})
	}
}

func TestCodexProviderBehavior_BuildArgs_RejectsUnsupportedOptionalCapabilities(t *testing.T) {
	behavior := codexProviderBehavior{logger: logging.NoopLogger{}}
	_, err := behavior.BuildArgs(interfaces.ProviderInferenceRequest{
		ModelProvider: string(ModelProviderCodex),
		UserMessage:   "summarize the workspace",
		Worktree:      "feature-worktree",
		RequiredOptionalCapabilities: []interfaces.RunnerOptionalCapability{
			interfaces.RunnerOptionalCapabilityWorktree,
		},
	}, false)
	if err == nil || err.Error() != "worktree selection is not supported by the codex runner in v1" {
		t.Fatalf("BuildArgs error = %v, want worktree rejection", err)
	}
}

func TestGeminiProviderBehavior_BuildArgs(t *testing.T) {
	testCases := []struct {
		name            string
		req             interfaces.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "BasicPrompt",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderGemini),
				UserMessage:   "summarize the workspace",
			},
			want: []string{"--prompt", "summarize the workspace"},
		},
		{
			name: "WithModelAndSkipPermissions",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderGemini),
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
			args, err := behavior.BuildArgs(tc.req, tc.skipPermissions)
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			assertStringSlicesEqual(t, tc.want, args)
		})
	}
}

func TestGeminiProviderBehavior_BuildArgs_RejectsUnsupportedOptionalCapabilities(t *testing.T) {
	behavior := geminiProviderBehavior{logger: logging.NoopLogger{}}
	_, err := behavior.BuildArgs(interfaces.ProviderInferenceRequest{
		ModelProvider: string(ModelProviderGemini),
		UserMessage:   "summarize the workspace",
		RequiredOptionalCapabilities: []interfaces.RunnerOptionalCapability{
			interfaces.RunnerOptionalCapabilityStructuredOutput,
		},
	}, false)
	if err == nil || err.Error() != "structured output is not supported by the gemini runner in v1" {
		t.Fatalf("BuildArgs error = %v, want structured output rejection", err)
	}
}

func TestKiroProviderBehavior_BuildArgs(t *testing.T) {
	testCases := []struct {
		name            string
		req             interfaces.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "BasicPrompt",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderKiro),
				UserMessage:   "summarize the workspace",
			},
			want: []string{"chat", "--no-interactive", "summarize the workspace"},
		},
		{
			name: "WithSystemPromptSessionAndTrustedTools",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderKiro),
				SystemPrompt:  "You are a careful reviewer.",
				UserMessage:   "run the tests",
				SessionID:     "kiro-session-123",
			},
			skipPermissions: true,
			want: []string{
				"chat",
				"--no-interactive",
				"--resume-id",
				"kiro-session-123",
				"--trust-all-tools",
				"System instructions:\nYou are a careful reviewer.\n\nUser request:\nrun the tests",
			},
		},
	}

	behavior := kiroProviderBehavior{logger: logging.NoopLogger{}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := behavior.BuildArgs(tc.req, tc.skipPermissions)
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			assertStringSlicesEqual(t, tc.want, args)
		})
	}
}

func TestKiroProviderBehavior_BuildArgs_RejectsUnsupportedOptionalCapabilities(t *testing.T) {
	behavior := kiroProviderBehavior{logger: logging.NoopLogger{}}
	_, err := behavior.BuildArgs(interfaces.ProviderInferenceRequest{
		ModelProvider: string(ModelProviderKiro),
		UserMessage:   "summarize the workspace",
		RequiredOptionalCapabilities: []interfaces.RunnerOptionalCapability{
			interfaces.RunnerOptionalCapabilityStructuredOutput,
		},
	}, false)
	if err == nil || err.Error() != "structured output is not supported by the kiro runner in v1" {
		t.Fatalf("BuildArgs error = %v, want structured output rejection", err)
	}
}

func TestCursorProviderBehavior_BuildArgs(t *testing.T) {
	testCases := []struct {
		name            string
		req             interfaces.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "BasicPrompt",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCursor),
				UserMessage:   "summarize the workspace",
			},
			want: []string{"-p", "summarize the workspace"},
		},
		{
			name: "WithModelSessionAndForce",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCursor),
				Model:         "gpt-5",
				SessionID:     "cursor-session-123",
				UserMessage:   "run the tests",
			},
			skipPermissions: true,
			want:            []string{"-f", "-p", "--model", "gpt-5", "--resume", "cursor-session-123", "run the tests"},
		},
	}

	behavior := cursorProviderBehavior{logger: logging.NoopLogger{}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := behavior.BuildArgs(tc.req, tc.skipPermissions)
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			assertStringSlicesEqual(t, tc.want, args)
		})
	}
}

func TestCursorProviderBehavior_BuildArgs_RejectsUnsupportedOptionalCapabilities(t *testing.T) {
	behavior := cursorProviderBehavior{logger: logging.NoopLogger{}}
	_, err := behavior.BuildArgs(interfaces.ProviderInferenceRequest{
		ModelProvider: string(ModelProviderCursor),
		UserMessage:   "summarize the workspace",
		RequiredOptionalCapabilities: []interfaces.RunnerOptionalCapability{
			interfaces.RunnerOptionalCapabilityStructuredOutput,
		},
	}, false)
	if err == nil || err.Error() != "structured output is not supported by the cursor-cli runner in v1" {
		t.Fatalf("BuildArgs error = %v, want structured output rejection", err)
	}
}

func TestOpenCodeProviderBehavior_BuildArgs(t *testing.T) {
	testCases := []struct {
		name            string
		req             interfaces.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "BasicPrompt",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderOpenCode),
				UserMessage:   "summarize the workspace",
			},
			want: []string{"run", "summarize the workspace"},
		},
		{
			name: "WithModelSessionWorkingDirectoryAndSkipPermissions",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider:    string(ModelProviderOpenCode),
				Model:            "openai/gpt-5",
				SessionID:        "opencode-session-123",
				WorkingDirectory: "/tmp/project",
				UserMessage:      "run the tests",
			},
			skipPermissions: true,
			want:            []string{"run", "--model", "openai/gpt-5", "--session", "opencode-session-123", "--dir", "/tmp/project", "--dangerously-skip-permissions", "run the tests"},
		},
	}

	behavior := openCodeProviderBehavior{logger: logging.NoopLogger{}}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := behavior.BuildArgs(tc.req, tc.skipPermissions)
			if err != nil {
				t.Fatalf("BuildArgs returned error: %v", err)
			}
			assertStringSlicesEqual(t, tc.want, args)
		})
	}
}

func TestOpenCodeProviderBehavior_BuildArgs_RejectsUnsupportedOptionalCapabilities(t *testing.T) {
	behavior := openCodeProviderBehavior{logger: logging.NoopLogger{}}
	_, err := behavior.BuildArgs(interfaces.ProviderInferenceRequest{
		ModelProvider: string(ModelProviderOpenCode),
		UserMessage:   "summarize the workspace",
		RequiredOptionalCapabilities: []interfaces.RunnerOptionalCapability{
			interfaces.RunnerOptionalCapabilityStructuredOutput,
		},
	}, false)
	if err == nil || err.Error() != "structured output is not supported by the opencode runner in v1" {
		t.Fatalf("BuildArgs error = %v, want structured output rejection", err)
	}
}

func TestNonCodexProviderBehavior_BuildCommandRequest(t *testing.T) {
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

type nonCodexCommandRequestTestCase struct {
	name    string
	req     interfaces.ProviderInferenceRequest
	args    []string
	wantEnv string
}

func nonCodexCommandRequestTestCases() []nonCodexCommandRequestTestCase {
	token := interfaces.Token{
		ID: "token-1",
		Color: interfaces.TokenColor{
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Text: "hello"},
			},
		},
	}

	return []nonCodexCommandRequestTestCase{
		{
			name: "Claude",
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderClaude),
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
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderGemini),
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
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderKiro),
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
			req: interfaces.ProviderInferenceRequest{
				ModelProvider: string(ModelProviderCursor),
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
			req: interfaces.ProviderInferenceRequest{
				ModelProvider:    string(ModelProviderOpenCode),
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

func TestNonCodexProviderBehavior_FormatTimeoutFailure(t *testing.T) {
	behaviors := map[string]providerBehavior{
		string(ModelProviderClaude):   claudeProviderBehavior{logger: logging.NoopLogger{}},
		string(ModelProviderGemini):   geminiProviderBehavior{logger: logging.NoopLogger{}},
		string(ModelProviderKiro):     kiroProviderBehavior{logger: logging.NoopLogger{}},
		string(ModelProviderCursor):   cursorProviderBehavior{logger: logging.NoopLogger{}},
		string(ModelProviderOpenCode): openCodeProviderBehavior{logger: logging.NoopLogger{}},
	}

	testCases := []struct {
		name   string
		result CommandResult
		want   string
	}{
		{
			name: "PrefersTrimmedStderr",
			result: CommandResult{
				Stderr: []byte("  provider timed out waiting for upstream  \n"),
				Stdout: []byte("stdout fallback"),
			},
			want: "provider timed out waiting for upstream",
		},
		{
			name: "FallsBackToStdout",
			result: CommandResult{
				Stdout: []byte("provider timeout echoed on stdout"),
			},
			want: "provider timeout echoed on stdout",
		},
		{
			name:   "UsesDefaultMessageWhenNoOutputExists",
			result: CommandResult{},
			want:   "execution timeout",
		},
	}

	for providerName, behavior := range behaviors {
		t.Run(providerName, func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					if got := behavior.FormatTimeoutFailure(tc.result); got != tc.want {
						t.Fatalf("FormatTimeoutFailure() = %q, want %q", got, tc.want)
					}
				})
			}
		})
	}
}

func TestCodexProviderBehavior_FormatTimeoutFailure(t *testing.T) {
	behavior := codexProviderBehavior{logger: logging.NoopLogger{}}

	testCases := []struct {
		name   string
		result CommandResult
		want   string
	}{
		{
			name: "PrefersTrimmedStderr",
			result: CommandResult{
				Stderr: []byte("  provider timed out waiting for upstream  \n"),
				Stdout: []byte("stdout fallback"),
			},
			want: "provider timed out waiting for upstream",
		},
		{
			name: "FallsBackToStdout",
			result: CommandResult{
				Stdout: []byte("provider timeout echoed on stdout"),
			},
			want: "provider timeout echoed on stdout",
		},
		{
			name:   "UsesDefaultMessageWhenNoOutputExists",
			result: CommandResult{},
			want:   "execution timeout",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := behavior.FormatTimeoutFailure(tc.result); got != tc.want {
				t.Fatalf("FormatTimeoutFailure() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGenericNonCodexProviderBehavior_ExitFailureBehavior(t *testing.T) {
	for _, providerName := range []string{
		string(ModelProviderGemini),
		string(ModelProviderKiro),
		string(ModelProviderOpenCode),
	} {
		behavior := providerBehaviorForErrorClassification(providerName)
		t.Run(providerName, func(t *testing.T) {
			assertProviderExitFailureFormatting(t, behavior, providerName)
			assertProviderExitFailureClassification(t, behavior)
		})
	}
}

func TestCursorAndCodexProviderBehavior_ExitFailureBehavior(t *testing.T) {
	cursorBehavior := providerBehaviorForErrorClassification(string(ModelProviderCursor))
	codexBehavior := providerBehaviorForErrorClassification(string(ModelProviderCodex))

	assertCodexDerivedExitFailureFormatting(t, cursorBehavior, string(ModelProviderCursor))
	assertCodexDerivedExitFailureFormatting(t, codexBehavior, string(ModelProviderCodex))

	result := CommandResult{
		ExitCode: 1,
		Stderr:   []byte("ERROR: unexpected status 500 from codex upstream"),
	}
	if got := cursorBehavior.ClassifyExitFailure(result); got != interfaces.ProviderErrorTypeInternalServerError {
		t.Fatalf("cursor ClassifyExitFailure() = %q, want %q", got, interfaces.ProviderErrorTypeInternalServerError)
	}
	if got := codexBehavior.ClassifyExitFailure(result); got != interfaces.ProviderErrorTypeInternalServerError {
		t.Fatalf("codex ClassifyExitFailure() = %q, want %q", got, interfaces.ProviderErrorTypeInternalServerError)
	}
}

func assertCodexDerivedExitFailureFormatting(t *testing.T, behavior providerBehavior, providerName string) {
	t.Helper()

	testCases := []struct {
		name   string
		result CommandResult
		want   string
	}{
		{
			name:   "ExtractsCodexErrorLine",
			result: CommandResult{ExitCode: 17, Stderr: []byte("noise before\nERROR: upstream rejected the request")},
			want:   "ERROR: upstream rejected the request",
		},
		{
			name:   "FallsBackToProviderExitCode",
			result: CommandResult{ExitCode: 17, Stderr: []byte("stderr detail"), Stdout: []byte("stdout detail")},
			want:   providerName + " exited with code 17",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := behavior.FormatExitFailure(providerName, tc.result); got != tc.want {
				t.Fatalf("FormatExitFailure() = %q, want %q", got, tc.want)
			}
		})
	}
}

func assertProviderExitFailureFormatting(t *testing.T, behavior providerBehavior, providerName string) {
	t.Helper()

	testCases := []struct {
		name   string
		result CommandResult
		want   string
	}{
		{
			name: "PrefersTrimmedStderr",
			result: CommandResult{
				ExitCode: 17,
				Stderr:   []byte("  stderr detail  \n"),
				Stdout:   []byte("stdout fallback"),
			},
			want: "stderr detail",
		},
		{
			name: "FallsBackToStdout",
			result: CommandResult{
				ExitCode: 17,
				Stdout:   []byte("stdout detail"),
			},
			want: "stdout detail",
		},
		{
			name:   "UsesProviderSpecificFallback",
			result: CommandResult{ExitCode: 17},
			want:   providerName + " exited with code 17",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := behavior.FormatExitFailure(providerName, tc.result); got != tc.want {
				t.Fatalf("FormatExitFailure() = %q, want %q", got, tc.want)
			}
		})
	}
}

func assertProviderExitFailureClassification(t *testing.T, behavior providerBehavior) {
	t.Helper()

	testCases := []struct {
		name   string
		result CommandResult
		want   interfaces.ProviderErrorType
	}{
		{
			name:   "AuthFailure",
			result: CommandResult{ExitCode: 1, Stderr: []byte("login required for this API key")},
			want:   interfaces.ProviderErrorTypeAuthFailure,
		},
		{
			name:   "PermanentBadRequest",
			result: CommandResult{ExitCode: 1, Stderr: []byte("invalid argument in request payload")},
			want:   interfaces.ProviderErrorTypePermanentBadRequest,
		},
		{
			name:   "Throttled",
			result: CommandResult{ExitCode: 1, Stderr: []byte("resource exhausted by 429 quota")},
			want:   interfaces.ProviderErrorTypeThrottled,
		},
		{
			name:   "InternalServerError",
			result: CommandResult{ExitCode: 1, Stderr: []byte("unexpected status 503 from upstream")},
			want:   interfaces.ProviderErrorTypeInternalServerError,
		},
		{
			name:   "Timeout",
			result: CommandResult{ExitCode: 124},
			want:   interfaces.ProviderErrorTypeTimeout,
		},
		{
			name:   "Unknown",
			result: CommandResult{ExitCode: 1, Stderr: []byte("provider stopped without classification markers")},
			want:   interfaces.ProviderErrorTypeUnknown,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := behavior.ClassifyExitFailure(tc.result); got != tc.want {
				t.Fatalf("ClassifyExitFailure() = %q, want %q", got, tc.want)
			}
		})
	}
}
