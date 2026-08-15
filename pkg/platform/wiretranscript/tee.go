package wiretranscript

import (
	"bufio"
	"bytes"
	"io"
)

// Recorder is the sink a tee reports observed lines to.
type Recorder interface {
	Record(conn string, peer Peer, direction Direction, stream Stream, line []byte) error
}

// OutboundReservation is a transcript record that has been made visible before
// the corresponding wire write. A failed or short write rolls the reservation
// back so the transcript never claims bytes crossed the wire when they did not.
type OutboundReservation interface {
	Commit() error
	Rollback() error
}

// OutboundRecorder optionally supplies the transactional boundary needed by a
// live ACP output sink. Recorders that cannot roll back a pre-publication
// record use TeeWriter's truthful post-write fallback instead.
type OutboundRecorder interface {
	Recorder
	BeginOutbound(conn string, peer Peer, stream Stream, line []byte) (OutboundReservation, error)
}

// TeeReader returns a reader that passes bytes through unchanged while
// recording each complete newline-delimited line it observes.
//
// Wrapping the pipe rather than hooking individual decode sites is deliberate:
// it captures every byte the peer sent, including frames a decoder later
// rejects and any future path that bypasses today's funnels.
func TeeReader(source io.Reader, recorder Recorder, conn string, peer Peer, stream Stream) io.Reader {
	if recorder == nil {
		return source
	}
	return &teeReader{
		source:   source,
		recorder: recorder,
		conn:     conn,
		peer:     peer,
		stream:   stream,
	}
}

type teeReader struct {
	source   io.Reader
	recorder Recorder
	conn     string
	peer     Peer
	stream   Stream
	pending  bytes.Buffer
}

func (t *teeReader) Read(p []byte) (int, error) {
	n, err := t.source.Read(p)
	if n > 0 {
		t.pending.Write(p[:n])
		t.drainCompleteLines()
	}
	if err != nil {
		// A stream that ends mid-line still recorded real bytes; report them
		// rather than discarding a partial final frame.
		if remaining := t.pending.Bytes(); len(remaining) > 0 {
			_ = t.recorder.Record(t.conn, t.peer, DirectionIn, t.stream, append([]byte(nil), remaining...))
			t.pending.Reset()
		}
	}
	return n, err
}

func (t *teeReader) drainCompleteLines() {
	for {
		buffered := t.pending.Bytes()
		index := bytes.IndexByte(buffered, '\n')
		if index < 0 {
			return
		}
		line := make([]byte, index+1)
		_, _ = t.pending.Read(line)
		_ = t.recorder.Record(t.conn, t.peer, DirectionIn, t.stream, line)
	}
}

// TeeWriter returns a writer that forwards bytes unchanged while recording
// each complete newline-delimited line written through it.
//
// When the recorder supports an outbound reservation, recording happens before
// forwarding the bytes and the reservation commits only after the sink accepts
// the complete line. A non-transactional recorder records accepted bytes after
// forwarding, because it cannot safely retract a record from a short or failed
// write.
func TeeWriter(sink io.Writer, recorder Recorder, conn string, peer Peer, stream Stream) io.Writer {
	if recorder == nil {
		return sink
	}
	return &teeWriter{
		sink:     sink,
		recorder: recorder,
		conn:     conn,
		peer:     peer,
		stream:   stream,
	}
}

type teeWriter struct {
	sink     io.Writer
	recorder Recorder
	conn     string
	peer     Peer
	stream   Stream
	pending  bytes.Buffer
}

func (t *teeWriter) Write(p []byte) (int, error) {
	if reservation, prefix, trailing, ok := t.beginOutboundReservation(p); ok {
		n, err := t.sink.Write(p)
		if n >= prefix {
			if commitErr := reservation.Commit(); commitErr != nil && err == nil {
				err = commitErr
			}
			t.pending.Reset()
			acceptedTrailing := n - prefix
			if acceptedTrailing > len(trailing) {
				acceptedTrailing = len(trailing)
			}
			if acceptedTrailing > 0 {
				t.pending.Write(trailing[:acceptedTrailing])
			}
		} else {
			if rollbackErr := reservation.Rollback(); rollbackErr != nil && err == nil {
				err = rollbackErr
			}
			if n > 0 {
				t.pending.Write(p[:n])
			}
			t.drainCompleteLines()
		}
		return n, err
	}

	n, err := t.sink.Write(p)
	if n > 0 {
		t.pending.Write(p[:n])
		t.drainCompleteLines()
	}
	return n, err
}

func (t *teeWriter) beginOutboundReservation(p []byte) (OutboundReservation, int, []byte, bool) {
	recorder, ok := t.recorder.(OutboundRecorder)
	if !ok || len(p) == 0 {
		return nil, 0, nil, false
	}
	previous := append([]byte(nil), t.pending.Bytes()...)
	buffered := append([]byte(nil), previous...)
	buffered = append(buffered, p...)
	firstLineEnd := bytes.IndexByte(buffered, '\n')
	if firstLineEnd < 0 {
		return nil, 0, nil, false
	}
	// One reservation is enough for the ACP frame writers, which issue one
	// newline-terminated frame per Write. A multi-frame Write falls back to
	// the accepted-byte path so no record can be left half-committed.
	if bytes.IndexByte(buffered[firstLineEnd+1:], '\n') >= 0 {
		return nil, 0, nil, false
	}
	line := append([]byte(nil), buffered[:firstLineEnd+1]...)
	prefix := firstLineEnd + 1 - len(previous)
	if prefix < 0 || prefix > len(p) {
		return nil, 0, nil, false
	}
	reservation, err := recorder.BeginOutbound(t.conn, t.peer, t.stream, line)
	if err != nil || reservation == nil {
		return nil, 0, nil, false
	}
	return reservation, prefix, append([]byte(nil), buffered[firstLineEnd+1:]...), true
}

func (t *teeWriter) drainCompleteLines() {
	for {
		buffered := t.pending.Bytes()
		index := bytes.IndexByte(buffered, '\n')
		if index < 0 {
			return
		}
		line := make([]byte, index+1)
		_, _ = t.pending.Read(line)
		_ = t.recorder.Record(t.conn, t.peer, DirectionOut, t.stream, line)
	}
}

// ReadAll decodes every record in a transcript. It uses a Reader rather than a
// Scanner so an arbitrarily large recorded frame is readable back, matching
// the writer's own willingness to record one.
func ReadAll(source io.Reader) ([]Record, error) {
	reader := bufio.NewReader(source)
	var records []Record
	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var record Record
			if decodeErr := unmarshalRecord(line, &record); decodeErr != nil {
				return records, decodeErr
			}
			records = append(records, record)
		}
		if err == io.EOF {
			return records, nil
		}
		if err != nil {
			return records, err
		}
	}
}
