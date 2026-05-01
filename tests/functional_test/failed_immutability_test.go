package functional_test

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestFailedImmutability_CannotBeReDispatched verifies that a token in the
// failed state cannot be re-dispatched or moved by any transition. The petri
// net has no outgoing transitions from the failed place, so the token must
// remain there permanently — matching the dispatcher's skip-failed behavior.
func TestFailedImmutability_CannotBeReDispatched(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "broken"}`))
	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{{}},
		[]error{errors.New("build error")},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Run to completion — SWE fails, token routes to failed and stays there.
	h.RunUntilComplete(t, 10*time.Second)

	// Failed token must remain in failed — no transitions fire.
	h.Assert().
		HasTokenInPlace("code-change:failed").
		PlaceTokenCount("code-change:failed", 1).
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review").
		HasNoTokenInPlace("code-change:complete")

	// SWE called exactly once (the initial dispatch), reviewer never called.
	if got := len(providerCallsForWorker(provider, "swe")); got != 1 {
		t.Errorf("expected swe called once, got %d", got)
	}
	if got := len(providerCallsForWorker(provider, "reviewer")); got != 0 {
		t.Errorf("expected reviewer never called, got %d", got)
	}
}

// TestFailedImmutability_ReviewerFailure verifies immutability when failure
// occurs at the review stage (not just the initial coding stage).
func TestFailedImmutability_ReviewerFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "risky-change"}`))
	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{
			acceptedProviderResponse(),
			{},
		},
		[]error{
			nil,
			errors.New("critical security issue"),
		},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	// Token failed at review stage and must remain there.
	h.Assert().
		HasTokenInPlace("code-change:failed").
		PlaceTokenCount("code-change:failed", 1).
		HasNoTokenInPlace("code-change:complete")

	if got := len(providerCallsForWorker(provider, "reviewer")); got != 1 {
		t.Errorf("expected reviewer called once, got %d", got)
	}
}

// TestFailedImmutability_NoDuplicateTokens verifies that extra ticks after
// failure do not create duplicate tokens as side effects.
func TestFailedImmutability_NoDuplicateTokens(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "code_review"))
	// Submit two tokens, both fail.
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "a"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "b"}`))
	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{{}, {}},
		[]error{
			errors.New("crash"),
			errors.New("crash"),
		},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("code-change:failed", 2).
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review").
		HasNoTokenInPlace("code-change:complete")
}
