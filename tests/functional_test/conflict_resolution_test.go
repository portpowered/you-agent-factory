package functional_test

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestConflictResolution_ReviewFailResolveReReview verifies the multi-workstation
// retry pattern: review fails (merge conflict) -> resolve-conflicts -> re-review -> approve -> archived.
func TestConflictResolution_ReviewFailResolveReReview(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "conflict_resolution_dir"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))

	work := map[string][]testutil.WorkResponse{
		"swe": {
			{Content: "Task processed successfully.<COMPLETE>"},
		},
		"reviewer": {
			{Error: errors.New("failed")},
			{Content: "Task execution failed.<COMPLETE>"},
		},
		"conflict-resolver": {
			{Content: "Conflicts resolved.<COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Coding workstation always succeeds.
	h.MockWorker("swe",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	// Reviewer fails on first attempt (merge conflict), approves on second.
	h.RunUntilComplete(t, 10*time.Second)

	// Token should reach complete (not failed).
	h.Assert().
		HasTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:failed").
		HasNoTokenInPlace("code-change:resolving-conflicts").
		HasNoTokenInPlace("code-change:in-review")

	// Verify call counts: reviewer called twice, resolver called once.
	if provider.CallCount("reviewer") != 2 {
		t.Errorf("expected reviewer called 2 times, got %d", provider.CallCount("reviewer"))
	}
	if provider.CallCount("conflict-resolver") != 1 {
		t.Errorf("expected conflict-resolver called 1 time, got %d", provider.CallCount("conflict-resolver"))
	}
}

// TestConflictResolution_ResolverFails verifies that when the conflict resolver
// itself fails, the token routes to the failed state.
func TestConflictResolution_ResolverFails(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "conflict_resolution_dir"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))

	work := map[string][]testutil.WorkResponse{
		"swe": {
			{Content: "Task processed successfully.<COMPLETE>"},
		},
		"reviewer": {
			{Error: errors.New("failed")},
		},
		"conflict-resolver": {
			{Error: errors.New("failed")},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	// Token should be in failed state since resolver couldn't fix it.
	h.Assert().
		HasTokenInPlace("code-change:failed").
		HasNoTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:resolving-conflicts")
}

// TestConflictResolution_ReviewApproveFirstTry verifies the happy path where
// the reviewer approves on the first attempt -- no conflict resolution needed.
func TestConflictResolution_ReviewApproveFirstTry(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "conflict_resolution_dir"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))
	work := map[string][]testutil.WorkResponse{
		"swe": {
			{Content: "Task processed successfully.<COMPLETE>"},
		},
		"reviewer": {
			{Content: "Task execution failed.<COMPLETE>"},
		},
		"conflict-resolver": {
			{Content: "Conflicts resolved.<COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:resolving-conflicts")

	if provider.CallCount("reviewer") != 1 {
		t.Errorf("expected reviewer called 2 times, got %d", provider.CallCount("reviewer"))
	}
	if provider.CallCount("conflict-resolver") != 0 {
		t.Errorf("expected conflict-resolver called 0 time, got %d", provider.CallCount("conflict-resolver"))
	}
}
