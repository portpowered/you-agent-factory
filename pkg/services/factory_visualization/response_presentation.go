package factory_visualization

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

const (
	defaultProgressQueueCapacity = 64
	progressDrainTimeout         = 250 * time.Millisecond
)

// ResponseEventSink is the transport-owned encoding edge for canonical
// Factory Session response events.
type ResponseEventSink interface {
	PresentResponseEvents([]factorysessions.FactoryResponseEvent)
}

// ResponseEventSource is the exact Factory Session observation capability
// consumed by response presentation.
type ResponseEventSource interface {
	SubscribeSessionResponseEventsFromLatest(string) (factorysessions.ResponseEventCursor, error)
}

// ResponseEventAttachment owns one response-event subscription lifecycle.
type ResponseEventAttachment interface {
	Stop()
}

// Output serializes encoded presentation records onto one transport writer.
// Best-effort outputs may reject records under backpressure; lossless outputs
// retain every accepted record until CloseAndDrain completes.
type Output interface {
	Enqueue([]byte) error
	CloseAndDrain() error
	WithWriterExclusive(func(io.Writer) error) error
	Dropped() int
}

// ResponsePresentation is the service-root operation used by transports.
type ResponsePresentation interface {
	Attach(context.Context, ResponseEventSource, string, ResponseEventSink) ResponseEventAttachment
	OpenBestEffortOutput(io.Writer) Output
	OpenLosslessOutput(io.Writer) Output
	OpenBestEffortResponseStream(io.Writer, ResponseEventEncoder) ResponseStream
	OpenLosslessResponseStream(io.Writer, ResponseEventEncoder) ResponseStream
}

type responsePresentation struct{}

// NewResponsePresentation constructs the inert response-presentation service.
// Wire is the application-level caller.
func NewResponsePresentation() ResponsePresentation {
	return responsePresentation{}
}

func (responsePresentation) Attach(
	ctx context.Context,
	source ResponseEventSource,
	sessionID string,
	sink ResponseEventSink,
) ResponseEventAttachment {
	if source == nil || sink == nil {
		return nil
	}
	cursor, err := source.SubscribeSessionResponseEventsFromLatest(sessionID)
	if err != nil || cursor == nil {
		return nil
	}
	attachCtx, cancel := context.WithCancel(ctx)
	attachment := &responseEventAttachment{
		cancel: cancel, done: make(chan struct{}), cursor: cursor, sink: sink,
	}
	go attachment.consume(attachCtx)
	return attachment
}

func (responsePresentation) OpenBestEffortOutput(writer io.Writer) Output {
	return newBestEffortOutput(writer)
}

func (responsePresentation) OpenLosslessOutput(writer io.Writer) Output {
	return newLosslessOutput(writer)
}

func (responsePresentation) OpenBestEffortResponseStream(
	writer io.Writer,
	encode ResponseEventEncoder,
) ResponseStream {
	return newSerializedResponseStream(newBestEffortOutput(writer), encode)
}

func (responsePresentation) OpenLosslessResponseStream(
	writer io.Writer,
	encode ResponseEventEncoder,
) ResponseStream {
	return newSerializedResponseStream(newLosslessOutput(writer), encode)
}

type responseEventAttachment struct {
	cancel context.CancelFunc
	done   chan struct{}
	cursor factorysessions.ResponseEventCursor
	sink   ResponseEventSink
}

func (a *responseEventAttachment) consume(ctx context.Context) {
	defer close(a.done)
	for {
		events, err := a.cursor.Next(ctx)
		if err != nil {
			return
		}
		a.sink.PresentResponseEvents(events)
	}
}

func (a *responseEventAttachment) Stop() {
	if a == nil {
		return
	}
	a.cancel()
	<-a.done
	if events, err := a.cursor.Drain(); err == nil && len(events) > 0 {
		a.sink.PresentResponseEvents(events)
	}
	a.cursor.Detach()
}

type bestEffortOutput struct {
	mu       sync.Mutex
	outputMu sync.Mutex
	writer   io.Writer
	queue    chan []byte
	wg       sync.WaitGroup
	closed   bool
	abandon  bool
	dropped  int
}

func newBestEffortOutput(writer io.Writer) *bestEffortOutput {
	if writer == nil {
		panic("response presentation output is nil")
	}
	output := &bestEffortOutput{
		writer: writer,
		queue:  make(chan []byte, defaultProgressQueueCapacity),
	}
	output.wg.Add(1)
	go output.run()
	return output
}

func (o *bestEffortOutput) Enqueue(payload []byte) error {
	line := appendLine(payload)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return errors.New("response presentation output is closed")
	}
	select {
	case o.queue <- line:
		return nil
	default:
		o.dropped++
		return errors.New("response presentation output backlog is full")
	}
}

func (o *bestEffortOutput) CloseAndDrain() error {
	o.mu.Lock()
	if !o.closed {
		o.closed = true
		close(o.queue)
	}
	o.mu.Unlock()
	if !waitOutput(&o.wg, progressDrainTimeout) {
		o.mu.Lock()
		o.abandon = true
		o.mu.Unlock()
	}
	return nil
}

func (o *bestEffortOutput) WithWriterExclusive(write func(io.Writer) error) error {
	o.outputMu.Lock()
	defer o.outputMu.Unlock()
	return write(o.writer)
}

func (o *bestEffortOutput) Dropped() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.dropped
}

func (o *bestEffortOutput) run() {
	defer o.wg.Done()
	for line := range o.queue {
		o.outputMu.Lock()
		o.mu.Lock()
		abandon := o.abandon
		o.mu.Unlock()
		if !abandon {
			_, _ = o.writer.Write(line)
		}
		o.outputMu.Unlock()
		if abandon {
			return
		}
	}
}

type losslessOutput struct {
	mu       sync.Mutex
	ready    *sync.Cond
	writer   io.Writer
	pending  [][]byte
	head     int
	closed   bool
	writeErr error
	wg       sync.WaitGroup
}

func newLosslessOutput(writer io.Writer) *losslessOutput {
	if writer == nil {
		panic("response presentation output is nil")
	}
	output := &losslessOutput{writer: writer}
	output.ready = sync.NewCond(&output.mu)
	output.wg.Add(1)
	go output.run()
	return output
}

func (o *losslessOutput) Enqueue(payload []byte) error {
	line := appendLine(payload)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.writeErr != nil {
		return o.writeErr
	}
	if o.closed {
		return errors.New("response presentation output is closed")
	}
	o.pending = append(o.pending, line)
	o.ready.Signal()
	return nil
}

func (o *losslessOutput) CloseAndDrain() error {
	o.mu.Lock()
	o.closed = true
	o.ready.Broadcast()
	o.mu.Unlock()
	o.wg.Wait()
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.writeErr
}

func (o *losslessOutput) WithWriterExclusive(write func(io.Writer) error) error {
	if err := o.CloseAndDrain(); err != nil {
		return err
	}
	return write(o.writer)
}

func (o *losslessOutput) Dropped() int { return 0 }

func (o *losslessOutput) run() {
	defer o.wg.Done()
	for {
		line, ok := o.next()
		if !ok {
			return
		}
		written, err := o.writer.Write(line)
		if err == nil && written != len(line) {
			err = io.ErrShortWrite
		}
		if err != nil {
			o.fail(fmt.Errorf("write response presentation output: %w", err))
			return
		}
	}
}

func (o *losslessOutput) next() ([]byte, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for o.head == len(o.pending) && !o.closed {
		o.ready.Wait()
	}
	if o.head == len(o.pending) {
		return nil, false
	}
	line := o.pending[o.head]
	o.head++
	if o.head == len(o.pending) {
		o.pending = nil
		o.head = 0
	}
	return line, true
}

func (o *losslessOutput) fail(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.writeErr = err
	o.closed = true
	o.pending = nil
	o.head = 0
	o.ready.Broadcast()
}

func appendLine(payload []byte) []byte {
	line := make([]byte, len(payload)+1)
	copy(line, payload)
	line[len(payload)] = '\n'
	return line
}

func waitOutput(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}
