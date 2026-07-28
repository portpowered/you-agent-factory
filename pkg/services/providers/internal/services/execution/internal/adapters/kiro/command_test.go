package kiro_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	kiro "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/kiro"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const privateKiroToken = "kiro-fixture-secret"

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

func TestCommandEffectBuildsKiroArgvEnvironmentAndResume(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	effect := kiro.NewCommandEffect(runner)
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        providers.IDKiro,
		AttemptID:       "dispatch-kiro-cmd",
		SystemPrompt:    "You are a careful reviewer.",
		UserMessage:     "review the workspace",
		SkipPermissions: true,
		WorkingDirectory: "workspace",
		WorkerType:      "agent-worker",
		WorkstationName: "review-work",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDKiro,
			Kind:     providers.SessionIDKind,
			ID:       kiroResumedSession,
		},
		EnvVars: map[string]string{
			"KIRO_API_TOKEN": privateKiroToken,
			"KIRO_TEST_ENV":  "configured",
		},
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	wantArgs := []string{
		"chat", "--no-interactive",
		"--resume-id", kiroResumedSession,
		"--trust-all-tools",
		"System instructions:\nYou are a careful reviewer.\n\nUser request:\nreview the workspace",
	}
	if !reflect.DeepEqual(runner.request.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", runner.request.Args, wantArgs)
	}
	if runner.request.Command != string(providers.IDKiro) {
		t.Fatalf("command = %q, want %q", runner.request.Command, providers.IDKiro)
	}
	if runner.request.DispatchID != "dispatch-kiro-cmd" ||
		runner.request.WorkerType != "agent-worker" ||
		runner.request.WorkstationName != "review-work" ||
		runner.request.WorkDir != "workspace" {
		t.Fatalf("execution context = %#v", runner.request)
	}
	for _, arg := range runner.request.Args {
		if strings.Contains(arg, privateKiroToken) {
			t.Fatalf("secret leaked into argv: %#v", runner.request.Args)
		}
	}
	if !slices.Contains(runner.request.Env, "KIRO_API_TOKEN="+privateKiroToken) {
		t.Fatalf("auth env missing from command env: %#v", runner.request.Env)
	}
	if !slices.Contains(runner.request.Env, "KIRO_TEST_ENV=configured") {
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
				Provider:     providers.IDKiro,
				AttemptID:    "attempt-1",
				UserMessage:  "summarize",
				OutputSchema: `{"type":"string"}`,
			},
			wantErr: "structured output is not supported by the kiro runner in v1",
		},
		{
			name: "Worktree",
			request: providers.ExecuteRequest{
				Provider:    providers.IDKiro,
				AttemptID:   "attempt-2",
				UserMessage: "summarize",
				Worktree:    "tree",
			},
			wantErr: "worktree selection is not supported by the kiro runner in v1",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			effect := kiro.NewCommandEffect(&recordingRunner{})
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
