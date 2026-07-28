package root_composition_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRecordReplayLifecycleActivatesThroughRootBuildProcessAfterLifecycle proves
// record and replay lifecycle activate only after runtime lifecycle on a process
// constructed only through root.BuildProcess with Recordings effects replaced via
// edges.Edges. Detailed record/replay coverage remains under
// tests/functional/replay_contracts and smoke/CLI record-replay proofs; this
// test closes the explicit public-process activation gap for the Recordings FUN
// suite home.
func TestRecordReplayLifecycleActivatesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	recorder := newRecordingsActivationRecorder()
	dir := support.ScaffoldFactory(t, recordingsLifecycleActivationFactoryConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"FUN Recordings record/replay activation"}`))

	artifactPath := filepath.Join(t.TempDir(), "fun-recordings-activation.replay.json")
	recordServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		Args:                      []string{"--record", artifactPath},
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     recorder.edges(),
	})
	t.Cleanup(func() { recordServer.Stop(t) })

	support.WaitForTerminalStatus(t, recordServer.URL(), 15*time.Second)
	waitForRecordingsActivationArtifact(t, artifactPath)

	if got := recorder.totalCanonicalRecording(); got <= 0 {
		t.Fatalf(
			"canonical submission/dispatch recording effect calls after lifecycle = %d, want > 0 via edges",
			got,
		)
	}

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if recordingsActivationEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("recorded artifact missing dispatch request events")
	}
	if recordingsActivationEventCount(artifact, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("recorded artifact missing dispatch response events")
	}

	recordServer.Stop(t)

	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath, "--no-record"},
	})
	t.Cleanup(func() { replayServer.Stop(t) })

	support.WaitForTerminalStatus(t, replayServer.URL(), 15*time.Second)
	replayedWork := support.ListDefaultSessionWork(t, replayServer.URL())
	if got := support.CountWorkAtCustomerState(replayedWork, "task:complete"); got != 1 {
		t.Fatalf(
			"replayed work at task:complete = %d, want 1; listed=%#v",
			got,
			replayedWork.Results,
		)
	}

	replayedEvents := replayServer.GetFactoryEvents(t)
	if recordingsActivationLiveEventCount(replayedEvents, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("replayed session missing dispatch response events")
	}
}

func recordingsLifecycleActivationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "recordings-lifecycle-activation",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "mock-worker"}},
		"workstations": []map[string]any{{
			"name":      "process-task",
			"worker":    "mock-worker",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func waitForRecordingsActivationArtifact(t *testing.T, artifactPath string) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(artifactPath)
		if err == nil {
			var artifact struct {
				Events []factoryapi.FactoryEvent `json:"events"`
			}
			if json.Unmarshal(data, &artifact) == nil &&
				recordingsActivationLiveEventCount(artifact.Events, factoryapi.FactoryEventTypeDispatchResponse) > 0 {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for completed portable recording at %s", artifactPath)
}

func recordingsActivationEventCount(
	artifact *interfaces.ReplayArtifact,
	kind factoryapi.FactoryEventType,
) int {
	if artifact == nil {
		return 0
	}
	count := 0
	for _, event := range artifact.Events {
		if string(event.Type) == string(kind) {
			count++
		}
	}
	return count
}

func recordingsActivationLiveEventCount(events []factoryapi.FactoryEvent, kind factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == kind {
			count++
		}
	}
	return count
}

type recordingsActivationRecorder struct {
	submission atomic.Int32
	dispatch   atomic.Int32
}

func newRecordingsActivationRecorder() *recordingsActivationRecorder {
	return &recordingsActivationRecorder{}
}

func (recorder *recordingsActivationRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		SubmissionRecorder: recorder.recordSubmission,
		DispatchRecorder:   recorder.recordDispatch,
	}
}

func (recorder *recordingsActivationRecorder) totalCanonicalRecording() int32 {
	return recorder.submission.Load() + recorder.dispatch.Load()
}

func (recorder *recordingsActivationRecorder) recordSubmission(work.FactorySubmissionRecord) {
	recorder.submission.Add(1)
}

func (recorder *recordingsActivationRecorder) recordDispatch(recordings.FactoryDispatchRecord) {
	recorder.dispatch.Add(1)
}
