package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestDependencyTracking_BlocksUntilSatisfied(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

	workIDA := "task-A-work-id"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     workIDA,
		Payload:    []byte("task A"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("task B"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: workIDA, RequiredState: "complete"},
		},
	})

	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		PlaceTokenCount("task:complete", 2)

	if got := len(support.ProviderCallsForWorker(provider, "starter")); got != 2 {
		t.Errorf("expected starter called 2 times, got %d", got)
	}
}

func TestDependencyTracking_NoDepsPassThrough(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_simple_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("no deps"))

	provider := testutil.NewMockProvider(support.AcceptedProviderResponse())
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		TokenCount(1)
}
