package factory_visualization

import (
	"io"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// FactoryEventEncoder is the transport-owned representation edge for one
// canonical Factory Event. A false result omits the event from that adapter.
type FactoryEventEncoder func(factorydefinitions.FactoryEvent) ([]byte, bool)

type serializedFactoryEventStream struct {
	mu           sync.Mutex
	output       Output
	encode       FactoryEventEncoder
	progressSeen bool
	finalized    bool
	finalErr     error
}

func newSerializedFactoryEventStream(output Output, encode FactoryEventEncoder) *serializedFactoryEventStream {
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

func (stream *serializedFactoryEventStream) Finalize(write FinalResponseWriter) (bool, error) {
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
