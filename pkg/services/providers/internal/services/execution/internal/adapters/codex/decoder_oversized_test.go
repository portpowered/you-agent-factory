package codex

import (
	"strings"
	"testing"
)

// TestDecoderSkipsOversizedRecordWithoutBuffering pins the memory-safety
// property of the skip path directly on the decoder: while an over-limit line
// streams through in small chunks, the decoder never retains more than
// maxRecordBytes, and once the limit trips it holds nothing at all -- it
// discards the rest of the line without buffering and keeps decoding.
func TestDecoderSkipsOversizedRecordWithoutBuffering(t *testing.T) {
	t.Parallel()

	decoder := newDecoder(nil)
	observe := func(chunk []byte) {
		t.Helper()
		if err := decoder.observe(chunk); err != nil {
			t.Fatalf("observe() error = %v", err)
		}
		if len(decoder.pending) > maxRecordBytes {
			t.Fatalf("decoder buffered %d bytes, want at most %d", len(decoder.pending), maxRecordBytes)
		}
	}

	observe([]byte(`{"type":"thread.started","thread_id":"thread-bounded-skip"}` + "\n"))
	oversized := []byte(
		`{"type":"item.completed","item":{"id":"tool-1","type":"command_execution","aggregated_output":"` +
			strings.Repeat("x", int(float64(maxRecordBytes)*1.5)) + `"}}` + "\n",
	)
	for len(oversized) > 0 {
		size := 32 << 10
		if size > len(oversized) {
			size = len(oversized)
		}
		observe(oversized[:size])
		oversized = oversized[size:]
		if decoder.discardLine && len(decoder.pending) != 0 {
			t.Fatalf("decoder retained %d bytes while discarding an oversized line", len(decoder.pending))
		}
	}
	observe([]byte(`{"type":"item.completed","item":{"id":"message-final","type":"agent_message","text":"<CONTINUE>"}}` + "\n"))

	if err := decoder.flush(); err != nil {
		t.Fatalf("flush() error = %v", err)
	}
	assertRecoveredAfterSkip(t, decoder)
}

func assertRecoveredAfterSkip(t *testing.T, decoder *decoder) {
	t.Helper()
	content, session, err := decoder.final()
	if err != nil {
		t.Fatalf("final() error = %v, want recovered decision after skipped record", err)
	}
	if content != "<CONTINUE>" {
		t.Fatalf("final() content = %q, want <CONTINUE>", content)
	}
	if session == nil || session.ID != "thread-bounded-skip" {
		t.Fatalf("final() session = %#v, want observed thread", session)
	}
	if decoder.limit != nil {
		t.Fatalf("decoder.limit = %#v, want no hard stream limit for a skipped record", decoder.limit)
	}
	if decoder.recordSkips != 1 || decoder.skippedRecord == nil || decoder.skippedRecord.line != 2 {
		t.Fatalf("recordSkips = %d, skippedRecord = %#v, want one skip at line 2",
			decoder.recordSkips, decoder.skippedRecord)
	}
	if decoder.resourceFailure() != nil {
		t.Fatal("resourceFailure() != nil, want no whole-execution failure for a skipped record")
	}
}
