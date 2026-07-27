package kiro_test

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	kiropkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/kiro"
)

const privateKiroToken = "kiro-fixture-secret"

func TestAdapterIdentity(t *testing.T) {
	t.Parallel()
	if got := kiropkg.NewAdapter().Identity(); got != adapter.Identity(modelprovider.ProviderKiro) {
		t.Fatalf("Identity() = %q, want %q", got, modelprovider.ProviderKiro)
	}
}

func TestBuildArgs_PreservesPromptResumeAndPermissionBehavior(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		req             workerexecution.ProviderInferenceRequest
		skipPermissions bool
		want            []string
	}{
		{
			name: "NewInvocation",
			req: workerexecution.ProviderInferenceRequest{
				UserMessage: "summarize the workspace",
			},
			want: []string{"chat", "--no-interactive", "summarize the workspace"},
		},
		{
			name: "ComposedPrompt",
			req: workerexecution.ProviderInferenceRequest{
				SystemPrompt: "  You are a careful reviewer. ",
				UserMessage:  " run the tests  ",
			},
			want: []string{
				"chat",
				"--no-interactive",
				"System instructions:\nYou are a careful reviewer.\n\nUser request:\nrun the tests",
			},
		},
		{
			name: "ResumedInvocation",
			req: workerexecution.ProviderInferenceRequest{
				SessionID:   "kiro-session-123",
				UserMessage: "continue the review",
			},
			want: []string{
				"chat", "--no-interactive",
				"--resume-id", "kiro-session-123",
				"continue the review",
			},
		},
		{
			name: "SkipPermissions",
			req: workerexecution.ProviderInferenceRequest{
				UserMessage: "run the tests",
			},
			skipPermissions: true,
			want:            []string{"chat", "--no-interactive", "--trust-all-tools", "run the tests"},
		},
		{
			name: "SystemInstructionsOnly",
			req: workerexecution.ProviderInferenceRequest{
				SystemPrompt: "Review carefully.",
			},
			want: []string{"chat", "--no-interactive", "Review carefully."},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := kiropkg.BuildArgs(tc.req, tc.skipPermissions)
			if err != nil {
				t.Fatalf("BuildArgs() error = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("BuildArgs() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestBuildArgs_RejectsUnsupportedCapabilityRequirements(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		capability workerexecution.RunnerOptionalCapability
		wantErr    string
	}{
		{
			name:       "ImageInput",
			capability: workerexecution.RunnerOptionalCapabilityImageInput,
			wantErr:    "image input is not supported by the kiro runner in v1",
		},
		{
			name:       "StructuredOutput",
			capability: workerexecution.RunnerOptionalCapabilityStructuredOutput,
			wantErr:    "structured output is not supported by the kiro runner in v1",
		},
		{
			name:       "WorkingDirectory",
			capability: workerexecution.RunnerOptionalCapabilityWorkingDirectory,
			wantErr:    "working directory is not supported by the kiro runner in v1",
		},
		{
			name:       "Worktree",
			capability: workerexecution.RunnerOptionalCapabilityWorktree,
			wantErr:    "worktree selection is not supported by the kiro runner in v1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := kiropkg.BuildArgs(workerexecution.ProviderInferenceRequest{
				UserMessage:                  "summarize the workspace",
				RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{tc.capability},
			}, false)
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("BuildArgs() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestBuildCommand_WiresEnvironmentAndDispatchMetadata(t *testing.T) {
	t.Parallel()

	built, err := kiropkg.NewAdapter().BuildCommand(context.Background(), adapter.CommandContext{
		SkipPermissions: true,
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch:         work.WorkDispatch{DispatchID: "dispatch-kiro-command"},
			UserMessage:      "review the workspace",
			WorkingDirectory: "workspace",
			WorkerType:       "agent-worker",
			WorkstationType:  "review-work",
			ProjectID:        "project-kiro",
			InputTokens:      []any{"input-token"},
			ProcessEnvironment: []string{
				"KIRO_PROCESS_VALUE=base",
			},
			EnvVars: map[string]string{
				"KIRO_API_TOKEN": privateKiroToken,
				"KIRO_TEST_ENV":  "configured",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}

	if built.Request.Command != string(modelprovider.ProviderKiro) {
		t.Fatalf("command = %q, want %q", built.Request.Command, modelprovider.ProviderKiro)
	}
	wantArgs := []string{"chat", "--no-interactive", "--trust-all-tools", "review the workspace"}
	if !reflect.DeepEqual(built.Request.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", built.Request.Args, wantArgs)
	}
	if built.Request.DispatchID != "dispatch-kiro-command" ||
		built.Request.WorkerType != "agent-worker" ||
		built.Request.WorkstationName != "review-work" ||
		built.Request.ProjectID != "project-kiro" ||
		built.Request.WorkDir != "workspace" ||
		!reflect.DeepEqual(built.Request.InputTokens, []any{"input-token"}) {
		t.Fatalf("execution context = %#v", built.Request)
	}
	for _, arg := range built.Request.Args {
		if strings.Contains(arg, privateKiroToken) {
			t.Fatalf("secret leaked into argv: %#v", built.Request.Args)
		}
	}
	for _, want := range []string{
		"KIRO_PROCESS_VALUE=base",
		"KIRO_API_TOKEN=" + privateKiroToken,
		"KIRO_TEST_ENV=configured",
		"GIT_TERMINAL_PROMPT=0",
	} {
		if !slices.Contains(built.Request.Env, want) {
			t.Fatalf("command env = %#v, want %q", built.Request.Env, want)
		}
	}
}
