package routing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const logicalMoveRouterWorkstation = "router"

// TestLogicalMoveCompletesWithoutWorkerDispatch proves that Work submitted into
// a LOGICAL_MOVE workstation advances to the authored terminal output state
// without any worker or provider dispatch attributable to that routing step.
func TestLogicalMoveCompletesWithoutWorkerDispatch(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "logical_move_dir"))
	configureLogicalMoveWorkstation(t, dir, logicalMoveRouterWorkstation)
	testutil.WriteSeedFile(t, dir, "task", []byte("my-payload"))

	provider := testutil.NewMockProvider()
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	if provider.CallCount() != 0 {
		t.Fatalf("provider call count = %d, want 0 for workerless logical move", provider.CallCount())
	}
	assertWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "done"): 1,
		support.WorkCustomerLocation("task", "init"): 0,
	})
	assertNoInferenceResponses(t, events)
	assertLogicalMoveDispatchesCompleteWithoutProviderFailure(
		t,
		support.ObserveDispatchEvents(t, events),
		logicalMoveRouterWorkstation,
	)
}

// TestLogicalMovePreservesWorkPayloadAndLineage proves that Work payload and
// observable lineage identity survive a LOGICAL_MOVE routing step so the next
// worker-backed workstation still receives the same customer content and Work ID.
func TestLogicalMovePreservesWorkPayloadAndLineage(t *testing.T) {
	const wantPayload = "preserved-payload"

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "logical_move_pipeline_dir"))
	configureLogicalMoveWorkstation(t, dir, logicalMoveRouterWorkstation)
	testutil.WriteSeedFile(t, dir, "task", []byte(wantPayload))

	provider := testutil.NewMockProvider(workerexecution.InferenceResponse{Content: "done COMPLETE"})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	assertWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "done"):    1,
		support.WorkCustomerLocation("task", "init"):    0,
		support.WorkCustomerLocation("task", "staging"): 0,
	})

	calls := provider.Calls()
	if len(calls) != 1 {
		t.Fatalf("provider call count = %d, want 1 worker dispatch after logical move", len(calls))
	}
	if got := string(support.FirstInputPayload(calls[0].Dispatch.InputTokens)); got != wantPayload {
		t.Fatalf("worker-bound payload = %q, want %q preserved across logical move", got, wantPayload)
	}
	workID := support.FirstInputWorkID(calls[0].Dispatch.InputTokens)
	if workID == "" {
		t.Fatal("worker-bound Work ID missing, want observable lineage after logical move")
	}
	if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "done")) {
		t.Fatalf(
			"listed Work %q missing at task:done, want same Work identity after logical move; listed=%#v",
			workID,
			listed,
		)
	}
}

// TestLogicalMoveMultipleOutputsCreatesEveryExpectedWork proves that a
// LOGICAL_MOVE workstation with multiple authored outputs creates Work in every
// expected downstream workType:state pair without dropping any fan-out branch.
func TestLogicalMoveMultipleOutputsCreatesEveryExpectedWork(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "logical_move_multi_output_dir"))
	configureLogicalMoveWorkstation(t, dir, logicalMoveRouterWorkstation)
	testutil.WriteSeedFile(t, dir, "task", []byte("fan-out-payload"))

	provider := testutil.NewMockProvider()
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	if provider.CallCount() != 0 {
		t.Fatalf("provider call count = %d, want 0 for workerless logical move fan-out", provider.CallCount())
	}
	assertWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "init"):      0,
		support.WorkCustomerLocation("task", "done"):      1,
		support.WorkCustomerLocation("branch-a", "done"):  1,
		support.WorkCustomerLocation("branch-b", "done"):  1,
		support.WorkCustomerLocation("branch-a", "init"):  0,
		support.WorkCustomerLocation("branch-b", "init"):  0,
	})
	assertNoInferenceResponses(t, events)
	assertLogicalMoveDispatchesCompleteWithoutProviderFailure(
		t,
		support.ObserveDispatchEvents(t, events),
		logicalMoveRouterWorkstation,
	)
}

func configureLogicalMoveWorkstation(t *testing.T, dir, workstationName string) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("decode factory config: %v", err)
	}
	for _, raw := range config["workstations"].([]any) {
		workstation := raw.(map[string]any)
		if workstation["name"] == workstationName {
			workstation["type"] = "LOGICAL_MOVE"
			workstation["worker"] = ""
		}
	}
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("encode factory config: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory config: %v", err)
	}

	workstationConfigPath := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	if err := os.WriteFile(workstationConfigPath, []byte("---\ntype: LOGICAL_MOVE\n---\n"), 0o644); err != nil {
		t.Fatalf("write logical workstation config: %v", err)
	}
}

func assertWorkCustomerStates(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	wants map[string]int,
) {
	t.Helper()
	for location, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Fatalf("%s work count = %d, want %d; listed=%#v", location, got, want, listed)
		}
	}
}

func assertNoInferenceResponses(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		t.Fatalf("factory event %q emitted INFERENCE_RESPONSE, want no provider invocation for logical move", event.Id)
	}
}

func assertLogicalMoveDispatchesCompleteWithoutProviderFailure(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	workstation string,
) {
	t.Helper()

	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != workstation {
			continue
		}
		if dispatch.Response == nil {
			continue
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf(
				"dispatch %q at logical-move workstation %q outcome = %s, want ACCEPTED workerless completion",
				dispatch.DispatchID,
				workstation,
				dispatch.Response.Outcome,
			)
		}
		if dispatch.Response.ProviderFailure != nil {
			t.Fatalf(
				"dispatch %q at logical-move workstation %q providerFailure = %#v, want no provider invocation",
				dispatch.DispatchID,
				workstation,
				dispatch.Response.ProviderFailure,
			)
		}
	}
}
