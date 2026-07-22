package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestDependencyTerminal_BlockedUntilArchived(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_terminal"))

	workIDA := "prd-A-work-id"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "prd",
		WorkID:     workIDA,
		Payload:    []byte("PRD A"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "prd",
		Payload:    []byte("PRD B"),
		Relations: []work.Relation{
			{Type: work.RelationDependsOn, TargetWorkID: workIDA, RequiredState: "archived"},
		},
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"executor": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})

	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{"prd:archived": 2, "prd:init": 0, "prd:in-review": 0})

	if provider.CallCount("executor") != 2 {
		t.Errorf("expected executor called 2 times (A+B), got %d", provider.CallCount("executor"))
	}
}

func TestDependencyTerminal_BlockedDuringProcessing(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_terminal"))

	workIDA := "prd-A-processing"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "prd",
		WorkID:     workIDA,
		Payload:    []byte("PRD A"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "prd",
		Payload:    []byte("PRD B"),
		Relations: []work.Relation{
			{Type: work.RelationDependsOn, TargetWorkID: workIDA, RequiredState: "archived"},
		},
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"executor": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})

	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{
		"prd:archived": 2, "prd:init": 0, "prd:in-review": 0, "prd:failed": 0,
	})
}

func TestDependencyTerminal_BothComplete(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_terminal"))

	workIDA := "prd-A-both"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "prd",
		WorkID:     workIDA,
		Payload:    []byte("PRD A"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "prd",
		Payload:    []byte("PRD B"),
		Relations: []work.Relation{
			{Type: work.RelationDependsOn, TargetWorkID: workIDA, RequiredState: "archived"},
		},
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"executor": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})

	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{
		"prd:archived": 2, "prd:init": 0, "prd:in-review": 0, "prd:failed": 0,
	})
}
