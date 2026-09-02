package http_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const remoteWorkerSessionProviderID = "session_fixture_codex_success"

// WSR-FT-014: root.BuildProcess/Process.Execute proves a live Worker Session
// continuation remains an explicit source/successor lineage with the exact
// Provider Session through durable public history and portable replay.
func TestWorkerSessionRemoteInvokeObserveContinueUsesServerAfterDisconnect(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	homeDir := t.TempDir()
	writeRemoteCodexRollout(t, homeDir, remoteWorkerSessionProviderID)
	providerOutput := readRemoteProviderFixture(t, "codex", "success", "stdout.jsonl")
	gate := make(chan struct{})
	runner := newRemoteInvokeContinueRunner(gate, providerOutput)
	recordingWriter := newRemoteWorkerRecordingStore()

	factoryDir := support.ScaffoldSingleStepFactory(t, "remote-worker-session-invoke-continue")
	support.WriteAgentConfig(t, factoryDir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "fixture-model"))
	env := remoteFunctionalEnvironment(homeDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       env,
		Args:                      []string{"--record", filepath.Join(t.TempDir(), "wsr-006-runtime-recording.json")},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
			WorkerRecordingWriter: recordingWriter,
			FactorySessionRuntimeInstanceIDGenerator: func() string {
				return "wsr-006-runtime-instance"
			},
			ProviderSessionResolveHomeDirectory: func() (string, error) {
				return homeDir, nil
			},
		},
	})
	recordingReader := server.WorkerRecordingReader()
	if recordingReader == nil {
		t.Fatal("root-built functional server did not expose a Worker recording reader")
	}
	clientRunner := testutil.NewProviderCommandRunner()
	client := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: clientRunner})
	support.CleanupProcess(t, client)

	// Use the public HTTP admission boundary and close the actual submitting
	// connection after Workers admission. The server-owned execution must keep
	// running without a caller context to cancel it.
	connection := openDirectWorkerSessionConnection(t, server.URL(), "remote-invoke-request", "remote-source-session", "remote-source-dispatch")
	runner.waitStarted(t)
	if err := connection.Close(); err != nil {
		t.Fatalf("close submitting connection: %v", err)
	}
	if runner.wasCanceled() {
		t.Fatal("server-owned remote Worker Session was canceled by submitting disconnect")
	}
	activeFailure, activeErr := executeRemoteWorkerCLIExpectError(t, ctx, client, env, factoryDir, server.URL(),
		"--json", "worker-sessions", "continue", "remote-source-session",
		"--request-id", "remote-active-continue-request", "--successor-worker-session-id", "remote-active-successor",
		"--user-message", "active source must be rejected", "--async")
	if activeErr == nil {
		t.Fatal("continuation of active source succeeded, want conflict")
	}
	assertRemoteWorkerCLIError(t, activeFailure, string(factoryapi.ErrorResponseCodeWORKERSESSIONCONTINUATIONCONFLICT))

	unknownFailure, unknownErr := executeRemoteWorkerCLIExpectError(t, ctx, client, env, factoryDir, server.URL(),
		"--json", "worker-sessions", "continue", "remote-unknown-source",
		"--request-id", "remote-unknown-continue-request", "--successor-worker-session-id", "remote-unknown-successor",
		"--user-message", "unknown source must be rejected", "--async")
	if unknownErr == nil {
		t.Fatal("continuation of unknown source succeeded, want not found")
	}
	assertRemoteWorkerCLIError(t, unknownFailure, string(factoryapi.ErrorResponseCodeNOTFOUND))
	if runnerCallCount := len(runner.requestsSnapshot()); runnerCallCount != 1 {
		t.Fatalf("provider calls after active/unknown continuation failures = %d, want one initial call", runnerCallCount)
	}
	close(gate)
	runner.waitFirstCompleted(t)

	serverURL := server.URL()
	assertRemoteSourceObservation(t, ctx, client, env, factoryDir, serverURL)
	continueRemoteWorkerSession(t, ctx, client, env, factoryDir, serverURL, recordingReader, runner)
	idempotencyFailure, idempotencyErr := executeRemoteWorkerCLIExpectError(t, ctx, client, env, factoryDir, serverURL,
		"--json", "worker-sessions", "continue", "remote-source-session",
		"--request-id", "remote-continue-request", "--successor-worker-session-id", "remote-other-successor",
		"--user-message", "different immutable input", "--async")
	if idempotencyErr == nil {
		t.Fatal("continuation request-id reuse succeeded, want conflict")
	}
	assertRemoteWorkerCLIError(t, idempotencyFailure, string(factoryapi.ErrorResponseCodeWORKERSESSIONCONTINUATIONREQUESTIDCONFLICT))
	assertRemoteContinuationUsesServer(t, clientRunner, runner)

	all := waitForRemoteWorkerSessionList(t, ctx, client, env, factoryDir, serverURL)
	if len(all) != 2 {
		t.Fatalf("top-level direct Worker Session count = %d, want source and distinct successor", len(all))
	}
	functionalevidence.Covers(t, "sse/streamWorkerSessionEventsByTopLevelWorkerSessionId")
}

func assertRemoteSourceObservation(t *testing.T, ctx context.Context, client support.Process, env []string, factoryDir, serverURL string) {
	t.Helper()
	listed := waitForRemoteWorkerSession(t, ctx, client, env, factoryDir, serverURL, "remote-source-session")
	if listed.State != "COMPLETED" || listed.WorkerSessionID != "remote-source-session" {
		t.Fatalf("remote source observation = %#v, want completed source", listed)
	}
	if listed.ProviderSession == nil || listed.ProviderSession.ID != remoteWorkerSessionProviderID {
		t.Fatalf("remote source provider session = %#v, want exact provider identity", listed.ProviderSession)
	}

	show := executeRemoteWorkerCLI(t, ctx, client, env, factoryDir, serverURL,
		"--json", "worker-sessions", "show", "--worker-session-id", "remote-source-session")
	var shown remoteWorkerSessionObservation
	decodeRemoteWorkerJSON(t, show.Stdout(), &shown)
	if shown.WorkerSessionID != "remote-source-session" || shown.State != "COMPLETED" {
		t.Fatalf("remote show = %#v, want completed source", shown)
	}

	read := executeRemoteWorkerCLI(t, ctx, client, env, factoryDir, serverURL,
		"--json", "worker-sessions", "read", "--worker-session-id", "remote-source-session")
	var transcript remoteWorkerSessionTranscript
	decodeRemoteWorkerJSON(t, read.Stdout(), &transcript)
	if transcript.WorkerSessionID != "remote-source-session" || transcript.ProviderSession.ID != remoteWorkerSessionProviderID || len(transcript.Entries) == 0 {
		t.Fatalf("remote transcript = %#v, want correlated entries", transcript)
	}
	transcriptBytes, _ := json.Marshal(transcript.Entries)
	if !strings.Contains(string(transcriptBytes), "Codex fixture answer COMPLETE") {
		t.Fatalf("remote transcript omitted provider answer: %s", transcriptBytes)
	}

	stream := executeRemoteWorkerCLI(t, ctx, client, env, factoryDir, serverURL,
		"--json", "worker-sessions", "stream", "--worker-session-id", "remote-source-session", "--replay-only")
	assertRemoteWorkerStreamTerminal(t, decodeRemoteWorkerNDJSON(t, stream.Stdout()), "remote-source-session")
}

func continueRemoteWorkerSession(
	t *testing.T,
	ctx context.Context,
	client support.Process,
	env []string,
	factoryDir,
	serverURL string,
	recordingReader recordings.WorkerRecordingReader,
	runner *remoteInvokeContinueRunner,
) {
	t.Helper()
	continued := executeRemoteWorkerCLI(t, ctx, client, env, factoryDir, serverURL,
		"--json", "worker-sessions", "continue", "remote-source-session",
		"--request-id", "remote-continue-request", "--successor-worker-session-id", "remote-successor-session",
		"--user-message", "continue on the exact provider session", "--async")
	var continuation remoteWorkerSessionContinuation
	decodeRemoteWorkerJSON(t, continued.Stdout(), &continuation)
	if !continuation.Accepted || continuation.SourceWorkerSessionID != "remote-source-session" ||
		continuation.SuccessorWorkerSessionID != "remote-successor-session" ||
		continuation.PredecessorWorkerSessionID != "remote-source-session" {
		t.Fatalf("remote continuation admission = %#v, want accepted source/successor", continuation)
	}

	listed := waitForRemoteWorkerSessionList(t, ctx, client, env, factoryDir, serverURL)
	var source, successor *remoteWorkerSessionObservation
	for index := range listed {
		switch listed[index].WorkerSessionID {
		case "remote-source-session":
			source = &listed[index]
		case "remote-successor-session":
			successor = &listed[index]
		}
	}
	if source == nil || successor == nil {
		t.Fatalf("public continuation observations = source=%#v successor=%#v, want source and successor", source, successor)
	}
	if source.ProviderSession == nil || successor.ProviderSession == nil || *source.ProviderSession != *successor.ProviderSession ||
		source.ProviderSession.ID != remoteWorkerSessionProviderID {
		t.Fatalf("public continuation Provider Sessions = source=%#v successor=%#v, want exact shared provider identity", source.ProviderSession, successor.ProviderSession)
	}

	sourceReplay := executeRemoteWorkerCLI(t, ctx, client, env, factoryDir, serverURL,
		"--json", "worker-sessions", "stream", "--worker-session-id", "remote-source-session", "--replay-only")
	sourceFrames := decodeRemoteWorkerNDJSON(t, sourceReplay.Stdout())
	assertRemoteSourceLineageReplay(t, sourceFrames, "remote-source-session", "remote-successor-session", remoteWorkerSessionProviderID)
	waitForRemoteWorkerSession(t, ctx, client, env, factoryDir, serverURL, "remote-successor-session")

	successorStream := executeRemoteWorkerCLI(t, ctx, client, env, factoryDir, serverURL,
		"--json", "worker-sessions", "stream", "--worker-session-id", "remote-successor-session", "--replay-only")
	successorFrames := decodeRemoteWorkerNDJSON(t, successorStream.Stdout())
	assertRemoteWorkerStreamTerminal(t, successorFrames, "remote-successor-session")
	assertRemoteSuccessorLineageReplay(t, successorFrames, "remote-source-session", "remote-successor-session", remoteWorkerSessionProviderID)
	if !strings.Contains(successorStream.Stdout(), "Codex fixture answer COMPLETE") {
		t.Fatalf("remote successor stream omitted continued provider output:\n%s", successorStream.Stdout())
	}
	assertRemoteWorkerRecordingParity(t, recordingReader, runner, *source, *successor, sourceFrames, successorFrames)
}

func assertRemoteContinuationUsesServer(t *testing.T, clientRunner *testutil.ProviderCommandRunner, runner *remoteInvokeContinueRunner) {
	t.Helper()
	if clientRunner.CallCount() != 0 {
		t.Fatalf("remote CLI caused local provider fallback: %d calls", clientRunner.CallCount())
	}
	requests := runner.requestsSnapshot()
	if len(requests) != 2 {
		t.Fatalf("server provider requests = %d, want initial and continuation only", len(requests))
	}
	if strings.Contains(strings.Join(requests[0].Args, " "), "resume") {
		t.Fatalf("initial server provider command resumed unexpectedly: %#v", requests[0].Args)
	}
	if !containsRemoteArgSequence(requests[1].Args, []string{"resume", remoteWorkerSessionProviderID}) {
		t.Fatalf("continuation server provider command = %#v, want exact resume identity", requests[1].Args)
	}
}

type remoteWorkerSessionObservation struct {
	WorkerSessionID            string                          `json:"workerSessionId"`
	State                      string                          `json:"state"`
	PredecessorWorkerSessionID string                          `json:"predecessorWorkerSessionId"`
	SuccessorWorkerSessionID   string                          `json:"successorWorkerSessionId"`
	ProviderSession            *remoteWorkerSessionProviderRef `json:"providerSession"`
}

type remoteWorkerSessionProviderRef struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}

type remoteWorkerSessionTranscript struct {
	WorkerSessionID string                         `json:"workerSessionId"`
	ProviderSession remoteWorkerSessionProviderRef `json:"providerSession"`
	Entries         []map[string]any               `json:"entries"`
}

type remoteWorkerSessionContinuation struct {
	RequestID                  string `json:"requestId"`
	SourceWorkerSessionID      string `json:"sourceWorkerSessionId"`
	SuccessorWorkerSessionID   string `json:"successorWorkerSessionId"`
	PredecessorWorkerSessionID string `json:"predecessorWorkerSessionId"`
	Accepted                   bool   `json:"accepted"`
	State                      string `json:"state"`
}

type remoteWorkerSessionListResponse struct {
	Sessions []remoteWorkerSessionObservation `json:"sessions"`
}

type remoteWorkerSessionStreamFrame struct {
	Delivery        string                     `json:"delivery"`
	WorkerSessionID string                     `json:"workerSessionId"`
	Event           *remoteWorkerSessionEvent  `json:"event"`
	ReplaySummary   *remoteWorkerSessionReplay `json:"replaySummary"`
}

type remoteWorkerSessionEvent struct {
	Payload       json.RawMessage `json:"payload"`
	SourceType    string          `json:"sourceType"`
	SourceEventID string          `json:"sourceEventId"`
}

type remoteWorkerSessionDraft struct {
	Payload remoteWorkerSessionPayload `json:"payload"`
}

type remoteWorkerSessionPayload struct {
	WorkerSessionID string                           `json:"workerSessionId"`
	RecordingID     string                           `json:"recordingId"`
	DispatchID      string                           `json:"dispatchId"`
	AttemptID       string                           `json:"attemptId"`
	Attempt         int                              `json:"attempt"`
	AttemptReason   string                           `json:"attemptReason"`
	StartedAt       *time.Time                       `json:"startedAt"`
	Continuation    *remoteWorkerSessionProviderRef  `json:"continuation"`
	Lineage         *remoteWorkerSessionLineageFacts `json:"lineage"`
}

type remoteWorkerSessionLineageFacts struct {
	PredecessorWorkerSessionID string `json:"predecessorWorkerSessionId"`
	SuccessorWorkerSessionID   string `json:"successorWorkerSessionId"`
	PreviousDispatchID         string `json:"previousDispatchId"`
	PreviousAttemptID          string `json:"previousAttemptId"`
}

type remoteWorkerSessionReplay struct {
	Complete bool `json:"complete"`
}

func waitForRemoteWorkerSession(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	factoryDir, serverURL, workerSessionID string,
) remoteWorkerSessionObservation {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	// The provider edge has completed, but the server commits the public
	// Worker Session projection asynchronously. Poll the public list operation
	// until that projection is observable; a fixed sleep would not establish
	// the rediscovery boundary this scenario is proving.
	for {
		for _, session := range waitForRemoteWorkerSessionList(t, ctx, process, env, factoryDir, serverURL) {
			if session.WorkerSessionID == workerSessionID && session.State == "COMPLETED" {
				return session
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for completed remote Worker Session %q", workerSessionID)
		case <-ctx.Done():
			t.Fatalf("waiting for remote Worker Session %q canceled: %v", workerSessionID, ctx.Err())
		}
	}
}

func waitForRemoteWorkerSessionList(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	factoryDir, serverURL string,
) []remoteWorkerSessionObservation {
	t.Helper()
	inputs := executeRemoteWorkerCLI(t, ctx, process, env, factoryDir, serverURL,
		"--json", "worker-sessions", "list", "--scope", "direct")
	var listed remoteWorkerSessionListResponse
	decodeRemoteWorkerJSON(t, inputs.Stdout(), &listed)
	return listed.Sessions
}

func executeRemoteWorkerCLI(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	factoryDir, serverURL string,
	args ...string,
) *support.CapturedInputs {
	t.Helper()
	command := append([]string{"you", "--remote", "--server", serverURL}, args...)
	inputs := support.FakeInputs(ctx, command)
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("remote CLI %s: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(command, " "), err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs
}

func executeRemoteWorkerCLIExpectError(
	t *testing.T,
	ctx context.Context,
	process support.Process,
	env []string,
	factoryDir, serverURL string,
	args ...string,
) (*support.CapturedInputs, error) {
	t.Helper()
	command := append([]string{"you", "--remote", "--server", serverURL}, args...)
	inputs := support.FakeInputs(ctx, command)
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = factoryDir
	return inputs, process.Execute(inputs.Input)
}

func assertRemoteWorkerCLIError(t *testing.T, inputs *support.CapturedInputs, wantCode string) {
	t.Helper()
	for _, output := range []string{inputs.Stderr(), inputs.Stdout()} {
		var response factoryapi.ErrorResponse
		if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &response); err == nil && response.Code != "" {
			if string(response.Code) != wantCode {
				t.Fatalf("remote CLI error code = %q, want %q; stderr=%s stdout=%s", response.Code, wantCode, inputs.Stderr(), inputs.Stdout())
			}
			return
		}
	}
	t.Fatalf("remote CLI emitted no typed error response; stderr=%s stdout=%s", inputs.Stderr(), inputs.Stdout())
}

func decodeRemoteWorkerJSON(t *testing.T, stdout string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), target); err != nil {
		t.Fatalf("decode remote Worker Session JSON: %v\nstdout:\n%s", err, stdout)
	}
}

func decodeRemoteWorkerNDJSON(t *testing.T, stdout string) []remoteWorkerSessionStreamFrame {
	t.Helper()
	var frames []remoteWorkerSessionStreamFrame
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var frame remoteWorkerSessionStreamFrame
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("decode Worker Session stream frame: %v\nline:%s", err, line)
		}
		if frame.Delivery == "" {
			var summary remoteWorkerSessionReplay
			if err := json.Unmarshal([]byte(line), &summary); err != nil {
				t.Fatalf("decode Worker Session replay summary: %v\nline:%s", err, line)
			}
			frame.Delivery = "REPLAY_SUMMARY"
			frame.ReplaySummary = &summary
		}
		frames = append(frames, frame)
	}
	return frames
}

func assertRemoteWorkerStreamTerminal(t *testing.T, frames []remoteWorkerSessionStreamFrame, workerSessionID string) {
	t.Helper()
	if len(frames) == 0 {
		t.Fatalf("Worker Session stream for %q returned no frames", workerSessionID)
	}
	terminal := false
	completeSummary := false
	for _, frame := range frames {
		if frame.WorkerSessionID != "" && frame.WorkerSessionID != workerSessionID {
			t.Fatalf("Worker Session stream frame identity = %q, want %q", frame.WorkerSessionID, workerSessionID)
		}
		if frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY" {
			terminal = true
		}
		if frame.Delivery == "REPLAY_SUMMARY" && frame.ReplaySummary != nil && frame.ReplaySummary.Complete {
			completeSummary = true
		}
	}
	if !terminal || !completeSummary {
		t.Fatalf("Worker Session stream frames = %#v, want terminal and complete replay summary", frames)
	}
}

func assertRemoteSourceLineageReplay(t *testing.T, frames []remoteWorkerSessionStreamFrame, sourceID, successorID, providerID string) {
	t.Helper()
	for _, frame := range frames {
		if frame.Delivery == "REPLAY_SUMMARY" || frame.Event.SourceType != "worker_session_lineage" {
			continue
		}
		var draft remoteWorkerSessionDraft
		if err := json.Unmarshal(frame.Event.Payload, &draft); err != nil {
			t.Fatalf("decode source continuation lineage payload: %v", err)
		}
		if draft.Payload.WorkerSessionID == sourceID && draft.Payload.Lineage != nil &&
			draft.Payload.Lineage.SuccessorWorkerSessionID == successorID &&
			draft.Payload.Continuation != nil && draft.Payload.Continuation.ID == providerID {
			return
		}
	}
	t.Fatalf("source replay frames = %#v, want durable source-to-successor lineage with provider %q", frames, providerID)
}

func assertRemoteSuccessorLineageReplay(t *testing.T, frames []remoteWorkerSessionStreamFrame, sourceID, successorID, providerID string) {
	t.Helper()
	for _, frame := range frames {
		if frame.Delivery == "REPLAY_SUMMARY" || frame.Event.SourceType != "worker_session_lifecycle" {
			continue
		}
		var draft remoteWorkerSessionDraft
		if err := json.Unmarshal(frame.Event.Payload, &draft); err != nil {
			t.Fatalf("decode successor continuation payload: %v", err)
		}
		if draft.Payload.WorkerSessionID == successorID && draft.Payload.Lineage != nil &&
			draft.Payload.Lineage.PredecessorWorkerSessionID == sourceID &&
			draft.Payload.Continuation != nil && draft.Payload.Continuation.ID == providerID {
			return
		}
	}
	t.Fatalf("successor replay frames = %#v, want predecessor lineage with provider %q", frames, providerID)
}

type remoteWorkerSessionLiveFacts struct {
	opening *remoteWorkerSessionPayload
	lineage *remoteWorkerSessionPayload
}

func assertRemoteWorkerRecordingParity(
	t *testing.T,
	reader recordings.WorkerRecordingReader,
	runner *remoteInvokeContinueRunner,
	source,
	successor remoteWorkerSessionObservation,
	sourceFrames,
	successorFrames []remoteWorkerSessionStreamFrame,
) {
	t.Helper()
	if reader == nil {
		t.Fatal("Worker recording reader is required")
	}
	recordingID := remoteWorkerRecordingID(t, sourceFrames, successorFrames)
	snapshot, err := reader.LoadWorkerRecording(t.Context(), recordingID)
	if err != nil {
		t.Fatalf("load durable Worker recording %q: %v", recordingID, err)
	}
	if snapshot.RecordingID != recordingID || len(snapshot.Sessions) != 2 {
		t.Fatalf("durable Worker recording = %#v, want recording %q with source and successor", snapshot, recordingID)
	}

	providerCallsBefore := len(runner.requestsSnapshot())
	codec := recordings.WorkerRecordingCodec{}
	for _, expected := range []struct {
		observation remoteWorkerSessionObservation
		frames      []remoteWorkerSessionStreamFrame
	}{
		{observation: source, frames: sourceFrames},
		{observation: successor, frames: successorFrames},
	} {
		session := remoteRecordingSession(t, snapshot, expected.observation.WorkerSessionID)
		if session.Status != recordings.WorkerRecordingStatusComplete {
			t.Fatalf("durable Worker Session %q health = %q, want COMPLETE", session.WorkerSessionID, session.Status)
		}
		durableReplay, err := codec.ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{
			Snapshot:        snapshot,
			WorkerSessionID: session.WorkerSessionID,
		})
		if err != nil {
			t.Fatalf("replay durable Worker Session %q: %v", session.WorkerSessionID, err)
		}
		portable, err := codec.ExportWorkerPortableRecording(snapshot, session.WorkerSessionID)
		if err != nil {
			t.Fatalf("export portable Worker Session %q: %v", session.WorkerSessionID, err)
		}
		encoded, err := codec.EncodeWorkerPortableRecording(portable)
		if err != nil {
			t.Fatalf("encode portable Worker Session %q: %v", session.WorkerSessionID, err)
		}
		decoded, err := codec.DecodeWorkerPortableRecording(encoded)
		if err != nil {
			t.Fatalf("decode portable Worker Session %q: %v", session.WorkerSessionID, err)
		}
		if !reflect.DeepEqual(portable, decoded) {
			t.Fatalf("portable Worker Session %q changed during encode/decode", session.WorkerSessionID)
		}
		portableReplay, err := codec.ReplayWorkerPortableRecording(decoded)
		if err != nil {
			t.Fatalf("replay portable Worker Session %q: %v", session.WorkerSessionID, err)
		}
		if !reflect.DeepEqual(durableReplay.Projection, portableReplay.Projection) {
			t.Fatalf("durable and portable Worker Session %q projections differ:\ndurable=%#v\nportable=%#v", session.WorkerSessionID, durableReplay.Projection, portableReplay.Projection)
		}
		if !portableReplay.Projection.Complete || portableReplay.Projection.Terminal == nil {
			t.Fatalf("portable Worker Session %q replay = %#v, want complete terminal history", session.WorkerSessionID, portableReplay.Projection)
		}

		liveFacts := remoteLiveWorkerSessionFacts(t, expected.frames, session.WorkerSessionID)
		portableFacts := remotePortableWorkerSessionFacts(t, portable)
		if !reflect.DeepEqual(*liveFacts.opening, *portableFacts.opening) {
			t.Fatalf("live and portable opening facts for %q differ:\nlive=%#v\nportable=%#v", session.WorkerSessionID, *liveFacts.opening, *portableFacts.opening)
		}
		if liveFacts.lineage == nil || portableFacts.lineage == nil || !reflect.DeepEqual(*liveFacts.lineage, *portableFacts.lineage) {
			t.Fatalf("live and portable continuation lineage facts for %q differ:\nlive=%#v\nportable=%#v", session.WorkerSessionID, liveFacts.lineage, portableFacts.lineage)
		}
		if expected.observation.ProviderSession == nil {
			t.Fatalf("live Worker Session %q has no Provider Session", session.WorkerSessionID)
		}
		wantOpeningContinuation := liveFacts.opening.Lineage != nil && liveFacts.opening.Lineage.PredecessorWorkerSessionID != ""
		assertRemotePortableProviderFacts(t, portable, *expected.observation.ProviderSession, *liveFacts.lineage, wantOpeningContinuation)
	}
	if providerCallsAfter := len(runner.requestsSnapshot()); providerCallsAfter != providerCallsBefore {
		t.Fatalf("portable Worker recording replay changed provider calls from %d to %d", providerCallsBefore, providerCallsAfter)
	}
}

func remoteRecordingSession(
	t *testing.T,
	snapshot recordings.WorkerRecordingSnapshot,
	workerSessionID string,
) recordings.WorkerSessionRecordingSnapshot {
	t.Helper()
	for _, session := range snapshot.Sessions {
		if session.WorkerSessionID == workerSessionID {
			return session
		}
	}
	t.Fatalf("durable Worker recording omitted Worker Session %q", workerSessionID)
	return recordings.WorkerSessionRecordingSnapshot{}
}

func remoteWorkerRecordingID(t *testing.T, streams ...[]remoteWorkerSessionStreamFrame) string {
	t.Helper()
	for _, frames := range streams {
		for _, frame := range frames {
			if frame.Delivery == "REPLAY_SUMMARY" || frame.Event == nil {
				continue
			}
			var draft remoteWorkerSessionDraft
			if err := json.Unmarshal(frame.Event.Payload, &draft); err != nil {
				t.Fatalf("decode Worker recording identity: %v", err)
			}
			if draft.Payload.RecordingID != "" {
				return draft.Payload.RecordingID
			}
		}
	}
	t.Fatal("Worker stream omitted durable recording identity")
	return ""
}

func remoteLiveWorkerSessionFacts(
	t *testing.T,
	frames []remoteWorkerSessionStreamFrame,
	workerSessionID string,
) remoteWorkerSessionLiveFacts {
	t.Helper()
	var facts remoteWorkerSessionLiveFacts
	for _, frame := range frames {
		if frame.Delivery == "REPLAY_SUMMARY" || frame.Event == nil {
			continue
		}
		var draft remoteWorkerSessionDraft
		if err := json.Unmarshal(frame.Event.Payload, &draft); err != nil {
			t.Fatalf("decode live Worker Session %q facts: %v", workerSessionID, err)
		}
		if draft.Payload.WorkerSessionID != workerSessionID {
			continue
		}
		payload := draft.Payload
		if frame.Event.SourceType == "worker_session_lineage" || payload.Lineage != nil {
			facts.lineage = &payload
		}
		if frame.Event.SourceType == "worker_session_lifecycle" &&
			(frame.Event.SourceEventID == "started" || payload.StartedAt != nil) {
			facts.opening = &payload
		}
	}
	if facts.opening == nil || facts.lineage == nil {
		t.Fatalf("live Worker Session %q facts = %#v, want opening and continuation lineage", workerSessionID, facts)
	}
	return facts
}

func remotePortableWorkerSessionFacts(
	t *testing.T,
	portable recordings.WorkerPortableRecording,
) remoteWorkerSessionLiveFacts {
	t.Helper()
	var facts remoteWorkerSessionLiveFacts
	for _, record := range portable.Records {
		var draft remoteWorkerSessionDraft
		if err := json.Unmarshal(record.Payload, &draft); err != nil {
			t.Fatalf("decode portable Worker Session %q facts: %v", portable.Identity.WorkerSessionID, err)
		}
		if draft.Payload.WorkerSessionID != portable.Identity.WorkerSessionID {
			continue
		}
		payload := draft.Payload
		if string(record.SourceType) == "worker_session_lineage" || payload.Lineage != nil {
			facts.lineage = &payload
		}
		if string(record.SourceType) == "worker_session_lifecycle" &&
			(string(record.SourceEventID) == "started" || payload.StartedAt != nil) {
			facts.opening = &payload
		}
	}
	if facts.opening == nil || facts.lineage == nil {
		t.Fatalf("portable Worker Session %q facts = %#v, want opening and continuation lineage", portable.Identity.WorkerSessionID, facts)
	}
	return facts
}

func assertRemotePortableProviderFacts(
	t *testing.T,
	portable recordings.WorkerPortableRecording,
	want remoteWorkerSessionProviderRef,
	lineage remoteWorkerSessionPayload,
	wantOpeningContinuation bool,
) {
	t.Helper()
	if portable.Provider.Provider != want.Provider {
		t.Fatalf("portable Worker Session %q provider attribution = %#v, want provider=%q", portable.Identity.WorkerSessionID, portable.Provider, want.Provider)
	}
	if portable.Provider.ProviderSessionRef != "" && portable.Provider.ProviderSessionRef != want.ID {
		t.Fatalf("portable Worker Session %q provider session attribution = %q, want empty or exact id %q", portable.Identity.WorkerSessionID, portable.Provider.ProviderSessionRef, want.ID)
	}
	lineageContinuation := lineage.Continuation
	if lineageContinuation == nil || lineageContinuation.Provider != want.Provider || lineageContinuation.Kind != want.Kind || lineageContinuation.ID != want.ID {
		t.Fatalf("portable Worker Session %q lineage continuation = %#v, want exact provider reference %#v", portable.Identity.WorkerSessionID, lineageContinuation, want)
	}
	if wantOpeningContinuation {
		continuation := portable.Correlation.Continuation
		if continuation == nil || continuation.Provider != want.Provider || continuation.Kind != want.Kind || continuation.ID != want.ID {
			t.Fatalf("portable Worker Session %q opening continuation = %#v, want exact provider reference %#v", portable.Identity.WorkerSessionID, continuation, want)
		}
	} else if portable.Correlation.Continuation != nil {
		t.Fatalf("portable source Worker Session %q opening continuation = %#v, want no opening continuation", portable.Identity.WorkerSessionID, portable.Correlation.Continuation)
	}
	if portable.Correlation.DispatchID == "" || portable.Correlation.AttemptID == "" {
		t.Fatalf("portable Worker Session %q correlation = %#v, want explicit dispatch and attempt identities", portable.Identity.WorkerSessionID, portable.Correlation)
	}
}

func readRemoteProviderFixture(t *testing.T, provider, caseName, fileName string) []byte {
	t.Helper()
	path := filepath.Join(testutil.MustRepoRoot(t), filepath.FromSlash(support.ProviderSessionFixturePath(provider, caseName, fileName)))
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read provider fixture %s: %v", path, err)
	}
	return contents
}

func writeRemoteCodexRollout(t *testing.T, homeDir, sessionID string) {
	t.Helper()
	directory := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "27")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create Codex rollout directory: %v", err)
	}
	contents := readRemoteProviderFixture(t, "codex", "success", "rollout.jsonl")
	path := filepath.Join(directory, "rollout-"+sessionID+".jsonl")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write Codex rollout fixture: %v", err)
	}
}

func remoteFunctionalEnvironment(homeDir string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	return env
}

func containsRemoteArgSequence(args, want []string) bool {
	for index := 0; index <= len(args)-len(want); index++ {
		match := true
		for offset := range want {
			if args[index+offset] != want[offset] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

type remoteInvokeContinueRunner struct {
	gate      <-chan struct{}
	outputs   [][]byte
	started   chan struct{}
	firstDone chan struct{}
	startOnce sync.Once
	doneOnce  sync.Once

	mu       sync.Mutex
	calls    int
	canceled bool
	requests []platformprocess.CommandRequest
}

func newRemoteInvokeContinueRunner(gate <-chan struct{}, output []byte) *remoteInvokeContinueRunner {
	return &remoteInvokeContinueRunner{
		gate: gate, outputs: [][]byte{append([]byte(nil), output...), append([]byte(nil), output...)},
		started: make(chan struct{}), firstDone: make(chan struct{}),
	}
}

func (r *remoteInvokeContinueRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	index := r.calls
	r.calls++
	request.Args = append([]string(nil), request.Args...)
	r.requests = append(r.requests, request)
	r.mu.Unlock()
	if index == 0 {
		r.startOnce.Do(func() { close(r.started) })
		select {
		case <-r.gate:
		case <-ctx.Done():
			r.mu.Lock()
			r.canceled = true
			r.mu.Unlock()
			return platformprocess.CommandResult{}, ctx.Err()
		}
		r.doneOnce.Do(func() { close(r.firstDone) })
	}
	return platformprocess.CommandResult{Stdout: append([]byte(nil), r.outputs[minRemoteRunnerIndex(index, len(r.outputs))]...)}, nil
}

func minRemoteRunnerIndex(index, length int) int {
	if index < length {
		return index
	}
	return length - 1
}

func (r *remoteInvokeContinueRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for remote Worker Session provider admission")
	}
}

func (r *remoteInvokeContinueRunner) waitFirstCompleted(t *testing.T) {
	t.Helper()
	select {
	case <-r.firstDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for remote Worker Session provider completion")
	}
}

func (r *remoteInvokeContinueRunner) wasCanceled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.canceled
}

func (r *remoteInvokeContinueRunner) requestsSnapshot() []platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(r.requests))
	for index, request := range r.requests {
		requests[index] = request
		requests[index].Args = append([]string(nil), request.Args...)
	}
	return requests
}

var _ platformprocess.CommandRunner = (*remoteInvokeContinueRunner)(nil)
