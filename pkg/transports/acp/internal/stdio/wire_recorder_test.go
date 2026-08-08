package stdio

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/wiretranscript"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
)

// capturingTranscript records what the server hands the recorder.
type capturingTranscript struct {
	mu      sync.Mutex
	records []wiretranscript.Record
	closed  bool
	failing bool
}

func (t *capturingTranscript) Record(
	conn string,
	peer wiretranscript.Peer,
	direction wiretranscript.Direction,
	stream wiretranscript.Stream,
	line []byte,
) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = append(t.records, wiretranscript.Record{
		Conn: conn, Peer: peer, Direction: direction, Stream: stream,
		Bytes: len(line), Text: string(line),
	})
	return nil
}

func (t *capturingTranscript) Path() string { return "/dev/null/transcript" }

func (t *capturingTranscript) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func (t *capturingTranscript) snapshot() ([]wiretranscript.Record, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]wiretranscript.Record(nil), t.records...), t.closed
}

// TestServeRecordsBothDirectionsIncludingRejectedFrames proves the recorder
// sees the inbound line even when the transport rejects it, and the outbound
// error frame it produces. A frame the decoder refuses is exactly what a
// customer opens the transcript to find.
func TestServeRecordsBothDirectionsIncludingRejectedFrames(t *testing.T) {
	t.Parallel()

	transcript := &capturingTranscript{}
	server := New(nil, nil, nil, nil, nil, nil, nil,
		acp.WireRecorder(func(string) (acp.WireTranscript, error) { return transcript, nil }))

	var out strings.Builder
	input := strings.NewReader("this is not json\n")
	if err := server.Serve(context.Background(), input, &out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	records, closed := transcript.snapshot()
	if !closed {
		t.Fatal("transcript was not closed when the connection ended")
	}

	var sawInbound, sawOutbound bool
	for _, record := range records {
		switch record.Direction {
		case wiretranscript.DirectionIn:
			if strings.Contains(record.Text, "this is not json") {
				sawInbound = true
			}
			if record.Peer != wiretranscript.PeerClient || record.Stream != wiretranscript.StreamStdin {
				t.Fatalf("inbound record attribution = %+v", record)
			}
		case wiretranscript.DirectionOut:
			if strings.Contains(record.Text, "Parse error") {
				sawOutbound = true
			}
			if record.Peer != wiretranscript.PeerAgent || record.Stream != wiretranscript.StreamStdout {
				t.Fatalf("outbound record attribution = %+v", record)
			}
		}
	}
	if !sawInbound {
		t.Fatalf("the rejected inbound line was not recorded; got %+v", records)
	}
	if !sawOutbound {
		t.Fatalf("the outbound error frame was not recorded; got %+v", records)
	}
}

// TestServeWithoutARecorderIsUnchanged keeps recording strictly additive: a
// server built without one must behave exactly as before.
func TestServeWithoutARecorderIsUnchanged(t *testing.T) {
	t.Parallel()

	server := New(nil, nil, nil, nil, nil, nil, nil, nil)
	var out strings.Builder
	if err := server.Serve(context.Background(), strings.NewReader("this is not json\n"), &out); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	if !strings.Contains(out.String(), "Parse error") {
		t.Fatalf("stdout = %q, want the ordinary parse-error frame", out.String())
	}
}

// TestServeSurvivesARecorderThatCannotOpen proves a diagnostic artifact never
// costs a customer their session.
func TestServeSurvivesARecorderThatCannotOpen(t *testing.T) {
	t.Parallel()

	server := New(nil, nil, nil, nil, nil, nil, nil,
		acp.WireRecorder(func(string) (acp.WireTranscript, error) {
			return nil, context.DeadlineExceeded
		}))

	var out strings.Builder
	if err := server.Serve(context.Background(), strings.NewReader("this is not json\n"), &out); err != nil {
		t.Fatalf("Serve() error = %v, want the connection to continue without recording", err)
	}
	if !strings.Contains(out.String(), "Parse error") {
		t.Fatalf("stdout = %q, want the ordinary parse-error frame", out.String())
	}
}
