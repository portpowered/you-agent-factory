package run

import (
	"context"
	"encoding/json"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)
type gatedResponseStreamWriter struct {
	mu                   sync.Mutex
	blocked              bool
	buf                  strings.Builder
	blockedWriteAttempts atomic.Int64
}

func (w *gatedResponseStreamWriter) block() {
	w.mu.Lock()
	w.blocked = true
	w.mu.Unlock()
}

func (w *gatedResponseStreamWriter) release() {
	w.mu.Lock()
	w.blocked = false
	w.mu.Unlock()
}

func (w *gatedResponseStreamWriter) Write(p []byte) (int, error) {
	for {
		w.mu.Lock()
		blocked := w.blocked
		w.mu.Unlock()
		if !blocked {
			return w.buf.Write(p)
		}
		w.blockedWriteAttempts.Add(1)
		time.Sleep(1 * time.Millisecond)
	}
}

func (w *gatedResponseStreamWriter) String() string {
	return w.buf.String()
}

func (w *gatedResponseStreamWriter) blockedWriteAttemptsCount() int64 {
	if w == nil {
		return 0
	}
	return w.blockedWriteAttempts.Load()
}

func floodResponseStreamProgress(sink responseStreamEventSink, count int) {
	for i := 0; i < count; i++ {
		sink.onStreamSegment(responsestream.ReadResult{
			Events: []responsestream.Event{
				{
					Sequence:   int64(i + 1),
					Kind:       responsestream.EventKindProgressFragment,
					Type:       responsestream.EventTypeProgress,
					DispatchID: "dispatch-1",
					Payload:    "working",
				},
			},
		})
	}
}

var responseStreamBacklogSuccessResult = apisurface.FactoryInvocationResult{
	Status: factoryapi.InvocationTerminalStatusCompleted,
	PrimaryResult: []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "goal completed"},
	},
}

func TestResponseStreamProgressWriter_EnqueueDoesNotBlockWhenOutputSlow(t *testing.T) {
	t.Parallel()

	output := &gatedResponseStreamWriter{}
	output.block()
	writer := newResponseStreamProgressWriter(output)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < defaultResponseStreamProgressQueueCapacity+8; i++ {
			writer.enqueue([]byte("line"))
		}
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("enqueue blocked while terminal output was slow")
	}

	if got := writer.droppedProgressLines(); got == 0 {
		t.Fatalf("dropped lines = 0, want backlog drops when queue is full")
	}

	output.release()
	writer.stopAndDrain()
}

func TestHumanResponseStreamRenderer_RendersOrderedProgressAndSeparatesPrimaryResult(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	renderer.onStreamSegment(responsestream.ReadResult{
		Events: []responsestream.Event{
			{
				Sequence:   1,
				Kind:       responsestream.EventKindProgressFragment,
				Type:       responsestream.EventTypeProgress,
				DispatchID: "dispatch-1",
				Payload:    "planning",
			},
			{
				Sequence:   2,
				Kind:       responsestream.EventKindProgressFragment,
				Type:       responsestream.EventTypeProgress,
				DispatchID: "dispatch-1",
				Payload:    "reviewing",
			},
		},
	})
	renderer.onStreamSegment(responsestream.ReadResult{
		Events: []responsestream.Event{
			{
				Sequence:   3,
				Kind:       responsestream.EventKindResponseFragment,
				Type:       responsestream.EventTypeTextDelta,
				DispatchID: "dispatch-1",
				Payload:    "goal completed",
			},
		},
	})

	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status: factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "goal completed"},
		},
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	got := output.String()
	wantProgress := []string{
		"planning",
		"reviewing",
	}
	for _, line := range wantProgress {
		if !strings.Contains(got, line) {
			t.Fatalf("output missing progress line %q:\n%s", line, got)
		}
	}
	planningIdx := strings.Index(got, wantProgress[0])
	reviewingIdx := strings.Index(got, wantProgress[1])
	if planningIdx < 0 || reviewingIdx < 0 || planningIdx > reviewingIdx {
		t.Fatalf("progress lines out of order:\n%s", got)
	}
	beforePrimary := got
	if idx := strings.Index(got, responseStreamPrimaryResultHeader); idx >= 0 {
		beforePrimary = got[:idx]
	}
	if strings.Contains(beforePrimary, "goal completed") {
		t.Fatalf("response fragment leaked into progress output:\n%s", got)
	}
	if !strings.Contains(got, responseStreamPrimaryResultHeader) {
		t.Fatalf("output missing primary-result header:\n%s", got)
	}
	if !strings.HasSuffix(got, "goal completed") {
		t.Fatalf("output = %q, want suffix primary result", got)
	}
}

func TestHumanResponseStreamRenderer_SkipsDuplicateSequencesAndBoundsPayload(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	longPayload := strings.Repeat("x", maxHumanProgressLineBytes+32)
	renderer.onStreamSegment(responsestream.ReadResult{
		Events: []responsestream.Event{
			{
				Sequence:   1,
				Kind:       responsestream.EventKindProgressFragment,
				Type:       responsestream.EventTypeProgress,
				DispatchID: "dispatch-1",
				Payload:    longPayload,
			},
			{
				Sequence:   1,
				Kind:       responsestream.EventKindProgressFragment,
				Type:       responsestream.EventTypeProgress,
				DispatchID: "dispatch-1",
				Payload:    "duplicate",
			},
		},
	})

	renderer.stopProgressRendering()
	got := output.String()
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one progress line, got %d:\n%s", len(lines), got)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("expected bounded payload suffix, got:\n%s", got)
	}
	if strings.Contains(got, "duplicate") {
		t.Fatalf("duplicate sequence was rendered:\n%s", got)
	}
}

func TestHumanResponseStreamRenderer_SuppressesCompactionNotice(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	renderer.onStreamSegment(responsestream.ReadResult{
		Compaction: &responsestream.CompactionSummary{
			Reason:               responsestream.CompactionReasonTruncated,
			DroppedSequenceCount: 3,
		},
		Events: []responsestream.Event{
			{
				Sequence:   1,
				Kind:       responsestream.EventKindProgressFragment,
				Type:       responsestream.EventTypeProgress,
				DispatchID: "dispatch-1",
				Payload:    "planning",
			},
		},
	})

	renderer.stopProgressRendering()
	got := output.String()
	if strings.Contains(got, "stream truncated") || strings.Contains(got, "earlier events omitted") {
		t.Fatalf("compaction notice must not appear in human output:\n%s", got)
	}
	if !strings.Contains(got, "planning") {
		t.Fatalf("readable progress must still render:\n%s", got)
	}
}

func TestHumanResponseStreamRenderer_SuppressesTokenUsageProgress(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	renderer.onStreamSegment(responsestream.ReadResult{
		Events: []responsestream.Event{
			{
				Sequence:          1,
				Kind:              responsestream.EventKindProgressFragment,
				Type:              responsestream.EventTypeProgress,
				DispatchID:        "dispatch-1",
				ExternalEventType: "token_count",
				Payload:           "input_tokens=120 output_tokens=34 total_tokens=154",
				Metadata: map[string]string{
					"input_tokens":  "120",
					"output_tokens": "34",
				},
			},
			{
				Sequence:   2,
				Kind:       responsestream.EventKindProgressFragment,
				Type:       responsestream.EventTypeProgress,
				DispatchID: "dispatch-1",
				Payload:    "reviewing plan",
			},
		},
	})

	renderer.stopProgressRendering()
	got := output.String()
	for _, marker := range []string{"input_tokens", "output_tokens", "total_tokens", "token_count"} {
		if strings.Contains(got, marker) {
			t.Fatalf("token usage chatter must not appear in human output (%q):\n%s", marker, got)
		}
	}
	if !strings.Contains(got, "reviewing plan") {
		t.Fatalf("readable progress must still render:\n%s", got)
	}
}

func assertHumanResponseStreamExcludesInternalMarkers(t *testing.T, got string) {
	t.Helper()

	for _, marker := range []string{
		"[you:progress]",
		"stream truncated",
		"stream coalesced",
		"stream compacted",
		"earlier events omitted",
		"terminal output backlog",
		"earlier progress unavailable",
		"input_tokens",
		"output_tokens",
		"total_tokens",
		"token_count",
		"cache_read_tokens",
	} {
		if strings.Contains(got, marker) {
			t.Fatalf("human output must not include internal marker %q:\n%s", marker, got)
		}
	}
}

func responseStreamDiagnosticStressReadResult() responsestream.ReadResult {
	return responsestream.ReadResult{
		BehindRetainedWindow: true,
		Compaction: &responsestream.CompactionSummary{
			Reason:               responsestream.CompactionReasonCoalesced,
			DroppedSequenceCount: 2,
		},
		Events: []responsestream.Event{
			{
				Sequence:   1,
				Kind:       responsestream.EventKindProgressFragment,
				Type:       responsestream.EventTypeProgress,
				DispatchID: "dispatch-1",
				Payload:    "stream coalesced (2 earlier events omitted)",
			},
			{
				Sequence:          2,
				Kind:              responsestream.EventKindProgressFragment,
				Type:              responsestream.EventTypeProgress,
				DispatchID:        "dispatch-1",
				ExternalEventType: "token_count",
				Payload:           "input_tokens=120 output_tokens=34 total_tokens=154",
				Metadata: map[string]string{
					"input_tokens":  "120",
					"output_tokens": "34",
				},
			},
			{
				Sequence:   3,
				Kind:       responsestream.EventKindProgressFragment,
				Type:       responsestream.EventTypeProgress,
				DispatchID: "dispatch-1",
				Payload:    "terminal output backlog (progress dropped)",
			},
			{
				Sequence:   4,
				Kind:       responsestream.EventKindProgressFragment,
				Type:       responsestream.EventTypeProgress,
				DispatchID: "dispatch-1",
				Payload:    "reviewing plan",
			},
		},
	}
}

func TestHumanResponseStreamRenderer_ExcludesAllInternalMarkersGolden(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	renderer.onStreamSegment(responseStreamDiagnosticStressReadResult())
	renderer.stopProgressRendering()

	got := output.String()
	assertHumanResponseStreamExcludesInternalMarkers(t, got)
	if !strings.Contains(got, "reviewing plan") {
		t.Fatalf("readable progress must still render:\n%s", got)
	}
}

func TestResponseStreamRenderer_JSONFidelityAndHumanGoldenNegatives(t *testing.T) {
	t.Parallel()

	segment := responseStreamDiagnosticStressReadResult()

	var humanOutput strings.Builder
	humanRenderer := newHumanResponseStreamRenderer(&humanOutput)
	humanRenderer.onStreamSegment(segment)
	humanRenderer.stopProgressRendering()
	humanGot := humanOutput.String()
	assertHumanResponseStreamExcludesInternalMarkers(t, humanGot)
	if !strings.Contains(humanGot, "reviewing plan") {
		t.Fatalf("human output missing readable progress:\n%s", humanGot)
	}

	var jsonOutput strings.Builder
	jsonRenderer := newJSONResponseStreamRenderer(&jsonOutput)
	jsonRenderer.onStreamSegment(segment)
	jsonRenderer.stopProgressRendering()
	jsonGot := jsonOutput.String()
	for _, want := range []string{
		`"recordType":"stream_gap"`,
		`"reason":"behind_retained_window"`,
		`"recordType":"compaction"`,
		`"reason":"COALESCED"`,
		`"recordType":"progress"`,
		`"payload":"input_tokens=120 output_tokens=34 total_tokens=154"`,
		`"payload":"reviewing plan"`,
	} {
		if !strings.Contains(jsonGot, want) {
			t.Fatalf("JSON output missing %q:\n%s", want, jsonGot)
		}
	}
}

func TestHumanResponseStreamRenderer_NoHeaderWithoutProgress(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		PrimaryResult: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "goal completed"},
		},
		Status: factoryapi.InvocationTerminalStatusCompleted,
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}
	if got := output.String(); got != "goal completed" {
		t.Fatalf("output = %q, want plain primary result", got)
	}
}

func TestHumanResponseStreamRenderer_WritesInvocationOutcomeForBlockedFailure(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		RequestID: "req-blocked",
		TraceID:   "trace-blocked",
		Status:    factoryapi.InvocationTerminalStatusFailed,
		ErrorCode: "INVOCATION_BLOCKED",
		Message:   `goal invocation blocked while work "Review plan" is in state goal:blocked`,
		SessionID: "session-1",
		WorkID:    "work-review-plan",
		WorkName:  "Review plan",
		WorkState: "goal:blocked",
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	got := output.String()
	for _, want := range []string{
		responseStreamInvocationOutcomeHeader,
		"status: FAILED",
		"error: INVOCATION_BLOCKED",
		`message: goal invocation blocked while work "Review plan" is in state goal:blocked`,
		"session: session-1",
		"workId: work-review-plan",
		"workName: Review plan",
		"workState: goal:blocked",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, responseStreamPrimaryResultHeader) {
		t.Fatalf("failure output must not use primary-result header:\n%s", got)
	}
}

func TestHumanResponseStreamRenderer_WritesInvocationOutcomeAfterProgress(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	renderer.onStreamSegment(responsestream.ReadResult{
		Events: []responsestream.Event{
			{
				Sequence:   1,
				Kind:       responsestream.EventKindProgressFragment,
				Type:       responsestream.EventTypeProgress,
				DispatchID: "dispatch-1",
				Payload:    "waiting for review",
			},
		},
	})
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status:    factoryapi.InvocationTerminalStatusTimedOut,
		ErrorCode: "INVOCATION_TIMED_OUT",
		Message:   "invocation timed out while waiting for primary result",
		SessionID: "session-1",
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	got := output.String()
	if !strings.Contains(got, "waiting for review") {
		t.Fatalf("output missing progress:\n%s", got)
	}
	if !strings.Contains(got, responseStreamInvocationOutcomeHeader) {
		t.Fatalf("output missing outcome header:\n%s", got)
	}
	if !strings.Contains(got, "status: TIMED_OUT") || !strings.Contains(got, "error: INVOCATION_TIMED_OUT") {
		t.Fatalf("output missing timed-out outcome:\n%s", got)
	}
}

func TestHumanResponseStreamRenderer_WritesUnresolvedPrimaryResultOutcome(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status:    factoryapi.InvocationTerminalStatusFailed,
		ErrorCode: "INVOCATION_PRIMARY_RESULT_UNRESOLVED",
		Message:   "primary result could not be resolved",
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	got := output.String()
	if !strings.Contains(got, "error: INVOCATION_PRIMARY_RESULT_UNRESOLVED") {
		t.Fatalf("output = %q, want unresolved-primary-result outcome", got)
	}
}

func TestJSONResponseStreamRenderer_EmitsPrimaryResultRecordForFailedOutcome(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newJSONResponseStreamRenderer(&output)
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		RequestID: "req-interrupted",
		TraceID:   "trace-interrupted",
		Status:    factoryapi.InvocationTerminalStatusFailed,
		ErrorCode: "INVOCATION_INTERRUPTED",
		Message:   "dispatch was interrupted before primary result resolved",
		SessionID: "session-1",
		WorkID:    "work-1",
		WorkState: "goal:review",
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 NDJSON line, got %d:\n%s", len(lines), output.String())
	}

	var finalRecord responseStreamJSONPrimaryResultRecord
	if err := json.Unmarshal([]byte(lines[0]), &finalRecord); err != nil {
		t.Fatalf("unmarshal primary result line: %v", err)
	}
	if finalRecord.RecordType != responseStreamJSONRecordPrimaryResult {
		t.Fatalf("record type = %q", finalRecord.RecordType)
	}
	if finalRecord.Invocation.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("status = %q, want FAILED", finalRecord.Invocation.Status)
	}
	if finalRecord.Invocation.ErrorCode == nil || *finalRecord.Invocation.ErrorCode != "INVOCATION_INTERRUPTED" {
		t.Fatalf("errorCode = %#v, want INVOCATION_INTERRUPTED", finalRecord.Invocation.ErrorCode)
	}
	if finalRecord.Invocation.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want nil on failure", finalRecord.Invocation.PrimaryResult)
	}
}

func TestJSONResponseStreamRenderer_EmitsOrderedProgressAndPrimaryResultRecords(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newJSONResponseStreamRenderer(&output)
	renderer.onStreamSegment(responsestream.ReadResult{
		Events: []responsestream.Event{
			{
				Sequence:   1,
				Kind:       responsestream.EventKindProgressFragment,
				Type:       responsestream.EventTypeProgress,
				DispatchID: "dispatch-1",
				Payload:    "planning",
			},
			{
				Sequence:   2,
				Kind:       responsestream.EventKindProgressFragment,
				Type:       responsestream.EventTypeProgress,
				DispatchID: "dispatch-1",
				Payload:    "reviewing",
			},
		},
	})
	renderer.onStreamSegment(responsestream.ReadResult{
		Events: []responsestream.Event{
			{
				Sequence:   3,
				Kind:       responsestream.EventKindResponseFragment,
				Type:       responsestream.EventTypeTextDelta,
				DispatchID: "dispatch-1",
				Payload:    "goal completed",
			},
		},
	})
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		RequestID: "req-1",
		TraceID:   "trace-1",
		Status:    factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "goal completed"},
		},
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines, got %d:\n%s", len(lines), output.String())
	}

	var progress1 responseStreamJSONProgressRecord
	if err := json.Unmarshal([]byte(lines[0]), &progress1); err != nil {
		t.Fatalf("unmarshal progress line 1: %v", err)
	}
	if progress1.RecordType != responseStreamJSONRecordProgress || progress1.Payload != "planning" {
		t.Fatalf("progress line 1 = %#v", progress1)
	}

	var progress2 responseStreamJSONProgressRecord
	if err := json.Unmarshal([]byte(lines[1]), &progress2); err != nil {
		t.Fatalf("unmarshal progress line 2: %v", err)
	}
	if progress2.RecordType != responseStreamJSONRecordProgress || progress2.Payload != "reviewing" {
		t.Fatalf("progress line 2 = %#v", progress2)
	}

	var finalRecord responseStreamJSONPrimaryResultRecord
	if err := json.Unmarshal([]byte(lines[2]), &finalRecord); err != nil {
		t.Fatalf("unmarshal primary result line: %v", err)
	}
	if finalRecord.RecordType != responseStreamJSONRecordPrimaryResult {
		t.Fatalf("final record type = %q", finalRecord.RecordType)
	}
	if finalRecord.Invocation.RequestId != "req-1" {
		t.Fatalf("invocation = %#v", finalRecord.Invocation)
	}
}

func TestJSONResponseStreamRenderer_SurfacesCompactionAndStreamGapRecords(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newJSONResponseStreamRenderer(&output)
	renderer.onStreamSegment(responsestream.ReadResult{
		BehindRetainedWindow: true,
		Compaction: &responsestream.CompactionSummary{
			Reason:               responsestream.CompactionReasonTruncated,
			DroppedSequenceCount: 3,
		},
	})

	renderer.stopProgressRendering()
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d:\n%s", len(lines), output.String())
	}

	var gap responseStreamJSONStreamGapRecord
	if err := json.Unmarshal([]byte(lines[0]), &gap); err != nil {
		t.Fatalf("unmarshal stream gap: %v", err)
	}
	if gap.RecordType != responseStreamJSONRecordStreamGap || gap.Reason != "behind_retained_window" {
		t.Fatalf("gap = %#v", gap)
	}

	var compaction responseStreamJSONCompactionRecord
	if err := json.Unmarshal([]byte(lines[1]), &compaction); err != nil {
		t.Fatalf("unmarshal compaction: %v", err)
	}
	if compaction.RecordType != responseStreamJSONRecordCompaction || compaction.DroppedSequenceCount != 3 {
		t.Fatalf("compaction = %#v", compaction)
	}
}

func TestJSONResponseStreamRenderer_EmitsTokenUsageProgressRecord(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newJSONResponseStreamRenderer(&output)
	renderer.onStreamSegment(responsestream.ReadResult{
		Events: []responsestream.Event{
			{
				Sequence:          1,
				Kind:              responsestream.EventKindProgressFragment,
				Type:              responsestream.EventTypeProgress,
				DispatchID:        "dispatch-1",
				ExternalEventType: "token_count",
				Payload:           "input_tokens=120 output_tokens=34 total_tokens=154",
				Metadata: map[string]string{
					"input_tokens":  "120",
					"output_tokens": "34",
				},
			},
		},
	})

	renderer.stopProgressRendering()
	got := output.String()
	if !strings.Contains(got, `"recordType":"progress"`) {
		t.Fatalf("expected progress record:\n%s", got)
	}
	if !strings.Contains(got, `"payload":"input_tokens=120 output_tokens=34 total_tokens=154"`) {
		t.Fatalf("token usage payload must remain in JSON diagnostics:\n%s", got)
	}
}

func TestHumanResponseStreamRenderer_SuppressesTerminalOutputBacklog(t *testing.T) {
	t.Parallel()

	output := &gatedResponseStreamWriter{}
	output.block()
	renderer := newHumanResponseStreamRenderer(output)
	floodResponseStreamProgress(renderer, defaultResponseStreamProgressQueueCapacity+4)

	output.release()
	if err := renderer.writeFinalInvocationResult(responseStreamBacklogSuccessResult); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	got := output.String()
	if strings.Contains(got, "terminal output backlog") {
		t.Fatalf("terminal backlog notice must not appear in human output:\n%s", got)
	}
	primaryIdx := strings.Index(got, responseStreamPrimaryResultHeader)
	if primaryIdx < 0 {
		t.Fatalf("want primary result header:\n%s", got)
	}
}

func TestJSONResponseStreamRenderer_SurfacesTerminalOutputBacklogRecord(t *testing.T) {
	t.Parallel()

	output := &gatedResponseStreamWriter{}
	output.block()
	renderer := newJSONResponseStreamRenderer(output)
	floodResponseStreamProgress(renderer, defaultResponseStreamProgressQueueCapacity+4)

	output.release()
	if err := renderer.writeFinalInvocationResult(responseStreamBacklogSuccessResult); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	got := output.String()
	backlogIdx := strings.Index(got, `"reason":"terminal_output_backlog"`)
	primaryIdx := strings.Index(got, `"recordType":"primary_result"`)
	if backlogIdx < 0 || !strings.Contains(got, `"recordType":"stream_gap"`) || primaryIdx < 0 || backlogIdx > primaryIdx {
		t.Fatalf("want backlog record before primary_result:\n%s", got)
	}
}

func TestJSONResponseStreamRenderer_EmitsOnlyPrimaryResultWithoutProgress(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newJSONResponseStreamRenderer(&output)
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status: factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "goal completed"},
		},
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 NDJSON line, got %d:\n%s", len(lines), output.String())
	}
	if !strings.Contains(lines[0], `"recordType":"primary_result"`) {
		t.Fatalf("output = %q", lines[0])
	}
}

func TestResponseStreamAttachment_SubscribesWhenDispatchAppears(t *testing.T) {
	t.Parallel()

	attachable := newRecordingResponseStreamAttachable()
	attachable.ensureDispatch("dispatch-1")
	sink := &countingResponseStreamSink{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attachment := startResponseStreamAttachment(ctx, attachable, factorysessions.DefaultSessionID, sink)
	if attachment == nil {
		t.Fatal("expected attachment")
	}
	defer attachment.stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(attachable.subscribeCalls) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(attachable.subscribeCalls) == 0 {
		t.Fatal("expected internal response-stream subscription")
	}

	attachable.stream("dispatch-1").Append(responsestream.Event{
		Kind:    responsestream.EventKindProgressFragment,
		Type:    responsestream.EventTypeProgress,
		Payload: "working",
	})

	for time.Now().Before(deadline) {
		if sink.segments() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if sink.segments() == 0 {
		t.Fatal("expected stream segment delivery after subscription")
	}
	if got := attachable.subscribeCalls[0].dispatchID; got != "dispatch-1" {
		t.Fatalf("dispatchID = %q, want dispatch-1", got)
	}
}

type countingResponseStreamSink struct {
	segmentCount int
}

func (s *countingResponseStreamSink) onStreamSegment(factorysessions.SessionResponseStreamReadResult) {
	s.segmentCount++
}

func (s *countingResponseStreamSink) segments() int {
	return s.segmentCount
}
