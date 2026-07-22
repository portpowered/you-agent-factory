//go:build functionallong

package replay_contracts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestReplayRegressionHarness_LoadsArtifactAndAssertsSuccessfulReplay(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay regression harness success sweep")

	artifactPath := recordReplayHarnessFixtureArtifact(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("expected replay fixture artifact to contain dispatches")
	}
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("expected replay fixture artifact to contain completions")
	}

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	session := support.GetDefaultSession(t, server.URL())
	if got := support.SessionPlaceTokenCount(session, "task:complete"); got != 1 {
		t.Fatalf("task:complete token count = %d, want 1", got)
	}
	for _, placeID := range []string{"task:init", "task:processing", "task:failed"} {
		if got := support.SessionPlaceTokenCount(session, placeID); got != 0 {
			t.Fatalf("%s token count = %d, want 0", placeID, got)
		}
	}
	server.Stop(t)
}

func TestReplayRegressionHarness_AssertsExpectedDivergence(t *testing.T) {
	support.SkipLongFunctional(t, "slow replay regression harness divergence sweep")

	artifactPath := recordReplayHarnessFixtureArtifact(t)
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest) == 0 {
		t.Fatal("expected replay fixture artifact to contain dispatches")
	}

	expectedEventID, expectedTick := mutateFirstDispatchCreatedEvent(t, artifact, func(payload *factoryapi.DispatchRequestEventPayload) {
		payload.TransitionId = "unexpected-transition"
	})
	divergentPath := filepath.Join(t.TempDir(), "divergent-replay.json")
	writeReplayArtifactFixture(t, divergentPath, artifact)

	message := assertReplayDiverges(t, divergentPath, 10*time.Second)
	for _, fragment := range []string{
		"replay divergence: category=dispatch_mismatch",
		fmt.Sprintf("tick=%d", expectedTick),
		"expected_event_id=" + expectedEventID,
	} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("replay error %q does not contain %q", message, fragment)
		}
	}
}

func assertReplayDiverges(t *testing.T, artifactPath string, timeout time.Duration) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	dir := t.TempDir()
	inputs := support.FakeInputs(ctx, []string{
		"you", "run",
		"--dir", dir,
		"--replay", artifactPath,
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.WorkingDirectory = dir
	err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input)
	if err == nil {
		t.Fatal("assertReplayDiverges: replay succeeded, expected divergence")
	}
	return err.Error()
}

func writeReplayArtifactFixture(t *testing.T, path string, artifact *interfaces.ReplayArtifact) {
	t.Helper()
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatalf("marshal divergent replay artifact: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write divergent replay artifact: %v", err)
	}
}

func mutateFirstDispatchCreatedEvent(t *testing.T, artifact *interfaces.ReplayArtifact, mutate func(*factoryapi.DispatchRequestEventPayload)) (string, int) {
	t.Helper()
	for i := range artifact.Events {
		event := testutil.GeneratedFactoryEvent(t, artifact.Events[i])
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch created event: %v", err)
		}
		mutate(&payload)
		var union factoryapi.FactoryEvent_Payload
		if err := union.FromDispatchRequestEventPayload(payload); err != nil {
			t.Fatalf("encode dispatch created event: %v", err)
		}
		event.Payload = union
		artifact.Events[i] = testutil.FactoryEvent(t, event)
		return event.Id, event.Context.Tick
	}
	t.Fatal("artifact has no DISPATCH_CREATED event")
	return "", 0
}

func recordReplayHarnessFixtureArtifact(t *testing.T) string {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "service-simple-replay.json")

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     "replay-fixture-work",
		TraceID:    "replay-fixture-trace",
		Payload:    []byte(`{"title": "replay regression harness"}`),
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Step one done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Step two done. COMPLETE"},
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Args:       []string{"--record", artifactPath},
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	server.Stop(t)

	return artifactPath
}
