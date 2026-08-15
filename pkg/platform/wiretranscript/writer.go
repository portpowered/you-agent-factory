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

// Clock supplies record timestamps. It is injected rather than defaulted so a
// transcript's timestamps come from the caller's own clock, which is also what
// makes them reproducible in tests.
type Clock interface{ Now() time.Time }

// Writer serializes records onto an underlying sink as JSONL.
//
// Sequence numbers are assigned under the same lock as the write, so they are
// gap-free and establish a total order across interleaved directions even
// though inbound and outbound are observed on different goroutines.
type Writer struct {
	mu       sync.Mutex
	out      io.Writer
	clock    Clock
	sequence uint64
	closed   bool
}

// NewWriter returns a Writer emitting to out. A nil out or clock yields a nil
// Writer, which is a total no-op, so callers can treat "recording disabled" as
// one shape rather than branching at every call site.
func NewWriter(out io.Writer, clock Clock) *Writer {
	if out == nil || clock == nil {
		return nil
	}
	return &Writer{out: out, clock: clock}
}

// Record writes one observed line. It never fails the caller's own work: a
// recording error is reported, but callers are expected to ignore it rather
// than fail a live protocol turn over a diagnostic artifact.
func (w *Writer) Record(conn string, peer Peer, direction Direction, stream Stream, line []byte) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	record := w.record(conn, peer, direction, stream, line, w.sequence+1)
	record.Timestamp = w.clock.Now().UTC().Format(time.RFC3339Nano)
	encoded, err := encodeRecord(record)
	if err != nil {
		return err
	}
	n, err := w.out.Write(encoded)
	if err == nil && n != len(encoded) {
		err = io.ErrShortWrite
	}
	if err == nil {
		w.sequence = record.Sequence
	}
	return err
}

type checkpointableOutput interface {
	Prepare(int) (any, error)
	Rollback(any) error
}

type writerOutboundReservation struct {
	writer         *Writer
	output         checkpointableOutput
	checkpoint     any
	sequenceBefore uint64
	done           bool
}

// BeginOutbound records one complete outbound line before publication when
// the underlying transcript sink supports a reversible checkpoint. The writer
// mutex remains held until Commit or Rollback, so another direction cannot be
// inserted between the reservation and its wire outcome.
func (w *Writer) BeginOutbound(
	conn string,
	peer Peer,
	stream Stream,
	line []byte,
) (OutboundReservation, error) {
	if w == nil {
		return nil, io.ErrClosedPipe
	}
	output, ok := w.out.(checkpointableOutput)
	if !ok {
		return nil, io.ErrClosedPipe
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil, io.ErrClosedPipe
	}
	record := w.record(conn, peer, DirectionOut, stream, line, w.sequence+1)
	record.Timestamp = w.clock.Now().UTC().Format(time.RFC3339Nano)
	encoded, err := encodeRecord(record)
	if err != nil {
		w.mu.Unlock()
		return nil, err
	}
	checkpoint, err := output.Prepare(len(encoded))
	if err != nil {
		w.mu.Unlock()
		return nil, err
	}
	n, err := w.out.Write(encoded)
	if err == nil && n != len(encoded) {
		err = io.ErrShortWrite
	}
	if err != nil {
		_ = output.Rollback(checkpoint)
		w.mu.Unlock()
		return nil, err
	}
	w.sequence = record.Sequence
	return &writerOutboundReservation{
		writer: w, output: output, checkpoint: checkpoint, sequenceBefore: record.Sequence - 1,
	}, nil
}

func (r *writerOutboundReservation) Commit() error {
	if r == nil || r.done {
		return nil
	}
	r.done = true
	r.writer.mu.Unlock()
	return nil
}

func (r *writerOutboundReservation) Rollback() error {
	if r == nil || r.done {
		return nil
	}
	err := r.output.Rollback(r.checkpoint)
	if err == nil {
		r.writer.sequence = r.sequenceBefore
	}
	r.done = true
	r.writer.mu.Unlock()
	return err
}

func (w *Writer) record(
	conn string,
	peer Peer,
	direction Direction,
	stream Stream,
	line []byte,
	sequence uint64,
) Record {
	record := Record{
		Version:   FormatVersion,
		Sequence:  sequence,
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
	return record
}

func encodeRecord(record Record) ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	// Protocol payloads routinely contain <, >, and &. Escaping them would
	// alter the recorded text without altering its meaning, which defeats the
	// byte-for-byte comparison against another agent's transcript.
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(record); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
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
