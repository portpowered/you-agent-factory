package run

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
		PrimaryResult: []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "goal completed"},
		},
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	got := output.String()
	wantProgress := []string{
		"[you:progress] planning",
		"[you:progress] reviewing",
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
	if strings.Contains(got, "[you:progress] goal completed") {
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

	got := output.String()
	if strings.Count(got, "[you:progress]") != 1 {
		t.Fatalf("expected one progress line, got:\n%s", got)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("expected bounded payload suffix, got:\n%s", got)
	}
	if strings.Contains(got, "duplicate") {
		t.Fatalf("duplicate sequence was rendered:\n%s", got)
	}
}

func TestHumanResponseStreamRenderer_SurfacesCompactionNotice(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output)
	renderer.onStreamSegment(responsestream.ReadResult{
		Compaction: &responsestream.CompactionSummary{
			Reason:               responsestream.CompactionReasonTruncated,
			DroppedSequenceCount: 3,
		},
	})

	got := output.String()
	if !strings.Contains(got, "[you:progress] stream truncated (3 earlier events omitted)") {
		t.Fatalf("compaction notice = %q", got)
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
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}
	if got := output.String(); got != "goal completed" {
		t.Fatalf("output = %q, want plain primary result", got)
	}
}

func TestHumanProgressRenderableType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		eventType responsestream.EventType
		want      bool
	}{
		{eventType: responsestream.EventTypeProgress, want: true},
		{eventType: responsestream.EventTypeStarted, want: true},
		{eventType: responsestream.EventTypeTextDelta, want: false},
		{eventType: responsestream.EventTypeFinalText, want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(string(tc.eventType), func(t *testing.T) {
			t.Parallel()
			if got := humanProgressRenderableType(tc.eventType); got != tc.want {
				t.Fatalf("humanProgressRenderableType(%q) = %t, want %t", tc.eventType, got, tc.want)
			}
		})
	}
}

func TestBoundedHumanProgressPayload(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("a", maxHumanProgressLineBytes+10)
	got := boundedHumanProgressPayload(payload)
	if len([]byte(got)) > maxHumanProgressLineBytes+3 {
		t.Fatalf("bounded payload too long: %d bytes", len([]byte(got)))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("bounded payload = %q, want ellipsis suffix", got)
	}
}

func TestFormatCompactionNotice(t *testing.T) {
	t.Parallel()

	got := formatCompactionNotice(responsestream.CompactionSummary{
		Reason:                responsestream.CompactionReasonCoalesced,
		DroppedSequenceCount:  2,
		FirstRetainedSequence: 5,
	})
	if got != "stream coalesced (2 earlier events omitted)" {
		t.Fatalf("notice = %q", got)
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
