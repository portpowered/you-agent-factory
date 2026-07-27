//go:build functionallong

package execution

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	statelessCollectorStage1Workstation = "step1"
	statelessCollectorStage2Workstation = "step2"
)

// TestExecutionWorkstationDispatchesEligibleWorkOnce proves that eligible
// execution workstations dispatch each submitted Work item exactly once per
// eligibility cycle through the public process boundary, including multi-item
// and staged two-step completion from the stateless collector fixture.
func TestExecutionWorkstationDispatchesEligibleWorkOnce(t *testing.T) {
	support.SkipLongFunctional(t, "slow execution-workstation multi-item staged sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "stateless_collector"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item": "w1"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item": "w2"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item": "w3"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Stage 1 done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Stage 2 done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Stage 1 done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Stage 2 done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Stage 1 done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Stage 2 done. COMPLETE"},
	)

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		30*time.Second,
	)
	dispatches := support.ObserveDispatchEvents(t, events)

	if provider.CallCount() != 6 {
		t.Fatalf("provider call count = %d, want 6 for three two-stage items", provider.CallCount())
	}
	if session.Runtime.Progress.Categories.Terminal != 3 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress categories = %+v, want three terminal and zero failed", session.Runtime.Progress.Categories)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "done")); got != 3 {
		t.Fatalf("CountWorkAtCustomerState(task:done) = %d, want 3; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0 after completion", got)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "stage1")); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:stage1) = %d, want 0 after completion", got)
	}

	workIDs := terminalTaskWorkIDs(t, listed)
	if len(workIDs) != 3 {
		t.Fatalf("terminal task work IDs = %v, want three completed items", workIDs)
	}
	for _, workID := range workIDs {
		assertExactlyOneCompletedDispatchPerWorkAtWorkstation(
			t,
			dispatches,
			workID,
			statelessCollectorStage1Workstation,
		)
		assertExactlyOneCompletedDispatchPerWorkAtWorkstation(
			t,
			dispatches,
			workID,
			statelessCollectorStage2Workstation,
		)
	}
}

func terminalTaskWorkIDs(t *testing.T, listed factoryapi.ListWorkResponse) []string {
	t.Helper()

	ids := make([]string, 0, len(listed.Results))
	for _, item := range listed.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != "task" {
			continue
		}
		if item.State == nil || item.State.Name != "done" || item.State.Type != factoryapi.WorkStateTypeTERMINAL {
			continue
		}
		workID := support.StringPointerValue(item.WorkId)
		if workID == "" {
			t.Fatalf("terminal task work has empty work ID: %#v", item)
		}
		ids = append(ids, workID)
	}
	return ids
}

func assertExactlyOneCompletedDispatchPerWorkAtWorkstation(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	workID, workstation string,
) {
	t.Helper()

	count := 0
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != workstation {
			continue
		}
		if !support.DispatchObservationIncludesWork(dispatch, workID) {
			continue
		}
		if dispatch.Response == nil {
			t.Fatalf(
				"dispatch %q for work %q at %q missing public DISPATCH_RESPONSE",
				dispatch.DispatchID,
				workID,
				workstation,
			)
		}
		count++
	}
	if count != 1 {
		t.Fatalf(
			"completed dispatch count for work %q at %q = %d, want 1",
			workID,
			workstation,
			count,
		)
	}
}
