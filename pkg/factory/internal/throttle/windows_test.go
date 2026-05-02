package throttle

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestDeriveActiveThrottlePauses_CreatesOneActiveLaneForThrottleFailure(t *testing.T) {
	now := time.Date(2026, time.May, 1, 18, 0, 0, 0, time.UTC)
	occurredAt := now.Add(-10 * time.Minute)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{
		{
			Provider:   "anthropic",
			Model:      "sonnet",
			OccurredAt: occurredAt,
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
		},
	}, 30*time.Minute, now)

	if len(pauses) != 1 {
		t.Fatalf("active pause count = %d, want 1", len(pauses))
	}
	if pauses[0].LaneID != "anthropic/sonnet" || pauses[0].Provider != "anthropic" || pauses[0].Model != "sonnet" {
		t.Fatalf("pause[0] = %#v, want anthropic/sonnet lane", pauses[0])
	}
	if !pauses[0].PausedAt.Equal(occurredAt) {
		t.Fatalf("pause[0].PausedAt = %s, want %s", pauses[0].PausedAt, occurredAt)
	}
	if !pauses[0].PausedUntil.Equal(occurredAt.Add(30 * time.Minute)) {
		t.Fatalf("pause[0].PausedUntil = %s, want %s", pauses[0].PausedUntil, occurredAt.Add(30*time.Minute))
	}
}

func TestDeriveActiveThrottlePauses_LaterFailureExtendsLaneExpiry(t *testing.T) {
	now := time.Date(2026, time.May, 1, 18, 0, 0, 0, time.UTC)
	earlier := now.Add(-20 * time.Minute)
	later := now.Add(-10 * time.Minute)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{
		{
			Provider:   "anthropic",
			Model:      "sonnet",
			OccurredAt: earlier,
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
		},
		{
			Provider:   "anthropic",
			Model:      "sonnet",
			OccurredAt: later,
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
		},
	}, 30*time.Minute, now)

	if len(pauses) != 1 {
		t.Fatalf("active pause count = %d, want 1", len(pauses))
	}
	if !pauses[0].PausedAt.Equal(earlier) {
		t.Fatalf("pause[0].PausedAt = %s, want %s", pauses[0].PausedAt, earlier)
	}
	if !pauses[0].PausedUntil.Equal(later.Add(30 * time.Minute)) {
		t.Fatalf("pause[0].PausedUntil = %s, want %s", pauses[0].PausedUntil, later.Add(30*time.Minute))
	}
}

func TestDeriveActiveThrottlePauses_OmitsExpiredWindows(t *testing.T) {
	now := time.Date(2026, time.May, 1, 18, 0, 0, 0, time.UTC)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{
		{
			Provider:   "anthropic",
			Model:      "sonnet",
			OccurredAt: now.Add(-45 * time.Minute),
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
		},
	}, 30*time.Minute, now)

	if len(pauses) != 0 {
		t.Fatalf("active pause count = %d, want 0", len(pauses))
	}
}

func TestDeriveActiveThrottlePauses_KeepsProviderOnlyAndProviderModelLanesIsolated(t *testing.T) {
	now := time.Date(2026, time.May, 1, 18, 0, 0, 0, time.UTC)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{
		{
			Provider:   "openai",
			OccurredAt: now.Add(-15 * time.Minute),
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
		},
		{
			Provider:   "openai",
			Model:      "gpt-5.4",
			OccurredAt: now.Add(-10 * time.Minute),
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
		},
	}, 30*time.Minute, now)

	if len(pauses) != 2 {
		t.Fatalf("active pause count = %d, want 2", len(pauses))
	}
	if pauses[0].LaneID != "openai" || pauses[0].Model != "" {
		t.Fatalf("pause[0] = %#v, want provider-only openai lane", pauses[0])
	}
	if pauses[1].LaneID != "openai/gpt-5.4" || pauses[1].Model != "gpt-5.4" {
		t.Fatalf("pause[1] = %#v, want model-specific openai/gpt-5.4 lane", pauses[1])
	}
}

func TestDeriveActiveThrottlePauses_IgnoresRetryableNonThrottleFailures(t *testing.T) {
	now := time.Date(2026, time.May, 1, 18, 0, 0, 0, time.UTC)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{
		{
			Provider:   "openai",
			Model:      "gpt-5.4",
			OccurredAt: now.Add(-10 * time.Minute),
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyRetryable,
				Type:   interfaces.ProviderErrorTypeInternalServerError,
			},
		},
	}, 30*time.Minute, now)

	if len(pauses) != 0 {
		t.Fatalf("active pause count = %d, want 0", len(pauses))
	}
}
