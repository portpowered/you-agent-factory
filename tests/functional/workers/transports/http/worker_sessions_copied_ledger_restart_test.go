package http_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	copiedLedgerReplayTargetName   = "worker-session-copied-ledger-target"
	copiedLedgerReplayFailureName  = "worker-session-copied-ledger-failure"
	copiedLedgerReplaySiblingCount = 5
	copiedLedgerReplayTimeout      = 90 * time.Second
	copiedLedgerReplayTargetModel  = "copied-ledger-target-model"
	copiedLedgerReplaySiblingModel = "copied-ledger-sibling-model"
	copiedLedgerReplayFailureModel = "copied-ledger-failure-model"
)

type copiedLedgerReplayFixture struct {
	server                *support.FunctionalAPIServer
	factoryDir            string
	factoryID             string
	runtimeInstanceID     string
	homeDir               string
	recordPath            string
	workerRecordingPath   string
	workerRecordingWriter *copiedLedgerWorkerRecordingWriter
	runner                *copiedLedgerReplayRunner
	releaseFirst          func()
}

type copiedLedgerWorkerRecordingWriter struct {
	delegate      *remoteWorkerRecordingStore
	directory     string
	mu            sync.RWMutex
	persistenceMu sync.Mutex
	identity      string
}

func (writer *copiedLedgerWorkerRecordingWriter) PersistWorkerRecord(
	ctx context.Context,
	record recordings.WorkerRecordingRecord,
) error {
	return writer.persist(ctx, record.RecordingID, func() error {
		return writer.delegate.PersistWorkerRecord(ctx, record)
	})
}

func (writer *copiedLedgerWorkerRecordingWriter) PersistWorkerRecordingFailure(
	ctx context.Context,
	failure recordings.WorkerRecordingFailure,
) error {
	return writer.persist(ctx, failure.RecordingID, func() error {
		return writer.delegate.PersistWorkerRecordingFailure(ctx, failure)
	})
}

func (writer *copiedLedgerWorkerRecordingWriter) LoadWorkerRecording(
	ctx context.Context,
	recordingID string,
) (recordings.WorkerRecordingSnapshot, error) {
	return writer.delegate.LoadWorkerRecording(ctx, recordingID)
}

func (writer *copiedLedgerWorkerRecordingWriter) persist(
	ctx context.Context,
	recordingID string,
	operation func() error,
) error {
	if err := writer.rememberIdentity(recordingID); err != nil {
		return err
	}
	writer.persistenceMu.Lock()
	defer writer.persistenceMu.Unlock()
	if err := operation(); err != nil {
		return err
	}
	snapshot, err := writer.delegate.LoadWorkerRecording(ctx, recordingID)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode copied-ledger Worker recording: %w", err)
	}
	fileName := hex.EncodeToString([]byte(recordingID)) + ".json"
	path := filepath.Join(writer.directory, fileName)
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return fmt.Errorf("persist copied-ledger Worker recording: %w", err)
	}
	return nil
}

func (writer *copiedLedgerWorkerRecordingWriter) rememberIdentity(identity string) error {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return recordings.ErrInvalidWorkerRecordingRequest
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.identity != "" && writer.identity != identity {
		return fmt.Errorf("copied-ledger fixture observed Worker recording identities %q and %q", writer.identity, identity)
	}
	writer.identity = identity
	return nil
}

func (writer *copiedLedgerWorkerRecordingWriter) recordingIdentity() string {
	writer.mu.RLock()
	defer writer.mu.RUnlock()
	return writer.identity
}

func newCopiedLedgerWorkerRecordingWriter(directory string) (*copiedLedgerWorkerRecordingWriter, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create copied-ledger Worker recording directory: %w", err)
	}
	delegate := newRemoteWorkerRecordingStore()
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read copied-ledger Worker recording directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read copied-ledger Worker recording snapshot: %w", err)
		}
		var snapshot recordings.WorkerRecordingSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return nil, fmt.Errorf("decode copied-ledger Worker recording snapshot: %w", err)
		}
		if strings.TrimSpace(snapshot.RecordingID) == "" {
			return nil, recordings.ErrInvalidWorkerRecordingRequest
		}
		delegate.snapshots[snapshot.RecordingID] = cloneRemoteWorkerRecordingSnapshot(snapshot)
	}
	return &copiedLedgerWorkerRecordingWriter{delegate: delegate, directory: directory}, nil
}

type copiedLedgerReplayRunner struct {
	homeDir       string
	successOutput []byte
	failureOutput []byte
	rollout       []byte
	release       <-chan struct{}
	targetStarted chan struct{}
	calls         atomic.Int32
	targetOnce    sync.Once
}

type copiedLedgerPublicSnapshot struct {
	inventory   factoryapi.ListWorkResponse
	works       map[string]factoryapi.Work
	lists       map[string]factoryapi.ListWorkerSessionsResponse
	details     map[string]factoryapi.WorkerSessionObservation
	transcripts map[string]factoryapi.WorkerSessionTranscriptResponse
	events      map[string][]factoryapi.WorkerSessionEvent
}

// TestWorkerSessionCopiedLedgerRestartPreservesWorkTranscriptAndCursor proves
// scoped attempt, provider transcript, failure, timing, and exclusive cursor
// behavior after a copied recording and provider store are opened by a fresh
// root-composed process.
func TestWorkerSessionCopiedLedgerRestartPreservesWorkTranscriptAndCursor(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()

	fixture := startCopiedLedgerReplayFixture(t)
	t.Cleanup(func() { fixture.server.Stop(t) })
	workIDsByName := submitCopiedLedgerWorkBatch(t, fixture.server.URL(), fixture.factoryID)
	targetWorkID := workIDsByName[copiedLedgerReplayTargetName]
	failureWorkID := workIDsByName[copiedLedgerReplayFailureName]
	siblingWorkIDs := make([]string, 0, copiedLedgerReplaySiblingCount)
	for index := 0; index < copiedLedgerReplaySiblingCount; index++ {
		name := fmt.Sprintf("worker-session-copied-ledger-sibling-%02d", index+1)
		siblingWorkIDs = append(siblingWorkIDs, workIDsByName[name])
	}
	firstWorkerSessionID, firstLiveEvents := runCopiedLedgerTargetWork(t, ctx, fixture, targetWorkID)
	runCopiedLedgerSiblingWorks(t, fixture, siblingWorkIDs)
	runCopiedLedgerFailureWork(t, fixture, failureWorkID)
	workIDs := append([]string{targetWorkID}, siblingWorkIDs...)
	workIDs = append(workIDs, failureWorkID)
	support.WaitForSessionTerminalStatus(t, fixture.server.URL(), fixture.factoryID, copiedLedgerReplayTimeout)
	for _, workID := range workIDs {
		waitCopiedLedgerWorkConfirmed(t, fixture.server.URL(), fixture.factoryID, workID)
	}

	before := captureCopiedLedgerSnapshot(t, fixture.server.URL(), fixture.factoryID, workIDs, targetWorkID, failureWorkID)
	workerRecordingID := fixture.workerRecordingWriter.recordingIdentity()
	if workerRecordingID == "" {
		t.Fatal("Worker Session execution did not persist a source-native recording identity")
	}
	beforeWorkerRecording, err := fixture.server.WorkerRecordingReader().LoadWorkerRecording(ctx, workerRecordingID)
	if err != nil {
		t.Fatalf("load source-native Worker recording before copy: %v", err)
	}
	assertCopiedLedgerWorkerRecordingTimings(t, beforeWorkerRecording, before)
	assertCopiedLedgerWorkRows(t, before.lists[targetWorkID], fixture.factoryID, targetWorkID, 2, factoryapi.WorkerSessionObservationStateCompleted)
	assertCopiedLedgerWorkRows(t, before.lists[failureWorkID], fixture.factoryID, failureWorkID, 1, factoryapi.WorkerSessionObservationStateFailed)
	if before.details[firstWorkerSessionID].State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("first Worker Session detail = %#v, want COMPLETED", before.details[firstWorkerSessionID])
	}
	if len(firstLiveEvents) < 2 || firstLiveEvents[0].Delivery != "RECORD" {
		t.Fatalf("first Worker Session live replay = %#v, want retained record followed by terminal delivery", firstLiveEvents)
	}

	fixture.server.Stop(t)
	copyDirectory := t.TempDir()
	copiedRecording := copyCopiedLedgerFile(t, fixture.recordPath, filepath.Join(copyDirectory, "worker-session-recording.json"))
	copiedFactoryDir := filepath.Join(copyDirectory, "factory")
	copyCopiedLedgerDirectory(t, fixture.factoryDir, copiedFactoryDir)
	replayHome := filepath.Join(copyDirectory, "home")
	copyCopiedLedgerDirectory(t,
		filepath.Join(fixture.homeDir, ".codex", "sessions"),
		filepath.Join(replayHome, ".codex", "sessions"),
	)
	copiedWorkerRecordingPath := filepath.Join(copyDirectory, "worker-recordings")
	copyCopiedLedgerDirectory(t, fixture.workerRecordingPath, copiedWorkerRecordingPath)
	resumedServer := startCopiedLedgerResumeProcess(t, copiedFactoryDir, fixture.factoryID, copiedRecording, replayHome, copiedWorkerRecordingPath, fixture.runtimeInstanceID)
	t.Cleanup(func() { resumedServer.Stop(t) })
	support.WaitForSessionTerminalStatus(t, resumedServer.URL(), fixture.factoryID, copiedLedgerReplayTimeout)

	after := captureCopiedLedgerSnapshot(t, resumedServer.URL(), fixture.factoryID, workIDs, targetWorkID, failureWorkID)
	afterWorkerRecording, err := resumedServer.WorkerRecordingReader().LoadWorkerRecording(ctx, workerRecordingID)
	if err != nil {
		t.Fatalf("load copied source-native Worker recording after restart: %v", err)
	}
	if !reflect.DeepEqual(beforeWorkerRecording, afterWorkerRecording) {
		t.Fatalf("copied source-native Worker recording changed after restart:\nbefore=%#v\nafter=%#v", beforeWorkerRecording, afterWorkerRecording)
	}
	assertCopiedLedgerWorkerRecordingTimings(t, afterWorkerRecording, after)
	assertCopiedLedgerWorkInventory(t, before.inventory, workIDs)
	assertCopiedLedgerWorkInventory(t, after.inventory, workIDs)
	assertCopiedLedgerSnapshotsEqual(t, before, after)
	assertCopiedLedgerWorkRows(t, after.lists[targetWorkID], fixture.factoryID, targetWorkID, 2, factoryapi.WorkerSessionObservationStateCompleted)
	assertCopiedLedgerWorkRows(t, after.lists[failureWorkID], fixture.factoryID, failureWorkID, 1, factoryapi.WorkerSessionObservationStateFailed)
	assertCopiedLedgerRepeatedCursorResume(t, resumedServer.URL(), fixture.factoryID, firstWorkerSessionID, after.events[firstWorkerSessionID])
	assertCopiedLedgerSyncPreflight(t, resumedServer.URL(), fixture.factoryID)
	assertCopiedLedgerCLIParity(t, resumedServer, fixture.factoryID, targetWorkID, firstWorkerSessionID, after)
}

func assertCopiedLedgerSyncPreflight(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	if len(events) == 0 {
		t.Fatal("copied Factory Session has no retained Factory Events for reconnect validation")
	}
	cursor := strings.TrimSpace(events[0].Id)
	if cursor == "" {
		t.Fatalf("first retained Factory Event has no stable ID: %#v", events[0])
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) +
		"/sync-preflight?after_event_id=" + url.QueryEscape(cursor)
	preflight := support.GetJSON[factoryapi.FactorySessionSyncPreflightResponse](t, endpoint)
	if preflight.ReasonCode != factoryapi.Ok || !preflight.CheckpointReusable {
		t.Fatalf("copied-ledger sync preflight = %#v, want reusable cursor", preflight)
	}
	if preflight.RequestedSessionId != sessionID || preflight.FactorySessionId == nil || *preflight.FactorySessionId != sessionID {
		t.Fatalf("copied-ledger sync preflight identity = %#v, want requested and resolved session %q", preflight, sessionID)
	}
	if !preflight.ReconnectCursor.Provided || !preflight.ReconnectCursor.ValidForStreamGeneration ||
		preflight.ReconnectCursor.AfterEventId == nil || *preflight.ReconnectCursor.AfterEventId != cursor {
		t.Fatalf("copied-ledger sync preflight cursor = %#v, want validated event ID %q", preflight.ReconnectCursor, cursor)
	}
}

func startCopiedLedgerReplayFixture(t *testing.T) copiedLedgerReplayFixture {
	t.Helper()
	factoryDir := support.ScaffoldFactory(t, copiedLedgerReplayFactoryConfig())
	support.WriteAgentConfig(t, factoryDir, "target-processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, copiedLedgerReplayTargetModel))
	support.WriteAgentConfig(t, factoryDir, "sibling-processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, copiedLedgerReplaySiblingModel))
	support.WriteAgentConfig(t, factoryDir, "failure-processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, copiedLedgerReplayFailureModel))
	homeDir := t.TempDir()
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseFirst := func() { releaseOnce.Do(func() { close(release) }) }
	runner := &copiedLedgerReplayRunner{
		homeDir:       homeDir,
		successOutput: readRemoteProviderFixture(t, "codex", "success", "stdout.jsonl"),
		failureOutput: readRemoteProviderFixture(t, "codex", "structured-failure", "stdout.jsonl"),
		rollout:       readRemoteProviderFixture(t, "codex", "success", "rollout.jsonl"),
		release:       release,
		targetStarted: make(chan struct{}),
	}
	factoryID := uuid.NewString()
	runtimeInstanceID := "copied-ledger-runtime-" + uuid.NewString()
	recordPath := filepath.Join(t.TempDir(), "worker-session-copied-ledger.recording.json")
	workerRecordingPath := filepath.Join(t.TempDir(), "worker-recordings")
	workerRecordingWriter, err := newCopiedLedgerWorkerRecordingWriter(workerRecordingPath)
	if err != nil {
		t.Fatalf("create copied-ledger Worker recording edge: %v", err)
	}
	env := remoteFunctionalEnvironment(homeDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       env,
		Args:                      []string{"--session", factoryID, "--record", recordPath},
		Edges: serviceedges.Edges{
			ProviderCommandRunner:                    runner,
			ProviderSessionResolveHomeDirectory:      func() (string, error) { return homeDir, nil },
			FactorySessionRuntimeInstanceIDGenerator: func() string { return runtimeInstanceID },
			WorkerRecordingWriter:                    workerRecordingWriter,
		},
	})
	return copiedLedgerReplayFixture{
		server: server, factoryDir: factoryDir, factoryID: factoryID, runtimeInstanceID: runtimeInstanceID,
		homeDir: homeDir, recordPath: recordPath, workerRecordingPath: workerRecordingPath,
		workerRecordingWriter: workerRecordingWriter, runner: runner, releaseFirst: releaseFirst,
	}
}

func copiedLedgerReplayFactoryConfig() map[string]any {
	return map[string]any{
		"name": "worker-session-copied-ledger-replay",
		"workTypes": []any{
			copiedLedgerWorkType("target-task", "review"),
			copiedLedgerWorkType("sibling-task"),
			copiedLedgerWorkType("failure-task"),
		},
		"workers": []any{
			map[string]any{"name": "target-processor"},
			map[string]any{"name": "sibling-processor"},
			map[string]any{"name": "failure-processor"},
		},
		"workstations": []map[string]any{
			{
				"name": "process-target", "worker": "target-processor",
				"inputs":    []any{map[string]any{"workType": "target-task", "state": "init"}},
				"outputs":   []any{map[string]any{"workType": "target-task", "state": "review"}},
				"onFailure": []any{map[string]any{"workType": "target-task", "state": "failed"}},
			},
			{
				"name": "review-target", "worker": "target-processor",
				"inputs":    []any{map[string]any{"workType": "target-task", "state": "review"}},
				"outputs":   []any{map[string]any{"workType": "target-task", "state": "complete"}},
				"onFailure": []any{map[string]any{"workType": "target-task", "state": "failed"}},
			},
			{
				"name": "process-sibling", "worker": "sibling-processor",
				"inputs":    []any{map[string]any{"workType": "sibling-task", "state": "init"}},
				"outputs":   []any{map[string]any{"workType": "sibling-task", "state": "complete"}},
				"onFailure": []any{map[string]any{"workType": "sibling-task", "state": "failed"}},
			},
			{
				"name": "process-failure", "worker": "failure-processor",
				"inputs":    []any{map[string]any{"workType": "failure-task", "state": "init"}},
				"outputs":   []any{map[string]any{"workType": "failure-task", "state": "complete"}},
				"onFailure": []any{map[string]any{"workType": "failure-task", "state": "failed"}},
			},
		},
	}
}

func copiedLedgerWorkType(name string, processStates ...string) map[string]any {
	states := []any{map[string]any{"name": "init", "type": "INITIAL"}}
	for _, state := range processStates {
		states = append(states, map[string]any{"name": state, "type": "PROCESSING"})
	}
	states = append(states,
		map[string]any{"name": "complete", "type": "TERMINAL"},
		map[string]any{"name": "failed", "type": "FAILED"},
	)
	return map[string]any{"name": name, "states": states}
}

func (runner *copiedLedgerReplayRunner) Run(ctx context.Context, request process.CommandRequest) (process.CommandResult, error) {
	call := runner.calls.Add(1)
	providerSessionID := fmt.Sprintf("worker-session-replay-provider-%03d", call)
	if err := runner.writeProviderTranscript(providerSessionID); err != nil {
		return process.CommandResult{}, err
	}
	output := runner.successOutput
	oldSessionID := "session_fixture_codex_success"
	model := copiedLedgerModelArgument(request.Args)
	switch model {
	case copiedLedgerReplayTargetModel:
		runner.targetOnce.Do(func() { close(runner.targetStarted) })
		select {
		case <-runner.release:
		case <-ctx.Done():
			return process.CommandResult{}, ctx.Err()
		}
	case copiedLedgerReplayFailureModel:
		output = runner.failureOutput
		oldSessionID = "session_fixture_codex_structured_failure"
	case copiedLedgerReplaySiblingModel:
	default:
		return process.CommandResult{}, fmt.Errorf("unexpected copied-ledger Provider model %q in arguments %q", model, request.Args)
	}
	output = bytes.ReplaceAll(output, []byte(oldSessionID), []byte(providerSessionID))
	return process.CommandResult{Stdout: output}, nil
}

func copiedLedgerModelArgument(args []string) string {
	for index, argument := range args {
		if argument == "--model" && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

func (runner *copiedLedgerReplayRunner) writeProviderTranscript(sessionID string) error {
	directory := filepath.Join(runner.homeDir, ".codex", "sessions", "2026", "07", "27")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create controlled Provider Session directory: %w", err)
	}
	path := filepath.Join(directory, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, runner.rollout, 0o600); err != nil {
		return fmt.Errorf("write controlled Provider Session transcript: %w", err)
	}
	return nil
}

func (runner *copiedLedgerReplayRunner) waitTargetStarted(t *testing.T) {
	t.Helper()
	select {
	case <-runner.targetStarted:
	case <-time.After(15 * time.Second):
		t.Fatalf("timed out waiting for the target Work Provider Session to start; provider calls=%d", runner.calls.Load())
	}
}

func runCopiedLedgerTargetWork(
	t *testing.T,
	ctx context.Context,
	fixture copiedLedgerReplayFixture,
	workID string,
) (string, []factoryapi.WorkerSessionEvent) {
	t.Helper()
	fixture.runner.waitTargetStarted(t)
	inFlight := waitCopiedLedgerInFlightWorkerSession(t, fixture.server.URL(), fixture.factoryID, workID)
	workerSessionID := inFlight.Sessions[0].WorkerSessionId
	response, cancelStream := openFactoryWorkerSessionEventStream(t, fixture.server.URL(), fixture.factoryID, workerSessionID)
	defer cancelStream()
	defer response.Body.Close()
	firstFrame := make(chan factoryapi.WorkerSessionEvent, 1)
	streamDone := make(chan workerSessionTerminalEventsResult, 1)
	go func() {
		frames, phase, err := readWorkScopedWorkerSessionEventStream(response, firstFrame)
		streamDone <- workerSessionTerminalEventsResult{frames: frames, phase: phase, err: err}
	}()
	var retained factoryapi.WorkerSessionEvent
	select {
	case retained = <-firstFrame:
	case result := <-streamDone:
		t.Fatalf("Worker Session stream ended before retained delivery: phase=%s err=%v", result.phase, result.err)
	case <-ctx.Done():
		t.Fatalf("wait for retained Worker Session event: %v", ctx.Err())
	}
	if retained.Delivery != "RECORD" {
		t.Fatalf("first live Worker Session event delivery = %q, want retained RECORD", retained.Delivery)
	}
	assertWorkScopedWorkerSessionEventIdentity(t, retained, fixture.factoryID, workerSessionID, workID)
	fixture.releaseFirst()
	state := waitCopiedLedgerWorkState(t, fixture.server.URL(), fixture.factoryID, workID, "complete")
	if state.State == nil || state.State.Name != "complete" {
		t.Fatalf("target Work after two workstation attempts = %#v, want complete", state.State)
	}
	var stream workerSessionTerminalEventsResult
	select {
	case stream = <-streamDone:
	case <-ctx.Done():
		t.Fatalf("wait for terminal Worker Session stream: %v", ctx.Err())
	}
	if stream.err != nil || stream.phase != "TERMINAL" {
		t.Fatalf("live Worker Session stream phase=%q err=%v frames=%#v", stream.phase, stream.err, stream.frames)
	}
	assertWorkScopedWorkerSessionEventSequence(t, stream.frames, fixture.factoryID, workerSessionID, workID, retained)
	return workerSessionID, stream.frames
}

func runCopiedLedgerSiblingWorks(t *testing.T, fixture copiedLedgerReplayFixture, workIDs []string) {
	t.Helper()
	for _, workID := range workIDs {
		waitCopiedLedgerWorkState(t, fixture.server.URL(), fixture.factoryID, workID, "complete")
		waitCopiedLedgerWorkerSessions(t, fixture.server.URL(), fixture.factoryID, workID, 1)
	}
}

func runCopiedLedgerFailureWork(t *testing.T, fixture copiedLedgerReplayFixture, workID string) {
	t.Helper()
	waitCopiedLedgerWorkState(t, fixture.server.URL(), fixture.factoryID, workID, "failed")
	listed := waitCopiedLedgerWorkerSessions(t, fixture.server.URL(), fixture.factoryID, workID, 1)
	if listed.Sessions[0].Failure == nil || listed.Sessions[0].State != factoryapi.WorkerSessionObservationStateFailed {
		t.Fatalf("failed Work Worker Session = %#v, want one typed failed attempt", listed.Sessions[0])
	}
}

func submitCopiedLedgerWorkBatch(t *testing.T, baseURL, factoryID string) map[string]string {
	t.Helper()
	works := make([]factoryapi.Work, 0, copiedLedgerReplaySiblingCount+2)
	appendWork := func(name string) {
		workTypeName := copiedLedgerWorkTypeForName(name)
		works = append(works, factoryapi.Work{
			Name: name, WorkTypeName: &workTypeName,
		})
	}
	appendWork(copiedLedgerReplayTargetName)
	for index := 0; index < copiedLedgerReplaySiblingCount; index++ {
		appendWork(fmt.Sprintf("worker-session-copied-ledger-sibling-%02d", index+1))
	}
	appendWork(copiedLedgerReplayFailureName)
	requestID := "worker-session-copied-ledger-" + uuid.NewString()
	request := factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:     &works,
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal copied-ledger Work batch: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(factoryID) + "/work-requests/" + url.PathEscape(requestID)
	httpRequest, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build copied-ledger Work batch request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("submit copied-ledger Work batch: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		responseBody, _ := io.ReadAll(response.Body)
		t.Fatalf("submit copied-ledger Work batch status=%d body=%s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var submitted factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(response.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode copied-ledger Work batch response: %v", err)
	}
	if submitted.RequestId != requestID || len(submitted.Works) != len(works) {
		t.Fatalf("copied-ledger Work batch response = %#v, want request %q and %d Works", submitted, requestID, len(works))
	}
	workIDs := make(map[string]string, len(submitted.Works))
	for _, work := range submitted.Works {
		if strings.TrimSpace(work.WorkId) == "" {
			t.Fatalf("copied-ledger Work batch returned no stable ID: %#v", work)
		}
		workIDs[work.Name] = work.WorkId
	}
	if len(workIDs) != len(works) {
		t.Fatalf("copied-ledger Work batch response has %d unique names, want %d: %#v", len(workIDs), len(works), workIDs)
	}
	return workIDs
}

func copiedLedgerWorkTypeForName(name string) string {
	switch name {
	case copiedLedgerReplayTargetName:
		return "target-task"
	case copiedLedgerReplayFailureName:
		return "failure-task"
	default:
		return "sibling-task"
	}
}

func waitCopiedLedgerWorkerSessions(
	t *testing.T,
	baseURL, factoryID, workID string,
	wantCount int,
) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	deadline := time.NewTimer(copiedLedgerReplayTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var last factoryapi.ListWorkerSessionsResponse
	for {
		last = support.ListSessionWorkerSessions(t, baseURL, factoryID, workID)
		if len(last.Sessions) == wantCount && copiedLedgerSessionsTerminal(last.Sessions) {
			return last
		}
		select {
		case <-t.Context().Done():
			t.Fatalf("waiting for %d terminal Worker Sessions for Work %q: %v", wantCount, workID, t.Context().Err())
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d terminal Worker Sessions for Work %q; last=%#v", wantCount, workID, last)
		case <-ticker.C:
		}
	}
}

func waitCopiedLedgerInFlightWorkerSession(
	t *testing.T,
	baseURL, factoryID, workID string,
) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	deadline := time.NewTimer(copiedLedgerReplayTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var last factoryapi.ListWorkerSessionsResponse
	for {
		last = support.ListSessionWorkerSessions(t, baseURL, factoryID, workID)
		if len(last.Sessions) > 0 && (last.Sessions[0].State == factoryapi.WorkerSessionObservationStateStarting ||
			last.Sessions[0].State == factoryapi.WorkerSessionObservationStateRunning) {
			return last
		}
		select {
		case <-t.Context().Done():
			t.Fatalf("waiting for a live Worker Session for Work %q: %v", workID, t.Context().Err())
		case <-deadline.C:
			t.Fatalf("timed out waiting for a live Worker Session for Work %q; last=%#v", workID, last)
		case <-ticker.C:
		}
	}
}

func copiedLedgerSessionsTerminal(sessions []factoryapi.WorkerSessionObservation) bool {
	for _, observation := range sessions {
		if observation.State == factoryapi.WorkerSessionObservationStateStarting || observation.State == factoryapi.WorkerSessionObservationStateRunning {
			return false
		}
	}
	return true
}

func waitCopiedLedgerWorkState(t *testing.T, baseURL, factoryID, workID, want string) factoryapi.Work {
	t.Helper()
	endpoint := copiedLedgerWorkURL(baseURL, factoryID, workID)
	deadline := time.NewTimer(copiedLedgerReplayTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var last factoryapi.Work
	for {
		last = support.GetJSON[factoryapi.Work](t, endpoint)
		if last.State != nil && last.State.Name == want {
			return last
		}
		select {
		case <-t.Context().Done():
			t.Fatalf("waiting for Work %q state %q: %v", workID, want, t.Context().Err())
		case <-deadline.C:
			t.Fatalf("timed out waiting for Work %q state %q; last=%#v", workID, want, last.State)
		case <-ticker.C:
		}
	}
}

func waitCopiedLedgerWorkConfirmed(t *testing.T, baseURL, factoryID, workID string) factoryapi.Work {
	t.Helper()
	endpoint := copiedLedgerWorkURL(baseURL, factoryID, workID)
	deadline := time.NewTimer(copiedLedgerReplayTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var last factoryapi.Work
	for {
		last = support.GetJSON[factoryapi.Work](t, endpoint)
		if last.ConfirmationState != nil && *last.ConfirmationState == factoryapi.CONFIRMED {
			return last
		}
		select {
		case <-t.Context().Done():
			t.Fatalf("waiting for confirmed Work %q: %v", workID, t.Context().Err())
		case <-deadline.C:
			t.Fatalf("timed out waiting for confirmed Work %q; last=%#v", workID, last)
		case <-ticker.C:
		}
	}
}

func captureCopiedLedgerSnapshot(
	t *testing.T,
	baseURL, factoryID string,
	workIDs []string,
	targetWorkID, failureWorkID string,
) copiedLedgerPublicSnapshot {
	t.Helper()
	snapshot := copiedLedgerPublicSnapshot{
		inventory:   support.GetJSON[factoryapi.ListWorkResponse](t, copiedLedgerWorkListURL(baseURL, factoryID)),
		works:       make(map[string]factoryapi.Work, len(workIDs)),
		lists:       make(map[string]factoryapi.ListWorkerSessionsResponse, len(workIDs)),
		details:     make(map[string]factoryapi.WorkerSessionObservation),
		transcripts: make(map[string]factoryapi.WorkerSessionTranscriptResponse),
		events:      make(map[string][]factoryapi.WorkerSessionEvent),
	}
	for _, workID := range workIDs {
		snapshot.works[workID] = support.GetJSON[factoryapi.Work](t, copiedLedgerWorkURL(baseURL, factoryID, workID))
		snapshot.lists[workID] = support.ListSessionWorkerSessions(t, baseURL, factoryID, workID)
		if workID != targetWorkID && workID != failureWorkID {
			continue
		}
		for _, listed := range snapshot.lists[workID].Sessions {
			workerSessionID := listed.WorkerSessionId
			snapshot.details[workerSessionID] = support.GetJSON[factoryapi.WorkerSessionObservation](t,
				copiedLedgerWorkerSessionURL(baseURL, factoryID, workerSessionID))
			if listed.ProviderSessionAvailable && listed.Transcript == factoryapi.WorkerSessionObservationTranscriptAVAILABLE {
				transcript := support.GetJSON[factoryapi.WorkerSessionTranscriptResponse](t,
					copiedLedgerWorkerSessionURL(baseURL, factoryID, workerSessionID)+"/transcript")
				snapshot.transcripts[workerSessionID] = transcript
				encoded, err := json.Marshal(transcript)
				if err != nil {
					t.Fatalf("marshal stable-ID transcript for redaction check: %v", err)
				}
				assertWorkerTranscriptRedacted(t, string(encoded))
			}
			snapshot.events[workerSessionID] = readCopiedLedgerEvents(t,
				copiedLedgerWorkerSessionURL(baseURL, factoryID, workerSessionID)+"/events?replayOnly=true")
		}
	}
	return snapshot
}

func assertCopiedLedgerWorkRows(
	t *testing.T,
	response factoryapi.ListWorkerSessionsResponse,
	factoryID, workID string,
	wantCount int,
	wantState factoryapi.WorkerSessionObservationState,
) {
	t.Helper()
	if response.Sessions == nil || len(response.Sessions) != wantCount {
		t.Fatalf("Work-scoped Worker Session rows for %q = %#v, want %d rows", workID, response.Sessions, wantCount)
	}
	seen := make(map[string]struct{}, len(response.Sessions))
	for index, observation := range response.Sessions {
		if observation.WorkerSessionId == "" || observation.AttemptId == "" || observation.State != wantState ||
			observation.FactorySessionId == nil || *observation.FactorySessionId != factoryID ||
			observation.WorkId == nil || *observation.WorkId != workID || !reflect.DeepEqual(observation.WorkIds, []string{workID}) {
			t.Fatalf("Work %q observation[%d] = %#v, want exact session/work/attempt attribution and %s", workID, index, observation, wantState)
		}
		if _, exists := seen[observation.WorkerSessionId]; exists {
			t.Fatalf("Work %q repeated Worker Session identity %q", workID, observation.WorkerSessionId)
		}
		seen[observation.WorkerSessionId] = struct{}{}
		if (observation.ConfirmationState != factoryapi.CONFIRMED && observation.ConfirmationState != factoryapi.UNCONFIRMED) ||
			observation.DurationMillis == nil || observation.DurationBasis == "" ||
			observation.StartedAt == nil || observation.EndedAt == nil || observation.Parse.EventCount == 0 ||
			observation.RecordingHealth == nil || *observation.RecordingHealth != factoryapi.WorkerSessionObservationRecordingHealthComplete {
			t.Fatalf("Work %q attempt %q lacks confirmation/parse/timing/recording evidence: %#v", workID, observation.WorkerSessionId, observation)
		}
		if !observation.ProviderSessionAvailable || observation.ProviderSession == nil || strings.TrimSpace(observation.ProviderSession.Id) == "" {
			t.Fatalf("Work %q attempt %q lacks exact Provider Session identity: %#v", workID, observation.WorkerSessionId, observation)
		}
	}
}

func assertCopiedLedgerSnapshotsEqual(t *testing.T, before, after copiedLedgerPublicSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(before.inventory, after.inventory) {
		assertCopiedLedgerWorkInventoryEqual(t, before.inventory, after.inventory)
	}
	if !reflect.DeepEqual(before.works, after.works) {
		t.Fatalf("public Work facts changed after copied-ledger restart:\nbefore=%#v\nafter=%#v", before.works, after.works)
	}
	if !reflect.DeepEqual(before.lists, after.lists) {
		assertCopiedLedgerSessionListsEqual(t, before.lists, after.lists)
	}
	if !reflect.DeepEqual(before.details, after.details) {
		t.Fatalf("stable-ID Worker Session details changed after restart:\nbefore=%#v\nafter=%#v", before.details, after.details)
	}
	if !reflect.DeepEqual(before.transcripts, after.transcripts) {
		t.Fatalf("stable-ID Provider Session transcripts changed after restart:\nbefore=%#v\nafter=%#v", before.transcripts, after.transcripts)
	}
	if !reflect.DeepEqual(before.events, after.events) {
		assertCopiedLedgerEventsEqual(t, before.events, after.events)
	}
}

func startCopiedLedgerResumeProcess(
	t *testing.T,
	factoryDir, factoryID, recordingPath, homeDir, workerRecordingPath, runtimeInstanceID string,
) *support.FunctionalAPIServer {
	t.Helper()
	successorPath := filepath.Join(filepath.Dir(recordingPath), "worker-session-resume-successor.json")
	workerRecordingWriter, err := newCopiedLedgerWorkerRecordingWriter(workerRecordingPath)
	if err != nil {
		t.Fatalf("create copied-ledger resume Worker recording edge: %v", err)
	}
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       remoteFunctionalEnvironment(homeDir),
		Args:                      []string{"--resume", recordingPath, "--record", successorPath},
		Edges: serviceedges.Edges{
			FactorySessionIDGenerator:                func() string { return factoryID },
			FactorySessionRuntimeInstanceIDGenerator: func() string { return runtimeInstanceID },
			ProviderSessionResolveHomeDirectory:      func() (string, error) { return homeDir, nil },
			WorkerRecordingWriter:                    workerRecordingWriter,
		},
	})
}

func copiedLedgerWorkURL(baseURL, factoryID, workID string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(factoryID) + "/work/" + url.PathEscape(workID)
}

func copiedLedgerWorkListURL(baseURL, factoryID string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(factoryID) + "/work"
}

func copiedLedgerWorkerSessionURL(baseURL, factoryID, workerSessionID string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(factoryID) + "/worker-sessions/" + url.PathEscape(workerSessionID)
}

func copyCopiedLedgerFile(t *testing.T, source, destination string) string {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read completed Factory Event recording: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("completed Factory Event recording is empty")
	}
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		t.Fatalf("copy completed Factory Event recording: %v", err)
	}
	return destination
}

func copyCopiedLedgerDirectory(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse to copy symlink from test Provider Session store")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatalf("copy controlled Provider Session transcript store: %v", err)
	}
}
