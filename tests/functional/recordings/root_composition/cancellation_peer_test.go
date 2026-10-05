package root_composition_test

import (
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func startConcurrentCancellationPeer(t *testing.T, process support.Process, service recordings.Service, runner *cancellationRecordingRunner, inputs *support.CapturedInputs) func() {
	t.Helper()
	const sessionID = "019a07c0-0000-7000-8000-000000000034"
	inputs.Args = append(inputs.Args, "--session", sessionID)
	var once sync.Once
	release := func() { once.Do(func() { close(runner.releasePeer) }) }
	done := executeGatedRecordingCommand(t, process, inputs, release)
	select {
	case <-runner.peerStarted:
	case err := <-done:
		t.Fatalf("concurrent peer ended before provider admission: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent peer provider was never admitted")
	}
	return func() {
		release()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("concurrent invocation affected by selected cancellation: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("concurrent peer did not complete")
		}
		if inputs.Stdout() != "Batch completed successfully.\n" {
			t.Fatalf("concurrent peer output = %q", inputs.Stdout())
		}
		id := recordings.RecordingID("019a07c0-0000-7000-8000-000000000034")
		status, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{RecordingID: id})
		if err != nil || status.Status.State != recordings.RecordingFinalized || len(status.Status.Failures) != 0 {
			t.Fatalf("concurrent peer recording = (%#v, %v)", status, err)
		}
		history, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: id, Artifact: status.Status.Artifact, Scope: recordings.CanonicalEventScope{FactorySessionID: sessionID},
		}})
		if err != nil || len(history.Dispatches) != 1 || history.Dispatches[0].Status != recordings.FactoryDispatchStatusCompleted {
			t.Fatalf("concurrent peer persisted dispatch = (%#v, %v)", history, err)
		}
	}
}
