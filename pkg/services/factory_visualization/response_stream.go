package factory_visualization

import (
	"io"
	"sync"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// ResponseEventEncoder is the transport-owned representation edge used by a
// serialized response stream. A false result omits the canonical event.
type ResponseEventEncoder func(factorysessions.FactoryResponseEvent) ([]byte, bool)

// FinalResponseWriter writes one terminal invocation representation after all
// accepted progress has drained. progressSeen reports whether any progress
// record was accepted by the stream.
type FinalResponseWriter func(io.Writer, bool) error

// ResponseStream owns response attachment serialization, canonical sequence
// deduplication, terminal ordering, and final-once state for one invocation.
// The CLI supplies representation-only encoders and an invocation-local writer.
type ResponseStream interface {
	ResponseEventSink
	Finalize(FinalResponseWriter) (first bool, err error)
	CloseAndDrain() error
}

type serializedResponseStream struct {
	mu                   sync.Mutex
	output               Output
	encode               ResponseEventEncoder
	lastResponseSequence int64
	progressSeen         bool
	finalized            bool
	finalErr             error
}

func newSerializedResponseStream(output Output, encode ResponseEventEncoder) *serializedResponseStream {
	if output == nil {
		panic("response presentation stream output is nil")
	}
	if encode == nil {
		panic("response presentation event encoder is nil")
	}
	return &serializedResponseStream{output: output, encode: encode}
}

func (stream *serializedResponseStream) PresentResponseEvents(events []factorysessions.FactoryResponseEvent) {
	if stream == nil {
		return
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.finalized {
		return
	}
	for _, event := range events {
		if event.Sequence > 0 && event.Sequence <= stream.lastResponseSequence {
			continue
		}
		if event.Sequence > 0 {
			stream.lastResponseSequence = event.Sequence
		}
		payload, ok := stream.encode(event)
		if !ok || len(payload) == 0 {
			continue
		}
		if err := stream.output.Enqueue(payload); err == nil {
			stream.progressSeen = true
		}
	}
}

func (stream *serializedResponseStream) Finalize(write FinalResponseWriter) (bool, error) {
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

func (stream *serializedResponseStream) CloseAndDrain() error {
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
