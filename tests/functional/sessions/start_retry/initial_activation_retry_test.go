package start_retry_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const initialOpeningReadCeiling = 5 * time.Second

func initialOpeningFactoryConfig() map[string]any {
	return map[string]any{
		"name": "initial-opening-retry",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name": "process", "worker": "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

// These scenarios share one immutable process shape and run as independent
// explicit sessions. Only the dependent failure/cleanup/retry sequence within
// each customer identity is serialized. The filesystem gate is an external
// effect; all session publication, resources, events and Work execution are real.
func TestExplicitSessionOpeningFailureAndCancellationPreservePeers(t *testing.T) {
	t.Parallel()

	failure := errors.New("controlled initial input directory opening failure")
	failed := newInitialOpeningScenario(t)
	canceled := newInitialOpeningScenario(t)
	reused := newInitialOpeningScenario(t)
	effects := &initialOpeningEffects{calls: make(map[string]int)}
	gate := &initialOpeningGate{entered: make(chan struct{}), release: make(chan struct{})}
	files := &initialOpeningDirectories{
		failedPath: filepath.Join(failed.candidateDir, factorydefinitions.InputsDir),
		gatedPath:  filepath.Join(canceled.candidateDir, factorydefinitions.InputsDir),
		failure:    failure, gate: gate, effects: effects,
	}
	api := support.NewProcessAPIServer()
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		FactoryRuntimeDirectories: files,
		ScriptCommandRunner:       initialOpeningScriptRunner{effects: effects},
		APIServerStarter:          api.Start,
	})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}
	support.CleanupProcess(t, process)
	// Boot through the customer's command boundary before using its public
	// identity-selecting service contract (HTTP Open cannot select an identity).
	hostDir := support.ScaffoldFactory(t, initialOpeningFactoryConfig())
	home := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = hostDir
	command := support.StartProcessCommand(t, process, inputs.Input)
	api.WaitForURL(t)
	t.Cleanup(func() { command.Stop(t) })
	sessions, ok := process.FactorySessions().FactorySessions().(factorysessions.Service)
	if !ok {
		t.Fatal("process did not expose its public Factory Sessions service")
	}

	t.Run("failed resource opening retries with the same identity", func(t *testing.T) {
		t.Parallel()
		peerHistory := failed.startPeer(t, sessions)
		request := failed.request()
		recordPath := filepath.Join(t.TempDir(), "retry.replay.jsonl")
		request.RuntimeSelection.Recording.RecordPath = recordPath
		_, err := sessions.Start(t.Context(), request)
		if !errors.Is(err, failure) {
			t.Fatalf("failed Start error = %v, want controlled opening cause", err)
		}
		assertInitialOpeningNotPublished(t, sessions, failed.candidateID)
		assertInitialOpeningHistoryPreserved(t, sessions, failed.peerID, peerHistory)
		assertInitialOpeningInvocation(t, sessions, failed.peerID)
		// Reuse the failed opening's identity and destinations. A new filename
		// would conceal a partial recording left behind by failed activation.
		startInitialOpeningSession(t, sessions, request)
		assertInitialOpeningInvocation(t, sessions, failed.candidateID)
		assertInitialOpeningDuplicate(t, sessions, failed, effects)
		history := initialOpeningHistory(t, sessions, failed.candidateID)
		closeInitialOpeningSession(t, sessions, failed.candidateID)
		assertInitialOpeningReplay(t, process, recordPath, failed.candidateID, history)
		assertInitialOpeningHistoryPreserved(t, sessions, failed.peerID, peerHistory)
		assertInitialOpeningInvocation(t, sessions, failed.peerID)
	})
	t.Run("cancellation while opening unwinds before same identity retry", func(t *testing.T) {
		t.Parallel()
		testCanceledInitialOpening(t, sessions, canceled, gate)
	})
	t.Run("closed recording preserves attributed history and live peer", func(t *testing.T) {
		t.Parallel()
		peerHistory := reused.startPeer(t, sessions)
		request := reused.request()
		recordPath := filepath.Join(t.TempDir(), "opening.replay.jsonl")
		request.RuntimeSelection.Recording.RecordPath = recordPath
		startInitialOpeningSession(t, sessions, request)
		assertInitialOpeningInvocation(t, sessions, reused.candidateID)
		firstHistory := initialOpeningHistory(t, sessions, reused.candidateID)
		closeInitialOpeningSession(t, sessions, reused.candidateID)
		assertInitialOpeningReplay(t, process, recordPath, reused.candidateID, firstHistory)
		assertInitialOpeningHistoryPreserved(t, sessions, reused.peerID, peerHistory)
		assertInitialOpeningInvocation(t, sessions, reused.peerID)
	})
}

func testCanceledInitialOpening(t *testing.T, sessions factorysessions.Service, scenario initialOpeningScenario, gate *initialOpeningGate) {
	t.Helper()
	t.Cleanup(gate.unblock)
	peerHistory := scenario.startPeer(t, sessions)
	request := scenario.request()
	request.RuntimeSelection.Recording.RecordPath = filepath.Join(t.TempDir(), "canceled.replay.jsonl")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := sessions.Start(ctx, request)
		result <- err
	}()
	select {
	case <-gate.entered:
	case <-time.After(initialOpeningReadCeiling):
		t.Fatal("initial opening did not reach the filesystem gate")
	}
	cancel()
	gate.unblock()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled Start error = %v, want context.Canceled", err)
		}
	case <-time.After(initialOpeningReadCeiling):
		t.Fatal("canceled initial opening did not return")
	}
	assertInitialOpeningNotPublished(t, sessions, scenario.candidateID)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
	startInitialOpeningSession(t, sessions, request)
	assertInitialOpeningInvocation(t, sessions, scenario.candidateID)
}

func assertInitialOpeningReplay(t *testing.T, process support.Process, recordPath, sessionID string, history *factorydefinitions.FactoryEventStream) {
	t.Helper()
	directory, home := t.TempDir(), t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", directory, "--replay", recordPath, "--no-record",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = directory
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("read finalized recording through replay: %v\n%s\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	output := inputs.Stdout()
	for _, expected := range []string{"Replayed Factory Session: " + sessionID, "Status: SUCCEEDED", "state=\"complete\""} {
		if !strings.Contains(output, expected) {
			t.Fatalf("replay output = %q, want %q", output, expected)
		}
	}
	lastPosition := -1
	for _, event := range history.History {
		position := strings.Index(output, fmt.Sprintf("%s (%s)", event.Type, event.Id))
		if position <= lastPosition {
			t.Fatalf("retained event %s missing or out of order in replay:\n%s", event.Id, output)
		}
		lastPosition = position
	}
}

func closeInitialOpeningSession(t *testing.T, sessions factorysessions.Service, sessionID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), initialOpeningReadCeiling)
	defer cancel()
	result, err := sessions.Control(ctx, factorysessions.SessionControlRequest{
		SessionID: sessionID, Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlClose,
	})
	if err != nil || !result.Closed {
		t.Fatalf("close owned session %s = %#v, %v", sessionID, result, err)
	}
}

type initialOpeningScenario struct {
	candidateDir, peerDir, home, logs, metrics string
	candidateID, runtimeID, peerID             string
}

func newInitialOpeningScenario(t *testing.T) initialOpeningScenario {
	t.Helper()
	config := initialOpeningFactoryConfig()
	config["workTypes"].([]map[string]any)[0]["handlingBehavior"] = []string{"DEFAULT"}
	config["workers"] = []map[string]string{{"name": "worker-a", "type": string(factorydefinitions.WorkerTypeScript), "command": "initial-opening-script"}}
	return initialOpeningScenario{
		candidateDir: support.ScaffoldFactory(t, config), peerDir: support.ScaffoldFactory(t, config),
		home: t.TempDir(), logs: t.TempDir(), metrics: t.TempDir(),
		candidateID: uuid.NewString(), runtimeID: uuid.NewString(), peerID: uuid.NewString(),
	}
}

func (scenario initialOpeningScenario) request() factorysessions.SessionStartRequest {
	return factorysessions.SessionStartRequest{
		SessionID: scenario.candidateID, Mode: factorysessions.SessionOperationModeLive,
		FolderPath: scenario.candidateDir, ActivationOnly: true,
		RuntimeSelection: &factorysessions.SessionRuntimeSelection{
			Mode:             factorysessions.SessionRuntimeModeService,
			SystemConfigHome: scenario.home, DefinitionSourcePath: filepath.Join(scenario.candidateDir, "factory.json"),
			ExecutionBaseDir: scenario.candidateDir, RuntimeInstanceID: scenario.runtimeID,
			LogPolicy: factorysessions.SessionArtifactPolicyEnabled, LogDirectory: scenario.logs,
			MetricsPolicy: factorysessions.SessionArtifactPolicyEnabled, MetricsDirectory: scenario.metrics,
		},
	}
}

func (scenario initialOpeningScenario) startPeer(t *testing.T, sessions factorysessions.Service) *factorydefinitions.FactoryEventStream {
	t.Helper()
	request := scenario.request()
	request.SessionID = scenario.peerID
	request.FolderPath = scenario.peerDir
	request.RuntimeSelection.DefinitionSourcePath = filepath.Join(scenario.peerDir, "factory.json")
	request.RuntimeSelection.ExecutionBaseDir = scenario.peerDir
	request.RuntimeSelection.RuntimeInstanceID = uuid.NewString()
	startInitialOpeningSession(t, sessions, request)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
	return initialOpeningHistory(t, sessions, scenario.peerID)
}

func startInitialOpeningSession(t *testing.T, sessions factorysessions.Service, request factorysessions.SessionStartRequest) {
	t.Helper()
	result, err := sessions.Start(t.Context(), request)
	if err != nil || result.SessionID != request.SessionID || result.Live == nil || result.Live.Session == nil || !result.Live.Session.RuntimeAvailable {
		t.Fatalf("Start explicit session = %#v, %v, want requested live identity", result, err)
	}
	t.Cleanup(func() {
		// A journey may already have closed its session to observe persisted
		// history. Cleanup owns only still-live IDs.
		ctx, cancel := context.WithTimeout(context.Background(), initialOpeningReadCeiling)
		defer cancel()
		_, getErr := sessions.Get(ctx, factorysessions.SessionGetRequest{
			SessionID: request.SessionID, Mode: factorysessions.SessionOperationModeLive,
		})
		if errors.Is(getErr, factorysessions.ErrSessionNotFound) {
			return
		}
		result, err := sessions.Control(ctx, factorysessions.SessionControlRequest{
			SessionID: request.SessionID, Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlClose,
		})
		if err != nil || !result.Closed {
			t.Errorf("close owned session %s = %#v, %v", request.SessionID, result, err)
		}
	})
}

func assertInitialOpeningNotPublished(t *testing.T, sessions factorysessions.Service, sessionID string) {
	t.Helper()
	_, err := sessions.Get(t.Context(), factorysessions.SessionGetRequest{SessionID: sessionID, Mode: factorysessions.SessionOperationModeLive})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("read failed opening %s error = %v, want session not found", sessionID, err)
	}
}

func assertInitialOpeningInvocation(t *testing.T, sessions factorysessions.Service, sessionID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), initialOpeningReadCeiling)
	defer cancel()
	result, err := sessions.Invoke(ctx, factorysessions.SessionInvokeRequest{
		SessionID: sessionID, ContentProvided: true,
		Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "prove this session remains usable"}},
	})
	if err != nil || result.Status != factorysessions.InvocationTerminalStatusCompleted || len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != "initial opening COMPLETE" {
		t.Fatalf("Invoke %s = %#v, %v, want completed controlled worker output", sessionID, result, err)
	}
}

func initialOpeningHistory(t *testing.T, sessions factorysessions.Service, sessionID string) *factorydefinitions.FactoryEventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	stream, err := sessions.SubscribeFactoryEventsForSession(ctx, sessionID, nil)
	if err != nil || stream == nil || len(stream.History) == 0 {
		t.Fatalf("session %s retained events = %#v, %v, want public history", sessionID, stream, err)
	}
	return stream
}

func assertInitialOpeningHistoryPreserved(t *testing.T, sessions factorysessions.Service, sessionID string, before *factorydefinitions.FactoryEventStream) {
	t.Helper()
	after := initialOpeningHistory(t, sessions, sessionID)
	if after.StreamGenerationID != before.StreamGenerationID || len(after.History) < len(before.History) {
		t.Fatal("opening another session reset the peer's generation or retained history")
	}
	for index, event := range before.History {
		if !reflect.DeepEqual(after.History[index], event) {
			t.Fatalf("peer retained event %d changed after another session's opening", index)
		}
	}
}

func assertInitialOpeningDuplicate(t *testing.T, sessions factorysessions.Service, scenario initialOpeningScenario, effects *initialOpeningEffects) {
	t.Helper()
	before := initialOpeningHistory(t, sessions, scenario.candidateID)
	openingEffects := effects.forScenario(scenario)
	if openingEffects[filepath.Clean(scenario.candidateDir)+"|worker.run"] == 0 {
		t.Fatal("candidate invocation was not observed at its selected command-runner edge")
	}
	_, err := sessions.Start(t.Context(), scenario.request())
	if err == nil || !strings.Contains(err.Error(), "already active") {
		t.Fatalf("duplicate ActivationOnly Start error = %v, want already-active diagnostic", err)
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.candidateID, before)
	if after := effects.forScenario(scenario); !reflect.DeepEqual(after, openingEffects) {
		t.Fatalf("duplicate Start executed candidate-owned external effects: before %v, after %v", openingEffects, after)
	}
}

type initialOpeningGate struct {
	entered, release chan struct{}
	once             sync.Once
}

func (gate *initialOpeningGate) unblock() { gate.once.Do(func() { close(gate.release) }) }

type initialOpeningDirectories struct {
	failedPath, gatedPath string
	failure               error
	gate                  *initialOpeningGate
	failed, gated         atomic.Bool
	effects               *initialOpeningEffects
}

func (files *initialOpeningDirectories) Stat(path string) (fs.FileInfo, error) {
	files.effects.record("runtime.stat", path)
	return os.Stat(path)
}

func (files *initialOpeningDirectories) MkdirAll(path string, mode fs.FileMode) error {
	files.effects.record("runtime.mkdir", path)
	if filepath.Clean(path) == files.failedPath && files.failed.CompareAndSwap(false, true) {
		return files.failure
	}
	if filepath.Clean(path) == files.gatedPath && files.gated.CompareAndSwap(false, true) {
		close(files.gate.entered)
		<-files.gate.release
	}
	return os.MkdirAll(path, mode)
}

type initialOpeningScriptRunner struct{ effects *initialOpeningEffects }

func (runner initialOpeningScriptRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.effects.record("worker.run", request.WorkDir)
	if request.Command != "initial-opening-script" {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected worker command %q", request.Command)
	}
	return platformprocess.CommandResult{Stdout: []byte("initial opening COMPLETE")}, nil
}

// Attribute external observations to scenario-owned paths so parallel peer
// activity cannot conceal or imitate a duplicate candidate opening.
type initialOpeningEffects struct {
	mu    sync.Mutex
	calls map[string]int
}

func (effects *initialOpeningEffects) record(operation, path string) {
	effects.mu.Lock()
	defer effects.mu.Unlock()
	effects.calls[filepath.Clean(path)+"|"+operation]++
}

func (effects *initialOpeningEffects) forScenario(scenario initialOpeningScenario) map[string]int {
	effects.mu.Lock()
	defer effects.mu.Unlock()
	result := make(map[string]int)
	for key, count := range effects.calls {
		path := filepath.Clean(scenario.candidateDir)
		if strings.HasPrefix(key, path+string(filepath.Separator)) || strings.HasPrefix(key, path+"|") {
			result[key] = count
		}
	}
	return result
}
