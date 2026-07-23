package responseeventstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
)

func decodeGap(t *testing.T, event responseevents.FactoryResponseEvent) responseevents.StreamGapPayload {
	t.Helper()
	if err := responseevents.ValidateEvent(event); err != nil {
		t.Fatalf("gap event validation: %v", err)
	}
	if event.Kind != responseevents.KindStreamGap || event.Phase != responseevents.PhaseUpdated {
		t.Fatalf("first event = %s/%s, want STREAM_GAP/UPDATED", event.Kind, event.Phase)
	}
	if event.Sequence != 0 {
		t.Fatalf("gap sequence = %d, want out-of-band sequence 0", event.Sequence)
	}
	if event.EventID == "" {
		t.Fatal("gap event ID is empty")
	}
	if event.Provenance.Fidelity != responseevents.FidelityLossy ||
		event.Provenance.Delivery != responseevents.DeliverySynthesized {
		t.Fatalf("gap provenance = %#v, want synthesized lossy", event.Provenance)
	}
	var payload responseevents.StreamGapPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode gap payload: %v", err)
	}
	if payload.Reason != "retention_window" {
		t.Fatalf("gap reason = %q, want retention_window", payload.Reason)
	}
	if payload.FirstAvailableSequence <= 0 {
		t.Fatalf("gap first available sequence = %d, want positive sequence", payload.FirstAvailableSequence)
	}
	return payload
}

func TestSessionResponseEventStoreSubscription_StaleCursorGetsExactGapBeforeOrderedCatchUp(t *testing.T) {
	t.Parallel()

	store := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 2, MaxBytes: generousByteLimit})
	published := publishRetentionEvents(t, store,
		retentionEvent(responseevents.KindMessage, responseevents.PhaseDelta, "dropped-1"),
		retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, "final-2"),
		retentionEvent(responseevents.KindProgress, responseevents.PhaseUpdated, "dropped-3"),
		retentionEvent(responseevents.KindTool, responseevents.PhaseFailed, "failure-4"),
		retentionEvent(responseevents.KindMessage, responseevents.PhaseDelta, "dropped-5"),
	)

	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()
	events, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("event count = %d, want gap plus two retained events", len(events))
	}
	gap := decodeGap(t, events[0])
	if gap.FromSequence != 1 || gap.ToSequence != 5 {
		t.Fatalf("gap = %#v, want dropped bounds [1,5]", gap)
	}
	if gap.FirstAvailableSequence != published[1].Sequence {
		t.Fatalf("gap first available sequence = %d, want %d", gap.FirstAvailableSequence, published[1].Sequence)
	}
	if !reflect.DeepEqual(events[1:], []responseevents.FactoryResponseEvent{published[1], published[3]}) {
		t.Fatalf("catch-up = %#v, want retained original envelopes", events[1:])
	}
}

func TestSessionResponseEventStoreSubscription_GapBoundsClipToCursorAndCurrentCursorHasNoGap(t *testing.T) {
	t.Parallel()

	store := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 2, MaxBytes: generousByteLimit})
	publishRetentionEvents(t, store,
		retentionEvent(responseevents.KindMessage, responseevents.PhaseDelta, "drop-1"),
		retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, "keep-2"),
		retentionEvent(responseevents.KindProgress, responseevents.PhaseUpdated, "drop-3"),
		retentionEvent(responseevents.KindTool, responseevents.PhaseFailed, "keep-4"),
		retentionEvent(responseevents.KindMessage, responseevents.PhaseDelta, "drop-5"),
	)
	store.Complete()

	for _, test := range []struct {
		after    int64
		wantFrom int64
		wantTo   int64
	}{
		{after: 1, wantFrom: 3, wantTo: 5},
		{after: 3, wantFrom: 5, wantTo: 5},
	} {
		subscription, err := store.Subscribe(test.after)
		if err != nil {
			t.Fatalf("Subscribe(%d): %v", test.after, err)
		}
		events, err := subscription.Next(context.Background())
		if err != nil {
			t.Fatalf("Next(%d): %v", test.after, err)
		}
		gap := decodeGap(t, events[0])
		if gap.FromSequence != test.wantFrom || gap.ToSequence != test.wantTo {
			t.Fatalf("after %d gap = %#v, want [%d,%d]", test.after, gap, test.wantFrom, test.wantTo)
		}
		subscription.Detach()
	}

	current, err := store.Subscribe(5)
	if err != nil {
		t.Fatalf("Subscribe(current): %v", err)
	}
	if _, err := current.Next(context.Background()); !errors.Is(err, responseeventstore.ErrSubscriptionClosed) {
		t.Fatalf("current cursor Next error = %v, want closed without a gap", err)
	}
}

func TestSessionResponseEventStoreSubscription_GapOnlyReadAdvancesThenContinuesLive(t *testing.T) {
	t.Parallel()

	store := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 4, MaxBytes: generousByteLimit})
	oversized := publishRetentionEvents(t, store,
		retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, "oversized"),
	)[0]
	size, err := responseeventstore.SerializedEventSize(oversized)
	if err != nil {
		t.Fatalf("SerializedEventSize: %v", err)
	}
	if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 4, MaxBytes: size - 1}); err != nil {
		t.Fatalf("SetRetentionLimits(drop): %v", err)
	}

	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()
	first, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(gap): %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("gap-only count = %d, want 1", len(first))
	}
	gap := decodeGap(t, first[0])
	if gap.FromSequence != oversized.Sequence || gap.ToSequence != oversized.Sequence {
		t.Fatalf("gap = %#v, want oversized sequence %d", gap, oversized.Sequence)
	}
	if gap.FirstAvailableSequence != oversized.Sequence+1 {
		t.Fatalf("gap first available sequence = %d, want next publish sequence %d", gap.FirstAvailableSequence, oversized.Sequence+1)
	}

	if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 4, MaxBytes: generousByteLimit}); err != nil {
		t.Fatalf("SetRetentionLimits(grow): %v", err)
	}
	live := publishRetentionEvents(t, store,
		retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, "live-final"),
	)[0]
	second, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next(live): %v", err)
	}
	if !reflect.DeepEqual(second, []responseevents.FactoryResponseEvent{live}) {
		t.Fatalf("second read = %#v, want live event once without repeated gap", second)
	}
}

func TestSessionResponseEventStoreSubscription_LateReaderReconstructsFinalSemanticState(t *testing.T) {
	t.Parallel()

	store := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 3, MaxBytes: generousByteLimit})
	finals := publishRetentionEvents(t, store,
		retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, "final answer"),
		retentionEvent(responseevents.KindTool, responseevents.PhaseFailed, "failed tool"),
		retentionEvent(responseevents.KindRun, responseevents.PhaseCompleted, "completed"),
	)
	for index := range 40 {
		publishRetentionEvents(t, store, retentionEvent(
			responseevents.KindMessage,
			responseevents.PhaseDelta,
			fmt.Sprintf("heavy-delta-%02d", index),
		))
	}

	subscription, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Detach()
	events, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	decodeGap(t, events[0])
	if !reflect.DeepEqual(events[1:], finals) {
		t.Fatalf("final catch-up = %#v, want exact semantic snapshots %#v", events[1:], finals)
	}
}

func TestSessionResponseEventStoreSubscription_RepeatedEvictionGapBoundsMatchRemovedLedger(t *testing.T) {
	t.Parallel()

	store := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 7, MaxBytes: generousByteLimit})
	all := make([]responseevents.FactoryResponseEvent, 0, 100)
	for index := range 100 {
		phase := responseevents.PhaseDelta
		if index%11 == 0 {
			phase = responseevents.PhaseCompleted
		}
		all = append(all, publishRetentionEvents(t, store,
			retentionEvent(responseevents.KindMessage, phase, fmt.Sprintf("event-%03d", index)),
		)[0])
		if index == 50 {
			if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 4, MaxBytes: generousByteLimit}); err != nil {
				t.Fatalf("SetRetentionLimits: %v", err)
			}
		}
	}
	store.Complete()
	retained := make(map[int64]struct{})
	for _, event := range store.Events() {
		retained[event.Sequence] = struct{}{}
	}

	for after := int64(0); after <= store.LatestSequence(); after++ {
		assertGapMatchesRemovedLedger(t, store, all, retained, after)
	}
}

func assertGapMatchesRemovedLedger(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
	all []responseevents.FactoryResponseEvent,
	retained map[int64]struct{},
	after int64,
) {
	t.Helper()
	wantFrom, wantTo, found := removedBoundsAfter(all, retained, after)
	subscription, err := store.Subscribe(after)
	if err != nil {
		t.Fatalf("Subscribe(%d): %v", after, err)
	}
	defer subscription.Detach()
	events, err := subscription.Next(context.Background())
	if !found {
		if err == nil && len(events) > 0 && events[0].Kind == responseevents.KindStreamGap {
			t.Fatalf("after %d received unexpected gap %#v", after, events[0])
		}
		return
	}
	if err != nil {
		t.Fatalf("Next(%d): %v", after, err)
	}
	gap := decodeGap(t, events[0])
	if gap.FromSequence != wantFrom || gap.ToSequence != wantTo {
		t.Fatalf("after %d gap = [%d,%d], actual removed bounds = [%d,%d]", after, gap.FromSequence, gap.ToSequence, wantFrom, wantTo)
	}
}

func removedBoundsAfter(
	all []responseevents.FactoryResponseEvent,
	retained map[int64]struct{},
	after int64,
) (int64, int64, bool) {
	from, to, found := int64(0), int64(0), false
	for _, event := range all {
		if event.Sequence <= after {
			continue
		}
		if _, ok := retained[event.Sequence]; ok {
			continue
		}
		if !found {
			from = event.Sequence
			found = true
		}
		to = event.Sequence
	}
	return from, to, found
}

func TestSessionResponseEventStoreSubscription_ConcurrentPublishEvictReadAndClose(t *testing.T) {
	store := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 8, MaxBytes: generousByteLimit})
	const readerCount = 6
	subscriptions := make([]*responseeventstore.Subscription, 0, readerCount)
	for range readerCount {
		subscription, err := store.Subscribe(0)
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		subscriptions = append(subscriptions, subscription)
	}

	var readers sync.WaitGroup
	readers.Add(len(subscriptions))
	for _, subscription := range subscriptions {
		go func(subscription *responseeventstore.Subscription) {
			defer readers.Done()
			lastSequence := int64(0)
			for {
				events, err := subscription.Next(context.Background())
				if errors.Is(err, responseeventstore.ErrSubscriptionClosed) {
					return
				}
				if err != nil {
					t.Errorf("Next: %v", err)
					return
				}
				for index, event := range events {
					if event.Kind == responseevents.KindStreamGap {
						if index != 0 {
							t.Errorf("gap index = %d, want first", index)
						}
						continue
					}
					if event.Sequence <= lastSequence {
						t.Errorf("sequence %d delivered after %d", event.Sequence, lastSequence)
					}
					lastSequence = event.Sequence
				}
			}
		}(subscription)
	}

	var publishers sync.WaitGroup
	for index := range 80 {
		publishers.Add(1)
		go func(index int) {
			defer publishers.Done()
			_, err := store.Publish(retentionEvent(
				responseevents.KindMessage,
				responseevents.PhaseDelta,
				fmt.Sprintf("concurrent-%03d", index),
			))
			if err != nil {
				t.Errorf("Publish(%d): %v", index, err)
			}
		}(index)
	}
	publishers.Wait()
	store.Complete()
	go store.Close()

	done := make(chan struct{})
	go func() {
		readers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for concurrent readers to drain")
	}
	assertExactRetentionAccounting(t, store)
}
