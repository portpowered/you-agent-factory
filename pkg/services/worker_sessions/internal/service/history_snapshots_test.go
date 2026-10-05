package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func historySnapshotFixtures() []workersessions.Observation {
	return []workersessions.Observation{
		{WorkerSessionID: "a", WorkIDs: []string{"work-a"}},
		{WorkerSessionID: "b", WorkIDs: []string{"work-b"}, StreamGenerationID: "generation", StateSequence: 7, StateSequenceKnown: true},
		{WorkerSessionID: "c", WorkIDs: []string{"work-c"}},
	}
}

func TestHistorySnapshotDetachedReplayAndSignedOffsets(t *testing.T) {
	t.Parallel()
	cache := newObservationSnapshots(new(HistorySnapshotBudget))
	now := time.Unix(0, 0)
	observations := historySnapshotFixtures()
	first, err := cache.first(observations, "filter", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	observations[1].WorkIDs[0] = "source mutation"
	second, err := cache.next(first.NextToken, "filter", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	got := second.Observations[0]
	if got.WorkIDs[0] != "work-b" || got.StreamGenerationID != "generation" || got.StateSequence != 7 || !got.StateSequenceKnown {
		t.Fatalf("frozen row = %#v", got)
	}
	second.Observations[0].WorkIDs[0] = "caller mutation"
	replay, err := cache.next(first.NextToken, "filter", int(^uint(0)>>1), now)
	if err != nil || len(replay.Observations) != 2 || replay.NextToken != "" || replay.Observations[0].WorkIDs[0] != "work-b" {
		t.Fatalf("replay = %#v, %v", replay, err)
	}
	cursor, err := decodeHistoryCursor(first.NextToken)
	if err != nil {
		t.Fatal(err)
	}
	cursor.Offset++
	data, err := json.Marshal(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cache.next(base64.StdEncoding.EncodeToString(data), "filter", 1, now); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("tampered offset error = %v", err)
	}
}

func TestHistorySnapshotIdleExpiryExtendsOnlyOnSuccessfulRead(t *testing.T) {
	t.Parallel()
	cache := newObservationSnapshots(new(HistorySnapshotBudget))
	now := time.Unix(0, 0)
	first, err := cache.first(historySnapshotFixtures(), "filter", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cache.next(first.NextToken, "filter", 1, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.next(first.NextToken, "wrong", 1, now.Add(8*time.Minute)); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("wrong filter error = %v", err)
	}
	if _, err := cache.next(first.NextToken, "filter", 1, now.Add(9*time.Minute)); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("expired cursor error = %v", err)
	}
	if len(cache.entries) != 0 || cache.bytes != 0 {
		t.Fatalf("expired entries = %d, bytes = %d", len(cache.entries), cache.bytes)
	}
}

func TestHistorySnapshotCountEvictsLeastRecentlyUsed(t *testing.T) {
	t.Parallel()
	cache := newObservationSnapshots(new(HistorySnapshotBudget))
	now := time.Unix(0, 0)
	tokens := make([]string, 0, historySnapshotCount)
	for range historySnapshotCount {
		page, err := cache.first(historySnapshotFixtures(), "filter", 1, now)
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, page.NextToken)
	}
	if _, err := cache.next(tokens[0], "filter", 1, now); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.first(historySnapshotFixtures(), "filter", 1, now); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.next(tokens[1], "filter", 1, now); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("evicted token error = %v", err)
	}
	if _, err := cache.next(tokens[0], "filter", 1, now); err != nil {
		t.Fatalf("recently read token error = %v", err)
	}
	if len(cache.entries) != historySnapshotCount || cache.bytes > historySnapshotBytes {
		t.Fatalf("cache count = %d, bytes = %d", len(cache.entries), cache.bytes)
	}
}

func TestHistorySnapshotConcurrentReplayHasDetachedPages(t *testing.T) {
	t.Parallel()
	cache := newObservationSnapshots(new(HistorySnapshotBudget))
	now := time.Unix(0, 0)
	first, err := cache.first(historySnapshotFixtures(), "filter", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	var joined sync.WaitGroup
	for range 8 {
		joined.Go(func() {
			page, err := cache.next(first.NextToken, "filter", 1, now)
			if err != nil || len(page.Observations) != 1 || page.Observations[0].WorkIDs[0] != "work-b" {
				t.Errorf("concurrent replay = %#v, %v", page, err)
				return
			}
			page.Observations[0].WorkIDs[0] = "caller mutation"
		})
	}
	joined.Wait()
}

func TestHistorySnapshotBytePressureEvictsBeforeAdmittingNextPage(t *testing.T) {
	t.Parallel()
	cache := newObservationSnapshots(new(HistorySnapshotBudget))
	now := time.Unix(0, 0)
	first, err := cache.first(historySnapshotFixtures(), "filter", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := decodeHistoryCursor(first.NextToken)
	if err != nil {
		t.Fatal(err)
	}
	// Model near-capacity charged storage without introducing capacity-sized
	// fixtures into the component suite. L1 owns actual large-fleet memory proof.
	entry := cache.entries[cursor.ID]
	entry.bytes = historySnapshotBytes - entry.bytes + 1
	cache.bytes = entry.bytes
	fleet := newObservationSnapshots(cache.HistorySnapshotBudget)
	second, err := fleet.first(historySnapshotFixtures(), "filter", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cache.next(first.NextToken, "filter", 1, now); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("evicted byte-pressure cursor error = %v", err)
	}
	if _, err := fleet.next(second.NextToken, "filter", 1, now); err != nil {
		t.Fatal(err)
	}
	if len(cache.entries) != 1 || cache.bytes > historySnapshotBytes {
		t.Fatalf("cache count = %d, bytes = %d", len(cache.entries), cache.bytes)
	}
}

func TestHistorySnapshotSharedBudgetEvictsAcrossViews(t *testing.T) {
	t.Parallel()
	budget := new(HistorySnapshotBudget)
	registry, fleet := newObservationSnapshots(budget), newObservationSnapshots(budget)
	now := time.Unix(0, 0)
	first, err := registry.first(historySnapshotFixtures(), "filter", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fleet.next(first.NextToken, "filter", 1, now); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("foreign view accepted registry cursor: %v", err)
	}
	var oldestFleetToken string
	for range historySnapshotCount - 1 {
		page, err := fleet.first(historySnapshotFixtures(), "filter", 1, now)
		if err != nil {
			t.Fatal(err)
		}
		if oldestFleetToken == "" {
			oldestFleetToken = page.NextToken
		}
	}
	if _, err := registry.next(first.NextToken, "filter", 1, now); err != nil {
		t.Fatal(err)
	}
	if _, err := fleet.first(historySnapshotFixtures(), "filter", 1, now); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.next(first.NextToken, "filter", 1, now); err != nil {
		t.Fatalf("recent registry cursor evicted: %v", err)
	}
	if _, err := fleet.next(oldestFleetToken, "filter", 1, now); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("shared count budget failed to evict oldest fleet cursor: %v", err)
	}

}

func TestHistorySnapshotSharedBudgetKeepsProfilesIsolated(t *testing.T) {
	t.Parallel()
	profile := new(HistorySnapshotBudget)
	registry, fleet := newObservationSnapshots(profile), newObservationSnapshots(profile)
	other := newObservationSnapshots(new(HistorySnapshotBudget))
	now := time.Unix(0, 0)
	page, err := registry.first(historySnapshotFixtures(), "filter", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	for range historySnapshotCount + 1 {
		if _, err := other.first(historySnapshotFixtures(), "filter", 1, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := other.next(page.NextToken, "filter", 1, now); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("foreign profile accepted cursor: %v", err)
	}
	if _, err := registry.next(page.NextToken, "filter", 1, now); err != nil {
		t.Fatalf("other profile pressure evicted cursor: %v", err)
	}
	// Pruning through another view expires the shared idle page.
	if _, err := fleet.first(historySnapshotFixtures(), "filter", 1, now.Add(historySnapshotIdle)); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.next(page.NextToken, "filter", 1, now.Add(historySnapshotIdle)); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("shared idle page still available: %v", err)
	}
}
