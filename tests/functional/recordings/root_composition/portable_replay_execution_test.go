package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testpath"
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

func TestHistoricalReadKeepsTypedCorruptionAndMissingCauses(t *testing.T) {
	t.Parallel()
	var service recordings.Service
	var providerRuns, writes atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		RecordingsRootObserver: func(root recordings.Service) { service = root },
		RecordingReadFile:      os.ReadFile,
		RecordingWriteFile:     func(string, []byte) error { writes.Add(1); return errors.New("unexpected historical write") },
		ProviderCommandRunner:  functionalReplayCommandRunner{calls: &providerRuns},
	})
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
	after, err := os.ReadFile(corruptPath)
	if err != nil || !reflect.DeepEqual(corruptBytes, after) || writes.Load() != 0 || providerRuns.Load() != 0 {
		t.Fatalf("failed historical read changed source or executed effects: read=%v writes=%d provider=%d", err, writes.Load(), providerRuns.Load())
	}
}
