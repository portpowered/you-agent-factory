package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession
// exercises the assembled replay/resume path through the customer process. The
// in-flight artifact is intentionally unfinalized, while the finished artifact
// retains its terminal Work state.
func TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	reusable := newSeededReplayResumeProcess(t)

	for _, test := range []struct {
		name     string
		finished bool
	}{
		{
			name: "in-flight tail",
		}, {
			name:     "finished recording",
			finished: true,
		}} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			artifactPayload := seededReplayResumeArtifactPayload(t, test.finished)
			factoryDir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			artifactPath := filepath.Join(factoryDir, "seeded-replay-resume.json")
			if err := os.WriteFile(artifactPath, artifactPayload, 0o644); err != nil {
				t.Fatalf("write replay artifact: %v", err)
			}
			running := reusable.run(t, factoryDir, artifactPath)

			stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(running.url, running.sessionID))
			waitForSeededReplayRuntimeStart(t, stream)
			status := support.GetJSON[factoryapi.StatusResponse](
				t,
				strings.TrimSuffix(running.url, "/")+"/factory-sessions/"+running.sessionID+"/status",
			)

			if status.TotalTokens != 1 {
				t.Fatalf("replayed status totalTokens = %d, want one Work token", status.TotalTokens)
			}
			if test.finished {
				if status.Categories.Terminal != 1 {
					t.Fatalf("finished replay terminal count = %d, want 1", status.Categories.Terminal)
				}
			} else if status.Categories.Initial != 1 {
				t.Fatalf("in-flight replay initial count = %d, want 1", status.Categories.Initial)
			}

			listed := support.GetJSON[factoryapi.ListWorkResponse](
				t,
				support.SessionWorkURL(running.url, running.sessionID, "/work"),
			)
			if len(listed.Results) != 1 {
				t.Fatalf("replayed Work listing length = %d, want one Work: %#v", len(listed.Results), listed.Results)
			}
			wantLocation := "task:init"
			if test.finished {
				wantLocation = "task:complete"
			}
			if !support.HasWorkAtCustomerState(listed, "work-seeded-replay-resume", wantLocation) {
				t.Fatalf("replayed Work is not at %s: %#v", wantLocation, listed.Results)
			}

			for _, event := range support.GetFactoryEventsForSessionAt(t, running.url, running.sessionID) {
				if event.Type == factoryapi.FactoryEventTypeDispatchRequest {
					t.Fatalf("replayed Work unexpectedly produced a dispatch request: %#v", event)
				}
			}
			running.daemon.Stop(t)
		})
	}
	t.Run("successor history", func(t *testing.T) {
		testSeededReplayResumePreservesSuccessorHistory(t, reusable)
	})
	t.Run("F01 read failure safety", func(t *testing.T) {
		testRecordStartupSafetyReadFailurePreservesTargetAndCause(t, reusable)
	})
	t.Run("JSON direct restore", func(t *testing.T) {
		testRecordStartupSafetyDirectRestore(t, reusable)
	})
}

// An explicitly selected UUID does not make a retained JSON board a fresh
// recording. Restore its Work and prefix before allowing any output flush.
func testRecordStartupSafetyDirectRestore(t *testing.T, reusable *seededReplayResumeProcess) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
	sessionID := uuid.NewString()
	selectedPath := filepath.Join(dir, "current-board.__factory_session_id__.json")
	path := strings.ReplaceAll(selectedPath, "__factory_session_id__", sessionID)
	var artifact factorydefinitions.ReplayArtifact
	if err := json.Unmarshal(seededReplayResumeArtifactPayload(t, true), &artifact); err != nil {
		t.Fatal(err)
	}
	for index := range artifact.Events {
		artifact.Events[index].Context.SessionID = &sessionID
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	// The same board is reopened only after its preceding host has joined.
	for opening := 0; opening < 3; opening++ {
		running := reusable.runForSession(t, dir, path, sessionID, "--record", selectedPath)
		assertSeededSuccessorWorkAndHistory(t, running, true)
		running.daemon.Stop(t)
	}
}

// A successor must remain recoverable after the live ledger has been released.
// Each format owns its files, session, profile and host on one shared process.
func testSeededReplayResumePreservesSuccessorHistory(t *testing.T, reusable *seededReplayResumeProcess) {
	t.Parallel()
	for _, format := range []string{"json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			source := filepath.Join(dir, "source.json")
			successor := filepath.Join(dir, "successor."+format)
			payload := seededReplayResumeArtifactPayload(t, true)
			sessionID := uuid.NewString()
			var artifact factorydefinitions.ReplayArtifact
			if err := json.Unmarshal(payload, &artifact); err != nil {
				t.Fatal(err)
			}
			for index := range artifact.Events {
				artifact.Events[index].Context.SessionID = &sessionID
			}
			payload, err := json.Marshal(artifact)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, payload, 0o644); err != nil {
				t.Fatal(err)
			}
			resumed := reusable.runForSession(t, dir, source, sessionID, "--resume", source, "--record", successor)
			assertSeededSuccessorWorkAndHistory(t, resumed, true)
			resumed.daemon.Stop(t)
			if !bytes.Equal(payload, mustReadSeededReplayArtifact(t, source)) {
				t.Fatal("resume changed its source recording")
			}
			replayed := reusable.runForSession(t, dir, successor, sessionID, "--replay", successor, "--no-record")
			assertSeededSuccessorWorkAndHistory(t, replayed, false)
			replayed.daemon.Stop(t)
		})
	}
}

func assertSeededSuccessorWorkAndHistory(t *testing.T, running seededReplayResumeRun, retainedHistory bool) {
	t.Helper()
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(running.url, running.sessionID))
	waitForSeededReplayRuntimeStart(t, stream)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t,
		support.SessionWorkURL(running.url, running.sessionID, "/work"))
	if len(listed.Results) != 1 || !support.HasWorkAtCustomerState(listed, "work-seeded-replay-resume", "task:complete") {
		t.Fatalf("successor lost the recorded terminal Work: %#v", listed.Results)
	}
	if !retainedHistory {
		return
	}
	events := support.GetFactoryEventsForSessionAt(t, running.url, running.sessionID)
	wantPrefix := []string{"run-request", "work-request", "work-state-change", "run-response"}
	if len(events) < len(wantPrefix) {
		t.Fatalf("successor history has %d events, want retained prefix %v", len(events), wantPrefix)
	}
	for index, id := range wantPrefix {
		if events[index].Id != id {
			t.Fatalf("successor event %d = %q, want retained %q", index, events[index].Id, id)
		}
	}
}

type seededReplayResumeProcess struct {
	process support.Process

	mu               sync.RWMutex
	serversByPort    map[int]*support.ProcessAPIServer
	payloadsByPath   map[string][]byte
	readErrorsByPath map[string]error
	nextPort         atomic.Int32
}

type seededReplayResumeRun struct {
	url       string
	sessionID string
	daemon    *support.ProcessCommand
}

func newSeededReplayResumeProcess(t *testing.T) *seededReplayResumeProcess {
	t.Helper()
	reusable := &seededReplayResumeProcess{
		serversByPort:    make(map[int]*support.ProcessAPIServer),
		payloadsByPath:   make(map[string][]byte),
		readErrorsByPath: make(map[string]error),
	}
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:                    reusable.startAPIServer,
		FactorySessionReplayRecordingReader: reusable.readReplayRecording,
		RecordingReadFile:                   reusable.readRecording,
		ProviderCommandRunner: testutil.NewProviderCommandRunner(platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdout("unexpected replay dispatch COMPLETE"),
		}),
	})
	support.CleanupProcess(t, process)
	reusable.process = process
	return reusable
}

func (reusable *seededReplayResumeProcess) run(
	t *testing.T,
	factoryDir string,
	artifactPath string,
) seededReplayResumeRun {
	return reusable.runWithRecordingArgs(t, factoryDir, artifactPath, "--replay", artifactPath, "--no-record")
}

func (reusable *seededReplayResumeProcess) runWithRecordingArgs(
	t *testing.T,
	factoryDir string,
	artifactPath string,
	recordingArgs ...string,
) seededReplayResumeRun {
	return reusable.runForSession(t, factoryDir, artifactPath, uuid.NewString(), recordingArgs...)
}

func (reusable *seededReplayResumeProcess) runForSession(
	t *testing.T,
	factoryDir string,
	artifactPath string,
	sessionID string,
	recordingArgs ...string,
) seededReplayResumeRun {
	t.Helper()
	api := support.NewProcessAPIServer()
	port := 22000 + int(reusable.nextPort.Add(1))
	reusable.mu.Lock()
	reusable.serversByPort[port] = api
	reusable.payloadsByPath[filepath.Clean(artifactPath)] = append([]byte(nil), mustReadSeededReplayArtifact(t, artifactPath)...)
	reusable.mu.Unlock()
	t.Cleanup(func() {
		reusable.mu.Lock()
		delete(reusable.serversByPort, port)
		delete(reusable.payloadsByPath, filepath.Clean(artifactPath))
		reusable.mu.Unlock()
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--session", sessionID,
		"--continuously", "--with-server", "--quiet",
		"--listen", fmt.Sprintf("127.0.0.1:%d", port),
		"--dir", factoryDir,
		"--provider", "CODEX", "--model", "gpt-5-codex",
	})
	inputs.Input.Args = append(inputs.Input.Args, recordingArgs...)
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = factoryDir
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
			t.Logf("seeded replay daemon stderr: %s", stderr)
		}
	})
	daemon := support.StartProcessCommand(t, reusable.process, inputs.Input)
	return seededReplayResumeRun{url: api.WaitForURL(t), sessionID: sessionID, daemon: daemon}
}

func mustReadSeededReplayArtifact(t testing.TB, path string) []byte {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replay artifact: %v", err)
	}
	return payload
}

func (reusable *seededReplayResumeProcess) readReplayRecording(path string) ([]byte, error) {
	reusable.mu.RLock()
	payload := reusable.payloadsByPath[filepath.Clean(path)]
	err := reusable.readErrorsByPath[filepath.Clean(path)]
	reusable.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, errors.New("seeded replay payload was not registered for this invocation")
	}
	return append([]byte(nil), payload...), nil
}

// A restore read failure must abort before any writes to the resolved target,
// including writes from startup cleanup. Each session owns its fault and file.
func testRecordStartupSafetyReadFailurePreservesTargetAndCause(t *testing.T, reusable *seededReplayResumeProcess) {
	t.Parallel()
	for _, test := range []struct {
		name, format, payload, code string
		readFailure                 bool
	}{
		{"JSON read denied", "json", "retained recording bytes", "CURRENT_BOARD_RECORDING_UNREADABLE", true},
		{"JSONL read denied", "jsonl", "retained recording bytes", "CURRENT_BOARD_RECORDING_UNREADABLE", true},
		{"corrupt JSON", "json", `{"schemaVersion":"replay.v1","events":["PRIVATE_RECORDING_PAYLOAD"`, "CURRENT_BOARD_RECORDING_CORRUPT", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			sessionID := uuid.NewString()
			selectedPath := filepath.Join(dir, "retained.__factory_session_id__."+test.format)
			path := strings.ReplaceAll(selectedPath, "__factory_session_id__", sessionID)
			support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
			support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
			before := []byte(test.payload)
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			cause := &fs.PathError{Op: "read recording", Path: path, Err: fs.ErrPermission}
			if test.readFailure {
				reusable.mu.Lock()
				reusable.readErrorsByPath[path] = cause
				reusable.mu.Unlock()
			}
			t.Cleanup(func() {
				reusable.mu.Lock()
				delete(reusable.readErrorsByPath, path)
				reusable.mu.Unlock()
			})
			inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--session", sessionID,
				"--dir", dir, "--continuously", "--with-server", "--quiet", "--record", selectedPath,
				"--provider", "CODEX", "--model", "gpt-5-codex"})
			home := t.TempDir()
			inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
			inputs.Input.WorkingDirectory = dir
			err := reusable.process.Execute(inputs.Input)
			if test.readFailure && !errors.Is(err, cause) {
				t.Fatalf("startup error = %v, want original read cause; stderr=%s", err, inputs.Stderr())
			}
			var response factoryapi.ErrorResponse
			if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &response); decodeErr != nil {
				t.Fatalf("decode ErrorResponse: %v; stderr=%s", decodeErr, inputs.Stderr())
			}
			if string(response.Code) != test.code || response.Family != factoryapi.ErrorFamilyInternalServerError ||
				!strings.Contains(response.Message, fmt.Sprintf("%q", path)) {
				t.Fatalf("startup response omits selected path or file cause: %#v", response)
			}
			if test.readFailure && !strings.Contains(response.Message, "permission denied") {
				t.Fatalf("startup response omits read cause: %#v", response)
			}
			if strings.Contains(inputs.Stdout()+inputs.Stderr(), "PRIVATE_RECORDING_PAYLOAD") {
				t.Fatal("startup diagnostic exposed recording payload")
			}
			if strings.Contains(inputs.Stdout()+inputs.Stderr(), "Factory initiated:") {
				t.Fatal("failed startup published readiness")
			}
			if !bytes.Equal(before, mustReadSeededReplayArtifact(t, path)) {
				t.Fatal("failed startup cleanup changed the retained recording")
			}
		})
	}
}

func (reusable *seededReplayResumeProcess) readRecording(path string) ([]byte, error) {
	reusable.mu.RLock()
	err := reusable.readErrorsByPath[filepath.Clean(path)]
	reusable.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (reusable *seededReplayResumeProcess) startAPIServer(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	reusable.mu.RLock()
	server := reusable.serversByPort[request.Port]
	reusable.mu.RUnlock()
	if server == nil {
		return fmt.Errorf("seeded replay API server is not registered for requested port %d", request.Port)
	}
	return server.Start(ctx, request)
}

func waitForSeededReplayRuntimeStart(t *testing.T, stream *support.FactoryEventStream) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	for {
		event := stream.NextEventContext(ctx)
		if event.Type == factoryapi.FactoryEventTypeFactoryStateResponse {
			return
		}
	}
}

func seededReplayResumeFactoryConfig() map[string]any {
	return map[string]any{
		"name": "seeded-replay-resume",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
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
}

func seededReplayResumeArtifactPayload(t *testing.T, finished bool) []byte {
	t.Helper()
	const (
		workID    = "work-seeded-replay-resume"
		requestID = "request-seeded-replay-resume"
		traceID   = "trace-seeded-replay-resume"
	)
	snapshot, err := factorydefinitions.NewFactorySnapshot(seededReplayResumeFactoryConfig())
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	base := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	requestIDValue := requestID
	workIDValue := []string{workID}
	traceIDValue := []string{traceID}
	requestSourceValue := "external-submit"
	sourceValue := requestSourceValue
	events := []factorydefinitions.FactoryEvent{
		seededReplayResumeEvent(t, "run-request", 0, 0, base, factorydefinitions.FactoryEventTypeRunRequest, factorydefinitions.RunRequestEventPayload{
			Factory:    snapshot,
			RecordedAt: base,
		}),
		seededReplayResumeEventWithContext(t, "work-request", 1, 1, base.Add(time.Second), factorydefinitions.FactoryEventTypeWorkRequest, work.WorkRequestEventPayload{
			Source: requestSourceValue,
			Type:   work.WorkRequestTypeFactoryRequestBatch,
			Works: []work.WorkRequestEventWork{{
				Name:       "recorded-work",
				WorkID:     workID,
				RequestID:  requestID,
				WorkTypeID: "task",
				State:      &work.WorkEventState{Name: "init", Type: "INITIAL"},
				TraceID:    traceID,
			}},
		}, &sourceValue, &requestIDValue, &workIDValue, &traceIDValue),
	}
	if finished {
		finishedSource := work.WorkStateChangeSourceAPI
		events = append(events,
			seededReplayResumeEventWithContext(t, "work-state-change", 2, 2, base.Add(2*time.Second), factorydefinitions.FactoryEventTypeWorkStateChange, factorydefinitions.WorkStateChangeEventPayload{
				FromPlaceID:  "task:init",
				FromState:    "init",
				Source:       finishedSource,
				ToPlaceID:    "task:complete",
				ToState:      "complete",
				WorkID:       workID,
				WorkTypeName: "task",
			}, &sourceValue, &requestIDValue, &workIDValue, &traceIDValue),
			seededReplayResumeEvent(t, "run-response", 3, 3, base.Add(3*time.Second), factorydefinitions.FactoryEventTypeRunResponse, func() factorydefinitions.RunResponseEventPayload {
				state := factorydefinitions.FactoryStateCompleted
				return factorydefinitions.RunResponseEventPayload{State: &state}
			}()),
		)
	}

	artifact := factorydefinitions.ReplayArtifact{
		SchemaVersion: factorydefinitions.ReplayV1SourceFormat,
		RecordedAt:    base,
		Events:        events,
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal seeded replay artifact: %v", err)
	}
	return payload
}

func seededReplayResumeEvent(
	t *testing.T,
	id string,
	sequence int,
	tick int,
	eventTime time.Time,
	eventType factorydefinitions.FactoryEventType,
	payload any,
) factorydefinitions.FactoryEvent {
	return seededReplayResumeEventWithContext(t, id, sequence, tick, eventTime, eventType, payload, nil, nil, nil, nil)
}

func seededReplayResumeEventWithContext(
	t *testing.T,
	id string,
	sequence int,
	tick int,
	eventTime time.Time,
	eventType factorydefinitions.FactoryEventType,
	payload any,
	source *string,
	contextRequestID *string,
	workIDs *[]string,
	traceIDs *[]string,
) factorydefinitions.FactoryEvent {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal replay event %q payload: %v", id, err)
	}
	return factorydefinitions.FactoryEvent{
		Id:            id,
		Payload:       payloadBytes,
		SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
		Type:          eventType,
		Context: factorydefinitions.FactoryEventContext{
			EventTime: eventTime,
			RequestID: contextRequestID,
			Sequence:  sequence,
			Source:    source,
			Tick:      tick,
			TraceIDs:  traceIDs,
			WorkIDs:   workIDs,
		},
	}
}
