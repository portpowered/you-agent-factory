//go:build functionallong

package replay_contracts

import (
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

func TestReplayRuntimeConfigSmoke_CanonicalWorkstationsDriveDispatchAndReplay(t *testing.T) {
	support.SkipLongFunctional(t, "slow canonical runtime-config replay sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "canonical-workstations.replay.json")
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     "canonical-workstation-work",
		TraceID:    "canonical-workstation-trace",
		Payload:    []byte(`{"title":"canonical workstation smoke"}`),
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Step one done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Step two done. COMPLETE"},
	)
	recordServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Args:       []string{"--record", artifactPath},
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.WaitForTerminalStatus(t, recordServer.URL(), 10*time.Second)
	assertReplaySessionPlaces(t, support.ListDefaultSessionWork(t, recordServer.URL()), map[string]int{
		"task:complete": 1, "task:init": 0, "task:processing": 0, "task:failed": 0,
	})
	assertRecordedDispatchHistory(t, recordServer.GetFactoryEvents(t))
	assertProviderSawCanonicalWorkstationPrompt(t, provider)
	recordServer.Stop(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertCanonicalReplayWorkstationMap(t, artifactPath, artifact)

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove original fixture: %v", err)
	}
	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(),
		Args:       []string{"--replay", artifactPath},
	})
	support.WaitForTerminalStatus(t, replayServer.URL(), 10*time.Second)
	assertReplaySessionPlaces(t, support.ListDefaultSessionWork(t, replayServer.URL()), map[string]int{
		"task:complete": 1, "task:init": 0, "task:processing": 0, "task:failed": 0,
	})
	replayServer.Stop(t)
}

func assertRecordedDispatchHistory(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	if got := countReplayEvents(events, factoryapi.FactoryEventTypeDispatchRequest); got != 2 {
		t.Fatalf("DISPATCH_CREATED events = %d, want 2", got)
	}
	if got := countReplayEvents(events, factoryapi.FactoryEventTypeDispatchResponse); got != 2 {
		t.Fatalf("DISPATCH_COMPLETED events = %d, want 2", got)
	}
}

func assertProviderSawCanonicalWorkstationPrompt(t *testing.T, provider *testutil.MockProvider) {
	t.Helper()

	calls := provider.Calls()
	if len(calls) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(calls))
	}
	for i, call := range calls {
		if !strings.Contains(call.UserMessage, "Do the work.") {
			t.Fatalf("provider call %d user message = %q, want canonical workstation prompt", i, call.UserMessage)
		}
	}
}

func assertCanonicalReplayWorkstationMap(t *testing.T, artifactPath string, artifact *interfaces.ReplayArtifact) {
	t.Helper()

	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read replay artifact: %v", err)
	}
	if strings.Contains(string(data), "workstation_configs") {
		t.Fatalf("replay artifact contains legacy workstation_configs map")
	}
	generatedFactory := requireFactoryOnlyRunStartedPayload(t, testutil.GeneratedFactoryEvents(t, artifact.Events)).Factory
	if generatedFactory.Workstations == nil || len(*generatedFactory.Workstations) != 2 {
		t.Fatalf("factory workstations = %#v, want 2", generatedFactory.Workstations)
	}
	found := false
	for _, workstation := range *generatedFactory.Workstations {
		if workstation.Name == "step-one" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("factory payload missing step-one workstation: %#v", generatedFactory.Workstations)
	}
}

func countReplayEvents(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}
