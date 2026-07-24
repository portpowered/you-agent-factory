//go:build functionallong

package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestDispatcherWorkflow_SingleSeedFile(t *testing.T) {
	support.SkipLongFunctional(t, "slow dispatcher single-seed sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))

	originTraceID := "trace-single-seed"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "add login page"}`),
		TraceID:    originTraceID,
	})

	runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(3)...)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{
		"prd:complete": 1, "idea:init": 0, "idea:failed": 0,
		"prd:init": 0, "prd:in-review": 0, "prd:failed": 0,
	})

	if got := runner.CallCount(); got != 3 {
		t.Errorf("expected three externally executed workflow steps, got %d", got)
	}

	assertWorkflowSessionPlaceHasTraceID(t, session, "prd:complete", originTraceID)
}

func TestDispatcherWorkflow_TwoSeedFiles(t *testing.T) {
	support.SkipLongFunctional(t, "slow dispatcher two-seed sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "feature-alpha"}`),
		TraceID:    "trace-alpha",
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "feature-beta"}`),
		TraceID:    "trace-beta",
	})

	runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(6)...)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"prd:complete": 2})

	if got := runner.CallCount(); got != 6 {
		t.Errorf("expected six externally executed workflow steps, got %d", got)
	}
}

func TestDispatcherWorkflow_MultipleSeedFiles(t *testing.T) {
	support.SkipLongFunctional(t, "slow dispatcher multi-seed sweep")
	const n = 5
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))

	for i := range n {
		testutil.WriteSeedFile(t, dir, "idea", fmt.Appendf(nil, `{"title": "idea-%d"}`, i))
	}

	runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(n * 3)...)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 15*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"prd:complete": n})

	if got := runner.CallCount(); got != n*3 {
		t.Errorf("expected %d externally executed workflow steps, got %d", n*3, got)
	}
}

func TestDispatcherWorkflow_ExecutionPoolIsolation(t *testing.T) {
	support.SkipLongFunctional(t, "slow dispatcher pool-isolation sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "file-1"}`),
		TraceID:    "trace-iso-1",
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "file-2"}`),
		TraceID:    "trace-iso-2",
	})

	runner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(6)...)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)

	if got := distinctPublicTokenIDsAtPlace(session, "prd:complete"); got != 2 {
		t.Errorf("expected 2 distinct public terminal token IDs, got %d", got)
	}

	assertWorkflowSessionPlaces(t, listed, map[string]int{"prd:complete": 2})
}

func TestDispatcherWorkflow_ReviewFailurePerItem(t *testing.T) {
	support.SkipLongFunctional(t, "slow dispatcher per-item review failure sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_workflow"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "will-fail"}`),
		TraceID:    "trace-will-fail",
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "will-pass"}`),
		TraceID:    "trace-will-pass",
	})

	runner := &traceAwareReviewCommandRunner{
		rejectTraceID: "trace-will-fail",
		callCounts:    make(map[string]int),
	}
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 15*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{"prd:complete": 1, "prd:failed": 1})

	runner.mu.Lock()
	failCount := runner.callCounts["trace-will-fail"]
	passCount := runner.callCounts["trace-will-pass"]
	runner.mu.Unlock()

	if failCount != 3 {
		t.Errorf("expected reviewer called 3 times for failing item, got %d", failCount)
	}
	if passCount != 1 {
		t.Errorf("expected reviewer called 1 time for passing item, got %d", passCount)
	}
}

type traceAwareReviewCommandRunner struct {
	rejectTraceID string
	mu            sync.Mutex
	callCounts    map[string]int
}

func (r *traceAwareReviewCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	invocation := strings.Join(append(append([]string(nil), req.Args...), string(req.Stdin)), "\n")
	if !strings.Contains(invocation, "Review workstation") {
		return platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")}, nil
	}

	traceID := "trace-will-pass"
	if strings.Contains(invocation, "will-fail") {
		traceID = "trace-will-fail"
	}

	r.mu.Lock()
	r.callCounts[traceID]++
	r.mu.Unlock()

	if traceID == r.rejectTraceID {
		return platformprocess.CommandResult{Stdout: []byte("needs revision")}, nil
	}

	return platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")}, nil
}

var _ platformprocess.CommandRunner = (*traceAwareReviewCommandRunner)(nil)

func distinctPublicTokenIDsAtPlace(session factoryapi.FactorySession, placeID string) int {
	ids := make(map[string]struct{})
	if session.Runtime.Petri != nil {
		for _, token := range session.Runtime.Petri.Marking {
			if token.PlaceId == placeID && token.Id != "" {
				ids[token.Id] = struct{}{}
			}
		}
	}
	return len(ids)
}

func assertWorkflowSessionPlaceHasTraceID(
	t *testing.T,
	session factoryapi.FactorySession,
	placeID string,
	traceID string,
) {
	t.Helper()
	if session.Runtime.Petri != nil {
		for _, token := range session.Runtime.Petri.Marking {
			if token.PlaceId == placeID && token.TraceId == traceID {
				return
			}
		}
	}
	t.Fatalf("place %s has no token with trace ID %q", placeID, traceID)
}
