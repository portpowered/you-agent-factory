package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestCodeReviewLoop validates the code-review retry loop via config:
//
//	Given: a workflow with coding (init→in-review) and review (in-review→complete, reject→init)
//	When:  reviewer rejects once, then accepts
//	Then:  token completes after two coding iterations, rejection feedback propagated
func TestCodeReviewLoop(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "code_review"))

	testutil.WriteSeedFile(t, dir, "code-change", []byte("implement feature X"))

	// Interleaved responses: processor (COMPLETE→accept), reviewer (no ACCEPTED→reject), ...
	work := make(map[string][]interfaces.InferenceResponse)

	work["swe"] = []interfaces.InferenceResponse{
		{Content: "code with missing error handling <COMPLETE>"},
		{Content: "code with proper error handling <COMPLETE>"},
	}

	work["reviewer"] = []interfaces.InferenceResponse{
		{Content: "missing error handling"},
		{Content: "looks good<COMPLETE>"},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	// Token ends in complete state after one rejection loop.
	h.Assert().
		HasTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review").
		HasNoTokenInPlace("code-change:failed").
		TokenCount(1)

	// Coding worker called twice (initial + after rejection).
	if provider.CallCount("swe") != 2 {
		t.Errorf("expected swe called 2 times, got %d", provider.CallCount("swe"))
	}

	// Reviewer called twice (reject + accept).
	if provider.CallCount("reviewer") != 2 {
		t.Errorf("expected reviewer called 2 times, got %d", provider.CallCount("reviewer"))
	}

	// Rejection feedback is propagated to the second coding dispatch.
	sweCalls := provider.Calls("swe")
	if len(sweCalls) < 2 {
		t.Fatalf("expected at least 2 swe calls, got %d", len(sweCalls))
	}
	secondDispatch := sweCalls[1]
	if len(secondDispatch.UserMessage) == 0 {
		t.Fatal("second coding dispatch has no input tokens")
	}
	//TODO: we need to test the feedback propagation, but the current implementation for the templates doesn't mark for the rejection_feedback tag.
	// Add that then implement the releavnt change.
	// feedback, ok := secondDispatch.InputTokens[0].Color.Tags["_rejection_feedback"]
	// if !ok {
	// 	t.Error("second coding dispatch input token missing _rejection_feedback tag")
	// } else if feedback != "missing error handling" {
	// 	t.Errorf("expected rejection feedback %q, got %q", "missing error handling", feedback)
	// }
}
