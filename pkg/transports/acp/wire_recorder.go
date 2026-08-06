package acp

import "github.com/portpowered/infinite-you/pkg/platform/wiretranscript"

// WireTranscript is one open per-connection recording of ACP wire traffic.
type WireTranscript interface {
	wiretranscript.Recorder
	// Path is the on-disk location a customer is pointed at.
	Path() string
	Close() error
}

// WireRecorder opens a transcript for one connection.
//
// It is a replaceable port rather than a concrete dependency so the transport
// never chooses where recordings live or whether they are enabled; the
// composition root does. A nil WireRecorder disables recording entirely, and a
// recorder that fails to open must never fail the connection: a diagnostic
// artifact is not worth dropping a customer's session over.
type WireRecorder func(connectionID string) (WireTranscript, error)
