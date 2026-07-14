package responseeventstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
)

const (
	backpressureFloodCount          = 200
	backpressurePublishLatencyBudget = 3 * time.Second
	backpressureScenarioTimeout     = 10 * time.Second
)

func TestSessionResponseEventStore_BackpressureFloodWithBlockedAndDrainingSubscribers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping backpressure stress test in short mode")
	}

	start := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	clock := &fixedClock{now: start}
	store, err := responseeventstore.NewSessionResponseEventStoreWithClockAndLimits(
		"session-backpressure-stress",
		clock,
		responseeventstore.RetentionLimits{MaxEvents: 16, MaxBytes: 64 * 1024},
	)
	if err != nil {
		t.Fatalf("NewSessionResponseEventStoreWithClockAndLimits: %v", err)
	}

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

	blockedWaiting := make(chan struct{})
	blockedRead := make(chan struct{})
	go func() {
		close(blockedWaiting)
		events, err := blockedSub.Next(scenarioCtx)
		if err != nil {
			return
		}
		if len(events) > 0 {
			close(blockedRead)
		}
	}()

	select {
	case <-blockedWaiting:
	case <-scenarioCtx.Done():
		t.Fatal("blocked subscriber did not start waiting")
	}

	var (
		drainedMu        sync.Mutex
		drainedSequences []int64
		drainTerminal    error
	)
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for {
			events, err := drainingSub.Next(scenarioCtx)
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

	publishDone := make(chan struct{})
	publishErr := make(chan error, 1)
	publishStarted := time.Now()
	go func() {
		defer close(publishDone)
		for i := 0; i < backpressureFloodCount; i++ {
			if _, err := store.Publish(deltaPublishInput(i)); err != nil {
				publishErr <- err
				return
			}
		}
		publishErr <- nil
	}()

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

	select {
	case <-drainDone:
	case <-time.After(5 * time.Second):
		t.Fatal("draining subscriber did not terminate after Complete")
	}
	if !errors.Is(drainTerminal, responseeventstore.ErrSubscriptionClosed) {
		t.Fatalf("draining terminal error = %v, want ErrSubscriptionClosed", drainTerminal)
	}

	drainedMu.Lock()
	sequences := append([]int64(nil), drainedSequences...)
	drainedMu.Unlock()
	assertAscendingSequences(t, sequences)

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

	clock.now = start.Add(responseeventstore.CompletedStreamRetentionWindow)
	if _, err := store.Subscribe(0); !errors.Is(err, responseeventstore.ErrStoreExpired) {
		t.Fatalf("Subscribe after retention expiry = %v, want ErrStoreExpired", err)
	}
	if events := store.Events(); len(events) == 0 {
		t.Fatal("retained snapshot empty after stress flood and expiry")
	}

	drainingSub.Detach()
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
