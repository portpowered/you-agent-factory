package scheduler

import (
	"context"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

func TestEnablementEvaluator_CronTimeEnablementUsesSharedTimePlace(t *testing.T) {
	now := time.Date(2026, 4, 18, 13, 0, 0, 0, time.UTC)
	net := schedulerCronNet()
	tests := []struct {
		name     string
		tokens   []*factorytoken.Token
		want     bool
		wantBind []string
	}{
		{
			name: "ready input and due time token enables cron",
			tokens: []*factorytoken.Token{
				schedulerWorkToken("task-ready"),
				schedulerCronTimeToken("time-due", "daily-refresh", now.Add(-time.Second), now.Add(time.Minute)),
			},
			want:     true,
			wantBind: []string{"task:ready:to:daily-refresh", interfaces.SystemTimePendingPlaceID + ":to:daily-refresh"},
		},
		{
			name: "missing configured input disables cron",
			tokens: []*factorytoken.Token{
				schedulerCronTimeToken("time-due", "daily-refresh", now.Add(-time.Second), now.Add(time.Minute)),
			},
		},
		{
			name: "not-yet-due time token disables cron",
			tokens: []*factorytoken.Token{
				schedulerWorkToken("task-ready"),
				schedulerCronTimeToken("time-early", "daily-refresh", now.Add(time.Second), now.Add(time.Minute)),
			},
		},
		{
			name: "expired time token disables cron",
			tokens: []*factorytoken.Token{
				schedulerWorkToken("task-ready"),
				schedulerCronTimeToken("time-expired", "daily-refresh", now.Add(-time.Minute), now),
			},
		},
		{
			name: "wrong workstation time token disables cron",
			tokens: []*factorytoken.Token{
				schedulerWorkToken("task-ready"),
				schedulerCronTimeToken("time-wrong", "other-refresh", now.Add(-time.Second), now.Add(time.Minute)),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := make(map[string]*factorytoken.Token, len(tt.tokens))
			for _, token := range tt.tokens {
				tokens[token.ID] = token
			}
			snapshot := makeTestSnapshot(tokens)
			enabled := NewEnablementEvaluator(nil, func() time.Time { return now }, nil).
				FindEnabledTransitions(context.Background(), net, &snapshot)
			cron := findEnabledTransition(enabled, "daily-refresh")
			if (cron != nil) != tt.want {
				t.Fatalf("cron enabled = %v, want %v; transitions=%+v", cron != nil, tt.want, enabled)
			}
			if cron == nil {
				return
			}
			for _, binding := range tt.wantBind {
				if len(cron.Bindings[binding]) != 1 {
					t.Fatalf("binding %q = %+v, want one token", binding, cron.Bindings[binding])
				}
			}
		})
	}
}

func TestEnablementEvaluator_DefaultExpiryTargetsExpiredTokenCronCannotUse(t *testing.T) {
	now := time.Date(2026, 4, 18, 13, 0, 0, 0, time.UTC)
	tokens := []*factorytoken.Token{
		schedulerWorkToken("task-ready"),
		schedulerCronTimeToken("time-expired", "daily-refresh", now.Add(-time.Minute), now),
	}
	tokenMap := make(map[string]*factorytoken.Token, len(tokens))
	for _, token := range tokens {
		tokenMap[token.ID] = token
	}
	snapshot := makeTestSnapshot(tokenMap)
	enabled := NewEnablementEvaluator(nil, func() time.Time { return now }, nil).
		FindEnabledTransitions(context.Background(), schedulerCronNet(), &snapshot)

	if cron := findEnabledTransition(enabled, "daily-refresh"); cron != nil {
		t.Fatalf("cron transition should reject expired time token, got %+v", cron)
	}
	expiry := findEnabledTransition(enabled, interfaces.SystemTimeExpiryTransitionID)
	if expiry == nil {
		t.Fatal("expected expiry transition to target the stale time token")
	}
	binding := interfaces.SystemTimePendingPlaceID + ":to:" + interfaces.SystemTimeExpiryTransitionID
	if got := expiry.Bindings[binding]; len(got) != 1 || got[0].ID != "time-expired" {
		t.Fatalf("expiry binding = %+v, want time-expired", got)
	}
}

func schedulerCronNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:ready":                        {ID: "task:ready"},
			interfaces.SystemTimePendingPlaceID: {ID: interfaces.SystemTimePendingPlaceID},
		},
		Transitions: map[string]*petri.Transition{
			"daily-refresh": {
				ID:         "daily-refresh",
				WorkerType: "cron-worker",
				InputArcs: []petri.Arc{
					{
						ID:          "task:ready:to:daily-refresh",
						Name:        "task:ready:to:daily-refresh",
						PlaceID:     "task:ready",
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
					},
					{
						ID:          interfaces.SystemTimePendingPlaceID + ":to:daily-refresh",
						Name:        interfaces.SystemTimePendingPlaceID + ":to:daily-refresh",
						PlaceID:     interfaces.SystemTimePendingPlaceID,
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.CronTimeWindowGuard{Workstation: "daily-refresh"},
					},
				},
			},
			interfaces.SystemTimeExpiryTransitionID: {
				ID:         interfaces.SystemTimeExpiryTransitionID,
				WorkerType: "system",
				InputArcs: []petri.Arc{{
					ID:          interfaces.SystemTimePendingPlaceID + ":to:" + interfaces.SystemTimeExpiryTransitionID,
					Name:        interfaces.SystemTimePendingPlaceID + ":to:" + interfaces.SystemTimeExpiryTransitionID,
					PlaceID:     interfaces.SystemTimePendingPlaceID,
					Direction:   petri.ArcInput,
					Cardinality: petri.ArcCardinality{Mode: petri.CardinalityAll},
					Guard:       &petri.ExpiredTimeWorkGuard{},
				}},
			},
		},
	}
}

func schedulerWorkToken(id string) *factorytoken.Token {
	return &factorytoken.Token{
		ID:      id,
		PlaceID: "task:ready",
		Color: factorytoken.Color{
			WorkID:     id,
			WorkTypeID: "task",
			DataType:   factorytoken.DataTypeWork,
		},
	}
}

func findEnabledTransition(
	enabled []interfaces.EnabledTransition,
	transitionID string,
) *interfaces.EnabledTransition {
	for i := range enabled {
		if enabled[i].TransitionID == transitionID {
			return &enabled[i]
		}
	}
	return nil
}
