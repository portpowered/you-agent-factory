package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func progressDraft(label string) workers.Draft {
	payload, err := json.Marshal(workers.ProgressPayload{Label: label})
	if err != nil {
		panic(err)
	}
	return workers.Draft{Kind: workers.KindProgress, Phase: workers.PhaseUpdated, Payload: payload}
}

func toolDraft(toolCallID string) workers.Draft {
	payload, err := json.Marshal(workers.ToolPayload{ToolCallID: toolCallID, ToolName: "grep"})
	if err != nil {
		panic(err)
	}
	return workers.Draft{Kind: workers.KindTool, Phase: workers.PhaseStarted, Payload: payload}
}

func messageDraft(text string) workers.Draft {
	payload, err := json.Marshal(workers.MessagePayload{
		Role:          "assistant",
		ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: text}},
	})
	if err != nil {
		panic(err)
	}
	return workers.Draft{Kind: workers.KindMessage, Phase: workers.PhaseCompleted, Payload: payload}
}

func validPublishRecordRequest(sessionID string, sequence events.SourceSequence, draft workers.Draft) workersessions.PublishRecordRequest {
	return workersessions.PublishRecordRequest{
		SessionID:      sessionID,
		Draft:          draft,
		SourceType:     "worker_provider",
		SourceID:       events.SourceID(sessionID),
		SourceSequence: sequence,
		SourceEventID:  events.SourceEventID(fmt.Sprintf("evt-%d", sequence)),
		SchemaID:       "workers.draft.v1",
	}
}

func readAllDrafts(t *testing.T, eventsSvc events.Service, topic events.Topic) []workers.Draft {
	t.Helper()
	result, err := eventsSvc.Read(context.Background(), events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	drafts := make([]workers.Draft, len(result.Records))
	for i, record := range result.Records {
		if err := json.Unmarshal(record.Payload, &drafts[i]); err != nil {
			t.Fatalf("unmarshal record payload error = %v", err)
		}
	}
	return drafts
}

// TestPublishRecord_AppendsValidatedDetachedSourceNativeDraftsInOrder proves
// that a sequence of valid Worker observations across multiple Kinds and
// Phases is committed after the W3 opening record, in the exact order
// PublishRecord is called, with Kind/Phase/Payload preserved verbatim.
func TestPublishRecord_AppendsValidatedDetachedSourceNativeDraftsInOrder(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	drafts := []workers.Draft{toolDraft("tc-1"), progressDraft("thinking"), messageDraft("done")}
	for i, draft := range drafts {
		req := validPublishRecordRequest("worker-1", events.SourceSequence(i+1), draft)
		result, err := registry.PublishRecord(ctx, req)
		if err != nil {
			t.Fatalf("PublishRecord() [%d] error = %v, want nil", i, err)
		}
		if result.Outcome != workersessions.PublishOutcomeAccepted {
			t.Fatalf("PublishRecord() [%d] outcome = %v, want Accepted", i, result.Outcome)
		}
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != len(drafts) {
		t.Fatalf("committed record count = %d, want %d", len(committed), len(drafts))
	}
	for i, want := range drafts {
		got := committed[i]
		if got.Kind != want.Kind || got.Phase != want.Phase {
			t.Fatalf("committed[%d] Kind/Phase = %s/%s, want %s/%s", i, got.Kind, got.Phase, want.Kind, want.Phase)
		}
		if string(got.Payload) != string(want.Payload) {
			t.Fatalf("committed[%d] Payload = %s, want %s", i, got.Payload, want.Payload)
		}
	}
}

// TestPublishRecord_OrderedAfterOpeningRecord proves that publishing source
// observations onto a session Started through Start lands after both the
// opening SESSION/STARTED record and the SESSION/COMPLETED terminal record
// Start's synchronous dispatch has already committed by the time Start
// returns.
func TestPublishRecord_OrderedAfterOpeningRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	if _, err := registry.Start(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if _, err := registry.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, toolDraft("tc-1"))); err != nil {
		t.Fatalf("PublishRecord() error = %v, want nil", err)
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 3 {
		t.Fatalf("committed record count = %d, want 3", len(committed))
	}
	if committed[0].Kind != workers.KindSession || committed[0].Phase != workers.PhaseStarted {
		t.Fatalf("committed[0] = %+v, want the SESSION/STARTED opening record", committed[0])
	}
	if committed[1].Kind != workers.KindSession || committed[1].Phase != workers.PhaseCompleted {
		t.Fatalf("committed[1] = %+v, want the SESSION/COMPLETED terminal record", committed[1])
	}
	if committed[2].Kind != workers.KindTool {
		t.Fatalf("committed[2] Kind = %s, want TOOL", committed[2].Kind)
	}
}

// TestPublishRecord_InvalidDraft_ReturnsErrorAndCommitsNoRecord proves that a
// draft violating the existing Workers Kind/Phase/payload rules is rejected
// explicitly and appends nothing.
func TestPublishRecord_InvalidDraft_ReturnsErrorAndCommitsNoRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	invalid := workers.Draft{Kind: workers.KindTool, Phase: workers.PhaseUpdated, Payload: json.RawMessage(`{}`)}
	if _, err := registry.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, invalid)); err == nil {
		t.Fatalf("PublishRecord() error = nil, want a validation error")
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 0 {
		t.Fatalf("committed record count = %d, want 0", len(committed))
	}
}

// TestPublishRecord_MalformedSourceIdentity_ReturnsErrorAndCommitsNoRecord
// proves that an incomplete Events idempotency identity is rejected before
// any Events append is attempted.
func TestPublishRecord_MalformedSourceIdentity_ReturnsErrorAndCommitsNoRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	req := validPublishRecordRequest("worker-1", 1, toolDraft("tc-1"))
	req.SourceType = ""
	if _, err := registry.PublishRecord(ctx, req); err == nil {
		t.Fatalf("PublishRecord() error = nil, want an identity validation error")
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 0 {
		t.Fatalf("committed record count = %d, want 0", len(committed))
	}
}

// TestPublishRecord_EventsAppendFailure_ReturnsErrorExplicitly proves that an
// Events-level append failure surfaces as an explicit error rather than a
// fabricated success result.
func TestPublishRecord_EventsAppendFailure_ReturnsErrorExplicitly(t *testing.T) {
	registry, err := service.New(succeedingExecution(), &brokenEventsAppender{}, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	result, err := registry.PublishRecord(context.Background(), validPublishRecordRequest("worker-1", 1, toolDraft("tc-1")))
	if err == nil {
		t.Fatalf("PublishRecord() error = nil, want the broken appender's error")
	}
	if result != (workersessions.PublishRecordResult{}) {
		t.Fatalf("PublishRecord() result = %+v, want the zero value on failure", result)
	}
}

// TestPublishRecord_DuplicateSourceIdentity_ReturnsOriginalWithoutNewRecord
// proves that repeating the exact (SourceType, SourceID, SourceSequence,
// SourceEventID) tuple resolves to the original committed record instead of
// appending a second one.
func TestPublishRecord_DuplicateSourceIdentity_ReturnsOriginalWithoutNewRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	req := validPublishRecordRequest("worker-1", 1, toolDraft("tc-1"))
	first, err := registry.PublishRecord(ctx, req)
	if err != nil {
		t.Fatalf("first PublishRecord() error = %v, want nil", err)
	}
	if first.Outcome != workersessions.PublishOutcomeAccepted {
		t.Fatalf("first PublishRecord() outcome = %v, want Accepted", first.Outcome)
	}

	second, err := registry.PublishRecord(ctx, req)
	if err != nil {
		t.Fatalf("second PublishRecord() error = %v, want nil", err)
	}
	if second.Outcome != workersessions.PublishOutcomeDuplicate {
		t.Fatalf("second PublishRecord() outcome = %v, want Duplicate", second.Outcome)
	}
	if second.AggregateSequence != first.AggregateSequence {
		t.Fatalf("second PublishRecord() aggregate sequence = %d, want %d (unchanged)", second.AggregateSequence, first.AggregateSequence)
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 1 {
		t.Fatalf("committed record count = %d, want exactly 1", len(committed))
	}
}

// TestPublishRecord_CallerPayloadMutationAfterPublish_DoesNotAffectRetainedRecord
// proves that mutating the caller-owned Payload byte slice after PublishRecord
// returns cannot alter the committed record.
func TestPublishRecord_CallerPayloadMutationAfterPublish_DoesNotAffectRetainedRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	original, err := json.Marshal(workers.ProgressPayload{Label: "original"})
	if err != nil {
		t.Fatalf("marshal payload error = %v", err)
	}
	mutable := append(json.RawMessage(nil), original...)
	draft := workers.Draft{Kind: workers.KindProgress, Phase: workers.PhaseUpdated, Payload: mutable}

	if _, err := registry.PublishRecord(ctx, validPublishRecordRequest("worker-1", 1, draft)); err != nil {
		t.Fatalf("PublishRecord() error = %v, want nil", err)
	}

	for i := range mutable {
		mutable[i] = 'x'
	}

	committed := readAllDrafts(t, eventsSvc, workersessions.Topic("worker-1"))
	if len(committed) != 1 {
		t.Fatalf("committed record count = %d, want 1", len(committed))
	}
	if string(committed[0].Payload) != string(original) {
		t.Fatalf("committed payload = %s, want unaffected %s", committed[0].Payload, original)
	}
}

// TestPublishRecord_ConcurrentSessionIsolation proves that concurrently
// publishing source records for distinct Worker Sessions never interleaves
// across topics: each session's topic, read independently, contains only its
// own records in its own submitted order.
func TestPublishRecord_ConcurrentSessionIsolation(t *testing.T) {
	eventsSvc := newEventsAppender()
	registry, err := service.New(succeedingExecution(), eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	const sessions = 5
	const recordsPerSession = 20

	var wg sync.WaitGroup
	for s := range sessions {
		sessionID := fmt.Sprintf("worker-%d", s)
		wg.Add(1)
		go func(sessionID string) {
			defer wg.Done()
			for i := 1; i <= recordsPerSession; i++ {
				req := validPublishRecordRequest(sessionID, events.SourceSequence(i), progressDraft(sessionID))
				req.SourceEventID = events.SourceEventID(fmt.Sprintf("%s-evt-%d", sessionID, i))
				if _, err := registry.PublishRecord(ctx, req); err != nil {
					panic(err)
				}
			}
		}(sessionID)
	}
	wg.Wait()

	for s := range sessions {
		sessionID := fmt.Sprintf("worker-%d", s)
		committed := readAllDrafts(t, eventsSvc, workersessions.Topic(sessionID))
		if len(committed) != recordsPerSession {
			t.Fatalf("session %s committed record count = %d, want %d", sessionID, len(committed), recordsPerSession)
		}
	}
}

func TestPublishRecordRequest_Validate_RejectsBlankSessionID(t *testing.T) {
	req := validPublishRecordRequest("   ", 1, toolDraft("tc-1"))
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("Validate() error = %v, want ErrInvalidSessionID", err)
	}
}
