package start_retry_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const initialOpeningReadCeiling = 5 * time.Second

// Keep timestamps visibly separate from host time while using the explicitly
// selected real timer effect for asynchronous Work and lifecycle deadlines.
type initialOpeningClock struct{ platformclock.Real }

func (initialOpeningClock) Now() time.Time {
	return time.Date(2041, 2, 3, 4, 5, 6, 0, time.UTC)
}

func initialOpeningFactoryConfig() map[string]any {
	return map[string]any{
		"name": "initial-opening-retry",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "waiting", "type": "PROCESSING"},
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

	failed := newInitialOpeningScenario(t)
	canceled := newInitialOpeningScenario(t)
	reused := newInitialOpeningScenario(t)
	selected := newInitialOpeningProviderScenario(t)
	defaulted := newInitialOpeningDefaultProviderScenario(t, "", "")
	parameterized := newInitialOpeningDefaultProviderScenario(t, "", "${model}")
	durable := newInitialOpeningScenario(t)
	child := newInitialOpeningChildScenario(t)
	checkout := newInitialOpeningWorktreeScenario(t)
	// An authored input directory makes initial activation emit its scoped
	// diagnostic, so selected backend propagation has an observable witness.
	for _, dir := range []string{reused.candidateDir, reused.peerDir} {
		if err := os.MkdirAll(filepath.Join(dir, factorydefinitions.InputsDir), 0o755); err != nil {
			t.Fatalf("prepare authored inputs: %v", err)
		}
	}
	effects := &initialOpeningEffects{calls: make(map[string]int)}
	persistence := &initialOpeningPersistence{effects: effects, sessionID: durable.candidateID, saved: make(chan struct{})}
	gate := &initialOpeningGate{entered: make(chan struct{}), release: make(chan struct{})}
	files := &initialOpeningDirectories{
		gatedPath: filepath.Join(canceled.candidateDir, factorydefinitions.InputsDir),
		gate:      gate, effects: effects,
	}
	api := support.NewProcessAPIServer()
	logCore, logs := observer.New(zap.InfoLevel)
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		ProcessLogger:             zap.New(logCore).With(zap.String("selected_backend", "initial-opening")),
		Clock:                     initialOpeningClock{},
		FactoryRuntimeDirectories: files,
		FactorySessionRuntimePersistenceFileSystem: persistence,
		ScriptCommandRunner:                        initialOpeningScriptRunner{effects: effects},
		ProviderCommandRunner:                      initialOpeningProviderRunner{effects: effects},
		WorkersWorktreeGit:                         initialOpeningWorktreeGit{effects: effects},
		APIServerStarter:                           api.Start,
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
	t.Run("CLI Work reaches the opened durable mutation owner", func(t *testing.T) {
		t.Parallel()
		testInitialOpeningDurableMutation(t, sessions, process, durable, effects, persistence, api.WaitForURL(t))
	})
	t.Run("remote CLI child uses the explicitly opened session", func(t *testing.T) {
		t.Parallel()
		testInitialOpeningChildInvocation(t, sessions, process, child, effects, api.WaitForURL(t))
	})

	runInitialOpeningCompatibilityScenarios(t, sessions, process, api.WaitForURL(t), home)
	for _, recovery := range []string{"retained", "corrupt", "missing"} {
		t.Run("current board recovery "+recovery, func(t *testing.T) {
			t.Parallel()
			testInitialOpeningBoardRecovery(t, sessions, process, newInitialOpeningScenario(t), effects, logs, api.WaitForURL(t), recovery)
		})
	}

	t.Run("fixed observations select empty and completed Work", func(t *testing.T) {
		t.Parallel()
		testFixedOpeningReads(t, sessions, api.WaitForURL(t))
	})
	t.Run("failed resource opening retries with the same identity", func(t *testing.T) {
		t.Parallel()
		testFailedInitialOpeningRetry(t, sessions, process, failed, effects)
	})
	t.Run("cancellation while opening unwinds before same identity retry", func(t *testing.T) {
		t.Parallel()
		testCanceledInitialOpening(t, sessions, process, canceled, gate)
	})
	t.Run("closed recording preserves attributed history and live peer", func(t *testing.T) {
		t.Parallel()
		testInitialOpeningRecordedHistory(t, sessions, process, reused, logs)
	})
	t.Run("selected Codex provider executes independently attributed sessions", func(t *testing.T) {
		t.Parallel()
		testInitialOpeningProviderSelection(t, sessions, selected, effects)
	})
	t.Run("reused customer checkout survives session close and destination reuse", func(t *testing.T) {
		t.Parallel()
		testInitialOpeningWorktreeReuse(t, sessions, checkout, effects)
	})
	for name, scenario := range map[string]initialOpeningScenario{
		"missing settings": defaulted, "absent model invocation parameter": parameterized,
	} {
		t.Run("selected operator defaults supply "+name, func(t *testing.T) {
			t.Parallel()
			testInitialOpeningOperatorDefaults(t, sessions, scenario, effects)
		})
	}
}

func testInitialOpeningOperatorDefaults(t *testing.T, sessions factorysessions.Service, scenario initialOpeningScenario, effects *initialOpeningEffects) {
	t.Helper()
	peerHistory := scenario.startPeer(t, sessions)
	request := scenario.request()
	request.RuntimeSelection.OperatorDefaults = operatorsettings.ResolvedDefaults{
		WorkerModelProvider: string(modelprovider.ProviderCodex), WorkerModel: "gpt-5-codex",
	}
	startInitialOpeningSession(t, sessions, request)
	assertInitialOpeningInvocation(t, sessions, scenario.candidateID)
	assertInitialOpeningProviderSelection(t, effects, scenario.candidateDir)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
}

func testInitialOpeningProviderSelection(t *testing.T, sessions factorysessions.Service, selected initialOpeningScenario, effects *initialOpeningEffects) {
	t.Helper()
	peerHistory := selected.startPeer(t, sessions)
	startInitialOpeningSession(t, sessions, selected.request())
	assertInitialOpeningInvocation(t, sessions, selected.candidateID)
	initialOpeningHistory(t, sessions, selected.candidateID)
	assertInitialOpeningProviderSelection(t, effects, selected.candidateDir)
	assertInitialOpeningProviderSelection(t, effects, selected.peerDir)
	assertInitialOpeningHistoryPreserved(t, sessions, selected.peerID, peerHistory)
	assertInitialOpeningInvocation(t, sessions, selected.peerID)
}

func testInitialOpeningDurableMutation(t *testing.T, sessions factorysessions.Service, process support.Process, durable initialOpeningScenario, effects *initialOpeningEffects, persistence *initialOpeningPersistence, serverURL string) {
	t.Helper()
	peerHistory := durable.startPeer(t, sessions)
	request := durable.request()
	request.Persistence = factorysessions.PersistencePolicyEnabled
	startInitialOpeningSession(t, sessions, request)
	select {
	case <-persistence.saved:
		t.Fatal("activation persisted before CLI Work; cannot attribute the observation to its mutation callback")
	default:
	}
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", serverURL, "--session", durable.candidateID,
		"submit", "batch", fmt.Sprintf(`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":"durable-observation","workTypeName":"task","payload":{"title":"prove durable observation ownership"}}]}`, durable.candidateID),
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+durable.home, "USERPROFILE="+durable.home)
	inputs.Input.WorkingDirectory = durable.candidateDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("CLI invoke opened session: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	ctx, cancel := context.WithTimeout(t.Context(), initialOpeningReadCeiling)
	defer cancel()
	select {
	case <-persistence.saved:
	case <-ctx.Done():
		t.Fatalf("CLI Work did not reach its opened durable persistence effect: %v", ctx.Err())
	}
	writes := effects.forScenario(durable)
	persisted := false
	for path, count := range writes {
		if strings.HasSuffix(path, durable.candidateID+".json|session.persist") && count > 0 {
			persisted = true
		}
	}
	if !persisted {
		t.Fatalf("CLI Work produced no session-owned durable persistence effect: %v", writes)
	}
	assertInitialOpeningHistoryPreserved(t, sessions, durable.peerID, peerHistory)
	assertInitialOpeningInvocation(t, sessions, durable.peerID)
}

func testInitialOpeningRecordedHistory(t *testing.T, sessions factorysessions.Service, process support.Process, reused initialOpeningScenario, logs *observer.ObservedLogs) {
	t.Helper()
	peerHistory := reused.startPeer(t, sessions)
	request := reused.request()
	recordPath := filepath.Join(t.TempDir(), "opening.replay.jsonl")
	request.RuntimeSelection.Recording.RecordPath = recordPath
	startInitialOpeningSession(t, sessions, request)
	assertInitialOpeningDiagnostics(t, logs, reused.candidateID, reused.candidateDir)
	assertInitialOpeningDiagnostics(t, logs, reused.peerID, reused.peerDir)
	assertInitialOpeningInvocation(t, sessions, reused.candidateID)
	firstHistory := initialOpeningHistory(t, sessions, reused.candidateID)
	closeInitialOpeningSession(t, sessions, reused.candidateID)
	assertInitialOpeningReplay(t, process, recordPath, reused.candidateID, firstHistory)
	assertInitialOpeningHistoryPreserved(t, sessions, reused.peerID, peerHistory)
	assertInitialOpeningInvocation(t, sessions, reused.peerID)
}

func testFailedInitialOpeningRetry(t *testing.T, sessions factorysessions.Service, process support.Process, scenario initialOpeningScenario, effects *initialOpeningEffects) {
	t.Helper()
	peerHistory := scenario.startPeer(t, sessions)
	request := scenario.request()
	blockedPath := filepath.Join(scenario.candidateDir, factorydefinitions.InputsDir)
	if err := os.WriteFile(blockedPath, []byte("blocks input directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(t.TempDir(), "retry.replay.jsonl")
	request.RuntimeSelection.Recording.RecordPath = recordPath
	_, err := sessions.Start(t.Context(), request)
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || filepath.Clean(pathErr.Path) != blockedPath || pathErr.Op != "mkdir" {
		t.Fatalf("failed Start error = %v, want original input MkdirAll file error", err)
	}
	if err := os.Remove(blockedPath); err != nil {
		t.Fatal(err)
	}
	assertInitialOpeningNotPublished(t, sessions, scenario.candidateID)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
	// Reuse the failed opening's identity and destinations. A new filename
	// would conceal a partial recording left behind by failed activation.
	startInitialOpeningSession(t, sessions, request)
	assertInitialOpeningInvocation(t, sessions, scenario.candidateID)
	assertInitialOpeningDuplicate(t, sessions, scenario, effects)
	history := initialOpeningHistory(t, sessions, scenario.candidateID)
	closeInitialOpeningSession(t, sessions, scenario.candidateID)
	assertInitialOpeningReplay(t, process, recordPath, scenario.candidateID, history)
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
	assertFixedPeerReplay(t, sessions, process, scenario)
}

func assertInitialOpeningDiagnostics(t *testing.T, logs *observer.ObservedLogs, sessionID, dir string) {
	t.Helper()
	entries := logs.FilterMessage("using inputs/ directory").FilterField(zap.String("session_id", sessionID)).All()
	if len(entries) != 1 {
		t.Fatalf("session %s opening diagnostics = %d, want one selected-backend entry", sessionID, len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["selected_backend"] != "initial-opening" || fields["factory_dir"] != dir ||
		fields["folder_path"] != dir || fields["dir"] != filepath.Join(dir, factorydefinitions.InputsDir) {
		t.Fatalf("session %s opening diagnostic scope = %#v", sessionID, fields)
	}
}

func testCanceledInitialOpening(t *testing.T, sessions factorysessions.Service, process support.Process, scenario initialOpeningScenario, gate *initialOpeningGate) {
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
	assertFixedPeerReplay(t, sessions, process, scenario)
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
	return initialOpeningScenarioWithConfig(t, config)
}

func newInitialOpeningProviderScenario(t *testing.T) initialOpeningScenario {
	t.Helper()
	config := initialOpeningFactoryConfig()
	config["workTypes"].([]map[string]any)[0]["handlingBehavior"] = []string{"DEFAULT"}
	scenario := initialOpeningScenarioWithConfig(t, config)
	for _, dir := range []string{scenario.candidateDir, scenario.peerDir} {
		support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	}
	return scenario
}

func newInitialOpeningDefaultProviderScenario(t *testing.T, provider, model string) initialOpeningScenario {
	t.Helper()
	config := initialOpeningFactoryConfig()
	config["workTypes"].([]map[string]any)[0]["handlingBehavior"] = []string{"DEFAULT"}
	scenario := initialOpeningScenarioWithConfig(t, config)
	support.WriteAgentConfig(t, scenario.candidateDir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Provider(provider), model))
	support.WriteAgentConfig(t, scenario.peerDir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return scenario
}

func initialOpeningScenarioWithConfig(t *testing.T, config map[string]any) initialOpeningScenario {
	t.Helper()
	return initialOpeningScenario{
		candidateDir: support.ScaffoldFactory(t, config), peerDir: support.ScaffoldFactory(t, config),
		home: t.TempDir(), logs: t.TempDir(), metrics: t.TempDir(),
		candidateID: uuid.NewString(), runtimeID: uuid.NewString(), peerID: uuid.NewString(),
	}
}

type initialOpeningProviderRunner struct{ effects *initialOpeningEffects }

func (runner initialOpeningProviderRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if request.Command != string(modelprovider.ProviderCodex) || !strings.Contains(strings.Join(request.Args, " "), "gpt-5-codex") {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider command %q with arguments %v", request.Command, request.Args)
	}
	runner.effects.record("worker.codex", request.WorkDir)
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("initial opening COMPLETE")}, nil
}

func assertInitialOpeningProviderSelection(t *testing.T, effects *initialOpeningEffects, dir string) {
	t.Helper()
	scenario := initialOpeningScenario{candidateDir: dir}
	calls := effects.forScenario(scenario)
	if calls[filepath.Clean(dir)+"|worker.codex"] != 1 || calls[filepath.Clean(dir)+"|worker.run"] != 0 {
		t.Fatalf("selected provider effects for %s = %v, want one Codex execution", dir, calls)
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
	request.RuntimeSelection.Recording.RecordPath = filepath.Join(scenario.peerDir, "peer.jsonl")
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

func assertInitialOpeningInvocation(t *testing.T, sessions factorysessions.Service, sessionID string) factorysessions.InvocationResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), initialOpeningReadCeiling)
	defer cancel()
	result, err := sessions.Invoke(ctx, factorysessions.SessionInvokeRequest{
		SessionID: sessionID, ContentProvided: true,
		Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: sessionID + " prove this session remains usable"}},
	})
	if err != nil || result.Status != factorysessions.InvocationTerminalStatusCompleted || len(result.PrimaryResult) != 1 || result.PrimaryResult[0].Text != "initial opening COMPLETE" {
		t.Fatalf("Invoke %s = %#v, %v, want completed controlled worker output", sessionID, result, err)
	}
	return result
}

func initialOpeningHistory(t *testing.T, sessions factorysessions.Service, sessionID string) *factorydefinitions.FactoryEventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	stream, err := sessions.SubscribeFactoryEventsForSession(ctx, sessionID, nil)
	if err != nil || stream == nil || len(stream.History) == 0 {
		t.Fatalf("session %s retained events = %#v, %v, want public history", sessionID, stream, err)
	}
	for _, event := range stream.History {
		if !event.Context.EventTime.Equal((initialOpeningClock{}).Now()) {
			t.Fatalf("session %s event %s (%s) time = %s, want selected process time %s",
				sessionID, event.Id, event.Type, event.Context.EventTime, (initialOpeningClock{}).Now())
		}
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
	gatedPath string
	gate      *initialOpeningGate
	gated     atomic.Bool
	effects   *initialOpeningEffects
}

func (files *initialOpeningDirectories) Stat(path string) (fs.FileInfo, error) {
	files.effects.record("runtime.stat", path)
	return os.Stat(path)
}

func (files *initialOpeningDirectories) MkdirAll(path string, mode fs.FileMode) error {
	files.effects.record("runtime.mkdir", path)
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

// Observe the selected persistence effect without decoding private snapshots.
// A Petri Work completion persists through its opening-owned mutation callback.
type initialOpeningPersistence struct {
	platformfilesystem.Local
	effects   *initialOpeningEffects
	sessionID string
	saved     chan struct{}
	once      sync.Once
}

func (files *initialOpeningPersistence) WriteFile(path string, content []byte, mode fs.FileMode) error {
	if err := files.Local.WriteFile(path, content, mode); err != nil {
		return err
	}
	files.effects.record("session.persist", path)
	if filepath.Base(path) == files.sessionID+".json" {
		files.once.Do(func() { close(files.saved) })
	}
	return nil
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

func testFixedOpeningReads(t *testing.T, sessions factorysessions.Service, baseURL string) {
	t.Helper()
	scenario := newInitialOpeningScenario(t)
	request := scenario.request()
	request.RuntimeSelection.Recording.RecordPath = filepath.Join(t.TempDir(), "selected.jsonl")
	startInitialOpeningSession(t, sessions, request)
	peer := scenario.request()
	peer.SessionID = scenario.peerID
	peer.FolderPath = scenario.peerDir
	peer.RuntimeSelection.RuntimeInstanceID = uuid.NewString()
	peer.RuntimeSelection.DefinitionSourcePath = filepath.Join(scenario.peerDir, "factory.json")
	peer.RuntimeSelection.ExecutionBaseDir = scenario.peerDir
	peer.RuntimeSelection.Recording.RecordPath = filepath.Join(t.TempDir(), "peer.jsonl")
	startInitialOpeningSession(t, sessions, peer)
	for _, id := range []string{scenario.candidateID, scenario.peerID} {
		listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, id, "/work"))
		if len(listed.Results) != 0 {
			t.Fatalf("empty selected Work %s = %#v", id, listed)
		}
		_, err := sessions.ReadResult(t.Context(), factorysessions.SessionResultReadRequest{SessionID: id, Mode: factorysessions.SessionOperationModeLive})
		if !errors.Is(err, factorysessions.ErrResultUnavailable) {
			t.Fatalf("empty Petri result %s = %v", id, err)
		}
	}
	for _, id := range []string{scenario.candidateID, scenario.peerID} {
		assertFixedCompletedWorkReads(t, sessions, baseURL, id)
	}
	for _, suffix := range []string{"/state", "/work", "/work/unknown-work"} {
		assertFixedMissingRead(t, support.SessionWorkURL(baseURL, "unknown-fixed-observation", suffix), suffix)
	}
	_, err := sessions.ReadResult(t.Context(), factorysessions.SessionResultReadRequest{SessionID: "unknown-fixed-observation", Mode: factorysessions.SessionOperationModeLive})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing selected result = %v", err)
	}
}

func assertFixedCompletedWorkReads(t *testing.T, sessions factorysessions.Service, baseURL, id string) {
	t.Helper()
	assertInitialOpeningInvocation(t, sessions, id)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, id, "/work"))
	if len(listed.Results) != 1 || listed.Results[0].WorkId == nil || fmt.Sprint(listed.Results[0].Payload) != id+" prove this session remains usable" {
		t.Fatalf("selected Work %s = %#v", id, listed)
	}
	detail := support.GetJSON[factoryapi.WorkRead](t, support.SessionWorkURL(baseURL, id, "/work/"+url.PathEscape(*listed.Results[0].WorkId)))
	if detail.WorkId == nil || *detail.WorkId != *listed.Results[0].WorkId || fmt.Sprint(detail.Payload) != id+" prove this session remains usable" || detail.State == nil || detail.State.Name != "complete" {
		t.Fatalf("selected detail = %#v", detail)
	}
	for i := 0; i < 2; i++ {
		_, err := sessions.ReadResult(t.Context(), factorysessions.SessionResultReadRequest{SessionID: id, Mode: factorysessions.SessionOperationModeLive})
		if !errors.Is(err, factorysessions.ErrResultUnavailable) {
			t.Fatalf("completed Petri result %s = %v", id, err)
		}
	}
	initialOpeningHistory(t, sessions, id)
}

// Only this scenario's recording destination faults; concurrent peers use the
// same composed Recordings owner with independent external effects.
type fixedRecordingFault struct {
	directory string
	cause     error
	fail      atomic.Bool
}

func (fault *fixedRecordingFault) write(path string, data []byte) error {
	if fault.fail.Load() && strings.HasPrefix(filepath.Clean(path), fault.directory+string(filepath.Separator)) {
		return fault.cause
	}
	return os.WriteFile(path, data, 0o600)
}

func testFixedRecordingFault(t *testing.T, sessions factorysessions.Service, process support.Process, fault *fixedRecordingFault) {
	t.Helper()
	scenario := newInitialOpeningScenario(t)
	peer := scenario.startPeer(t, sessions)
	request := scenario.request()
	request.RuntimeSelection.Recording.RecordPath = filepath.Join(fault.directory, "fault.json")
	if _, err := sessions.Start(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	assertInitialOpeningInvocation(t, sessions, scenario.candidateID)
	fault.fail.Store(true)
	defer fault.fail.Store(false)
	result, err := sessions.Control(t.Context(), factorysessions.SessionControlRequest{
		SessionID: scenario.candidateID, Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlClose,
	})
	if !errors.Is(err, fault.cause) || result.Closed {
		t.Fatalf("faulted recording close = %#v, %v, want selected write cause and no false close success", result, err)
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peer)
	assertInitialOpeningInvocation(t, sessions, scenario.peerID)
	assertFixedPeerReplay(t, sessions, process, scenario)
}

func assertFixedPeerReplay(t *testing.T, sessions factorysessions.Service, process support.Process, scenario initialOpeningScenario) {
	t.Helper()
	history := initialOpeningHistory(t, sessions, scenario.peerID)
	closeInitialOpeningSession(t, sessions, scenario.peerID)
	assertInitialOpeningReplay(t, process, filepath.Join(scenario.peerDir, "peer.jsonl"), scenario.peerID, history)
}

func assertFixedMissingRead(t *testing.T, endpoint, suffix string) {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var diagnostic factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&diagnostic); err != nil {
		t.Fatal(err)
	}
	wantStatus, wantCode := http.StatusNotFound, factoryapi.ErrorResponseCodeNOTFOUND
	if strings.HasPrefix(suffix, "/work") {
		wantStatus, wantCode = http.StatusInternalServerError, factoryapi.ErrorResponseCode("INTERNAL_ERROR")
	}
	if response.StatusCode != wantStatus || diagnostic.Code != wantCode {
		t.Fatalf("missing selected read = %d, %#v", response.StatusCode, diagnostic)
	}
	// Missing identities must not expose filesystem or provider credentials.
	text := fmt.Sprint(diagnostic)
	for _, secret := range []string{"Bearer ", "sk-", "USERPROFILE", "C:\\Users"} {
		if strings.Contains(text, secret) {
			t.Fatalf("unsafe selected read diagnostic: %s", text)
		}
	}
}

// A terminal recording fault deliberately survives cleanup. This fixture owns
// its process so that the expected retained failure cannot contaminate peers
// in the reusable healthy-process matrix.
func TestFixedObservationTerminalFlushFailure(t *testing.T) {
	t.Parallel()
	fault := &fixedRecordingFault{directory: t.TempDir(), cause: errors.New("selected recording write failed")}
	api := support.NewProcessAPIServer()
	effects := &initialOpeningEffects{calls: make(map[string]int)}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		Clock: initialOpeningClock{}, ScriptCommandRunner: initialOpeningScriptRunner{effects: effects},
		RecordingWriteFile: fault.write, APIServerStarter: api.Start,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), initialOpeningReadCeiling)
		defer cancel()
		err := process.Close(ctx)
		if !errors.Is(err, fault.cause) {
			t.Errorf("terminal process cleanup = %v, want retained recording cause", err)
		}
	})
	directory, home := support.ScaffoldFactory(t, initialOpeningFactoryConfig()), t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", directory, "--continuously", "--with-server", "--quiet", "--no-record"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = directory
	command := support.StartProcessCommand(t, process, inputs.Input)
	api.WaitForURL(t)
	sessions := process.FactorySessions().FactorySessions().(factorysessions.Service)
	testFixedRecordingFault(t, sessions, process, fault)
	// Execute may also surface the already asserted terminal resource error.
	command.AcceptError()
}
