package replay_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events"
	replay "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

var javascriptRecordingSnapshotDecoder factorydefinitionswire.FactorySnapshotJSONDecoder = func(
	data []byte,
) (*interfaces.FactorySnapshot, error) {
	generated, err := factorymapping.GeneratedFactoryFromOpenAPIJSON(data)
	if err != nil {
		return nil, err
	}
	return interfaces.NewFactorySnapshot(generated)
}

func TestJavaScriptRecordingContract_RoundTripsCompletedAndFailedSessionFacts(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name         string
		factoryState interfaces.FactoryState
		failure      string
		wantStatus   factoryapi.FactorySessionDurableLifecycleStatus
	}{
		{name: "completed", factoryState: interfaces.FactoryStateCompleted, wantStatus: factoryapi.FactorySessionDurableLifecycleStatusSucceeded},
		{name: "failed", factoryState: interfaces.FactoryStateFailed, failure: "child execution failed", wantStatus: factoryapi.FactorySessionDurableLifecycleStatusFailed},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recordedAt := time.Date(2026, time.July, 12, 16, 0, 0, 0, time.UTC)
			config := javascriptRecordingFactoryConfig()
			history := factoryevents.NewFactoryEventHistory(nil, func() time.Time { return recordedAt }, "test-stream-generation")
			history.RecordSessionLifecycleFromFactoryConfig("session-recorded-js", config, 0, recordedAt)
			if testCase.factoryState == interfaces.FactoryStateCompleted {
				phaseStartedAt := recordedAt.Add(250 * time.Millisecond)
				phaseCompletedAt := recordedAt.Add(750 * time.Millisecond)
				history.RecordOrchestratorPhaseChanged(factoryevents.OrchestratorPhaseChangedInput{
					SessionID:           "session-recorded-js",
					OrchestratorKind:    string(factoryapi.JAVASCRIPT),
					OrchestratorDialect: "workflow-v1",
					PhaseID:             "phase-plan",
					PhaseName:           "plan",
					Source:              "runtime",
					Tick:                1,
					PhaseStatus:         interfaces.OrchestratorPhaseStatusCompleted,
					StartedAt:           &phaseStartedAt,
					CompletedAt:         &phaseCompletedAt,
					ProgressSummary:     "plan completed",
				}, phaseCompletedAt)
				history.RecordOrchestratorCheckpointWritten(factoryevents.OrchestratorCheckpointWrittenInput{
					SessionID:             "session-recorded-js",
					OrchestratorKind:      interfaces.OrchestratorKindJavaScript,
					OrchestratorDialect:   "workflow-v1",
					CheckpointID:          "checkpoint-1",
					Source:                "runtime",
					Tick:                  1,
					Label:                 "after-plan",
					SourceHash:            "sha256:source",
					RuntimeSnapshotDigest: "sha256:checkpoint-state",
					ArtifactRef: &interfaces.FactoryArtifactRef{
						ID:         "artifact-checkpoint-1",
						Kind:       interfaces.JavaScriptCheckpointArtifactKind,
						Visibility: interfaces.JavaScriptCheckpointArtifactVisibility,
					},
					ResumabilityStatus: interfaces.CheckpointResumabilityStatusResumable,
				}, recordedAt.Add(time.Second))
			}
			history.RecordSessionLifecycleCompletion("session-recorded-js", config, 2, testCase.factoryState, testCase.failure, recordedAt.Add(2*time.Second))

			factorySnapshot, err := interfaces.NewFactorySnapshot(javascriptRecordingFactory())
			if err != nil {
				t.Fatalf("NewFactorySnapshot: %v", err)
			}
			artifact, err := replay.NewEventLogArtifact(recordedAt, factorySnapshot, nil, interfaces.ReplayDiagnostics{})
			if err != nil {
				t.Fatalf("NewEventLogArtifact: %v", err)
			}
			artifact.Events = append(artifact.Events, history.CanonicalEvents()...)
			path := filepath.Join(t.TempDir(), testCase.name+".replay.json")
			if err := replay.Save(testReplayStorage(), path, artifact); err != nil {
				t.Fatalf("Save: %v", err)
			}
			loaded, err := replay.Load(
				testReplayStorage(),
				path,
				javascriptRecordingSnapshotDecoder,
			)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			assertRecordedJavaScriptLifecycle(t, testutil.GeneratedFactoryEvents(t, loaded.Events), testCase.wantStatus, testCase.failure)
			assertRecordingOmitsRawJavaScriptInternals(t, loaded)
			publicFactory, err := factorysnapshot.ToAPI(loaded.Factory)
			if err != nil {
				t.Fatalf("map replay Factory snapshot: %v", err)
			}
			if publicFactory == nil || publicFactory.Name != "recorded-javascript-factory" {
				t.Fatalf("mapped replay Factory = %#v, want recorded-javascript-factory", publicFactory)
			}
		})
	}
}

func TestReplayFactorySnapshotMappingPreservesAbsentOptionalFactory(t *testing.T) {
	t.Parallel()
	publicFactory, err := factorysnapshot.ToAPI(nil)
	if err != nil || publicFactory != nil {
		t.Fatalf("ToAPI(nil) = (%#v, %v), want (nil, nil)", publicFactory, err)
	}
}

func javascriptRecordingFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Name: "recorded-javascript-factory",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				Dialect:       "workflow-v1",
				SourceRef:     "workflows/main.js",
				SourceHash:    "sha256:source",
				ArgsSchema:    json.RawMessage(`{"type":"object"}`),
				DefaultPolicy: json.RawMessage(`{"maxAgents":4}`),
			},
		},
	}
}

func javascriptRecordingFactory() factoryapi.Factory {
	kind := factoryapi.JAVASCRIPT
	dialect := "workflow-v1"
	sourceRef := "workflows/main.js"
	return factoryapi.Factory{
		Name:      "recorded-javascript-factory",
		WorkTypes: &[]factoryapi.WorkType{{Name: "task"}},
		Workers:   &[]factoryapi.Worker{{Name: "worker-a"}},
		Workstations: &[]factoryapi.Workstation{{
			Name: "process", Worker: "worker-a", Inputs: []factoryapi.WorkstationIO{}, Outputs: &[]factoryapi.WorkstationIO{},
		}},
		Orchestrator: &factoryapi.FactoryOrchestrator{
			Kind: kind,
			Javascript: &factoryapi.FactoryOrchestratorJavaScriptConfig{
				Dialect:    &dialect,
				SourceRef:  &sourceRef,
				SourceHash: stringPointer("sha256:source"),
			},
		},
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func assertRecordedJavaScriptLifecycle(t *testing.T, recorded []factoryapi.FactoryEvent, wantStatus factoryapi.FactorySessionDurableLifecycleStatus, wantFailure string) {
	t.Helper()
	var started *factoryapi.SessionStartedEventPayload
	var completed *factoryapi.SessionCompletedEventPayload
	var phaseChanged *factoryapi.OrchestratorPhaseChangedEventPayload
	for _, event := range recorded {
		switch event.Type {
		case factoryapi.FactoryEventTypeSessionStarted:
			payload, err := event.Payload.AsSessionStartedEventPayload()
			if err != nil {
				t.Fatalf("decode session started: %v", err)
			}
			started = &payload
		case factoryapi.FactoryEventTypeSessionCompleted:
			payload, err := event.Payload.AsSessionCompletedEventPayload()
			if err != nil {
				t.Fatalf("decode session completed: %v", err)
			}
			completed = &payload
		case factoryapi.FactoryEventTypeOrchestratorPhaseChanged:
			payload, err := event.Payload.AsOrchestratorPhaseChangedEventPayload()
			if err != nil {
				t.Fatalf("decode orchestrator phase changed: %v", err)
			}
			phaseChanged = &payload
		}
	}
	if started == nil || started.SourceRef == nil || *started.SourceRef != "workflows/main.js" || started.SourceHash == nil || *started.SourceHash != "sha256:source" {
		t.Fatalf("session start identity = %#v, want source ref and digest", started)
	}
	if started.PolicyHash == nil || *started.PolicyHash == "" || started.ArgsDigest == nil || *started.ArgsDigest == "" {
		t.Fatalf("session start digests = %#v, want policy and argument digests", started)
	}
	if completed == nil || completed.FinalStatus != wantStatus || completed.ResultStatus == nil {
		t.Fatalf("session completion = %#v, want status %s and result availability", completed, wantStatus)
	}
	if wantFailure != "" && (completed.FailureDetail == nil || completed.FailureDetail.Message != wantFailure) {
		t.Fatalf("failure detail = %#v, want %q", completed.FailureDetail, wantFailure)
	}
	if wantStatus == factoryapi.FactorySessionDurableLifecycleStatusSucceeded &&
		(phaseChanged == nil || phaseChanged.PhaseStatus != factoryapi.COMPLETED ||
			phaseChanged.ProgressSummary == nil || *phaseChanged.ProgressSummary != "plan completed") {
		t.Fatalf("orchestrator phase change = %#v, want completed plan progress", phaseChanged)
	}
}

func assertRecordingOmitsRawJavaScriptInternals(t *testing.T, artifact *interfaces.ReplayArtifact) {
	t.Helper()
	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal loaded artifact: %v", err)
	}
	for _, forbidden := range []string{"vmState", "checkpointState", "childDispatches", "providerTranscript"} {
		if strings.Contains(string(data), `"`+forbidden+`"`) {
			t.Fatalf("recording contains unsupported raw field %q", forbidden)
		}
	}
}

func stringPointer(value string) *string {
	return &value
}
