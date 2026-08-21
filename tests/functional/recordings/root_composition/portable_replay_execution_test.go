package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testpath"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
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

// TestPortableReplayResumeProbeUsesRootExecutionOpening proves the
// checkpoint-bearing replay reaches the canonical Factory Sessions durable
// owner through the root-composed opening. A public checkpoint summary alone
// must still produce the non-live replay rejection when no durable snapshot is
// restorable.
func TestPortableReplayResumeProbeUsesRootExecutionOpening(t *testing.T) {
	sessionID := "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := functionalPortableReplayPayloadForSession(t, sessionID)
	calls := &functionalReplayLiveConstructionCalls{}
	home := t.TempDir()
	projectRoot := t.TempDir()
	process := support.BuildProcess(t, functionalPortableReplayEdges(t, payload, calls))
	opening := root.ExecutionRuntimeOpeningFromProcess(process)
	if opening == nil {
		t.Fatal("ExecutionRuntimeOpeningFromProcess() returned nil")
	}
	opened, err := opening.OpenExecutionRuntime(t.Context(), factorysessions.ExecutionRuntimeOpeningRequest{
		ProjectRoot:       projectRoot,
		SystemConfigHome:  home,
		FactorySessionID:  sessionID,
		ReplayPath:        "recording.json",
		PersistencePolicy: factorysessions.PersistencePolicyDisabled,
	})
	if err != nil {
		t.Fatalf("OpenExecutionRuntime() error = %v", err)
	}
	if opened.Close == nil {
		t.Fatal("OpenExecutionRuntime() close hook = nil")
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close replay execution runtime: %v", err)
		}
	})
	if opened.Execution == nil {
		t.Fatal("OpenExecutionRuntime() execution owner = nil")
	}
	if _, err := opened.Execution.ResumeInterruptedSession(
		t.Context(),
		sessionID,
		factorysessions.ResumeSessionRequest{RequestID: "resume-summary-only"},
	); err == nil {
		t.Fatal("ResumeInterruptedSession() error = nil, want non-live replay rejection")
	}
}

// TestPortableReplayResumeHandoffUsesRootExecutionOpening proves a real
// interrupted durable snapshot makes the portable replay owner assemble and
// delegate to the live execution owner. The resumed owner then answers the
// dispatch read, which keeps the handoff observable at the public durable
// boundary.
func TestPortableReplayResumeHandoffUsesRootExecutionOpening(t *testing.T) {
	const workflowName = "resumable-two-step-fake-children"
	projectRoot := functionalWriteResumableWorkflowFixture(t, workflowName)

	provider := newFunctionalReplayHandoffProvider()
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:       projectRoot,
		Env:              env,
		ProviderOverride: provider,
	})
	started := functionalPostJSON[factoryapi.FactorySessionExecutionResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/async",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: "portable-replay-handoff-start",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
				WorkflowName: functionalStringPointer(workflowName),
			},
			Args: &map[string]any{"subject": "workflows"},
		},
	)
	if started.SessionId == "" {
		t.Fatal("started session id is empty")
	}
	functionalWaitForDispatchStatus(t, server.URL(), started.SessionId, "dispatch-1", "COMPLETED")
	functionalWaitForDispatchStatus(t, server.URL(), started.SessionId, "dispatch-2", "RUNNING")
	provider.waitForSecondCall(t)
	reason := "portable replay handoff interrupt"
	functionalPostJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+url.PathEscape(started.SessionId)+"/interrupt-dispatch",
		factoryapi.FactorySessionInterruptDispatchRequest{DispatchId: "dispatch-2", Reason: &reason},
	)
	functionalWaitForDurableStatus(
		t, server.URL(), started.SessionId,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
		10*time.Second,
	)
	server.Stop(t)

	payload := functionalPortableReplayPayloadForSessionSource(t, started.SessionId, workflowName+".js")
	edges := functionalPortableReplayEdges(t, payload, &functionalReplayLiveConstructionCalls{})
	edges.ProviderOverride = support.MockInferenceProvider("resumed step")
	process := support.BuildProcess(t, edges)
	opening := root.ExecutionRuntimeOpeningFromProcess(process)
	if opening == nil {
		t.Fatal("ExecutionRuntimeOpeningFromProcess() returned nil")
	}
	opened, err := opening.OpenExecutionRuntime(t.Context(), factorysessions.ExecutionRuntimeOpeningRequest{
		ProjectRoot:       projectRoot,
		SystemConfigHome:  home,
		FactorySessionID:  started.SessionId,
		ReplayPath:        "recording.json",
		PersistencePolicy: factorysessions.PersistencePolicyEnabled,
	})
	if err != nil {
		t.Fatalf("OpenExecutionRuntime(restorable replay) error = %v", err)
	}
	if opened.Close == nil || opened.Execution == nil {
		t.Fatalf("opened restorable runtime = execution:%#v close:%v, want both", opened.Execution, opened.Close != nil)
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close restorable replay execution runtime: %v", err)
		}
	})
	resumed, err := opened.Execution.ResumeInterruptedSession(
		t.Context(),
		started.SessionId,
		factorysessions.ResumeSessionRequest{RequestID: "portable-replay-handoff-resume"},
	)
	if err != nil {
		t.Fatalf("ResumeInterruptedSession(restorable replay) error = %v", err)
	}
	if resumed.SessionID != started.SessionId {
		t.Fatalf("resumed session id = %q, want %q", resumed.SessionID, started.SessionId)
	}
	functionalWaitForDurableOwnerStatus(t, opened.Execution, started.SessionId, factorysessions.LifecycleStatusSucceeded, 10*time.Second)
	listed, err := opened.Execution.ListDispatches(t.Context(), started.SessionId)
	if err != nil {
		t.Fatalf("ListDispatches(after handoff) error = %v", err)
	}
	if len(listed.Dispatches) != 2 {
		t.Fatalf("ListDispatches(after handoff) count = %d, want two restored dispatches", len(listed.Dispatches))
	}
}

func functionalWriteResumableWorkflowFixture(t *testing.T, workflowName string) string {
	t.Helper()
	projectRoot := support.ScaffoldSingleStepFactory(t, "portable-replay-handoff")
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

func functionalWaitForDispatchStatus(
	t testing.TB,
	baseURL, sessionID, dispatchID, want string,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/dispatches"
	for time.Now().Before(deadline) {
		listed := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](t, endpoint)
		for _, dispatch := range listed.Dispatches {
			if dispatch.Id == dispatchID && string(dispatch.Status) == want {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("dispatch %s did not reach %s within timeout", dispatchID, want)
}

func functionalWaitForDurableStatus(
	t testing.TB,
	baseURL, sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	deadline := time.Now().Add(timeout)
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	for time.Now().Before(deadline) {
		response := support.GetJSON[factoryapi.FactorySessionGetResponse](t, endpoint)
		model, err := response.AsFactorySessionDurableReadModel()
		if err != nil {
			t.Fatalf("decode durable session status: %v", err)
		}
		if model.Status == want {
			return model
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("durable session %s did not reach %s within %s", sessionID, want, timeout)
	return factoryapi.FactorySessionDurableReadModel{}
}

func functionalWaitForDurableOwnerStatus(
	t testing.TB,
	owner factorysessions.DurableExecutionService,
	sessionID string,
	want factorysessions.LifecycleStatus,
	timeout time.Duration,
) factorysessions.DurableInspectResult {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		read, err := owner.GetSession(t.Context(), sessionID)
		if err != nil {
			t.Fatalf("GetSession(%s): %v", sessionID, err)
		}
		if read.Status == want {
			return read
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("durable owner session %s did not reach %s within %s", sessionID, want, timeout)
	return factorysessions.DurableInspectResult{}
}

type functionalReplayHandoffProvider struct {
	testutil.NativeProvider
	secondStarted chan struct{}
	secondOnce    sync.Once
	mu            sync.Mutex
	calls         int
}

func newFunctionalReplayHandoffProvider() *functionalReplayHandoffProvider {
	provider := &functionalReplayHandoffProvider{secondStarted: make(chan struct{})}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (provider *functionalReplayHandoffProvider) Execute(
	ctx context.Context,
	_ providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	provider.mu.Lock()
	provider.calls++
	call := provider.calls
	provider.mu.Unlock()
	if call == 2 {
		provider.secondOnce.Do(func() { close(provider.secondStarted) })
		<-ctx.Done()
		return providers.ExecuteResult{}, ctx.Err()
	}
	return providers.ExecuteResult{Content: "first child"}, nil
}

func (provider *functionalReplayHandoffProvider) waitForSecondCall(t testing.TB) {
	t.Helper()
	select {
	case <-provider.secondStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for second child provider call")
	}
}

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
