package responseeventstore_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
)

const generousByteLimit = 1 << 30

func retentionEvent(kind responseevents.Kind, phase responseevents.Phase, label string) responseevents.FactoryResponseEvent {
	event := samplePublishInput()
	event.Kind = kind
	event.Phase = phase
	event.ItemID = label
	switch kind {
	case responseevents.KindMessage:
		if phase == responseevents.PhaseDelta {
			event.Payload = json.RawMessage(fmt.Sprintf(
				`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":%q}`,
				label,
			))
		} else {
			event.Payload = json.RawMessage(fmt.Sprintf(
				`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":%q}]}`,
				label,
			))
		}
	case responseevents.KindTool:
		event.Payload = json.RawMessage(fmt.Sprintf(
			`{"toolCallId":%q,"toolName":"shell","status":%q}`,
			label,
			phase,
		))
	case responseevents.KindRun:
		event.ItemID = ""
		event.Payload = json.RawMessage(fmt.Sprintf(`{"status":%q}`, phase))
	case responseevents.KindProgress:
		event.Payload = json.RawMessage(fmt.Sprintf(`{"label":%q}`, label))
	default:
		panic("unsupported retention test event kind")
	}
	return event
}

func newRetentionStore(t *testing.T, limits responseeventstore.RetentionLimits) *responseeventstore.SessionResponseEventStore {
	t.Helper()
	store, err := newResponseEventStoreWithLimits("session-abc", limits)
	if err != nil {
		t.Fatalf("NewSessionResponseEventStoreWithLimits: %v", err)
	}
	return store
}

func publishRetentionEvents(
	t *testing.T,
	store *responseeventstore.SessionResponseEventStore,
	inputs ...responseevents.FactoryResponseEvent,
) []responseevents.FactoryResponseEvent {
	t.Helper()
	published := make([]responseevents.FactoryResponseEvent, 0, len(inputs))
	for _, input := range inputs {
		event, err := store.Publish(input)
		if err != nil {
			t.Fatalf("Publish(%s/%s): %v", input.Kind, input.Phase, err)
		}
		published = append(published, event)
	}
	return published
}

func assertExactRetentionAccounting(t *testing.T, store *responseeventstore.SessionResponseEventStore) {
	t.Helper()
	events := store.Events()
	wantBytes := 0
	for _, event := range events {
		size, err := responseeventstore.SerializedEventSize(event)
		if err != nil {
			t.Fatalf("SerializedEventSize: %v", err)
		}
		wantBytes += size
	}
	accounting := store.RetentionAccounting()
	if accounting.EventCount != len(events) || accounting.TotalBytes != wantBytes {
		t.Fatalf("accounting = %#v, want count=%d bytes=%d", accounting, len(events), wantBytes)
	}
	limits := store.RetentionLimits()
	if accounting.EventCount > limits.MaxEvents || accounting.TotalBytes > limits.MaxBytes {
		t.Fatalf("accounting %#v exceeds limits %#v", accounting, limits)
	}
}

func TestSessionResponseEventStore_RejectsNonPositiveRetentionLimits(t *testing.T) {
	t.Parallel()

	for _, limits := range []responseeventstore.RetentionLimits{
		{MaxEvents: 0, MaxBytes: 1},
		{MaxEvents: 1, MaxBytes: 0},
		{MaxEvents: -1, MaxBytes: 1},
		{MaxEvents: 1, MaxBytes: -1},
	} {
		if _, err := newResponseEventStoreWithLimits("session-abc", limits); !errors.Is(err, responseeventstore.ErrInvalidRetentionLimits) {
			t.Fatalf("limits %#v error = %v, want ErrInvalidRetentionLimits", limits, err)
		}
	}

	store := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 3, MaxBytes: 3_000})
	if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 0, MaxBytes: 1}); !errors.Is(err, responseeventstore.ErrInvalidRetentionLimits) {
		t.Fatalf("SetRetentionLimits error = %v, want ErrInvalidRetentionLimits", err)
	}
	if got := store.RetentionLimits(); got != (responseeventstore.RetentionLimits{MaxEvents: 3, MaxBytes: 3_000}) {
		t.Fatalf("limits changed after rejected update: %#v", got)
	}
}

func TestSessionResponseEventStore_CountRetentionUsesSemanticPriorityAndAge(t *testing.T) {
	t.Parallel()

	store := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 8, MaxBytes: generousByteLimit})
	published := publishRetentionEvents(t, store,
		retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, "final-message"),
		retentionEvent(responseevents.KindTool, responseevents.PhaseStarted, "active-tool"),
		retentionEvent(responseevents.KindMessage, responseevents.PhaseDelta, "old-delta"),
		retentionEvent(responseevents.KindProgress, responseevents.PhaseUpdated, "new-progress"),
		retentionEvent(responseevents.KindTool, responseevents.PhaseFailed, "failed-tool"),
		retentionEvent(responseevents.KindRun, responseevents.PhaseCompleted, "run-outcome"),
	)

	if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 3, MaxBytes: generousByteLimit}); err != nil {
		t.Fatalf("SetRetentionLimits: %v", err)
	}
	got := store.Events()
	want := []responseevents.FactoryResponseEvent{published[0], published[4], published[5]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("retained events = %#v, want final semantic events %#v", got, want)
	}
	assertExactRetentionAccounting(t, store)
}

func TestSessionResponseEventStore_ByteRetentionUsesSerializedEnvelopeBoundary(t *testing.T) {
	t.Parallel()

	store := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 8, MaxBytes: generousByteLimit})
	published := publishRetentionEvents(t, store,
		retentionEvent(responseevents.KindMessage, responseevents.PhaseDelta, "a large transient delta that should be removed first"),
		retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, "final"),
		retentionEvent(responseevents.KindProgress, responseevents.PhaseUpdated, "transient-progress"),
	)
	finalBytes, err := responseeventstore.SerializedEventSize(published[1])
	if err != nil {
		t.Fatalf("SerializedEventSize(final): %v", err)
	}

	if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 8, MaxBytes: finalBytes}); err != nil {
		t.Fatalf("SetRetentionLimits: %v", err)
	}
	if got := store.Events(); !reflect.DeepEqual(got, []responseevents.FactoryResponseEvent{published[1]}) {
		t.Fatalf("retained events = %#v, want final event only", got)
	}
	assertExactRetentionAccounting(t, store)
}

func TestSessionResponseEventStore_SimultaneousLimitsAndPriorityTiesAreDeterministic(t *testing.T) {
	t.Parallel()

	store := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 8, MaxBytes: generousByteLimit})
	published := publishRetentionEvents(t, store,
		retentionEvent(responseevents.KindMessage, responseevents.PhaseDelta, "oldest"),
		retentionEvent(responseevents.KindMessage, responseevents.PhaseDelta, "newest"),
		retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, "preferred"),
	)
	preferredBytes, err := responseeventstore.SerializedEventSize(published[2])
	if err != nil {
		t.Fatalf("SerializedEventSize(preferred): %v", err)
	}
	if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 1, MaxBytes: preferredBytes}); err != nil {
		t.Fatalf("SetRetentionLimits: %v", err)
	}
	if got := store.Events(); !reflect.DeepEqual(got, []responseevents.FactoryResponseEvent{published[2]}) {
		t.Fatalf("retained events = %#v, want preferred event only", got)
	}

	tieStore := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 1, MaxBytes: generousByteLimit})
	tiePublished := publishRetentionEvents(t, tieStore,
		retentionEvent(responseevents.KindMessage, responseevents.PhaseDelta, "oldest"),
		retentionEvent(responseevents.KindMessage, responseevents.PhaseDelta, "newest"),
	)
	if got := tieStore.Events(); !reflect.DeepEqual(got, []responseevents.FactoryResponseEvent{tiePublished[1]}) {
		t.Fatalf("tie retention = %#v, want newest event", got)
	}

	preferredTieStore := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 1, MaxBytes: generousByteLimit})
	preferredTiePublished := publishRetentionEvents(t, preferredTieStore,
		retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, "old-final"),
		retentionEvent(responseevents.KindTool, responseevents.PhaseFailed, "new-failure"),
	)
	if got := preferredTieStore.Events(); !reflect.DeepEqual(got, []responseevents.FactoryResponseEvent{preferredTiePublished[1]}) {
		t.Fatalf("preferred tie retention = %#v, want newest final semantic event", got)
	}
}

func TestSessionResponseEventStore_OversizedEventIsDroppedWithoutSequenceReuse(t *testing.T) {
	t.Parallel()

	store := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 8, MaxBytes: generousByteLimit})
	first := publishRetentionEvents(t, store,
		retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, "oversized"),
	)[0]
	firstBytes, err := responseeventstore.SerializedEventSize(first)
	if err != nil {
		t.Fatalf("SerializedEventSize(first): %v", err)
	}
	if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 8, MaxBytes: firstBytes - 1}); err != nil {
		t.Fatalf("SetRetentionLimits: %v", err)
	}
	if got := store.Events(); len(got) != 0 {
		t.Fatalf("oversized retained events = %#v, want none", got)
	}

	second, err := store.Publish(retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, "also-oversized"))
	if err != nil {
		t.Fatalf("Publish(second): %v", err)
	}
	if second.Sequence != first.Sequence+1 || second.EventID == first.EventID {
		t.Fatalf("second identity = (%d, %q), first = (%d, %q)", second.Sequence, second.EventID, first.Sequence, first.EventID)
	}
	if got := store.Events(); len(got) != 0 {
		t.Fatalf("second oversized retained events = %#v, want none", got)
	}
	if latest := store.LatestSequence(); latest != second.Sequence {
		t.Fatalf("LatestSequence = %d, want published sequence %d", latest, second.Sequence)
	}
	assertExactRetentionAccounting(t, store)
}

func TestSessionResponseEventStore_RepeatedRetentionKeepsExactCountersAndImmutableEvents(t *testing.T) {
	t.Parallel()

	limits := responseeventstore.RetentionLimits{MaxEvents: 7, MaxBytes: 3_200}
	store := newRetentionStore(t, limits)
	rng := rand.New(rand.NewSource(42))
	publishedBytes := make(map[int64][]byte)
	publishedIDs := make(map[string]struct{})

	for index := 0; index < 250; index++ {
		var input responseevents.FactoryResponseEvent
		switch rng.Intn(6) {
		case 0, 1:
			input = retentionEvent(responseevents.KindMessage, responseevents.PhaseDelta, fmt.Sprintf("delta-%03d-%s", index, strings.Repeat("x", rng.Intn(24))))
		case 2:
			input = retentionEvent(responseevents.KindProgress, responseevents.PhaseUpdated, fmt.Sprintf("progress-%03d", index))
		case 3:
			input = retentionEvent(responseevents.KindMessage, responseevents.PhaseCompleted, fmt.Sprintf("final-%03d", index))
		case 4:
			input = retentionEvent(responseevents.KindTool, responseevents.PhaseFailed, fmt.Sprintf("tool-%03d", index))
		default:
			input = retentionEvent(responseevents.KindRun, responseevents.PhaseCompleted, fmt.Sprintf("run-%03d", index))
		}
		published, err := store.Publish(input)
		if err != nil {
			t.Fatalf("Publish(%d): %v", index, err)
		}
		if published.Sequence != int64(index+1) {
			t.Fatalf("Publish(%d) sequence = %d, want %d", index, published.Sequence, index+1)
		}
		if _, exists := publishedIDs[published.EventID]; exists {
			t.Fatalf("Publish(%d) reused event ID %q", index, published.EventID)
		}
		publishedIDs[published.EventID] = struct{}{}
		serialized, err := json.Marshal(published)
		if err != nil {
			t.Fatalf("json.Marshal(published): %v", err)
		}
		publishedBytes[published.Sequence] = serialized
		assertExactRetentionAccounting(t, store)

		for _, retained := range store.Events() {
			got, err := json.Marshal(retained)
			if err != nil {
				t.Fatalf("json.Marshal(retained): %v", err)
			}
			if !reflect.DeepEqual(got, publishedBytes[retained.Sequence]) {
				t.Fatalf("retained sequence %d changed\ngot=%s\nwant=%s", retained.Sequence, got, publishedBytes[retained.Sequence])
			}
		}
	}
	if latest := store.LatestSequence(); latest != 250 {
		t.Fatalf("LatestSequence = %d, want 250", latest)
	}
}

func TestSessionResponseEventStore_ConcurrentPublishRetentionAccounting(t *testing.T) {
	store := newRetentionStore(t, responseeventstore.RetentionLimits{MaxEvents: 9, MaxBytes: generousByteLimit})

	const workers = 64
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := 0; index < workers; index++ {
		go func(index int) {
			defer wait.Done()
			_, err := store.Publish(retentionEvent(
				responseevents.KindMessage,
				responseevents.PhaseDelta,
				fmt.Sprintf("delta-%d", index),
			))
			if err != nil {
				t.Errorf("Publish(%d): %v", index, err)
			}
		}(index)
	}
	wait.Wait()

	if latest := store.LatestSequence(); latest != workers {
		t.Fatalf("LatestSequence = %d, want %d", latest, workers)
	}
	assertExactRetentionAccounting(t, store)
}
