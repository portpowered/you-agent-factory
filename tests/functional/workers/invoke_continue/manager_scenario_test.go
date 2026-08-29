package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	s8WorkerAID                 = "s8-worker-a"
	s8WorkerBID                 = "s8-worker-b"
	s8DispatchAID               = "s8-dispatch-a"
	s8DispatchBID               = "s8-dispatch-b"
	s8RequestAID                = "s8-request-a"
	s8RequestBID                = "s8-request-b"
	s8ProviderSessionA          = "s8-provider-session-a"
	s8ProviderSessionB          = "s8-provider-session-b"
	s8InterruptProviderSessionA = "s8-interrupt-provider-session-a"
	s8InterruptProviderSessionB = "s8-interrupt-provider-session-b"
	s8RepositoryAMarker         = "S8_REPOSITORY_A"
	s8RepositoryBMarker         = "S8_REPOSITORY_B"
	s8MessageA                  = "inspect repository A marker"
	s8MessageB                  = "inspect repository B marker"
	s8OutputA                   = "S8 repository A provider output"
	s8OutputB                   = "S8 repository B provider output"
)

type s8ScenarioIdentities struct {
	workerA, workerB     string
	dispatchA, dispatchB string
	requestA, requestB   string
	workA, workB         string
	successor            string
	interruptRequest     string
}

func newS8ScenarioIdentities(kind string, runNumber uint64) s8ScenarioIdentities {
	prefix := fmt.Sprintf("s8-%s-%d", kind, runNumber)
	return s8ScenarioIdentities{
		workerA:          prefix + "-worker-a",
		workerB:          prefix + "-worker-b",
		dispatchA:        prefix + "-dispatch-a",
		dispatchB:        prefix + "-dispatch-b",
		requestA:         prefix + "-request-a",
		requestB:         prefix + "-request-b",
		workA:            prefix + "-work-a",
		workB:            prefix + "-work-b",
		successor:        prefix + "-successor-a",
		interruptRequest: prefix + "-interrupt-request",
	}
}

type s8Correlation struct {
	factorySessionID         string
	repository               string
	marker                   string
	dispatchID               string
	workID                   string
	workerSessionID          string
	providerSessionID        string
	message                  string
	output                   string
	successorWorkerSessionID string
}

func (correlation s8Correlation) tokens() []string {
	return []string{
		correlation.factorySessionID,
		correlation.marker,
		correlation.repository,
		correlation.dispatchID,
		correlation.workID,
		correlation.workerSessionID,
		correlation.providerSessionID,
		correlation.message,
		correlation.output,
	}
}

func (correlation s8Correlation) owns(token string) bool {
	if token == "" {
		return false
	}
	for _, own := range correlation.tokens() {
		if own == token {
			return true
		}
	}
	return false
}

type s8ManagerScenario struct {
	ctx         context.Context
	fixture     *invokeContinuePackageFixture
	manager     support.Process
	env         []string
	factoryDir  string
	serverURL   string
	sessionA    *invokeContinueFactorySession
	sessionB    *invokeContinueFactorySession
	repositoryA s8Repository
	repositoryB s8Repository
	runner      *s8RemoteProviderRunner
	ids         s8ScenarioIdentities
}

type s8ManagerOverlap struct {
	streamA      s8StreamCapture
	streamB      s8StreamCapture
	correlationA s8Correlation
	correlationB s8Correlation
}

// TestDWROS8ManagerInspectsTwoIsolatedRemoteWorkers proves the first S8
// story through the public manager boundary. The server and manager processes
// both come from root.BuildProcess; only the provider command edge is replaced.
// Each provider command publishes its own Provider Session identity before
// waiting on its repository-specific barrier, which makes overlap and public
// inspection deterministic without sleeps or polling.
func TestDWROS8ManagerInspectsTwoIsolatedRemoteWorkers(t *testing.T) {
	// The context is a bounded deadlock watchdog. Normal readiness comes from
	// provider-edge signals and public CLI stream observations below.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	scenario := newS8ManagerScenario(t, ctx)
	defer scenario.runner.releaseAll()

	overlap := startS8ManagerWorkers(t, scenario)
	assertS8ManagerOverlap(t, scenario, overlap)
	finishS8ManagerScenario(t, scenario, overlap)
}

func newS8ManagerScenario(t *testing.T, ctx context.Context) s8ManagerScenario {
	t.Helper()
	fixture := ensureInvokeContinuePackageFixture(t)
	ownedScenario := fixture.scenario(t, "manager-isolation")
	return s8ManagerScenario{
		ctx: ctx, fixture: fixture, manager: fixture.process,
		env: invokeContinueEnvironment(fixture.homeDir), factoryDir: fixture.hostDir,
		serverURL: fixture.baseURL, sessionA: ownedScenario.session,
		sessionB: fixture.openSession(t), repositoryA: fixture.managerRepositoryA,
		repositoryB: fixture.managerRepositoryB, runner: fixture.managerRunner,
		ids: newS8ScenarioIdentities("manager", ownedScenario.runNumber),
	}
}

func (scenario s8ManagerScenario) close(t testing.TB) {
	t.Helper()
	scenario.sessionB.close(t)
	scenario.sessionB.assertDeleted(t)
	scenario.sessionA.close(t)
	scenario.sessionA.assertDeleted(t)
}

func startS8ManagerWorkers(t *testing.T, scenario s8ManagerScenario) s8ManagerOverlap {
	t.Helper()
	ids := scenario.ids
	invokeS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestA, workerSessionID: ids.workerA, dispatchID: ids.dispatchA,
		factorySessionID: scenario.sessionA.id, repository: scenario.repositoryA.path, workID: ids.workA, message: s8MessageA,
	})
	scenario.runner.waitStarted(t, scenario.repositoryA.path, scenario.fixture.router.requests)

	invokeS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, s8RemoteWorkerInvocation{
		requestID: ids.requestB, workerSessionID: ids.workerB, dispatchID: ids.dispatchB,
		factorySessionID: scenario.sessionB.id, repository: scenario.repositoryB.path, workID: ids.workB, message: s8MessageB,
	})
	scenario.runner.waitStarted(t, scenario.repositoryB.path, scenario.fixture.router.requests)

	return s8ManagerOverlap{
		streamA: startS8LiveStream(t, scenario.fixture, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, scenario.sessionA.id, ids.workerA, s8ProviderSessionA),
		streamB: startS8LiveStream(t, scenario.fixture, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, scenario.sessionB.id, ids.workerB, s8ProviderSessionB),
		correlationA: s8Correlation{
			factorySessionID: scenario.sessionA.id, repository: scenario.repositoryA.path, marker: scenario.repositoryA.marker, dispatchID: ids.dispatchA,
			workID: ids.workA, workerSessionID: ids.workerA, providerSessionID: s8ProviderSessionA, message: s8MessageA, output: s8OutputA,
		},
		correlationB: s8Correlation{
			factorySessionID: scenario.sessionB.id, repository: scenario.repositoryB.path, marker: scenario.repositoryB.marker, dispatchID: ids.dispatchB,
			workID: ids.workB, workerSessionID: ids.workerB, providerSessionID: s8ProviderSessionB, message: s8MessageB, output: s8OutputB,
		},
	}
}

func assertS8ManagerOverlap(t *testing.T, scenario s8ManagerScenario, overlap s8ManagerOverlap) {
	t.Helper()
	ids := scenario.ids
	overlap.streamA.writer.waitWorkerSessionFrame(t, ids.workerA)
	overlap.streamB.writer.waitWorkerSessionFrame(t, ids.workerB)

	active := listS8RemoteWorkers(t, scenario.ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL, "STARTING", "RUNNING")
	if len(active) != 2 {
		t.Fatalf("active direct Worker Sessions = %d, want two: %#v", len(active), active)
	}
	assertS8ActiveObservation(t, findS8Observation(t, active, ids.workerA), ids.workerA, scenario.sessionA.id, ids.workA, s8ProviderSessionA)
	assertS8ActiveObservation(t, findS8Observation(t, active, ids.workerB), ids.workerB, scenario.sessionB.id, ids.workB, s8ProviderSessionB)

	showA := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA)
	showB := showS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, ids.workerB)
	assertS8TopLevelActiveObservation(t, showA, ids.workerA, ids.workA, s8ProviderSessionA)
	assertS8TopLevelActiveObservation(t, showB, ids.workerB, ids.workB, s8ProviderSessionB)
}

func finishS8ManagerScenario(t *testing.T, scenario s8ManagerScenario, overlap s8ManagerOverlap) {
	t.Helper()
	ids := scenario.ids
	scenario.runner.release(t, scenario.repositoryA.path)
	scenario.runner.release(t, scenario.repositoryB.path)
	waitS8Stream(t, overlap.streamA, ids.workerA)
	waitS8Stream(t, overlap.streamB, ids.workerB)
	assertS8StreamIsolation(t, overlap.streamA.writer.bytes(), overlap.correlationA, overlap.correlationB)
	assertS8StreamIsolation(t, overlap.streamB.writer.bytes(), overlap.correlationB, overlap.correlationA)

	retainedA := replayS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA)
	retainedB := replayS8RemoteWorker(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, ids.workerB)
	assertS8RetainedStream(t, retainedA, overlap.correlationA, overlap.correlationB)
	assertS8RetainedStream(t, retainedB, overlap.correlationB, overlap.correlationA)

	completed := listS8RemoteWorkers(t, scenario.ctx, scenario.manager, scenario.env, scenario.factoryDir, scenario.serverURL, "COMPLETED")
	assertS8TopLevelCompletedObservation(t, findS8Observation(t, completed, ids.workerA), ids.workerA, ids.workA, s8ProviderSessionA)
	assertS8TopLevelCompletedObservation(t, findS8Observation(t, completed, ids.workerB), ids.workerB, ids.workB, s8ProviderSessionB)
	transcriptA := readS8RemoteTranscript(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryA.path, scenario.serverURL, ids.workerA)
	transcriptB := readS8RemoteTranscript(t, scenario.ctx, scenario.manager, scenario.env, scenario.repositoryB.path, scenario.serverURL, ids.workerB)
	assertS8Transcript(t, transcriptA, overlap.correlationA, overlap.correlationB)
	assertS8Transcript(t, transcriptB, overlap.correlationB, overlap.correlationA)

	assertS8ProviderRequests(t, scenario.runner.requests(), scenario.runner.markers(), overlap.correlationA, overlap.correlationB)
	scenario.close(t)
}

type s8Repository struct {
	path   string
	marker string
}

func newS8Repository(t *testing.T, name, marker string) s8Repository {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	repository, err := newS8RepositoryAt(path, marker)
	if err != nil {
		t.Fatalf("create repository %s: %v", name, err)
	}
	return repository
}

func newS8RepositoryAt(path, marker string) (s8Repository, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return s8Repository{}, fmt.Errorf("create repository directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(path, "S8_MARKER"), []byte(marker+"\n"), 0o644); err != nil {
		return s8Repository{}, fmt.Errorf("write repository marker: %w", err)
	}
	return s8Repository{path: filepath.Clean(path), marker: marker}, nil
}

type s8RemoteWorkerInvocation struct {
	requestID        string
	workerSessionID  string
	dispatchID       string
	factorySessionID string
	repository       string
	workID           string
	message          string
}

func invokeS8RemoteWorker(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL string,
	invocation s8RemoteWorkerInvocation,
) {
	t.Helper()
	document := s8ExecutionDocument(t, invocation)
	inputs := support.FakeInputs(ctx, []string{
		"you", "--remote", "--server", serverURL, "--json", "worker-sessions", "invoke",
		"--request-id", invocation.requestID,
		"--worker-session-id", invocation.workerSessionID,
		"--dispatch-id", invocation.dispatchID,
		"--execution", document,
		"--async",
	})
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("remote invoke %s: %v\nstdout:\n%s\nstderr:\n%s", invocation.workerSessionID, err, inputs.Stdout(), inputs.Stderr())
	}
	var result s8InvokeResult
	decodeS8JSON(t, inputs.Stdout(), &result)
	if !result.Accepted || result.RequestID != invocation.requestID || result.WorkerSessionID != invocation.workerSessionID {
		t.Fatalf("remote invoke %s result = %#v, want accepted exact identity", invocation.workerSessionID, result)
	}
}

func s8ExecutionDocument(t *testing.T, invocation s8RemoteWorkerInvocation) string {
	t.Helper()
	document := struct {
		RequestID       string `json:"requestId"`
		WorkerSessionID string `json:"workerSessionId"`
		Execution       struct {
			FactorySessionID string `json:"factorySessionId"`
			WorkstationName  string `json:"workstationName"`
			Dispatch         struct {
				DispatchID      string `json:"dispatchId"`
				WorkstationName string `json:"workstationName"`
				WorkerType      string `json:"workerType"`
				Execution       struct {
					WorkIDs []string `json:"workIds"`
				} `json:"execution"`
			} `json:"dispatch"`
			WorkerType               string `json:"workerType"`
			RunnerID                 string `json:"runnerId"`
			ExecutorProvider         string `json:"executorProvider"`
			ModelProvider            string `json:"modelProvider"`
			Model                    string `json:"model"`
			WorkingDirectory         string `json:"workingDirectory"`
			WorkingDirectoryAuthored bool   `json:"workingDirectoryAuthored"`
			UserMessage              string `json:"userMessage"`
		} `json:"execution"`
	}{RequestID: invocation.requestID, WorkerSessionID: invocation.workerSessionID}
	document.Execution.WorkstationName = workers.ProviderInvocationRoute
	document.Execution.FactorySessionID = invocation.factorySessionID
	document.Execution.Dispatch.DispatchID = invocation.dispatchID
	document.Execution.Dispatch.WorkstationName = workers.ProviderInvocationRoute
	document.Execution.Dispatch.WorkerType = "processor"
	document.Execution.Dispatch.Execution.WorkIDs = []string{invocation.workID}
	document.Execution.WorkerType = "processor"
	document.Execution.RunnerID = "codex"
	document.Execution.ExecutorProvider = "codex"
	document.Execution.ModelProvider = "codex"
	document.Execution.Model = "functional-model"
	document.Execution.WorkingDirectory = invocation.repository
	document.Execution.WorkingDirectoryAuthored = true
	document.Execution.UserMessage = invocation.message
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode S8 execution document: %v", err)
	}
	return string(encoded)
}

type s8InvokeResult struct {
	RequestID       string `json:"requestId"`
	WorkerSessionID string `json:"workerSessionId"`
	Accepted        bool   `json:"accepted"`
	State           string `json:"state"`
}

type s8WorkerObservation struct {
	WorkerSessionID          string             `json:"workerSessionId"`
	FactorySessionID         *string            `json:"factorySessionId"`
	Direct                   bool               `json:"direct"`
	ProviderSessionAvailable bool               `json:"providerSessionAvailable"`
	ProviderSession          *s8ProviderSession `json:"providerSession"`
	AttemptID                string             `json:"attemptId"`
	State                    string             `json:"state"`
	Transcript               string             `json:"transcript"`
	WorkIDs                  []string           `json:"workIds"`
}

type s8ProviderSession struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}

type s8WorkerList struct {
	Sessions []s8WorkerObservation `json:"sessions"`
}

func listS8RemoteWorkers(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL string,
	states ...string,
) []s8WorkerObservation {
	t.Helper()
	args := []string{"--json", "worker-sessions", "list", "--scope", "direct"}
	for _, state := range states {
		args = append(args, "--state", state)
	}
	inputs := executeS8RemoteCLI(t, ctx, process, env, workingDirectory, serverURL, args...)
	var result s8WorkerList
	decodeS8JSON(t, inputs.Stdout(), &result)
	return result.Sessions
}

func showS8RemoteWorker(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL, workerSessionID string,
) s8WorkerObservation {
	t.Helper()
	inputs := executeS8RemoteCLI(t, ctx, process, env, workingDirectory, serverURL,
		"--json", "worker-sessions", "show", "--worker-session-id", workerSessionID)
	var result s8WorkerObservation
	decodeS8JSON(t, inputs.Stdout(), &result)
	return result
}

func replayS8RemoteWorker(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL, workerSessionID string,
) []s8StreamFrame {
	t.Helper()
	inputs := executeS8RemoteCLI(t, ctx, process, env, workingDirectory, serverURL,
		"--json", "worker-sessions", "stream", "--worker-session-id", workerSessionID, "--replay-only")
	return decodeS8Stream(t, inputs.Stdout())
}

type s8Transcript struct {
	WorkerSessionID string            `json:"workerSessionId"`
	ProviderSession s8ProviderSession `json:"providerSession"`
	WorkIDs         []string          `json:"workIds"`
	Entries         []map[string]any  `json:"entries"`
}

func readS8RemoteTranscript(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL, workerSessionID string,
) s8Transcript {
	t.Helper()
	inputs := executeS8RemoteCLI(t, ctx, process, env, workingDirectory, serverURL,
		"--json", "worker-sessions", "read", "--worker-session-id", workerSessionID)
	var result s8Transcript
	decodeS8JSON(t, inputs.Stdout(), &result)
	return result
}

func executeS8RemoteCLI(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	workingDirectory, serverURL string,
	args ...string,
) *support.CapturedInputs {
	t.Helper()
	command := append([]string{"you", "--remote", "--server", serverURL}, args...)
	inputs := support.FakeInputs(ctx, command)
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("remote CLI %s: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(command, " "), err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs
}

func findS8Observation(t *testing.T, observations []s8WorkerObservation, workerSessionID string) s8WorkerObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.WorkerSessionID == workerSessionID {
			return observation
		}
	}
	t.Fatalf("Worker Session %q missing from public list: %#v", workerSessionID, observations)
	return s8WorkerObservation{}
}

func assertS8NoUnexpectedActiveObservations(t *testing.T, observations []s8WorkerObservation, workerSessionIDs ...string) {
	t.Helper()
	allowed := make(map[string]struct{}, len(workerSessionIDs))
	for _, workerSessionID := range workerSessionIDs {
		allowed[workerSessionID] = struct{}{}
	}
	for _, observation := range observations {
		if observation.State != "STARTING" && observation.State != "RUNNING" {
			continue
		}
		if _, ok := allowed[observation.WorkerSessionID]; !ok {
			t.Fatalf("unexpected active direct Worker Session %q in current overlap: %#v", observation.WorkerSessionID, observations)
		}
	}
}

func assertS8ActiveObservation(t *testing.T, observation s8WorkerObservation, workerSessionID, factorySessionID, workID, providerSessionID string) {
	t.Helper()
	if observation.WorkerSessionID != workerSessionID || !observation.Direct || observation.AttemptID == "" {
		t.Fatalf("active observation = %#v, want direct exact attempt %q", observation, workerSessionID)
	}
	if observation.State != "STARTING" && observation.State != "RUNNING" {
		t.Fatalf("Worker Session %q state = %q, want active state", workerSessionID, observation.State)
	}
	assertS8FactorySession(t, observation.FactorySessionID, factorySessionID)
	assertS8WorkIDs(t, observation.WorkIDs, workID)
	assertS8ProviderSession(t, observation.ProviderSession, observation.ProviderSessionAvailable, providerSessionID)
}

func assertS8TopLevelActiveObservation(t *testing.T, observation s8WorkerObservation, workerSessionID, workID, providerSessionID string) {
	t.Helper()
	if observation.WorkerSessionID != workerSessionID || !observation.Direct || observation.AttemptID == "" {
		t.Fatalf("top-level active observation = %#v, want direct exact attempt %q", observation, workerSessionID)
	}
	if observation.State != "STARTING" && observation.State != "RUNNING" {
		t.Fatalf("Worker Session %q state = %q, want active state", workerSessionID, observation.State)
	}
	assertS8WorkIDs(t, observation.WorkIDs, workID)
	assertS8ProviderSession(t, observation.ProviderSession, observation.ProviderSessionAvailable, providerSessionID)
}

func assertS8CompletedObservation(t *testing.T, observation s8WorkerObservation, workerSessionID, factorySessionID, workID, providerSessionID string) {
	t.Helper()
	if observation.WorkerSessionID != workerSessionID || !observation.Direct || observation.State != "COMPLETED" || observation.AttemptID == "" {
		t.Fatalf("completed observation = %#v, want direct completed exact attempt %q", observation, workerSessionID)
	}
	assertS8FactorySession(t, observation.FactorySessionID, factorySessionID)
	assertS8WorkIDs(t, observation.WorkIDs, workID)
	assertS8ProviderSession(t, observation.ProviderSession, observation.ProviderSessionAvailable, providerSessionID)
}

func assertS8TopLevelCompletedObservation(t *testing.T, observation s8WorkerObservation, workerSessionID, workID, providerSessionID string) {
	t.Helper()
	if observation.WorkerSessionID != workerSessionID || !observation.Direct || observation.State != "COMPLETED" || observation.AttemptID == "" {
		t.Fatalf("top-level completed observation = %#v, want direct completed exact attempt %q", observation, workerSessionID)
	}
	assertS8WorkIDs(t, observation.WorkIDs, workID)
	assertS8ProviderSession(t, observation.ProviderSession, observation.ProviderSessionAvailable, providerSessionID)
}

func assertS8FactorySession(t *testing.T, actual *string, expected string) {
	t.Helper()
	if actual == nil || strings.TrimSpace(*actual) == "" || *actual != expected {
		actualValue := "<nil>"
		if actual != nil {
			actualValue = *actual
		}
		t.Fatalf("Factory Session = %q, want explicit %q", actualValue, expected)
	}
}

func assertS8FactorySessionIfPresent(t *testing.T, actual *string, expected string) {
	t.Helper()
	if actual == nil || strings.TrimSpace(*actual) == "" || *actual == workers.DefaultSessionID {
		return
	}
	assertS8FactorySession(t, actual, expected)
}

func assertS8WorkIDs(t *testing.T, actual []string, expected string) {
	t.Helper()
	if len(actual) != 1 || actual[0] != expected {
		t.Fatalf("Work IDs = %#v, want exactly %q", actual, expected)
	}
}

func assertS8Transcript(t *testing.T, transcript s8Transcript, own s8Correlation, foreign ...s8Correlation) {
	t.Helper()
	if transcript.WorkerSessionID != own.workerSessionID {
		t.Fatalf("transcript Worker Session = %q, want %q", transcript.WorkerSessionID, own.workerSessionID)
	}
	if transcript.ProviderSession.Provider != "codex" || transcript.ProviderSession.Kind == "" || transcript.ProviderSession.ID != own.providerSessionID {
		t.Fatalf("transcript Provider Session = %#v, want codex exact %q", transcript.ProviderSession, own.providerSessionID)
	}
	assertS8WorkIDs(t, transcript.WorkIDs, own.workID)
	if len(transcript.Entries) == 0 {
		t.Fatalf("transcript for %q returned no entries", own.workerSessionID)
	}
	encoded, err := json.Marshal(transcript.Entries)
	if err != nil {
		t.Fatalf("encode transcript for %q: %v", own.workerSessionID, err)
	}
	if !bytes.Contains(encoded, []byte(own.output)) {
		t.Fatalf("transcript for %q omitted its provider output %q: %s", own.workerSessionID, own.output, encoded)
	}
	for _, correlation := range foreign {
		for _, token := range correlation.tokens() {
			if token != "" && !own.owns(token) && bytes.Contains(encoded, []byte(token)) {
				t.Fatalf("transcript for %q contains foreign correlation %q: %s", own.workerSessionID, token, encoded)
			}
		}
	}
}

func assertS8ProviderSession(t *testing.T, session *s8ProviderSession, available bool, expectedID string) {
	t.Helper()
	if !available || session == nil || session.Provider != "codex" || session.Kind == "" || session.ID != expectedID {
		t.Fatalf("Provider Session = %#v available=%t, want codex exact %q", session, available, expectedID)
	}
}

type s8StreamFrame struct {
	Delivery         string             `json:"delivery"`
	WorkerSessionID  string             `json:"workerSessionId"`
	FactorySessionID *string            `json:"factorySessionId"`
	ProviderSession  *s8ProviderSession `json:"providerSession"`
	WorkIDs          []string           `json:"workIds"`
	Event            *s8StreamEvent     `json:"event"`
	ReplaySummary    *s8ReplaySummary   `json:"replaySummary"`
}

type s8StreamEvent struct {
	Position       uint64          `json:"position"`
	SourceType     string          `json:"sourceType"`
	SourceID       string          `json:"sourceId"`
	SourceSequence uint64          `json:"sourceSequence"`
	SourceEventID  string          `json:"sourceEventId"`
	SchemaID       string          `json:"schemaId"`
	Payload        json.RawMessage `json:"payload"`
}

type s8ReplaySummary struct {
	Kind          string `json:"kind"`
	Complete      bool   `json:"complete"`
	Reason        string `json:"reason"`
	EventsEmitted int64  `json:"eventsEmitted"`
}

func decodeS8Stream(t *testing.T, stdout string) []s8StreamFrame {
	t.Helper()
	var frames []s8StreamFrame
	for index, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var frame s8StreamFrame
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("decode S8 stream frame %d: %v\nline:%s", index+1, err, line)
		}
		if frame.Delivery == "" {
			var summary s8ReplaySummary
			if err := json.Unmarshal([]byte(line), &summary); err != nil {
				t.Fatalf("decode S8 replay summary %d: %v\nline:%s", index+1, err, line)
			}
			frame.ReplaySummary = &summary
		}
		frames = append(frames, frame)
	}
	return frames
}

func assertS8StreamIsolation(
	t *testing.T,
	stdout []byte,
	own s8Correlation,
	foreign ...s8Correlation,
) {
	t.Helper()
	frames := decodeS8Stream(t, string(stdout))
	if len(frames) == 0 {
		t.Fatalf("live stream for %q returned no frames", own.workerSessionID)
	}
	if !bytes.Contains(stdout, []byte(own.output)) {
		t.Fatalf("live stream for %q omitted its provider output %q:\n%s", own.workerSessionID, own.output, stdout)
	}
	assertS8Frames(t, frames, own, true, foreign...)
	assertS8PartialOutputOrdering(t, frames, own)
	var terminal bool
	for _, frame := range frames {
		if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
			terminal = true
		}
	}
	if !terminal {
		t.Fatalf("live stream for %q omitted terminal delivery: %#v", own.workerSessionID, frames)
	}
}

func assertS8PartialOutputOrdering(t *testing.T, frames []s8StreamFrame, own s8Correlation) {
	t.Helper()
	terminalIndex := -1
	outputIndex := -1
	for index, frame := range frames {
		if frame.ReplaySummary != nil {
			continue
		}
		if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
			if terminalIndex < 0 {
				terminalIndex = index
			}
			continue
		}
		if frame.Event != nil {
			encoded, err := json.Marshal(frame.Event)
			if err != nil {
				t.Fatalf("encode partial stream event for %q: %v", own.workerSessionID, err)
			}
			if outputIndex < 0 && bytes.Contains(encoded, []byte(own.output)) {
				outputIndex = index
			}
		}
	}
	if terminalIndex < 0 || outputIndex < 0 || outputIndex >= terminalIndex || outputIndex == 0 {
		t.Fatalf("live stream for %q did not preserve partial-output-before-terminal ordering: output frame=%d terminal frame=%d frames=%#v", own.workerSessionID, outputIndex, terminalIndex, frames)
	}
}

func assertS8RetainedStream(
	t *testing.T,
	frames []s8StreamFrame,
	own s8Correlation,
	foreign ...s8Correlation,
) {
	t.Helper()
	if len(frames) == 0 {
		t.Fatalf("retained stream for %q returned no frames", own.workerSessionID)
	}
	encoded, err := json.Marshal(frames)
	if err != nil {
		t.Fatalf("encode retained stream for %q: %v", own.workerSessionID, err)
	}
	if !bytes.Contains(encoded, []byte(own.output)) {
		t.Fatalf("retained stream for %q omitted provider output %q: %s", own.workerSessionID, own.output, encoded)
	}
	assertS8Frames(t, frames, own, true, foreign...)
	var terminal, complete bool
	for _, frame := range frames {
		if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
			terminal = true
		}
		if frame.ReplaySummary != nil && frame.ReplaySummary.Complete {
			complete = true
		}
	}
	if !terminal || !complete {
		t.Fatalf("retained stream for %q = %#v, want terminal and complete replay summary", own.workerSessionID, frames)
	}
}

type s8FrameEvidence struct {
	worker, factorySession, provider, work, dispatch, repository, message, output, workingDirectory, event bool
}

func assertS8Frames(
	t *testing.T,
	frames []s8StreamFrame,
	own s8Correlation,
	requireOutput bool,
	foreign ...s8Correlation,
) {
	t.Helper()
	evidence := s8FrameEvidence{}
	var previousPosition uint64
	for index, frame := range frames {
		if frame.ReplaySummary != nil {
			continue
		}
		previousPosition = inspectS8Frame(t, index, frame, own, foreign, previousPosition, &evidence)
	}
	if !evidence.worker || !evidence.factorySession || !evidence.provider || !evidence.work || !evidence.dispatch || !evidence.repository ||
		!evidence.workingDirectory || !evidence.event || (requireOutput && !evidence.output) {
		encoded, _ := json.Marshal(frames)
		t.Fatalf("stream for %q omitted own correlation: %#v frames=%s", own.workerSessionID, evidence, encoded)
	}
}

func inspectS8Frame(
	t *testing.T,
	index int,
	frame s8StreamFrame,
	own s8Correlation,
	foreign []s8Correlation,
	previousPosition uint64,
	evidence *s8FrameEvidence,
) uint64 {
	t.Helper()
	encoded, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("encode stream frame %d for %q: %v", index, own.workerSessionID, err)
	}
	assertS8ForeignFrame(t, index, encoded, frame, own, foreign)
	if frame.FactorySessionID != nil {
		assertS8FactorySession(t, frame.FactorySessionID, own.factorySessionID)
		evidence.factorySession = true
	}
	if frame.WorkerSessionID != "" && frame.WorkerSessionID != own.workerSessionID {
		t.Fatalf("stream frame %d Worker Session = %q, want %q", index, frame.WorkerSessionID, own.workerSessionID)
	}
	if frame.WorkerSessionID == own.workerSessionID {
		evidence.worker = true
	}
	if frame.ProviderSession != nil {
		if frame.ProviderSession.Provider != "codex" || frame.ProviderSession.ID != own.providerSessionID {
			t.Fatalf("stream frame %d Provider Session = %#v, want codex/%q", index, frame.ProviderSession, own.providerSessionID)
		}
		evidence.provider = true
	}
	if len(frame.WorkIDs) > 0 {
		assertS8WorkIDs(t, frame.WorkIDs, own.workID)
		evidence.work = true
	}
	return inspectS8StreamEvent(t, index, frame.Event, own, previousPosition, evidence)
}

func assertS8ForeignFrame(t *testing.T, index int, encoded []byte, frame s8StreamFrame, own s8Correlation, foreign []s8Correlation) {
	t.Helper()
	expectedSuccessorLineage := s8HasExpectedSuccessorLineage(frame, own)
	for _, correlation := range foreign {
		for _, token := range correlation.tokens() {
			if token == "" || own.owns(token) ||
				(expectedSuccessorLineage && token == own.successorWorkerSessionID && token == correlation.workerSessionID) ||
				(token == correlation.workerSessionID && correlation.successorWorkerSessionID == own.workerSessionID) ||
				(token == correlation.dispatchID && strings.HasPrefix(own.dispatchID, token+"/continue/")) {
				continue
			}
			if bytes.Contains(encoded, []byte(token)) {
				t.Fatalf("stream frame %d for %q contains foreign correlation %q: %s", index, own.workerSessionID, token, encoded)
			}
		}
	}
}

func s8HasExpectedSuccessorLineage(frame s8StreamFrame, own s8Correlation) bool {
	if own.successorWorkerSessionID == "" || frame.Event == nil {
		return false
	}
	successorWorkerSessionID, ok := s8SuccessorLineage(frame.Event.Payload)
	return ok && successorWorkerSessionID == own.successorWorkerSessionID
}

func s8SuccessorLineage(payload json.RawMessage) (string, bool) {
	type lineage struct {
		SuccessorWorkerSessionID string `json:"successorWorkerSessionId"`
	}

	var decoded struct {
		Lineage *lineage        `json:"lineage"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", false
	}
	if decoded.Lineage != nil && decoded.Lineage.SuccessorWorkerSessionID != "" {
		return decoded.Lineage.SuccessorWorkerSessionID, true
	}

	var nested struct {
		Lineage *lineage `json:"lineage"`
	}
	if len(decoded.Payload) == 0 || json.Unmarshal(decoded.Payload, &nested) != nil || nested.Lineage == nil || nested.Lineage.SuccessorWorkerSessionID == "" {
		return "", false
	}
	return nested.Lineage.SuccessorWorkerSessionID, true
}

func inspectS8StreamEvent(
	t *testing.T,
	index int,
	event *s8StreamEvent,
	own s8Correlation,
	previousPosition uint64,
	evidence *s8FrameEvidence,
) uint64 {
	t.Helper()
	if event == nil {
		return previousPosition
	}
	evidence.event = true
	if event.SourceType == "" || event.SourceID == "" || event.SourceEventID == "" || event.SchemaID == "" {
		t.Fatalf("stream frame %d for %q has incomplete event identity: %#v", index, own.workerSessionID, event)
	}
	if event.Position <= previousPosition {
		t.Fatalf("stream frame positions for %q are not increasing: previous=%d current=%d", own.workerSessionID, previousPosition, event.Position)
	}
	if successorWorkerSessionID, ok := s8SuccessorLineage(event.Payload); ok && own.successorWorkerSessionID != "" && successorWorkerSessionID != own.successorWorkerSessionID {
		t.Fatalf("stream frame %d for %q successor lineage = %q, want %q", index, own.workerSessionID, successorWorkerSessionID, own.successorWorkerSessionID)
	}
	eventEncoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("encode stream event %d for %q: %v", index, own.workerSessionID, err)
	}
	evidence.factorySession = evidence.factorySession || bytes.Contains(eventEncoded, []byte(own.factorySessionID))
	evidence.dispatch = evidence.dispatch || bytes.Contains(eventEncoded, []byte(own.dispatchID))
	evidence.work = evidence.work || bytes.Contains(eventEncoded, []byte(own.workID))
	evidence.repository = evidence.repository || bytes.Contains(eventEncoded, []byte(own.repository))
	evidence.message = evidence.message || bytes.Contains(eventEncoded, []byte(own.message))
	evidence.output = evidence.output || bytes.Contains(eventEncoded, []byte(own.output))
	if directory, ok := s8JSONStringValue(event.Payload, "workingDirectory"); ok {
		if directory != own.repository {
			t.Fatalf("stream frame %d for %q workingDirectory = %q, want %q", index, own.workerSessionID, directory, own.repository)
		}
		evidence.repository = true
		evidence.workingDirectory = true
	}
	return event.Position
}

func s8JSONStringValue(payload json.RawMessage, key string) (string, bool) {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return "", false
	}
	return findS8JSONStringValue(value, key)
}

func findS8JSONStringValue(value any, key string) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if candidate, ok := typed[key].(string); ok {
			return candidate, true
		}
		for _, nested := range typed {
			if found, ok := findS8JSONStringValue(nested, key); ok {
				return found, true
			}
		}
	case []any:
		for _, nested := range typed {
			if found, ok := findS8JSONStringValue(nested, key); ok {
				return found, true
			}
		}
	}
	return "", false
}
