package customer_commands_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// These tests are the in-process form of the former built-binary CLI surface
// checks: help, version, unknown-command diagnostics, stdout/stderr purity,
// stdin consumption, and validation failures. None of them needs a real OS
// process, so each drives Process.Execute on a root-built process with an
// isolated HOME and captured streams. The exact OS exit status mapping stays
// in tests/integration/transport/cli/process.

var expectedPublicRootCommandFamilies = []string{
	"run", "docs", "session", "work", "factory", "submit", "init", "server",
}

var forbiddenRootDiscoveryCommands = []string{
	"batch", "list", "show", "validate", "save", "flatten", "expand", "query", "create", "delete",
	"pause", "resume", "dispatches", "move", "render", "inspect", "invoke", "pull", "replace-current",
}

var machineReadableVersionLinePattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.+-]+)*$|^dev$`)

var activationMarkers = []string{"Factory initiated:", "Dashboard URL:", "Dashboard server disabled"}

var plainCLIProcess struct {
	once    sync.Once
	process support.ApplicationProcess
	err     error
}

// plainProcess returns one package-shared root-built process with default
// functional edges. Process wiring is immutable, so concurrent invocations
// with distinct HOME and working directories are independent.
func plainProcess(t testing.TB) support.ApplicationProcess {
	t.Helper()
	plainCLIProcess.once.Do(func() {
		plainCLIProcess.process, plainCLIProcess.err = support.BuildProcessWithContext(
			context.Background(), serviceedges.Edges{},
		)
	})
	if plainCLIProcess.err != nil {
		t.Fatalf("BuildProcess() for plain CLI surface: %v", plainCLIProcess.err)
	}
	return plainCLIProcess.process
}

func closePlainProcess(ctx context.Context) error {
	if plainCLIProcess.process == nil {
		return nil
	}
	return plainCLIProcess.process.Close(ctx)
}

type cliHome struct {
	home string
	work string
}

func newCLIHome(t testing.TB) cliHome {
	t.Helper()
	root := t.TempDir()
	h := cliHome{home: filepath.Join(root, "home"), work: filepath.Join(root, "work")}
	for _, dir := range []string{h.home, h.work} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create CLI session directory %s: %v", dir, err)
		}
	}
	return h
}

type cliResult struct {
	Stdout string
	Stderr string
	Err    error
}

func (h cliHome) run(t testing.TB, process support.Process, stdin io.Reader, args ...string) cliResult {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you"}, args...))
	inputs.Input.Env = builtcliacceptance.ProcessEnvForIsolatedHome(h.home)
	inputs.Input.WorkingDirectory = h.work
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	inputs.Input.Stdin = stdin
	err := process.Execute(inputs.Input)
	return cliResult{Stdout: inputs.Stdout(), Stderr: inputs.Stderr(), Err: err}
}

func (h cliHome) requireSuccess(t testing.TB, process support.Process, args ...string) cliResult {
	t.Helper()
	result := h.run(t, process, nil, args...)
	if result.Err != nil {
		t.Fatalf("you %v failed: %v\nstdout:\n%s\nstderr:\n%s", args, result.Err, result.Stdout, result.Stderr)
	}
	return result
}

func (h cliHome) assertNoProductFilesystemEffects(t testing.TB) {
	t.Helper()
	for _, path := range []string{
		filepath.Join(h.work, "factory"),
		filepath.Join(h.home, ".you-agent-factory", "config.json"),
	} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("command created product filesystem path %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat product filesystem path %s: %v", path, err)
		}
	}
}

func assertNoActivationMarkers(t testing.TB, label, stream string) {
	t.Helper()
	for _, forbidden := range activationMarkers {
		if strings.Contains(stream, forbidden) {
			t.Fatalf("%s contains product activation marker %q:\n%s", label, forbidden, stream)
		}
	}
}

func testProcessCLIHelpListsPublicCommandFamilies(t *testing.T) {
	t.Parallel()
	process := plainProcess(t)
	listings := map[string][]string{}
	var mu sync.Mutex
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "bare_root"}, {name: "explicit_help", args: []string{"--help"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := newCLIHome(t)
			result := home.requireSuccess(t, process, tc.args...)
			if strings.TrimSpace(result.Stderr) != "" {
				t.Fatalf("root help stderr = %q, want empty", result.Stderr)
			}
			assertNoActivationMarkers(t, "root help stdout", result.Stdout)
			if !strings.Contains(result.Stdout, "Available Commands:") || !strings.Contains(result.Stdout, "Run and manage CPN-based workflow factories") {
				t.Fatalf("root help omitted its public heading or title:\n%s", result.Stdout)
			}
			listed := parseListedRootCommands(result.Stdout)
			if len(listed) == 0 {
				t.Fatalf("root help did not list command families:\n%s", result.Stdout)
			}
			for _, family := range expectedPublicRootCommandFamilies {
				if !containsString(listed, family) {
					t.Fatalf("root help missing public family %q; listed=%v", family, listed)
				}
			}
			for _, forbidden := range forbiddenRootDiscoveryCommands {
				if containsString(listed, forbidden) {
					t.Fatalf("root help listed hidden command %q; listed=%v", forbidden, listed)
				}
			}
			if tc.name == "bare_root" && strings.Contains(result.Stdout, "How to use:") {
				t.Fatalf("bare root help emitted long-form discovery text:\n%s", result.Stdout)
			}
			home.assertNoProductFilesystemEffects(t)
			mu.Lock()
			listings[tc.name] = listed
			mu.Unlock()
		})
	}
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		bare, help := listings["bare_root"], listings["explicit_help"]
		if len(bare) == 0 || len(help) == 0 {
			return
		}
		if !sameStringSet(bare, help) {
			t.Errorf("bare root and --help listed different command families:\nbare=%v\nhelp=%v", bare, help)
		}
	})
}

func testProcessCLISubcommandHelpUsesStableUsageAndExitZero(t *testing.T) {
	t.Parallel()
	process := plainProcess(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "docs_help_flag", args: []string{"docs", "--help"}},
		{name: "docs_short_help", args: []string{"docs", "-h"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := newCLIHome(t)
			result := home.requireSuccess(t, process, tc.args...)
			if strings.TrimSpace(result.Stderr) != "" {
				t.Fatalf("nested help stderr = %q, want empty", result.Stderr)
			}
			assertNoActivationMarkers(t, "nested help stdout", result.Stdout)
			for _, marker := range []string{
				"Usage:\n  you docs [topic] [flags]",
				"Print packaged markdown reference topics from the installed binary.",
				"Flags:", "-h, --help", "help for docs",
			} {
				if !strings.Contains(result.Stdout, marker) {
					t.Fatalf("nested docs help omitted %q:\n%s", marker, result.Stdout)
				}
			}
			home.assertNoProductFilesystemEffects(t)
		})
	}
}

func testProcessCLIVersionWritesOneMachineReadableVersion(t *testing.T) {
	t.Parallel()
	home := newCLIHome(t)
	result := home.requireSuccess(t, plainProcess(t), "--version")
	if strings.TrimSpace(result.Stderr) != "" {
		t.Fatalf("version stderr = %q, want empty", result.Stderr)
	}
	assertNoActivationMarkers(t, "version stdout", result.Stdout)
	for _, forbidden := range []string{"Available Commands:", "How to use:"} {
		if strings.Contains(result.Stdout, forbidden) {
			t.Fatalf("version stdout contains help noise %q:\n%s", forbidden, result.Stdout)
		}
	}
	versionLine := strings.TrimSpace(result.Stdout)
	if versionLine == "" || strings.Contains(versionLine, "\n") || !machineReadableVersionLinePattern.MatchString(versionLine) {
		t.Fatalf("version stdout = %q, want one machine-readable version token", result.Stdout)
	}
	home.assertNoProductFilesystemEffects(t)
}

func testProcessCLIGroupHelpRendersExactlyOnce(t *testing.T) {
	t.Parallel()
	process := plainProcess(t)
	for _, family := range []string{"factory", "config", "worker-sessions"} {
		t.Run(family, func(t *testing.T) {
			t.Parallel()
			long := runGroupHelp(t, process, family, "--help")
			short := runGroupHelp(t, process, family, "-h")
			if long.Stdout != short.Stdout || long.Stderr != short.Stderr {
				t.Fatalf("%s long and short help differ:\n--help stdout:\n%s\n-h stdout:\n%s", family, long.Stdout, short.Stdout)
			}
		})
	}
	for _, family := range []string{"models", "docs"} {
		t.Run(family, func(t *testing.T) {
			t.Parallel()
			runGroupHelp(t, process, family, "--help")
		})
	}
}

func runGroupHelp(t *testing.T, process support.Process, family, flag string) cliResult {
	t.Helper()
	home := newCLIHome(t)
	result := home.requireSuccess(t, process, family, flag)
	if result.Stderr != "" {
		t.Fatalf("you %s %s stderr=%q; want empty", family, flag, result.Stderr)
	}
	if strings.TrimSpace(result.Stdout) == "" {
		t.Fatalf("you %s %s stdout is empty; want complete help", family, flag)
	}
	if got := countExactLines(result.Stdout, "Usage:"); got != 1 {
		t.Fatalf("you %s %s Usage: line count = %d, want 1; stdout:\n%s", family, flag, got, result.Stdout)
	}
	if !strings.Contains(result.Stdout, "Usage:\n  you "+family) {
		t.Fatalf("you %s %s help omitted its command usage:\n%s", family, flag, result.Stdout)
	}
	assertNoActivationMarkers(t, "group help stdout", result.Stdout)
	home.assertNoProductFilesystemEffects(t)
	return result
}

func testProcessCLIUnknownCommandWritesSafeCodedStderr(t *testing.T) {
	t.Parallel()
	home := newCLIHome(t)
	result := home.run(t, plainProcess(t), nil, "not-a-command")
	if result.Err == nil {
		t.Fatalf("unknown command succeeded; stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	// The diagnostic reaches the customer through stderr; when the process
	// boundary reports it as the returned error instead, the same text is
	// carried there.
	diagnostic := result.Stderr + "\n" + result.Err.Error()
	for _, want := range []string{
		`unknown command "not-a-command" for "you"`,
		"Run 'you --help' for usage.",
	} {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("unknown-command diagnostic = %q, want Cobra text %q", diagnostic, want)
		}
	}
	if strings.Contains(diagnostic, "CLI_COMMAND_FAILED") || strings.Contains(diagnostic, "INTERNAL_SERVER_ERROR") {
		t.Fatalf("unknown-command diagnostic used an internal failure envelope: %q", diagnostic)
	}
	for _, forbidden := range forbiddenRootDiscoveryCommands {
		if containsSuggestionToken(diagnostic, forbidden) {
			t.Fatalf("unknown command diagnostic leaked hidden command %q:\n%s", forbidden, diagnostic)
		}
	}
	for _, marker := range []string{"Factory initiated:", "Dashboard URL:", "Available Commands:"} {
		if strings.Contains(diagnostic, marker) {
			t.Fatalf("unknown command diagnostic contains non-diagnostic surface %q:\n%s", marker, diagnostic)
		}
	}
	if strings.TrimSpace(result.Stdout) != "" {
		t.Fatalf("stdout = %q, want empty after unknown command", result.Stdout)
	}
	assertNoActivationMarkers(t, "unknown command stdout", result.Stdout)
	home.assertNoProductFilesystemEffects(t)
}

// TestCLIValidationFailureReturnsErrorWithoutSuccessOutput is the in-process
// form of the validation-failure exit check; the numeric OS status remains
// covered by the built-binary exit-status integration test.
func testProcessCLIValidationFailureReturnsErrorWithoutSuccessOutput(t *testing.T) {
	t.Parallel()
	process := plainProcess(t)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "default", args: []string{"run", "--named", "@you/missing", "--no-record", "invalid-goal-prompt"}},
		{name: "quiet", args: []string{"run", "--named", "@you/missing", "--no-record", "--quiet", "invalid-goal-prompt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := newCLIHome(t)
			result := home.run(t, process, nil, tc.args...)
			if result.Err == nil {
				t.Fatalf("invalid input succeeded; stdout=%q stderr=%q", result.Stdout, result.Stderr)
			}
			if strings.TrimSpace(result.Stdout) != "" {
				t.Fatalf("invalid input stdout = %q, want empty", result.Stdout)
			}
		})
	}
}

func parseListedRootCommands(help string) []string {
	const section = "Available Commands:"
	start := strings.Index(help, section)
	if start < 0 {
		return nil
	}
	rest := help[start+len(section):]
	if flagsIdx := strings.Index(rest, "\n\nFlags:"); flagsIdx >= 0 {
		rest = rest[:flagsIdx]
	} else if flagsIdx := strings.Index(rest, "\nFlags:"); flagsIdx >= 0 {
		rest = rest[:flagsIdx]
	}
	var names []string
	for _, line := range strings.Split(rest, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	set := make(map[string]struct{}, len(left))
	for _, value := range left {
		set[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func countExactLines(output, want string) int {
	count := 0
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if line == want {
			count++
		}
	}
	return count
}

func containsSuggestionToken(text, command string) bool {
	for _, line := range strings.Split(text, "\n") {
		for _, field := range strings.Fields(line) {
			if strings.Trim(field, `"'`) == command {
				return true
			}
		}
	}
	return false
}

// ---- stdout/stderr purity and quiet mode ----

const (
	successStdoutPrimaryResult   = "mock worker accepted"
	quietPrimaryResultSeparator  = "--- primary result ---"
	goalConfigDefaultsJSON       = `{"defaults":{"workerModelProvider":"codex","workerModel":"gpt-5-codex"}}`
	stdinPromptFactoryConfigName = "stdin-run-process"
)

// configuredGoalHome materializes the initializer-owned home through a failing
// Factory selection, then installs the operator config the goal run needs.
func configuredGoalHome(t testing.TB, process support.Process) cliHome {
	t.Helper()
	home := newCLIHome(t)
	missing := filepath.Join(home.work, "missing-initialization-factory.json")
	if result := home.run(t, process, nil, "run", "--factory", missing); result.Err == nil {
		t.Fatalf("missing Factory unexpectedly succeeded: %#v", result)
	}
	configPath := filepath.Join(home.home, ".you-agent-factory", "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("initializer-owned config missing at %s: %v", configPath, err)
	}
	if err := os.WriteFile(configPath, []byte(goalConfigDefaultsJSON), 0o600); err != nil {
		t.Fatalf("write operator config %q: %v", configPath, err)
	}
	return home
}

func (h cliHome) goalRunArgs(t testing.TB, mockWorkersPath string, extra ...string) []string {
	t.Helper()
	port, err := builtcliacceptance.ReserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve goal run listener: %v", err)
	}
	args := []string{
		"--runtime-log-dir", filepath.Join(filepath.Dir(h.home), "logs"),
		"--server", fmt.Sprintf("http://127.0.0.1:%d", port),
		"run", "--named", "@you/goal", "--with-mock-workers=" + mockWorkersPath, "--no-record",
		"--session", uuid.NewString(),
	}
	args = append(args, extra...)
	return args
}

func goalMockWorkers(t testing.TB, runType workers.MockWorkerRunType, passthrough bool) string {
	t.Helper()
	config := workers.MockWorkersConfig{}
	if passthrough {
		config.UnmatchedDispatchPolicy = workers.MockWorkerUnmatchedDispatchPolicyPassthrough
	}
	for _, pair := range [][2]string{
		{"goal-planner", "plan-goal"}, {"goal-executor", "execute-goal"},
		{"goal-checker", "check-goal"}, {"goal-reviewer", "review-goal"},
	} {
		config.MockWorkers = append(config.MockWorkers, workers.MockWorkerConfig{
			WorkerName: pair[0], WorkstationName: pair[1], RunType: runType,
		})
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal goal mock workers: %v", err)
	}
	path := filepath.Join(t.TempDir(), "goal-mock-workers.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write goal mock workers: %v", err)
	}
	return path
}

func testProcessCLISuccessWritesPrimaryResultOnlyToStdout(t *testing.T) {
	t.Parallel()
	process := plainProcess(t)
	home := configuredGoalHome(t, process)
	args := home.goalRunArgs(t, goalMockWorkers(t, workers.MockWorkerRunTypeAccept, true), "--quiet", "success-stdout-purity")
	result := home.run(t, process, nil, args...)
	assertQuietSuccess(t, result)
	for _, forbidden := range []string{"error:", "Error:", "traceId:", "requestId:"} {
		if strings.Contains(result.Stdout, forbidden) {
			t.Fatalf("stdout mixed diagnostic noise %q into primary result:\n%s", forbidden, result.Stdout)
		}
	}
	assertNoActivationMarkers(t, "success stdout", result.Stdout)
}

func testProcessCLIFailureWritesDiagnosticToStderr(t *testing.T) {
	t.Parallel()
	process := plainProcess(t)
	home := configuredGoalHome(t, process)
	args := home.goalRunArgs(t, goalMockWorkers(t, workers.MockWorkerRunTypeReject, false), "--quiet", "failure-stderr")
	result := home.run(t, process, nil, args...)
	if result.Err == nil {
		t.Fatalf("terminal worker failure succeeded: %#v", result)
	}
	if strings.TrimSpace(result.Stderr) == "" {
		t.Fatal("terminal worker failure stderr was empty; want actionable diagnostic")
	}
	if diagnostic := result.Stderr + "\n" + result.Err.Error(); !strings.Contains(diagnostic, "INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("terminal worker failure diagnostic missing INVOCATION_RUNTIME_FAILURE:\n%s", diagnostic)
	}
	if strings.Contains(result.Stderr, "Factory initiated:") || strings.Contains(result.Stderr, "Dashboard URL:") {
		t.Fatalf("stderr mixed lifecycle chatter into diagnostic stream:\n%s", result.Stderr)
	}
	if strings.TrimSpace(result.Stdout) != "" {
		t.Fatalf("stdout = %q, want no false primary-result payload", result.Stdout)
	}
}

func testProcessCLIQuietModeSuppressesNonResultNoise(t *testing.T) {
	t.Parallel()
	process := plainProcess(t)
	accept := goalMockWorkers(t, workers.MockWorkerRunTypeAccept, true)
	reject := goalMockWorkers(t, workers.MockWorkerRunTypeReject, false)

	t.Run("success suppresses stdout lifecycle presentation", func(t *testing.T) {
		t.Parallel()
		streamHome := configuredGoalHome(t, process)
		stream := streamHome.run(t, process, nil, streamHome.goalRunArgs(t, accept, "--output", "response-stream", "quiet-mode-stream-baseline")...)
		if stream.Err != nil {
			t.Fatalf("response-stream success failed: %v\nstdout:\n%s\nstderr:\n%s", stream.Err, stream.Stdout, stream.Stderr)
		}
		if !strings.Contains(stream.Stdout, quietPrimaryResultSeparator) || !containsHumanLifecycleNoise(stream.Stdout) || stream.Stdout == successStdoutPrimaryResult {
			t.Fatalf("response-stream stdout = %q, want lifecycle presentation", stream.Stdout)
		}
		quietHome := configuredGoalHome(t, process)
		quiet := quietHome.run(t, process, nil, quietHome.goalRunArgs(t, accept, "--quiet", "quiet-mode-success")...)
		assertQuietSuccess(t, quiet)
		for _, forbidden := range []string{quietPrimaryResultSeparator, "factory started", "work accepted"} {
			if strings.Contains(quiet.Stdout, forbidden) {
				t.Fatalf("quiet stdout leaked non-result noise %q:\n%s", forbidden, quiet.Stdout)
			}
		}
	})
	t.Run("success suppresses verbose stderr operator logs", func(t *testing.T) {
		t.Parallel()
		verboseHome := configuredGoalHome(t, process)
		verbose := verboseHome.run(t, process, nil, verboseHome.goalRunArgs(t, accept, "--verbose", "quiet-mode-verbose-baseline")...)
		if verbose.Err != nil || !strings.HasSuffix(verbose.Stdout, successStdoutPrimaryResult) {
			t.Fatalf("verbose result=%#v, want successful primary-result suffix", verbose)
		}
		quietHome := configuredGoalHome(t, process)
		quiet := quietHome.run(t, process, nil, quietHome.goalRunArgs(t, accept, "--quiet", "quiet-mode-verbose-contrast")...)
		assertQuietSuccess(t, quiet)
	})
	t.Run("failure keeps quiet stdout script-safe", func(t *testing.T) {
		t.Parallel()
		home := configuredGoalHome(t, process)
		result := home.run(t, process, nil, home.goalRunArgs(t, reject, "--quiet", "quiet-mode-failure")...)
		if result.Err == nil || strings.TrimSpace(result.Stdout) != "" || strings.TrimSpace(result.Stderr) == "" {
			t.Fatalf("quiet failure result=%#v, want error, empty stdout, diagnostic stderr", result)
		}
	})
}

func assertQuietSuccess(t testing.TB, result cliResult) {
	t.Helper()
	if result.Err != nil || result.Stdout != successStdoutPrimaryResult || strings.TrimSpace(result.Stderr) != "" {
		t.Fatalf("quiet success result=%#v, want exact primary stdout and empty stderr", result)
	}
}

func containsHumanLifecycleNoise(stdout string) bool {
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == quietPrimaryResultSeparator || line == successStdoutPrimaryResult {
			continue
		}
		closingBracket := strings.Index(line, "] ")
		if !strings.HasPrefix(line, "[") || closingBracket < 2 {
			continue
		}
		message := line[closingBracket+2:]
		for _, prefix := range []string{"work accepted", "work moved", "factory started", "factory completed", "workstation queued", "workstation started", "workstation completed", "final output updated"} {
			if strings.HasPrefix(message, prefix) {
				return true
			}
		}
	}
	return false
}

// ---- stdin consumption ----

func writeStdinRunFactory(t testing.TB, workDir string) string {
	t.Helper()
	factoryDir := filepath.Join(workDir, "stdin-run-factory")
	workstationPath := filepath.Join(factoryDir, "workstations", "process-prompt", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(workstationPath), 0o755); err != nil {
		t.Fatalf("create stdin run factory directory: %v", err)
	}
	factoryJSON := fmt.Sprintf(`{
  "name": %q,
  "workTypes": [{"name": "prompt-task", "handlingBehavior": ["DEFAULT"], "states": [
    {"name": "init", "type": "INITIAL"}, {"name": "complete", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}]}],
  "workers": [{"name": "mock-worker"}],
  "workstations": [{"name": "process-prompt", "worker": "mock-worker",
    "inputs": [{"workType": "prompt-task", "state": "init"}],
    "outputs": [{"workType": "prompt-task", "state": "complete"}],
    "onFailure": [{"workType": "prompt-task", "state": "failed"}]}]
}`, stdinPromptFactoryConfigName)
	factoryPath := filepath.Join(factoryDir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(factoryJSON), 0o600); err != nil {
		t.Fatalf("write stdin run factory.json: %v", err)
	}
	workstation := "---\ntype: MODEL_WORKSTATION\n---\nProcess {{ (index .Inputs 0).Payload }}.\n"
	if err := os.WriteFile(workstationPath, []byte(workstation), 0o644); err != nil {
		t.Fatalf("write stdin run workstation config: %v", err)
	}
	return factoryPath
}

func writeStdinRunMockWorkers(t testing.TB) string {
	t.Helper()
	data, err := json.MarshalIndent(workers.NewEmptyMockWorkersConfig(), "", "  ")
	if err != nil {
		t.Fatalf("marshal stdin run mock workers: %v", err)
	}
	path := filepath.Join(t.TempDir(), "stdin-run-mock-workers.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write stdin run mock workers: %v", err)
	}
	return path
}

func stdinRunArgs(t testing.TB, home cliHome) []string {
	t.Helper()
	port, err := builtcliacceptance.ReserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve stdin run listener: %v", err)
	}
	return []string{
		"--runtime-log-dir", filepath.Join(filepath.Dir(home.home), "logs"),
		"--server", fmt.Sprintf("http://127.0.0.1:%d", port),
		"run", "--factory", writeStdinRunFactory(t, home.work), "--session", uuid.NewString(),
		"--with-mock-workers", writeStdinRunMockWorkers(t),
		"--no-record", "--quiet", "-",
	}
}

func testProcessRunReadsPromptFromStdin(t *testing.T) {
	t.Parallel()
	process := plainProcess(t)
	home := newCLIHome(t)
	prompt := "functional-stdin-run-café-résumé-" + filepath.Base(t.TempDir())
	result := home.run(t, process, strings.NewReader(prompt), stdinRunArgs(t, home)...)
	if result.Err != nil {
		t.Fatalf("you run - via stdin: %v\nstdout:\n%s\nstderr:\n%s", result.Err, result.Stdout, result.Stderr)
	}
	if result.Stdout != prompt {
		t.Fatalf("worker-bound primary result = %q, want exact stdin prompt %q", result.Stdout, prompt)
	}
}

func testProcessCLIEmptyRequiredStdinFailsWithoutDispatch(t *testing.T) {
	t.Parallel()
	process := plainProcess(t)
	home := newCLIHome(t)
	result := home.run(t, process, strings.NewReader(""), stdinRunArgs(t, home)...)
	if result.Err == nil {
		t.Fatalf("you run - with empty stdin succeeded; want pre-dispatch rejection\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
	if strings.TrimSpace(result.Stdout) != "" {
		t.Fatalf("stdout = %q, want empty (no worker-bound primary result before stdin rejection)", result.Stdout)
	}
	if diagnostic := result.Stderr + "\n" + result.Err.Error(); !strings.Contains(diagnostic, "INVOCATION_INPUT_EMPTY") {
		t.Fatalf("empty stdin diagnostic missing INVOCATION_INPUT_EMPTY:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}

const (
	stdinSubmitBatchRequestID = "functional-stdin-submit-batch"
	stdinSubmitBatchWorkName  = "stdin-batch-task"
	stdinSubmitBatchWorkType  = "task"
)

// TestSubmitBatchReadsJSONFromStdin proves `you submit batch -` consumes one
// canonical FACTORY_REQUEST_BATCH document from stdin and acknowledges the
// accepted Work against a live in-process server.
func testProcessSubmitBatchReadsJSONFromStdin(t *testing.T) {
	t.Parallel()
	factoryDir := support.ScaffoldFactory(t, map[string]any{
		"name": "stdin-submit-batch",
		"workTypes": []map[string]any{{
			"name": stdinSubmitBatchWorkType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "processor",
			"inputs":    []map[string]string{{"workType": stdinSubmitBatchWorkType, "state": "init"}},
			"outputs":   []map[string]string{{"workType": stdinSubmitBatchWorkType, "state": "complete"}},
			"onFailure": []map[string]string{{"workType": stdinSubmitBatchWorkType, "state": "failed"}},
		}},
	})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	batchJSON := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Stdin submit batch process"}}]}`,
		stdinSubmitBatchRequestID, stdinSubmitBatchWorkName, stdinSubmitBatchWorkType,
	)
	home := newCLIHome(t)
	inputs := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "submit", "batch", "-"})
	inputs.Input.Env = builtcliacceptance.ProcessEnvForIsolatedHome(home.home)
	inputs.Input.WorkingDirectory = home.work
	inputs.Input.Stdin = strings.NewReader(batchJSON)
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("you submit batch - via stdin: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	for _, marker := range []string{
		"requestId: " + stdinSubmitBatchRequestID,
		"traceId:",
		"work count: 1",
		stdinSubmitBatchWorkName + " (" + stdinSubmitBatchWorkType + ")",
	} {
		if !strings.Contains(inputs.Stdout(), marker) {
			t.Fatalf("submit batch output missing %q:\n%s", marker, inputs.Stdout())
		}
	}
}
