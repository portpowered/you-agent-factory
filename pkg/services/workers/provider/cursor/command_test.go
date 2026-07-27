package cursor_test

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	cursorpkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/cursor"
)

const privateCursorToken = "cursor-fixture-secret"

func TestAdapterIdentity(t *testing.T) {
	t.Parallel()
	if got := cursorpkg.NewAdapter().Identity(); got != adapter.Identity("cursor") {
		t.Fatalf("Identity() = %q, want cursor", got)
	}
}

func TestBuildArgsPreservesInvocationBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		request         workerexecution.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name:    "normal",
			request: workerexecution.ProviderInferenceRequest{UserMessage: "summarize the workspace"},
			want: []string{
				"-p",
				"--output-format", cursorpkg.CursorOutputFormatStreamJSON,
				"--stream-partial-output",
				"summarize the workspace",
			},
		},
		{
			name: "combined prompt",
			request: workerexecution.ProviderInferenceRequest{
				SystemPrompt: "  You are a careful reviewer. ",
				UserMessage:  " run the tests  ",
			},
			want: []string{
				"-p",
				"--output-format", cursorpkg.CursorOutputFormatStreamJSON,
				"--stream-partial-output",
				"System instructions:\nYou are a careful reviewer.\n\nUser request:\nrun the tests",
			},
		},
		{
			name: "resume model and permissions",
			request: workerexecution.ProviderInferenceRequest{
				Model:       "cursor-model",
				SessionID:   "cursor-session-123",
				UserMessage: "continue the review",
			},
			skipPermissions: true,
			want: []string{
				"-f", "-p",
				"--model", "cursor-model",
				"--resume", "cursor-session-123",
				"--output-format", cursorpkg.CursorOutputFormatStreamJSON,
				"--stream-partial-output",
				"continue the review",
			},
		},
		{
			name: "workspace and worktree arguments",
			request: workerexecution.ProviderInferenceRequest{
				WorkingDirectory: "C:/factory/workspace",
				Worktree:         "feature/cursor-migration",
				UserMessage:      "inspect the checkout",
			},
			want: []string{
				"-p",
				"--workspace", "C:/factory/workspace",
				"--worktree", "feature/cursor-migration",
				"--output-format", cursorpkg.CursorOutputFormatStreamJSON,
				"--stream-partial-output",
				"inspect the checkout",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := cursorpkg.BuildArgs(test.request, test.skipPermissions)
			if err != nil {
				t.Fatalf("BuildArgs() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("BuildArgs() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestBuildArgsRejectsUnsupportedCapabilityRequirements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		capability workerexecution.RunnerOptionalCapability
		wantError  string
	}{
		{
			capability: workerexecution.RunnerOptionalCapabilityImageInput,
			wantError:  "image input is not supported by the cursor-cli runner in v1",
		},
		{
			capability: workerexecution.RunnerOptionalCapabilityStructuredOutput,
			wantError:  "structured output is not supported by the cursor-cli runner in v1",
		},
		{
			capability: workerexecution.RunnerOptionalCapabilityWorktree,
			wantError:  "worktree selection is not supported by the cursor-cli runner in v1",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(string(test.capability), func(t *testing.T) {
			t.Parallel()
			_, err := cursorpkg.BuildArgs(workerexecution.ProviderInferenceRequest{
				UserMessage:                  "summarize the workspace",
				RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{test.capability},
			}, false)
			if err == nil || err.Error() != test.wantError {
				t.Fatalf("BuildArgs() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestBuildArgsAcceptsManifestSupportedCapabilityRequirements(t *testing.T) {
	t.Parallel()

	_, err := cursorpkg.BuildArgs(workerexecution.ProviderInferenceRequest{
		SessionID:   "cursor-session-123",
		UserMessage: "continue",
		RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{
			workerexecution.RunnerOptionalCapabilitySessionResume,
			workerexecution.RunnerOptionalCapabilityWorkingDirectory,
		},
	}, false)
	if err != nil {
		t.Fatalf("BuildArgs() error = %v", err)
	}
}

func TestBuildCommandWiresAgentEnvironmentAndDispatchMetadata(t *testing.T) {
	t.Parallel()

	built, err := cursorpkg.NewAdapter().BuildCommand(context.Background(), adapter.CommandContext{
		SkipPermissions: true,
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch:         work.WorkDispatch{DispatchID: "dispatch-cursor-command"},
			UserMessage:      "review the workspace",
			WorkingDirectory: "workspace",
			WorkerType:       "agent-worker",
			WorkstationType:  "review-work",
			ProjectID:        "project-cursor",
			InputTokens:      []any{"input-token"},
			ProcessEnvironment: []string{
				"CURSOR_PROCESS_VALUE=base",
			},
			EnvVars: map[string]string{
				"CURSOR_API_TOKEN": privateCursorToken,
				"CURSOR_TEST_ENV":  "configured",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}

	if built.Request.Command != "agent" {
		t.Fatalf("command = %q, want agent", built.Request.Command)
	}
	wantArgs := []string{
		"-f", "-p",
		"--workspace", "workspace",
		"--output-format", cursorpkg.CursorOutputFormatStreamJSON,
		"--stream-partial-output",
		"review the workspace",
	}
	if !reflect.DeepEqual(built.Request.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", built.Request.Args, wantArgs)
	}
	if built.Request.DispatchID != "dispatch-cursor-command" ||
		built.Request.WorkerType != "agent-worker" ||
		built.Request.WorkstationName != "review-work" ||
		built.Request.ProjectID != "project-cursor" ||
		built.Request.WorkDir != "workspace" ||
		!reflect.DeepEqual(built.Request.InputTokens, []any{"input-token"}) {
		t.Fatalf("execution context = %#v", built.Request)
	}
	for _, arg := range built.Request.Args {
		if strings.Contains(arg, privateCursorToken) {
			t.Fatalf("secret leaked into argv: %#v", built.Request.Args)
		}
	}
	for _, want := range []string{
		"CURSOR_PROCESS_VALUE=base",
		"CURSOR_API_TOKEN=" + privateCursorToken,
		"CURSOR_TEST_ENV=configured",
		"GIT_TERMINAL_PROMPT=0",
	} {
		if !slices.Contains(built.Request.Env, want) {
			t.Fatalf("command env = %#v, want %q", built.Request.Env, want)
		}
	}
}
