// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

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

func TestProviderBehaviorBuildArgsRejectsUnsupportedReasoningEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		behavior providerBehavior
		provider string
		effort   string
		want     string
	}{
		{
			name:     "Claude rejects provider-unsupported minimal",
			behavior: claudeProviderBehavior{logger: logging.NoopLogger{}},
			provider: string(modelprovider.ProviderClaude),
			effort:   " MINIMAL ",
			want:     `does not support reasoning effort "minimal"`,
		},
		{
			name:     "Claude rejects globally unsupported value",
			behavior: claudeProviderBehavior{logger: logging.NoopLogger{}},
			provider: string(modelprovider.ProviderClaude),
			effort:   "extreme",
			want:     `unsupported reasoning effort "extreme"`,
		},
		{
			name:     "Codex rejects globally unsupported value",
			behavior: codexProviderBehavior{logger: logging.NoopLogger{}},
			provider: string(modelprovider.ProviderCodex),
			effort:   "extreme",
			want:     `unsupported reasoning effort "extreme"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.behavior.BuildArgs(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider:   test.provider,
				ReasoningEffort: test.effort,
				UserMessage:     "hello",
			}, false, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("BuildArgs() error = %v, want %q", err, test.want)
			}
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
			name: "WithLunaXHighReasoningEffort",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider:   string(modelprovider.ProviderCodex),
				Model:           "gpt-5.6-luna",
				ReasoningEffort: " XHIGH ",
				UserMessage:     "hello",
			},
			want: []string{"exec", "--model", "gpt-5.6-luna", "--config", `model_reasoning_effort="xhigh"`, "-"},
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

// Migrated Gemini must no longer own aggregate command/failure/timeout branches.
// Production selection stays on registry + conductor; these assertions prove the
// legacy Gemini-named aggregate ownership is gone without introducing a new
// concrete Gemini switch in shared orchestration.
func TestAggregateSurfacesOmitMigratedGeminiBranches(t *testing.T) {
	t.Parallel()

	t.Run("command_construction", func(t *testing.T) {
		t.Parallel()
		behavior := providerBehaviorFor(string(modelprovider.ProviderGemini), logging.NoopLogger{})
		args, err := behavior.BuildArgs(context.Background(), workerexecution.ProviderInferenceRequest{
			ModelProvider: string(modelprovider.ProviderGemini),
			UserMessage:   "summarize the workspace",
		}, false, nil)
		if err != nil {
			t.Fatalf("BuildArgs error = %v", err)
		}
		if len(args) > 0 && args[0] == "--prompt" {
			t.Fatalf("aggregate still owns Gemini argv: %#v", args)
		}
		command := behavior.BuildCommandRequest(workerexecution.ProviderInferenceRequest{
			ModelProvider: string(modelprovider.ProviderGemini),
			UserMessage:   "summarize the workspace",
		}, args)
		if command.Command == string(modelprovider.ProviderGemini) && containsArgPair(command.Args, "--prompt", "summarize the workspace") {
			t.Fatalf("aggregate still owns Gemini command request: %#v", command)
		}
	})

	t.Run("exit_failure", func(t *testing.T) {
		t.Parallel()
		parsed := parseProviderExitFailure(string(modelprovider.ProviderGemini), CommandResult{
			ExitCode: 1,
			Stderr:   []byte(`{"error":{"status":"UNAUTHENTICATED"}}`),
		})
		if parsed.failure.Message == "Gemini authentication failed." {
			t.Fatal("aggregate still owns Gemini exit-failure parsing")
		}
	})

	t.Run("timeout_failure", func(t *testing.T) {
		t.Parallel()
		parsed := parseProviderTimeoutFailure(string(modelprovider.ProviderGemini), CommandResult{})
		if parsed.Message == "Gemini request timed out." {
			t.Fatal("aggregate still owns Gemini timeout parsing")
		}
	})
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}
