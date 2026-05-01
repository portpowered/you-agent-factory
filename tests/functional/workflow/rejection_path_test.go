package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestRejectionPath_NoRejectionArcsFailsToken(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_no_arcs"))

	testutil.WriteSeedFile(t, dir, "task", []byte("work payload"))

	provider := testutil.NewMockProvider(support.RejectedProviderResponse("not good enough"))
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:done")
}

func TestRejectionPath_NoRejectionArcsReleasesResources(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_no_arcs_resources"))

	testutil.WriteSeedFile(t, dir, "task", []byte("first item"))
	testutil.WriteSeedFile(t, dir, "task", []byte("second item"))

	provider := testutil.NewMockProvider(
		support.RejectedProviderResponse("not good enough"),
		support.AcceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init")
}

func TestRejectionPath_WithRejectionArcsRoutesViaArcs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_with_arcs"))

	testutil.WriteSeedFile(t, dir, "task", []byte("work"))

	provider := testutil.NewMockProvider(
		support.RejectedProviderResponse("needs work"),
		support.AcceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

func TestRejectionPath_NoRejectionArcsFailureRecordSet(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_no_arcs"))

	testutil.WriteSeedFile(t, dir, "task", []byte("work"))

	provider := testutil.NewMockProvider(support.RejectedProviderResponse("missing tests"))
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 5*time.Second)

	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID != "task:failed" {
			continue
		}
		if len(tok.History.FailureLog) == 0 {
			t.Error("expected FailureLog to be populated on token failed via rejection fallback")
		}
		if tok.History.TotalVisits["process"] == 0 {
			t.Error("expected TotalVisits[process] > 0")
		}
		return
	}
	t.Error("no token found in task:failed")
}
