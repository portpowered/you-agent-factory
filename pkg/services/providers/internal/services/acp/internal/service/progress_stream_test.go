package service

import (
	"reflect"
	"sync"
	"testing"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// streamHarness drives a promptProgressStream the way the ACP client does:
// emission happens under the caller's lock, pending facts are taken there, and
// delivery is handed to the dispatcher outside it.
type streamHarness struct {
	stream   *promptProgressStream
	mu       sync.Mutex
	observed []providers.ExecuteProgress
}

func newStreamHarness() *streamHarness {
	harness := &streamHarness{}
	harness.stream = newPromptProgressStream(func(fact providers.ExecuteProgress) {
		harness.mu.Lock()
		harness.observed = append(harness.observed, fact)
		harness.mu.Unlock()
	})
	harness.flush()
	return harness
}

func (h *streamHarness) flush() {
	h.stream.Deliver(h.stream.takePending())
}

func (h *streamHarness) observe(updates []providers.ExecuteProgress) {
	h.stream.Observe(updates)
	h.flush()
}

// facts returns the observations delivered so far. Delivery is asynchronous,
// so callers that need every fact must close the stream first.
func (h *streamHarness) facts() []providers.ExecuteProgress {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]providers.ExecuteProgress(nil), h.observed...)
}

// finish closes the turn and joins delivery, so every fact has been observed
// by the time it returns.
func (h *streamHarness) finish(
	close func(*promptProgressStream) []providers.ExecuteProgress,
) []providers.ExecuteProgress {
	returned := close(h.stream)
	h.flush()
	h.stream.close()
	return returned
}

// promptUpdateFixture is one turn's worth of raw update-derived facts covering
// reasoning, a tool item, and a message item left implicitly open.
func promptUpdateFixture() []providers.ExecuteProgress {
	return []providers.ExecuteProgress{
		{Phase: "delta", Detail: "thinking ", Metadata: map[string]string{"kind": "reasoning", "item_id": "r1"}},
		{Phase: "delta", Detail: "harder", Metadata: map[string]string{"kind": "reasoning", "item_id": "r1"}},
		{Phase: "started", Metadata: map[string]string{"kind": "tool", "item_id": "t1"}},
		{Phase: "completed", Detail: "ran", Metadata: map[string]string{"kind": "tool", "item_id": "t1"}},
		{Phase: "delta", Detail: "hello ", Metadata: map[string]string{"kind": "message", "item_id": "m1"}},
		{Phase: "delta", Detail: "world", Metadata: map[string]string{"kind": "message", "item_id": "m1"}},
	}
}

func collectStream(
	t *testing.T,
	updates []providers.ExecuteProgress,
	close func(*promptProgressStream) []providers.ExecuteProgress,
) (observed, returned []providers.ExecuteProgress) {
	t.Helper()
	harness := newStreamHarness()
	harness.observe(updates)
	returned = harness.finish(close)
	return harness.facts(), returned
}

func completeStream(stream *promptProgressStream) []providers.ExecuteProgress {
	return stream.Complete()
}

func failStream(stream *promptProgressStream) []providers.ExecuteProgress {
	return stream.Fail()
}

// TestPromptProgressStreamObservedSequenceMatchesReturnedProgress is the
// anti-duplication invariant the whole live-streaming path rests on. The
// runner publishes the live observations and then skips
// ExecuteDiagnostics.Progress because the two are the same sequence. If they
// could ever diverge, a customer would see either duplicated or missing facts
// in the Worker trace.
func TestPromptProgressStreamObservedSequenceMatchesReturnedProgress(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name  string
		close func(*promptProgressStream) []providers.ExecuteProgress
	}{
		{"complete", completeStream},
		{"fail", failStream},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			observed, returned := collectStream(t, promptUpdateFixture(), testCase.close)
			if len(observed) == 0 {
				t.Fatal("no facts observed")
			}
			if !reflect.DeepEqual(observed, returned) {
				t.Fatalf("observed sequence and returned progress diverged:\nobserved = %#v\nreturned = %#v",
					observed, returned)
			}
		})
	}
}

// TestPromptProgressStreamMatchesNonStreamingNormalization proves adding live
// delivery did not change the normalized shape callers already depended on.
func TestPromptProgressStreamMatchesNonStreamingNormalization(t *testing.T) {
	t.Parallel()

	observedComplete, _ := collectStream(t, promptUpdateFixture(), completeStream)
	if want := completedPromptProgress(promptUpdateFixture()); !reflect.DeepEqual(observedComplete, want) {
		t.Fatalf("streamed completion = %#v, want %#v", observedComplete, want)
	}

	observedFail, _ := collectStream(t, promptUpdateFixture(), failStream)
	if want := failedPromptProgress(promptUpdateFixture()); !reflect.DeepEqual(observedFail, want) {
		t.Fatalf("streamed failure = %#v, want %#v", observedFail, want)
	}
}

// TestPromptProgressStreamEmitsContentBeforeTheTurnCloses proves the facts
// actually arrive mid-turn rather than being flushed when the turn closes.
// This is the behavior problems.md reported missing: a Worker's trace must be
// visible while the Worker is still running.
func TestPromptProgressStreamEmitsContentBeforeTheTurnCloses(t *testing.T) {
	t.Parallel()

	harness := newStreamHarness()
	harness.observe(promptUpdateFixture())

	// Delivery is asynchronous, so wait for the message content to arrive --
	// but do it before the turn is closed, which is the whole point: nothing
	// here calls Complete or Fail.
	deadline := time.Now().Add(2 * time.Second)
	var delivered []providers.ExecuteProgress
	for {
		delivered = harness.facts()
		if containsMessageText(delivered) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no message content delivered before the turn closed: %#v", delivered)
		}
		time.Sleep(time.Millisecond)
	}

	if delivered[0].Phase != "started" || delivered[0].Metadata["kind"] != "run" {
		t.Fatalf("first delivered fact = %#v, want the run start marker", delivered[0])
	}
	beforeClose := len(delivered)

	harness.finish(completeStream)
	if len(harness.facts()) <= beforeClose {
		t.Fatal("closing the turn delivered no additional markers")
	}
}

func containsMessageText(facts []providers.ExecuteProgress) bool {
	for _, fact := range facts {
		if fact.Metadata["kind"] == "message" && fact.Detail != "" {
			return true
		}
	}
	return false
}

// TestPromptProgressStreamFlagsOnlyStillOpenMessagesAsPartial pins the failure
// path: a message item the provider already reported completed is not
// still-open, so it is never relabeled after the fact.
func TestPromptProgressStreamFlagsOnlyStillOpenMessagesAsPartial(t *testing.T) {
	t.Parallel()

	updates := []providers.ExecuteProgress{
		{Phase: "delta", Detail: "closed ", Metadata: map[string]string{"kind": "message", "item_id": "done"}},
		{Phase: "completed", Detail: "closed", Metadata: map[string]string{"kind": "message", "item_id": "done"}},
		{Phase: "delta", Detail: "open", Metadata: map[string]string{"kind": "message", "item_id": "open"}},
	}
	observed, returned := collectStream(t, updates, failStream)
	if !reflect.DeepEqual(observed, returned) {
		t.Fatal("observed sequence and returned progress diverged on the failure path")
	}

	partialsByItem := map[string]bool{}
	for _, fact := range returned {
		if fact.Phase == "completed" && fact.Metadata["kind"] == "message" {
			partialsByItem[fact.Metadata["item_id"]] = fact.Metadata["partial"] == "true"
		}
	}
	if partialsByItem["done"] {
		t.Error("a message item that already completed was relabeled partial")
	}
	if !partialsByItem["open"] {
		t.Error("a still-open message item was not flagged partial")
	}
}

// TestPromptProgressStreamObserverReceivesDetachedFacts proves a listener
// cannot mutate the slice the turn later returns.
func TestPromptProgressStreamObserverReceivesDetachedFacts(t *testing.T) {
	t.Parallel()

	stream := newPromptProgressStream(func(fact providers.ExecuteProgress) {
		if fact.Metadata != nil {
			fact.Metadata["injected"] = "true"
		}
	})
	stream.Deliver(stream.takePending())
	stream.Observe(promptUpdateFixture())
	stream.Deliver(stream.takePending())
	returned := stream.Complete()
	stream.Deliver(stream.takePending())
	stream.close()

	for _, fact := range returned {
		if _, mutated := fact.Metadata["injected"]; mutated {
			t.Fatalf("observer mutated a returned fact: %#v", fact)
		}
	}
}

// TestPromptProgressStreamWithoutObserverStillNormalizes proves a nil observer
// is inert rather than a panic, which is what keeps non-streaming callers
// working unchanged.
func TestPromptProgressStreamWithoutObserverStillNormalizes(t *testing.T) {
	t.Parallel()

	stream := newPromptProgressStream(nil)
	stream.Observe(promptUpdateFixture())
	stream.Deliver(stream.takePending())
	stream.close()
	if got := stream.Complete(); len(got) == 0 {
		t.Fatal("Complete() with a nil observer returned no facts")
	}
}
