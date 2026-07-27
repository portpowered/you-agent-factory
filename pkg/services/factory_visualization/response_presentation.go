package factory_visualization

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	defaultProgressQueueCapacity = 64
	progressDrainTimeout         = 250 * time.Millisecond
)

var (
	errPresentationOutputClosed = errors.New("response presentation output is closed")
	errPresentationBacklogFull  = errors.New("response presentation output backlog is full")
)

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
	OpenBestEffortOutput(io.Writer) Output
	OpenLosslessOutput(io.Writer) Output
	OpenBestEffortFactoryEventStream(io.Writer, FactoryEventEncoder) interface {
		PresentFactoryEvents([]factorydefinitions.FactoryEvent)
		Finalize(FinalResponseWriter) (bool, error)
		CloseAndDrain() error
	}
	OpenLosslessFactoryEventStream(io.Writer, FactoryEventEncoder) interface {
		PresentFactoryEvents([]factorydefinitions.FactoryEvent)
		Finalize(FinalResponseWriter) (bool, error)
		CloseAndDrain() error
	}
}

type responsePresentation struct{}

// NewResponsePresentation constructs the inert response-presentation service.
// Wire is the application-level caller.
func NewResponsePresentation() ResponsePresentation {
	return responsePresentation{}
}

func (responsePresentation) OpenBestEffortOutput(writer io.Writer) Output {
	return newBestEffortOutput(writer)
}

func (responsePresentation) OpenLosslessOutput(writer io.Writer) Output {
	return newLosslessOutput(writer)
}

func (responsePresentation) OpenBestEffortFactoryEventStream(
	writer io.Writer,
	encode FactoryEventEncoder,
) interface {
	PresentFactoryEvents([]factorydefinitions.FactoryEvent)
	Finalize(FinalResponseWriter) (bool, error)
	CloseAndDrain() error
} {
	return newSerializedFactoryEventStream(newBestEffortOutput(writer), encode)
}

func (responsePresentation) OpenLosslessFactoryEventStream(
	writer io.Writer,
	encode FactoryEventEncoder,
) interface {
	PresentFactoryEvents([]factorydefinitions.FactoryEvent)
	Finalize(FinalResponseWriter) (bool, error)
	CloseAndDrain() error
} {
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
		return errPresentationOutputClosed
	}
	select {
	case o.queue <- line:
		return nil
	default:
		o.dropped++
		return errPresentationBacklogFull
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
		return errPresentationOutputClosed
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
