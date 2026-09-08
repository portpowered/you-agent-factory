//go:build windows

package omni_diag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/locking"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"golang.org/x/sys/windows"
)

const (
	localAIOMNIRealEnableEnv   = "INFINITE_YOU_LOCALAI_OMNI_REAL"
	localAIOMNIRealManifestEnv = "INFINITE_YOU_LOCALAI_OMNI_REAL_MANIFEST"

	localAIOMNIProcessHelperName = "TestLocalAIOMNIProcessHelper"
	localAIOMNIChildPIDFileEnv   = "INFINITE_YOU_LOCALAI_OMNI_CHILD_PID_FILE"
	localAIOMNIWorkRootEnv       = "INFINITE_YOU_LOCALAI_OMNI_WORK_ROOT"
	localAIOMNIProcessWaitGrace  = 5 * time.Second
)

// localAIOMNIRealAdmission is deliberately the first boundary of the real
// entry. In particular, it must not inspect the manifest variable until the
// literal opt-in has been accepted. This keeps the default diagnostic suite
// inert even when callers provide a trap path or a stale manifest.
func localAIOMNIRealAdmission(getenv func(string) string) (localAIOMNIInvocation, bool, error) {
	if getenv == nil {
		return localAIOMNIInvocation{}, true, errors.New("real entry environment lookup is required")
	}
	if strings.TrimSpace(getenv(localAIOMNIRealEnableEnv)) != "1" {
		return localAIOMNIInvocation{}, false, nil
	}
	return func() (localAIOMNIInvocation, bool, error) {
		manifestPath := getenv(localAIOMNIRealManifestEnv)
		invocation, err := localAIOMNIInvocationValues("1", manifestPath)
		return invocation, true, err
	}()
}

// localAIOMNIRealEntryWith keeps environment admission and runner construction
// separate so component tests can prove that disabled or invalid input never
// constructs a runner. Production use goes through localAIOMNIRealEntry below.
func localAIOMNIRealEntryWith(
	ctx context.Context,
	getenv func(string) string,
	newRunner func() (localAIOMNIRunner, error),
) (localAIOMNIReport, bool, error) {
	invocation, enabled, err := localAIOMNIRealAdmission(getenv)
	if !enabled || err != nil {
		return localAIOMNIReport{}, enabled, err
	}
	if newRunner == nil {
		return localAIOMNIReport{}, true, errors.New("real entry runner factory is required")
	}
	runner, err := newRunner()
	if err != nil {
		return localAIOMNIReport{}, true, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	report, err := runner.Run(ctx, invocation)
	return report, true, err
}

// localAIOMNIRealEntry is the only environment-bound real path. It delegates
// all manifest policy, reservation, classification, reporting, and release
// inspection to the merged diagnostic runner.
func localAIOMNIRealEntry(ctx context.Context) (localAIOMNIReport, bool, error) {
	return localAIOMNIRealEntryWith(ctx, os.Getenv, localAIOMNIWindowsRunner)
}

func localAIOMNIWindowsRunner() (localAIOMNIRunner, error) {
	locks, err := locking.New(locking.LocalFileSystem{})
	if err != nil {
		return localAIOMNIRunner{}, fmt.Errorf("create real-entry budget locks: %w", err)
	}
	return localAIOMNIRunner{executor: localAIOMNIWindowsExecutor{}, locks: locks}, nil
}

// localAIOMNIWindowsExecutor is the contained Windows boundary. The command
// path and arguments are copied exactly from the immutable runner command; the
// executor contributes only process supervision, bounded capture, and the
// invocation-local environment.
type localAIOMNIWindowsExecutor struct {
	onStart func()
}

var _ localAIOMNIExecutor = localAIOMNIWindowsExecutor{}

func (executor localAIOMNIWindowsExecutor) Execute(ctx context.Context, spec localAICommandSpec, roots localAIRealRoots) localAIOMNIObservation {
	observation := localAIOMNIObservation{localAICommandObservation: localAICommandObservation{ExitCode: -1}}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		observation.Cancelled = errors.Is(err, context.Canceled)
		observation.TimedOut = errors.Is(err, context.DeadlineExceeded)
		return observation
	}
	if err := localAIOMNIExecutorRoots(roots); err != nil || strings.TrimSpace(spec.BinaryPath) == "" || !filepath.IsAbs(spec.BinaryPath) || len(spec.Arguments) == 0 {
		return observation
	}

	command := exec.Command(spec.BinaryPath, spec.Arguments...)
	command.Dir = roots.Work
	command.Env = localAIOMNIWindowsEnvironment(roots)
	command.WaitDelay = localAIOMNIProcessWaitGrace
	var stdout, stderr localAIOMNIWindowsBoundedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	platformprocess.ConfigureSubprocessTree(command)
	if err := command.Start(); err != nil {
		return observation
	}
	observation.Started = true
	if executor.onStart != nil {
		executor.onStart()
	}

	tree, attachErr := platformprocess.AttachSubprocessTree(command)
	attached := attachErr == nil
	if attachErr != nil {
		// A process that cannot be attached is never a successful execution.
		// TerminateSubprocessTree falls back to the started parent for the empty
		// tree, after which Wait still reaps the process deterministically.
		_ = platformprocess.TerminateSubprocessTree(command, tree)
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- command.Wait() }()

	waitErr, waited := localAIOMNIWaitForProcess(ctx, waitCh)
	if !waited {
		observation.Cancelled = errors.Is(ctx.Err(), context.Canceled)
		observation.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		_ = platformprocess.TerminateSubprocessTree(command, tree)
		waitErr, waited = localAIOMNIWaitForProcessAfterTerminate(waitCh)
	} else if ctx.Err() != nil {
		observation.Cancelled = errors.Is(ctx.Err(), context.Canceled)
		observation.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
	}

	if attached {
		platformprocess.CloseSubprocessTree(command, tree)
	}
	observation.ProcessTreeClosed = attached && waited
	observation.ProcessExited = command.ProcessState != nil
	if waitErr == nil {
		observation.ExitCode = 0
	} else {
		var exitErr *exec.ExitError
		switch {
		case errors.As(waitErr, &exitErr):
			observation.ExitCode = exitErr.ExitCode()
		case errors.Is(waitErr, exec.ErrWaitDelay) && command.ProcessState != nil:
			observation.ExitCode = command.ProcessState.ExitCode()
		}
	}
	observation.Stdout = append([]byte(nil), stdout.Bytes()...)
	observation.Stderr = append([]byte(nil), stderr.Bytes()...)
	observation.StdoutTruncated = stdout.Truncated()
	observation.StderrTruncated = stderr.Truncated()
	return observation
}

func localAIOMNIWaitForProcess(ctx context.Context, waitCh <-chan error) (error, bool) {
	select {
	case waitErr := <-waitCh:
		return waitErr, true
	case <-ctx.Done():
		return nil, false
	}
}

func localAIOMNIWaitForProcessAfterTerminate(waitCh <-chan error) (error, bool) {
	timer := time.NewTimer(localAIOMNIProcessWaitGrace)
	defer timer.Stop()
	select {
	case waitErr := <-waitCh:
		return waitErr, true
	case <-timer.C:
		return nil, false
	}
}

func localAIOMNIExecutorRoots(roots localAIRealRoots) error {
	for _, root := range []string{roots.Work, roots.Profile, roots.Cache, roots.HFHome, roots.HFCache, roots.Temp, roots.Output, roots.Streams} {
		if err := localAIOMNIAbs(root); err != nil {
			return errors.New("executor requires absolute isolated roots")
		}
	}
	return nil
}

type localAIOMNIWindowsBoundedBuffer struct {
	body      bytes.Buffer
	truncated bool
}

func (buffer *localAIOMNIWindowsBoundedBuffer) Write(value []byte) (int, error) {
	remaining := localAIOMNIMaxStream - buffer.body.Len()
	if remaining <= 0 {
		buffer.truncated = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = buffer.body.Write(value[:remaining])
		buffer.truncated = true
		return len(value), nil
	}
	return buffer.body.Write(value)
}

func (buffer *localAIOMNIWindowsBoundedBuffer) Bytes() []byte { return buffer.body.Bytes() }

func (buffer *localAIOMNIWindowsBoundedBuffer) Truncated() bool { return buffer.truncated }

func localAIOMNIWindowsEnvironment(roots localAIRealRoots) []string {
	overrides := []struct{ name, value string }{
		{"HOME", roots.Profile},
		{"USERPROFILE", roots.Profile},
		{"APPDATA", filepath.Join(roots.Profile, "AppData", "Roaming")},
		{"LOCALAPPDATA", filepath.Join(roots.Profile, "AppData", "Local")},
		{"XDG_CONFIG_HOME", filepath.Join(roots.Profile, "xdg-config")},
		{"XDG_CACHE_HOME", roots.Cache},
		{"XDG_DATA_HOME", filepath.Join(roots.Profile, "xdg-data")},
		{"XDG_STATE_HOME", roots.Profile},
		{"HF_HOME", roots.HFHome},
		{"HF_HUB_CACHE", roots.HFCache},
		{"HUGGINGFACE_HUB_CACHE", roots.HFCache},
		{"TEMP", roots.Temp},
		{"TMP", roots.Temp},
		{"TMPDIR", roots.Temp},
		{localAIOMNIWorkRootEnv, roots.Work},
		{"INFINITE_YOU_LOCALAI_OMNI_OUTPUT_ROOT", roots.Output},
		{"INFINITE_YOU_LOCALAI_OMNI_STREAMS_ROOT", roots.Streams},
	}
	values := make([]string, 0, len(os.Environ())+len(overrides))
	positions := map[string]int{}
	set := func(name, value string) {
		key := strings.ToUpper(name)
		if position, ok := positions[key]; ok {
			values[position] = name + "=" + value
			return
		}
		positions[key] = len(values)
		values = append(values, name+"="+value)
	}
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok && !localAIOMNISecretEnvironmentName(name) {
			set(name, value)
		}
	}
	for _, override := range overrides {
		set(override.name, override.value)
	}
	return values
}

func localAIOMNISecretEnvironmentName(name string) bool {
	name = strings.ToUpper(strings.TrimSpace(name))
	return name == "HF_TOKEN" || name == "HUGGINGFACE_TOKEN" || strings.Contains(name, "API_KEY") || strings.Contains(name, "ACCESS_TOKEN") || strings.Contains(name, "PASSWORD") || strings.Contains(name, "SECRET")
}

// TestLocalAIOMNIRealEntry is the authorized environment-bound entry. It is
// skipped unless the caller explicitly opts in, so normal CI cannot inspect a
// manifest, reserve a budget, or start a process.
func TestLocalAIOMNIRealEntry(t *testing.T) {
	report, enabled, err := localAIOMNIRealEntry(t.Context())
	if !enabled {
		t.Skip("OMNI real entry is disabled; set INFINITE_YOU_LOCALAI_OMNI_REAL=1 for an authorized run")
	}
	if err != nil {
		t.Fatalf("authorized OMNI real entry: %v", err)
	}
	if report.Status == "" {
		t.Fatal("authorized OMNI real entry returned an empty report")
	}
}

func TestLocalAIOMNIRealEntryAdmission(t *testing.T) {
	t.Run("disabled-before-manifest-lookup", localAIOMNIRealEntryDisabledTest)
	t.Run("valid-manifest-counts-one-unchanged-command", localAIOMNIRealEntryValidTest)
	t.Run("invalid-input-stops-before-runner-construction", localAIOMNIRealEntryInvalidTest)
	t.Run("symlink-and-digest-drift-stop-before-runner-construction", localAIOMNIRealEntryIdentityTest)
}

func localAIOMNIRealEntryDisabledTest(t *testing.T) {
	for _, value := range []string{"", "0", "true", " 0 ", "01"} {
		value := value
		t.Run(strconv.Quote(value), func(t *testing.T) {
			var manifestLookups, runnerBuilds atomic.Int32
			getenv := func(name string) string {
				switch name {
				case localAIOMNIRealEnableEnv:
					return value
				case localAIOMNIRealManifestEnv:
					manifestLookups.Add(1)
					return filepath.Join(t.TempDir(), "trap.json")
				default:
					return ""
				}
			}
			_, enabled, err := localAIOMNIRealEntryWith(t.Context(), getenv, func() (localAIOMNIRunner, error) {
				runnerBuilds.Add(1)
				return localAIOMNIRunner{}, nil
			})
			if err != nil || enabled || manifestLookups.Load() != 0 || runnerBuilds.Load() != 0 {
				t.Fatalf("disabled admission = enabled:%t err:%v manifestLookups:%d runnerBuilds:%d", enabled, err, manifestLookups.Load(), runnerBuilds.Load())
			}
		})
	}
}

func localAIOMNIRealEntryValidTest(t *testing.T) {
	for _, selector := range []string{localAIOMNIText, localAIOMNIImage, localAIOMNIVideo} {
		selector := selector
		t.Run(selector, func(t *testing.T) {
			root := t.TempDir()
			manifest, manifestPath := localAIOMNIRealManifestForTest(t, root, selector)
			executor := &localAIOMNIRealCountingExecutor{output: localAIOMNIRealOutputFor(selector, manifest)}
			locks, err := locking.New(locking.LocalFileSystem{})
			if err != nil {
				t.Fatalf("new locks: %v", err)
			}
			runnerFactory := func() (localAIOMNIRunner, error) {
				return localAIOMNIRunner{executor: executor, locks: locks}, nil
			}
			getenv := localAIOMNIRealEntryEnv(" 1 ", manifestPath)
			report, enabled, err := localAIOMNIRealEntryWith(t.Context(), getenv, runnerFactory)
			if err != nil || !enabled || report.Status != "PASS" {
				t.Fatalf("entry report=%#v enabled=%t err=%v", report, enabled, err)
			}
			if got := executor.calls.Load(); got != 1 {
				t.Fatalf("executor calls=%d, want 1", got)
			}
			commands := executor.Commands()
			want := localAIOMNICommand(manifest)
			if len(commands) != 1 || commands[0].BinaryPath != want.BinaryPath || !sameLocalAIOMNIArgs(commands[0].Arguments, want.Arguments) {
				t.Fatalf("command=%#v, want unchanged %#v", commands, want)
			}
			if report.Reservation == nil || report.Reservation.State != "COMMITTED" || !report.Release.Checked || !report.Release.ProcessTreeClosed || report.Release.OwnedProcesses != 0 {
				t.Fatalf("entry did not preserve runner release proof: %#v", report)
			}
		})
	}
}

func localAIOMNIRealEntryInvalidTest(t *testing.T) {
	cases := []struct {
		name string
		body []byte
		path string
	}{
		{name: "missing", path: filepath.Join(t.TempDir(), "missing.json")},
		{name: "relative", path: "manifest.json"},
		{name: "malformed", body: []byte("{"), path: filepath.Join(t.TempDir(), "malformed.json")},
		{name: "oversized", body: bytes.Repeat([]byte("x"), localAIOMNIMaxManifest+1), path: filepath.Join(t.TempDir(), "oversized.json")},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			if len(testCase.body) > 0 {
				if err := os.WriteFile(testCase.path, testCase.body, 0o600); err != nil {
					t.Fatalf("write manifest: %v", err)
				}
			}
			var runnerBuilds atomic.Int32
			_, enabled, err := localAIOMNIRealEntryWith(t.Context(), localAIOMNIRealEntryEnv("1", testCase.path), func() (localAIOMNIRunner, error) {
				runnerBuilds.Add(1)
				return localAIOMNIRunner{}, nil
			})
			if !enabled || err == nil || runnerBuilds.Load() != 0 {
				t.Fatalf("admission enabled:%t err:%v runnerBuilds:%d", enabled, err, runnerBuilds.Load())
			}
		})
	}
}

func localAIOMNIRealEntryIdentityTest(t *testing.T) {
	root := t.TempDir()
	manifest, manifestPath := localAIOMNIRealManifestForTest(t, root, localAIOMNIText)
	symlinkPath := filepath.Join(root, "symlink.json")
	symlinkErr := os.Symlink(manifestPath, symlinkPath)
	t.Run("digest-drift", func(t *testing.T) {
		if err := os.WriteFile(manifest.Image.Path, []byte("drifted image fixture"), 0o600); err != nil {
			t.Fatalf("drift fixture: %v", err)
		}
		localAIOMNIRealEntryMustRejectBeforeRunner(t, manifestPath)
	})
	if symlinkErr != nil {
		t.Logf("symlink admission witness unavailable: %v", symlinkErr)
		return
	}
	t.Run("symlink", func(t *testing.T) {
		localAIOMNIRealEntryMustRejectBeforeRunner(t, symlinkPath)
	})
}

func TestLocalAIOMNIRealEntryRejectsDirectoryJunctionBeforeRunner(t *testing.T) {
	targetRoot := t.TempDir()
	_, targetManifestPath := localAIOMNIRealManifestForTest(t, targetRoot, localAIOMNIText)
	linkRoot := t.TempDir()
	junctionPath := filepath.Join(linkRoot, "manifest-root")
	command := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", junctionPath, targetRoot)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create directory junction: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			_ = os.Remove(junctionPath)
		}
	})

	linkedManifestPath := filepath.Join(junctionPath, filepath.Base(targetManifestPath))
	var runnerBuilds atomic.Int32
	_, enabled, err := localAIOMNIRealEntryWith(t.Context(), localAIOMNIRealEntryEnv("1", linkedManifestPath), func() (localAIOMNIRunner, error) {
		runnerBuilds.Add(1)
		return localAIOMNIRunner{}, nil
	})
	var reparseErr *localAIOMNIReparsePathError
	if !enabled || !errors.As(err, &reparseErr) || reparseErr.Label != "manifest" || reparseErr.Final || reparseErr.ComponentIndex != 1 || runnerBuilds.Load() != 0 {
		t.Fatalf("junction admission enabled=%t err=%v typed=%#v runner builds=%d", enabled, err, reparseErr, runnerBuilds.Load())
	}
	if strings.Contains(err.Error(), junctionPath) || strings.Contains(err.Error(), targetRoot) {
		t.Fatalf("junction error leaked path: %q", err)
	}
	if _, err := os.Stat(targetManifestPath); err != nil {
		t.Fatalf("target manifest after rejected junction: %v", err)
	}
	if err := os.Remove(junctionPath); err != nil {
		t.Fatalf("remove task-owned directory junction: %v", err)
	}
	cleaned = true
	if _, err := os.Lstat(junctionPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("junction cleanup stat=%v, want not-exist", err)
	}
}

func localAIOMNIRealEntryEnv(enable, manifestPath string) func(string) string {
	return func(name string) string {
		switch name {
		case localAIOMNIRealEnableEnv:
			return enable
		case localAIOMNIRealManifestEnv:
			return manifestPath
		default:
			return ""
		}
	}
}

func localAIOMNIRealEntryMustRejectBeforeRunner(t *testing.T, manifestPath string) {
	t.Helper()
	var runnerBuilds atomic.Int32
	_, enabled, err := localAIOMNIRealEntryWith(t.Context(), localAIOMNIRealEntryEnv("1", manifestPath), func() (localAIOMNIRunner, error) {
		runnerBuilds.Add(1)
		return localAIOMNIRunner{}, nil
	})
	if !enabled || err == nil || runnerBuilds.Load() != 0 {
		t.Fatalf("admission enabled:%t err:%v runnerBuilds:%d", enabled, err, runnerBuilds.Load())
	}
}

type localAIOMNIRealCountingExecutor struct {
	output []byte
	calls  atomic.Int32
	mu     sync.Mutex
	cmds   []localAICommandSpec
}

func (executor *localAIOMNIRealCountingExecutor) Execute(_ context.Context, spec localAICommandSpec, roots localAIRealRoots) localAIOMNIObservation {
	executor.calls.Add(1)
	executor.mu.Lock()
	executor.cmds = append(executor.cmds, localAICommandSpec{BinaryPath: spec.BinaryPath, Arguments: append([]string(nil), spec.Arguments...)})
	executor.mu.Unlock()
	output := append([]byte(nil), executor.output...)
	observation := localAIOMNIPass(roots.Work, string(output))
	observation.Started = true
	return observation
}

func (executor *localAIOMNIRealCountingExecutor) Commands() []localAICommandSpec {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	commands := make([]localAICommandSpec, len(executor.cmds))
	for i, command := range executor.cmds {
		commands[i] = localAICommandSpec{BinaryPath: command.BinaryPath, Arguments: append([]string(nil), command.Arguments...)}
	}
	return commands
}

func sameLocalAIOMNIArgs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func localAIOMNIRealManifestForTest(t testing.TB, root, selector string) (localAIOMNIManifest, string) {
	t.Helper()
	manifest := localAIOMNIManifestForTest(t, root)
	manifest.Selector = selector
	path, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatalf("absolute test binary: %v", err)
	}
	identity, ok := localAIReadFileIdentity(path)
	if !ok {
		t.Fatalf("read test binary identity: %s", path)
	}
	manifest.CLI = localAIOMNICLI{Path: path, SHA256: identity.SHA256}
	manifest.Evidence.ReportPath = filepath.Join(root, "evidence", selector, "report.json")
	manifest.Evidence.LedgerPath = filepath.Join(root, "evidence", selector, "ledger.json")
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	manifestPath := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifestPath, body, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return manifest, manifestPath
}

func localAIOMNIRealOutputFor(selector string, manifest localAIOMNIManifest) []byte {
	switch selector {
	case localAIOMNIImage:
		return []byte(strings.Join(manifest.Image.RequiredFacts, "; "))
	case localAIOMNIVideo:
		return []byte(fmt.Sprintf("%s %s then %s %s transition 2000 ms", manifest.Video.Phase1, manifest.Video.Phase1Color, manifest.Video.Phase2, manifest.Video.Phase2Color))
	default:
		return []byte(manifest.Text.Token)
	}
}

func TestLocalAIOMNIRealExecutor(t *testing.T) {
	t.Run("success-forwards-exact-command-and-isolates-environment", func(t *testing.T) {
		roots := localAIOMNIExecutorRootsForTest(t)
		executor := localAIOMNIWindowsExecutor{}
		observation := executor.Execute(t.Context(), localAIOMNIHelperCommand(t, "success"), roots)
		if !observation.Started || !observation.ProcessExited || observation.ExitCode != 0 || !observation.ProcessTreeClosed || observation.StdoutTruncated || observation.StderrTruncated {
			t.Fatalf("success observation = %#v", observation)
		}
		output := string(observation.Stdout)
		for _, want := range []string{roots.Profile, roots.Cache, roots.HFHome, roots.HFCache, roots.Temp, roots.Work} {
			if !strings.Contains(output, want) {
				t.Fatalf("success output %q does not contain isolated root %q", output, want)
			}
		}
	})

	t.Run("nonzero-exit-is-observed-and-closed", func(t *testing.T) {
		observation := localAIOMNIWindowsExecutor{}.Execute(t.Context(), localAIOMNIHelperCommand(t, "failure"), localAIOMNIExecutorRootsForTest(t))
		if !observation.Started || !observation.ProcessExited || observation.ExitCode != 17 || !observation.ProcessTreeClosed {
			t.Fatalf("failure observation = %#v", observation)
		}
	})

	t.Run("invalid-executable-does-not-start", func(t *testing.T) {
		roots := localAIOMNIExecutorRootsForTest(t)
		observation := localAIOMNIWindowsExecutor{}.Execute(t.Context(), localAICommandSpec{BinaryPath: filepath.Join(roots.Work, "missing.exe"), Arguments: []string{"--help"}}, roots)
		if observation.Started || observation.ProcessExited || observation.ProcessTreeClosed {
			t.Fatalf("invalid executable observation = %#v, want no process", observation)
		}
	})

	t.Run("bounded-streams-are-marked", func(t *testing.T) {
		observation := localAIOMNIWindowsExecutor{}.Execute(t.Context(), localAIOMNIHelperCommand(t, "oversized"), localAIOMNIExecutorRootsForTest(t))
		if !observation.Started || !observation.ProcessExited || !observation.ProcessTreeClosed || !observation.StdoutTruncated || !observation.StderrTruncated || len(observation.Stdout) != localAIOMNIMaxStream || len(observation.Stderr) != localAIOMNIMaxStream {
			t.Fatalf("bounded observation started=%t exited=%t closed=%t stdout=%d/%t stderr=%d/%t", observation.Started, observation.ProcessExited, observation.ProcessTreeClosed, len(observation.Stdout), observation.StdoutTruncated, len(observation.Stderr), observation.StderrTruncated)
		}
	})

	t.Run("deadline-terminates-contained-tree", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
		defer cancel()
		observation := localAIOMNIWindowsExecutor{}.Execute(ctx, localAIOMNIHelperCommand(t, "sleep"), localAIOMNIExecutorRootsForTest(t))
		if !observation.Started || !observation.ProcessExited || !observation.TimedOut || observation.Cancelled || !observation.ProcessTreeClosed {
			t.Fatalf("timeout observation = %#v", observation)
		}
	})

	t.Run("cancellation-terminates-contained-tree", func(t *testing.T) {
		started := make(chan struct{})
		executor := localAIOMNIWindowsExecutor{onStart: func() { close(started) }}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		result := make(chan localAIOMNIObservation, 1)
		go func() {
			result <- executor.Execute(ctx, localAIOMNIHelperCommand(t, "cancel"), localAIOMNIExecutorRootsForTest(t))
		}()
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("controlled process did not start")
		}
		cancel()
		observation := <-result
		if !observation.Started || !observation.ProcessExited || observation.TimedOut || !observation.Cancelled || !observation.ProcessTreeClosed {
			t.Fatalf("cancellation observation = %#v", observation)
		}
	})

	t.Run("child-tree-is-closed-after-cancellation", func(t *testing.T) {
		roots := localAIOMNIExecutorRootsForTest(t)
		started := make(chan struct{})
		executor := localAIOMNIWindowsExecutor{onStart: func() { close(started) }}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		result := make(chan localAIOMNIObservation, 1)
		go func() { result <- executor.Execute(ctx, localAIOMNIHelperCommand(t, "child"), roots) }()
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("controlled parent did not start")
		}
		pidPath := filepath.Join(roots.Streams, "child.pid")
		pid := localAIOMNIWaitForChildPID(t, pidPath)
		cancel()
		observation := <-result
		if !observation.ProcessTreeClosed || !observation.ProcessExited || !observation.Cancelled {
			t.Fatalf("child cancellation observation = %#v", observation)
		}
		if !localAIOMNIWindowsProcessExitedWithin(pid, localAIOMNIProcessWaitGrace) {
			localAIOMNITerminateWindowsProcess(pid)
			t.Fatalf("child process %d survived the closed process tree", pid)
		}
	})
}

func localAIOMNIExecutorRootsForTest(t testing.TB) localAIRealRoots {
	t.Helper()
	root := t.TempDir()
	roots := localAIRealRoots{
		Work:    filepath.Join(root, "work"),
		Profile: filepath.Join(root, "profile"),
		Cache:   filepath.Join(root, "cache"),
		HFHome:  filepath.Join(root, "hf-home"),
		HFCache: filepath.Join(root, "hf-cache"),
		Temp:    filepath.Join(root, "temp"),
		Output:  filepath.Join(root, "output"),
		Streams: filepath.Join(root, "streams"),
	}
	if err := prepareLocalAIRoots(roots); err != nil {
		t.Fatalf("prepare executor roots: %v", err)
	}
	return roots
}

func localAIOMNIHelperCommand(t testing.TB, mode string) localAICommandSpec {
	t.Helper()
	binary, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatalf("absolute helper binary: %v", err)
	}
	return localAICommandSpec{BinaryPath: binary, Arguments: []string{"-test.run=" + localAIOMNIProcessHelperName, "--", mode}}
}

func localAIOMNIWaitForChildPID(t testing.TB, path string) uint32 {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if body, err := os.ReadFile(path); err == nil {
			pid, err := strconv.ParseUint(strings.TrimSpace(string(body)), 10, 32)
			if err == nil && pid > 0 {
				return uint32(pid)
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for controlled child PID file %s", path)
			return 0
		}
	}
}

func localAIOMNIWindowsProcessExitedWithin(pid uint32, timeout time.Duration) bool {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return true
	}
	defer windows.CloseHandle(process)
	event, err := windows.WaitForSingleObject(process, uint32(timeout.Milliseconds()))
	return err == nil && event == uint32(windows.WAIT_OBJECT_0)
}

func localAIOMNITerminateWindowsProcess(pid uint32) {
	process, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(process)
	_ = windows.TerminateProcess(process, 1)
}

// TestLocalAIOMNIProcessHelper is a controlled child entry used only by the
// integration cases above. It performs no product work and has no network or
// model dependency.
func TestLocalAIOMNIProcessHelper(t *testing.T) {
	mode := ""
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			mode = os.Args[i+1]
			break
		}
	}
	if mode == "" {
		return
	}
	switch mode {
	case "success":
		_, _ = fmt.Fprintf(os.Stdout, "HOME=%s\nUSERPROFILE=%s\nAPPDATA=%s\nXDG_CONFIG_HOME=%s\nXDG_CACHE_HOME=%s\nHF_HOME=%s\nHF_HUB_CACHE=%s\nTEMP=%s\nWORK=%s\nCWD=%s\n", os.Getenv("HOME"), os.Getenv("USERPROFILE"), os.Getenv("APPDATA"), os.Getenv("XDG_CONFIG_HOME"), os.Getenv("XDG_CACHE_HOME"), os.Getenv("HF_HOME"), os.Getenv("HF_HUB_CACHE"), os.Getenv("TEMP"), os.Getenv(localAIOMNIWorkRootEnv), currentWorkingDirectory(t))
	case "failure":
		_, _ = fmt.Fprintln(os.Stderr, "controlled nonzero exit")
		os.Exit(17)
	case "oversized":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("stdout-boundary "), localAIOMNIMaxStream/len("stdout-boundary ")+2))
		_, _ = os.Stderr.Write(bytes.Repeat([]byte("stderr-boundary "), localAIOMNIMaxStream/len("stderr-boundary ")+2))
	case "sleep", "cancel", "child-sleep":
		// A long timer keeps the controlled helper alive without triggering Go's
		// all-goroutines-asleep deadlock detector; the parent executor owns the
		// only termination path.
		<-time.NewTimer(24 * time.Hour).C
	case "child":
		child := exec.Command(os.Args[0], "-test.run="+localAIOMNIProcessHelperName, "--", "child-sleep")
		child.Dir = os.Getenv(localAIOMNIWorkRootEnv)
		child.Env = os.Environ()
		if err := child.Start(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(18)
		}
		pidPath := os.Getenv(localAIOMNIChildPIDFileEnv)
		if pidPath == "" {
			pidPath = filepath.Join(os.Getenv("INFINITE_YOU_LOCALAI_OMNI_STREAMS_ROOT"), "child.pid")
		}
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			_ = child.Process.Kill()
			os.Exit(19)
		}
		_ = child.Wait()
	default:
		_, _ = fmt.Fprintln(os.Stderr, "unknown controlled helper mode")
		os.Exit(20)
	}
}

func currentWorkingDirectory(t testing.TB) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "<unavailable>"
	}
	return workingDirectory
}
