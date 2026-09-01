package lifecycle_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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

type hostedServerAttachedObservation struct {
	baseURL     string
	session     factoryapi.FactorySession
	workID      string
	workVisible bool
	err         error
}

// TestCLIRunCleanInvocationCompletesWithoutDashboardStartup proves a
// clean/prompt-style public you run invocation completes with only the Factory
// primary result on stdout and does not emit dashboard open or startup sidecar
// output on that primary-result stream.
func TestCLIRunCleanInvocationCompletesWithoutDashboardStartup(t *testing.T) {
	t.Parallel()

	factoryDir := scaffoldProviderBackedFactory(t)
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		"--quiet",
		"prove workers-owned clean invocation lifecycle",
	}
	inputs, err := executeSharedLifecycleInvocation(
		t,
		args,
		support.NewStaticSuccessCommandRunner(wantCleanInvocationPrimaryResult),
	)
	if err != nil {
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
	args := []string{
		"you", "run", "--factory", factoryPath,
		"--provider", "codex", "--worker-reasoning-effort", "xhigh",
		"--to-file", promptPath, "--no-record", "--quiet",
	}
	inputs, err := executeSharedLifecycleInvocation(t, args, runner)
	if err != nil {
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
	assertFilePromptConflicts(t, factoryPath, promptPath, args)
	assertUnreadableFilePrompt(t, factoryPath, promptDir)
}

func assertFilePromptConflicts(t *testing.T, factoryPath, promptPath string, fileArgs []string) {
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
			inputs := newSharedLifecycleInputs(t, test.args)
			inputs.Input.Stdin = strings.NewReader(test.stdin)
			inputs.Input.StdinIsTTY = &test.stdinIsTTY
			err := executeSharedLifecycleInputs(t, inputs, runner)
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

func assertUnreadableFilePrompt(t *testing.T, factoryPath, promptDir string) {
	t.Helper()
	missingPath := filepath.Join(promptDir, "missing prompt.txt")
	runner := support.NewShapedProviderCommandRunner()
	missingArgs := []string{
		"you", "run", "--factory", factoryPath,
		"--to-file", missingPath, "--no-record", "--quiet",
	}
	_, err := executeSharedLifecycleInvocation(t, missingArgs, runner)
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
	_, err = executeSharedLifecycleInvocation(t, directoryArgs, runner)
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
	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--provider", "codex",
		"--worker-reasoning-effort", " XHIGH ",
		"--no-record",
		"--quiet",
		"prove the explicit reasoning effort path",
	}
	inputs, err := executeSharedLifecycleInvocation(t, args, runner)
	if err != nil {
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
	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--provider", "codex",
		"--worker-reasoning-effort", "turbo",
		"--no-record",
		"--quiet",
		"reject the unsupported reasoning effort",
	}
	_, err := executeSharedLifecycleInvocation(t, args, runner)
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

	// The worker dispatch is held open until the lifecycle coordinator has
	// deterministically observed the hosted Factory Session at least once.
	// Without this gate, a mocked worker that returns instantly races the
	// invocation's own completion (which cancels and tears down the hosted
	// API listener) against the polling goroutine's first real HTTP round
	// trip, and the listener side of that race wins essentially every time.
	sessionObservedGate := newLifecycleGate("provider release")
	hostedServerEdges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(
		t,
		&hostedServerEdges,
		support.NewGatedSuccessCommandRunner(wantServerAttachedInvocationPrimaryResult, sessionObservedGate.channel()),
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
	workObservedGate := newLifecycleGate("listener release")
	correlationDone := newLifecycleGate("public correlation")
	hostedAPI := support.NewProcessAPIServer()
	hostedAPI.HoldShutdownUntilSignaled(workObservedGate.channel())
	hostedServerEdges.APIServerStarter = hostedAPI.Start
	hostedProcess := buildLifecycleProcess(t, hostedServerEdges)
	hostedProcess.TrackGate(sessionObservedGate)
	hostedProcess.TrackGate(workObservedGate)
	hostedProcess.TrackGate(correlationDone)

	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--with-server",
		"--no-record",
		"--quiet",
		"prove workers-owned server-attached lifecycle",
	}
	inputs := hostedProcess.Inputs(args, factoryDir)
	command := hostedProcess.StartCommand(inputs)
	releaseWorker := func() {
		hostedProcess.ReleaseGate(sessionObservedGate, lifecyclePhaseProviderRelease, "Factory Session observation")
	}
	releaseShutdown := func() {
		hostedProcess.ReleaseGate(workObservedGate, lifecyclePhaseTerminal, "public lifecycle correlation")
	}

	observationsReady := make(chan hostedServerAttachedObservation, 1)
	go func() {
		// Always release both lifecycle gates before this goroutine exits, even
		// on a phase failure, so Process.Execute and cleanup cannot be stranded.
		defer releaseWorker()
		defer releaseShutdown()

		baseURL, err := hostedProcess.WaitForReadiness(hostedAPI)
		if err != nil {
			observationsReady <- hostedServerAttachedObservation{err: err}
			return
		}
		session, workID, workVisible, pollErr := hostedProcess.ObserveHostedServerAttached(
			baseURL,
			wantServerAttachedInvocationPrimaryResult,
			releaseWorker,
			command.Done(),
		)
		observationsReady <- hostedServerAttachedObservation{
			baseURL:     baseURL,
			session:     session,
			workID:      workID,
			workVisible: workVisible,
			err:         pollErr,
		}
		if pollErr == nil {
			<-correlationDone.channel()
		}
	}()

	observation := <-observationsReady
	if observation.err != nil {
		t.Fatal(observation.err)
	}
	assertHostedServerAttachedPublicCorrelation(
		t,
		observation.baseURL,
		factorysessions.DefaultSessionID,
		observation.workID,
	)
	hostedProcess.ReleaseGate(correlationDone, lifecyclePhaseTerminal, "public Factory Session, Worker Session, Work, and Event correlation")
	if err := hostedProcess.WaitCommand(command); err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s",
			args,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
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
	args := []string{
		"you", "run",
		"--factory", factoryPath,
		"--no-record",
		"--quiet",
		"prove workers-owned clean invocation failure lifecycle",
	}
	inputs, executeErr := executeSharedLifecycleInvocation(t, args, runner)
	if executeErr == nil {
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

	t.Run("adverse lifecycle matrix", runLifecycleAdverseMatrix)
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
	process := buildLifecycleProcess(t, clientEdges)
	inputs := process.Inputs([]string{
		"you", "--server", continuousBaseURL,
		"run",
		"--factory", factoryPath,
		"--no-record",
		"prove detached server preference cannot attach",
	}, factoryDir)
	if err := process.Execute(inputs); err == nil {
		t.Fatalf(
			"Process.Execute(detached --server run) unexpectedly succeeded; want failure when client provider edges are isolated from the continuous host\nstdout:\n%s\nstderr:\n%s",
			inputs.Stdout(),
			inputs.Stderr(),
		)
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
	response, err := lifecycleHTTPClient.Get(endpoint)
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

func assertHostedServerAttachedPublicCorrelation(
	t *testing.T,
	baseURL string,
	sessionID string,
	workID string,
) {
	t.Helper()

	workerSessions := support.ListDefaultSessionWorkerSessions(t, baseURL, workID)
	if len(workerSessions.Sessions) != 1 {
		t.Fatalf("public Worker Sessions for Work %q = %#v, want exactly one", workID, workerSessions.Sessions)
	}
	worker := workerSessions.Sessions[0]
	if strings.TrimSpace(worker.WorkerSessionId) == "" {
		t.Fatalf("public Worker Session for Work %q has empty identity: %#v", workID, worker)
	}
	if worker.State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("public Worker Session %q state = %q, want COMPLETED", worker.WorkerSessionId, worker.State)
	}
	if worker.FactorySessionId == nil || *worker.FactorySessionId != sessionID {
		t.Fatalf("public Worker Session %q Factory Session = %#v, want %q", worker.WorkerSessionId, worker.FactorySessionId, sessionID)
	}
	if worker.WorkId == nil || *worker.WorkId != workID || !containsString(worker.WorkIds, workID) {
		t.Fatalf("public Worker Session %q Work correlation = workId:%#v workIds:%#v, want %q", worker.WorkerSessionId, worker.WorkId, worker.WorkIds, workID)
	}

	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	dispatches := support.ObserveDispatchEvents(t, events)
	assertHostedServerAttachedFactoryEvents(t, events, dispatches, sessionID, workID, worker.WorkerSessionId)
}

func assertHostedServerAttachedFactoryEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	dispatches []support.DispatchEventObservation,
	sessionID, workID, workerSessionID string,
) string {
	t.Helper()

	var matching *support.DispatchEventObservation
	for index := range dispatches {
		if !support.DispatchObservationIncludesWork(dispatches[index], workID) {
			continue
		}
		if matching != nil {
			t.Fatalf("public dispatch observations for Work %q contain more than one dispatch", workID)
		}
		matching = &dispatches[index]
	}
	if matching == nil || matching.Response == nil {
		t.Fatalf("public Factory Events contain no completed dispatch for Work %q: %#v", workID, dispatches)
	}
	if matching.Response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("public dispatch %q outcome = %q, want ACCEPTED", matching.DispatchID, matching.Response.Outcome)
	}
	if matching.StartedAt.IsZero() || matching.CompletedAt.IsZero() || matching.StartedAt.After(matching.CompletedAt) {
		t.Fatalf("public dispatch %q event times = %s -> %s, want ordered non-zero times", matching.DispatchID, matching.StartedAt, matching.CompletedAt)
	}

	requestIndex, responseIndex, associationIndex := -1, -1, -1
	for index, event := range events {
		if event.Context.DispatchId == nil || *event.Context.DispatchId != matching.DispatchID {
			continue
		}
		if event.Id == "" {
			t.Fatalf("public Factory Event at index %d for dispatch %q has empty identity", index, matching.DispatchID)
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != sessionID {
			t.Fatalf("public Factory Event %q session = %#v, want %q", event.Id, event.Context.SessionId, sessionID)
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			requestIndex = index
		case factoryapi.FactoryEventTypeDispatchResponse:
			responseIndex = index
		case factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation:
			associationIndex = index
			association, err := event.Payload.AsDispatchWorkerSessionAssociationEventPayload()
			if err != nil {
				t.Fatalf("decode public Worker Session association event %q: %v", event.Id, err)
			}
			if association.WorkerSessionId != workerSessionID {
				t.Fatalf("public dispatch %q Worker Session association = %q, want %q", matching.DispatchID, association.WorkerSessionId, workerSessionID)
			}
		}
	}
	if requestIndex < 0 || responseIndex < 0 || associationIndex < 0 {
		t.Fatalf("public Factory Events for dispatch %q missing request/association/response: request=%d association=%d response=%d", matching.DispatchID, requestIndex, associationIndex, responseIndex)
	}
	if requestIndex >= associationIndex || associationIndex >= responseIndex {
		t.Fatalf("public Factory Event order for dispatch %q = request:%d association:%d response:%d, want request < association < response", matching.DispatchID, requestIndex, associationIndex, responseIndex)
	}
	return matching.DispatchID
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func tryReadTerminalWorkPrimaryText(serverURL, wantText string) (string, bool, string) {
	endpoint := support.DefaultSessionWorkURL(serverURL, "/work")
	response, err := lifecycleHTTPClient.Get(endpoint)
	if err != nil {
		return "", false, fmt.Sprintf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		return "", false, fmt.Sprintf(
			"GET %s status = %d: %s",
			endpoint,
			response.StatusCode,
			strings.TrimSpace(string(payload)),
		)
	}
	var listed factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		return "", false, fmt.Sprintf("decode GET %s: %v", endpoint, err)
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
		if part.Text == wantText && item.WorkId != nil && strings.TrimSpace(*item.WorkId) != "" {
			return *item.WorkId, true, ""
		}
	}
	return "", false, fmt.Sprintf("listed work missing terminal primary text %q: %#v", wantText, listed.Results)
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
