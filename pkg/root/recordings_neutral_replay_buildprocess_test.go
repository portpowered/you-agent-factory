package root_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testpath"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// TestBuildProcessWiresRecordingsNeutralReplayGraph proves root.BuildProcess
// composes the Recordings graph that includes neutral replay without
// constructing Recordings implementation packages from pkg/root.
func TestBuildProcessWiresRecordingsNeutralReplayGraph(t *testing.T) {
	t.Parallel()

	replayReads := 0
	if _, err := root.BuildProcess(context.Background(), serviceedges.Edges{
		FactorySessionReplayRecordingReader: func(string) ([]byte, error) {
			replayReads++
			return nil, errors.New("replay input must remain inert during process construction")
		},
	}); err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if replayReads != 0 {
		t.Fatalf("BuildProcess() read replay input %d times, want zero before runtime opening", replayReads)
	}
}

// TestProcessExecuteRejectsInvalidPortableReplayBeforeLiveRuntimeConstruction
// proves the customer run command reaches the Recordings-owned replay loader
// through the canonical application graph. Rejected recording facts must not
// open any live provider, script, or Factory Session control path.
func TestProcessExecuteRejectsInvalidPortableReplayBeforeLiveRuntimeConstruction(t *testing.T) {
	payload := invalidPortableReplayOrderPayload(t)
	var replayReads atomic.Int32
	var providerRuns atomic.Int32
	var scriptRuns atomic.Int32
	var sessionIDRequests atomic.Int32
	var hostBindings atomic.Int32

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
			replayReads.Add(1)
			if path != "recording.json" {
				t.Fatalf("replay input path = %q, want recording.json", path)
			}
			return payload, nil
		},
		ProviderCommandRunner: replayConstructionCommandRunner{calls: &providerRuns},
		ScriptCommandRunner:   replayConstructionCommandRunner{calls: &scriptRuns},
		FactorySessionIDGenerator: func() string {
			sessionIDRequests.Add(1)
			return "must-not-create-live-session"
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			hostBindings.Add(1)
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	workingDirectory := t.TempDir()
	err = process.Execute(root.Input{
		Args: []string{
			"you", "run", "--dir", workingDirectory,
			"--replay", "recording.json", "--no-record", "--quiet",
		},
		Context:          t.Context(),
		Env:              replayTestHomeEnvironment(t.TempDir()),
		WorkingDirectory: workingDirectory,
	})
	if err == nil {
		t.Fatal("Process.Execute(run --replay) error = nil")
	}
	var replayErr *recordings.ReplayInputError
	if !errors.As(err, &replayErr) ||
		replayErr.Family != recordings.ReplayInputFamilyPortable ||
		replayErr.Diagnostic.Code != recordings.ReplayArtifactDiagnosticInvalidOrder {
		t.Fatalf("Process.Execute(run --replay) error = %v, want portable invalid-order diagnostic", err)
	}
	if replayReads.Load() != 1 {
		t.Fatalf("replay input reads = %d, want one canonical runtime-opening read", replayReads.Load())
	}
	if providerRuns.Load() != 0 || scriptRuns.Load() != 0 || sessionIDRequests.Load() != 0 || hostBindings.Load() != 0 {
		t.Fatalf(
			"live replay construction calls = provider:%d script:%d sessionID:%d host:%d, want zero",
			providerRuns.Load(), scriptRuns.Load(), sessionIDRequests.Load(), hostBindings.Load(),
		)
	}
}

// TestProcessExecuteReturnsPortableReplayInspectionWithoutLiveRuntimeConstruction
// proves the canonical customer command accepts a valid portable recording as
// an inspection-only historical replay. It must not allocate a live session or
// construct provider, script, or host components to do so.
func TestProcessExecuteReturnsPortableReplayInspectionWithoutLiveRuntimeConstruction(t *testing.T) {
	payload := validPortableReplayPayload(t)
	var replayReads atomic.Int32
	var providerRuns atomic.Int32
	var scriptRuns atomic.Int32
	var sessionIDRequests atomic.Int32
	var hostBindings atomic.Int32

	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
			replayReads.Add(1)
			if path != "recording.json" {
				t.Fatalf("replay input path = %q, want recording.json", path)
			}
			return payload, nil
		},
		ProviderCommandRunner: replayConstructionCommandRunner{calls: &providerRuns},
		ScriptCommandRunner:   replayConstructionCommandRunner{calls: &scriptRuns},
		FactorySessionIDGenerator: func() string {
			sessionIDRequests.Add(1)
			return "must-not-create-live-session"
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			hostBindings.Add(1)
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	workingDirectory := t.TempDir()
	var stdout bytes.Buffer
	err = process.Execute(root.Input{
		Args: []string{
			"you", "run", "--dir", workingDirectory,
			"--replay", "recording.json", "--no-record",
		},
		Context:          t.Context(),
		Env:              replayTestHomeEnvironment(t.TempDir()),
		WorkingDirectory: workingDirectory,
		Stdout:           &stdout,
	})
	if err != nil {
		t.Fatalf("Process.Execute(run --replay) error = %v", err)
	}
	if replayReads.Load() != 1 {
		t.Fatalf("replay input reads = %d, want one canonical runtime-opening read", replayReads.Load())
	}
	if providerRuns.Load() != 0 || scriptRuns.Load() != 0 || sessionIDRequests.Load() != 0 || hostBindings.Load() != 0 {
		t.Fatalf(
			"live replay construction calls = provider:%d script:%d sessionID:%d host:%d, want zero",
			providerRuns.Load(), scriptRuns.Load(), sessionIDRequests.Load(), hostBindings.Load(),
		)
	}
	for _, want := range []string{
		"Replayed Factory Session: session-js-001",
		"Source: workflow/example.js",
		"Status: SUCCEEDED",
		"Result: FINAL",
		"Artifacts: 1",
		"Artifact: artifact-1 (CHECKPOINT)",
		"Checkpoint: checkpoint-1 (Waiting for operator input)",
		"Events: 3",
		"Redaction: runtimeStateOmitted=true checkpointBodiesOmitted=true providerTranscriptsOmitted=true childDispatchesOmitted=true secretsRedacted=2",
		"Event 0: SESSION_STARTED (event-1)",
		"Event 1: JAVASCRIPT_CHECKPOINT_REF (event-2)",
		"Event 2: SESSION_COMPLETED (event-3)",
	} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("portable replay inspection output = %q, want %q", stdout.String(), want)
		}
	}
}

func invalidPortableReplayOrderPayload(t *testing.T) []byte {
	t.Helper()

	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read valid portable recording: %v", err)
	}
	recording, err := recordings.DecodePortableRecording(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("decode valid portable recording: %v", err)
	}
	recording.Events[1].Sequence = recording.Events[0].Sequence
	payload, err = json.Marshal(recording)
	if err != nil {
		t.Fatalf("marshal invalid portable recording: %v", err)
	}
	return payload
}

func validPortableReplayPayload(t *testing.T) []byte {
	t.Helper()

	checkpointAt := time.Date(2026, time.July, 12, 12, 0, 1, 0, time.UTC)
	recording, err := recordings.BuildPortableRecording(recordings.PortableRecordingCanonicalFacts{
		SessionID:        "session-js-001",
		Status:           "SUCCEEDED",
		OrchestratorKind: "JAVASCRIPT",
		SourceRef:        "workflow/example.js",
		SourceHash:       replayPortableDigest('1'),
		PolicyHash:       replayPortableDigest('3'),
		Artifacts: []recordings.PortableRecordingCanonicalArtifact{{
			ID: "artifact-1", Kind: "CHECKPOINT", Visibility: "PUBLIC", Label: "Approval checkpoint",
			ContentHash: replayPortableDigest('4'), SizeBytes: 42, CreatedAt: checkpointAt, SecretsRedacted: 2,
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

func replayPortableDigest(character byte) string {
	return "sha256:" + string(bytes.Repeat([]byte{character}, 64))
}

func replayTestHomeEnvironment(home string) []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"USERPROFILE=" + home}
	case "plan9":
		return []string{"home=" + home}
	default:
		return []string{"HOME=" + home}
	}
}

type replayConstructionCommandRunner struct {
	calls *atomic.Int32
}

func (runner replayConstructionCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if runner.calls != nil {
		runner.calls.Add(1)
	}
	return platformprocess.CommandResult{}, errors.New("replay validation must not execute commands")
}

var _ platformprocess.CommandRunner = replayConstructionCommandRunner{}
