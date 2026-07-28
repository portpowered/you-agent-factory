package contracts

import (
	"errors"
	"io"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	DefaultProgressQueueCapacity = 64
)

var (
	ErrOutputClosed  = errors.New("response presentation output is closed")
	ErrBacklogFull   = errors.New("response presentation output backlog is full")
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

// FactoryEventEncoder is the transport-owned representation edge for one
// canonical Factory Event. A false result omits the event from that adapter.
type FactoryEventEncoder func(factorydefinitions.FactoryEvent) ([]byte, bool)

// FinalResponseWriter writes one terminal invocation representation after all
// accepted Factory Event records have drained. progressSeen reports whether
// the stream accepted any lifecycle record.
type FinalResponseWriter func(io.Writer, bool) error

// FactoryEventStream presents accepted Factory Events then finalizes one
// terminal payload after drain.
type FactoryEventStream interface {
	PresentFactoryEvents([]factorydefinitions.FactoryEvent)
	Finalize(FinalResponseWriter) (bool, error)
	CloseAndDrain() error
}

// AppendLine formats one presentation record with a trailing newline.
func AppendLine(payload []byte) []byte {
	line := make([]byte, len(payload)+1)
	copy(line, payload)
	line[len(payload)] = '\n'
	return line
}
