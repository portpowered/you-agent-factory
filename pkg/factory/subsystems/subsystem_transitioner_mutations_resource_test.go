package subsystems

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

type acceptedMixedWorkResourceMutationFixture struct {
	transformer *token_transformer.Transformer
	resource    interfaces.Token
	work        interfaces.Token
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
		transformer: token_transformer.New(places, workTypes, token_transformer.WithWorkIDGenerator(petri.NewWorkIDGenerator())),
		resource: interfaces.Token{
			ID:        "agent-slot:resource:0",
			PlaceID:   "agent-slot:available",
			CreatedAt: createdAt,
			EnteredAt: createdAt,
			Color: interfaces.TokenColor{
				WorkID:     "agent-slot:0",
				WorkTypeID: "agent-slot",
				DataType:   interfaces.DataTypeResource,
			},
		},
		work: interfaces.Token{
			ID:        "work-story-1",
			PlaceID:   "story:in-review",
			CreatedAt: createdAt,
			EnteredAt: createdAt,
			Color: interfaces.TokenColor{
				WorkID:     "work-story-1",
				WorkTypeID: "story",
				DataType:   interfaces.DataTypeWork,
				TraceID:    "trace-batch-idea-001",
			},
			History: interfaces.TokenHistory{
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
			outcome:      interfaces.OutcomeAccepted,
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
		case interfaces.DataTypeWork:
			workMutation = &mutations[i]
		case interfaces.DataTypeResource:
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

func assertReleasedResourceMutationBasics(t *testing.T, resourceMutation *interfaces.MarkingMutation, resource interfaces.Token) {
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

func assertAcceptedMixedWorkResourceMutations(t *testing.T, mutations []interfaces.MarkingMutation, resource interfaces.Token) {
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
		consumed []interfaces.Token
	}{
		{name: "resource-first", consumed: []interfaces.Token{fixture.resource, fixture.work}},
		{name: "work-first", consumed: []interfaces.Token{fixture.work, fixture.resource}},
	}

	for _, ordering := range orderings {
		t.Run(ordering.name, func(t *testing.T) {
			mutations, err := calculateMutations(mutationCalculationInput{
				transition:  &petri.Transition{ID: "review-story"},
				arcs:        fixture.arcs,
				consumed:    ordering.consumed,
				result:      fixture.result,
				now:         fixture.now,
				history:     fixture.work.History,
				inputColors: tokenColorsFromTokens(ordering.consumed),
				transformer: fixture.transformer,
			})
			if err != nil {
				t.Fatalf("calculateMutations() error = %v", err)
			}
			assertAcceptedMixedWorkResourceMutations(t, mutations, fixture.resource)
		})
	}
}
