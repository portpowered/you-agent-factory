package subsystems

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/token"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/token_transformer"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestReleaseResourceTokensOnFailure_PreservesConsumedTokenIdentityRegardlessOfInputOrder(t *testing.T) {
	now := time.Date(2026, time.July, 3, 10, 30, 0, 0, time.UTC)
	createdAt := now.Add(-2 * time.Hour)
	resourceConsumed := factorytoken.Token{
		ID:        "executor:resource:0",
		PlaceID:   "executor:available",
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		Color: factorytoken.Color{
			WorkID:     "executor:0",
			WorkTypeID: "executor",
			DataType:   factorytoken.DataTypeResource,
			Tags:       map[string]string{"pool": "shared"},
		},
		History: factorytoken.History{
			PlaceVisits: map[string]int{"executor:available": 4},
		},
	}
	workConsumed := factorytoken.Token{
		ID:        "tok-1",
		PlaceID:   "wt-code:init",
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		Color: factorytoken.Color{
			WorkID:     "w-resource-failure",
			WorkTypeID: "wt-code",
			DataType:   factorytoken.DataTypeWork,
		},
	}
	failureArcs := []petri.Arc{{ID: "a3", Name: "fail", PlaceID: "wt-code:failed", Direction: petri.ArcOutput}}
	transitioner := NewTransitioner(&state.Net{
		Places: map[string]*petri.Place{
			"executor:available": {ID: "executor:available", TypeID: "executor", State: "available"},
		},
	}, nil, func() time.Time { return now }, testTokenTransformer(&state.Net{
		Places: map[string]*petri.Place{
			"executor:available": {ID: "executor:available", TypeID: "executor", State: "available"},
		},
	}),

		nil, nil, nil, testWorkPropagationPolicy())

	orderings := []struct {
		name     string
		consumed []factorytoken.Token
	}{
		{name: "resource-first", consumed: []factorytoken.Token{resourceConsumed, workConsumed}},
		{name: "work-first", consumed: []factorytoken.Token{workConsumed, resourceConsumed}},
	}

	for _, ordering := range orderings {
		t.Run(ordering.name, func(t *testing.T) {
			mutations := transitioner.releaseResourceTokensOnFailureMutations(
				workerexecution.OutcomeFailed,
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

type acceptedMixedWorkResourceMutationFixture struct {
	transformer *token_transformer.Transformer
	resource    factorytoken.Token
	work        factorytoken.Token
	arcs        []petri.Arc
	result      resolvedWorkResult
	now         time.Time
}

func newAcceptedMixedWorkResourceMutationFixture() acceptedMixedWorkResourceMutationFixture {
	now := time.Date(2026, time.April, 7, 14, 0, 0, 0, time.UTC)
	createdAt := now.Add(-time.Hour)
	places := map[string]*petri.Place{
		"story:complete":       {ID: "story:complete", TypeID: "story", State: "complete"},
		"agent-slot:available": {ID: "agent-slot:available", TypeID: "agent-slot", State: "available"},
		"story:in-review":      {ID: "story:in-review", TypeID: "story", State: "in-review"},
	}
	workTypes := map[string]*state.WorkType{
		"story": {ID: "story"},
	}
	return acceptedMixedWorkResourceMutationFixture{
		transformer: token_transformer.New(places, workTypes, petri.NewWorkIDGenerator()),
		resource: factorytoken.Token{
			ID:        "agent-slot:resource:0",
			PlaceID:   "agent-slot:available",
			CreatedAt: createdAt,
			EnteredAt: createdAt,
			Color: factorytoken.Color{
				WorkID:     "agent-slot:0",
				WorkTypeID: "agent-slot",
				DataType:   factorytoken.DataTypeResource,
			},
		},
		work: factorytoken.Token{
			ID:        "work-story-1",
			PlaceID:   "story:in-review",
			CreatedAt: createdAt,
			EnteredAt: createdAt,
			Color: factorytoken.Color{
				WorkID:     "work-story-1",
				WorkTypeID: "story",
				DataType:   factorytoken.DataTypeWork,
				TraceID:    "trace-batch-idea-001",
			},
			History: factorytoken.History{
				TotalVisits:         map[string]int{},
				ConsecutiveFailures: map[string]int{},
				PlaceVisits:         map[string]int{},
			},
		},
		arcs: []petri.Arc{
			{ID: "work-out", PlaceID: "story:complete"},
			{ID: "slot-out", PlaceID: "agent-slot:available"},
		},
		result: resolvedWorkResult{
			transitionID: "review-story",
			outcome:      workerexecution.OutcomeAccepted,
			output:       "Done. COMPLETE ACCEPTED",
		},
		now: now,
	}
}

func findWorkAndResourceMutations(mutations []interfaces.MarkingMutation) (*interfaces.MarkingMutation, *interfaces.MarkingMutation) {
	var workMutation *interfaces.MarkingMutation
	var resourceMutation *interfaces.MarkingMutation
	for i := range mutations {
		if mutations[i].NewToken == nil {
			continue
		}
		switch mutations[i].NewToken.Color.DataType {
		case factorytoken.DataTypeWork:
			workMutation = &mutations[i]
		case factorytoken.DataTypeResource:
			resourceMutation = &mutations[i]
		}
	}
	return workMutation, resourceMutation
}

func assertAcceptedMixedWorkOutputMutation(t *testing.T, workMutation *interfaces.MarkingMutation) {
	t.Helper()
	if workMutation == nil {
		t.Fatal("expected work output mutation")
	}
	if workMutation.ToPlace != "story:complete" {
		t.Fatalf("work ToPlace = %q, want story:complete", workMutation.ToPlace)
	}
	if workMutation.NewToken.Color.TraceID != "trace-batch-idea-001" {
		t.Fatalf("work TraceID = %q, want trace-batch-idea-001", workMutation.NewToken.Color.TraceID)
	}
}

func assertReleasedResourceMutationBasics(t *testing.T, resourceMutation *interfaces.MarkingMutation, resource factorytoken.Token) {
	t.Helper()
	if resourceMutation == nil {
		t.Fatal("expected resource release mutation")
	}
	if resourceMutation.ToPlace != "agent-slot:available" {
		t.Fatalf("resource ToPlace = %q, want agent-slot:available", resourceMutation.ToPlace)
	}
	if resourceMutation.NewToken.ID != resource.ID {
		t.Fatalf("released resource ID = %q, want consumed identity %q", resourceMutation.NewToken.ID, resource.ID)
	}
	if resourceMutation.NewToken.Color.WorkID != "agent-slot:0" {
		t.Fatalf("released resource WorkID = %q, want agent-slot:0", resourceMutation.NewToken.Color.WorkID)
	}
	if resourceMutation.NewToken.Color.TraceID != "" {
		t.Fatalf("released resource TraceID = %q, want empty", resourceMutation.NewToken.Color.TraceID)
	}
}

func assertAcceptedMixedWorkResourceMutations(t *testing.T, mutations []interfaces.MarkingMutation, resource factorytoken.Token) {
	t.Helper()
	if len(mutations) != 2 {
		t.Fatalf("mutation count = %d, want 2 (work output + resource release)", len(mutations))
	}
	workMutation, resourceMutation := findWorkAndResourceMutations(mutations)
	assertAcceptedMixedWorkOutputMutation(t, workMutation)
	assertReleasedResourceMutationBasics(t, resourceMutation, resource)
}

func TestCalculateMutations_AcceptedMixedWorkResource_ReleasesConsumedResourceRegardlessOfInputOrder(t *testing.T) {
	fixture := newAcceptedMixedWorkResourceMutationFixture()
	orderings := []struct {
		name     string
		consumed []factorytoken.Token
	}{
		{name: "resource-first", consumed: []factorytoken.Token{fixture.resource, fixture.work}},
		{name: "work-first", consumed: []factorytoken.Token{fixture.work, fixture.resource}},
	}

	for _, ordering := range orderings {
		t.Run(ordering.name, func(t *testing.T) {
			mutations, err := calculateMutations(mutationCalculationInput{
				workPropagation: testWorkPropagationPolicy(),
				transition:      &petri.Transition{ID: "review-story"},
				arcs:            fixture.arcs,
				consumed:        ordering.consumed,
				result:          fixture.result,
				now:             fixture.now,
				history:         fixture.work.History,
				inputColors:     tokenColorsFromTokens(ordering.consumed),
				transformer:     fixture.transformer,
			})
			if err != nil {
				t.Fatalf("calculateMutations() error = %v", err)
			}
			assertAcceptedMixedWorkResourceMutations(t, mutations, fixture.resource)
		})
	}
}
