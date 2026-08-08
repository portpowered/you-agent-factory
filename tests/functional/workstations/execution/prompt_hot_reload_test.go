package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	promptReloadWorkerName       = "processor"
	promptReloadWorkstationName  = "process"
	promptReloadOldWorkerPrompt  = "worker prompt OLD"
	promptReloadNewWorkerPrompt  = "worker prompt NEW"
	promptReloadOldStationPrompt = "workstation prompt OLD"
	promptReloadNewStationPrompt = "workstation prompt NEW"
)

// TestRunningSessionReloadsPromptOnNextDispatch proves that one continuously
// running Factory Session captures an authored worker and workstation prompt
// for each dispatch. The command-runner edge holds the first provider handoff
// after the prompt snapshot, so editing both AGENTS.md files before releasing
// it proves that in-flight work keeps the old values while the next Work item
// receives the new values.
func TestRunningSessionReloadsPromptOnNextDispatch(t *testing.T) {
	dir := support.ScaffoldSingleStepFactory(t, "dispatch-time-prompt-hot-reload")
	support.WriteAgentConfig(t, dir, promptReloadWorkerName, promptReloadWorkerConfig(promptReloadOldWorkerPrompt))
	support.WriteWorkstationConfig(t, dir, promptReloadWorkstationName, promptReloadWorkstationConfig(promptReloadOldStationPrompt))

	firstRelease := make(chan struct{})
	runner := newPromptReloadCommandRunner(firstRelease)
	server := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:      server.Start,
		ProviderCommandRunner: runner,
	})
	support.CleanupProcess(t, process)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir
	daemon := support.StartProcessCommand(t, process, inputs.Input)

	baseURL := server.WaitForURL(t)
	startedSession := showPromptReloadSession(t, process, baseURL, "")
	submitPromptReloadWork(t, process, baseURL, "first-work")

	firstRequest := runner.waitForFirstRequest(t)
	support.WriteAgentConfig(t, dir, promptReloadWorkerName, promptReloadWorkerConfig(promptReloadNewWorkerPrompt))
	support.WriteWorkstationConfig(t, dir, promptReloadWorkstationName, promptReloadWorkstationConfig(promptReloadNewStationPrompt))
	close(firstRelease)
	waitForPromptReloadSessionTerminal(t, process, baseURL, startedSession.Id)

	submitPromptReloadWork(t, process, baseURL, "second-work")
	secondRequest := runner.waitForSecondRequest(t)
	waitForPromptReloadSessionTerminal(t, process, baseURL, startedSession.Id)

	if got := len(runner.requestsSnapshot()); got != 2 {
		t.Fatalf("provider command requests = %d, want exactly two; requests=%#v", got, runner.requestsSnapshot())
	}
	assertClaudePromptRequest(t, firstRequest, promptReloadOldWorkerPrompt, promptReloadOldStationPrompt)
	assertClaudePromptRequest(t, secondRequest, promptReloadNewWorkerPrompt, promptReloadNewStationPrompt)

	finishedSession := showPromptReloadSession(t, process, baseURL, startedSession.Id)
	if finishedSession.Id != startedSession.Id {
		t.Fatalf("Factory Session ID changed across prompt reload dispatches: started=%q finished=%q", startedSession.Id, finishedSession.Id)
	}
	if daemon.Err() != nil {
		t.Fatalf("running Factory Session exited before the test completed: %v", daemon.Err())
	}
}

func promptReloadWorkerConfig(prompt string) string {
	return "---\n" +
		"type: MODEL_WORKER\n" +
		"model: test-claude-model\n" +
		"modelProvider: " + string(modelprovider.ProviderClaude) + "\n" +
		"stopToken: COMPLETE\n" +
		"---\n" + prompt + "\n"
}

func promptReloadWorkstationConfig(prompt string) string {
	return "---\n" +
		"type: MODEL_WORKSTATION\n" +
		"---\n" + prompt + "\n"
}

type promptReloadSubmitResult struct {
	EndpointPath string  `json:"endpointPath"`
	Name         string  `json:"name"`
	SessionID    string  `json:"sessionId"`
	WorkID       *string `json:"workId"`
	WorkTypeName string  `json:"workTypeName"`
}

func submitPromptReloadWork(t testing.TB, process support.Process, serverURL, name string) promptReloadSubmitResult {
	t.Helper()

	payload, err := json.Marshal(map[string]string{"title": name})
	if err != nil {
		t.Fatalf("marshal %s payload: %v", name, err)
	}
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--server", serverURL,
		"--json",
		"submit",
		"--name", name,
		"--work-type-name", "task",
		"--payload", "-",
	})
	inputs.Input.Stdin = strings.NewReader(string(payload))
	stdinIsTTY := false
	inputs.Input.StdinIsTTY = &stdinIsTTY
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(submit %s) error = %v\nstdout:\n%s\nstderr:\n%s", name, err, inputs.Stdout(), inputs.Stderr())
	}

	var result promptReloadSubmitResult
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatalf("decode CLI submit %s response: %v\nstdout:\n%s", name, err, inputs.Stdout())
	}
	if result.Name != name || result.WorkTypeName != "task" {
		t.Fatalf("CLI submit %s response = %#v, want accepted task work", name, result)
	}
	return result
}

func showPromptReloadSession(t testing.TB, process support.Process, serverURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	session, err := readPromptReloadSession(t, process, serverURL, sessionID)
	if err != nil {
		t.Fatalf("read Factory Session %q through CLI: %v", sessionID, err)
	}
	return session
}

func readPromptReloadSession(t testing.TB, process support.Process, serverURL, sessionID string) (factoryapi.FactorySession, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	args := []string{"you", "--server", serverURL, "--json", "session", "show"}
	if sessionID != "" {
		args = append(args, sessionID)
	}
	inputs := support.FakeInputs(ctx, args)
	if err := process.Execute(inputs.Input); err != nil {
		return factoryapi.FactorySession{}, fmt.Errorf("Process.Execute(session show): %w; stdout=%s; stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var session factoryapi.FactorySession
	if err := json.Unmarshal([]byte(inputs.Stdout()), &session); err != nil {
		return factoryapi.FactorySession{}, fmt.Errorf("decode session JSON: %w; stdout=%s", err, inputs.Stdout())
	}
	if session.Id == "" {
		return factoryapi.FactorySession{}, fmt.Errorf("session JSON omitted id: %s", inputs.Stdout())
	}
	return session, nil
}

func waitForPromptReloadSessionTerminal(t testing.TB, process support.Process, serverURL, sessionID string) factoryapi.FactorySession {
	t.Helper()

	// The provider edge synchronizes prompt capture, but only the public session
	// projection reports terminal runtime state. Bounded CLI reads are therefore
	// the customer-visible observation boundary for this asynchronous transition.
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	var lastErr error
	var lastSession factoryapi.FactorySession
	for {
		lastSession, lastErr = readPromptReloadSession(t, process, serverURL, sessionID)
		if lastErr == nil && promptReloadSessionIsTerminal(lastSession) {
			return lastSession
		}
		select {
		case <-poll.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for Factory Session %q to become terminal: last=%#v error=%v", sessionID, lastSession, lastErr)
		}
	}
}

func promptReloadSessionIsTerminal(session factoryapi.FactorySession) bool {
	completed := session.Runtime.Progress.Categories.Terminal + session.Runtime.Progress.Categories.Failed
	if completed == 0 || session.Runtime.Progress.Categories.Initial != 0 || session.Runtime.Progress.Categories.Processing != 0 || session.Runtime.Progress.InFlightCount != 0 {
		return false
	}
	return session.Runtime.Status == factoryapi.FactorySessionStatusIDLE || session.Runtime.Status == factoryapi.FactorySessionStatusFINISHED
}

func assertClaudePromptRequest(t *testing.T, request platformprocess.CommandRequest, wantWorker, wantWorkstation string) {
	t.Helper()

	if request.Command != string(modelprovider.ProviderClaude) {
		t.Fatalf("provider command = %q, want %q; request=%#v", request.Command, modelprovider.ProviderClaude, request)
	}
	systemPrompt, ok := argumentAfter(request.Args, "--system-prompt")
	if !ok {
		t.Fatalf("provider request args missing --system-prompt: %#v", request.Args)
	}
	if systemPrompt != wantWorker {
		t.Fatalf("provider worker prompt = %q, want %q; args=%#v", systemPrompt, wantWorker, request.Args)
	}
	if len(request.Args) == 0 {
		t.Fatal("provider request args are empty")
	}
	userMessage := request.Args[len(request.Args)-1]
	if !strings.Contains(userMessage, wantWorkstation) {
		t.Fatalf("provider workstation prompt = %q, want it to contain %q; args=%#v", userMessage, wantWorkstation, request.Args)
	}
}

func argumentAfter(args []string, flag string) (string, bool) {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag {
			return args[index+1], true
		}
	}
	return "", false
}

type promptReloadCommandRunner struct {
	mu            sync.Mutex
	requests      []platformprocess.CommandRequest
	firstRequest  chan platformprocess.CommandRequest
	secondRequest chan platformprocess.CommandRequest
	firstRelease  <-chan struct{}
}

func newPromptReloadCommandRunner(firstRelease <-chan struct{}) *promptReloadCommandRunner {
	return &promptReloadCommandRunner{
		firstRequest:  make(chan platformprocess.CommandRequest, 1),
		secondRequest: make(chan platformprocess.CommandRequest, 1),
		firstRelease:  firstRelease,
	}
}

func (runner *promptReloadCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	request = clonePromptReloadCommandRequest(request)
	runner.mu.Lock()
	requestIndex := len(runner.requests)
	runner.requests = append(runner.requests, request)
	runner.mu.Unlock()

	switch requestIndex {
	case 0:
		runner.firstRequest <- request
		select {
		case <-runner.firstRelease:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	case 1:
		runner.secondRequest <- request
	}
	return platformprocess.CommandResult{Stdout: support.ClaudeSuccessStdout("Done. COMPLETE")}, nil
}

func (runner *promptReloadCommandRunner) waitForFirstRequest(t *testing.T) platformprocess.CommandRequest {
	t.Helper()
	return waitForPromptReloadRequest(t, runner.firstRequest)
}

func (runner *promptReloadCommandRunner) waitForSecondRequest(t *testing.T) platformprocess.CommandRequest {
	t.Helper()
	return waitForPromptReloadRequest(t, runner.secondRequest)
}

func waitForPromptReloadRequest(t *testing.T, requests <-chan platformprocess.CommandRequest) platformprocess.CommandRequest {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	select {
	case request := <-requests:
		return request
	case <-ctx.Done():
		t.Fatalf("timed out waiting for provider command request: %v", ctx.Err())
		return platformprocess.CommandRequest{}
	}
}

func (runner *promptReloadCommandRunner) requestsSnapshot() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index, request := range runner.requests {
		requests[index] = clonePromptReloadCommandRequest(request)
	}
	return requests
}

func clonePromptReloadCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

var _ platformprocess.CommandRunner = (*promptReloadCommandRunner)(nil)
