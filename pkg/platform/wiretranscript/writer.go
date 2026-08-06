package wiretranscript

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"
	"time"
)

// MaxFrameBytes bounds how much of a single line is retained. A frame larger
// than this is recorded truncated and marked, so one oversized frame can never
// consume a whole rotation budget, and is never silently dropped either.
const MaxFrameBytes = 256 * 1024

// Clock supplies record timestamps.
type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// Writer serializes records onto an underlying sink as JSONL.
//
// Sequence numbers are assigned under the same lock as the write, so they are
// gap-free and establish a total order across interleaved directions even
// though inbound and outbound are observed on different goroutines.
type Writer struct {
	mu       sync.Mutex
	out      io.Writer
	encoder  *json.Encoder
	clock    Clock
	sequence uint64
	closed   bool
}

// NewWriter returns a Writer emitting to out. A nil out yields a nil Writer,
// which is a no-op, so callers can treat "recording disabled" as one shape.
func NewWriter(out io.Writer, clock Clock) *Writer {
	if out == nil {
		return nil
	}
	if clock == nil {
		clock = systemClock{}
	}
	encoder := json.NewEncoder(out)
	// Protocol payloads routinely contain <, >, and &. Escaping them would
	// alter the recorded text without altering its meaning, which defeats a
	// byte-for-byte comparison against another agent's transcript.
	encoder.SetEscapeHTML(false)
	return &Writer{out: out, encoder: encoder, clock: clock}
}

// Record writes one observed line. It never fails the caller's own work: a
// recording error is reported, but callers are expected to ignore it rather
// than fail a live protocol turn over a diagnostic artifact.
func (w *Writer) Record(conn string, peer Peer, direction Direction, stream Stream, line []byte) error {
	if w == nil {
		return nil
	}
	record := Record{
		Version:   FormatVersion,
		Conn:      conn,
		Peer:      peer,
		Direction: direction,
		Stream:    stream,
		Bytes:     len(line),
	}

	trimmed := bytes.TrimRight(line, "\r\n")
	switch {
	case len(trimmed) > MaxFrameBytes:
		record.Text = string(trimmed[:MaxFrameBytes])
		record.Err = "frame exceeded the retained size limit and was truncated"
	case stream == StreamStderr || !json.Valid(trimmed):
		record.Text = string(trimmed)
		if stream != StreamStderr && len(trimmed) > 0 {
			record.Err = "line is not valid JSON"
		}
	default:
		record.Frame = json.RawMessage(append([]byte(nil), trimmed...))
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.sequence++
	record.Sequence = w.sequence
	record.Timestamp = w.clock.Now().UTC().Format(time.RFC3339Nano)
	return w.encoder.Encode(record)
}

// Close stops further recording and closes the sink when it owns one.
func (w *Writer) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if closer, ok := w.out.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
