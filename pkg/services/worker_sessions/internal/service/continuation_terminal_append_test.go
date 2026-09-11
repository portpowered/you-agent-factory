package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// terminalAppendObserver preserves the real in-memory Events implementation
// while exposing a signal after a targeted Worker Session terminal record has
// been committed. The controlled Workers boundary returns before Worker
// Sessions commits that record, so waiting for the boundary alone is not a
// sufficient observation point for a terminal Get assertion.
type terminalAppendObserver struct {
	events.Service

	mu      sync.Mutex
	signals map[events.Topic]chan struct{}
	once    map[events.Topic]*sync.Once
}

func newTerminalAppendObserver(inner events.Service, topics ...events.Topic) *terminalAppendObserver {
	observer := &terminalAppendObserver{
		Service: inner,
		signals: make(map[events.Topic]chan struct{}, len(topics)),
		once:    make(map[events.Topic]*sync.Once, len(topics)),
	}
	for _, topic := range topics {
		observer.signals[topic] = make(chan struct{})
		observer.once[topic] = &sync.Once{}
	}
	return observer
}

func (o *terminalAppendObserver) terminalAppendSignal(topic events.Topic) (<-chan struct{}, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	signal, registered := o.signals[topic]
	return signal, registered
}

func (o *terminalAppendObserver) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	result, err := o.Service.Append(ctx, req)
	if err != nil || !isWorkerSessionTerminalAppend(req) {
		return result, err
	}

	o.mu.Lock()
	signal := o.signals[req.Topic]
	once := o.once[req.Topic]
	o.mu.Unlock()
	if signal != nil && once != nil {
		once.Do(func() { close(signal) })
	}
	return result, nil
}

func (o *terminalAppendObserver) waitForTerminalAppend(t *testing.T, topic events.Topic) {
	t.Helper()
	signal, registered := o.terminalAppendSignal(topic)
	if !registered {
		t.Fatalf("terminal append signal for topic %q is not registered", topic)
	}
	if err := waitControlledSignal(signal, controlledBoundaryWaitTimeout); err != nil {
		t.Fatalf("terminal append for topic %q: %v", topic, err)
	}
}

func isWorkerSessionTerminalAppend(req events.AppendRequest) bool {
	return req.SourceType == "worker_session_lifecycle" &&
		req.SourceSequence == 2 &&
		req.SourceEventID == "terminal" &&
		req.SchemaID == "workers.draft.v1"
}
