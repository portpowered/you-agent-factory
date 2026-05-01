package throttle

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func TestDeriveActiveThrottlePauses_DerivesDeterministicActiveWindows(t *testing.T) {
	now := time.Date(2026, time.May, 1, 18, 0, 0, 0, time.UTC)
	earlier := now.Add(-20 * time.Minute)
	later := now.Add(-10 * time.Minute)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{
		{
			Provider:   "zeta",
			OccurredAt: later,
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
		},
		{
			Provider:   "alpha",
			Model:      "sonnet",
			OccurredAt: earlier,
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
		},
		{
			Provider:   "alpha",
			Model:      "sonnet",
			OccurredAt: later,
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyThrottle,
				Type:   interfaces.ProviderErrorTypeThrottled,
			},
		},
		{
			Provider:   "beta",
			Model:      "gpt-5.4",
			OccurredAt: later,
			ProviderFailure: &interfaces.ProviderFailureMetadata{
				Family: interfaces.ProviderErrorFamilyRetryable,
				Type:   interfaces.ProviderErrorTypeInternalServerError,
			},
		},
	}, 30*time.Minute, now)

	if len(pauses) != 2 {
		t.Fatalf("active pause count = %d, want 2", len(pauses))
	}
	if pauses[0].LaneID != "alpha/sonnet" || pauses[0].Provider != "alpha" || pauses[0].Model != "sonnet" {
		t.Fatalf("pause[0] = %#v, want alpha/sonnet lane first", pauses[0])
	}
	if !pauses[0].PausedAt.Equal(earlier) {
		t.Fatalf("pause[0].PausedAt = %s, want %s", pauses[0].PausedAt, earlier)
	}
	if !pauses[0].PausedUntil.Equal(later.Add(30 * time.Minute)) {
		t.Fatalf("pause[0].PausedUntil = %s, want %s", pauses[0].PausedUntil, later.Add(30*time.Minute))
	}
	if pauses[1].LaneID != "zeta" || pauses[1].Provider != "zeta" || pauses[1].Model != "" {
		t.Fatalf("pause[1] = %#v, want provider-only zeta lane", pauses[1])
	}
}

func TestDeriveActiveThrottlePauses_OmitsExpiredWindows(t *testing.T) {
	now := time.Date(2026, time.May, 1, 18, 0, 0, 0, time.UTC)

	pauses := DeriveActiveThrottlePauses([]FailureRecord{
		{
			Provider:   "claude",
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
