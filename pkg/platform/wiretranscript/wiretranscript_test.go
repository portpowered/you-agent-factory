package wiretranscript_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/wiretranscript"
)

// TestRecordedFramesAreVerbatim is the invariant everything else rests on. A
// transcript is only comparable against another agent's transcript if the
// recorded bytes are exactly what crossed the wire: re-encoding would reorder
// keys, drop duplicates, and escape characters, erasing the very differences a
// comparison exists to find.
func TestRecordedFramesAreVerbatim(t *testing.T) {
	t.Parallel()

	frames := []string{
		// Non-alphabetical key order must survive.
		`{"zeta":1,"alpha":2}`,
		// Duplicate keys are legal JSON and must not be collapsed.
		`{"dup":1,"dup":2}`,
		// HTML-significant characters must not be escaped.
		`{"text":"a <b> & c"}`,
		// Escape sequences must pass through as written rather than being
		// re-encoded into their literal characters.
		`{"tab":"\t","quote":"\"","backslash":"\\"}`,
		// Unknown fields must not be dropped.
		`{"jsonrpc":"2.0","method":"x","undocumented":{"deep":[1,2,3]}}`,
	}

	var out bytes.Buffer
	writer := wiretranscript.NewWriter(&out, nil)
	for _, frame := range frames {
		if err := writer.Record("c1", wiretranscript.PeerAgent, wiretranscript.DirectionIn,
			wiretranscript.StreamStdout, []byte(frame+"\n")); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	records, err := wiretranscript.ReadAll(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(records) != len(frames) {
		t.Fatalf("records = %d, want %d", len(records), len(frames))
	}
	for index, record := range records {
		if string(record.Frame) != frames[index] {
			t.Fatalf("record[%d].Frame = %s, want verbatim %s", index, record.Frame, frames[index])
		}
	}
}

// TestSequenceIsGapFreeUnderConcurrentWriters proves the total order holds when
// inbound and outbound are recorded from different goroutines, which is how a
// live connection actually uses this.
func TestSequenceIsGapFreeUnderConcurrentWriters(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	writer := wiretranscript.NewWriter(&out, nil)

	const writers, perWriter = 8, 50
	var group sync.WaitGroup
	group.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer group.Done()
			for j := 0; j < perWriter; j++ {
				_ = writer.Record("c1", wiretranscript.PeerClient, wiretranscript.DirectionOut,
					wiretranscript.StreamStdin, []byte(`{"n":1}`+"\n"))
			}
		}()
	}
	group.Wait()

	records, err := wiretranscript.ReadAll(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(records) != writers*perWriter {
		t.Fatalf("records = %d, want %d", len(records), writers*perWriter)
	}
	for index, record := range records {
		if record.Sequence != uint64(index+1) {
			t.Fatalf("record[%d].Sequence = %d, want %d (gap or torn write)", index, record.Sequence, index+1)
		}
		if record.Version != wiretranscript.FormatVersion {
			t.Fatalf("record[%d].Version = %d, want %d", index, record.Version, wiretranscript.FormatVersion)
		}
	}
}

// TestMalformedLineIsRecordedNotDropped matters because a frame the decoder
// rejects is exactly the frame a customer is trying to see.
func TestMalformedLineIsRecordedNotDropped(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	writer := wiretranscript.NewWriter(&out, nil)
	if err := writer.Record("c1", wiretranscript.PeerAgent, wiretranscript.DirectionIn,
		wiretranscript.StreamStdout, []byte("{not json\n")); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	records, err := wiretranscript.ReadAll(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Text != "{not json" {
		t.Fatalf("Text = %q, want the raw line preserved", records[0].Text)
	}
	if records[0].Err == "" {
		t.Fatal("Err is empty, want the parse failure recorded")
	}
	if records[0].Frame != nil {
		t.Fatalf("Frame = %s, want nil for an unparsable line", records[0].Frame)
	}
}

// TestOversizedFrameIsTruncatedNotDropped proves one huge frame cannot consume
// the rotation budget while still leaving evidence it occurred.
func TestOversizedFrameIsTruncatedNotDropped(t *testing.T) {
	t.Parallel()

	huge := `{"blob":"` + strings.Repeat("x", wiretranscript.MaxFrameBytes+2048) + `"}`
	var out bytes.Buffer
	writer := wiretranscript.NewWriter(&out, nil)
	if err := writer.Record("c1", wiretranscript.PeerAgent, wiretranscript.DirectionIn,
		wiretranscript.StreamStdout, []byte(huge+"\n")); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	records, err := wiretranscript.ReadAll(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Err == "" {
		t.Fatal("Err is empty, want the truncation recorded")
	}
	if records[0].Bytes != len(huge)+1 {
		t.Fatalf("Bytes = %d, want the original length %d", records[0].Bytes, len(huge)+1)
	}
	if len(records[0].Text) != wiretranscript.MaxFrameBytes {
		t.Fatalf("retained %d bytes, want the %d-byte cap", len(records[0].Text), wiretranscript.MaxFrameBytes)
	}
}

// TestTeeReaderPassesBytesThroughUnchanged proves the tee is transparent: a
// recorder must never alter what the protocol layer reads.
func TestTeeReaderPassesBytesThroughUnchanged(t *testing.T) {
	t.Parallel()

	payload := `{"a":1}` + "\n" + `{"b":2}` + "\n" + `{"partial":3}`
	var out bytes.Buffer
	writer := wiretranscript.NewWriter(&out, nil)

	reader := wiretranscript.TeeReader(strings.NewReader(payload), writer,
		"c1", wiretranscript.PeerClient, wiretranscript.StreamStdin)
	read, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(tee) error = %v", err)
	}
	if string(read) != payload {
		t.Fatalf("tee altered the stream:\n got %q\nwant %q", read, payload)
	}

	records, err := wiretranscript.ReadAll(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("ReadAll(transcript) error = %v", err)
	}
	// Two complete lines plus the partial trailing frame, which must not be
	// silently discarded when the stream ends mid-line.
	if len(records) != 3 {
		t.Fatalf("records = %d, want 3 (two complete frames and one partial)", len(records))
	}
	if string(records[2].Frame) != `{"partial":3}` {
		t.Fatalf("trailing record = %s, want the partial frame recorded", records[2].Frame)
	}
}

// TestTeeWriterPassesBytesThroughUnchanged is the outbound counterpart.
func TestTeeWriterPassesBytesThroughUnchanged(t *testing.T) {
	t.Parallel()

	var sink bytes.Buffer
	var out bytes.Buffer
	writer := wiretranscript.NewWriter(&out, nil)
	tee := wiretranscript.TeeWriter(&sink, writer, "c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout)

	frames := []string{`{"id":1,"result":{}}` + "\n", `{"method":"session/update"}` + "\n"}
	for _, frame := range frames {
		if _, err := tee.Write([]byte(frame)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if sink.String() != strings.Join(frames, "") {
		t.Fatalf("tee altered the sink:\n got %q\nwant %q", sink.String(), strings.Join(frames, ""))
	}

	records, err := wiretranscript.ReadAll(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}
	for _, record := range records {
		if record.Direction != wiretranscript.DirectionOut || record.Peer != wiretranscript.PeerAgent {
			t.Fatalf("record = %+v, want agent/out attribution", record)
		}
	}
}

// TestNilRecorderIsATotalNoOp keeps "recording disabled" from needing a second
// code path at every call site.
func TestNilRecorderIsATotalNoOp(t *testing.T) {
	t.Parallel()

	if got := wiretranscript.NewWriter(nil, nil); got != nil {
		t.Fatal("NewWriter(nil) returned a non-nil writer")
	}
	var writer *wiretranscript.Writer
	if err := writer.Record("c", wiretranscript.PeerAgent, wiretranscript.DirectionIn,
		wiretranscript.StreamStdout, []byte("{}\n")); err != nil {
		t.Fatalf("nil Writer.Record() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("nil Writer.Close() error = %v", err)
	}

	source := strings.NewReader("payload")
	if wiretranscript.TeeReader(source, nil, "c", wiretranscript.PeerClient, wiretranscript.StreamStdin) != io.Reader(source) {
		t.Fatal("TeeReader with a nil recorder wrapped the source")
	}
}

// TestEnvelopeDoesNotEscapeHTML pins that the record's own string fields are
// written literally, matching the frame passthrough.
func TestEnvelopeDoesNotEscapeHTML(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	writer := wiretranscript.NewWriter(&out, nil)
	if err := writer.Record("c1", wiretranscript.PeerAgent, wiretranscript.DirectionIn,
		wiretranscript.StreamStderr, []byte("warn <a> & <b>\n")); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if strings.Contains(out.String(), `\u003c`) || strings.Contains(out.String(), `\u0026`) {
		t.Fatalf("transcript escaped HTML characters: %s", out.String())
	}
	if !strings.Contains(out.String(), `<a> & <b>`) {
		t.Fatalf("transcript did not retain the literal text: %s", out.String())
	}
	var decoded wiretranscript.Record
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &decoded); err != nil {
		t.Fatalf("decode record: %v", err)
	}
	if decoded.Text != "warn <a> & <b>" {
		t.Fatalf("Text = %q, want the literal stderr line", decoded.Text)
	}
}
