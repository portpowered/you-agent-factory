package dispatch

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/providers/inference"
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
		t.Parallel()
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
			support.WorkCustomerLocation("story", "complete"):  3,
			support.WorkCustomerLocation("idea", "init"):       0,
			support.WorkCustomerLocation("prd", "init"):        0,
			support.WorkCustomerLocation("story", "init"):      0,
			support.WorkCustomerLocation("story", "in-review"): 0,
		})
		assertCompletedStoryTraces(t, listed, traceIDs)

		if provider.CallCount() != 9 {
			t.Errorf("expected exactly 9 provider calls, got %d", provider.CallCount())
		}

		assertResourceAvailability(t, session, "agent-slot", 2)
	})

	t.Run("serial_concurrency_limit_completes_all_work", func(t *testing.T) {
		t.Parallel()
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
			support.WorkCustomerLocation("story", "complete"):  3,
			support.WorkCustomerLocation("idea", "init"):       0,
			support.WorkCustomerLocation("prd", "init"):        0,
			support.WorkCustomerLocation("story", "init"):      0,
			support.WorkCustomerLocation("story", "in-review"): 0,
		})
		assertCompletedStoryTraces(t, listed, traceIDs)

		if provider.CallCount() != 9 {
			t.Errorf("expected exactly 9 provider calls, got %d", provider.CallCount())
		}

		assertResourceAvailability(t, session, "agent-slot", 1)
	})
}

// TestPetriConcurrentResultsCorrelateToOriginalWork proves each concurrent
// completion remains attributable to its originating Work identity through
// public Work listings and Factory Event projections. Single-, two-, and
// multi-seed dispatcher flows and concurrent execution-pool isolation all
// produce one distinct successful terminal Work per seeded Trace ID without
// collapsing, swapping, or duplicating identities.
func TestPetriConcurrentResultsCorrelateToOriginalWork(t *testing.T) {
	t.Run("single_seed_correlates_to_terminal_work", func(t *testing.T) {
		t.Parallel()
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
		traceID := "trace-single-seed"
		seedIdeas(t, dir, []seedIdea{{traceID: traceID, title: "add login page"}})

		runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(3)...)
		_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderCommandRunner: runner,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("prd", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 1})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, []string{traceID})
	})

	t.Run("two_concurrent_seeds_each_reach_terminal_work", func(t *testing.T) {
		t.Parallel()
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
		traceIDs := seedIdeas(t, dir, []seedIdea{
			{traceID: "trace-alpha", title: "feature-alpha"},
			{traceID: "trace-beta", title: "feature-beta"},
		})

		runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(6)...)
		_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderCommandRunner: runner,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("prd", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 2})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, traceIDs)
		assertDistinctTerminalWorkIDs(t, listed, terminal, 2)
	})

	t.Run("multi_seed_completions_remain_distinct", func(t *testing.T) {
		t.Parallel()
		const n = 5
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))
		ideas := make([]seedIdea, n)
		for i := range n {
			ideas[i] = seedIdea{
				traceID: fmt.Sprintf("trace-multi-%03d", i+1),
				title:   fmt.Sprintf("idea-%d", i),
			}
		}
		traceIDs := seedIdeas(t, dir, ideas)

		runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(n * 3)...)
		_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
			ProviderCommandRunner: runner,
		}, 15*time.Second)

		terminal := support.WorkCustomerLocation("prd", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{terminal: n})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, traceIDs)
		assertDistinctTerminalWorkIDs(t, listed, terminal, n)
	})

	t.Run("concurrent_execution_pool_keeps_distinct_work_identities", func(t *testing.T) {
		t.Parallel()
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "batch_ideation_pipeline"))
		traceIDs := seedIdeas(t, dir, []seedIdea{
			{traceID: "trace-iso-1", title: "file-1"},
			{traceID: "trace-iso-2", title: "file-2"},
		})

		var responses []workerexecution.InferenceResponse
		for range 10 {
			responses = append(responses, workerexecution.InferenceResponse{
				Content: "Done. COMPLETE ACCEPTED",
			})
		}
		provider := testutil.NewMockProvider(responses...)

		_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
			ProviderOverride: provider,
		}, 10*time.Second)

		terminal := support.WorkCustomerLocation("story", "complete")
		assertWorkAtCustomerStates(t, listed, map[string]int{terminal: 2})
		assertTerminalWorkCorrelatesToTraceIDs(t, listed, terminal, traceIDs)
		assertDistinctTerminalWorkIDs(t, listed, terminal, 2)
		assertDispatchEventsReferenceTerminalWork(t, events, listed, terminal, traceIDs)
	})
}

// TestPetriConcurrentFailureDoesNotDuplicateDispatch proves one concurrent Work
// item can fail at the external-effect edge while siblings succeed, projecting
// the failing identity to the Factory-configured failed location without a
// second successful dispatch or completion path for that same Work.
func TestPetriConcurrentFailureDoesNotDuplicateDispatch(t *testing.T) {
	const (
		failTraceID = "trace-will-fail"
		passTraceID = "trace-will-pass"
	)
	successTerminal := support.WorkCustomerLocation("story", "complete")
	failedTerminal := support.WorkCustomerLocation("story", "failed")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "batch_ideation_pipeline"))
	seedIdeas(t, dir, []seedIdea{
		{traceID: failTraceID, title: "will-fail"},
		{traceID: passTraceID, title: "will-pass"},
	})

	provider := &traceAwareReviewInferenceProvider{
		rejectTraceID: failTraceID,
		reviewCounts:  make(map[string]int),
	}
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 15*time.Second)

	assertWorkAtCustomerStates(t, listed, map[string]int{
		successTerminal: 1,
		failedTerminal:  1,
		support.WorkCustomerLocation("idea", "init"):       0,
		support.WorkCustomerLocation("story", "init"):      0,
		support.WorkCustomerLocation("story", "in-review"): 0,
		support.WorkCustomerLocation("story", "executing"): 0,
	})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, successTerminal, []string{passTraceID})
	assertTerminalWorkCorrelatesToTraceIDs(t, listed, failedTerminal, []string{failTraceID})
	assertTraceAbsentAtCustomerState(t, listed, successTerminal, failTraceID)

	failedWorkID, ok := workIDAtCustomerState(t, listed, failedTerminal, failTraceID)
	if !ok {
		t.Fatalf("missing failed Work for trace %q at %s", failTraceID, failedTerminal)
	}
	assertNoAcceptedDispatchMovesWorkToCustomerState(t, events, failedWorkID, successTerminal)
	assertDispatchEventsReferenceTerminalWork(t, events, listed, successTerminal, []string{passTraceID})

	provider.mu.Lock()
	failReviewCalls := provider.reviewCounts[failTraceID]
	passReviewCalls := provider.reviewCounts[passTraceID]
	provider.mu.Unlock()
	if failReviewCalls != 3 {
		t.Errorf("review calls for failing trace = %d, want 3", failReviewCalls)
	}
	if passReviewCalls != 1 {
		t.Errorf("review calls for passing trace = %d, want 1", passReviewCalls)
	}
}

type traceAwareReviewInferenceProvider struct {
	rejectTraceID string
	mu            sync.Mutex
	reviewCounts  map[string]int
}

func (p *traceAwareReviewInferenceProvider) Infer(
	_ context.Context,
	req workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	traceID := req.Dispatch.Execution.TraceID
	if traceID == "" {
		traceID = req.Dispatch.CurrentChainingTraceID
	}
	if req.WorkerType == "reviewer" {
		p.mu.Lock()
		p.reviewCounts[traceID]++
		p.mu.Unlock()
		if traceID == p.rejectTraceID {
			return workerexecution.InferenceResponse{Content: "needs revision"}, nil
		}
	}
	return workerexecution.InferenceResponse{Content: "Done. COMPLETE ACCEPTED"}, nil
}

var _ workerprovider.Provider = (*traceAwareReviewInferenceProvider)(nil)

type seedIdea struct {
	traceID string
	title   string
}

func seedIdeas(t *testing.T, dir string, ideas []seedIdea) []string {
	t.Helper()
	traceIDs := make([]string, len(ideas))
	for i, idea := range ideas {
		traceIDs[i] = idea.traceID
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkTypeID: "idea",
			TraceID:    idea.traceID,
			Payload:    fmt.Appendf(nil, `{"title":%q}`, idea.title),
		})
	}
	return traceIDs
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
	assertTerminalWorkCorrelatesToTraceIDs(
		t,
		listed,
		support.WorkCustomerLocation("story", "complete"),
		traceIDs,
	)
}

func assertTerminalWorkCorrelatesToTraceIDs(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	terminalLocation string,
	traceIDs []string,
) {
	t.Helper()
	wants := make(map[string]bool, len(traceIDs))
	for _, traceID := range traceIDs {
		wants[traceID] = true
	}
	found := map[string]bool{}
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != terminalLocation {
			continue
		}
		if item.TraceId == nil || !wants[*item.TraceId] {
			t.Errorf("unexpected %s trace ID %#v", terminalLocation, item.TraceId)
			continue
		}
		if item.WorkId == nil || *item.WorkId == "" {
			t.Errorf("terminal Work at %s missing workId for trace %q", terminalLocation, *item.TraceId)
			continue
		}
		if found[*item.TraceId] {
			t.Errorf("duplicate terminal Work for trace %q at %s", *item.TraceId, terminalLocation)
			continue
		}
		found[*item.TraceId] = true
	}
	for traceID := range wants {
		if !found[traceID] {
			t.Errorf("listed Work missing %s trace %q", terminalLocation, traceID)
		}
	}
}

func assertDistinctTerminalWorkIDs(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	terminalLocation string,
	want int,
) {
	t.Helper()
	ids := map[string]bool{}
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != terminalLocation {
			continue
		}
		if item.WorkId == nil || *item.WorkId == "" {
			t.Errorf("terminal Work at %s missing workId", terminalLocation)
			continue
		}
		if ids[*item.WorkId] {
			t.Errorf("duplicate terminal workId %q at %s", *item.WorkId, terminalLocation)
		}
		ids[*item.WorkId] = true
	}
	if got := len(ids); got != want {
		t.Errorf("distinct terminal work IDs at %s = %d, want %d", terminalLocation, got, want)
	}
}

func assertDispatchEventsReferenceTerminalWork(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	listed factoryapi.ListWorkResponse,
	terminalLocation string,
	traceIDs []string,
) {
	t.Helper()
	terminalWorkIDs := map[string]string{}
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != terminalLocation {
			continue
		}
		if item.TraceId == nil || item.WorkId == nil {
			continue
		}
		terminalWorkIDs[*item.TraceId] = *item.WorkId
	}
	for _, traceID := range traceIDs {
		workID, ok := terminalWorkIDs[traceID]
		if !ok {
			t.Errorf("missing terminal Work ID for trace %q", traceID)
			continue
		}
		referenced := false
		for _, dispatch := range support.ObserveDispatchEvents(t, events) {
			if support.DispatchObservationIncludesWork(dispatch, workID) {
				referenced = true
				break
			}
		}
		if !referenced {
			t.Errorf("dispatch events missing public Work ID %q for trace %q", workID, traceID)
		}
	}
}

func assertTraceAbsentAtCustomerState(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	location string,
	traceID string,
) {
	t.Helper()
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != location {
			continue
		}
		if item.TraceId != nil && *item.TraceId == traceID {
			t.Errorf("trace %q should not be at %s", traceID, location)
		}
	}
}

func workIDAtCustomerState(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	location string,
	traceID string,
) (string, bool) {
	t.Helper()
	for _, item := range listed.Results {
		if support.WorkItemCustomerLocation(item) != location {
			continue
		}
		if item.TraceId == nil || *item.TraceId != traceID {
			continue
		}
		if item.WorkId == nil || *item.WorkId == "" {
			t.Fatalf("work at %s for trace %q missing workId", location, traceID)
		}
		return *item.WorkId, true
	}
	return "", false
}

func assertNoAcceptedDispatchMovesWorkToCustomerState(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	workID string,
	terminalLocation string,
) {
	t.Helper()
	for _, dispatch := range support.ObserveDispatchEvents(t, events) {
		if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			continue
		}
		if !support.DispatchObservationIncludesWork(dispatch, workID) {
			continue
		}
		if dispatch.Response.OutputWork == nil {
			continue
		}
		for _, item := range *dispatch.Response.OutputWork {
			if support.WorkItemCustomerLocation(item) == terminalLocation {
				t.Errorf(
					"accepted dispatch %s moved work %s to success terminal %s",
					dispatch.DispatchID,
					workID,
					terminalLocation,
				)
			}
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
