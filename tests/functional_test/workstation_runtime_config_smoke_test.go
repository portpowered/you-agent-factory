package functional_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestRuntimeReplayIntegrationSmoke_CanonicalWorkstationsDriveDispatchAndReplay(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "service_simple"))
	artifactPath := filepath.Join(t.TempDir(), "canonical-workstations.replay.json")
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     "canonical-workstation-work",
		TraceID:    "canonical-workstation-trace",
		Payload:    []byte(`{"title":"canonical workstation smoke"}`),
	})

	loaded, err := config.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	assertCanonicalSmokeWorkstation(t, loaded.Workstation, "step-one", "worker-a")

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
	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:failed")
	assertRecordedDispatchHistory(t, h)
	assertProviderSawCanonicalWorkstationPrompt(t, provider)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	assertCanonicalReplayWorkstationMap(t, artifactPath, artifact)
	replayRuntime, err := replay.RuntimeConfigFromGeneratedFactory(artifact.Factory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	assertCanonicalSmokeWorkstation(t, replayRuntime.Workstation, "step-one", "worker-a")

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove original fixture: %v", err)
	}
	replayHarness := testutil.AssertReplaySucceeds(t, artifactPath, 10*time.Second)
	replayHarness.Service.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:failed")
}

func assertCanonicalSmokeWorkstation(
	t *testing.T,
	lookup func(string) (*interfaces.FactoryWorkstationConfig, bool),
	name string,
	workerName string,
) {
	t.Helper()

	workstation, ok := lookup(name)
	if !ok {
		t.Fatalf("expected workstation %q", name)
	}
	if workstation.WorkerTypeName != workerName {
		t.Fatalf("%s worker = %q, want %q", name, workstation.WorkerTypeName, workerName)
	}
	if workstation.Type != interfaces.WorkstationTypeModel {
		t.Fatalf("%s runtime type = %q, want %q", name, workstation.Type, interfaces.WorkstationTypeModel)
	}
	if !strings.Contains(workstation.PromptTemplate, "Do the work.") {
		t.Fatalf("%s prompt template = %q, want split workstation prompt", name, workstation.PromptTemplate)
	}
}

func assertRecordedDispatchHistory(t *testing.T, h *testutil.ServiceTestHarness) {
	t.Helper()

	events, err := h.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	if got := countFactoryEvents(events, factoryapi.FactoryEventTypeDispatchRequest); got != 2 {
		t.Fatalf("DISPATCH_CREATED events = %d, want 2", got)
	}
	if got := countFactoryEvents(events, factoryapi.FactoryEventTypeDispatchResponse); got != 2 {
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
	if artifact.Factory.Workstations == nil || len(*artifact.Factory.Workstations) != 2 {
		t.Fatalf("factory workstations = %#v, want 2", artifact.Factory.Workstations)
	}
	found := false
	for _, workstation := range *artifact.Factory.Workstations {
		if workstation.Name == "step-one" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("factory payload missing step-one workstation: %#v", artifact.Factory.Workstations)
	}
}

func countFactoryEvents(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}
