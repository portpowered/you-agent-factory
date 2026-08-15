package wiretranscript_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/rollingfile"
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
	writer := wiretranscript.NewWriter(&out, testClock{})
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
	writer := wiretranscript.NewWriter(&out, testClock{})

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
	writer := wiretranscript.NewWriter(&out, testClock{})
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
	writer := wiretranscript.NewWriter(&out, testClock{})
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
	writer := wiretranscript.NewWriter(&out, testClock{})

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
	writer := wiretranscript.NewWriter(&out, testClock{})
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

func TestTeeWriterRollsBackRejectedFrameWithRollingTranscript(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	rolling := &rollingfile.Writer{Filename: path, MaxSize: 1}
	writer := wiretranscript.NewWriter(rolling, testClock{})
	tee := wiretranscript.TeeWriter(shortWriteSink{}, writer, "c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout)

	frame := []byte(`{"id":1}` + "\n")
	n, err := tee.Write(frame)
	if err != nil {
		t.Fatalf("TeeWriter.Write() error = %v, want the sink result", err)
	}
	if n != len(frame)-1 {
		t.Fatalf("TeeWriter.Write() bytes = %d, want %d", n, len(frame)-1)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rejected frame left transcript file, stat error = %v", err)
	}
}

func TestTeeWriterPublishesAfterRollingRotationRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	const maxBytes = 1024 * 1024
	seed := bytes.Repeat([]byte("x"), maxBytes-1)
	if err := os.WriteFile(path, seed, 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	rolling := &rollingfile.Writer{
		Filename: path,
		MaxSize:  1,
	}
	writer := wiretranscript.NewWriter(rolling, testClock{})
	sink := &rotationObservingSink{path: path, want: []byte(`"frame":{"id":1}`)}
	tee := wiretranscript.TeeWriter(sink, writer, "c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout)
	frame := []byte(`{"id":1}` + "\n")

	n, err := tee.Write(frame)
	if err != nil {
		t.Fatalf("TeeWriter.Write() error = %v", err)
	}
	if n != len(frame) || string(sink.out.Bytes()) != string(frame) {
		t.Fatalf("published bytes = (%d, %q), want (%d, %q)", n, sink.out.Bytes(), len(frame), frame)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}
	active, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(active): %v", err)
	}
	if !bytes.Contains(active, sink.want) {
		t.Fatalf("active transcript = %q, want the outbound record", active)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "transcript-*.jsonl"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("rotation backups = (%v, %v), want one backup", backups, err)
	}
	if got, err := os.ReadFile(backups[0]); err != nil || !bytes.Equal(got, seed) {
		t.Fatalf("rotation backup = (%q, %v), want the prefilled seed", got, err)
	}
}

func TestTeeWriterRollsBackErroringOutboundWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	rolling := &rollingfile.Writer{Filename: path, MaxSize: 1}
	writer := wiretranscript.NewWriter(rolling, testClock{})
	sinkErr := errors.New("sink failed")
	tee := wiretranscript.TeeWriter(errorSink{err: sinkErr}, writer, "c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout)

	n, err := tee.Write([]byte(`{"id":1}` + "\n"))
	if n != 0 || !errors.Is(err, sinkErr) {
		t.Fatalf("TeeWriter.Write() = (%d, %v), want (0, %v)", n, err, sinkErr)
	}
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatalf("writer.Close() error = %v", closeErr)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("erroring write left transcript, stat error = %v", statErr)
	}
}

func TestTeeWriterCommitsAcceptedTrailingBytes(t *testing.T) {
	recorder := &testOutboundRecorder{}
	sink := &partialTeeSink{}
	tee := wiretranscript.TeeWriter(sink, recorder, "c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout)

	first := []byte(`{"prefix":1`)
	second := []byte("}\ntrailing")
	third := []byte("ailing\n")
	if _, err := tee.Write(first); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}
	if _, err := tee.Write(second); err != nil {
		t.Fatalf("second Write() error = %v", err)
	}
	if _, err := tee.Write(third); err != nil {
		t.Fatalf("third Write() error = %v", err)
	}
	if got := string(sink.out.Bytes()); got != string(append(append(first, second[:4]...), third...)) {
		t.Fatalf("sink bytes = %q, want accepted bytes", got)
	}
	if len(recorder.lines) != 2 || string(recorder.lines[0]) != string(append(first, second[:2]...)) || string(recorder.lines[1]) != "trailing\n" {
		t.Fatalf("recorded lines = %q, want complete prefix and trailing lines", recorder.lines)
	}
}

func TestTeeWriterSurfacesCommitAndRollbackErrors(t *testing.T) {
	commitErr := errors.New("commit failed")
	commitRecorder := &testOutboundRecorder{commitErr: commitErr}
	commitSink := &bytes.Buffer{}
	commitTee := wiretranscript.TeeWriter(commitSink, commitRecorder, "c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout)
	frame := []byte(`{"id":1}` + "\n")
	if n, err := commitTee.Write(frame); n != len(frame) || !errors.Is(err, commitErr) {
		t.Fatalf("commit Write() = (%d, %v), want accepted bytes and %v", n, err, commitErr)
	}

	rollbackErr := errors.New("rollback failed")
	rollbackRecorder := &testOutboundRecorder{rollbackErr: rollbackErr}
	rollbackTee := wiretranscript.TeeWriter(shortWriteSink{}, rollbackRecorder, "c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout)
	if n, err := rollbackTee.Write(frame); n != len(frame)-1 || !errors.Is(err, rollbackErr) {
		t.Fatalf("rollback Write() = (%d, %v), want short write and %v", n, err, rollbackErr)
	}
	if len(rollbackRecorder.lines) != 0 {
		t.Fatalf("rolled-back lines = %q, want none", rollbackRecorder.lines)
	}
}

type shortWriteSink struct{}

func (shortWriteSink) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

type errorSink struct{ err error }

func (s errorSink) Write([]byte) (int, error) { return 0, s.err }

type rotationObservingSink struct {
	path string
	want []byte
	out  bytes.Buffer
}

func (s *rotationObservingSink) Write(p []byte) (int, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return 0, err
	}
	if !bytes.Contains(data, s.want) {
		return 0, errors.New("outbound bytes exposed before rotation transcript record")
	}
	return s.out.Write(p)
}

type partialTeeSink struct {
	out   bytes.Buffer
	calls int
}

func (s *partialTeeSink) Write(p []byte) (int, error) {
	s.calls++
	n := len(p)
	if s.calls == 2 {
		n = 4
	}
	_, _ = s.out.Write(p[:n])
	return n, nil
}

type testOutboundRecorder struct {
	lines       [][]byte
	commitErr   error
	rollbackErr error
}

func (r *testOutboundRecorder) Record(_ string, _ wiretranscript.Peer, _ wiretranscript.Direction, _ wiretranscript.Stream, line []byte) error {
	r.lines = append(r.lines, append([]byte(nil), line...))
	return nil
}

func (r *testOutboundRecorder) BeginOutbound(_ string, _ wiretranscript.Peer, _ wiretranscript.Stream, line []byte) (wiretranscript.OutboundReservation, error) {
	r.lines = append(r.lines, append([]byte(nil), line...))
	return &testOutboundReservation{recorder: r, index: len(r.lines) - 1}, nil
}

type testOutboundReservation struct {
	recorder *testOutboundRecorder
	index    int
	done     bool
}

func (r *testOutboundReservation) Commit() error {
	if r.done {
		return nil
	}
	r.done = true
	return r.recorder.commitErr
}

func (r *testOutboundReservation) Rollback() error {
	if r.done {
		return nil
	}
	r.done = true
	r.recorder.lines = append(r.recorder.lines[:r.index], r.recorder.lines[r.index+1:]...)
	return r.recorder.rollbackErr
}

// TestNilRecorderIsATotalNoOp keeps "recording disabled" from needing a second
// code path at every call site.
func TestNilRecorderIsATotalNoOp(t *testing.T) {
	t.Parallel()

	if got := wiretranscript.NewWriter(nil, testClock{}); got != nil {
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
	writer := wiretranscript.NewWriter(&out, testClock{})
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

func TestOpenerDefaultsAndValidatesInputs(t *testing.T) {
	if got := wiretranscript.Root("/home/andre"); got != filepath.Join("/home/andre", ".you-agent-factory", "acp-wire") {
		t.Fatalf("Root() = %q, want ACP wire root", got)
	}
	if got := wiretranscript.DefaultConfig(); got.MaxSizeMB != 32 || got.MaxBackups != 4 || got.MaxAgeDays != 7 || !got.Compress {
		t.Fatalf("DefaultConfig() = %+v, want documented defaults", got)
	}

	if opener, err := wiretranscript.NewOpener(nil, testClock{}); opener != nil || err == nil {
		t.Fatalf("NewOpener(nil paths) = (%v, %v), want error", opener, err)
	}
	paths := &staticPathReserver{}
	if opener, err := wiretranscript.NewOpener(paths, nil); opener != nil || err == nil {
		t.Fatalf("NewOpener(nil clock) = (%v, %v), want error", opener, err)
	}
	opener, err := wiretranscript.NewOpener(paths, testClock{})
	if err != nil {
		t.Fatalf("NewOpener() error = %v", err)
	}
	if _, err := opener.Open(wiretranscript.OpeningRequest{ConnectionID: "c1"}); err == nil {
		t.Fatal("Open(empty root) error = nil, want validation error")
	}
	if _, err := opener.Open(wiretranscript.OpeningRequest{RootDirectory: "root"}); err == nil {
		t.Fatal("Open(empty connection) error = nil, want validation error")
	}
	var nilOpener *wiretranscript.Opener
	if _, err := nilOpener.Open(wiretranscript.OpeningRequest{}); err == nil {
		t.Fatal("nil Opener.Open() error = nil, want validation error")
	}
	var nilSink *wiretranscript.Sink
	if got := nilSink.Path(); got != "" {
		t.Fatalf("nil Sink.Path() = %q, want empty", got)
	}
}

func TestOpenerCreatesReadableTranscriptSink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	paths := &staticPathReserver{path: path}
	opener, err := wiretranscript.NewOpener(paths, testClock{})
	if err != nil {
		t.Fatalf("NewOpener() error = %v", err)
	}
	sink, err := opener.Open(wiretranscript.OpeningRequest{
		RootDirectory: t.TempDir(), ConnectionID: "c1", StartTimeUTC: time.Time{},
		Config: wiretranscript.Config{},
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if sink.Path() != path || paths.root == "" || paths.kind != wiretranscript.ArtifactKind || paths.suffix != "c1" {
		t.Fatalf("Open() path request = %+v, want configured transcript reservation", paths)
	}
	if err := sink.Record("c1", wiretranscript.PeerClient, wiretranscript.DirectionIn, wiretranscript.StreamStdin, []byte(`{"id":1}`+"\n")); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(data, []byte(`"id":1`)) {
		t.Fatalf("transcript = %q, want recorded frame", data)
	}
}

func TestOutboundReservationLifecycleRejectsUnsupportedAndClosedWriters(t *testing.T) {
	var nilWriter *wiretranscript.Writer
	if _, err := nilWriter.BeginOutbound("c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout, []byte("{}\n")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("nil BeginOutbound() error = %v, want %v", err, io.ErrClosedPipe)
	}
	unsupported := wiretranscript.NewWriter(&bytes.Buffer{}, testClock{})
	if _, err := unsupported.BeginOutbound("c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout, []byte("{}\n")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("unsupported BeginOutbound() error = %v, want %v", err, io.ErrClosedPipe)
	}

	commitPath := filepath.Join(t.TempDir(), "commit.jsonl")
	commitWriter := wiretranscript.NewWriter(&rollingfile.Writer{Filename: commitPath, MaxSize: 1}, testClock{})
	reservation, err := commitWriter.BeginOutbound("c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout, []byte(`{"id":1}`+"\n"))
	if err != nil {
		t.Fatalf("BeginOutbound(commit) error = %v", err)
	}
	if err := reservation.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if err := reservation.Commit(); err != nil {
		t.Fatalf("second Commit() error = %v", err)
	}
	if err := reservation.Rollback(); err != nil {
		t.Fatalf("Rollback(after Commit) error = %v", err)
	}
	if err := commitWriter.Close(); err != nil {
		t.Fatalf("commitWriter.Close() error = %v", err)
	}

	rollbackPath := filepath.Join(t.TempDir(), "rollback.jsonl")
	rollbackWriter := wiretranscript.NewWriter(&rollingfile.Writer{Filename: rollbackPath, MaxSize: 1}, testClock{})
	reservation, err = rollbackWriter.BeginOutbound("c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout, []byte(`{"id":2}`+"\n"))
	if err != nil {
		t.Fatalf("BeginOutbound(rollback) error = %v", err)
	}
	if err := reservation.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if err := reservation.Rollback(); err != nil {
		t.Fatalf("second Rollback() error = %v", err)
	}
	if err := rollbackWriter.Close(); err != nil {
		t.Fatalf("rollbackWriter.Close() error = %v", err)
	}
	if _, err := os.Stat(rollbackPath); !os.IsNotExist(err) {
		t.Fatalf("rolled-back writer path exists, stat error = %v", err)
	}

	closedPath := filepath.Join(t.TempDir(), "closed.jsonl")
	closedWriter := wiretranscript.NewWriter(&rollingfile.Writer{Filename: closedPath, MaxSize: 1}, testClock{})
	if err := closedWriter.Close(); err != nil {
		t.Fatalf("closedWriter.Close() error = %v", err)
	}
	if _, err := closedWriter.BeginOutbound("c1", wiretranscript.PeerAgent, wiretranscript.StreamStdout, []byte("{}\n")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("closed BeginOutbound() error = %v, want %v", err, io.ErrClosedPipe)
	}
}

func TestOpenerPropagatesReservationError(t *testing.T) {
	reserveErr := errors.New("reservation failed")
	opener, err := wiretranscript.NewOpener(&staticPathReserver{err: reserveErr}, testClock{})
	if err != nil {
		t.Fatalf("NewOpener() error = %v", err)
	}
	_, err = opener.Open(wiretranscript.OpeningRequest{RootDirectory: "root", ConnectionID: "c1"})
	if !errors.Is(err, reserveErr) {
		t.Fatalf("Open() error = %v, want %v", err, reserveErr)
	}
}

// testClock is a fixed clock; these cells assert structure and ordering, never
// wall-clock values.
type testClock struct{}

func (testClock) Now() time.Time { return time.Unix(1700000000, 0).UTC() }

type staticPathReserver struct {
	path   string
	err    error
	root   string
	at     time.Time
	kind   string
	suffix string
}

func (r *staticPathReserver) Reserve(root string, at time.Time, kind, suffix string) (string, error) {
	r.root, r.at, r.kind, r.suffix = root, at, kind, suffix
	if r.err != nil {
		return "", r.err
	}
	return r.path, nil
}
