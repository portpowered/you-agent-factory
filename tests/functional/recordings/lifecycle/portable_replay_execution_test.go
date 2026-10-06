package lifecycle_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testpath"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPortableReplayInspectionExecutesThroughRootProcess proves a valid
// portable recording follows the customer-facing run command into the
// historical-replay path. Historical inspection must not create live runtime
// components as part of the replay.
func TestPortableReplayInspectionExecutesThroughRootProcess(t *testing.T) {
	t.Parallel()

	payload := functionalPortableReplayPayload(t)
	calls := &functionalReplayLiveConstructionCalls{}
	process := support.BuildProcess(t, functionalPortableReplayEdges(t, payload, calls))

	output := functionalExecutePortableReplay(t, process)
	functionalAssertPortableReplayInspection(t, output, calls)
}

// The API owns explicit durable-session controls and artifact selection. Capture
// its production handler at the host edge: this witness needs no listener. One
// process hosts both sessions; provider gates prove they really overlap.
func TestCancelActiveSessionPreservesPeerProviderAndArtifacts(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := support.ScaffoldFactory(t, map[string]any{"name": "recording-peer-isolation"})
	runner := &recordingPeerRunner{firstStarted: make(chan struct{}), peerStarted: make(chan struct{}),
		firstCanceled: make(chan error, 1), peerRelease: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(runner.peerRelease) }) }
	t.Cleanup(release)
	bound := make(chan http.Handler, 1)
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner:              runner,
		FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			bound <- request.Handler
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: request.Port})
			}
			<-ctx.Done()
			return nil
		},
	})
	inputs := recordingContinuationInputs(t, dir, home, []string{"--continuously", "--with-server", "--no-record"}, false)
	ctx, cancel := context.WithCancel(inputs.Context)
	inputs.Context = ctx
	done := executeGatedRecordingCommand(t, process, inputs, func() { release(); cancel() })
	var handler http.Handler
	select {
	case handler = <-bound:
	case err := <-done:
		t.Fatalf("host ended before handler binding: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("host handler did not bind")
	}
	firstRequest := recordingPeerWorkflowRequest("first")
	peerRequest := recordingPeerWorkflowRequest("peer")
	firstStarted := recordingPeerJSON[factoryapi.FactorySessionExecutionResponse](t, handler, "POST", "/factory-sessions/async", firstRequest)
	waitRecordingPeerSignal(t, runner.firstStarted, "first provider admission")
	first := firstStarted.SessionId
	peerStarted := recordingPeerJSON[factoryapi.FactorySessionExecutionResponse](t, handler, "POST", "/factory-sessions/async", peerRequest)
	waitRecordingPeerSignal(t, runner.peerStarted, "peer provider admission")
	peer := peerStarted.SessionId
	if first == "" || peer == "" || first == peer || first == "~default" || peer == "~default" {
		t.Fatalf("session identities must be explicit and independent: first=%s peer=%s", first, peer)
	}
	firstArtifact := assertRecordingPeerArtifact(t, handler, first, "first")
	peerArtifact := assertRecordingPeerArtifact(t, handler, peer, "peer")
	// Artifact IDs are session-scoped. Equal local IDs still select different
	// content through the owning session route; do not invent global uniqueness.
	if *firstArtifact.ContentHash == *peerArtifact.ContentHash {
		t.Fatal("independent sessions lost their distinct artifact content")
	}
	control := recordingPeerJSON[factoryapi.FactorySessionLifecycleControlResponse](t, handler, "POST", "/factory-sessions/"+first+"/cancel", map[string]any{})
	if control.SessionId != first || control.Operation != "CANCEL" || (control.Outcome != "ACCEPTED" && control.Outcome != "APPLIED") {
		t.Fatalf("selected cancellation control: %#v", control)
	}
	select {
	case err := <-runner.firstCanceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("selected provider cancellation: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("selected provider did not acknowledge cancellation")
	}
	firstTerminal := waitRecordingPeerStatus(t, handler, first, factoryapi.FactorySessionDurableLifecycleStatusCanceled)
	if firstTerminal.SessionId != first || firstTerminal.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceled {
		t.Fatalf("selected terminal result = %#v, want original canceled session", firstTerminal)
	}
	peerRead := recordingPeerJSON[factoryapi.FactorySessionDurableReadModel](t, handler, "GET", "/factory-sessions/"+peer, nil)
	if peerRead.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("canceling first terminated peer: %#v", peerRead)
	}
	firstCursor := assertRecordingPeerHistory(t, handler, first, "first")
	release()
	peerTerminal := waitRecordingPeerStatus(t, handler, peer, factoryapi.FactorySessionDurableLifecycleStatusSucceeded)
	if peerTerminal.SessionId != peer || peerTerminal.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded || runner.calls.Load() != 2 {
		t.Fatalf("peer did not finish its original provider attempt: result=%#v calls=%d", peerTerminal, runner.calls.Load())
	}
	assertRecordingPeerArtifactRetained(t, firstArtifact, assertRecordingPeerArtifact(t, handler, first, "first"))
	assertRecordingPeerArtifactRetained(t, peerArtifact, assertRecordingPeerArtifact(t, handler, peer, "peer"))
	assertRecordingPeerArtifactDetail(t, handler, first, firstArtifact, "first")
	assertRecordingPeerArtifactDetail(t, handler, peer, peerArtifact, "peer")
	assertRecordingPeerHistory(t, handler, peer, "peer")
	request := httptest.NewRequest("GET", "/factory-sessions/"+peer+"/events?after_event_id="+url.QueryEscape(firstCursor), nil).WithContext(t.Context())
	request.Header.Set("Accept", "application/json")
	foreign := httptest.NewRecorder()
	handler.ServeHTTP(foreign, request)
	if foreign.Code != http.StatusOK || !strings.Contains(foreign.Body.String(), `"outcome":"CURSOR_STALE"`) {
		t.Fatalf("foreign session cursor = %d: %s", foreign.Code, foreign.Body.String())
	}
	if runner.calls.Load() != 2 {
		t.Fatal("selected historical reads admitted new provider work")
	}
}

// Drain the public retained prefix using its declared count, then cancel only
// this snapshot request. No stream-quietness or optional-frame wait is used.
func assertRecordingPeerHistory(t *testing.T, handler http.Handler, id, label string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	writer := &recordingPeerSnapshotWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	request := httptest.NewRequest("GET", "/factory-sessions/"+id+"/events", nil).WithContext(ctx)
	handler.ServeHTTP(writer, request)
	if writer.Code != http.StatusOK || writer.count == 0 {
		t.Fatalf("selected history = %d: %s", writer.Code, writer.Body.String())
	}
	var events []factoryapi.FactoryEvent
	for _, line := range strings.Split(writer.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatal(err)
		}
		if event.Context.SessionId != nil && *event.Context.SessionId != id {
			t.Fatalf("selected history leaked foreign session: %#v", event)
		}
		events = append(events, event)
	}
	if len(events) != writer.count {
		t.Fatalf("retained history count=%d frames=%d", writer.count, len(events))
	}
	if !strings.Contains(writer.Body.String(), `"`+label+`"`) {
		t.Fatalf("selected history lost owned artifact: %s", writer.Body.String())
	}
	for index := 1; index < len(events); index++ {
		if events[index].Context.Sequence <= events[index-1].Context.Sequence {
			t.Fatal("selected history lost event order")
		}
	}
	reconnected := &recordingPeerSnapshotWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	reconnectContext, reconnectCancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer reconnectCancel()
	reconnected.cancel = reconnectCancel
	reconnect := httptest.NewRequest("GET", "/factory-sessions/"+id+"/events?after_event_id="+url.QueryEscape(events[0].Id), nil).WithContext(reconnectContext)
	handler.ServeHTTP(reconnected, reconnect)
	if reconnected.Code != http.StatusOK || reconnected.count != len(events)-1 {
		t.Fatalf("selected reconnect count=%d, want %d: %s", reconnected.count, len(events)-1, reconnected.Body.String())
	}
	return events[0].Id
}

type recordingPeerSnapshotWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
	count  int
}

func (writer *recordingPeerSnapshotWriter) Write(data []byte) (int, error) {
	n, err := writer.ResponseRecorder.Write(data)
	writer.count += bytes.Count(data, []byte("data:"))
	return n, err
}

func (writer *recordingPeerSnapshotWriter) Flush() {
	writer.ResponseRecorder.Flush()
	retained, err := strconv.Atoi(writer.Header().Get("X-Factory-Session-Retained-Event-Count"))
	if err == nil && writer.count >= retained {
		writer.cancel()
	}
}
func assertRecordingPeerArtifactRetained(t *testing.T, before, after factoryapi.FactorySessionArtifactSummary) {
	t.Helper()
	// Active runtime and persisted artifact summaries currently derive CreatedAt
	// differently. Preserve identity, content/hash, visibility and retrieval;
	// this witness makes no artifact timestamp-provenance claim.
	before.CreatedAt, after.CreatedAt = nil, nil
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("terminal artifact changed retained facts: before=%+v after=%+v", before, after)
	}
}

// Async session status is the API-owned completion observer. Provider channels
// establish admission/cancellation; this bounded read waits for the guaranteed
// terminal projection, never optional progress frames or stream quietness.
func waitRecordingPeerStatus(t *testing.T, handler http.Handler, id string, want factoryapi.FactorySessionDurableLifecycleStatus) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	result, err := support.WaitForObservation(30*time.Second,
		func() (factoryapi.FactorySessionDurableReadModel, error) {
			return recordingPeerJSON[factoryapi.FactorySessionDurableReadModel](t, handler, "GET", "/factory-sessions/"+id, nil), nil
		},
		func(result factoryapi.FactorySessionDurableReadModel) bool { return result.Status == want },
	)
	if err != nil {
		t.Fatalf("session %s terminal status: got=%#v want=%s error=%v", id, result, want, err)
	}
	return result
}

type recordingPeerRunner struct {
	calls                     atomic.Int32
	firstStarted, peerStarted chan struct{}
	firstCanceled             chan error
	peerRelease               chan struct{}
}

func (runner *recordingPeerRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	if strings.Contains(strings.Join(request.Args, " ")+string(request.Stdin), "recording-isolation-first") {
		close(runner.firstStarted)
		<-ctx.Done()
		runner.firstCanceled <- ctx.Err()
		return platformprocess.CommandResult{}, ctx.Err()
	}
	close(runner.peerStarted)
	select {
	case <-runner.peerRelease:
		return support.NewStaticSuccessCommandRunner("recording peer COMPLETE").Run(ctx, request)
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func recordingPeerWorkflowRequest(label string) factoryapi.FactorySessionExecutionRequest {
	return factoryapi.FactorySessionExecutionRequest{RequestId: "recording-isolation-" + label,
		Source: factoryapi.FactorySessionExecutionSource{Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline: `return (async function () {
  const artifact = workflow.artifact({kind: "log", label: "` + label + `", content: {owner: "` + label + `"}});
  const child = await agent.run({prompt: "recording-isolation-` + label + `", label: "` + label + `", modelProvider: "codex", model: "gpt-5-codex"});
  return {artifact, child};
})();`,
				},
			},
		},
	}
}

func waitRecordingPeerSignal[T any](t *testing.T, signal <-chan T, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatalf("did not observe %s", label)
	}
}

func recordingPeerResponse(ctx context.Context, handler http.Handler, method, path string, payload []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func recordingPeerJSON[T any](t *testing.T, handler http.Handler, method, path string, body any) T {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	response := recordingPeerResponse(ctx, handler, method, path, payload)
	if response.Code < 200 || response.Code >= 300 {
		t.Fatalf("%s %s = %d: %s", method, path, response.Code, response.Body.String())
	}
	var result T
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertRecordingPeerArtifact(t *testing.T, handler http.Handler, sessionID, label string) factoryapi.FactorySessionArtifactSummary {
	t.Helper()
	listed := recordingPeerJSON[factoryapi.ListFactorySessionArtifactsResponse](t, handler, "GET", "/factory-sessions/"+sessionID+"/artifacts", nil)
	var selected *factoryapi.FactorySessionArtifactSummary
	for _, artifact := range listed.Artifacts {
		if artifact.Label != nil && *artifact.Label == label && artifact.Kind == "log" {
			copy := artifact
			selected = &copy
		}
		if artifact.Label != nil && (*artifact.Label == "first" || *artifact.Label == "peer") && *artifact.Label != label {
			t.Fatalf("session %s inherited foreign artifact: %#v", sessionID, artifact)
		}
	}
	if selected == nil || selected.ContentHash == nil || *selected.ContentHash == "" {
		t.Fatalf("session %s has no durable %s artifact: %#v", sessionID, label, listed)
	}
	return *selected
}

func assertRecordingPeerArtifactDetail(t *testing.T, handler http.Handler, sessionID string, summary factoryapi.FactorySessionArtifactSummary, label string) {
	t.Helper()
	path := "/factory-sessions/" + sessionID + "/artifacts/" + summary.Id
	detail := recordingPeerJSON[factoryapi.FactorySessionArtifactDetail](t, handler, "GET", path, nil)
	// WORKFLOW_RUNTIME visibility exposes a hash and safe reference, not raw
	// bodies. Pin that public outcome and qualify equal local IDs by session.
	if detail.SessionId != sessionID || detail.Id != summary.Id || detail.Label == nil || *detail.Label != label ||
		detail.ContentHash == nil || *detail.ContentHash != *summary.ContentHash || detail.ContentRef == nil || detail.ContentRef.Href != path {
		t.Fatalf("selected artifact detail = %#v, want owned identity/hash/reference", detail)
	}
}

func functionalWriteResumableWorkflowFixture(t *testing.T, workflowName string) string {
	t.Helper()
	scaffoldRoot := support.ScaffoldSingleStepFactory(t, "portable-replay-handoff")
	projectRoot, err := os.MkdirTemp("", "portable-replay-handoff-")
	if err != nil {
		t.Fatalf("create portable replay project root: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(projectRoot); err != nil {
			t.Errorf("remove portable replay project root: %v", err)
		}
	})
	factoryConfig, err := os.ReadFile(filepath.Join(scaffoldRoot, "factory.json"))
	if err != nil {
		t.Fatalf("read portable replay factory scaffold: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "factory.json"), factoryConfig, 0o600); err != nil {
		t.Fatalf("copy portable replay factory scaffold: %v", err)
	}
	workflowPath := testpath.MustRepoPathFromCaller(
		t, 0, "tests", "fixtures", "javascript_runtime", workflowName+".workflow.js",
	)
	workflowBody, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read resumable workflow fixture: %v", err)
	}
	workflowDir := filepath.Join(projectRoot, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflow directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), workflowBody, 0o600); err != nil {
		t.Fatalf("write resumable workflow: %v", err)
	}
	return projectRoot
}

// TestWSRFT016RecordingCompatibilityThroughCustomerFacingReplayPath proves
// that the root-composed replay command keeps legacy recordings readable and
// carries the current Worker-history outcome through historical inspection.
//
// WSR-FT-016: legacy/current fixture load, honest legacy Worker-history
// unavailability, current Worker-history preservation, and no live execution.
func TestWSRFT016RecordingCompatibilityThroughCustomerFacingReplayPath(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name          string
		fixture       string
		wantSession   string
		wantStatus    string
		wantWorker    string
		wantEventLine string
	}{
		{
			name: "legacy-schema-v2", fixture: "valid-v2.json", wantSession: "session-js-001",
			wantStatus: "SUCCEEDED", wantWorker: "Worker history: UNAVAILABLE (reason=SCHEMA_DID_NOT_RECORD_CANONICAL_WORKER_HISTORY)",
			wantEventLine: "Events: 2",
		},
		{
			name: "current-schema-v3-worker-history", fixture: "valid-v3-worker-history.json", wantSession: "session-current-worker-001",
			wantStatus: "SUCCEEDED", wantWorker: "Worker history: AVAILABLE (reason=)", wantEventLine: "Events: 2",
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			payloadPath := testpath.MustRepoPathFromCaller(
				t, 0, "pkg", "services", "recordings", "internal", "artifacts", "testdata", testCase.fixture,
			)
			payload, err := os.ReadFile(payloadPath)
			if err != nil {
				t.Fatalf("read fixture %q: %v", testCase.fixture, err)
			}
			calls := &functionalReplayLiveConstructionCalls{}
			process := support.BuildProcess(t, functionalPortableReplayEdges(t, payload, calls))
			output := functionalExecutePortableReplay(t, process)
			if calls.replayReads.Load() != 1 || calls.providerRuns.Load() != 0 || calls.scriptRuns.Load() != 0 ||
				calls.sessionIDRequests.Load() != 0 || calls.hostBindings.Load() != 0 {
				t.Fatalf("replay/live calls = reads:%d provider:%d script:%d sessionID:%d host:%d", calls.replayReads.Load(), calls.providerRuns.Load(), calls.scriptRuns.Load(), calls.sessionIDRequests.Load(), calls.hostBindings.Load())
			}
			for _, want := range []string{
				"Replayed Factory Session: " + testCase.wantSession,
				"Status: " + testCase.wantStatus,
				testCase.wantWorker,
				testCase.wantEventLine,
			} {
				if !bytes.Contains([]byte(output), []byte(want)) {
					t.Fatalf("WSR-FT-016 replay output = %q, want %q", output, want)
				}
			}
		})
	}
}

type functionalReplayLiveConstructionCalls struct {
	replayReads       atomic.Int32
	providerRuns      atomic.Int32
	scriptRuns        atomic.Int32
	sessionIDRequests atomic.Int32
	hostBindings      atomic.Int32
}

func functionalPortableReplayEdges(
	t *testing.T,
	payload []byte,
	calls *functionalReplayLiveConstructionCalls,
) serviceedges.Edges {
	t.Helper()

	return serviceedges.Edges{
		FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
			calls.replayReads.Add(1)
			if path != "recording.json" {
				t.Fatalf("replay input path = %q, want recording.json", path)
			}
			return payload, nil
		},
		ProviderCommandRunner: functionalReplayCommandRunner{calls: &calls.providerRuns},
		ScriptCommandRunner:   functionalReplayCommandRunner{calls: &calls.scriptRuns},
		FactorySessionIDGenerator: func() string {
			calls.sessionIDRequests.Add(1)
			return "must-not-create-live-session"
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			calls.hostBindings.Add(1)
		},
	}
}

func functionalExecutePortableReplay(t *testing.T, process support.Process) string {
	t.Helper()

	workingDirectory := t.TempDir()
	var stdout bytes.Buffer
	err := process.Execute(root.Input{
		Args: []string{
			"you", "run", "--dir", workingDirectory,
			"--replay", "recording.json", "--no-record",
		},
		Context: t.Context(),
		Env: append(
			os.Environ(),
			"HOME="+t.TempDir(),
			"USERPROFILE="+t.TempDir(),
		),
		WorkingDirectory: workingDirectory,
		Stdout:           &stdout,
	})
	if err != nil {
		t.Fatalf("Process.Execute(run --replay) error = %v\nstdout:\n%s", err, stdout.String())
	}
	return stdout.String()
}

func functionalAssertPortableReplayInspection(
	t *testing.T,
	output string,
	calls *functionalReplayLiveConstructionCalls,
) {
	t.Helper()

	if calls.replayReads.Load() != 1 {
		t.Fatalf("replay input reads = %d, want one", calls.replayReads.Load())
	}
	if calls.providerRuns.Load() != 0 || calls.scriptRuns.Load() != 0 ||
		calls.sessionIDRequests.Load() != 0 || calls.hostBindings.Load() != 0 {
		t.Fatalf(
			"live construction calls = provider:%d script:%d sessionID:%d host:%d, want zero",
			calls.providerRuns.Load(), calls.scriptRuns.Load(),
			calls.sessionIDRequests.Load(), calls.hostBindings.Load(),
		)
	}
	for _, want := range []string{
		"Replayed Factory Session: session-js-001",
		"Source: workflow/example.js",
		"Status: SUCCEEDED",
		"Result: FINAL",
		"Worker history: UNAVAILABLE (reason=CANONICAL_WORKER_HISTORY_NOT_CAPTURED)",
		"Artifacts: 1",
		"Artifact: artifact-1 (CHECKPOINT)",
		"Checkpoint: checkpoint-1 (Waiting for operator input)",
		"Events: 3",
		"Redaction: runtimeStateOmitted=true checkpointBodiesOmitted=true providerTranscriptsOmitted=true childDispatchesOmitted=true secretsRedacted=2",
		"Event 0: SESSION_STARTED (event-1)",
		"Event 1: JAVASCRIPT_CHECKPOINT_REF (event-2)",
		"Event 2: SESSION_COMPLETED (event-3)",
	} {
		if !bytes.Contains([]byte(output), []byte(want)) {
			t.Fatalf("portable replay inspection output = %q, want %q", output, want)
		}
	}
}

func functionalPortableReplayPayload(t *testing.T) []byte {
	return functionalPortableReplayPayloadForSession(t, "session-js-001")
}

func functionalPortableReplayPayloadForSession(t *testing.T, sessionID string) []byte {
	return functionalPortableReplayPayloadForSessionSource(t, sessionID, "workflow/example.js")
}

func functionalPortableReplayPayloadForSessionSource(t *testing.T, sessionID, sourceRef string) []byte {
	t.Helper()

	checkpointAt := time.Date(2026, time.July, 12, 12, 0, 1, 0, time.UTC)
	recording, err := recordings.BuildPortableRecording(recordings.PortableRecordingCanonicalFacts{
		SessionID:        sessionID,
		Status:           "SUCCEEDED",
		OrchestratorKind: "JAVASCRIPT",
		SourceRef:        sourceRef,
		SourceHash:       functionalReplayDigest('1'),
		PolicyHash:       functionalReplayDigest('3'),
		Artifacts: []recordings.PortableRecordingCanonicalArtifact{{
			ID: "artifact-1", Kind: "CHECKPOINT", Visibility: "PUBLIC", Label: "Approval checkpoint",
			ContentHash: functionalReplayDigest('4'), SizeBytes: 42, CreatedAt: checkpointAt, SecretsRedacted: 2,
		}},
		Events: []json.RawMessage{
			json.RawMessage(`{"id":"event-1","type":"SESSION_STARTED","context":{"sequence":0,"eventTime":"2026-07-12T12:00:00Z"},"payload":{}}`),
			json.RawMessage(`{"id":"event-2","type":"JAVASCRIPT_CHECKPOINT_REF","context":{"sequence":1,"eventTime":"2026-07-12T12:00:01Z","checkpointId":"checkpoint-1"},"payload":{"artifactIds":["artifact-1"]}}`),
			json.RawMessage(`{"id":"event-3","type":"SESSION_COMPLETED","context":{"sequence":2,"eventTime":"2026-07-12T12:00:02Z"},"payload":{"artifactIds":["artifact-1"]}}`),
		},
		Checkpoint: &recordings.PortableRecordingCanonicalCheckpoint{
			ID: "checkpoint-1", Label: "Approval", Summary: "Waiting for operator input",
			ArtifactID: "artifact-1", Timestamp: checkpointAt,
		},
		Result: &recordings.PortableRecordingCanonicalResult{
			Status: "FINAL", Mode: "final", PrimaryResult: json.RawMessage(`{"answer":"done"}`), ArtifactIDs: []string{"artifact-1"},
		},
	})
	if err != nil {
		t.Fatalf("build valid portable recording: %v", err)
	}
	payload, err := json.Marshal(recording)
	if err != nil {
		t.Fatalf("marshal valid portable recording: %v", err)
	}
	return payload
}

func functionalReplayDigest(character byte) string {
	return "sha256:" + string(bytes.Repeat([]byte{character}, 64))
}

func functionalPostJSON[T any](t testing.TB, endpoint string, request any) T {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal POST %s: %v", endpoint, err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result T
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode POST %s: %v", endpoint, err)
	}
	return result
}

func functionalStringPointer(value string) *string {
	return &value
}

func functionalWaitForResponseTerminal(t testing.TB, cursor *factorysessions.ResponseEventCursor) {
	t.Helper()
	if cursor == nil {
		t.Fatal("response-event cursor is nil")
	}
	defer cursor.Detach()
	// Cursor.Next blocks on retained/live publication. The bounded context is
	// only a failure guard for a malformed or stalled response stream, not a
	// polling interval.
	waitContext, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		events, err := cursor.Next(waitContext)
		if err != nil {
			t.Fatalf("read resumed response events: %v", err)
		}
		for _, event := range events {
			if event.Kind == factorysessions.ResponseEventKindRun &&
				event.Phase == factorysessions.ResponseEventPhaseCompleted {
				return
			}
		}
	}
}

func functionalWaitForInitialResponseTerminal(
	t testing.TB,
	stream *support.FactoryResponseEventStream,
) {
	t.Helper()
	for {
		// The public response stream closes only after the runtime has persisted
		// its terminal candidate. Each frame wait is a failure guard, not polling.
		event := stream.NextFrame(10 * time.Second).Event
		if event.Kind == factoryapi.FactoryResponseEventKindRun &&
			(event.Phase == factoryapi.FactoryResponseEventPhaseCompleted ||
				event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
				event.Phase == factoryapi.FactoryResponseEventPhaseCanceled) {
			return
		}
		if event.Kind == factoryapi.FactoryResponseEventKindError &&
			(event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
				event.Phase == factoryapi.FactoryResponseEventPhaseCanceled) {
			return
		}
	}
}

type functionalReplayHandoffCommandRunner struct {
	secondStarted     chan struct{}
	secondReleased    chan struct{}
	secondOnce        sync.Once
	secondReleaseOnce sync.Once
	calls             atomic.Int32
}

func newFunctionalReplayHandoffCommandRunner() *functionalReplayHandoffCommandRunner {
	return &functionalReplayHandoffCommandRunner{
		secondStarted:  make(chan struct{}),
		secondReleased: make(chan struct{}),
	}
}

func (runner *functionalReplayHandoffCommandRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	call := runner.calls.Add(1)
	if call == 2 {
		runner.secondOnce.Do(func() { close(runner.secondStarted) })
		<-ctx.Done()
		runner.secondReleaseOnce.Do(func() { close(runner.secondReleased) })
		return platformprocess.CommandResult{}, ctx.Err()
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("first child")}, nil
}

func (runner *functionalReplayHandoffCommandRunner) waitForSecondCall(t testing.TB) {
	t.Helper()
	// The command edge owns this admission signal; the timeout only guards a
	// broken workflow that never reaches the second child.
	select {
	case <-runner.secondStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for second child command")
	}
}

func (runner *functionalReplayHandoffCommandRunner) waitForSecondRelease(t testing.TB) {
	t.Helper()
	// Interrupt cancellation is the release signal. The timeout is only a
	// failure guard if the control request does not cancel the child context.
	select {
	case <-runner.secondReleased:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for interrupt to release second child command")
	}
}

var _ platformprocess.CommandRunner = (*functionalReplayHandoffCommandRunner)(nil)

type functionalReplayCommandRunner struct {
	calls *atomic.Int32
}

func (runner functionalReplayCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if runner.calls != nil {
		runner.calls.Add(1)
	}
	return platformprocess.CommandResult{}, errors.New("historical replay must not execute commands")
}

var _ platformprocess.CommandRunner = functionalReplayCommandRunner{}

func assertContinuationExport(t *testing.T, service recordings.Service, history recordings.HistoricalRecordingQueryResult, id, path string) {
	t.Helper()
	exported, err := service.ExportPortableArtifact(t.Context(), recordings.ExportPortableArtifactRequest{RecordingID: recordings.RecordingID(id)})
	if err != nil || exported.Reference != recordings.RecordingArtifactReference(path) {
		t.Fatalf("public successor export = (%#v, %v)", exported.Reference, err)
	}
	read, err := service.ReadPortableArtifact(t.Context(), recordings.ReadPortableArtifactRequest{
		RecordingID: recordings.RecordingID(id), Reference: exported.Reference,
	})
	if err != nil || len(read.Artifact.Events) != len(history.Events) {
		t.Fatalf("public exported read = %d events, %v", len(read.Artifact.Events), err)
	}
	for index, event := range read.Artifact.Events {
		assertPublicHistoricalFact(t, event, history.Events[index])
	}
	exportedHistory := queryContinuationHistory(t, service, path, id)
	// Legacy inspection reconstructs a historical generation; portable export
	// preserves the native generation. Compare identity, order, usage and
	// associations after mapping only that documented representation difference.
	if len(exportedHistory.Dispatches) != len(history.Dispatches) {
		t.Fatal("export lost dispatch history")
	}
	generation := read.Artifact.Events[0].Cursor.StreamGenerationID
	if generation == "" {
		t.Fatal("export lost native stream generation")
	}
	for index, dispatch := range exportedHistory.Dispatches {
		want := history.Dispatches[index]
		want.FirstCursor.StreamGenerationID = generation
		want.LastCursor.StreamGenerationID = generation
		if want.Association != nil {
			association := *want.Association
			association.Cursor.StreamGenerationID = generation
			want.Association = &association
		}
		if !reflect.DeepEqual(dispatch, want) {
			t.Fatalf("export changed dispatch[%d] facts: got=%#v want=%#v", index, dispatch, want)
		}
	}
	bytesBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.CreateReplayPlan(recordings.CreateReplayPlanRequest{
		SchemaVersion: recordings.ReplayPlanSchemaV1, Timing: recordings.ReplayTimingOrderOnly,
		Recording: recordings.ReplayRecordingFacts{RecordingID: recordings.RecordingID(id), Scope: history.Recording.Scope, Events: read.Artifact.Events},
	})
	if err != nil {
		t.Fatalf("public exported replay plan: %v", err)
	}
	for index := range read.Artifact.Events {
		observed, err := service.ObserveReplay(recordings.ObserveReplayRequest{Plan: plan.Plan.Handle})
		if err != nil || observed.Observation.ProcessedEvents != index+1 {
			t.Fatalf("public replay progress[%d] = (%#v, %v)", index, observed, err)
		}
		if index == len(read.Artifact.Events)-1 && observed.Observation.Kind != recordings.ReplayCompleted {
			t.Fatalf("exported replay did not complete: %#v", observed)
		}
	}
	bytesAfter, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(bytesBefore, bytesAfter) {
		t.Fatalf("read-only exported replay changed source: %v", err)
	}
}

// These read-only scenarios share immutable production edges. Each parallel cell
// owns its source, profile, inputs and synchronous Execute completion; parent
// cleanup joins all cells before checking that no live effect was admitted.
func TestReadOnlyHistoricalInspection(t *testing.T) {
	t.Parallel()
	var service recordings.Service
	var providerRuns, writes atomic.Int32
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		RecordingsRootObserver: func(root recordings.Service) { service = root },
		RecordingReadFile:      os.ReadFile,
		RecordingWriteFile:     func(string, []byte) error { writes.Add(1); return errors.New("unexpected historical write") },
		ProviderCommandRunner:  functionalReplayCommandRunner{calls: &providerRuns},
	})
	if err != nil {
		t.Fatal(err)
	}
	support.CleanupProcess(t, process)
	sessions := process.FactorySessions().FactorySessions().(factorysessions.Service)
	t.Cleanup(func() {
		if writes.Load() != 0 || providerRuns.Load() != 0 {
			t.Errorf("read-only scenarios admitted live effects: writes=%d provider=%d", writes.Load(), providerRuns.Load())
		}
	})
	t.Run("TestHistoricalReadKeepsTypedCorruptionAndMissingCauses", func(t *testing.T) {
		t.Parallel()
		assertHistoricalReadKeepsTypedCauses(t, process, service)
	})
	t.Run("TestEmptyRecordingKeepsInspectionAndUnavailableProjection", func(t *testing.T) {
		t.Parallel()
		assertEmptyRecordingInspection(t, process, service)
	})
	t.Run("selected concurrent reads and peer release", func(t *testing.T) {
		t.Parallel()
		assertSelectedReplayPeers(t, process, sessions)
	})
}

// F17F-1 and the ordinary release portion of F17F-7 use two simultaneously
// held CLI inspections on the same process, each with selected durable reads.
func assertSelectedReplayPeers(t *testing.T, process support.Process, sessions factorysessions.Service) {
	t.Helper()
	type peer struct {
		id, path string
		payload  []byte
		done     <-chan error
		release  func()
	}
	peers := make([]peer, 2)
	for i := range peers {
		id := fmt.Sprintf("session-js-selected-%s-%d", filepath.Base(t.TempDir()), i)
		payload := functionalPortableReplayPayloadForSessionSource(t, id, "workflow/"+id+".js")
		dir := t.TempDir()
		path := filepath.Join(dir, "selected.json")
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		writer := &checkpointInspectionWriter{entered: make(chan struct{}), release: make(chan struct{})}
		var once sync.Once
		release := func() { once.Do(func() { close(writer.release) }) }
		inputs := recordingContinuationInputs(t, dir, t.TempDir(), []string{"--replay", path, "--no-record"}, false)
		inputs.Input.Args = []string{"you", "run", "--dir", dir, "--session", id + "-inspection", "--replay", path, "--no-record"}
		inputs.Input.WorkingDirectory = dir
		inputs.Input.Stdout = writer
		peers[i] = peer{id: id, path: path, payload: payload, release: release,
			done: executeGatedRecordingCommand(t, process, inputs, release)}
		select {
		case <-writer.entered:
		case err := <-peers[i].done:
			t.Fatalf("selected inspection returned before readiness: %v; %s", err, inputs.Stderr())
		case <-time.After(10 * time.Second):
			t.Fatal("selected inspection did not become ready")
		}
	}
	for _, selected := range peers {
		assertSelectedReplayRead(t, sessions, selected.id)
	}
	// Releasing one owned opening must preserve the other selected route.
	peers[0].release()
	assertSelectedReplayCommandJoined(t, peers[0].done)
	assertSelectedReplayRead(t, sessions, peers[1].id)
	peers[1].release()
	assertSelectedReplayCommandJoined(t, peers[1].done)
	for _, selected := range peers {
		after, err := os.ReadFile(selected.path)
		if err != nil || !bytes.Equal(after, selected.payload) {
			t.Fatalf("selected replay changed source %s: %v", selected.id, err)
		}
	}
}

func assertSelectedReplayCommandJoined(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("selected replay cleanup: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("selected replay cleanup did not join")
	}
}

func assertSelectedReplayRead(t *testing.T, sessions factorysessions.Service, id string) {
	t.Helper()
	read, err := sessions.GetSession(t.Context(), id)
	if err != nil || read.SessionID != id || read.ResolvedSource.SourceRef != "workflow/"+id+".js" || read.Status != factorysessions.LifecycleStatusSucceeded {
		t.Fatalf("selected session %s: %#v, %v", id, read, err)
	}
	for _, mode := range []factorysessions.ResultMode{factorysessions.ResultModeFinal, factorysessions.ResultModePartial} {
		for _, include := range []bool{false, true} {
			result, resultErr := sessions.GetResult(t.Context(), id, factorysessions.ResultRequest{Mode: mode, IncludeArtifacts: include})
			if resultErr != nil || result.SessionID != id || result.Mode != mode || result.IncludeArtifacts != include || (len(result.ArtifactRefs) == 1) != include || string(result.PrimaryResult) != `{"answer":"done"}` {
				t.Fatalf("selected result %s mode=%s include=%t: %#v, %v", id, mode, include, result, resultErr)
			}
			result.PrimaryResult[0] = ' '
		}
	}
	artifacts, err := sessions.ListArtifacts(t.Context(), id)
	if err != nil || artifacts.SessionID != id || len(artifacts.Artifacts) != 1 || artifacts.Artifacts[0].ID != "artifact-1" {
		t.Fatalf("selected artifacts %s: %#v, %v", id, artifacts, err)
	}
	artifacts.Artifacts[0].ID = "caller-mutated"
	artifact, err := sessions.GetArtifact(t.Context(), id, "artifact-1")
	if err != nil || artifact.SessionID != id || artifact.ID != "artifact-1" || len(artifact.Content) != 0 {
		t.Fatalf("selected redacted artifact %s: %#v, %v", id, artifact, err)
	}
	sequence := 0
	events, err := sessions.ReadEvents(t.Context(), id, factorysessions.EventReconnectRequest{AfterSequence: &sequence})
	if err != nil || events.SessionID != id || len(events.Events) != 2 {
		t.Fatalf("selected reconnect %s: %#v, %v", id, events, err)
	}
	for i, raw := range events.Events {
		var event struct {
			ID      string `json:"id"`
			Context struct {
				SessionID string `json:"sessionId"`
				Sequence  int    `json:"sequence"`
			} `json:"context"`
		}
		if err := json.Unmarshal(raw, &event); err != nil || event.Context.Sequence != i+1 || event.ID != fmt.Sprintf("event-%d", i+2) {
			t.Fatalf("selected event %s: %s, %v", id, raw, err)
		}
	}
}

func assertHistoricalReadKeepsTypedCauses(t *testing.T, process support.Process, service recordings.Service) {
	t.Helper()
	corruptPath := filepath.Join(t.TempDir(), "corrupt.json")
	corruptBytes := []byte(`{"schemaVersion":"agent-factory.replay.v1","recordedAt":"2026-10-03T12:34:56Z","events":[{"schemaVersion":"agent-factory.event.v1","id":"broken","type":"WORK_REQUEST","context":{"sequence":0,"eventTime":"2026-10-03T12:34:56Z"},"payload":[]}]}`)
	if err := os.WriteFile(corruptPath, corruptBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, cell := range []struct {
		name string
		path string
		kind recordings.HistoricalRecordingQueryErrorKind
	}{
		{"corrupt", corruptPath, recordings.HistoricalRecordingQueryErrorCorruptHistory},
		{"missing", filepath.Join(t.TempDir(), "missing.json"), recordings.HistoricalRecordingQueryErrorMissingHistory},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			result, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
				Recording: recordings.HistoricalRecordingIdentity{RecordingID: "selected-history", Artifact: recordings.RecordingArtifactReference(cell.path)},
			})
			var typed *recordings.HistoricalRecordingQueryError
			if !errors.As(err, &typed) || typed.Kind != cell.kind || len(result.Events) != 0 {
				t.Fatalf("selected historical read = (%#v, %v), want typed %s and no partial facts", result, err, cell.kind)
			}
			if cell.name == "corrupt" && !errors.Is(err, recordings.ErrInvalidProjectionInput) {
				t.Fatalf("corrupt event lost typed projection cause: %v", err)
			}
			if cell.name == "missing" && !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("missing history lost filesystem cause: %v", err)
			}
			inputs := recordingContinuationInputs(t, t.TempDir(), t.TempDir(), []string{"--replay", cell.path, "--no-record"}, false)
			if err := process.Execute(inputs.Input); err == nil {
				t.Fatal("invalid selected replay unexpectedly succeeded")
			}
		})
	}
	t.Cleanup(func() {
		after, err := os.ReadFile(corruptPath)
		if err != nil || !reflect.DeepEqual(corruptBytes, after) {
			t.Errorf("failed historical read changed source: %v", err)
		}
	})
}

func assertEmptyRecordingInspection(t *testing.T, process support.Process, service recordings.Service) {
	t.Helper()
	artifact, err := recordings.BuildPortableRecording(recordings.PortableRecordingCanonicalFacts{
		SessionID: "session-empty-history", Status: "SUCCEEDED", OrchestratorKind: "JAVASCRIPT",
		SourceRef: "workflow/empty.js", SourceHash: functionalReplayDigest('a'), PolicyHash: functionalReplayDigest('b'),
		Events: []json.RawMessage{},
		Result: &recordings.PortableRecordingCanonicalResult{Status: "FINAL", Mode: "final"},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{RecordingID: "empty-history", Artifact: recordings.RecordingArtifactReference(path)},
	})
	var unavailable *recordings.HistoricalRecordingQueryError
	if !errors.As(err, &unavailable) || unavailable.Kind != recordings.HistoricalRecordingQueryErrorCorruptHistory || len(history.Events) != 0 || len(history.Dispatches) != 0 {
		t.Fatalf("empty public projection = %#v, %v, want characterized CORRUPT_HISTORY without invented facts", history, err)
	}
	inputs := recordingContinuationInputs(t, t.TempDir(), t.TempDir(), []string{"--replay", path, "--no-record"}, false)
	inputs.Args = []string{"you", "run", "--dir", t.TempDir(), "--replay", path, "--no-record"}
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("empty Execute replay: %v\n%s", err, inputs.Stderr())
	}
	if !strings.Contains(inputs.Stdout(), "Events: 0") || !strings.Contains(inputs.Stdout(), "Status: SUCCEEDED") {
		t.Fatalf("empty replay inspection = %s", inputs.Stdout())
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(payload, unchanged) {
		t.Fatalf("empty replay changed source: %v", err)
	}
}
