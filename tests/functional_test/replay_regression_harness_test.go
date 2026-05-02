package functional_test

import (
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestReplayRegressionHarness_LoadsArtifactAndAssertsSuccessfulReplay(t *testing.T) {
	artifactPath := recordReplayHarnessFixtureArtifact(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("expected replay fixture artifact to contain dispatches")
	}
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("expected replay fixture artifact to contain completions")
	}

	h := testutil.AssertReplaySucceeds(t, artifactPath, 10*time.Second)
	h.Service.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:failed")
}

func TestReplayRegressionHarness_AssertsExpectedDivergence(t *testing.T) {
	artifactPath := recordReplayHarnessFixtureArtifact(t)
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("expected replay fixture artifact to contain dispatches")
	}

	expectedEventID, expectedTick := mutateFirstDispatchCreatedEvent(t, artifact, func(payload *factoryapi.DispatchRequestEventPayload) {
		payload.TransitionId = "unexpected-transition"
	})
	divergentPath := filepath.Join(t.TempDir(), "divergent-replay.json")
	if err := replay.Save(divergentPath, artifact); err != nil {
		t.Fatalf("save divergent replay artifact: %v", err)
	}

	report := testutil.AssertReplayDiverges(t, divergentPath, 10*time.Second)
	if report.Category != replay.DivergenceCategoryDispatchMismatch {
		t.Fatalf("divergence category = %q, want %q", report.Category, replay.DivergenceCategoryDispatchMismatch)
	}
	if report.ExpectedEventID != expectedEventID {
		t.Fatalf("expected event id = %q, want %q", report.ExpectedEventID, expectedEventID)
	}
	if report.Tick != expectedTick {
		t.Fatalf("divergence tick = %d, want %d", report.Tick, expectedTick)
	}
}

func mutateFirstDispatchCreatedEvent(t *testing.T, artifact *interfaces.ReplayArtifact, mutate func(*factoryapi.DispatchRequestEventPayload)) (string, int) {
	t.Helper()
	for i := range artifact.Events {
		if artifact.Events[i].Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := artifact.Events[i].Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch created event: %v", err)
		}
		mutate(&payload)
		var union factoryapi.FactoryEvent_Payload
		if err := union.FromDispatchRequestEventPayload(payload); err != nil {
			t.Fatalf("encode dispatch created event: %v", err)
		}
		artifact.Events[i].Payload = union
		return artifact.Events[i].Id, artifact.Events[i].Context.Tick
	}
	t.Fatal("artifact has no DISPATCH_CREATED event")
	return "", 0
}

func recordReplayHarnessFixtureArtifact(t *testing.T) string {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "service-simple-replay.json")

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     "replay-fixture-work",
		TraceID:    "replay-fixture-trace",
		Payload:    []byte(`{"title": "replay regression harness"}`),
	})

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Step one done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Step two done. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRecordPath(artifactPath),
	)
	h.RunUntilComplete(t, 10*time.Second)

	return artifactPath
}
