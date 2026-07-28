package gemini_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	gemini "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/gemini"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const privateGeminiToken = "sk-gemini-fixture-secret"

type recordingRunner struct {
	request workers.CommandRequest
}

func (r *recordingRunner) Run(
	_ context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	r.request = request
	return workers.CommandResult{Stdout: []byte("ok")}, nil
}

func TestCommandEffectBuildsGeminiArgvAndEnvironment(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	effect := gemini.NewCommandEffect(runner)
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:         providers.IDGemini,
		AttemptID:        "dispatch-gemini-cmd",
		Model:            "gemini-2.5-flash",
		UserMessage:      "review the workspace",
		SkipPermissions:  true,
		WorkingDirectory: "workspace",
		WorkerType:       "agent-worker",
		WorkstationName:  "review-work",
		EnvVars: map[string]string{
			"GEMINI_API_KEY":       privateGeminiToken,
			"GEMINI_TEST_BOUNDARY": "value",
		},
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	wantArgs := []string{
		"--prompt", "review the workspace",
		"--model", "gemini-2.5-flash",
		"--approval-mode", "yolo",
		"--sandbox", "false",
	}
	if !reflect.DeepEqual(runner.request.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", runner.request.Args, wantArgs)
	}
	if runner.request.Command != string(providers.IDGemini) {
		t.Fatalf("command = %q, want %q", runner.request.Command, providers.IDGemini)
	}
	if runner.request.DispatchID != "dispatch-gemini-cmd" ||
		runner.request.WorkerType != "agent-worker" ||
		runner.request.WorkstationName != "review-work" ||
		runner.request.WorkDir != "workspace" {
		t.Fatalf("execution context = %#v", runner.request)
	}
	for _, arg := range runner.request.Args {
		if strings.Contains(arg, privateGeminiToken) {
			t.Fatalf("secret leaked into argv: %#v", runner.request.Args)
		}
	}
	if !slices.Contains(runner.request.Env, "GEMINI_API_KEY="+privateGeminiToken) {
		t.Fatalf("auth env missing from command env: %#v", runner.request.Env)
	}
	if !slices.Contains(runner.request.Env, "GEMINI_TEST_BOUNDARY=value") {
		t.Fatalf("explicit env missing from command env: %#v", runner.request.Env)
	}
	if !slices.Contains(runner.request.Env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("automation env missing from command env: %#v", runner.request.Env)
	}
}

func TestCommandEffectRejectsUnsupportedCapabilities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request providers.ExecuteRequest
		wantErr string
	}{
		{
			name: "StructuredOutput",
			request: providers.ExecuteRequest{
				Provider:     providers.IDGemini,
				AttemptID:    "attempt-1",
				UserMessage:  "summarize",
				OutputSchema: `{"type":"string"}`,
			},
			wantErr: "structured output is not supported by the gemini runner in v1",
		},
		{
			name: "SessionResume",
			request: providers.ExecuteRequest{
				Provider:    providers.IDGemini,
				AttemptID:   "attempt-2",
				UserMessage: "summarize",
				ResumeSession: &providers.SessionRef{
					Provider: providers.IDGemini,
					Kind:     providers.SessionIDKind,
					ID:       "gemini-session-1",
				},
			},
			wantErr: "session resume is not supported by the gemini runner in v1",
		},
		{
			name: "Worktree",
			request: providers.ExecuteRequest{
				Provider:    providers.IDGemini,
				AttemptID:   "attempt-3",
				UserMessage: "summarize",
				Worktree:    "tree",
			},
			wantErr: "worktree selection is not supported by the gemini runner in v1",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			effect := gemini.NewCommandEffect(&recordingRunner{})
			_, err := effect.Execute(context.Background(), test.request, func([]byte) error { return nil })
			var attemptFailure execution.AttemptFailure
			if !errors.As(err, &attemptFailure) ||
				attemptFailure.NativeError == nil ||
				attemptFailure.NativeError.Error() != test.wantErr {
				t.Fatalf("Execute() error = %v, want native error %q", err, test.wantErr)
			}
		})
	}
}
