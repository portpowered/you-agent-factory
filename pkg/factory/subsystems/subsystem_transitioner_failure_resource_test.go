package subsystems

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestReleaseResourceTokensOnFailure_PreservesConsumedTokenIdentityRegardlessOfInputOrder(t *testing.T) {
	now := time.Date(2026, time.July, 3, 10, 30, 0, 0, time.UTC)
	createdAt := now.Add(-2 * time.Hour)
	resourceConsumed := interfaces.Token{
		ID:        "executor:resource:0",
		PlaceID:   "executor:available",
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color: interfaces.TokenColor{
			WorkID:     "executor:0",
			WorkTypeID: "executor",
			DataType:   interfaces.DataTypeResource,
			Tags:       map[string]string{"pool": "shared"},
		},
		History: interfaces.TokenHistory{
			PlaceVisits: map[string]int{"executor:available": 4},
		},
	}
	workConsumed := interfaces.Token{
		ID:        "tok-1",
		PlaceID:   "wt-code:init",
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		Color: interfaces.TokenColor{
			WorkID:     "w-resource-failure",
			WorkTypeID: "wt-code",
			DataType:   interfaces.DataTypeWork,
		},
	}
	failureArcs := []petri.Arc{{ID: "a3", Name: "fail", PlaceID: "wt-code:failed", Direction: petri.ArcOutput}}
	transitioner := NewTransitioner(&state.Net{
		Places: map[string]*petri.Place{
			"executor:available": {ID: "executor:available", TypeID: "executor", State: "available"},
		},
	}, nil, WithTransitionerClock(func() time.Time { return now }))

	orderings := []struct {
		name     string
		consumed []interfaces.Token
	}{
		{name: "resource-first", consumed: []interfaces.Token{resourceConsumed, workConsumed}},
		{name: "work-first", consumed: []interfaces.Token{workConsumed, resourceConsumed}},
	}

	for _, ordering := range orderings {
		t.Run(ordering.name, func(t *testing.T) {
			mutations := transitioner.releaseResourceTokensOnFailureMutations(
				interfaces.OutcomeFailed,
				"t1",
				ordering.consumed,
				failureArcs,
				now,
			)
			if len(mutations) != 1 {
				t.Fatalf("mutation count = %d, want 1 resource release", len(mutations))
			}
			released := mutations[0]
			if released.ToPlace != "executor:available" {
				t.Fatalf("ToPlace = %q, want executor:available", released.ToPlace)
			}
			if released.NewToken.ID != resourceConsumed.ID {
				t.Fatalf("released ID = %q, want %q", released.NewToken.ID, resourceConsumed.ID)
			}
			if released.NewToken.Color.WorkID != resourceConsumed.Color.WorkID {
				t.Fatalf("released WorkID = %q, want %q", released.NewToken.Color.WorkID, resourceConsumed.Color.WorkID)
			}
			if released.NewToken.Color.Tags["pool"] != "shared" {
				t.Fatalf("released tag pool = %q, want shared", released.NewToken.Color.Tags["pool"])
			}
			if !released.NewToken.CreatedAt.Equal(createdAt) {
				t.Fatalf("CreatedAt = %v, want %v", released.NewToken.CreatedAt, createdAt)
			}
		})
	}
}
