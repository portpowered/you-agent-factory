package responseeventstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseeventstore"
)

const (
	backpressureFloodCount           = 200
	backpressurePublishLatencyBudget = 3 * time.Second
	backpressureScenarioTimeout      = 10 * time.Second
	lifecycleRaceWorkerCount         = 36
	lifecycleRacePublishBurst        = 24
)

func TestSessionResponseEventStore_BackpressureFloodWithBlockedAndDrainingSubscribers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping backpressure stress test in short mode")
	}

	start := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	store, clock := newTightRetentionBackpressureStore(t, "session-backpressure-stress", start)

	blockedSub, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe(blocked): %v", err)
	}
	drainingSub, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe(draining): %v", err)
	}

	scenarioCtx, cancelScenario := context.WithTimeout(context.Background(), backpressureScenarioTimeout)
	defer cancelScenario()

	blockedWaiting, blockedRead := startBlockedSubscriberGoroutine(scenarioCtx, blockedSub)
	select {
	case <-blockedWaiting:
	case <-scenarioCtx.Done():
		t.Fatal("blocked subscriber did not start waiting")
	}

	drain := startDrainingSubscriberGoroutine(scenarioCtx, drainingSub)
	publishErr, publishStarted := floodPublishDeltasAsync(store)
	waitPublishFloodWithinBudget(t, store, publishErr, scenarioCtx, publishStarted)

	select {
	case <-blockedRead:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocked subscriber never received the first retained batch")
	}

	store.Complete()
	if !store.Completed() {
		t.Fatal("Completed() = false after Complete during backpressure scenario")
	}

	detachStarted := time.Now()
	blockedSub.Detach()
	if elapsed := time.Since(detachStarted); elapsed > 200*time.Millisecond {
		t.Fatalf("blocked Detach took %v, want prompt return without hanging publisher", elapsed)
	}

	assertDrainingSubscriberClosed(t, drain)
	assertBackpressureResumeAndExpiry(t, store, clock, start, drainingSub)
}

func TestSessionResponseEventStore_BackpressureFloodExactGapsAndFinalRetention(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping backpressure gap/final retention stress test in short mode")
	}

	start := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	store, _ := newTightRetentionBackpressureStore(t, "session-backpressure-gap-finals", start)

	finals := publishRetentionEvents(t, store,
		retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, "final-answer"),
		retentionEvent(responseevents.KindTool, responseevents.PhaseFailed, "failed-tool"),
		retentionEvent(responseevents.KindRun, responseevents.PhaseCompleted, "run-completed"),
	)

	staleSub, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe(stale): %v", err)
	}
	defer staleSub.Detach()

	allPublished, blockedSub := publishFinalsAndFloodDeltas(t, store, scenarioCtxForBackpressure(t), finals)
	blockedSub.Detach()

	assertFinalsSurvivePressure(t, store, finals)
	assertStaleCursorExactGap(t, staleSub, store, allPublished)
	assertExactRetentionAccounting(t, store)
}

// TestSessionResponseEventStore_BackpressureLifecycleConcurrentRace interleaves
// subscribe, publish bursts, complete, detach, retention-window expiry, and close
// under retention-limited flood pressure so overlapping lifecycle work stays
// race-clean with bounded CI timeouts.
//
// Soak locally or in optional CI lanes with:
//
//	go test -race -count=5 ./pkg/services/factory_sessions/responseeventstore/ \
//	  -run BackpressureLifecycleConcurrentRace -timeout 120s
func TestSessionResponseEventStore_BackpressureLifecycleConcurrentRace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping lifecycle concurrent race stress test in short mode")
	}

	start := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC)
	store, clock := newTightRetentionBackpressureStore(t, "session-lifecycle-race", start)

	scenarioCtx, cancelScenario := context.WithTimeout(context.Background(), backpressureScenarioTimeout)
	defer cancelScenario()

	waitBackpressureLifecycleRaceWorkers(t, store, clock, start, scenarioCtx)

	if got := store.SubscriberCount(); got != 0 {
		t.Fatalf("subscriber count after lifecycle race = %d, want 0", got)
	}
	if events := store.Events(); store.LatestSequence() > 0 && len(events) == 0 {
		t.Fatal("retained snapshot empty after lifecycle race despite published events")
	}
}

func newTightRetentionBackpressureStore(
	t *testing.T,
	sessionID string,
	start time.Time,
) (*responseeventstore.SessionResponseEventStore, *fixedClock) {
	t.Helper()
	clock := &fixedClock{now: start}
	store, err := responseeventstore.NewSessionResponseEventStoreWithClockAndLimits(
		sessionID,
		clock,
		responseeventstore.RetentionLimits{MaxEvents: 16, MaxBytes: 64 * 1024},
		testResponseEventID,
	)
	if err != nil {
		t.Fatalf("NewSessionResponseEventStoreWithClockAndLimits: %v", err)
	}
	return store, clock
}

func scenarioCtxForBackpressure(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backpressureScenarioTimeout)
	t.Cleanup(cancel)
	return ctx
}

func startBlockedSubscriberGoroutine(
	scenarioCtx context.Context,
	sub *responseeventstore.Subscription,
) (blockedWaiting <-chan struct{}, blockedRead <-chan struct{}) {
	blockedWaitingCh := make(chan struct{})
	blockedReadCh := make(chan struct{})
	go func() {
		close(blockedWaitingCh)
		events, err := sub.Next(scenarioCtx)
		if err != nil {
			return
		}
		if len(events) > 0 {
			close(blockedReadCh)
		}
	}()
	return blockedWaitingCh, blockedReadCh
}

type drainingSubscriberHarness struct {
	done      <-chan struct{}
	sequences *[]int64
	terminal  *error
}

func startDrainingSubscriberGoroutine(
	scenarioCtx context.Context,
	sub *responseeventstore.Subscription,
) drainingSubscriberHarness {
	var (
		drainedMu        sync.Mutex
		drainedSequences []int64
		drainTerminal    error
	)
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for {
			events, err := sub.Next(scenarioCtx)
			if err != nil {
				drainTerminal = err
				return
			}
			drainedMu.Lock()
			for _, event := range events {
				if event.Sequence > 0 {
					drainedSequences = append(drainedSequences, event.Sequence)
				}
			}
			drainedMu.Unlock()
		}
	}()
	return drainingSubscriberHarness{
		done:      drainDone,
		sequences: &drainedSequences,
		terminal:  &drainTerminal,
	}
}

func floodPublishDeltasAsync(
	store *responseeventstore.SessionResponseEventStore,
) (<-chan error, time.Time) {
	publishErr := make(chan error, 1)
	publishStarted := time.Now()
	go func() {
		for i := 0; i < backpressureFloodCount; i++ {
			if _, err := store.Publish(deltaPublishInput(i)); err != nil {
				publishErr <- err
				return
			}
		}
		publishErr <- nil
	}()
	return publishErr, publishStarted
}

func waitPublishFloodWithinBudget(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
	publishErr <-chan error,
	scenarioCtx context.Context,
	publishStarted time.Time,
) {
	t.Helper()
	select {
	case err := <-publishErr:
		if err != nil {
			t.Fatalf("publish flood: %v", err)
		}
	case <-time.After(backpressurePublishLatencyBudget):
		t.Fatalf("publisher exceeded bounded latency budget %v", backpressurePublishLatencyBudget)
	case <-scenarioCtx.Done():
		t.Fatalf("scenario timed out before publish flood finished: %v", scenarioCtx.Err())
	}

	publishElapsed := time.Since(publishStarted)
	if publishElapsed > backpressurePublishLatencyBudget {
		t.Fatalf("publish elapsed = %v, want <= %v", publishElapsed, backpressurePublishLatencyBudget)
	}
	if got := store.LatestSequence(); got != backpressureFloodCount {
		t.Fatalf("latest sequence = %d, want %d published deltas", got, backpressureFloodCount)
	}
}

func assertDrainingSubscriberClosed(t *testing.T, drain drainingSubscriberHarness) {
	t.Helper()
	select {
	case <-drain.done:
	case <-time.After(5 * time.Second):
		t.Fatal("draining subscriber did not terminate after Complete")
	}
	if !errors.Is(*drain.terminal, responseeventstore.ErrSubscriptionClosed) {
		t.Fatalf("draining terminal error = %v, want ErrSubscriptionClosed", *drain.terminal)
	}
	assertAscendingSequences(t, *drain.sequences)
}

func assertBackpressureResumeAndExpiry(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
	clock *fixedClock,
	start time.Time,
	drainingSub *responseeventstore.Subscription,
) {
	t.Helper()
	resumeSub, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe(resume) before expiry: %v", err)
	}
	resumeEvents, err := resumeSub.Drain()
	if err != nil {
		t.Fatalf("Drain(resume) before expiry: %v", err)
	}
	if len(resumeEvents) == 0 {
		t.Fatal("resume subscription drained zero events before expiry")
	}
	resumeSub.Detach()

	clock.Set(start.Add(responseeventstore.CompletedStreamRetentionWindow))
	if _, err := store.Subscribe(0); !errors.Is(err, responseeventstore.ErrStoreExpired) {
		t.Fatalf("Subscribe after retention expiry = %v, want ErrStoreExpired", err)
	}
	if events := store.Events(); len(events) == 0 {
		t.Fatal("retained snapshot empty after stress flood and expiry")
	}

	drainingSub.Detach()
}

func publishFinalsAndFloodDeltas(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
	scenarioCtx context.Context,
	finals []responseevents.FactoryResponseEvent,
) ([]responseevents.FactoryResponseEvent, *responseeventstore.Subscription) {
	t.Helper()
	blockedSub, err := store.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe(blocked): %v", err)
	}

	blockedWaiting, _ := startBlockedSubscriberGoroutine(scenarioCtx, blockedSub)
	select {
	case <-blockedWaiting:
	case <-scenarioCtx.Done():
		t.Fatal("blocked subscriber did not start waiting")
	}

	allPublished := append([]responseevents.FactoryResponseEvent{}, finals...)
	publishStarted := time.Now()
	for i := 0; i < backpressureFloodCount; i++ {
		event, err := store.Publish(deltaPublishInput(i))
		if err != nil {
			t.Fatalf("publish flood delta %d: %v", i, err)
		}
		allPublished = append(allPublished, event)
	}
	if elapsed := time.Since(publishStarted); elapsed > backpressurePublishLatencyBudget {
		t.Fatalf("publish elapsed = %v, want <= %v", elapsed, backpressurePublishLatencyBudget)
	}

	store.Complete()
	return allPublished, blockedSub
}

func assertFinalsSurvivePressure(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
	finals []responseevents.FactoryResponseEvent,
) {
	t.Helper()
	retainedSet := make(map[int64]struct{}, len(store.Events()))
	for _, event := range store.Events() {
		retainedSet[event.Sequence] = struct{}{}
	}
	for _, final := range finals {
		if _, ok := retainedSet[final.Sequence]; !ok {
			t.Fatalf("final event seq=%d kind=%s phase=%s evicted under flood pressure", final.Sequence, final.Kind, final.Phase)
		}
	}
}

func assertStaleCursorExactGap(
	t *testing.T,
	staleSub *responseeventstore.Subscription,
	store *responseeventstore.SessionResponseEventStore,
	allPublished []responseevents.FactoryResponseEvent,
) {
	t.Helper()
	retainedBySequence := make(map[int64]responseevents.FactoryResponseEvent, len(store.Events()))
	retainedSet := make(map[int64]struct{}, len(store.Events()))
	for _, event := range store.Events() {
		retainedBySequence[event.Sequence] = event
		retainedSet[event.Sequence] = struct{}{}
	}

	wantFrom, wantTo, wantGap := removedBoundsAfter(allPublished, retainedSet, 0)
	if !wantGap {
		t.Fatal("stale cursor after flood: want retention gap, got none")
	}

	readCtx, cancelRead := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelRead()
	events, err := staleSub.Next(readCtx)
	if err != nil {
		t.Fatalf("stale Next after flood: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("stale read count = %d, want gap plus retained catch-up", len(events))
	}

	gap := decodeGap(t, events[0])
	if gap.FromSequence != wantFrom || gap.ToSequence != wantTo {
		t.Fatalf("gap = [%d,%d], want exact removed bounds [%d,%d]", gap.FromSequence, gap.ToSequence, wantFrom, wantTo)
	}
	wantFirstAvailable := firstRetainedSequenceAfter(store.Events(), 0)
	if gap.FirstAvailableSequence != wantFirstAvailable {
		t.Fatalf("gap first available sequence = %d, want %d", gap.FirstAvailableSequence, wantFirstAvailable)
	}

	wantCatchUp := retainedCatchUpAfter(allPublished, retainedBySequence, 0)
	if !reflect.DeepEqual(events[1:], wantCatchUp) {
		t.Fatalf("stale catch-up = %#v, want retained originals %#v", events[1:], wantCatchUp)
	}
}

func waitBackpressureLifecycleRaceWorkers(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
	clock *fixedClock,
	start time.Time,
	scenarioCtx context.Context,
) {
	t.Helper()
	startWorkers := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(lifecycleRaceWorkerCount)
	for workerID := 0; workerID < lifecycleRaceWorkerCount; workerID++ {
		go func(id int) {
			defer wg.Done()
			<-startWorkers
			runBackpressureLifecycleRaceWorker(t, id, store, clock, start, scenarioCtx)
		}(workerID)
	}
	close(startWorkers)

	workersDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(workersDone)
	}()

	select {
	case <-workersDone:
	case <-scenarioCtx.Done():
		t.Fatalf("lifecycle concurrent race timed out: %v", scenarioCtx.Err())
	}
}

func runBackpressureLifecycleRaceWorker(
	t *testing.T,
	id int,
	store *responseeventstore.SessionResponseEventStore,
	clock *fixedClock,
	start time.Time,
	scenarioCtx context.Context,
) {
	workerCtx, cancelWorker := context.WithTimeout(scenarioCtx, 5*time.Second)
	defer cancelWorker()

	switch id % 6 {
	case 0:
		lifecycleRaceDrainWorker(store, workerCtx)
	case 1:
		lifecycleRaceSubscribeDetachWorker(store)
	case 2:
		lifecycleRaceBlockedDetachWorker(t, store, workerCtx)
	case 3, 4:
		lifecycleRacePublishBurstWorker(t, store, id)
	case 5:
		lifecycleRaceCompleteExpireCloseWorker(t, store, clock, start, id)
	}
}

func lifecycleRaceDrainWorker(
	store *responseeventstore.SessionResponseEventStore,
	workerCtx context.Context,
) {
	subscription, err := store.Subscribe(0)
	if err != nil {
		return
	}
	for {
		_, err := subscription.Next(workerCtx)
		if err != nil {
			subscription.Detach()
			return
		}
	}
}

func lifecycleRaceSubscribeDetachWorker(store *responseeventstore.SessionResponseEventStore) {
	subscription, err := store.Subscribe(0)
	if err != nil {
		return
	}
	subscription.Detach()
}

func lifecycleRaceBlockedDetachWorker(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
	workerCtx context.Context,
) {
	subscription, err := store.Subscribe(0)
	if err != nil {
		return
	}
	blockedDone := make(chan struct{})
	go func() {
		defer close(blockedDone)
		_, _ = subscription.Next(workerCtx)
	}()
	select {
	case <-blockedDone:
	case <-time.After(50 * time.Millisecond):
	}
	subscription.Detach()
	select {
	case <-blockedDone:
	case <-time.After(500 * time.Millisecond):
		t.Errorf("blocked subscriber did not return promptly after Detach")
	}
}

func lifecycleRacePublishBurstWorker(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
	id int,
) {
	for burst := 0; burst < lifecycleRacePublishBurst; burst++ {
		if _, err := store.Publish(deltaPublishInput(id*lifecycleRacePublishBurst + burst)); err != nil {
			if !errors.Is(err, responseeventstore.ErrStoreCompleted) &&
				!errors.Is(err, responseeventstore.ErrStoreClosed) {
				t.Errorf("publish burst worker=%d index=%d: %v", id, burst, err)
			}
			return
		}
	}
}

func lifecycleRaceCompleteExpireCloseWorker(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
	clock *fixedClock,
	start time.Time,
	id int,
) {
	time.Sleep(time.Duration(id%7) * time.Millisecond)
	store.Complete()
	if store.Completed() {
		clock.Set(start.Add(responseeventstore.CompletedStreamRetentionWindow))
		if _, err := store.Subscribe(0); err != nil &&
			!errors.Is(err, responseeventstore.ErrStoreExpired) &&
			!errors.Is(err, responseeventstore.ErrStoreClosed) {
			t.Errorf("Subscribe after retention expiry = %v, want ErrStoreExpired or ErrStoreClosed", err)
		}
	}
	store.Close()
}

func firstRetainedSequenceAfter(events []responseevents.FactoryResponseEvent, after int64) int64 {
	for _, event := range events {
		if event.Sequence > after {
			return event.Sequence
		}
	}
	return 0
}

func retainedCatchUpAfter(
	all []responseevents.FactoryResponseEvent,
	retained map[int64]responseevents.FactoryResponseEvent,
	after int64,
) []responseevents.FactoryResponseEvent {
	catchUp := make([]responseevents.FactoryResponseEvent, 0, len(retained))
	for _, event := range all {
		if event.Sequence <= after {
			continue
		}
		if kept, ok := retained[event.Sequence]; ok {
			catchUp = append(catchUp, kept)
		}
	}
	return catchUp
}

func deltaPublishInput(index int) responseevents.FactoryResponseEvent {
	input := samplePublishInput()
	input.Phase = responseevents.PhaseDelta
	input.Payload = json.RawMessage(fmt.Sprintf(
		`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":%q}`,
		"delta-"+strconv.Itoa(index),
	))
	return input
}

func assertAscendingSequences(t *testing.T, sequences []int64) {
	t.Helper()
	if len(sequences) == 0 {
		t.Fatal("drained sequences = empty, want ordered live delivery")
	}
	for i := 1; i < len(sequences); i++ {
		if sequences[i] <= sequences[i-1] {
			t.Fatalf("drained sequences = %v, want strictly ascending order", sequences)
		}
	}
}
