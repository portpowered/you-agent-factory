package gemini_test

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	geminipkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/gemini"
)

const privateGeminiToken = "sk-gemini-fixture-secret"

func TestAdapterIdentity(t *testing.T) {
	t.Parallel()
	if got := geminipkg.NewAdapter().Identity(); got != adapter.Identity(modelprovider.ProviderGemini) {
		t.Fatalf("Identity() = %q, want %q", got, modelprovider.ProviderGemini)
	}
}

func TestBuildArgs_BasicPromptAndSkipPermissions(t *testing.T) {
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
			want: []string{
				"--prompt", "run the tests",
				"--model", "gemini-2.5-flash",
				"--approval-mode", "yolo",
				"--sandbox", "false",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args, err := geminipkg.BuildArgs(tc.req, tc.skipPermissions)
			if err != nil {
				t.Fatalf("BuildArgs() error = %v", err)
			}
			if !reflect.DeepEqual(args, tc.want) {
				t.Fatalf("BuildArgs() = %#v, want %#v", args, tc.want)
			}
		})
	}
}

func TestBuildArgs_RejectsUnsupportedOptionalCapabilities(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		req     workerexecution.ProviderInferenceRequest
		wantErr string
	}{
		{
			name: "StructuredOutput",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderGemini),
				UserMessage:   "summarize the workspace",
				RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{
					workerexecution.RunnerOptionalCapabilityStructuredOutput,
				},
			},
			wantErr: "structured output is not supported by the gemini runner in v1",
		},
		{
			name: "SessionID",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderGemini),
				UserMessage:   "summarize the workspace",
				SessionID:     "gemini-session-1",
			},
			wantErr: "session resume is not supported by the gemini runner in v1",
		},
		{
			name: "ImageInput",
			req: workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderGemini),
				UserMessage:   "describe the image",
				RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{
					workerexecution.RunnerOptionalCapabilityImageInput,
				},
			},
			wantErr: "image input is not supported by the gemini runner in v1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := geminipkg.BuildArgs(tc.req, false)
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("BuildArgs() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestBuildCommand_WiresArgvAndEnvironment(t *testing.T) {
	t.Parallel()

	built, err := geminipkg.NewAdapter().BuildCommand(context.Background(), adapter.CommandContext{
		SkipPermissions: true,
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch:         work.WorkDispatch{DispatchID: "dispatch-gemini-cmd"},
			ModelProvider:    string(modelprovider.ProviderGemini),
			Model:            "gemini-2.5-flash",
			UserMessage:      "review the workspace",
			WorkingDirectory: "workspace",
			WorkerType:       "agent-worker",
			WorkstationType:  "review-work",
			ProjectID:        "project-gemini",
			InputTokens:      []any{"input-token"},
			EnvVars: map[string]string{
				"GEMINI_API_KEY":       privateGeminiToken,
				"GEMINI_TEST_BOUNDARY": "value",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}

	wantArgs := []string{
		"--prompt", "review the workspace",
		"--model", "gemini-2.5-flash",
		"--approval-mode", "yolo",
		"--sandbox", "false",
	}
	if !reflect.DeepEqual(built.Request.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", built.Request.Args, wantArgs)
	}
	if built.Request.Command != string(modelprovider.ProviderGemini) {
		t.Fatalf("command = %q, want %q", built.Request.Command, modelprovider.ProviderGemini)
	}
	if built.Request.DispatchID != "dispatch-gemini-cmd" ||
		built.Request.WorkerType != "agent-worker" ||
		built.Request.WorkstationName != "review-work" ||
		built.Request.ProjectID != "project-gemini" ||
		built.Request.WorkDir != "workspace" ||
		!reflect.DeepEqual(built.Request.InputTokens, []any{"input-token"}) {
		t.Fatalf("execution context = %#v", built.Request)
	}
	for _, arg := range built.Request.Args {
		if strings.Contains(arg, privateGeminiToken) {
			t.Fatalf("secret leaked into argv: %#v", built.Request.Args)
		}
	}
	if !slices.Contains(built.Request.Env, "GEMINI_API_KEY="+privateGeminiToken) {
		t.Fatalf("auth env missing from command env: %#v", built.Request.Env)
	}
	if !slices.Contains(built.Request.Env, "GEMINI_TEST_BOUNDARY=value") {
		t.Fatalf("explicit env missing from command env: %#v", built.Request.Env)
	}
	if !slices.Contains(built.Request.Env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("automation env missing from command env: %#v", built.Request.Env)
	}
}

func TestBuildCommandRequest_OwnsEnvironmentHandling(t *testing.T) {
	t.Parallel()

	req := workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-gemini-env"},
		ModelProvider: string(modelprovider.ProviderGemini),
		UserMessage:   "plan the change",
		EnvVars: map[string]string{
			"GEMINI_API_KEY": privateGeminiToken,
		},
	}
	command := geminipkg.BuildCommandRequest(req, []string{"--prompt", "plan the change"})
	if command.Command != string(modelprovider.ProviderGemini) {
		t.Fatalf("command = %q, want gemini", command.Command)
	}
	if !reflect.DeepEqual(command.Args, []string{"--prompt", "plan the change"}) {
		t.Fatalf("args = %#v", command.Args)
	}
	if !slices.Contains(command.Env, "GEMINI_API_KEY="+privateGeminiToken) {
		t.Fatalf("env missing provider secret: %#v", command.Env)
	}
	if !slices.Contains(command.Env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("env missing automation default: %#v", command.Env)
	}
}
