package builtcliacceptance

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
)

func TestNewHarnessSuppressesBrowserOpeningAndAllowsExplicitOverride(t *testing.T) {
	harness := NewHarness(t, t.TempDir())
	if harness.Edges.BrowserOpener == nil {
		t.Fatal("NewHarness BrowserOpener = nil, want functional-test no-op")
	}
	if err := harness.Edges.BrowserOpener(context.Background(), "http://127.0.0.1/dashboard/ui"); err != nil {
		t.Fatalf("default BrowserOpener() error = %v", err)
	}

	called := false
	harness.Edges.BrowserOpener = func(context.Context, string) error {
		called = true
		return nil
	}
	if err := harness.Edges.BrowserOpener(context.Background(), "http://127.0.0.1/dashboard/ui"); err != nil {
		t.Fatalf("override BrowserOpener() error = %v", err)
	}
	if !called {
		t.Fatal("explicit BrowserOpener override was not called")
	}
}

func TestProcessEnvForIsolatedHome_ReplacesHomeVariables(t *testing.T) {
	t.Setenv("HOME", "/real/home")
	t.Setenv("USERPROFILE", "/real/home")
	t.Setenv("HOMEDRIVE", "C:")
	t.Setenv("HOMEPATH", "\\Users\\real")
	t.Setenv("PATH", "/bin:/usr/bin")

	isolatedHome := filepath.Join(t.TempDir(), "isolated-home")
	env := ProcessEnvForIsolatedHome(isolatedHome)

	if got := envValue(env, "HOME"); got != isolatedHome {
		t.Fatalf("HOME = %q, want %q", got, isolatedHome)
	}
	if got := envValue(env, "USERPROFILE"); got != isolatedHome {
		t.Fatalf("USERPROFILE = %q, want %q", got, isolatedHome)
	}
	if got := envValue(env, "HOMEDRIVE"); got != filepath.VolumeName(isolatedHome) {
		t.Fatalf("HOMEDRIVE = %q, want %q", got, filepath.VolumeName(isolatedHome))
	}
	if got := envValue(env, "HOMEPATH"); got != string(os.PathSeparator) {
		t.Fatalf("HOMEPATH = %q, want %q", got, string(os.PathSeparator))
	}
	if got := envValue(env, "PATH"); got != "/bin:/usr/bin" {
		t.Fatalf("PATH = %q, want preserved PATH", got)
	}
	for _, entry := range env {
		if strings.HasPrefix(entry, "HOME=/real/home") || strings.HasPrefix(entry, "USERPROFILE=/real/home") {
			t.Fatalf("env still contains real home entry: %q", entry)
		}
	}
}

func TestProcessEnvForIsolatedHome_NormalizesBrowserOpenOptOut(t *testing.T) {
	t.Setenv(browserOpenOptOutEnvironment, "0")
	t.Setenv("BUILT_CHILD_UNRELATED", "preserved")

	env := ProcessEnvForIsolatedHome(filepath.Join(t.TempDir(), "isolated-home"))
	entries := envEntries(env, browserOpenOptOutEnvironment)
	if len(entries) != 1 || entries[0] != browserOpenOptOutEnvironment+"=1" {
		t.Fatalf("browser opt-out entries = %#v, want exactly one canonical entry", entries)
	}
	if got := envValue(env, "BUILT_CHILD_UNRELATED"); got != "preserved" {
		t.Fatalf("unrelated environment = %q, want preserved value", got)
	}
}

func TestProcessEnvWithRetainsCanonicalBrowserOpenOptOutAcrossOverrides(t *testing.T) {
	session := &Session{HomeDir: filepath.Join(t.TempDir(), "isolated-home")}
	for _, extra := range []string{
		browserOpenOptOutEnvironment + "=",
		browserOpenOptOutEnvironment + "=0",
		browserOpenOptOutEnvironment + "=true",
		browserOpenOptOutEnvironment + "=1",
	} {
		extra := extra
		t.Run(extra, func(t *testing.T) {
			env := session.ProcessEnvWith(extra, "BUILT_CHILD_EXTRA=preserved")
			entries := envEntries(env, browserOpenOptOutEnvironment)
			if len(entries) != 1 || entries[0] != browserOpenOptOutEnvironment+"=1" {
				t.Fatalf("browser opt-out entries = %#v, want exactly one canonical entry", entries)
			}
			if got := envValue(env, "BUILT_CHILD_EXTRA"); got != "preserved" {
				t.Fatalf("extra environment = %q, want preserved value", got)
			}
			if got := envValue(env, "HOME"); got != session.HomeDir {
				t.Fatalf("HOME = %q, want isolated home %q", got, session.HomeDir)
			}
		})
	}
}

func TestRunWithEnvPassesCanonicalBrowserOpenOptOutToChildRunner(t *testing.T) {
	const extraValue = "child-runner-extra"
	process := &recordingReusableProcess{}
	process.execute = func(input root.Input) error {
		entries := envEntries(input.Env, browserOpenOptOutEnvironment)
		if len(entries) != 1 || entries[0] != browserOpenOptOutEnvironment+"=1" {
			t.Fatalf("child browser opt-out entries = %#v, want exactly one canonical entry", entries)
		}
		if got := envValue(input.Env, "BUILT_CHILD_EXTRA"); got != extraValue {
			t.Fatalf("child extra environment = %q, want %q", got, extraValue)
		}
		return nil
	}
	harness := newTestReusableHarness(process)
	session := &Session{
		harness: harness,
		HomeDir: filepath.Join(t.TempDir(), "isolated-home"),
		WorkDir: t.TempDir(),
	}
	if _, err := session.RunWithEnv(context.Background(), []string{
		browserOpenOptOutEnvironment + "=false",
		"BUILT_CHILD_EXTRA=" + extraValue,
	}, "docs"); err != nil {
		t.Fatalf("RunWithEnv() error = %v", err)
	}
	if err := harness.Close(context.Background()); err != nil {
		t.Fatalf("harness.Close() error = %v", err)
	}
}

func TestScenarioFailure_ErrorIncludesDiagnostics(t *testing.T) {
	failure := &ScenarioFailure{
		Scenario:        "invalid-goal",
		Phase:           "run_process",
		Message:         "exit status 2",
		ExitCode:        2,
		StdoutTail:      "primary only",
		StderrTail:      "invalid goal syntax",
		HomeDir:         "/tmp/home",
		LogDir:          "/tmp/logs",
		ProcessBoundary: "root.BuildProcess",
	}

	got := failure.Error()
	for _, want := range []string{
		"scenario=invalid-goal",
		"run_process",
		"exit status 2",
		"exit_code=2",
		"stdout_tail=primary only",
		"stderr_tail=invalid goal syntax",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Error() = %q, want substring %q", got, want)
		}
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func envEntries(env []string, key string) []string {
	entries := make([]string, 0)
	for _, entry := range env {
		entryKey, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(entryKey, key) {
			entries = append(entries, entry)
		}
	}
	return entries
}

func TestReusableHarnessPreservesInvocationLocalInputsAcrossSuccessFailureSuccess(t *testing.T) {
	const contextKey = reusableTestContextKey("invocation")
	expectedFailure := errors.New("expected reusable invocation failure")
	workingDirs := map[string]string{
		"first":   filepath.Join(t.TempDir(), "first"),
		"failure": filepath.Join(t.TempDir(), "failure"),
		"last":    filepath.Join(t.TempDir(), "last"),
	}
	process := &recordingReusableProcess{}
	process.execute = func(input root.Input) error {
		if len(input.Args) != 2 || input.Args[0] != "you" {
			t.Fatalf("process args = %#v, want executable plus one invocation argument", input.Args)
		}
		name := input.Args[1]
		wantContext, ok := input.Context.Value(contextKey).(string)
		if !ok {
			t.Fatalf("process context has no invocation value for %q", name)
		}
		if got := envValue(input.Env, "TEST_REUSABLE"); got != wantContext {
			t.Fatalf("process environment TEST_REUSABLE = %q, want %q", got, wantContext)
		}
		if got := input.WorkingDirectory; got != workingDirs[name] {
			t.Fatalf("process working directory for %s = %q, want %q", name, got, workingDirs[name])
		}
		payload, err := io.ReadAll(input.Stdin)
		if err != nil {
			t.Fatalf("read %s stdin: %v", name, err)
		}
		if got := string(payload); got != "input-"+name {
			t.Fatalf("process stdin for %s = %q, want %q", name, got, "input-"+name)
		}
		if _, err := io.WriteString(input.Stdout, "stdout-"+name); err != nil {
			t.Fatalf("write %s stdout: %v", name, err)
		}
		if _, err := io.WriteString(input.Stderr, "stderr-"+name); err != nil {
			t.Fatalf("write %s stderr: %v", name, err)
		}
		if name == "failure" {
			return expectedFailure
		}
		return nil
	}
	harness := newTestReusableHarness(process)

	cases := []struct {
		name        string
		workingDir  string
		wantFailure bool
	}{
		{name: "first", workingDir: workingDirs["first"]},
		{name: "failure", workingDir: workingDirs["failure"], wantFailure: true},
		{name: "last", workingDir: workingDirs["last"]},
	}
	for _, tc := range cases {
		args := []string{tc.name}
		ctx := context.WithValue(context.Background(), contextKey, tc.name)
		command := harness.CommandContext(ctx, args...)
		args[0] = "mutated-after-command-construction"
		command.Dir = tc.workingDir
		command.Env = []string{"TEST_REUSABLE=" + tc.name}
		command.Stdin = strings.NewReader("input-" + tc.name)
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr

		err := command.Run()
		if tc.wantFailure {
			if !errors.Is(err, expectedFailure) {
				t.Fatalf("%s error = %v, want expected failure", tc.name, err)
			}
		} else if err != nil {
			t.Fatalf("%s error = %v, want nil", tc.name, err)
		}
		if got := stdout.String(); got != "stdout-"+tc.name {
			t.Fatalf("%s stdout = %q, want invocation-local output", tc.name, got)
		}
		if got := stderr.String(); got != "stderr-"+tc.name {
			t.Fatalf("%s stderr = %q, want invocation-local output", tc.name, got)
		}
	}
	if got := process.inputCount(); got != len(cases) {
		t.Fatalf("reusable process execute calls = %d, want %d", got, len(cases))
	}
	if err := harness.Close(context.Background()); err != nil {
		t.Fatalf("harness.Close() error = %v, want nil", err)
	}
	if got := process.closeCallCount(); got != 1 {
		t.Fatalf("reusable process close calls = %d, want exactly one", got)
	}
}

func TestReusableHarnessRejectsOverlappingStartedCommandAndRecovers(t *testing.T) {
	started := make(chan struct{})
	var startedOnce sync.Once
	process := &recordingReusableProcess{}
	process.execute = func(input root.Input) error {
		if len(input.Args) > 1 && input.Args[1] == "blocking" {
			startedOnce.Do(func() { close(started) })
			<-input.Context.Done()
			return input.Context.Err()
		}
		return nil
	}
	harness := newTestReusableHarness(process)

	first := harness.Command("blocking")
	if err := first.Start(); err != nil {
		t.Fatalf("first.Start() error = %v, want nil", err)
	}
	// This is a failure ceiling for a broken asynchronous start, not a
	// synchronization delay; the channel is closed by the Process.Execute call.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first command did not reach the reusable process")
	}

	second := harness.Command("overlap")
	if err := second.Start(); err == nil || !strings.Contains(err.Error(), "overlapping invocations") {
		t.Fatalf("second.Start() error = %v, want actionable overlap rejection", err)
	}
	first.Cancel()
	if err := first.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("first.Wait() error = %v, want context.Canceled after cancellation", err)
	}
	if err := harness.Command("after-cancel").Run(); err != nil {
		t.Fatalf("post-overlap serial command error = %v, want nil", err)
	}
	if got := process.inputCount(); got != 2 {
		t.Fatalf("reusable process execute calls = %d, want first and recovered commands only", got)
	}
	if err := harness.Close(context.Background()); err != nil {
		t.Fatalf("harness.Close() error = %v, want nil", err)
	}
}

func TestConcurrentReusableHarnessOverlapsCommandsAndWaitsForIdleBeforeClose(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	process := &recordingReusableProcess{execute: func(root.Input) error {
		entered <- struct{}{}
		<-release
		return nil
	}}
	harness := &Harness{
		process:       process,
		reusableState: newReusableHarnessState(true),
	}

	first := harness.Command("first")
	second := harness.Command("second")
	if err := first.Start(); err != nil {
		t.Fatalf("first.Start() error = %v, want nil", err)
	}
	if err := second.Start(); err != nil {
		t.Fatalf("second.Start() error = %v, want nil", err)
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent commands did not both enter the reusable process")
		}
	}

	closed := make(chan error, 1)
	go func() { closed <- harness.Close(context.Background()) }()
	select {
	case err := <-closed:
		t.Fatalf("Close() returned before active commands completed: %v", err)
	default:
	}
	close(release)
	if err := first.Wait(); err != nil {
		t.Fatalf("first.Wait() error = %v, want nil", err)
	}
	if err := second.Wait(); err != nil {
		t.Fatalf("second.Wait() error = %v, want nil", err)
	}
	if err := <-closed; err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
	if got := process.closeCallCount(); got != 1 {
		t.Fatalf("reusable process close calls = %d, want exactly one", got)
	}
}

func TestReusableHarnessCloseIsExactlyOnceAndReportsError(t *testing.T) {
	expectedCloseError := errors.New("real reusable process close failure")
	process := &recordingReusableProcess{closeErr: expectedCloseError}
	harness := newTestReusableHarness(process)

	firstErr := harness.Close(context.Background())
	secondErr := harness.Close(context.Background())
	if !errors.Is(firstErr, expectedCloseError) || !errors.Is(secondErr, expectedCloseError) {
		t.Fatalf("harness close errors = %v, %v; want propagated close error", firstErr, secondErr)
	}
	if got := process.closeCallCount(); got != 1 {
		t.Fatalf("reusable process close calls = %d, want exactly one", got)
	}
	if err := harness.Command("after-close").Run(); err == nil || !strings.Contains(err.Error(), reusableHarnessClosedMessage) {
		t.Fatalf("command after close error = %v, want closed-harness rejection", err)
	}
}

func TestNewReusableHarnessUsesRealRootForSuccessFailureSuccess(t *testing.T) {
	harness := NewReusableHarness(t, testutil.MustRepoRoot(t))
	run := func(args ...string) (string, string, error) {
		command := harness.CommandContext(t.Context(), args...)
		command.Dir = t.TempDir()
		command.Env = ProcessEnvForIsolatedHome(t.TempDir())
		command.Stdin = strings.NewReader("reusable-root-input")
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		err := command.Run()
		return stdout.String(), stderr.String(), err
	}

	stdout, stderr, err := run("docs")
	if err != nil {
		t.Fatalf("first real-root docs command error = %v", err)
	}
	if !strings.Contains(stdout, "# Docs") || stderr != "" {
		t.Fatalf("first real-root result stdout=%q stderr=%q, want docs stdout and empty stderr", stdout, stderr)
	}

	stdout, _, err = run("docs", "unknown")
	if err == nil || !strings.Contains(err.Error(), `unsupported docs topic "unknown"`) {
		t.Fatalf("expected real-root docs failure, got error = %v", err)
	}
	if stdout != "" {
		t.Fatalf("real-root failed docs stdout = %q, want empty", stdout)
	}

	stdout, stderr, err = run("docs")
	if err != nil {
		t.Fatalf("last real-root docs command error = %v", err)
	}
	if !strings.Contains(stdout, "# Docs") || stderr != "" {
		t.Fatalf("last real-root result stdout=%q stderr=%q, want docs stdout and empty stderr", stdout, stderr)
	}
	if err := harness.Close(context.Background()); err != nil {
		t.Fatalf("real-root harness.Close() error = %v, want nil", err)
	}
}

type reusableTestContextKey string

type recordingReusableProcess struct {
	mu         sync.Mutex
	execute    func(root.Input) error
	inputs     []root.Input
	closeErr   error
	closeCalls int
}

func (p *recordingReusableProcess) Execute(input root.Input) error {
	p.mu.Lock()
	p.inputs = append(p.inputs, input)
	execute := p.execute
	p.mu.Unlock()
	if execute == nil {
		return nil
	}
	return execute(input)
}

func (p *recordingReusableProcess) Close(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeCalls++
	return p.closeErr
}

func (p *recordingReusableProcess) inputCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.inputs)
}

func (p *recordingReusableProcess) closeCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closeCalls
}

func newTestReusableHarness(process reusableProcess) *Harness {
	return &Harness{process: process, reusableState: newReusableHarnessState(false)}
}
