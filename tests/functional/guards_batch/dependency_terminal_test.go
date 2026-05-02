package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestDependencyTerminal_BlockedUntilArchived(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_terminal"))

	workIDA := "prd-A-work-id"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "prd",
		WorkID:     workIDA,
		Payload:    []byte("PRD A"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "prd",
		Payload:    []byte("PRD B"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: workIDA, RequiredState: "archived"},
		},
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"executor": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("prd:archived", 2).
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("prd:in-review")

	if provider.CallCount("executor") != 2 {
		t.Errorf("expected executor called 2 times (A+B), got %d", provider.CallCount("executor"))
	}
}

func TestDependencyTerminal_BlockedDuringProcessing(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_terminal"))

	workIDA := "prd-A-processing"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "prd",
		WorkID:     workIDA,
		Payload:    []byte("PRD A"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "prd",
		Payload:    []byte("PRD B"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: workIDA, RequiredState: "archived"},
		},
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"executor": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("prd:archived", 2).
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("prd:in-review").
		HasNoTokenInPlace("prd:failed")
}

func TestDependencyTerminal_BothComplete(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_terminal"))

	workIDA := "prd-A-both"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "prd",
		WorkID:     workIDA,
		Payload:    []byte("PRD A"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "prd",
		Payload:    []byte("PRD B"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: workIDA, RequiredState: "archived"},
		},
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"executor": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("prd:archived", 2).
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("prd:in-review").
		HasNoTokenInPlace("prd:failed").
		AllTokensTerminal()
}
