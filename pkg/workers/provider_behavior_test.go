package workers

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
			want: []string{"--print", "summarize the workspace", "--output-format", "text"},
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
			want:            []string{"--print", "run the tests", "--output-format", "text", "--model", "gpt-5", "--resume", "cursor-session-123", "--force"},
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

func TestClaudeProviderBehavior_FormatTimeoutFailure(t *testing.T) {
	behavior := claudeProviderBehavior{logger: logging.NoopLogger{}}

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
