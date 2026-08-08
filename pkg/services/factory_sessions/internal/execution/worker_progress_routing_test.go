package factorysessionexecution

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestPublishWorkerProgress_ReachesOnlyTheSessionThatStartedTheWorker pins the
// routing a JavaScript child's output depends on.
//
// A child is a Worker, so its output arrives from Workers addressed only by
// dispatch. Two durable sessions of one Factory share that Factory's Workers
// pool, so the dispatch identity is the only thing that can tell their output
// apart -- and a fragment delivered to the wrong session would show one
// customer's provider output inside another's session.
func TestPublishWorkerProgress_ReachesOnlyTheSessionThatStartedTheWorker(t *testing.T) {
	service := newDurableResponseEventsService(t)
	first := seedResponseEventSession(t, service, "dur-sess-first")
	second := seedResponseEventSession(t, service, "dur-sess-second")
	if err := service.ensureSessionResponseEvents("dur-sess-first", first); err != nil {
		t.Fatalf("ensure first response events: %v", err)
	}
	if err := service.ensureSessionResponseEvents("dur-sess-second", second); err != nil {
		t.Fatalf("ensure second response events: %v", err)
	}

	release := service.observeWorkerDispatch("dur-sess-first/dispatch-1", "dur-sess-first")
	defer release()

	service.PublishWorkerProgress(workers.ProgressFragment{
		DispatchID:     "dur-sess-first/dispatch-1",
		CanonicalDraft: validMessageDeltaDraft("dur-sess-first/dispatch-1"),
	})

	if got := first.responseEvents.RetentionAccounting().EventCount; got != 1 {
		t.Fatalf("owning session response events = %d, want 1", got)
	}
	if got := second.responseEvents.RetentionAccounting().EventCount; got != 0 {
		t.Fatalf("other session response events = %d, want 0", got)
	}
}

// TestPublishWorkerProgress_IgnoresADispatchNoSessionOwns keeps the fan-out
// safe for every other Worker in the process: a Petri Worker's progress passes
// through the same publisher and must land nowhere here.
func TestPublishWorkerProgress_IgnoresADispatchNoSessionOwns(t *testing.T) {
	service := newDurableResponseEventsService(t)
	state := seedResponseEventSession(t, service, "dur-sess-first")
	if err := service.ensureSessionResponseEvents("dur-sess-first", state); err != nil {
		t.Fatalf("ensure response events: %v", err)
	}

	service.PublishWorkerProgress(workers.ProgressFragment{
		DispatchID:     "petri-dispatch-1",
		CanonicalDraft: validMessageDeltaDraft("petri-dispatch-1"),
	})

	if got := state.responseEvents.RetentionAccounting().EventCount; got != 0 {
		t.Fatalf("response events for an unowned dispatch = %d, want 0", got)
	}
}

// TestPublishWorkerProgress_StopsOnceTheWorkerIsReleased proves the claim is
// scoped to the Worker's run. Holding it forever would grow the index by one
// entry per child for the life of the process and keep routing output from a
// dispatch identity Workers may hand to nobody.
func TestPublishWorkerProgress_StopsOnceTheWorkerIsReleased(t *testing.T) {
	service := newDurableResponseEventsService(t)
	state := seedResponseEventSession(t, service, "dur-sess-first")
	if err := service.ensureSessionResponseEvents("dur-sess-first", state); err != nil {
		t.Fatalf("ensure response events: %v", err)
	}

	release := service.observeWorkerDispatch("dur-sess-first/dispatch-1", "dur-sess-first")
	release()

	service.PublishWorkerProgress(workers.ProgressFragment{
		DispatchID:     "dur-sess-first/dispatch-1",
		CanonicalDraft: validMessageDeltaDraft("dur-sess-first/dispatch-1"),
	})

	if got := state.responseEvents.RetentionAccounting().EventCount; got != 0 {
		t.Fatalf("response events after release = %d, want 0", got)
	}
}

// TestChildWorkerExecutor_ScopesTheWorkersIdentityToItsSession pins what makes
// those routing keys distinct in the first place.
//
// Child dispatch identities are minted per session and start again at
// dispatch-1 for each, while the Workers pool they share treats a dispatch ID
// as single-use for its whole life. Two sessions submitting an unqualified
// dispatch-1 would leave the second refused outright.
func TestChildWorkerExecutor_ScopesTheWorkersIdentityToItsSession(t *testing.T) {
	first := newChildWorkerExecutor("dur-sess-first", nil, nil, nil, nil, 0, "")
	second := newChildWorkerExecutor("dur-sess-second", nil, nil, nil, nil, 0, "")

	firstID := first.workerDispatchIdentity("dispatch-1")
	secondID := second.workerDispatchIdentity("dispatch-1")
	if firstID == secondID {
		t.Fatalf("two sessions submitted the same Workers dispatch identity %q", firstID)
	}
	if firstID != "dur-sess-first/dispatch-1" {
		t.Fatalf("Workers dispatch identity = %q, want the session-scoped identity", firstID)
	}
}
