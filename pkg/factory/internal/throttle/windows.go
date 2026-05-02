package throttle

import (
	"sort"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// FailureRecord is the normalized runtime history input consumed by the
// throttle-window derivation seam that later guard work can reuse directly.
type FailureRecord struct {
	Provider        string
	Model           string
	OccurredAt      time.Time
	ProviderFailure *interfaces.ProviderFailureMetadata
}

type laneKey struct {
	provider string
	model    string
}

// DeriveActiveThrottlePauses reduces normalized failure history into the set
// of currently active provider/model throttle windows for an explicit clock.
// Dispatcher code still owns the live pause-state map in this lane; this
// helper is the pure internal primitive that later event-history-backed guard
// work can reuse without depending on dispatcher mutation paths.
func DeriveActiveThrottlePauses(history []FailureRecord, pauseDuration time.Duration, now time.Time) []interfaces.ActiveThrottlePause {
	if len(history) == 0 || pauseDuration <= 0 {
		return nil
	}
	ordered := append([]FailureRecord(nil), history...)
	sort.Slice(ordered, func(i, j int) bool {
		if !ordered[i].OccurredAt.Equal(ordered[j].OccurredAt) {
			return ordered[i].OccurredAt.Before(ordered[j].OccurredAt)
		}
		if ordered[i].Provider != ordered[j].Provider {
			return ordered[i].Provider < ordered[j].Provider
		}
		return ordered[i].Model < ordered[j].Model
	})

	windows := make(map[laneKey]interfaces.ActiveThrottlePause)
	for _, record := range ordered {
		if record.Provider == "" {
			continue
		}
		if !workers.ProviderFailureDecisionFromMetadata(record.ProviderFailure).TriggersThrottlePause {
			continue
		}
		key := laneKey{provider: record.Provider, model: record.Model}
		candidate := interfaces.ActiveThrottlePause{
			LaneID:      laneID(key),
			Provider:    record.Provider,
			Model:       record.Model,
			PausedAt:    record.OccurredAt,
			PausedUntil: record.OccurredAt.Add(pauseDuration),
		}
		if existing, ok := windows[key]; ok {
			if existing.PausedUntil.After(record.OccurredAt) {
				candidate.PausedAt = existing.PausedAt
			}
			if existing.PausedUntil.After(candidate.PausedUntil) {
				candidate.PausedUntil = existing.PausedUntil
			}
		}
		windows[key] = candidate
	}

	active := make([]interfaces.ActiveThrottlePause, 0, len(windows))
	for _, pause := range windows {
		if !pause.PausedUntil.After(now) {
			continue
		}
		active = append(active, pause)
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].Provider != active[j].Provider {
			return active[i].Provider < active[j].Provider
		}
		if active[i].Model != active[j].Model {
			return active[i].Model < active[j].Model
		}
		return active[i].LaneID < active[j].LaneID
	})
	return active
}

func laneID(key laneKey) string {
	if key.model == "" {
		return key.provider
	}
	return key.provider + "/" + key.model
}
