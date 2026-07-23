package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factorysessionfixtures"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type gatedResponseStreamWriter struct {
	mu                   sync.Mutex
	blocked              bool
	buf                  strings.Builder
	blockedWriteAttempts atomic.Int64
}

type recordingResponseEventAttachable struct {
	cursor     *factorysessionfixtures.ResponseEventCursor
	subscribed chan struct{}
	once       sync.Once
}

func newRecordingResponseEventAttachable() *recordingResponseEventAttachable {
	return &recordingResponseEventAttachable{
		cursor:     factorysessionfixtures.NewResponseEventCursor(defaultResponseStreamProgressQueueCapacity + 16),
		subscribed: make(chan struct{}),
	}
}

func (a *recordingResponseEventAttachable) SubscribeSessionResponseEventsFromLatest(
	string,
) (factoryvisualization.ResponseEventCursor, error) {
	a.once.Do(func() { close(a.subscribed) })
	return a.cursor, nil
}

func (a *recordingResponseEventAttachable) publish(event factorysessions.FactoryResponseEvent) error {
	a.cursor.Batches <- []factorysessions.FactoryResponseEvent{event}
	return nil
}

type stubResponseEventInvocationService struct {
	stubInvocationService
	attachable *recordingResponseEventAttachable
}

func (s stubResponseEventInvocationService) SubscribeSessionResponseEventsFromLatest(
	sessionID string,
) (factoryvisualization.ResponseEventCursor, error) {
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

func floodCanonicalHumanProgress(sink responseEventSink, count int) {
	for i := 0; i < count; i++ {
		event := humanResponseEvent(
			factorysessions.ResponseEventKindProgress,
			factorysessions.ResponseEventPhaseUpdated,
			factorysessions.ResponseEventProgress{Label: "working"},
		)
		event.Sequence = int64(i + 1)
		sink.PresentResponseEvents([]factorysessions.FactoryResponseEvent{event})
	}
}

var responseStreamBacklogSuccessResult = apisurface.FactoryInvocationResult{
	Status: interfaces.InvocationTerminalStatusCompleted,
	PrimaryResult: []work.WorkContentPart{
		{Type: work.WorkContentPartTypeText, Text: "goal completed"},
	},
}

func TestResponseStreamProgressWriter_EnqueueDoesNotBlockWhenOutputSlow(t *testing.T) {
	t.Parallel()

	output := &gatedResponseStreamWriter{}
	output.block()
	writer := testResponsePresentation().OpenBestEffortOutput(output)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < defaultResponseStreamProgressQueueCapacity+8; i++ {
			_ = writer.Enqueue([]byte("line"))
		}
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("enqueue blocked while terminal output was slow")
	}

	if got := writer.Dropped(); got == 0 {
		t.Fatalf("dropped lines = 0, want backlog drops when queue is full")
	}

	output.release()
	_ = writer.CloseAndDrain()
}

func TestHumanResponseStreamRenderer_RendersOrderedProgressAndSeparatesPrimaryResult(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
	planning := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: "planning"})
	planning.Sequence = 1
	reviewing := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: "reviewing"})
	reviewing.Sequence = 2
	message := humanResponseEvent(factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseDelta, factorysessions.ResponseEventMessageDelta{
		ContentBlockIndex: 0, ContentBlockKind: factorysessions.ResponseEventContentBlockText, TextDelta: "goal completed",
	})
	message.Sequence = 3
	renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{planning, reviewing, message})

	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status: interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "goal completed"},
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
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
	longPayload := strings.Repeat("x", maxHumanProgressLineBytes+32)
	first := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: longPayload})
	first.Sequence = 1
	duplicate := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: "duplicate"})
	duplicate.Sequence = 1
	renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{first, duplicate})

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

func TestHumanResponseStreamRenderer_SuppressesTokenUsageProgress(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
	usage := humanResponseEvent(factorysessions.ResponseEventKindUsage, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventUsage{InputTokens: 120, OutputTokens: 34})
	usage.Sequence = 1
	progress := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: "reviewing plan"})
	progress.Sequence = 2
	renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{usage, progress})

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

func TestHumanResponseStreamRenderer_NoHeaderWithoutProgress(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "goal completed"},
		},
		Status: interfaces.InvocationTerminalStatusCompleted,
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
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		RequestID: "req-blocked",
		TraceID:   "trace-blocked",
		Status:    interfaces.InvocationTerminalStatusFailed,
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
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
	progress := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: "waiting for review"})
	progress.Sequence = 1
	renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{progress})
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status:    interfaces.InvocationTerminalStatusTimedOut,
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
	renderer := newHumanResponseStreamRenderer(&output, testResponsePresentation(), testResponseEventValidator())
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status:    interfaces.InvocationTerminalStatusFailed,
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
	renderer := newJSONResponseStreamRenderer(&output, testResponsePresentation())
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		RequestID: "req-interrupted",
		TraceID:   "trace-interrupted",
		Status:    interfaces.InvocationTerminalStatusFailed,
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
	renderer := newJSONResponseStreamRenderer(&output, testResponsePresentation())
	events := []factorysessions.FactoryResponseEvent{
		canonicalResponseEventFixture(41, factorysessions.ResponseEventKindMessage),
		canonicalResponseEventFixture(42, factorysessions.ResponseEventKindStreamGap),
	}
	renderer.PresentResponseEvents(events)
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		RequestID: "req-1",
		TraceID:   "trace-1",
		Status:    interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "goal completed"},
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
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
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
						Status:    interfaces.InvocationTerminalStatusCompleted,
						PrimaryResult: []work.WorkContentPart{
							{Type: work.WorkContentPartTypeText, Text: text},
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
	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubResponseEventInvocationService{
			stubInvocationService: stubInvocationService{
				run: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
				invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
					<-attachable.subscribed
					if err := attachable.publish(canonicalResponseEventFixture(1, factorysessions.ResponseEventKindMessage)); err != nil {
						return apisurface.FactoryInvocationResult{}, err
					}
					return apisurface.FactoryInvocationResult{
						RequestID: "req-terminal-boundary",
						TraceID:   "trace-terminal-boundary",
						Status:    interfaces.InvocationTerminalStatusCompleted,
						PrimaryResult: []work.WorkContentPart{
							{Type: work.WorkContentPartTypeText, Text: text},
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
		if err := attachable.publish(canonicalResponseEventFixture(int64(index), factorysessions.ResponseEventKindMessage)); err != nil {
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

	event := canonicalResponseEventFixture(42, factorysessions.ResponseEventKindMessage)
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
	renderer := newJSONResponseStreamRenderer(&output, testResponsePresentation())
	renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{event})
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
	renderer := newJSONResponseStreamRenderer(&output, testResponsePresentation())
	renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{
		canonicalResponseEventFixture(42, factorysessions.ResponseEventKindStreamGap),
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
	renderer := newHumanResponseStreamRenderer(output, testResponsePresentation(), testResponseEventValidator())
	floodCanonicalHumanProgress(renderer, defaultResponseStreamProgressQueueCapacity+4)

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
	renderer := newJSONResponseStreamRenderer(&output, testResponsePresentation())
	if err := renderer.writeFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status: interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "goal completed"},
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

func canonicalResponseEventFixture(sequence int64, kind factorysessions.ResponseEventKind) factorysessions.FactoryResponseEvent {
	payload := json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"Hello"}`)
	phase := factorysessions.ResponseEventPhaseDelta
	if kind == factorysessions.ResponseEventKindStreamGap {
		payload = json.RawMessage(`{"fromSequence":1,"toSequence":2,"reason":"retention_eviction"}`)
		phase = factorysessions.ResponseEventPhaseUpdated
	}
	return factorysessions.FactoryResponseEvent{
		SchemaVersion:    factorysessions.ResponseEventSchemaVersionV1,
		EventID:          fmt.Sprintf("event-%d", sequence),
		Sequence:         sequence,
		RecordedAt:       time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		FactorySessionID: "session-1",
		RunID:            "run-1",
		Kind:             kind,
		Phase:            phase,
		Provenance: factorysessions.ResponseEventProvenance{
			Provider:        "test-provider",
			NativeEventType: "message.delta",
			Delivery:        factorysessions.ResponseEventDeliveryNativeStream,
			Representation:  factorysessions.ResponseEventRepresentationDelta,
			Fidelity:        factorysessions.ResponseEventFidelityLossless,
		},
		Payload:    payload,
		DispatchID: "dispatch-1",
	}
}

func TestResponseEventAttachment_DeliversCanonicalEvents(t *testing.T) {
	t.Parallel()

	attachable := newRecordingResponseEventAttachable()
	sink := &countingResponseEventSink{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attachment := testResponsePresentation().Attach(ctx, attachable, factorysessions.DefaultSessionID, sink)
	if attachment == nil {
		t.Fatal("expected attachment")
	}
	defer attachment.Stop()

	select {
	case <-attachable.subscribed:
	case <-time.After(2 * time.Second):
		t.Fatal("expected canonical response-event subscription")
	}
	event := humanResponseEvent(factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated, factorysessions.ResponseEventProgress{Label: "working"})
	if err := attachable.publish(event); err != nil {
		t.Fatalf("publish response event: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sink.events() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if sink.events() == 0 {
		t.Fatal("expected canonical event delivery after subscription")
	}
}

type countingResponseEventSink struct {
	eventCount atomic.Int64
}

func (s *countingResponseEventSink) PresentResponseEvents(events []factorysessions.FactoryResponseEvent) {
	s.eventCount.Add(int64(len(events)))
}

func (s *countingResponseEventSink) events() int64 {
	return s.eventCount.Load()
}

func TestResponseStreamNDJSON_PublicVocabularyDecodesAfterPrivateRemoval(t *testing.T) {
	t.Parallel()

	event := factorysessions.FactoryResponseEvent{
		SchemaVersion:    factorysessions.ResponseEventSchemaVersionV1,
		EventID:          "event-migration-1",
		Sequence:         1,
		FactorySessionID: "session-1",
		RunID:            "run-1",
		Kind:             factorysessions.ResponseEventKindProgress,
		Phase:            factorysessions.ResponseEventPhaseUpdated,
		Payload:          []byte(`{"label":"planning","message":"next step"}`),
		Provenance: factorysessions.ResponseEventProvenance{
			Provider:       "test-provider",
			Delivery:       factorysessions.ResponseEventDeliverySynthesized,
			Representation: factorysessions.ResponseEventRepresentationNotification,
			Fidelity:       factorysessions.ResponseEventFidelityNormalized,
		},
		DispatchID: "dispatch-1",
	}
	invocation := interfaces.FactoryInvocationResult{
		RequestID: "req-migration-1",
		TraceID:   "trace-migration-1",
		Status:    interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "done"},
		},
	}

	lines, err := encodePublicResponseStreamFixtures([]factorysessions.FactoryResponseEvent{event}, invocation)
	if err != nil {
		t.Fatalf("EncodeTransportCLINDJSON: %v", err)
	}
	decodedEvents, decodedInvocation, err := decodePublicResponseStreamFixtures(lines)
	if err != nil {
		t.Fatalf("DecodeTransportCLINDJSON: %v", err)
	}
	if len(decodedEvents) != 1 || decodedEvents[0].EventID != event.EventID {
		t.Fatalf("decoded events = %#v", decodedEvents)
	}
	if decodedInvocation.RequestId != invocation.RequestID {
		t.Fatalf("decoded invocation = %#v", decodedInvocation)
	}

	for _, retired := range []string{"progress", "compaction", "primary_result", "stream_gap"} {
		if strings.Contains(strings.Join(lines, "\n"), `"recordType":"`+retired+`"`) {
			t.Fatalf("public vocabulary output still contains retired recordType %q:\n%s", retired, strings.Join(lines, "\n"))
		}
	}
}

func TestResponseStreamNDJSON_RendererOutputDecodesThroughPublicContract(t *testing.T) {
	t.Parallel()

	event := canonicalResponseEventFixture(2, factorysessions.ResponseEventKindMessage)
	result := apisurface.FactoryInvocationResult{
		RequestID: "req-migration-2",
		Status:    interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "final"},
		},
	}

	var output strings.Builder
	renderer := newJSONResponseStreamRenderer(&output, testResponsePresentation())
	renderer.PresentResponseEvents([]factorysessions.FactoryResponseEvent{event})
	if err := renderer.writeFinalInvocationResult(result); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}
	if _, _, err := decodePublicResponseStreamFixtures(strings.Split(strings.TrimSpace(output.String()), "\n")); err != nil {
		t.Fatalf("renderer output must decode through public transport contract: %v\n%s", err, output.String())
	}
}
