package agy_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter/testkit"
	"github.com/portpowered/infinite-you/pkg/workers/provider/agy"
)

const privatePrompt = "run; rm -rf / | cat"

type stubPTYSession struct {
	launch agypty.ProcessLaunch
	result agypty.SessionResult
}

func (s *stubPTYSession) Run(context.Context) (agypty.SessionResult, error) {
	return s.result, nil
}

func (s *stubPTYSession) Close() error { return nil }

type stubAllocator struct {
	sessions []*stubPTYSession
	result   agypty.SessionResult
}

func TestNewAdapterWithAllocatorRequiresAndUsesPTYEdge(t *testing.T) {
	if _, err := agy.NewAdapterWithAllocator(t.TempDir(), nil); err == nil || !strings.Contains(err.Error(), "PTY allocator is required") {
		t.Fatalf("missing allocator error = %v", err)
	}
	allocator := &stubAllocator{}
	built, err := agy.NewAdapterWithAllocator(t.TempDir(), allocator)
	if err != nil {
		t.Fatalf("NewAdapterWithAllocator() error = %v", err)
	}
	if built.Allocator != allocator {
		t.Fatalf("allocator = %T, want selected %T", built.Allocator, allocator)
	}
}

func (a *stubAllocator) Allocate(_ context.Context, launch agypty.ProcessLaunch, _ agypty.SessionConfig) (agypty.PTYSession, error) {
	session := &stubPTYSession{launch: launch, result: a.result}
	a.sessions = append(a.sessions, session)
	return session, nil
}

func (a *stubAllocator) lastLaunch() agypty.ProcessLaunch {
	if len(a.sessions) == 0 {
		return agypty.ProcessLaunch{}
	}
	return a.sessions[len(a.sessions)-1].launch
}

func TestAdapterFinalOnlyConformance(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &agypty.MockAllocator{}
	providerAdapter := agy.NewAdapter(factoryRoot, agy.WithAllocator(mock), agy.WithExecutable("agy"), agy.WithSessionConfig(agypty.DefaultSessionConfig()))
	testkit.RunFinalOnly(t, testkit.FinalOnlyFixture{
		NewAdapter: func() adapter.Adapter { return providerAdapter },
		Request: workerexecution.ProviderInferenceRequest{
			Model: "gemini-pro", UserMessage: privatePrompt,
		},
		Success: workerprocess.CommandResult{Stdout: []byte("Complete response\n")},
		Failures: []testkit.FinalOnlyFailureCase{
			{Name: "empty final output is normalized", Result: workerprocess.CommandResult{}},
			{Name: "invalid final output is normalized", Result: workerprocess.CommandResult{Stdout: []byte{0xff}}},
		},
		Expected:            testkit.FinalOnlyExpected{Content: "Complete response"},
		ForbiddenDiagnostic: []string{privatePrompt},
	})
}

func TestAdapterBuildCommandUsesTypedArgvWorkspaceAndEnvironment(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	workspace := filepath.Join(factoryRoot, "workspaces", "a")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	executable := filepath.Join(factoryRoot, "agy.exe")
	if err := os.WriteFile(executable, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	providerAdapter := agy.NewAdapter(factoryRoot, agy.WithExecutable(executable))
	built, err := providerAdapter.BuildCommand(context.Background(), adapter.CommandContext{
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch:         work.WorkDispatch{DispatchID: "dispatch-agy"},
			Model:            "gemini-pro",
			SessionID:        "session-1",
			WorkingDirectory: filepath.Join("workspaces", "a"),
			UserMessage:      privatePrompt,
			EnvVars:          map[string]string{"AGY_TOKEN": "secret"},
		},
	})
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	wantArgs := []string{"chat", "--headless", "--model", "gemini-pro", "--session", "session-1", privatePrompt}
	if built.Request.Command != executable || !reflect.DeepEqual(built.Request.Args, wantArgs) {
		t.Fatalf("command = %q %#v, want %q %#v", built.Request.Command, built.Request.Args, executable, wantArgs)
	}
	if built.Request.WorkDir != workspace {
		t.Fatalf("work dir = %q, want %q", built.Request.WorkDir, workspace)
	}
	if !containsEnv(built.Request.Env, "AGY_TOKEN", "secret") {
		t.Fatalf("env = %#v, want provider override", built.Request.Env)
	}
	if !containsEnv(built.Request.Env, "GIT_TERMINAL_PROMPT", "0") {
		t.Fatalf("env = %#v, want automation defaults", built.Request.Env)
	}
}

func TestAdapterExecutePreservesPromptMetacharactersInArgv(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: "Hello from Agy"}}
	providerAdapter := agy.NewAdapter(factoryRoot, agy.WithAllocator(mock), agy.WithExecutable("agy"), agy.WithSessionConfig(agypty.DefaultSessionConfig()))
	registry, err := adapter.NewRegistry(providerAdapter)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runner, err := providerAdapter.PTYRunner()
	if err != nil {
		t.Fatalf("PTYRunner() error = %v", err)
	}
	request := workerexecution.ProviderInferenceRequest{
		Dispatch:         work.WorkDispatch{DispatchID: "dispatch-agy-42"},
		WorkingDirectory: ".",
		UserMessage:      privatePrompt,
	}
	result, err := adapter.Execute(context.Background(), registry, runner, adapter.ExecuteInput{
		Provider: providerAdapter.Identity(),
		Command:  adapter.CommandContext{Request: request},
		Decoder:  adapter.DecoderContext{RunID: "run-agy-42", DispatchID: "dispatch-agy-42"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertAdapterMetacharacterExecuteResult(t, result, mock, privatePrompt)
}

func assertAdapterMetacharacterExecuteResult(
	t *testing.T,
	result adapter.ExecuteResult,
	mock *stubAllocator,
	wantPrompt string,
) {
	t.Helper()
	if result.Response.Content != "Hello from Agy" {
		t.Fatalf("response content = %q, want cleaned final text", result.Response.Content)
	}
	if !result.Capabilities.FinalOnly || result.Capabilities.NativeStreaming {
		t.Fatalf("capabilities = %#v", result.Capabilities)
	}
	if len(result.Drafts) != 3 {
		t.Fatalf("drafts = %#v, want run start, message completion, run completion", result.Drafts)
	}
	message := result.Drafts[1]
	if message.Kind != responseevents.KindMessage || message.Phase != responseevents.PhaseCompleted {
		t.Fatalf("message draft = %#v", message)
	}
	if message.Provenance.Fidelity != responseevents.FidelityFinalOnly {
		t.Fatalf("message provenance = %#v", message.Provenance)
	}
	if len(mock.sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(mock.sessions))
	}
	launch := mock.lastLaunch()
	if launch.Executable != "agy" || launch.Argv[0] != "agy" {
		t.Fatalf("launch = %#v", launch)
	}
	if err := agypty.ValidateArgv(launch.Argv); err != nil {
		t.Fatalf("ValidateArgv() error = %v", err)
	}
	if got := launch.Argv[len(launch.Argv)-1]; got != wantPrompt {
		t.Fatalf("prompt argv = %q, want single metacharacter-bearing element %q", got, wantPrompt)
	}
}

func containsEnv(env []string, name, want string) bool {
	prefix := name + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) && strings.TrimPrefix(entry, prefix) == want {
			return true
		}
	}
	return false
}
