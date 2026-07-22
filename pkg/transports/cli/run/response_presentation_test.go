package run

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

const (
	defaultResponseStreamProgressQueueCapacity = 64
	responseStreamProgressDrainTimeout         = 250 * time.Millisecond
)

// testResponsePresentation is the service-root test edge used by transport
// encoding tests. Queue and attachment invariants are owned and tested in the
// Factory Visualization package.
func testResponsePresentation() factoryvisualization.ResponsePresentation {
	return fakeResponsePresentation{}
}

func testResponseEventValidator() factorysessions.ResponseEventValidator {
	return func(factorysessions.FactoryResponseEvent) error { return nil }
}

type fakeResponsePresentation struct{}

func (fakeResponsePresentation) Attach(
	ctx context.Context,
	source factoryvisualization.ResponseEventSource,
	sessionID string,
	sink factoryvisualization.ResponseEventSink,
) factoryvisualization.ResponseEventAttachment {
	cursor, err := source.SubscribeSessionResponseEventsFromLatest(sessionID)
	if err != nil {
		return nil
	}
	attachmentCtx, cancel := context.WithCancel(ctx)
	attachment := &fakeResponseEventAttachment{
		cancel: cancel, done: make(chan struct{}), cursor: cursor, sink: sink,
	}
	go attachment.consume(attachmentCtx)
	return attachment
}

func (fakeResponsePresentation) OpenBestEffortOutput(writer io.Writer) factoryvisualization.Output {
	return &fakePresentationOutput{writer: writer, capacity: defaultResponseStreamProgressQueueCapacity}
}

func (fakeResponsePresentation) OpenLosslessOutput(writer io.Writer) factoryvisualization.Output {
	return &fakePresentationOutput{writer: writer}
}

func (fakeResponsePresentation) OpenBestEffortResponseStream(
	writer io.Writer,
	encode factoryvisualization.ResponseEventEncoder,
) factoryvisualization.ResponseStream {
	return &fakeResponseStream{
		output: &fakePresentationOutput{writer: writer, capacity: defaultResponseStreamProgressQueueCapacity},
		encode: encode,
	}
}

func (fakeResponsePresentation) OpenLosslessResponseStream(
	writer io.Writer,
	encode factoryvisualization.ResponseEventEncoder,
) factoryvisualization.ResponseStream {
	return &fakeResponseStream{
		output: &fakePresentationOutput{writer: writer},
		encode: encode,
	}
}

// fakeResponseStream is a programmable service-root edge for transport tests.
// Serialization ownership and concurrency invariants are tested by the
// Factory Visualization owner package.
type fakeResponseStream struct {
	mu           sync.Mutex
	output       factoryvisualization.Output
	encode       factoryvisualization.ResponseEventEncoder
	lastSequence int64
	progressSeen bool
	finalized    bool
	finalErr     error
}

func (s *fakeResponseStream) PresentResponseEvents(events []factorysessions.FactoryResponseEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finalized {
		return
	}
	for _, event := range events {
		if event.Sequence > 0 && event.Sequence <= s.lastSequence {
			continue
		}
		if event.Sequence > 0 {
			s.lastSequence = event.Sequence
		}
		payload, ok := s.encode(event)
		if !ok || len(payload) == 0 {
			continue
		}
		if err := s.output.Enqueue(payload); err == nil {
			s.progressSeen = true
		}
	}
}

func (s *fakeResponseStream) Finalize(write factoryvisualization.FinalResponseWriter) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finalized {
		return false, s.finalErr
	}
	s.finalized = true
	if err := s.output.CloseAndDrain(); err != nil {
		s.finalErr = err
		return true, err
	}
	s.finalErr = s.output.WithWriterExclusive(func(writer io.Writer) error {
		return write(writer, s.progressSeen)
	})
	return true, s.finalErr
}

func (s *fakeResponseStream) CloseAndDrain() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.finalized {
		s.finalized = true
		s.finalErr = s.output.CloseAndDrain()
	}
	return s.finalErr
}

type fakeResponseEventAttachment struct {
	cancel context.CancelFunc
	done   chan struct{}
	cursor factorysessions.ResponseEventCursor
	sink   factoryvisualization.ResponseEventSink
}

func (a *fakeResponseEventAttachment) consume(ctx context.Context) {
	defer close(a.done)
	for {
		events, err := a.cursor.Next(ctx)
		if err != nil {
			return
		}
		a.sink.PresentResponseEvents(events)
	}
}

func (a *fakeResponseEventAttachment) Stop() {
	a.cancel()
	<-a.done
	events, _ := a.cursor.Drain()
	a.sink.PresentResponseEvents(events)
	a.cursor.Detach()
}

type fakePresentationOutput struct {
	mu       sync.Mutex
	writer   io.Writer
	capacity int
	pending  [][]byte
	dropped  int
	closed   bool
}

func (o *fakePresentationOutput) Enqueue(payload []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return errors.New("fake response presentation output is closed")
	}
	if o.capacity > 0 && len(o.pending) >= o.capacity {
		o.dropped++
		return errors.New("fake response presentation output backlog is full")
	}
	o.pending = append(o.pending, append([]byte(nil), payload...))
	return nil
}

func (o *fakePresentationOutput) CloseAndDrain() error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil
	}
	o.closed = true
	pending := append([][]byte(nil), o.pending...)
	o.pending = nil
	o.mu.Unlock()
	for _, payload := range pending {
		if _, err := o.writer.Write(append(payload, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func (o *fakePresentationOutput) WithWriterExclusive(write func(io.Writer) error) error {
	return write(o.writer)
}

func (o *fakePresentationOutput) Dropped() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.dropped
}
