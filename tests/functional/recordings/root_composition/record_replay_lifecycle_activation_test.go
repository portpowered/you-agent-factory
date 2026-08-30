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
	recordEdges := recorder.edges()
	recordEdges.ProviderCommandRunner = support.NewStaticSuccessCommandRunner("recording activation provider COMPLETE")
	var recordObservation *support.TerminalFactoryEventObservation
	recordServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		Args:                      []string{"--record", artifactPath},
		WaitForServiceModeRuntime: true,
		Edges:                     recordEdges,
		BeforeRuntimeStart: func(tb testing.TB, baseURL string) {
			recordObservation = support.OpenDefaultSessionTerminalFactoryEventObservation(tb, baseURL)
		},
	})
	t.Cleanup(func() { recordServer.Stop(t) })

	support.WaitForSessionWorkTerminalFromFactoryEvents(t, recordServer.URL(), "~default", 15*time.Second)
	waitForRecordingsActivationArtifact(t, artifactPath)

	if got := recorder.totalCanonicalRecording(); got <= 0 {
		t.Fatalf(
			"canonical submission/dispatch recording effect calls after lifecycle = %d, want > 0 via edges",
			got,
		)
	}

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertRecordingActivationArtifact(t, artifact)

	recordServer.Stop(t)
	recordObservation.Wait(15 * time.Second)

	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath, "--no-record"},
	})
	t.Cleanup(func() { replayServer.Stop(t) })

	support.WaitForSessionWorkTerminalFromFactoryEvents(t, replayServer.URL(), "~default", 15*time.Second)
	replayedWork := support.ListDefaultSessionWork(t, replayServer.URL())
	replayedEvents := replayServer.GetFactoryEvents(t)
	assertReplayedRecordingActivation(t, replayedWork, replayedEvents)
	replayServer.Stop(t)

	support.ClearSeedInputs(t, dir)
	resumeArtifactPath := filepath.Join(t.TempDir(), "fun-recordings-resume-successor.replay.json")
	resumeEdges := recorder.edges()
	resumeEdges.ProviderCommandRunner = support.NewStaticSuccessCommandRunner("resume activation provider COMPLETE")
	resumeServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		Args:                      []string{"--resume", artifactPath, "--record", resumeArtifactPath},
		WaitForServiceModeRuntime: true,
		Edges:                     resumeEdges,
	})
	t.Cleanup(func() { resumeServer.Stop(t) })

	support.WaitForSessionWorkTerminalFromFactoryEvents(t, resumeServer.URL(), "~default", 15*time.Second)
	resumedSession := support.GetDefaultSession(t, resumeServer.URL())
	resumedWork := support.ListDefaultSessionWork(t, resumeServer.URL())
	resumeEvents := resumeServer.GetFactoryEvents(t)
	assertResumedRecordingActivation(t, resumedSession, resumedWork, resumeEvents)
	resumeServer.Stop(t)
	resumedArtifact, err := os.ReadFile(resumeArtifactPath)
	if err != nil {
		t.Fatalf("read live resume successor recording: %v", err)
	}
	var resumedRecording struct {
		Events []factoryapi.FactoryEvent `json:"events"`
	}
	if err := json.Unmarshal(resumedArtifact, &resumedRecording); err != nil {
		t.Fatalf("decode live resume successor recording: %v", err)
	}
	if len(resumedRecording.Events) == 0 {
		t.Fatal("live resume successor recording has no Factory Events")
	}
}

func assertRecordingActivationArtifact(t *testing.T, artifact *interfaces.ReplayArtifact) {
	t.Helper()
	if recordingsActivationEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("recorded artifact missing dispatch request events")
	}
	if recordingsActivationEventCount(artifact, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("recorded artifact missing dispatch response events")
	}
}

func assertReplayedRecordingActivation(t *testing.T, work factoryapi.ListWorkResponse, events []factoryapi.FactoryEvent) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(work, "task:complete"); got != 1 {
		t.Fatalf("replayed work at task:complete = %d, want 1; listed=%#v", got, work.Results)
	}
	if recordingsActivationLiveEventCount(events, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("replayed session missing dispatch response events")
	}
}

func assertResumedRecordingActivation(t *testing.T, session factoryapi.FactorySession, work factoryapi.ListWorkResponse, events []factoryapi.FactoryEvent) {
	t.Helper()
	if session.Id == "" {
		t.Fatal("resumed session missing default session identity")
	}
	if got := support.CountWorkAtCustomerState(work, "task:complete"); got != 1 {
		t.Fatalf("resumed work at task:complete = %d, want recorded terminal work; listed=%#v", got, work.Results)
	}
	if recordingsActivationLiveEventCount(events, factoryapi.FactoryEventTypeRunResponse) == 0 {
		t.Fatal("resumed session missing canonical RUN_RESPONSE event")
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
		"workers": []map[string]any{{
			"name":             "model-worker",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CODEX",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "gpt-5-codex",
		}},
		"workstations": []map[string]any{{
			"name":      "process-task",
			"worker":    "model-worker",
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
