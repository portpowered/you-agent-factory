package throttle

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func TestDeriveActiveThrottlePauses_ResolvesFailureMetadataOnlyRecord(t *testing.T) {
	now := time.Date(2026, time.May, 1, 12, 0, 0, 0, time.UTC)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{{
		Provider:   "claude",
		Model:      "claude-sonnet",
		OccurredAt: now.Add(-5 * time.Minute),
		FailureMetadata: &interfaces.WorkFailureMetadata{
			Family: interfaces.WorkFailureFamilyThrottle,
			Type:   interfaces.WorkFailureTypeThrottled,
		},
	}}, 30*time.Minute, now)

	if len(pauses) != 1 {
		t.Fatalf("pause count = %d, want 1", len(pauses))
	}
	if pauses[0].LaneID != "claude/claude-sonnet" {
		t.Fatalf("lane ID = %q, want claude/claude-sonnet", pauses[0].LaneID)
	}
}

func TestDeriveActiveThrottlePauses_CreatesOneActiveLaneForThrottleFailure(t *testing.T) {
	now := time.Date(2026, time.May, 1, 12, 0, 0, 0, time.UTC)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{{
		Provider:   "claude",
		Model:      "claude-sonnet",
		OccurredAt: now.Add(-5 * time.Minute),
		FailureMetadata: &interfaces.WorkFailureMetadata{
			Family: interfaces.WorkFailureFamilyThrottle,
			Type:   interfaces.WorkFailureTypeThrottled,
		},
	}}, 30*time.Minute, now)

	if len(pauses) != 1 {
		t.Fatalf("pause count = %d, want 1", len(pauses))
	}
	if pauses[0].LaneID != "claude/claude-sonnet" {
		t.Fatalf("lane ID = %q, want claude/claude-sonnet", pauses[0].LaneID)
	}
}

func TestDeriveActiveThrottlePauses_LaterFailureExtendsLaneExpiry(t *testing.T) {
	now := time.Date(2026, time.May, 1, 12, 0, 0, 0, time.UTC)
	first := now.Add(-20 * time.Minute)
	second := now.Add(-10 * time.Minute)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{
		{
			Provider:   "claude",
			Model:      "claude-sonnet",
			OccurredAt: first,
			FailureMetadata: &interfaces.WorkFailureMetadata{
				Family: interfaces.WorkFailureFamilyThrottle,
				Type:   interfaces.WorkFailureTypeThrottled,
			},
		},
		{
			Provider:   "claude",
			Model:      "claude-sonnet",
			OccurredAt: second,
			FailureMetadata: &interfaces.WorkFailureMetadata{
				Family: interfaces.WorkFailureFamilyThrottle,
				Type:   interfaces.WorkFailureTypeThrottled,
			},
		},
	}, 15*time.Minute, now)

	if len(pauses) != 1 {
		t.Fatalf("pause count = %d, want 1", len(pauses))
	}
	if !pauses[0].PausedAt.Equal(first) {
		t.Fatalf("PausedAt = %s, want %s", pauses[0].PausedAt, first)
	}
	if !pauses[0].PausedUntil.Equal(second.Add(15 * time.Minute)) {
		t.Fatalf("PausedUntil = %s, want %s", pauses[0].PausedUntil, second.Add(15*time.Minute))
	}
}

func TestDeriveActiveThrottlePauses_OmitsExpiredWindows(t *testing.T) {
	now := time.Date(2026, time.May, 1, 12, 0, 0, 0, time.UTC)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{{
		Provider:   "claude",
		Model:      "claude-sonnet",
		OccurredAt: now.Add(-45 * time.Minute),
		FailureMetadata: &interfaces.WorkFailureMetadata{
			Family: interfaces.WorkFailureFamilyThrottle,
			Type:   interfaces.WorkFailureTypeThrottled,
		},
	}}, 15*time.Minute, now)

	if len(pauses) != 0 {
		t.Fatalf("pause count = %d, want 0", len(pauses))
	}
}

func TestDeriveActiveThrottlePauses_KeepsProviderOnlyAndProviderModelLanesIsolated(t *testing.T) {
	now := time.Date(2026, time.May, 1, 12, 0, 0, 0, time.UTC)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{
		{
			Provider:   "claude",
			Model:      "",
			OccurredAt: now.Add(-5 * time.Minute),
			FailureMetadata: &interfaces.WorkFailureMetadata{
				Family: interfaces.WorkFailureFamilyThrottle,
				Type:   interfaces.WorkFailureTypeThrottled,
			},
		},
		{
			Provider:   "claude",
			Model:      "claude-sonnet",
			OccurredAt: now.Add(-4 * time.Minute),
			FailureMetadata: &interfaces.WorkFailureMetadata{
				Family: interfaces.WorkFailureFamilyThrottle,
				Type:   interfaces.WorkFailureTypeThrottled,
			},
		},
	}, 30*time.Minute, now)

	if len(pauses) != 2 {
		t.Fatalf("pause count = %d, want 2", len(pauses))
	}
	if pauses[0].LaneID != "claude" || pauses[1].LaneID != "claude/claude-sonnet" {
		t.Fatalf("lane IDs = %#v, want provider-only and provider/model lanes", pauses)
	}
}

func TestDeriveActiveThrottlePauses_IgnoresRetryableNonThrottleFailures(t *testing.T) {
	now := time.Date(2026, time.May, 1, 12, 0, 0, 0, time.UTC)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{{
		Provider:   "claude",
		Model:      "claude-sonnet",
		OccurredAt: now.Add(-5 * time.Minute),
		FailureMetadata: &interfaces.WorkFailureMetadata{
			Family: interfaces.WorkFailureFamilyRetryable,
			Type:   interfaces.WorkFailureTypeInternalServerError,
		},
	}}, 30*time.Minute, now)

	if len(pauses) != 0 {
		t.Fatalf("pause count = %d, want 0", len(pauses))
	}
}
