package lifecycle_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// One process creates real interrupted durable state, then opens its portable
// history for inspection. Inspection must preserve the interrupted snapshot.
func TestPortableCheckpointInspectionPreservesPersistedInterruptedChild(t *testing.T) {
	t.Parallel()
	dir := functionalWriteResumableWorkflowFixture(t, "resumable-two-step-fake-children")
	home := t.TempDir()
	runner := &checkpointContinuationRunner{entered: make(chan struct{}), canceled: make(chan struct{})}
	api := support.NewProcessAPIServer()
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{ProviderCommandRunner: runner, APIServerStarter: api.Start})
	if err != nil {
		t.Fatal(err)
	}
	support.CleanupProcess(t, process)
	inputs := recordingContinuationInputs(t, dir, home, []string{"--continuously", "--with-server", "--no-record"}, false)
	command := support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(func() { command.Stop(t) })
	api.WaitForURL(t)
	sessions := process.FactorySessions().FactorySessions().(factorysessions.Service)
	started, err := sessions.StartAsync(t.Context(), factorysessions.StartRequest{
		RequestID: uuid.NewString(), ProjectRoot: dir, PersistencePolicy: factorysessions.PersistencePolicyEnabled,
		Source: factorysessions.Source{Kind: "WORKFLOW_NAME", WorkflowName: "resumable-two-step-fake-children"},
		Args:   map[string]any{"subject": "portable checkpoint"},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitRecordingPeerSignal(t, runner.entered, "second child admission")
	if _, err := sessions.InterruptDispatch(t.Context(), started.SessionID, factorysessions.InterruptDispatchRequest{DispatchID: "dispatch-2"}); err != nil {
		t.Fatal(err)
	}
	waitRecordingPeerSignal(t, runner.canceled, "interrupted child cancellation")
	before := waitCheckpointContinuationStatus(t, sessions, started.SessionID, factorysessions.LifecycleStatusInterrupted)
	if before.Progress == nil || before.Progress.CompletedDispatches != 1 || before.LatestCheckpoint == nil {
		t.Fatalf("missing durable checkpoint/child: %#v", before)
	}
	snapshotPath := filepath.Join(dir, ".you-agent-factory", "durable-sessions", started.SessionID+".json")
	snapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	path := writeCheckpointContinuationRecording(t, sessions, before)
	command.Stop(t)
	replay := recordingContinuationInputs(t, dir, home, []string{"--replay", path, "--no-record"}, false)
	replay.Input.WorkingDirectory = dir
	replay.Input.Args = []string{"you", "run", "--dir", dir, "--replay", path, "--no-record"}
	if err := process.Execute(replay.Input); err != nil {
		t.Fatalf("portable inspection: %v", err)
	}
	if runner.calls.Load() != 2 || !strings.Contains(replay.Stdout(), "Checkpoint: "+before.LatestCheckpoint.ID) {
		t.Fatalf("inspection lost checkpoint or executed children: calls=%d output=%s", runner.calls.Load(), replay.Stdout())
	}
	retained, err := os.ReadFile(snapshotPath)
	if err != nil || !bytes.Equal(snapshot, retained) {
		t.Fatalf("inspection/cleanup changed interrupted durable child state: %v", err)
	}
}

// One process creates real interrupted durable state, then opens its portable
// history for inspection and explicitly hands it back to live execution.
func TestPortableCheckpointContinuesPersistedChildThroughInitialOpening(t *testing.T) {
	t.Parallel()
	dir, home, runner, process, sessions, started, path := preparePortableCheckpointContinuation(t)
	assertPortableCheckpointWithoutRestorableState(t, process, sessions, dir, home, path, started.SessionID, runner)
	writer := &checkpointInspectionWriter{entered: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(writer.release) }) }
	t.Cleanup(release)
	replay := recordingContinuationInputs(t, dir, home, []string{"--replay", path, "--no-record"}, false)
	replay.Input.WorkingDirectory = dir
	replay.Input.Args = []string{"you", "run", "--dir", dir, "--replay", path, "--no-record"}
	replay.Input.Stdout = writer
	done := executeGatedRecordingCommand(t, process, replay, release)
	select {
	case <-writer.entered:
	case err := <-done:
		t.Fatalf("replay before inspection: %v stdout=%s stderr=%s", err, replay.Stdout(), replay.Stderr())
	case <-time.After(10 * time.Second):
		t.Fatal("portable inspection did not start")
	}

	if runner.calls.Load() != 2 {
		t.Fatalf("inspection executed children: calls=%d", runner.calls.Load())
	}
	if _, err := sessions.ResumeInterruptedSession(t.Context(), started.SessionID, factorysessions.ResumeSessionRequest{RequestID: uuid.NewString()}); err != nil {
		t.Fatalf("checkpoint handoff: %v", err)
	}
	after := waitCheckpointContinuationStatus(t, sessions, started.SessionID, factorysessions.LifecycleStatusSucceeded)
	if after.Progress == nil || after.Progress.CompletedDispatches != 2 || runner.calls.Load() != 3 {
		t.Fatalf("continuation repeated completed child: %#v calls=%d", after, runner.calls.Load())
	}
	// A second resume is rejected by terminal lifecycle, and must not open or
	// execute another generation. The durable child identities survive handoff.
	if _, err := sessions.ResumeInterruptedSession(t.Context(), started.SessionID, factorysessions.ResumeSessionRequest{RequestID: uuid.NewString()}); err == nil {
		t.Fatal("terminal checkpoint resumed twice")
	}
	dispatches, err := sessions.ListDispatches(t.Context(), started.SessionID)
	if err != nil || len(dispatches.Dispatches) != 2 || runner.calls.Load() != 3 {
		t.Fatalf("retained dispatches: %#v err=%v calls=%d", dispatches, err, runner.calls.Load())
	}
	release()
	joinCheckpointInspection(t, done, dir, started.SessionID)
}

func assertPortableCheckpointWithoutRestorableState(t *testing.T, process support.Process, sessions factorysessions.Service, dir, home, path, sessionID string, runner *checkpointContinuationRunner) {
	t.Helper()
	writer := &checkpointInspectionWriter{entered: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	release := func() { once.Do(func() { close(writer.release) }) }
	replay := recordingContinuationInputs(t, dir, home, []string{"--replay", path, "--no-record"}, false)
	// The portable history identifies the same checkpoint, but this selected
	// execution workspace has no durable snapshot. It must stay inspection-only.
	replay.Input.WorkingDirectory = t.TempDir()
	replay.Input.Args = []string{"you", "run", "--dir", dir, "--replay", path, "--no-record"}
	replay.Input.Stdout = writer
	done := executeGatedRecordingCommand(t, process, replay, release)
	waitRecordingPeerSignal(t, writer.entered, "nonrestorable checkpoint inspection")
	_, err := sessions.ResumeInterruptedSession(t.Context(), sessionID, factorysessions.ResumeSessionRequest{RequestID: uuid.NewString()})
	if err == nil || !strings.Contains(err.Error(), "historical and do not support live execution") {
		t.Fatalf("nonrestorable checkpoint probe: %v", err)
	}
	read, err := sessions.GetSession(t.Context(), sessionID)
	if err != nil || read.Status != factorysessions.LifecycleStatusInterrupted || runner.calls.Load() != 2 {
		t.Fatalf("nonrestorable probe executed or lost inspection: %#v %v calls=%d", read, err, runner.calls.Load())
	}
	release()
	joinCheckpointInspection(t, done, dir, sessionID)
}

func joinCheckpointInspection(t *testing.T, done <-chan error, dir, sessionID string) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("replay cleanup: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("checkpoint cleanup did not join")
	}
	if _, err := os.Stat(filepath.Join(dir, ".you-agent-factory", "durable-sessions", sessionID+".json")); err != nil {
		t.Fatalf("cleanup removed durable child state: %v", err)
	}
}

func preparePortableCheckpointContinuation(t *testing.T) (string, string, *checkpointContinuationRunner, support.Process, factorysessions.Service, factorysessions.AsyncStartResult, string) {
	t.Helper()
	dir := functionalWriteResumableWorkflowFixture(t, "resumable-two-step-fake-children")
	home := t.TempDir()
	runner := &checkpointContinuationRunner{entered: make(chan struct{}), canceled: make(chan struct{})}
	api := support.NewProcessAPIServer()
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{ProviderCommandRunner: runner, APIServerStarter: api.Start})
	if err != nil {
		t.Fatal(err)
	}
	support.CleanupProcess(t, process)
	inputs := recordingContinuationInputs(t, dir, home, []string{"--continuously", "--with-server", "--no-record"}, false)
	command := support.StartProcessCommand(t, process, inputs.Input)
	t.Cleanup(func() { command.Stop(t) })
	api.WaitForURL(t)
	sessions := process.FactorySessions().FactorySessions().(factorysessions.Service)
	started, err := sessions.StartAsync(t.Context(), factorysessions.StartRequest{
		RequestID: uuid.NewString(), ProjectRoot: dir, PersistencePolicy: factorysessions.PersistencePolicyEnabled,
		Source: factorysessions.Source{Kind: "WORKFLOW_NAME", WorkflowName: "resumable-two-step-fake-children"},
		Args:   map[string]any{"subject": "portable checkpoint"},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitRecordingPeerSignal(t, runner.entered, "second child admission")
	if _, err := sessions.InterruptDispatch(t.Context(), started.SessionID, factorysessions.InterruptDispatchRequest{DispatchID: "dispatch-2"}); err != nil {
		t.Fatal(err)
	}
	waitRecordingPeerSignal(t, runner.canceled, "interrupted child cancellation")
	before := waitCheckpointContinuationStatus(t, sessions, started.SessionID, factorysessions.LifecycleStatusInterrupted)
	if before.Progress == nil || before.Progress.CompletedDispatches != 1 || before.LatestCheckpoint == nil {
		t.Fatalf("missing durable checkpoint/child: %#v", before)
	}
	path := writeCheckpointContinuationRecording(t, sessions, before)
	command.Stop(t)
	return dir, home, runner, process, sessions, started, path
}

func waitCheckpointContinuationStatus(t *testing.T, sessions factorysessions.Service, id string, status factorysessions.LifecycleStatus) factorysessions.SessionReadResult {
	t.Helper()
	read, err := support.WaitForObservation(10*time.Second, func() (factorysessions.SessionReadResult, error) {
		return sessions.GetSession(t.Context(), id)
	}, func(read factorysessions.SessionReadResult) bool { return read.Status == status })
	if err != nil {
		t.Fatalf("checkpoint status %s: %#v %v", status, read, err)
	}
	return read
}

func writeCheckpointContinuationRecording(t *testing.T, sessions factorysessions.Service, read factorysessions.SessionReadResult) string {
	t.Helper()
	events, err := sessions.ReadEvents(t.Context(), read.SessionID, factorysessions.EventReconnectRequest{})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := sessions.ListArtifacts(t.Context(), read.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	facts := recordings.PortableRecordingCanonicalFacts{SessionID: read.SessionID, Status: string(read.Status),
		OrchestratorKind: read.OrchestratorKind, SourceRef: read.ResolvedSource.SourceRef, SourceHash: read.SourceHash,
		PolicyHash: read.Policy.EffectiveHash, Events: events.Events, Arguments: map[string]any{"subject": "portable checkpoint"}}
	for _, artifact := range artifacts.Artifacts {
		detail, err := sessions.GetArtifact(t.Context(), read.SessionID, artifact.ID)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(detail.Content)
		facts.Artifacts = append(facts.Artifacts, recordings.PortableRecordingCanonicalArtifact{ID: artifact.ID, Kind: artifact.Kind,
			Visibility: artifact.Visibility, Label: artifact.Label, ContentHash: fmt.Sprintf("sha256:%x", digest), SizeBytes: int64(len(detail.Content)), CreatedAt: *artifact.CreatedAt})
	}
	for _, raw := range events.Events {
		var event struct {
			Context struct {
				CheckpointID string    `json:"checkpointId"`
				EventTime    time.Time `json:"eventTime"`
			} `json:"context"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		if event.Context.CheckpointID == read.LatestCheckpoint.ID {
			facts.Checkpoint = &recordings.PortableRecordingCanonicalCheckpoint{ID: read.LatestCheckpoint.ID, Label: read.LatestCheckpoint.Label, Timestamp: event.Context.EventTime}
		}
	}
	facts.Result = &recordings.PortableRecordingCanonicalResult{Status: "NOT_READY", Mode: "final"}
	portable, err := recordings.BuildPortableRecording(facts)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(portable)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "checkpoint.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type checkpointContinuationRunner struct {
	calls             atomic.Int32
	entered, canceled chan struct{}
}

func (runner *checkpointContinuationRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	call := runner.calls.Add(1)
	if call == 2 {
		close(runner.entered)
		<-ctx.Done()
		close(runner.canceled)
		return platformprocess.CommandResult{}, ctx.Err()
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(strings.Join(request.Args, " ") + " checkpoint COMPLETE")}, nil
}

type checkpointInspectionWriter struct {
	once             sync.Once
	entered, release chan struct{}
}

func (writer *checkpointInspectionWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "Replayed Factory Session:") {
		writer.once.Do(func() { close(writer.entered); <-writer.release })
	}
	return len(data), nil
}
