package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestDependencyTracking_BlocksUntilSatisfied(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

	workIDA := "task-A-work-id"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     workIDA,
		Payload:    []byte("task A"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("task B"),
		Relations: []work.Relation{
			{Type: work.RelationDependsOn, TargetWorkID: workIDA, RequiredState: "complete"},
		},
	})

	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
	)
	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{"task:init": 0, "task:processing": 0, "task:complete": 2})

	if got := len(support.ProviderCallsForWorker(provider, "starter")); got != 2 {
		t.Errorf("expected starter called 2 times, got %d", got)
	}
}

func TestDependencyTracking_NoDepsPassThrough(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_simple_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("no deps"))

	provider := testutil.NewMockProvider(support.AcceptedProviderResponse())
	session := support.RunFactoryToCompletion(t, dir, provider, 5*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{"task:complete": 1, "task:init": 0})
}
