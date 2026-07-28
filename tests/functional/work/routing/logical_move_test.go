package routing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
