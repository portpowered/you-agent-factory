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

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
)

const privatePrompt = "run; rm -rf / | cat"

type stubPTYSession struct {
	launch agypty.ProcessLaunch
	result agypty.SessionResult
	run    func(context.Context) (agypty.SessionResult, error)
	closes int
}

func (s *stubPTYSession) Run(ctx context.Context) (agypty.SessionResult, error) {
	if s.run != nil {
		return s.run(ctx)
	}
	return s.result, nil
}

func (s *stubPTYSession) Close() error { s.closes++; return nil }

type stubAllocator struct {
	sessions []*stubPTYSession
	result   agypty.SessionResult
	run      func(context.Context) (agypty.SessionResult, error)
	config   agypty.SessionConfig
}

func (a *stubAllocator) Allocate(_ context.Context, launch agypty.ProcessLaunch, config agypty.SessionConfig) (agypty.PTYSession, error) {
	a.config = config
	session := &stubPTYSession{launch: launch, result: a.result, run: a.run}
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

func executableInspector(existingPaths ...string) fakeExecutableInspector {
	inspector := make(fakeExecutableInspector, len(existingPaths))
	for _, path := range existingPaths {
		inspector[path] = fakeExecutableInfo{}
	}
	return inspector
}

func continuationRequest(request providers.ExecuteRequest) execution.ContinuationRequest {
	return execution.ContinuationRequest{ExecuteRequest: request}
}

func TestPTYEffectRejectsSeparateReasoningEffort(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	executable := filepath.Join(factoryRoot, "agy.exe")
	if err := os.WriteFile(executable, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	allocator := &stubAllocator{}
	effect := agy.NewPTYEffect(allocator, fakeExecutableLocator(nil), executableInspector(executable), platformclock.Real{}, agy.PTYPolicy{
		FactoryRoot: factoryRoot,
		Executable:  executable,
	})
	_, err := effect.Execute(context.Background(), continuationRequest(providers.ExecuteRequest{
		Provider:        providers.IDAntigravity,
		AttemptID:       "dispatch-agy-effort",
		ReasoningEffort: "xhigh",
		UserMessage:     "review",
	}), func([]byte) error { return nil })
	var failure execution.AttemptFailure
	if !errors.As(err, &failure) ||
		failure.NativeError == nil ||
		!strings.Contains(failure.NativeError.Error(), "does not support a separate reasoning effort") {
		t.Fatalf("Execute() error = %v, want unsupported reasoning effort", err)
	}
	if len(allocator.sessions) != 0 {
		t.Fatalf("PTY allocations = %d, want none", len(allocator.sessions))
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
	effect := agy.NewPTYEffect(mock, fakeExecutableLocator(nil), executableInspector(executable), platformclock.Real{}, agy.PTYPolicy{
		FactoryRoot: factoryRoot,
		Executable:  executable,
	})
	if effect == nil {
		t.Fatal("NewPTYEffect() returned nil")
	}
	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:         providers.IDAntigravity,
			AttemptID:        "dispatch-agy",
			Model:            "gemini-pro",
			UserMessage:      privatePrompt,
			WorkingDirectory: filepath.Join("workspaces", "a"),
			WorkerType:       "agent-worker",
			WorkstationName:  "review-work",
			EnvVars:          map[string]string{"AGY_TOKEN": "secret"},
		},
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDAntigravity,
			Kind:     providers.SessionIDKind,
			ID:       "session-1",
		},
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
	effect := agy.NewPTYEffect(mock, fakeExecutableLocator(nil), executableInspector(), platformclock.Real{}, agy.PTYPolicy{
		FactoryRoot: factoryRoot,
		Executable:  "agy",
	})
	const prompt = "summarize this; preserve argv boundaries"
	var observed []byte
	result, err := effect.Execute(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:         providers.IDAntigravity,
			AttemptID:        "dispatch-agy-contract",
			Model:            "gemini-pro",
			UserMessage:      prompt,
			WorkingDirectory: ".",
		},
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDAntigravity,
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
	effect := agy.NewPTYEffect(mock, fakeExecutableLocator(nil), executableInspector(), platformclock.Real{}, agy.PTYPolicy{
		FactoryRoot: factoryRoot,
		Executable:  "agy",
	})
	_, err := effect.Execute(context.Background(), continuationRequest(providers.ExecuteRequest{
		Provider:         providers.IDAntigravity,
		AttemptID:        "dispatch-agy-42",
		WorkingDirectory: ".",
		UserMessage:      privatePrompt,
	}), func([]byte) error { return nil })
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
	effect := agy.NewPTYEffect(mock, fakeExecutableLocator(nil), executableInspector(), platformclock.Real{}, agy.PTYPolicy{
		FactoryRoot: t.TempDir(),
		Executable:  "agy",
	})
	_, err := effect.Execute(context.Background(), continuationRequest(providers.ExecuteRequest{
		Provider:    providers.IDAntigravity,
		AttemptID:   "dispatch-agy-timeout",
		UserMessage: "hello",
	}), func([]byte) error {
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

func TestPTYEffectResolvesBareExecutableThroughInjectedSearchPath(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	resolved := filepath.Join("toolchain", "agy")
	mock := &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: "ok"}}
	effect := agy.NewPTYEffect(mock, fakeExecutableLocator(map[string]string{"agy": resolved}), executableInspector(resolved), platformclock.Real{}, agy.PTYPolicy{
		FactoryRoot: factoryRoot,
		Executable:  "agy",
	})
	_, err := effect.Execute(context.Background(), continuationRequest(providers.ExecuteRequest{
		Provider:         providers.IDAntigravity,
		AttemptID:        "dispatch-agy-path",
		WorkingDirectory: ".",
		UserMessage:      "hello",
	}), func([]byte) error { return nil })
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
	effect := agy.NewPTYEffect(mock, fakeExecutableLocator(nil), executableInspector(), platformclock.Real{}, agy.PTYPolicy{
		FactoryRoot: t.TempDir(),
		Executable:  "agy",
	})
	_, err := effect.Execute(context.Background(), continuationRequest(providers.ExecuteRequest{
		Provider:        providers.IDAntigravity,
		AttemptID:       "dispatch-agy-context",
		WorkerType:      "agent-worker",
		WorkstationName: "review-work",
		UserMessage:     "hello",
	}), func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	launch := mock.lastLaunch()
	if !slices.Contains(launch.Env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("env = %#v, want automation defaults", launch.Env)
	}
}

func TestPTYEffectUsesInjectedClockAndClosesTerminalSession(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		runErr     error
		observeErr error
		cancel     bool
	}{
		{name: "success"},
		{name: "run failure", runErr: errors.New("provider failed")},
		{name: "timeout", runErr: context.DeadlineExceeded},
		{name: "cancellation", cancel: true},
		{name: "observer failure", observeErr: errors.New("observer failed")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			allocator := &stubAllocator{run: func(ctx context.Context) (agypty.SessionResult, error) {
				clock.SetTick(37)
				if tc.cancel {
					cancel()
					return agypty.SessionResult{}, ctx.Err()
				}
				return agypty.SessionResult{CleanedText: "answer"}, tc.runErr
			}}
			effect := agy.NewPTYEffect(allocator, fakeExecutableLocator(nil), executableInspector(), clock,
				agy.PTYPolicy{FactoryRoot: t.TempDir()})
			result, err := effect.Execute(ctx, continuationRequest(providers.ExecuteRequest{
				UserMessage: "hello",
			}), func([]byte) error { return tc.observeErr })
			if result.DurationMillis != 37 {
				t.Fatalf("duration = %d, want injected 37ms", result.DurationMillis)
			}
			wantFailure := tc.runErr != nil || tc.observeErr != nil || tc.cancel
			if (err != nil) != wantFailure {
				t.Fatalf("error = %v, want failure %v", err, wantFailure)
			}
			if len(allocator.sessions) != 1 || allocator.sessions[0].closes != 1 {
				t.Fatalf("sessions = %#v, want one closed session", allocator.sessions)
			}
			if allocator.config != agypty.DefaultSessionConfig() {
				t.Fatalf("session config = %#v, want default policy", allocator.config)
			}
		})
	}
}

func TestPTYEffectPreservesExplicitSessionPolicy(t *testing.T) {
	t.Parallel()
	policy := agypty.DefaultSessionConfig()
	policy.HardTimeout = 17 * time.Minute
	allocator := &stubAllocator{}
	clock := platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond)
	effect := agy.NewPTYEffect(allocator, fakeExecutableLocator(nil), executableInspector(), clock,
		agy.PTYPolicy{FactoryRoot: t.TempDir(), SessionConfig: policy})
	_, err := effect.Execute(t.Context(), continuationRequest(providers.ExecuteRequest{
		UserMessage: "hello",
	}), func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if allocator.config != policy {
		t.Fatalf("session config = %#v, want %#v", allocator.config, policy)
	}
}
