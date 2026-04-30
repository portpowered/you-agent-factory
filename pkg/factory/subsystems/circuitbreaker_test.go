package subsystems_test

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/factory/subsystems"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/testutil/runtimefixtures"
)

// helper to build a minimal net with one work type having states: init, processing, complete, failed.
func buildTestNet() *state.Net {
	wt := &state.WorkType{
		ID:   "task",
		Name: "Task",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "processing", Category: state.StateCategoryProcessing},
			{Value: "complete", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}

	places := make(map[string]*petri.Place)
	for _, p := range wt.GeneratePlaces() {
		places[p.ID] = p
	}

	return &state.Net{
		ID:          "test-wf",
		Places:      places,
		Transitions: make(map[string]*petri.Transition),
		WorkTypes:   map[string]*state.WorkType{"task": wt},
		Resources:   make(map[string]*state.ResourceDef),
	}
}

func makeToken(id, placeID string, createdAt time.Time) *interfaces.Token {
	return &interfaces.Token{
		ID:        id,
		PlaceID:   placeID,
		Color:     interfaces.TokenColor{WorkID: id, WorkTypeID: "task"},
		CreatedAt: createdAt,
		EnteredAt: createdAt,
		History: interfaces.TokenHistory{
			TotalVisits:         make(map[string]int),
			ConsecutiveFailures: make(map[string]int),
			PlaceVisits:         make(map[string]int),
		},
	}
}

func TestCircuitBreaker_TickGroup(t *testing.T) {
	n := buildTestNet()
	cb := subsystems.NewCircuitBreaker(n, nil)
	if cb.TickGroup() != subsystems.CircuitBreaker {
		t.Errorf("expected TickGroup %d, got %d", subsystems.CircuitBreaker, cb.TickGroup())
	}
}

func TestCircuitBreaker_MaxTokenAge(t *testing.T) {
	n := buildTestNet()
	n.Limits.MaxTokenAge = 1 * time.Hour

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	cb := subsystems.NewCircuitBreakerWithClock(n, func() time.Time { return now }, nil)

	// Token created 2 hours ago — should exceed MaxTokenAge.
	marking := petri.NewMarking("test-wf")
	tok := makeToken("tok-1", "task:init", now.Add(-2*time.Hour))
	marking.AddToken(tok)

	markingSnap := marking.Snapshot()
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}
	result, err := cb.Execute(context.Background(), &snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %v", result)
	}

	mut := result.Mutations[0]
	if mut.Type != interfaces.MutationMove {
		t.Errorf("expected MOVE, got %s", mut.Type)
	}
	if mut.TokenID != "tok-1" {
		t.Errorf("expected token tok-1, got %s", mut.TokenID)
	}
	if mut.ToPlace != "task:failed" {
		t.Errorf("expected task:failed, got %s", mut.ToPlace)
	}
}

func TestCircuitBreaker_MaxTotalVisits(t *testing.T) {
	n := buildTestNet()
	n.Limits.MaxTotalVisits = 5

	cb := subsystems.NewCircuitBreaker(n, nil)

	marking := petri.NewMarking("test-wf")
	tok := makeToken("tok-1", "task:processing", time.Now())
	tok.History.TotalVisits["coding"] = 3
	tok.History.TotalVisits["review"] = 3 // total = 6 >= 5
	marking.AddToken(tok)

	markingSnap := marking.Snapshot()
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}
	result, err := cb.Execute(context.Background(), &snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %v", result)
	}
	if result.Mutations[0].ToPlace != "task:failed" {
		t.Errorf("expected task:failed, got %s", result.Mutations[0].ToPlace)
	}
}

func TestCircuitBreaker_MaxRetries(t *testing.T) {
	n := buildTestNet()
	n.Transitions["coding"] = &petri.Transition{
		ID:   "coding",
		Name: "coding",
		Type: petri.TransitionNormal,
	}
	runtimeConfig := runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"coding": {
				Name:   "coding",
				Limits: interfaces.WorkstationLimits{MaxRetries: 3},
			},
		},
	}

	cb := subsystems.NewCircuitBreaker(n, nil, subsystems.WithCircuitBreakerRuntimeConfig(runtimeConfig))

	marking := petri.NewMarking("test-wf")
	tok := makeToken("tok-1", "task:init", time.Now())
	tok.History.ConsecutiveFailures["coding"] = 3 // exactly at limit
	marking.AddToken(tok)

	markingSnap := marking.Snapshot()
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}
	result, err := cb.Execute(context.Background(), &snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %v", result)
	}
	if result.Mutations[0].ToPlace != "task:failed" {
		t.Errorf("expected task:failed, got %s", result.Mutations[0].ToPlace)
	}
}

func TestCircuitBreaker_MissingRuntimeRetryLimitSkipsPerWorkstationExhaustion(t *testing.T) {
	n := buildTestNet()
	n.Transitions["coding"] = &petri.Transition{
		ID:   "coding",
		Name: "coding",
		Type: petri.TransitionNormal,
	}
	runtimeConfig := runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"coding": {
				Name:   "coding",
				Limits: interfaces.WorkstationLimits{},
			},
		},
	}

	cb := subsystems.NewCircuitBreaker(n, nil, subsystems.WithCircuitBreakerRuntimeConfig(runtimeConfig))

	marking := petri.NewMarking("test-wf")
	tok := makeToken("tok-1", "task:init", time.Now())
	tok.History.ConsecutiveFailures["coding"] = 4
	marking.AddToken(tok)

	markingSnap := marking.Snapshot()
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}
	result, err := cb.Execute(context.Background(), &snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected no mutation without runtime-config retry limit, got %+v", result)
	}
}

func TestCircuitBreaker_TokenWithinLimits(t *testing.T) {
	n := buildTestNet()
	n.Limits.MaxTokenAge = 1 * time.Hour
	n.Limits.MaxTotalVisits = 10
	n.Transitions["coding"] = &petri.Transition{
		ID:   "coding",
		Name: "coding",
		Type: petri.TransitionNormal,
	}
	runtimeConfig := runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"coding": {
				Name:   "coding",
				Limits: interfaces.WorkstationLimits{MaxRetries: 5},
			},
		},
	}

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	cb := subsystems.NewCircuitBreakerWithClock(
		n,
		func() time.Time { return now },
		nil,
		subsystems.WithCircuitBreakerRuntimeConfig(runtimeConfig),
	)

	marking := petri.NewMarking("test-wf")
	tok := makeToken("tok-1", "task:init", now.Add(-30*time.Minute)) // 30 min < 1 hr
	tok.History.TotalVisits["coding"] = 2                            // 2 < 10
	tok.History.ConsecutiveFailures["coding"] = 1                    // 1 < 5
	marking.AddToken(tok)

	markingSnap := marking.Snapshot()
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}
	result, err := cb.Execute(context.Background(), &snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for token within limits, got %+v", result)
	}
}

func TestCircuitBreaker_SkipsTerminalTokens(t *testing.T) {
	n := buildTestNet()
	n.Limits.MaxTotalVisits = 1

	cb := subsystems.NewCircuitBreaker(n, nil)

	marking := petri.NewMarking("test-wf")
	// Token already in terminal place — should be skipped even if it exceeds limits.
	tok := makeToken("tok-1", "task:complete", time.Now())
	tok.History.TotalVisits["coding"] = 10
	marking.AddToken(tok)

	markingSnap := marking.Snapshot()
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}
	result, err := cb.Execute(context.Background(), &snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for terminal token, got %+v", result)
	}
}

func TestCircuitBreaker_ExhaustionTransition(t *testing.T) {
	n := buildTestNet()

	// Add an exhaustion transition: when coding visits >= 3, move to failed.
	n.Transitions["coding-exhausted"] = &petri.Transition{
		ID:   "coding-exhausted",
		Type: petri.TransitionExhaustion,
		InputArcs: []petri.Arc{
			{
				ID:      "exh-in",
				Name:    "work",
				PlaceID: "task:init",
				Guard: &petri.VisitCountGuard{
					TransitionID: "coding",
					MaxVisits:    3,
				},
				Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
			},
		},
		OutputArcs: []petri.Arc{
			{
				ID:      "exh-out",
				PlaceID: "task:failed",
			},
		},
	}

	cb := subsystems.NewCircuitBreaker(n, nil)

	marking := petri.NewMarking("test-wf")
	tok := makeToken("tok-1", "task:init", time.Now())
	tok.History.TotalVisits["coding"] = 3 // meets threshold
	marking.AddToken(tok)

	markingSnap := marking.Snapshot()
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}
	result, err := cb.Execute(context.Background(), &snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected 1 mutation, got %v", result)
	}

	mut := result.Mutations[0]
	if mut.Type != interfaces.MutationMove {
		t.Errorf("expected MOVE, got %s", mut.Type)
	}
	if mut.ToPlace != "task:failed" {
		t.Errorf("expected task:failed, got %s", mut.ToPlace)
	}
	if mut.TokenID != "tok-1" {
		t.Errorf("expected tok-1, got %s", mut.TokenID)
	}
}

func TestCircuitBreaker_ExhaustionNotTriggered(t *testing.T) {
	n := buildTestNet()

	n.Transitions["coding-exhausted"] = &petri.Transition{
		ID:   "coding-exhausted",
		Type: petri.TransitionExhaustion,
		InputArcs: []petri.Arc{
			{
				ID:      "exh-in",
				Name:    "work",
				PlaceID: "task:init",
				Guard: &petri.VisitCountGuard{
					TransitionID: "coding",
					MaxVisits:    3,
				},
				Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
			},
		},
		OutputArcs: []petri.Arc{
			{
				ID:      "exh-out",
				PlaceID: "task:failed",
			},
		},
	}

	cb := subsystems.NewCircuitBreaker(n, nil)

	marking := petri.NewMarking("test-wf")
	tok := makeToken("tok-1", "task:init", time.Now())
	tok.History.TotalVisits["coding"] = 2 // below threshold
	marking.AddToken(tok)

	markingSnap := marking.Snapshot()
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}
	result, err := cb.Execute(context.Background(), &snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestCircuitBreaker_DefaultTimeExpiryTransitionConsumesExpiredTimeWork(t *testing.T) {
	now := time.Date(2026, 4, 18, 14, 0, 0, 0, time.UTC)
	n := buildTimeExpiryNet()
	cb := subsystems.NewCircuitBreakerWithClock(n, func() time.Time { return now }, nil)

	marking := petri.NewMarking("test-wf")
	expired := makeCronTimeToken("time-expired", "daily-refresh", now.Add(-10*time.Minute), now.Add(-time.Minute))
	notExpired := makeCronTimeToken("time-pending", "daily-refresh", now.Add(-time.Minute), now.Add(time.Minute))
	marking.AddToken(expired)
	marking.AddToken(notExpired)

	markingSnap := marking.Snapshot()
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}
	result, err := cb.Execute(context.Background(), &snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected one expiry mutation, got %+v", result)
	}

	mut := result.Mutations[0]
	if mut.Type != interfaces.MutationConsume {
		t.Fatalf("expected CONSUME mutation, got %s", mut.Type)
	}
	if mut.TokenID != "time-expired" {
		t.Fatalf("expected expired token to be consumed, got %s", mut.TokenID)
	}
	if mut.FromPlace != interfaces.SystemTimePendingPlaceID || mut.ToPlace != "" {
		t.Fatalf("expected expiry consume from pending place with no output, got %+v", mut)
	}
}

func TestCircuitBreaker_DefaultTimeExpiryTransitionIgnoresPendingTimeWork(t *testing.T) {
	now := time.Date(2026, 4, 18, 14, 0, 0, 0, time.UTC)
	n := buildTimeExpiryNet()
	cb := subsystems.NewCircuitBreakerWithClock(n, func() time.Time { return now }, nil)

	marking := petri.NewMarking("test-wf")
	marking.AddToken(makeCronTimeToken("time-pending", "daily-refresh", now.Add(-time.Minute), now.Add(time.Minute)))

	markingSnap := marking.Snapshot()
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}
	result, err := cb.Execute(context.Background(), &snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected no expiry result for pending time work, got %+v", result)
	}
}

func TestCircuitBreaker_Phase1PreemptsPhase2(t *testing.T) {
	n := buildTestNet()
	n.Limits.MaxTotalVisits = 5

	// Exhaustion transition also targets the same token.
	n.Transitions["coding-exhausted"] = &petri.Transition{
		ID:   "coding-exhausted",
		Type: petri.TransitionExhaustion,
		InputArcs: []petri.Arc{
			{
				ID:      "exh-in",
				Name:    "work",
				PlaceID: "task:init",
				Guard: &petri.VisitCountGuard{
					TransitionID: "coding",
					MaxVisits:    3,
				},
				Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
			},
		},
		OutputArcs: []petri.Arc{
			{
				ID:      "exh-out",
				PlaceID: "task:failed",
			},
		},
	}

	cb := subsystems.NewCircuitBreaker(n, nil)

	marking := petri.NewMarking("test-wf")
	tok := makeToken("tok-1", "task:init", time.Now())
	tok.History.TotalVisits["coding"] = 5 // exceeds both global limit and exhaustion threshold
	marking.AddToken(tok)

	markingSnap := marking.Snapshot()
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{Marking: markingSnap}
	result, err := cb.Execute(context.Background(), &snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should only get 1 mutation (Phase 1), not 2.
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("expected exactly 1 mutation (Phase 1 preempts Phase 2), got %v", result)
	}
	if result.Mutations[0].Reason == "" {
		t.Error("expected non-empty reason")
	}
}

func buildTimeExpiryNet() *state.Net {
	return &state.Net{
		ID: "test-wf",
		Places: map[string]*petri.Place{
			interfaces.SystemTimePendingPlaceID: {
				ID:     interfaces.SystemTimePendingPlaceID,
				TypeID: interfaces.SystemTimeWorkTypeID,
				State:  interfaces.SystemTimePendingState,
			},
		},
		Transitions: map[string]*petri.Transition{
			interfaces.SystemTimeExpiryTransitionID: {
				ID:   interfaces.SystemTimeExpiryTransitionID,
				Name: interfaces.SystemTimeExpiryTransitionID,
				Type: petri.TransitionExhaustion,
				InputArcs: []petri.Arc{
					{
						ID:           "time-expiry-in",
						Name:         interfaces.SystemTimePendingPlaceID + ":to:" + interfaces.SystemTimeExpiryTransitionID,
						PlaceID:      interfaces.SystemTimePendingPlaceID,
						TransitionID: interfaces.SystemTimeExpiryTransitionID,
						Direction:    petri.ArcInput,
						Mode:         interfaces.ArcModeConsume,
						Guard:        &petri.ExpiredTimeWorkGuard{},
						Cardinality:  petri.ArcCardinality{Mode: petri.CardinalityAll},
					},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			interfaces.SystemTimeWorkTypeID: {
				ID:   interfaces.SystemTimeWorkTypeID,
				Name: interfaces.SystemTimeWorkTypeID,
				States: []state.StateDefinition{
					{Value: interfaces.SystemTimePendingState, Category: state.StateCategoryProcessing},
				},
			},
		},
		Resources: make(map[string]*state.ResourceDef),
	}
}

func makeCronTimeToken(id string, workstation string, dueAt time.Time, expiresAt time.Time) *interfaces.Token {
	return &interfaces.Token{
		ID:        id,
		PlaceID:   interfaces.SystemTimePendingPlaceID,
		CreatedAt: dueAt,
		EnteredAt: dueAt,
		Color: interfaces.TokenColor{
			WorkID:     id,
			WorkTypeID: interfaces.SystemTimeWorkTypeID,
			DataType:   interfaces.DataTypeWork,
			Tags: map[string]string{
				interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
				interfaces.TimeWorkTagKeyCronWorkstation: workstation,
				interfaces.TimeWorkTagKeyNominalAt:       dueAt.UTC().Format(time.RFC3339Nano),
				interfaces.TimeWorkTagKeyDueAt:           dueAt.UTC().Format(time.RFC3339Nano),
				interfaces.TimeWorkTagKeyExpiresAt:       expiresAt.UTC().Format(time.RFC3339Nano),
				interfaces.TimeWorkTagKeyJitter:          "0s",
			},
		},
	}
}
