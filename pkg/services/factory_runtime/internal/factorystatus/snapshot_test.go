package factorystatus

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/legacysnapshot"
)

func TestProjectFromSnapshot_CategorizesPublicWorkAndResources(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	snapshot := &legacysnapshot.Snapshot{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
		Topology: &factory.Net{
			Places: map[string]*factory.PetriPlace{
				"task:init":     {ID: "task:init", TypeID: "task", State: "init"},
				"task:review":   {ID: "task:review", TypeID: "task", State: "review"},
				"task:complete": {ID: "task:complete", TypeID: "task", State: "complete"},
				"task:failed":   {ID: "task:failed", TypeID: "task", State: "failed"},
			},
			WorkTypes: map[string]*factory.WorkType{"task": {ID: "task", States: []factory.StateDefinition{
				{Value: "init", Category: factory.StateCategoryInitial},
				{Value: "review", Category: factory.StateCategoryProcessing},
				{Value: "complete", Category: factory.StateCategoryTerminal},
				{Value: "failed", Category: factory.StateCategoryFailed},
			}}},
			Resources: map[string]*factory.ResourceDef{"agent-slot": {ID: "agent-slot", Capacity: 2}},
		},
		Marking: factory.PetriMarkingSnapshot{Tokens: map[string]*factory.RuntimeToken{
			"tok-init":     factoryStatusTestToken("tok-init", "task:init", "work-init", now),
			"tok-review":   factoryStatusTestToken("tok-review", "task:review", "work-review", now),
			"tok-complete": factoryStatusTestToken("tok-complete", "task:complete", "work-complete", now),
			"tok-failed":   factoryStatusTestToken("tok-failed", "task:failed", "work-failed", now),
			"resource": {
				ID: "resource", PlaceID: "agent-slot:available",
				Color: factory.RuntimeTokenColor{DataType: factory.RuntimeTokenDataTypeResource}, CreatedAt: now, EnteredAt: now,
			},
			"time": {
				ID: "time", PlaceID: interfaces.SystemTimePendingPlaceID,
				Color: factory.RuntimeTokenColor{WorkID: "time-daily-refresh", WorkTypeID: interfaces.SystemTimeWorkTypeID}, CreatedAt: now, EnteredAt: now,
			},
		}},
	}

	got := ProjectFromSnapshot(snapshot)
	if got.FactoryState != "RUNNING" || got.RuntimeStatus != "ACTIVE" || got.TotalTokens != 5 {
		t.Fatalf("status = %#v, want RUNNING/ACTIVE with five public status tokens", got)
	}
	if got.Categories != (factory.FactoryStatusCategories{Initial: 1, Processing: 1, Terminal: 1, Failed: 1}) {
		t.Fatalf("categories = %#v, want one Work token in each category", got.Categories)
	}
	if len(got.Resources) != 1 || got.Resources[0] != (factory.FactoryResourceUsage{Name: "agent-slot", Available: 1, Total: 2}) {
		t.Fatalf("resources = %#v, want detached agent-slot 1/2 projection", got.Resources)
	}
}

func TestProjectFromSnapshot_ReturnsDetachedOwnerResult(t *testing.T) {
	t.Parallel()
	snapshot := &legacysnapshot.Snapshot{
		Topology: &factory.Net{Resources: map[string]*factory.ResourceDef{"gpu": {ID: "gpu", Capacity: 4}}},
		Marking: factory.PetriMarkingSnapshot{Tokens: map[string]*factory.RuntimeToken{
			"gpu": {ID: "gpu", PlaceID: "gpu:available", Color: factory.RuntimeTokenColor{DataType: factory.RuntimeTokenDataTypeResource}},
		}},
	}

	got := ProjectFromSnapshot(snapshot)
	snapshot.Topology.Resources["gpu"].Capacity = 99
	snapshot.Marking.Tokens["gpu"].PlaceID = "gpu:busy"

	if len(got.Resources) != 1 || got.Resources[0] != (factory.FactoryResourceUsage{Name: "gpu", Available: 1, Total: 4}) {
		t.Fatalf("detached resources after source mutation = %#v, want original gpu 1/4", got.Resources)
	}
}

func factoryStatusTestToken(id, placeID, workID string, now time.Time) *factory.RuntimeToken {
	return &factory.RuntimeToken{
		ID: id, PlaceID: placeID,
		Color:     factory.RuntimeTokenColor{WorkID: workID, WorkTypeID: "task"},
		CreatedAt: now, EnteredAt: now,
	}
}
