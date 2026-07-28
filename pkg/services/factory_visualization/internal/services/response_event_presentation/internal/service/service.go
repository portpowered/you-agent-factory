package service

import (
	"fmt"
	"io"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/contracts"
	responseeventpresentation "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation"
)

const progressDrainTimeout = 250 * time.Millisecond

// Service keeps response/event presentation queue and drain behavior behind
// the Visualization-owned response_event_presentation capability.
type Service struct{}

var _ responseeventpresentation.Service = (*Service)(nil)

// New constructs the inert response/event presentation owner. No presentation
// worker goroutines start until an output or stream is explicitly opened.
func New() *Service {
	return &Service{}
}

func (Service) OpenBestEffortOutput(writer io.Writer) contracts.Output {
	return newBestEffortOutput(writer)
}

func (Service) OpenLosslessOutput(writer io.Writer) contracts.Output {
	return newLosslessOutput(writer)
}

func (Service) OpenBestEffortFactoryEventStream(
	writer io.Writer,
	encode contracts.FactoryEventEncoder,
) contracts.FactoryEventStream {
	return newSerializedFactoryEventStream(newBestEffortOutput(writer), encode)
}

func (Service) OpenLosslessFactoryEventStream(
	writer io.Writer,
	encode contracts.FactoryEventEncoder,
) contracts.FactoryEventStream {
	return newSerializedFactoryEventStream(newLosslessOutput(writer), encode)
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
		queue:  make(chan []byte, contracts.DefaultProgressQueueCapacity),
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
		return contracts.ErrOutputClosed
	}
	select {
	case o.queue <- line:
		return nil
	default:
		o.dropped++
		return contracts.ErrBacklogFull
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
		return contracts.ErrOutputClosed
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

type serializedFactoryEventStream struct {
	mu           sync.Mutex
	output       contracts.Output
	encode       contracts.FactoryEventEncoder
	progressSeen bool
	finalized    bool
	finalErr     error
}

func newSerializedFactoryEventStream(
	output contracts.Output,
	encode contracts.FactoryEventEncoder,
) *serializedFactoryEventStream {
	if output == nil {
		panic("Factory Event presentation output is nil")
	}
	if encode == nil {
		panic("Factory Event presentation encoder is nil")
	}
	return &serializedFactoryEventStream{output: output, encode: encode}
}

func (stream *serializedFactoryEventStream) PresentFactoryEvents(events []factorydefinitions.FactoryEvent) {
	if stream == nil {
		return
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.finalized {
		return
	}
	for _, event := range events {
		payload, ok := stream.encode(event)
		if !ok || len(payload) == 0 {
			continue
		}
		if err := stream.output.Enqueue(payload); err == nil {
			stream.progressSeen = true
		}
	}
}

func (stream *serializedFactoryEventStream) Finalize(write contracts.FinalResponseWriter) (bool, error) {
	if stream == nil {
		return false, io.ErrClosedPipe
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.finalized {
		return false, stream.finalErr
	}
	stream.finalized = true
	if write == nil {
		stream.finalErr = io.ErrClosedPipe
		return true, stream.finalErr
	}
	if err := stream.output.CloseAndDrain(); err != nil {
		stream.finalErr = err
		return true, err
	}
	stream.finalErr = stream.output.WithWriterExclusive(func(writer io.Writer) error {
		return write(writer, stream.progressSeen)
	})
	return true, stream.finalErr
}

func (stream *serializedFactoryEventStream) CloseAndDrain() error {
	if stream == nil {
		return nil
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if !stream.finalized {
		stream.finalized = true
		stream.finalErr = stream.output.CloseAndDrain()
	}
	return stream.finalErr
}

func appendLine(payload []byte) []byte {
	return contracts.AppendLine(payload)
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

