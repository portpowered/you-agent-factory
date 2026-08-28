package lifecycle_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	wantCleanInvocationPrimaryResult          = "deterministic workers lifecycle primary COMPLETE"
	wantServerAttachedInvocationPrimaryResult = "deterministic workers lifecycle server-attached COMPLETE"
	deterministicProviderFailureExit          = 7
	deterministicProviderFailureStderr        = "deterministic provider rejection"
)

var cleanInvocationForbiddenOperatorChatter = []string{
	"Factory initiated",
	"Dashboard URL",
	"Runtime log",
	"Opening dashboard",
	"Recording saved to",
	"Factory:",
}

// TestCLIRunCleanInvocationCompletesWithoutDashboardStartup proves a
// clean/prompt-style public you run invocation completes with only the Factory
// primary result on stdout and does not emit dashboard open or startup sidecar
// output on that primary-result stream.
func TestCLIRunCleanInvocationCompletesWithoutDashboardStartup(t *testing.T) {
	t.Parallel()

	factoryDir := scaffoldProviderBackedFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(
		t,
		&edges,
		support.NewStaticSuccessCommandRunner(wantCleanInvocationPrimaryResult),
		nil,
	)

	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		"--quiet",
		"prove workers-owned clean invocation lifecycle",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s",
			args,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	stdout := strings.TrimSuffix(inputs.Stdout(), "\n")
	if stdout != wantCleanInvocationPrimaryResult {
		t.Fatalf("stdout = %q, want exact primary clean invocation output %q", stdout, wantCleanInvocationPrimaryResult)
	}
	assertCleanInvocationStdoutFreeOfOperatorChatter(t, stdout)
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", inputs.Stderr())
	}
}

// TestCLIRunToFilePreservesExactPromptAndRejectsBeforeProviderDispatch proves
// the public one-shot run path carries a real multiline UTF-8 file and the
// canonical xhigh effort through one injected Codex command edge unchanged,
// while file conflicts and unreadable paths fail before any runtime/provider
// effect is activated.
func TestCLIRunToFilePreservesExactPromptAndRejectsBeforeProviderDispatch(t *testing.T) {
	t.Parallel()

	factoryDir := scaffoldProviderBackedFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	support.WriteWorkstationConfig(t, factoryDir, "process", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}")

	promptDir := filepath.Join(factoryDir, "prompt fixtures")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("create prompt fixture directory: %v", err)
	}
	promptPath := filepath.Join(promptDir, "long prompt.txt")
	var promptBuilder strings.Builder
	for line := 1; line <= 30; line++ {
		fmt.Fprintf(&promptBuilder, "line %02d — 東京\r\n", line)
	}
	wantPrompt := promptBuilder.String()
	if err := os.WriteFile(promptPath, []byte(wantPrompt), 0o600); err != nil {
		t.Fatalf("write prompt fixture: %v", err)
	}

	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("file prompt accepted COMPLETE"),
	})
	edges := serviceedges.Edges{
		ProviderCommandRunner:          runner,
		WorkSubmittedFileReader:        os.ReadFile,
		WorkSubmittedFilePathInspector: os.Stat,
	}
	args := []string{
		"you", "run", "--factory", factoryPath,
		"--provider", "codex", "--worker-reasoning-effort", "xhigh",
		"--to-file", promptPath, "--no-record", "--quiet",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want exactly one file-backed dispatch", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != string(modelprovider.ProviderCodex) {
		t.Fatalf("provider command = %q, want %q", request.Command, modelprovider.ProviderCodex)
	}
	if got := string(request.Stdin); got != wantPrompt {
		t.Fatalf("provider stdin = %q, want exact 30-line prompt %q", got, wantPrompt)
	}
	support.AssertArgsContainSequence(t, request.Args, []string{
		"--config", `model_reasoning_effort="xhigh"`,
	})
	assertFilePromptConflicts(t, factoryDir, factoryPath, promptPath, args)
	assertUnreadableFilePrompt(t, factoryDir, factoryPath, promptDir)
}

func assertFilePromptConflicts(t *testing.T, factoryDir, factoryPath, promptPath string, fileArgs []string) {
	t.Helper()
	for _, test := range []struct {
		name        string
		args        []string
		stdin       string
		stdinIsTTY  bool
		wantSources []string
	}{
		{
			name:        "file and positional",
			args:        []string{"you", "run", "--factory", factoryPath, "also positional", "--to-file", promptPath, "--no-record", "--quiet"},
			stdinIsTTY:  true,
			wantSources: []string{"file_text", "positional_text"},
		},
		{
			name:        "file and supplied stdin",
			args:        fileArgs,
			stdin:       "supplied stdin",
			stdinIsTTY:  false,
			wantSources: []string{"file_text", "stdin_text"},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			runner := support.NewShapedProviderCommandRunner()
			inputs := support.FakeInputs(t.Context(), test.args)
			inputs.Input.WorkingDirectory = factoryDir
			inputs.Input.Stdin = strings.NewReader(test.stdin)
			inputs.Input.StdinIsTTY = &test.stdinIsTTY
			process := support.BuildProcess(t, serviceedges.Edges{
				ProviderCommandRunner:          runner,
				WorkSubmittedFileReader:        os.ReadFile,
				WorkSubmittedFilePathInspector: os.Stat,
			})
			support.CleanupProcess(t, process)
			err := process.Execute(inputs.Input)
			if err == nil || !strings.Contains(err.Error(), "INVOCATION_INPUT_SOURCE_CONFLICT") {
				t.Fatalf("Process.Execute(%v) error = %v, want stable source conflict", test.args, err)
			}
			for _, source := range test.wantSources {
				if !strings.Contains(err.Error(), source) {
					t.Fatalf("error = %v, want source %q", err, source)
				}
			}
			if runner.CallCount() != 0 {
				t.Fatalf("provider command runner calls = %d, want zero for %s conflict", runner.CallCount(), test.name)
			}
		})
	}
}

func assertUnreadableFilePrompt(t *testing.T, factoryDir, factoryPath, promptDir string) {
	t.Helper()
	missingPath := filepath.Join(promptDir, "missing prompt.txt")
	runner := support.NewShapedProviderCommandRunner()
	missingArgs := []string{
		"you", "run", "--factory", factoryPath,
		"--to-file", missingPath, "--no-record", "--quiet",
	}
	inputs := support.FakeInputs(t.Context(), missingArgs)
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner:          runner,
		WorkSubmittedFileReader:        os.ReadFile,
		WorkSubmittedFilePathInspector: os.Stat,
	})
	support.CleanupProcess(t, process)
	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), missingPath) {
		t.Fatalf("Process.Execute(%v) error = %v, want unreadable path diagnostic", missingArgs, err)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want zero for unreadable file", runner.CallCount())
	}

	directoryArgs := []string{
		"you", "run", "--factory", factoryPath,
		"--to-file", promptDir, "--no-record", "--quiet",
	}
	directoryInputs := support.FakeInputs(t.Context(), directoryArgs)
	directoryInputs.Input.WorkingDirectory = factoryDir
	directoryProcess := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner:          runner,
		WorkSubmittedFileReader:        os.ReadFile,
		WorkSubmittedFilePathInspector: os.Stat,
	})
	support.CleanupProcess(t, directoryProcess)
	err = directoryProcess.Execute(directoryInputs.Input)
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("Process.Execute(%v) error = %v, want non-regular source diagnostic", directoryArgs, err)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want zero for non-regular file", runner.CallCount())
	}
}

// TestCLIRunWorkerReasoningEffortOverrideReachesCodexCommand proves the
// run-scoped effort override crosses the public root process and reaches the
// deterministic Codex command edge in canonical form.
func TestCLIRunWorkerReasoningEffortOverrideReachesCodexCommand(t *testing.T) {
	t.Parallel()

	factoryDir := scaffoldProviderBackedFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("reasoning effort override COMPLETE"),
	})
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, runner, nil)

	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--provider", "codex",
		"--worker-reasoning-effort", " XHIGH ",
		"--no-record",
		"--quiet",
		"prove the explicit reasoning effort path",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}

	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want one dispatch", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "codex" {
		t.Fatalf("provider command = %q, want codex", request.Command)
	}
	support.AssertArgsContainSequence(t, request.Args, []string{
		"--config", `model_reasoning_effort="xhigh"`,
	})
}

// TestCLIRunUnsupportedWorkerReasoningEffortRejectsBeforeProviderDispatch
// proves invalid run input fails at the public command boundary before the
// injected provider effect can be invoked.
func TestCLIRunUnsupportedWorkerReasoningEffortRejectsBeforeProviderDispatch(t *testing.T) {
	t.Parallel()

	factoryDir := scaffoldProviderBackedFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("must not dispatch"),
	})
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, runner, nil)

	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--provider", "codex",
		"--worker-reasoning-effort", "turbo",
		"--no-record",
		"--quiet",
		"reject the unsupported reasoning effort",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), `invalid --worker-reasoning-effort "turbo"`) {
		t.Fatalf("Process.Execute(%v) error = %v, want actionable effort validation", args, err)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want zero for invalid effort", runner.CallCount())
	}
}

// TestCLIRunServerAttachedInvocationTargetsExistingFactorySession proves a
// hosted public you run --with-server invocation routes through the already-open
// Factory Session on the live runtime host rather than a detached local one-shot
// lifecycle, with Factory Event correlation on that hosted session identity.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestCLIRunServerAttachedInvocationTargetsExistingFactorySession(t *testing.T) {
	t.Parallel()

	factoryDir := scaffoldProviderBackedFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	// The worker dispatch is held open until the polling goroutine below has
	// deterministically observed the hosted Factory Session at least once.
	// Without this gate, a mocked worker that returns instantly races the
	// invocation's own completion (which cancels and tears down the hosted
	// API listener) against the polling goroutine's first real HTTP round
	// trip, and the listener side of that race wins essentially every time.
	sessionObservedGate := make(chan struct{})
	hostedServerEdges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(
		t,
		&hostedServerEdges,
		support.NewGatedSuccessCommandRunner(wantServerAttachedInvocationPrimaryResult, sessionObservedGate),
		nil,
	)
	continuousHost := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges:                     hostedServerEdges,
	})
	defer continuousHost.Stop(t)

	continuousBaseURL := continuousHost.URL()
	assertDetachedServerPrefRunCannotAttachToContinuousHost(
		t,
		factoryDir,
		factoryPath,
		continuousBaseURL,
	)

	// The hosted API listener is likewise held open, once the invocation asks
	// it to shut down, until the polling goroutine below has deterministically
	// observed terminal Work at least once. Without this gate, the invocation
	// completing (which cancels and tears down the listener) structurally
	// outraces the polling goroutine's next HTTP round trip, making terminal
	// Work observability a best-effort race rather than a guarantee.
	workObservedGate := make(chan struct{})
	hostedAPI := support.NewProcessAPIServer()
	hostedAPI.HoldShutdownUntilSignaled(workObservedGate)
	hostedServerEdges.APIServerStarter = hostedAPI.Start
	hostedProcess := support.BuildProcess(t, hostedServerEdges)
	support.CleanupProcess(t, hostedProcess)

	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--with-server",
		"--no-record",
		"--quiet",
		"prove workers-owned server-attached lifecycle",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = factoryDir
	command := support.StartProcessCommand(t, hostedProcess, inputs.Input)

	observationsReady := make(chan struct {
		session     factoryapi.FactorySession
		workVisible bool
		err         error
	}, 1)
	go func() {
		var releaseWorkerOnce, releaseShutdownOnce sync.Once
		releaseWorker := func() { releaseWorkerOnce.Do(func() { close(sessionObservedGate) }) }
		releaseShutdown := func() { releaseShutdownOnce.Do(func() { close(workObservedGate) }) }
		// Always release both gates before this goroutine exits, even on a
		// WaitForBaseURL timeout, so an unexpected failure here cannot hang
		// Process.Execute (and this test's t.Cleanup teardown) forever.
		defer releaseWorker()
		defer releaseShutdown()

		baseURL, err := hostedAPI.WaitForBaseURL(5 * time.Second)
		if err != nil {
			observationsReady <- struct {
				session     factoryapi.FactorySession
				workVisible bool
				err         error
			}{err: err}
			return
		}
		session, workVisible, pollErr := pollHostedServerAttachedInvocationObservations(
			baseURL,
			wantServerAttachedInvocationPrimaryResult,
			releaseWorker,
			releaseShutdown,
			command.Done(),
		)
		observationsReady <- struct {
			session     factoryapi.FactorySession
			workVisible bool
			err         error
		}{session: session, workVisible: workVisible, err: pollErr}
	}()

	observation := <-observationsReady
	<-command.Done()
	if err := command.Err(); err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s",
			args,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if observation.err != nil {
		t.Logf("hosted server-attached session/work observations before host shutdown: %v", observation.err)
	}

	stdout := strings.TrimSuffix(inputs.Stdout(), "\n")
	if stdout != wantServerAttachedInvocationPrimaryResult {
		t.Fatalf("stdout = %q, want exact server-attached primary result %q", stdout, wantServerAttachedInvocationPrimaryResult)
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful server-attached stderr", inputs.Stderr())
	}

	if strings.TrimSpace(observation.session.Id) == "" {
		t.Fatal("hosted Factory Session id observed during invocation is empty, want observable session identity")
	}
	if !observation.workVisible {
		t.Fatal("terminal /work was not observable before hosted API shutdown, want observable terminal Work content")
	}
}

// TestCLIRunCleanInvocationFailurePreservesPublicError proves a failed
// clean/prompt-style public you run invocation exits unsuccessfully, writes the
// documented public error contract to stderr, and does not emit a false-success
// primary result on stdout.
func TestCLIRunCleanInvocationFailurePreservesPublicError(t *testing.T) {
	t.Parallel()

	factoryDir := scaffoldProviderBackedFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: deterministicProviderFailureExit,
		Stderr:   []byte(deterministicProviderFailureStderr),
	})
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, runner, nil)

	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		"--quiet",
		"prove workers-owned clean invocation failure lifecycle",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute error = nil, want terminal clean invocation failure")
	}

	stdout := strings.TrimSpace(inputs.Stdout())
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty clean failure stdout without false primary result", stdout)
	}
	if strings.Contains(stdout, wantCleanInvocationPrimaryResult) {
		t.Fatalf("stdout contains false-success primary result %q", wantCleanInvocationPrimaryResult)
	}

	errorResponse := decodeSingleErrorResponse(t, inputs.Stderr())
	if errorResponse.Code == "" || strings.TrimSpace(errorResponse.Message) == "" {
		t.Fatalf("ErrorResponse = %#v, want actionable code and message", errorResponse)
	}
	if errorResponse.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("ErrorResponse family = %q, want %q", errorResponse.Family, factoryapi.ErrorFamilyInternalServerError)
	}
}

func scaffoldProviderBackedFactory(t *testing.T) string {
	t.Helper()

	cfg := map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
			"handlingBehavior": []string{"DEFAULT"},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
	dir := support.ScaffoldFactory(t, cfg)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}

func assertDetachedServerPrefRunCannotAttachToContinuousHost(
	t *testing.T,
	factoryDir, factoryPath, continuousBaseURL string,
) {
	t.Helper()

	clientEdges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(
		t,
		&clientEdges,
		support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
			ExitCode: deterministicProviderFailureExit,
			Stderr:   []byte(deterministicProviderFailureStderr),
		}),
		nil,
	)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", continuousBaseURL,
		"run",
		"--factory", factoryPath,
		"--no-record",
		"prove detached server preference cannot attach",
	})
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, clientEdges)
	support.CleanupProcess(t, process)
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatalf(
			"Process.Execute(detached --server run) unexpectedly succeeded; want failure when client provider edges are isolated from the continuous host\nstdout:\n%s\nstderr:\n%s",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
}

func pollHostedServerAttachedInvocationObservations(
	baseURL, wantWorkText string,
	releaseWorker, releaseShutdown func(),
	done <-chan struct{},
) (factoryapi.FactorySession, bool, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var (
		sessionRead    bool
		workVisible    bool
		sessionDuring  factoryapi.FactorySession
		lastSessionErr string
	)

	for {
		if !sessionRead {
			if session, ok, diagnostic := tryReadDefaultFactorySession(baseURL); ok {
				sessionDuring = session
				sessionRead = true
				// The gated worker dispatch cannot return (and so the
				// invocation cannot complete and tear down the hosted API
				// listener) until this deterministic release fires, which
				// guarantees the session identity was observable while the
				// invocation was still active.
				releaseWorker()
			} else if diagnostic != "" {
				lastSessionErr = diagnostic
			}
		}
		if !workVisible {
			if ok, _ := tryReadTerminalWorkPrimaryText(baseURL, wantWorkText); ok {
				workVisible = true
				// The hosted API listener cannot close (and so the process
				// cannot finish) until this deterministic release fires,
				// which guarantees terminal Work was observable before host
				// shutdown.
				releaseShutdown()
			}
		}
		if sessionRead && workVisible {
			return sessionDuring, true, nil
		}

		select {
		case <-done:
			if !sessionRead {
				return factoryapi.FactorySession{}, workVisible, fmt.Errorf(
					"hosted CLI run finished before session identity was readable at %s: %s",
					baseURL,
					lastSessionErr,
				)
			}
			return sessionDuring, workVisible, nil
		default:
		}

		select {
		case <-done:
			if !sessionRead {
				return factoryapi.FactorySession{}, workVisible, fmt.Errorf(
					"hosted CLI run finished before session identity was readable at %s: %s",
					baseURL,
					lastSessionErr,
				)
			}
			return sessionDuring, workVisible, nil
		case <-ticker.C:
		}
	}
}

func tryReadDefaultFactorySession(baseURL string) (factoryapi.FactorySession, bool, string) {
	session, err := readDefaultFactorySession(baseURL)
	if err != nil {
		return factoryapi.FactorySession{}, false, err.Error()
	}
	return session, true, ""
}

func readDefaultFactorySession(baseURL string) (factoryapi.FactorySession, error) {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/~default"
	response, err := http.Get(endpoint)
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.FactorySession{}, fmt.Errorf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded factoryapi.FactorySessionGetResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return factoryapi.FactorySession{}, err
	}
	session, err := decoded.AsFactorySession()
	if err != nil {
		return factoryapi.FactorySession{}, err
	}
	if strings.TrimSpace(session.Id) == "" {
		return factoryapi.FactorySession{}, fmt.Errorf("GET %s returned empty session id", endpoint)
	}
	return session, nil
}

func tryReadTerminalWorkPrimaryText(serverURL, wantText string) (bool, string) {
	endpoint := support.DefaultSessionWorkURL(serverURL, "/work")
	response, err := http.Get(endpoint)
	if err != nil {
		return false, fmt.Sprintf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		return false, fmt.Sprintf(
			"GET %s status = %d: %s",
			endpoint,
			response.StatusCode,
			strings.TrimSpace(string(payload)),
		)
	}
	var listed factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		return false, fmt.Sprintf("decode GET %s: %v", endpoint, err)
	}
	for _, item := range listed.Results {
		if item.State == nil || item.State.Type != factoryapi.WorkStateTypeTERMINAL {
			continue
		}
		if item.Content == nil || len(*item.Content) != 1 {
			continue
		}
		part, err := (*item.Content)[0].AsWorkTextContentPart()
		if err != nil {
			continue
		}
		if part.Text == wantText {
			return true, ""
		}
	}
	return false, fmt.Sprintf("listed work missing terminal primary text %q: %#v", wantText, listed.Results)
}

func assertCleanInvocationStdoutFreeOfOperatorChatter(t *testing.T, stdout string) {
	t.Helper()

	for _, forbidden := range cleanInvocationForbiddenOperatorChatter {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout contains operator lifecycle chatter %q:\n%s", forbidden, stdout)
		}
	}
}

func decodeSingleErrorResponse(t *testing.T, stderr string) factoryapi.ErrorResponse {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stderr))
	var response factoryapi.ErrorResponse
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("decode ErrorResponse: %v\nstderr:\n%s", err, stderr)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("stderr contains data after ErrorResponse: %v\nstderr:\n%s", err, stderr)
	}
	return response
}
