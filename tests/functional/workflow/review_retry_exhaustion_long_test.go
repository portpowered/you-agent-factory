//go:build functionallong

package workflow

import (
	"slices"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestReviewRetryLoopBreaker_TerminatesAfterMaxRetries(t *testing.T) {
	support.SkipLongFunctional(t, "slow review-retry exhaustion sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "review_retry_exhaustion"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))
	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
		support.RejectedProviderResponse("missing tests"),
		support.AcceptedProviderResponse(),
		support.RejectedProviderResponse("still no tests"),
		support.AcceptedProviderResponse(),
		support.RejectedProviderResponse("tests still missing"),
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	session := support.GetDefaultSession(t, server.URL())

	if got := len(support.ProviderCallsForWorker(provider, "swe")); got != 3 {
		t.Errorf("expected swe called 3 times, got %d", got)
	}
	if got := len(support.ProviderCallsForWorker(provider, "reviewer")); got != 3 {
		t.Errorf("expected reviewer called 3 times, got %d", got)
	}

	assertWorkflowSessionPlaces(t, listed, map[string]int{
		"code-change:failed": 1, "code-change:init": 0, "code-change:in-review": 0, "code-change:complete": 0,
	})
	assertPublicDispatchRoute(t, server.GetFactoryEvents(t), "review-exhaustion", "code-change:failed")
	server.Stop(t)
}

func TestReviewRetryLoopBreaker_FeedbackPropagated(t *testing.T) {
	support.SkipLongFunctional(t, "slow review-retry feedback propagation sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "review_retry_exhaustion"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))
	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
		support.RejectedProviderResponse("add unit tests"),
		support.AcceptedProviderResponse(),
		support.RejectedProviderResponse("tests incomplete"),
		support.AcceptedProviderResponse(),
		support.RejectedProviderResponse("coverage too low"),
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	assertWorkflowSessionPlaces(t, listed, map[string]int{"code-change:failed": 1})

	var rejectedOutputs []string
	for _, event := range server.GetFactoryEvents(t) {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Outcome == factoryapi.WorkOutcomeRejected && payload.Output != nil {
			rejectedOutputs = append(rejectedOutputs, *payload.Output)
		}
	}
	wants := []string{"add unit tests", "tests incomplete", "coverage too low"}
	if !slices.Equal(rejectedOutputs, wants) {
		t.Fatalf("public rejected outputs = %q, want %q", rejectedOutputs, wants)
	}
	server.Stop(t)
}

func TestReviewRetryLoopBreaker_SucceedsBeforeLimit(t *testing.T) {
	support.SkipLongFunctional(t, "slow review-retry success-before-limit sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "review_retry_exhaustion"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "login"}`))
	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
		support.RejectedProviderResponse("needs work"),
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
	)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWorkStable(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)

	if got := len(support.ProviderCallsForWorker(provider, "swe")); got != 2 {
		t.Errorf("expected swe called 2 times, got %d", got)
	}
	if got := len(support.ProviderCallsForWorker(provider, "reviewer")); got != 2 {
		t.Errorf("expected reviewer called 2 times, got %d", got)
	}

	assertWorkflowSessionPlaces(t, listed, map[string]int{
		"code-change:complete": 1, "code-change:failed": 0, "code-change:init": 0,
	})
}
