package run

import (
	"context"
	"encoding/json"
	"fmt"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
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

type recordingResponseEventAttachable struct {
	store      *responseeventstore.SessionResponseEventStore
	subscribed chan struct{}
	once       sync.Once
}

func newRecordingResponseEventAttachable() *recordingResponseEventAttachable {
	return &recordingResponseEventAttachable{
		store:      responseeventstore.NewSessionResponseEventStore("session-1"),
		subscribed: make(chan struct{}),
	}
}

func (a *recordingResponseEventAttachable) SubscribeSessionResponseEventsFromLatest(
	string,
) (*responseeventstore.Subscription, error) {
	subscription, err := a.store.Subscribe(a.store.LatestSequence())
	if err == nil {
		a.once.Do(func() { close(a.subscribed) })
	}
	return subscription, err
}

func (a *recordingResponseEventAttachable) publish(event responseevents.FactoryResponseEvent) error {
	_, err := a.store.Publish(event)
	return err
}

type stubResponseEventInvocationService struct {
	stubInvocationService
	attachable *recordingResponseEventAttachable
}

func (s stubResponseEventInvocationService) SubscribeSessionResponseEventsFromLatest(
	sessionID string,
) (*responseeventstore.Subscription, error) {
	return s.attachable.SubscribeSessionResponseEventsFromLatest(sessionID)
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
	jsonRenderer.stopProgressRendering()
	if jsonGot := jsonOutput.String(); jsonGot != "" {
		t.Fatalf("legacy response fragments must not produce JSON records:\n%s", jsonGot)
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

func TestJSONResponseStreamRenderer_EmitsInvocationResultRecordForFailedOutcome(t *testing.T) {
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

	var finalRecord responseStreamJSONInvocationResultRecord
	if err := json.Unmarshal([]byte(lines[0]), &finalRecord); err != nil {
		t.Fatalf("unmarshal invocation result line: %v", err)
	}
	if finalRecord.RecordType != responseStreamJSONRecordInvocationResult {
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

func TestJSONResponseStreamRenderer_EmitsCanonicalResponseEventsAndInvocationResult(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newJSONResponseStreamRenderer(&output)
	events := []responseevents.FactoryResponseEvent{
		canonicalResponseEventFixture(41, responseevents.KindMessage),
		canonicalResponseEventFixture(42, responseevents.KindStreamGap),
	}
	renderer.onResponseEvents(events)
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

	for index, want := range events {
		var record responseStreamJSONResponseEventRecord
		if err := json.Unmarshal([]byte(lines[index]), &record); err != nil {
			t.Fatalf("unmarshal response event line %d: %v", index, err)
		}
		if record.RecordType != responseStreamJSONRecordResponseEvent || record.Event.EventID != want.EventID {
			t.Fatalf("response event line %d = %#v", index, record)
		}
	}

	var finalRecord responseStreamJSONInvocationResultRecord
	if err := json.Unmarshal([]byte(lines[2]), &finalRecord); err != nil {
		t.Fatalf("unmarshal invocation result line: %v", err)
	}
	if finalRecord.RecordType != responseStreamJSONRecordInvocationResult {
		t.Fatalf("final record type = %q", finalRecord.RecordType)
	}
	if finalRecord.Invocation.RequestId != "req-1" {
		t.Fatalf("invocation = %#v", finalRecord.Invocation)
	}
}

func TestRun_FactoryInvocationResponseStreamJSONPreservesSlowWriterOrder(t *testing.T) {
	preserveRunGlobals(t)

	const eventCount = defaultResponseStreamProgressQueueCapacity + 4
	text := "goal completed"
	output := &gatedResponseStreamWriter{}
	output.block()
	eventsPublished := make(chan struct{})
	attachable := newRecordingResponseEventAttachable()
	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		return stubResponseEventInvocationService{
			stubInvocationService: stubInvocationService{
				run: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
				invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
					select {
					case <-attachable.subscribed:
					case <-time.After(2 * time.Second):
						return apisurface.FactoryInvocationResult{}, fmt.Errorf("canonical response-event subscription was not established")
					}
					if err := publishCanonicalResponseEventFixtures(attachable, eventCount); err != nil {
						return apisurface.FactoryInvocationResult{}, err
					}
					close(eventsPublished)
					return apisurface.FactoryInvocationResult{
						RequestID: "req-slow-writer",
						TraceID:   "trace-slow-writer",
						Status:    factoryapi.InvocationTerminalStatusCompleted,
						PrimaryResult: []interfaces.WorkContentPart{
							{Type: interfaces.WorkContentPartTypeText, Text: text},
						},
					}, nil
				},
			},
			attachable: attachable,
		}, nil
	}

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), RunConfig{
			FactoryConfigPath:        "/tmp/factory.json",
			InvocationPositionalText: &text,
			InvocationOutputMode:     InvocationOutputResponseStream,
			JSONOutput:               true,
			StdinIsTTY:               func() bool { return true },
			Output:                   output,
			Port:                     7437,
		})
	}()

	select {
	case <-eventsPublished:
	case err := <-done:
		t.Fatalf("Run completed before canonical events were published: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for canonical response events")
	}
	waitForBlockedStdoutWrites(t, output, 2*time.Second)
	time.Sleep(responseStreamProgressDrainTimeout + 50*time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("Run completed while stdout remained blocked: %v", err)
	default:
	}
	output.release()
	waitForResponseStreamRunCompletion(t, done, 2*time.Second)

	assertSlowWriterCanonicalRecords(t, output.String(), eventCount)
}

func TestRun_FactoryInvocationResponseStreamJSONDrainsEventPublishedAtInvocationReturn(t *testing.T) {
	preserveRunGlobals(t)
	text := "goal completed"
	var output strings.Builder
	attachable := newRecordingResponseEventAttachable()
	buildInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (sessionInvocationRunner, error) {
		return stubResponseEventInvocationService{
			stubInvocationService: stubInvocationService{
				run: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
				invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
					<-attachable.subscribed
					if err := attachable.publish(canonicalResponseEventFixture(1, responseevents.KindMessage)); err != nil {
						return apisurface.FactoryInvocationResult{}, err
					}
					return apisurface.FactoryInvocationResult{
						RequestID: "req-terminal-boundary",
						TraceID:   "trace-terminal-boundary",
						Status:    factoryapi.InvocationTerminalStatusCompleted,
						PrimaryResult: []interfaces.WorkContentPart{
							{Type: interfaces.WorkContentPartTypeText, Text: text},
						},
					}, nil
				},
			},
			attachable: attachable,
		}, nil
	}
	if err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		InvocationOutputMode:     InvocationOutputResponseStream,
		JSONOutput:               true,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("NDJSON lines = %d, want terminal-boundary event plus final result:\n%s", len(lines), output.String())
	}
	if !strings.Contains(lines[0], `"recordType":"response_event"`) {
		t.Fatalf("first record = %q, want response_event", lines[0])
	}
	if !strings.Contains(lines[1], `"recordType":"invocation_result"`) {
		t.Fatalf("final record = %q, want invocation_result", lines[1])
	}
}

func publishCanonicalResponseEventFixtures(attachable *recordingResponseEventAttachable, count int) error {
	for index := 1; index <= count; index++ {
		if err := attachable.publish(canonicalResponseEventFixture(int64(index), responseevents.KindMessage)); err != nil {
			return fmt.Errorf("publish canonical response event %d: %w", index, err)
		}
	}
	return nil
}

func assertSlowWriterCanonicalRecords(t *testing.T, output string, eventCount int) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != eventCount+1 {
		t.Fatalf("NDJSON lines = %d, want %d lossless events plus final result", len(lines), eventCount+1)
	}
	for index := 0; index < eventCount; index++ {
		var record responseStreamJSONResponseEventRecord
		if err := json.Unmarshal([]byte(lines[index]), &record); err != nil {
			t.Fatalf("decode response event line %d: %v", index, err)
		}
		if record.RecordType != responseStreamJSONRecordResponseEvent || record.Event.Sequence != int64(index+1) {
			t.Fatalf("response event line %d = %#v", index, record)
		}
	}
	var finalRecord responseStreamJSONInvocationResultRecord
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &finalRecord); err != nil {
		t.Fatalf("decode final invocation result: %v", err)
	}
	if finalRecord.RecordType != responseStreamJSONRecordInvocationResult || finalRecord.Invocation.RequestId != "req-slow-writer" {
		t.Fatalf("final record = %#v", finalRecord)
	}
}

func TestJSONResponseStreamRenderer_CanonicalEventBytesMatchAPIPayload(t *testing.T) {
	t.Parallel()

	event := canonicalResponseEventFixture(42, responseevents.KindMessage)
	domainBytes, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal canonical event fixture: %v", err)
	}
	var apiEvent factoryapi.FactoryResponseEvent
	if err := json.Unmarshal(domainBytes, &apiEvent); err != nil {
		t.Fatalf("project fixture through generated API type: %v", err)
	}
	apiBytes, err := json.Marshal(apiEvent)
	if err != nil {
		t.Fatalf("marshal generated API event fixture: %v", err)
	}
	var output strings.Builder
	renderer := newJSONResponseStreamRenderer(&output)
	renderer.onResponseEvents([]responseevents.FactoryResponseEvent{event})
	renderer.stopProgressRendering()
	var wire struct {
		RecordType string          `json:"recordType"`
		Event      json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output.String())), &wire); err != nil {
		t.Fatalf("decode NDJSON record: %v", err)
	}
	if wire.RecordType != responseStreamJSONRecordResponseEvent {
		t.Fatalf("recordType = %q", wire.RecordType)
	}
	if string(wire.Event) != string(apiBytes) {
		t.Fatalf("CLI/API event bytes differ:\ncli=%s\napi=%s", wire.Event, apiBytes)
	}
}

func TestJSONResponseStreamRenderer_EmitsOnlyPublicRecordTypes(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newJSONResponseStreamRenderer(&output)
	renderer.onResponseEvents([]responseevents.FactoryResponseEvent{
		canonicalResponseEventFixture(42, responseevents.KindStreamGap),
	})
	if err := renderer.writeFinalInvocationResult(responseStreamBacklogSuccessResult); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var header struct {
			RecordType string `json:"recordType"`
		}
		if err := json.Unmarshal([]byte(line), &header); err != nil {
			t.Fatalf("line is not independently decodable: %v\n%s", err, line)
		}
		if header.RecordType != responseStreamJSONRecordResponseEvent && header.RecordType != responseStreamJSONRecordInvocationResult {
			t.Fatalf("private record type emitted: %q", header.RecordType)
		}
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

func TestJSONResponseStreamRenderer_EmitsOnlyInvocationResultWithoutEvents(t *testing.T) {
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
	if !strings.Contains(lines[0], `"recordType":"invocation_result"`) {
		t.Fatalf("output = %q", lines[0])
	}
}

func canonicalResponseEventFixture(sequence int64, kind responseevents.Kind) responseevents.FactoryResponseEvent {
	payload := json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"Hello"}`)
	phase := responseevents.PhaseDelta
	if kind == responseevents.KindStreamGap {
		payload = json.RawMessage(`{"fromSequence":1,"toSequence":2,"reason":"retention_eviction"}`)
		phase = responseevents.PhaseUpdated
	}
	return responseevents.FactoryResponseEvent{
		SchemaVersion:    responseevents.SchemaVersionV1,
		EventID:          fmt.Sprintf("event-%d", sequence),
		Sequence:         sequence,
		RecordedAt:       time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		FactorySessionID: "session-1",
		RunID:            "run-1",
		Kind:             kind,
		Phase:            phase,
		Provenance: responseevents.Provenance{
			Provider:        "test-provider",
			NativeEventType: "message.delta",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationDelta,
			Fidelity:        responseevents.FidelityLossless,
		},
		Payload:    payload,
		DispatchID: "dispatch-1",
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
