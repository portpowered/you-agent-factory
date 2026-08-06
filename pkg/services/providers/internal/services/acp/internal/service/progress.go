package service

import (
	"sort"
	"sync"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// progressDispatcher delivers observations to a caller-supplied observer on
// its own goroutine, in enqueue order.
//
// The hand-off is required rather than a convenience. Emission happens inside
// the ACP connection's session/update callback, and the SDK holds a turn's
// session/prompt response until every pre-response notification handler has
// returned. Publishing downstream directly from that callback therefore keeps
// the turn open for the duration of arbitrary downstream work, and an agent
// that disconnects promptly after responding fails the turn with "peer
// disconnected while waiting for pre-response notifications".
type progressDispatcher struct {
	observe providers.ProgressObserver
	mu      sync.Mutex
	cond    *sync.Cond
	queue   []providers.ExecuteProgress
	closed  bool
	done    chan struct{}
}

func newProgressDispatcher(observe providers.ProgressObserver) *progressDispatcher {
	if observe == nil {
		return nil
	}
	dispatcher := &progressDispatcher{observe: observe, done: make(chan struct{})}
	dispatcher.cond = sync.NewCond(&dispatcher.mu)
	go dispatcher.run()
	return dispatcher
}

func (d *progressDispatcher) run() {
	defer close(d.done)
	for {
		d.mu.Lock()
		for len(d.queue) == 0 && !d.closed {
			d.cond.Wait()
		}
		if len(d.queue) == 0 && d.closed {
			d.mu.Unlock()
			return
		}
		batch := d.queue
		d.queue = nil
		d.mu.Unlock()
		for _, fact := range batch {
			d.observe(fact)
		}
	}
}

// Enqueue hands facts to the delivery goroutine and returns immediately.
func (d *progressDispatcher) Enqueue(facts []providers.ExecuteProgress) {
	if d == nil || len(facts) == 0 {
		return
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.queue = append(d.queue, facts...)
	d.cond.Signal()
	d.mu.Unlock()
}

// Close flushes every queued fact and blocks until delivery has finished, so a
// turn cannot return before its own observations have been delivered.
func (d *progressDispatcher) Close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		<-d.done
		return
	}
	d.closed = true
	d.cond.Signal()
	d.mu.Unlock()
	<-d.done
}

// promptProgressStream normalizes one session/prompt turn's raw
// SessionUpdate-derived progress facts into the established started/completed
// shape every other Providers execution kind already reports, synthesizing a
// start marker for the first message/reasoning chunk and a closing completed
// marker for any item left implicitly finished by the turn's own completion.
//
// It emits each fact through the caller's ProgressObserver at the moment the
// fact becomes final, and accumulates the identical sequence for the returned
// ExecuteDiagnostics.Progress. Live delivery and the returned slice are the
// same sequence in the same order, which is what lets the caller publish one
// and skip the other instead of double-publishing every fact.
type promptProgressStream struct {
	observe    providers.ProgressObserver
	dispatcher *progressDispatcher
	result     []providers.ExecuteProgress
	pending    []providers.ExecuteProgress
	started    map[string]bool
	items      map[string]map[string]bool
	content    map[string]map[string]string
}

// newPromptProgressStream begins a turn, emitting the synthetic run start
// marker immediately so a listener sees the attempt open before the provider
// has produced any content.
func newPromptProgressStream(observe providers.ProgressObserver) *promptProgressStream {
	stream := &promptProgressStream{
		observe:    observe,
		dispatcher: newProgressDispatcher(observe),
		started:    map[string]bool{},
		items:      map[string]map[string]bool{},
		content:    map[string]map[string]string{},
	}
	stream.emit(providers.ExecuteProgress{
		Phase:    "started",
		Metadata: map[string]string{"kind": "run", "native_type": "session/prompt"},
	})
	return stream
}

// emit records one fact and queues a detached copy for delivery. Delivery is
// deliberately deferred to deliver() rather than performed here: emit runs
// under the owning client's mutex, and invoking a caller-supplied observer
// while holding an internal lock lets arbitrary downstream work run inside the
// ACP connection's notification callback, which stalls the read loop and can
// fail the in-flight session/prompt turn.
func (stream *promptProgressStream) emit(progress providers.ExecuteProgress) {
	stream.result = append(stream.result, progress)
	if stream.observe != nil {
		stream.pending = append(stream.pending, progress.Clone())
	}
}

// takePending removes and returns the queued observations. It mutates the
// stream, so callers must hold the same lock that guards emission; delivery
// itself happens afterwards, outside that lock, via Deliver.
func (stream *promptProgressStream) takePending() []providers.ExecuteProgress {
	pending := stream.pending
	stream.pending = nil
	return pending
}

// Deliver hands facts taken by takePending to the delivery goroutine. It is
// safe to call without the emitting lock held: the dispatcher has its own
// synchronization and each fact is already a detached copy, so a listener
// cannot mutate the slice this turn returns.
func (stream *promptProgressStream) Deliver(pending []providers.ExecuteProgress) {
	stream.dispatcher.Enqueue(pending)
}

// close flushes and joins the delivery goroutine so a turn never returns
// before its own observations have been delivered.
func (stream *promptProgressStream) close() {
	stream.dispatcher.Close()
}

// Observe streams one batch of raw update-derived facts, synthesizing the
// per-kind start marker the first time a message or reasoning item appears.
func (stream *promptProgressStream) Observe(updates []providers.ExecuteProgress) {
	for _, update := range updates {
		kind, itemID := update.Metadata["kind"], update.Metadata["item_id"]
		if (kind == "message" || kind == "reasoning") && !stream.started[kind] {
			stream.emit(providers.ExecuteProgress{Phase: "started", Metadata: map[string]string{
				"kind": kind, "item_id": itemID, "native_type": "acp/synthetic_start",
			}})
			stream.started[kind] = true
		}
		if kind != "" && itemID != "" {
			if stream.items[kind] == nil {
				stream.items[kind] = map[string]bool{}
			}
			if stream.content[kind] == nil {
				stream.content[kind] = map[string]string{}
			}
			stream.items[kind][itemID] = update.Phase == "completed"
			stream.content[kind][itemID] += update.Detail
		}
		stream.emit(update)
	}
}

// closeOpenItems emits one completed marker per item the turn left implicitly
// finished, in a stable kind-then-id order. markPartial flags those markers as
// partial content for a turn that never reached its own completed marker.
func (stream *promptProgressStream) closeOpenItems(markPartial bool) {
	for _, kind := range []string{"reasoning", "tool", "session", "message"} {
		ids := make([]string, 0, len(stream.items[kind]))
		for itemID := range stream.items[kind] {
			ids = append(ids, itemID)
		}
		sort.Strings(ids)
		for _, itemID := range ids {
			if stream.items[kind][itemID] {
				continue
			}
			metadata := map[string]string{
				"kind": kind, "item_id": itemID, "native_type": "session/prompt",
			}
			if markPartial && kind == "message" {
				metadata["partial"] = "true"
			}
			stream.emit(providers.ExecuteProgress{
				Phase:    "completed",
				Detail:   stream.content[kind][itemID],
				Metadata: metadata,
			})
		}
	}
}

// Complete closes the turn normally and returns the full ordered fact list,
// which is exactly the sequence already delivered to the observer.
func (stream *promptProgressStream) Complete() []providers.ExecuteProgress {
	stream.closeOpenItems(false)
	stream.emit(providers.ExecuteProgress{
		Phase:    "completed",
		Metadata: map[string]string{"kind": "run", "native_type": "session/prompt"},
	})
	return stream.result
}

// Fail closes a turn that did not reach its own completed marker. Still-open
// message content is flagged partial instead of claiming it finished normally,
// and no run completed marker is emitted.
//
// Only the synthetic closing markers carry the partial flag. A message item
// that already reported completed while the turn was live is not still-open,
// and its fact has already been delivered to the observer, so retroactively
// relabeling it would both misdescribe it and desynchronize the returned slice
// from what was streamed.
func (stream *promptProgressStream) Fail(extra ...providers.ExecuteProgress) []providers.ExecuteProgress {
	stream.closeOpenItems(true)
	// Failure diagnostics carried by the transport error are emitted through
	// the same path so the returned slice stays identical to what was
	// observed. Appending them afterwards instead would leave the caller
	// unable to say the whole slice was pre-observed, forcing it to republish
	// every streamed fact alongside them.
	for _, fact := range extra {
		stream.emit(fact)
	}
	return stream.result
}

// completedPromptProgress returns the normalized fact list for a turn whose
// updates were all collected before normalization. It is the non-streaming
// spelling of promptProgressStream and stays the definition of the expected
// shape.
func completedPromptProgress(updates []providers.ExecuteProgress) []providers.ExecuteProgress {
	stream := newPromptProgressStream(nil)
	stream.Observe(updates)
	return stream.Complete()
}

// failedPromptProgress is completedPromptProgress for a turn that did not
// reach its own completed marker.
func failedPromptProgress(updates []providers.ExecuteProgress) []providers.ExecuteProgress {
	stream := newPromptProgressStream(nil)
	stream.Observe(updates)
	return stream.Fail()
}
