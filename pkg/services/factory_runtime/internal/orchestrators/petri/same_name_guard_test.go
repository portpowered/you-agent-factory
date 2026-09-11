package petri

import (
	"testing"

	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

func TestSameNameGuardRuntime_UsesOrderedRegisteredParentChild(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "project-token",
		Color: factorytoken.Color{WorkID: "project-work", Name: "resume-project"},
	}
	historical := factorytoken.Token{
		ID:      "a-token-history",
		PlaceID: "project-cycle:blocked",
		Color: factorytoken.Color{
			Name:       "resume-project",
			WorkID:     "cycle-44",
			WorkTypeID: "project-cycle",
			ParentID:   "project-work",
		},
	}
	current := historical
	current.ID = "z-token-current"
	current.Color.WorkID = "cycle-93"

	guard := &SameNameGuard{MatchBinding: "project"}
	marking := &MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
		historical.ID: &historical,
		current.ID:    &current,
	}}
	matched, ok := guard.EvaluateRuntime(
		RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
			"project-work": {
				Children: []factorytoken.Token{historical, current},
				Complete: true,
			},
		}},
		[]factorytoken.Token{current, historical},
		map[string]*factorytoken.Token{"project": parent},
		marking,
	)
	if !ok || len(matched) != 1 || matched[0].Color.WorkID != "cycle-93" {
		t.Fatalf("same-name runtime binding = %#v, %t; want only registered current cycle-93", matched, ok)
	}
}

func TestSameNameGuardRuntime_PreservesUnrelatedEquality(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "project-token",
		Color: factorytoken.Color{WorkID: "project-work", Name: "shared-name"},
	}
	candidates := []factorytoken.Token{
		{ID: "unrelated-a", Color: factorytoken.Color{Name: "shared-name"}},
		{ID: "unrelated-b", Color: factorytoken.Color{Name: "shared-name", ParentID: "other-parent"}},
	}

	matched, ok := (&SameNameGuard{MatchBinding: "project"}).EvaluateRuntime(
		RuntimeGuardContext{},
		candidates,
		map[string]*factorytoken.Token{"project": parent},
		nil,
	)
	if !ok || len(matched) != len(candidates) {
		t.Fatalf("unrelated same-name runtime match = %#v, %t; want equality-compatible candidates", matched, ok)
	}
}

func TestSameNameGuardRuntime_CanceledCurrentAdmissionDoesNotFallback(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "project-token",
		Color: factorytoken.Color{WorkID: "project-work", Name: "resume-project"},
	}
	historical := factorytoken.Token{
		ID:      "a-token-history",
		PlaceID: "project-cycle:blocked",
		Color: factorytoken.Color{
			Name:       "resume-project",
			WorkID:     "cycle-44",
			WorkTypeID: "project-cycle",
			ParentID:   "project-work",
		},
	}
	current := historical
	current.ID = "z-token-canceled"
	current.Color.WorkID = "cycle-93"
	marking := &MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
		historical.ID: &historical,
		current.ID:    &current,
	}}
	matched, ok := (&SameNameGuard{MatchBinding: "project"}).EvaluateRuntime(
		RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
			"project-work": {Children: []factorytoken.Token{historical, current}, Complete: false},
		}},
		[]factorytoken.Token{historical, current},
		map[string]*factorytoken.Token{"project": parent},
		marking,
	)
	if ok || len(matched) != 0 {
		t.Fatalf("canceled current admission fell back to historical cycle: matched=%#v ok=%t", matched, ok)
	}
}

func TestSameNameGuardRuntime_FailsClosedForIncompleteParentRegistration(t *testing.T) {
	parent := &factorytoken.Token{
		ID:    "project-token",
		Color: factorytoken.Color{WorkID: "project-work", Name: "resume-project"},
	}
	historical := factorytoken.Token{
		ID:      "cycle-history-token",
		PlaceID: "project-cycle:blocked",
		Color: factorytoken.Color{
			Name:       "resume-project",
			WorkID:     "cycle-44",
			WorkTypeID: "project-cycle",
			ParentID:   "project-work",
		},
	}
	current := historical
	current.ID = "cycle-current-token"
	current.Color.WorkID = "cycle-93"
	bindings := map[string]*factorytoken.Token{"project": parent}

	baseSnapshot := func(tokens ...factorytoken.Token) *MarkingSnapshot {
		snapshot := &MarkingSnapshot{Tokens: make(map[string]*factorytoken.Token, len(tokens))}
		for index := range tokens {
			token := tokens[index]
			snapshot.Tokens[token.ID] = &token
		}
		return snapshot
	}
	assertBlocked := func(name string, ctx RuntimeGuardContext, candidates []factorytoken.Token, marking *MarkingSnapshot) {
		t.Run(name, func(t *testing.T) {
			matched, ok := (&SameNameGuard{MatchBinding: "project"}).EvaluateRuntime(ctx, candidates, bindings, marking)
			if ok || len(matched) != 0 {
				t.Fatalf("incomplete parent registration enabled join: matched=%#v ok=%t", matched, ok)
			}
		})
	}

	assertBlocked("missing projection", RuntimeGuardContext{}, []factorytoken.Token{historical, current}, baseSnapshot(historical, current))
	assertBlocked("incomplete projection", RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"project-work": {Children: []factorytoken.Token{historical, current}, Complete: false},
	}}, []factorytoken.Token{historical, current}, baseSnapshot(historical, current))
	assertBlocked("registered child is not visible", RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"project-work": {Children: []factorytoken.Token{historical, current}, Complete: true},
	}}, []factorytoken.Token{historical}, baseSnapshot(historical))
	assertBlocked("visible child is not registered", RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"project-work": {Children: []factorytoken.Token{historical}, Complete: true},
	}}, []factorytoken.Token{historical, current}, baseSnapshot(historical, current))

	contradictory := historical
	contradictory.Color.ParentID = "other-parent"
	assertBlocked("registration has contradictory parent", RuntimeGuardContext{ParentChildRegistrations: ParentChildRegistrationProjection{
		"project-work": {Children: []factorytoken.Token{contradictory, current}, Complete: true},
	}}, []factorytoken.Token{historical, current}, baseSnapshot(historical, current))
}
