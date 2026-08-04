package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
	t.Helper()

	checkpointAt := time.Date(2026, time.July, 12, 12, 0, 1, 0, time.UTC)
	recording, err := recordings.BuildPortableRecording(recordings.PortableRecordingCanonicalFacts{
		SessionID:        "session-js-001",
		Status:           "SUCCEEDED",
		OrchestratorKind: "JAVASCRIPT",
		SourceRef:        "workflow/example.js",
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
