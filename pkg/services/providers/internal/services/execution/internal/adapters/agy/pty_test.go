package agy_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
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

type fakeExecutableLocator map[string]string

func (l fakeExecutableLocator) LookPath(name string) (string, error) {
	if path, ok := l[name]; ok {
		return path, nil
	}
	return "", fs.ErrNotExist
}

type fakeExecutableInspector map[string]fs.FileInfo

func (i fakeExecutableInspector) Stat(path string) (fs.FileInfo, error) {
	if info, ok := i[path]; ok {
		return info, nil
	}
	return nil, fs.ErrNotExist
}

type fakeExecutableInfo struct{ directory bool }

func (i fakeExecutableInfo) Name() string       { return "agy" }
func (i fakeExecutableInfo) Size() int64        { return 0 }
func (i fakeExecutableInfo) Mode() fs.FileMode  { return 0o755 }
func (i fakeExecutableInfo) ModTime() time.Time { return time.Time{} }
func (i fakeExecutableInfo) IsDir() bool        { return i.directory }
func (i fakeExecutableInfo) Sys() any           { return nil }

func executableDependencies(
	locations map[string]string,
	existingPaths ...string,
) agy.ExecutableDependencies {
	locator := fakeExecutableLocator(locations)
	inspector := make(fakeExecutableInspector, len(existingPaths))
	for _, path := range existingPaths {
		inspector[path] = fakeExecutableInfo{}
	}
	return agy.ExecutableDependencies{Locator: locator, Inspector: inspector}
}

func TestPTYEffectRequiresAllocator(t *testing.T) {
	t.Parallel()

	if effect := agy.NewPTYEffect(agy.PTYEffectOptions{FactoryRoot: t.TempDir()}); effect != nil {
		t.Fatal("NewPTYEffect() without allocator = non-nil, want nil")
	}
}

func TestPTYEffectBuildsArgvWorkspaceAndEnvironment(t *testing.T) {
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
	mock := &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: "ok"}}
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot: factoryRoot,
		Allocator:   mock,
		Executable:  executable,
		ExecutableDependencies: executableDependencies(nil, executable),
	})
	if effect == nil {
		t.Fatal("NewPTYEffect() returned nil")
	}
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:         providers.IDAgy,
		AttemptID:        "dispatch-agy",
		Model:            "gemini-pro",
		UserMessage:      privatePrompt,
		WorkingDirectory: filepath.Join("workspaces", "a"),
		WorkerType:       "agent-worker",
		WorkstationName:  "review-work",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDAgy,
			Kind:     providers.SessionIDKind,
			ID:       "session-1",
		},
		EnvVars: map[string]string{"AGY_TOKEN": "secret"},
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	wantArgs := []string{"chat", "--headless", "--model", "gemini-pro", "--session", "session-1", privatePrompt}
	launch := mock.lastLaunch()
	if launch.Executable != executable || !reflect.DeepEqual(launch.Argv[1:], wantArgs) {
		t.Fatalf("launch = %#v, want executable %q argv suffix %#v", launch, executable, wantArgs)
	}
	if launch.WorkDir != workspace {
		t.Fatalf("work dir = %q, want %q", launch.WorkDir, workspace)
	}
	if !containsEnv(launch.Env, "AGY_TOKEN", "secret") {
		t.Fatalf("env = %#v, want provider override", launch.Env)
	}
	if !containsEnv(launch.Env, "GIT_TERMINAL_PROMPT", "0") {
		t.Fatalf("env = %#v, want automation defaults", launch.Env)
	}
	for _, arg := range launch.Argv {
		if strings.Contains(arg, "secret") {
			t.Fatalf("secret leaked into argv: %#v", launch.Argv)
		}
	}
}

func TestPTYEffectExecutesThroughInjectedNativePTY(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: "Agy adapter response"}}
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot:            factoryRoot,
		Allocator:              mock,
		Executable:             "agy",
		ExecutableDependencies: executableDependencies(nil),
	})
	const prompt = "summarize this; preserve argv boundaries"
	var observed []byte
	result, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:         providers.IDAgy,
		AttemptID:        "dispatch-agy-contract",
		Model:            "gemini-pro",
		UserMessage:      prompt,
		WorkingDirectory: ".",
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDAgy,
			Kind:     providers.SessionIDKind,
			ID:       "session-agy-contract",
		},
	}, func(chunk []byte) error {
		observed = append(observed, chunk...)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(observed) != "Agy adapter response" {
		t.Fatalf("observed stdout = %q, want cleaned final text", string(observed))
	}
	if result.SessionRef == nil || result.SessionRef.ID != "session-agy-contract" {
		t.Fatalf("session ref = %#v, want resumed session", result.SessionRef)
	}
	launch := mock.lastLaunch()
	if len(launch.Argv) == 0 || launch.Argv[len(launch.Argv)-1] != prompt {
		t.Fatalf("PTY launch argv = %#v, want prompt preserved as one argument", launch.Argv)
	}
}

func TestPTYEffectPreservesPromptMetacharactersInArgv(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: "Hello from Agy"}}
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot:            factoryRoot,
		Allocator:              mock,
		Executable:             "agy",
		ExecutableDependencies: executableDependencies(nil),
	})
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:         providers.IDAgy,
		AttemptID:        "dispatch-agy-42",
		WorkingDirectory: ".",
		UserMessage:      privatePrompt,
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	launch := mock.lastLaunch()
	if got := launch.Argv[len(launch.Argv)-1]; got != privatePrompt {
		t.Fatalf("prompt argv = %q, want single metacharacter-bearing element %q", got, privatePrompt)
	}
}

func TestPTYEffectTimeoutCleansCaptureBeforeObserve(t *testing.T) {
	t.Parallel()

	raw := []byte("spinning\rpartial answer\x1b[2K\n")
	mock := &timeoutCleaningAllocator{result: agypty.SessionResult{
		ExitCode: 124,
		TimedOut: true,
		RawBytes: raw,
	}}
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot:            t.TempDir(),
		Allocator:              mock,
		Executable:             "agy",
		ExecutableDependencies: executableDependencies(nil),
	})
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDAgy,
		AttemptID:   "dispatch-agy-timeout",
		UserMessage: "hello",
	}, func([]byte) error {
		t.Fatal("observe() called on timeout, want no public stream emit")
		return nil
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want timeout failure")
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindTimeout {
		t.Fatalf("Execute() error = %#v, want timeout failure", err)
	}
}

type timeoutCleaningSession struct {
	result agypty.SessionResult
}

func (s *timeoutCleaningSession) Run(context.Context) (agypty.SessionResult, error) {
	return s.result, nil
}

func (s *timeoutCleaningSession) Close() error { return nil }

type timeoutCleaningAllocator struct {
	result agypty.SessionResult
}

func (a *timeoutCleaningAllocator) Allocate(_ context.Context, _ agypty.ProcessLaunch, _ agypty.SessionConfig) (agypty.PTYSession, error) {
	return &timeoutCleaningSession{result: a.result}, nil
}

func TestPTYEffectFailsClosedWithoutExecutableEffects(t *testing.T) {
	t.Parallel()

	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot: t.TempDir(),
		Allocator:   &stubAllocator{},
		Executable:  "agy",
	})
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDAgy,
		AttemptID:   "dispatch-agy-missing-effects",
		UserMessage: "hello",
	}, func([]byte) error { return nil })
	var attemptFailure execution.AttemptFailure
	if !errors.As(err, &attemptFailure) ||
		attemptFailure.NativeError == nil ||
		!strings.Contains(attemptFailure.NativeError.Error(), "executable locator is required") {
		t.Fatalf("missing locator error = %v", err)
	}
}

func TestPTYEffectResolvesBareExecutableThroughInjectedSearchPath(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	resolved := filepath.Join("toolchain", "agy")
	mock := &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: "ok"}}
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot:            factoryRoot,
		Allocator:              mock,
		Executable:             "agy",
		ExecutableDependencies: executableDependencies(map[string]string{"agy": resolved}, resolved),
	})
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:         providers.IDAgy,
		AttemptID:        "dispatch-agy-path",
		WorkingDirectory: ".",
		UserMessage:      "hello",
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if mock.lastLaunch().Executable != resolved {
		t.Fatalf("executable = %q, want injected search result %q", mock.lastLaunch().Executable, resolved)
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

func TestPTYEffectDispatchContextIsPreservedInLaunch(t *testing.T) {
	t.Parallel()

	mock := &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: "ok"}}
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot:            t.TempDir(),
		Allocator:              mock,
		Executable:             "agy",
		ExecutableDependencies: executableDependencies(nil),
	})
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        providers.IDAgy,
		AttemptID:       "dispatch-agy-context",
		WorkerType:      "agent-worker",
		WorkstationName: "review-work",
		UserMessage:     "hello",
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	launch := mock.lastLaunch()
	if !slices.Contains(launch.Env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("env = %#v, want automation defaults", launch.Env)
	}
}
