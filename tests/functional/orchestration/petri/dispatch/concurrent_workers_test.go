package dispatch

import (
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPetriIndependentWorkDispatchesConcurrently proves multiple independent Work
// items submitted through the customer process dispatch and complete under
// resource-limited concurrency without deadlock or sleep-based synchronization.
// Capacity-limited (agent-slot=2) and serial (agent-slot=1) ideation pipelines
// both reach the expected public terminal Work locations and restore resource
// availability after quiescence.
func TestPetriIndependentWorkDispatchesConcurrently(t *testing.T) {
	t.Run("capacity_limited_concurrency_completes_all_work", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "batch_ideation_pipeline"))
		traceIDs := seedBatchIdeas(t, dir, 3)

		var responses []workerexecution.InferenceResponse
		for range 15 {
			responses = append(responses, workerexecution.InferenceResponse{
				Content: "Done. COMPLETE ACCEPTED",
			})
		}
		provider := testutil.NewMockProvider(responses...)

		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		assertWorkAtCustomerStates(t, listed, map[string]int{
			support.WorkCustomerLocation("story", "complete"): 3,
			support.WorkCustomerLocation("idea", "init"):      0,
			support.WorkCustomerLocation("prd", "init"):       0,
			support.WorkCustomerLocation("story", "init"):     0,
			support.WorkCustomerLocation("story", "in-review"): 0,
		})
		assertCompletedStoryTraces(t, listed, traceIDs)

		if provider.CallCount() != 9 {
			t.Errorf("expected exactly 9 provider calls, got %d", provider.CallCount())
		}

		assertResourceAvailability(t, session, "agent-slot", 2)
	})

	t.Run("serial_concurrency_limit_completes_all_work", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "serial_ideation_pipeline"))
		traceIDs := seedBatchIdeas(t, dir, 3)

		var responses []workerexecution.InferenceResponse
		for range 15 {
			responses = append(responses, workerexecution.InferenceResponse{
				Content: "Done. COMPLETE ACCEPTED",
			})
		}
		provider := testutil.NewMockProvider(responses...)

		session, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 30*time.Second)

		assertWorkAtCustomerStates(t, listed, map[string]int{
			support.WorkCustomerLocation("story", "complete"): 3,
			support.WorkCustomerLocation("idea", "init"):      0,
			support.WorkCustomerLocation("prd", "init"):       0,
			support.WorkCustomerLocation("story", "init"):     0,
			support.WorkCustomerLocation("story", "in-review"): 0,
		})
		assertCompletedStoryTraces(t, listed, traceIDs)

		if provider.CallCount() != 9 {
			t.Errorf("expected exactly 9 provider calls, got %d", provider.CallCount())
		}

		assertResourceAvailability(t, session, "agent-slot", 1)
	})
}

func seedBatchIdeas(t *testing.T, dir string, count int) []string {
	t.Helper()
	traceIDs := make([]string, count)
	for i := range count {
		traceIDs[i] = fmt.Sprintf("trace-batch-idea-%03d", i+1)
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    traceIDs[i],
			Payload:    fmt.Appendf(nil, `{"title":"batch idea %d"}`, i+1),
		})
	}
	return traceIDs
}

func assertWorkAtCustomerStates(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for location, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Errorf("%s Work count = %d, want %d", location, got, want)
		}
	}
}

func assertCompletedStoryTraces(t *testing.T, listed factoryapi.ListWorkResponse, traceIDs []string) {
	t.Helper()
	wants := make(map[string]bool, len(traceIDs))
	for _, traceID := range traceIDs {
		wants[traceID] = true
	}
	found := map[string]bool{}
	complete := support.WorkCustomerLocation("story", "complete")
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != complete {
			continue
		}
		if item.TraceId == nil || !wants[*item.TraceId] {
			t.Errorf("unexpected story:complete trace ID %#v", item.TraceId)
			continue
		}
		found[*item.TraceId] = true
	}
	for traceID := range wants {
		if !found[traceID] {
			t.Errorf("listed Work missing story:complete trace %q", traceID)
		}
	}
}

func assertResourceAvailability(t *testing.T, session factoryapi.FactorySession, name string, want int) {
	t.Helper()
	for _, resource := range session.Runtime.Usage.Resources {
		if resource.Name == name {
			if resource.Available != want || resource.Total != want {
				t.Errorf("resource %s usage = %#v, want %d available and total", name, resource, want)
			}
			return
		}
	}
	t.Errorf("session usage missing resource %q", name)
}
