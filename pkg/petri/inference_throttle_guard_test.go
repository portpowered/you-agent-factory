package petri

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
)

func TestInferenceThrottleGuard_ActivePausesDeriveFromCompletedDispatchHistoryAndClock(t *testing.T) {
	now := time.Date(2026, time.May, 2, 15, 0, 0, 0, time.UTC)
	guard := &InferenceThrottleGuard{
		Provider:      "claude",
		Model:         "claude-sonnet",
		WorkerName:    "writer",
		RefreshWindow: 10 * time.Minute,
	}

	active := guard.ActivePauses(RuntimeGuardContext{
		Now: now,
		DispatchHistory: []interfaces.CompletedDispatch{
			completedThrottleFailure("dispatch-match", "t-claude", now.Add(-4*time.Minute)),
			completedThrottleFailure("dispatch-other", "t-codex", now.Add(-2*time.Minute)),
			completedNonThrottleFailure("dispatch-nonthrottle", "t-claude", now.Add(-time.Minute)),
		},
		RuntimeConfig: runtimefixtures.RuntimeDefinitionLookupFixture{
			Workers: map[string]*interfaces.WorkerConfig{
				"writer": {Name: "writer", ModelProvider: "claude", Model: "claude-sonnet"},
				"codex":  {Name: "codex", ModelProvider: "openai", Model: "gpt-5.4"},
			},
		},
		TransitionWorkers: map[string]string{
			"t-claude": "writer",
			"t-codex":  "codex",
		},
	})

	if len(active) != 1 {
		t.Fatalf("active pause count = %d, want 1", len(active))
	}
	pause := active[0]
	if pause.LaneID != "claude/claude-sonnet" || pause.Provider != "claude" || pause.Model != "claude-sonnet" {
		t.Fatalf("active pause identity = %#v, want claude/claude-sonnet lane", pause)
	}
	if !pause.PausedAt.Equal(now.Add(-4 * time.Minute)) {
		t.Fatalf("PausedAt = %s, want %s", pause.PausedAt, now.Add(-4*time.Minute))
	}
	if !pause.PausedUntil.Equal(now.Add(6 * time.Minute)) {
		t.Fatalf("PausedUntil = %s, want %s", pause.PausedUntil, now.Add(6*time.Minute))
	}
}

func TestInferenceThrottleGuard_EvaluateRuntimeBlocksOnlyWhilePauseWindowIsActive(t *testing.T) {
	pausedAt := time.Date(2026, time.May, 2, 15, 0, 0, 0, time.UTC)
	guard := &InferenceThrottleGuard{
		Provider:      "claude",
		Model:         "claude-sonnet",
		WorkerName:    "writer",
		RefreshWindow: 5 * time.Minute,
	}
	candidates := []interfaces.Token{{ID: "tok-1"}}
	runtimeConfig := runtimefixtures.RuntimeDefinitionLookupFixture{
		Workers: map[string]*interfaces.WorkerConfig{
			"writer": {Name: "writer", ModelProvider: "claude", Model: "claude-sonnet"},
		},
	}
	history := []interfaces.CompletedDispatch{
		completedThrottleFailure("dispatch-match", "t-claude", pausedAt),
	}
	transitionWorkers := map[string]string{
		"t-claude": "writer",
	}

	matched, ok := guard.EvaluateRuntime(RuntimeGuardContext{
		Now:               pausedAt.Add(4 * time.Minute),
		DispatchHistory:   history,
		RuntimeConfig:     runtimeConfig,
		TransitionWorkers: transitionWorkers,
	}, candidates, nil, nil)
	if ok || matched != nil {
		t.Fatalf("active pause evaluation = (%v, %t), want (nil, false)", matched, ok)
	}

	matched, ok = guard.EvaluateRuntime(RuntimeGuardContext{
		Now:               pausedAt.Add(6 * time.Minute),
		DispatchHistory:   history,
		RuntimeConfig:     runtimeConfig,
		TransitionWorkers: transitionWorkers,
	}, candidates, nil, nil)
	if !ok {
		t.Fatal("expected authored lane to resume after pause expiry")
	}
	if len(matched) != 1 || matched[0].ID != "tok-1" {
		t.Fatalf("matched tokens after expiry = %#v, want original candidate", matched)
	}
}

func completedThrottleFailure(dispatchID, transitionID string, endedAt time.Time) interfaces.CompletedDispatch {
	return interfaces.CompletedDispatch{
		DispatchID:   dispatchID,
		TransitionID: transitionID,
		EndTime:      endedAt,
		ProviderFailure: &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyThrottle,
		},
	}
}

func completedNonThrottleFailure(dispatchID, transitionID string, endedAt time.Time) interfaces.CompletedDispatch {
	return interfaces.CompletedDispatch{
		DispatchID:   dispatchID,
		TransitionID: transitionID,
		EndTime:      endedAt,
		ProviderFailure: &interfaces.ProviderFailureMetadata{
			Family: interfaces.ProviderErrorFamilyRetryable,
		},
	}
}
