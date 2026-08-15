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
// Recording deliberately happens before forwarding the bytes. A writer's
// sink can make bytes readable by another goroutine before Write returns, so
// forwarding first lets a consumer observe a complete response before its
// transcript record exists. Record is synchronous and therefore acts as the
// publication barrier for each complete outbound line.
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
	if len(p) > 0 {
		t.pending.Write(p)
		for {
			buffered := t.pending.Bytes()
			index := bytes.IndexByte(buffered, '\n')
			if index < 0 {
				break
			}
			line := make([]byte, index+1)
			_, _ = t.pending.Read(line)
			_ = t.recorder.Record(t.conn, t.peer, DirectionOut, t.stream, line)
		}
	}
	return t.sink.Write(p)
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
