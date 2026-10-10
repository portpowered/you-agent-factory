package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
)

func validAppendRequest() events.AppendRequest {
	return events.AppendRequest{
		Topic:          "chat-session/abc/events",
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{"tool":"grep","status":"ok"}`),
	}
}

func TestAppend_AcceptsAndAssignsFirstPosition(t *testing.T) {
	st := NewWithRetention(0, logging.NoopLogger{})
	result, err := st.Append(context.Background(), validAppendRequest())
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if result.Outcome != events.AppendOutcomeAccepted {
		t.Fatalf("Outcome = %v, want AppendOutcomeAccepted", result.Outcome)
	}
	if result.Record.ID.Position != 1 {
		t.Fatalf("Position = %d, want 1", result.Record.ID.Position)
	}
	if err := result.Record.Validate(); err != nil {
		t.Fatalf("Record.Validate() = %v, want a fully consistent public Record", err)
	}
}

func TestAppend_AssignsContiguousPositionsInCallOrder(t *testing.T) {
	st := NewWithRetention(0, logging.NoopLogger{})
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		req := validAppendRequest()
		req.SourceSequence = events.SourceSequence(i)
		req.SourceEventID = events.SourceEventID("evt-" + string(rune('0'+i)))
		result, err := st.Append(ctx, req)
		if err != nil {
			t.Fatalf("Append() [%d] error = %v", i, err)
		}
		if result.Record.ID.Position != events.AggregateSequence(i) {
			t.Fatalf("Append() [%d] Position = %d, want %d", i, result.Record.ID.Position, i)
		}
	}
}

func TestAppend_DifferentTopicsHaveIndependentOrdering(t *testing.T) {
	st := NewWithRetention(0, logging.NoopLogger{})
	ctx := context.Background()

	reqA := validAppendRequest()
	reqA.Topic = "chat-session/a/events"
	reqB := validAppendRequest()
	reqB.Topic = "chat-session/b/events"

	resultA, err := st.Append(ctx, reqA)
	if err != nil {
		t.Fatalf("Append(A) error = %v", err)
	}
	resultB, err := st.Append(ctx, reqB)
	if err != nil {
		t.Fatalf("Append(B) error = %v", err)
	}
	if resultA.Record.ID.Position != 1 || resultB.Record.ID.Position != 1 {
		t.Fatalf("independent topics must both start at position 1, got A=%d B=%d",
			resultA.Record.ID.Position, resultB.Record.ID.Position)
	}
}

func TestAppend_ConcurrentTopicsPreserveDetachedHistoryAndPeerSubscription(t *testing.T) {
	t.Parallel()
	st := NewWithRetention(0, logging.NoopLogger{})
	t.Cleanup(func() { _ = st.Close(context.Background()) })
	topics := []events.Topic{"chat-session/a/events", "chat-session/b/events"}
	ctx := t.Context()
	canceled, cancel := context.WithCancel(ctx)
	subA, err := st.Subscribe(canceled, events.SubscribeRequest{Topic: topics[0], From: events.Cursor{Topic: topics[0]}, Limit: 1})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	subB, err := st.Subscribe(ctx, events.SubscribeRequest{Topic: topics[1], From: events.Cursor{Topic: topics[1]}, Limit: 3})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	if got := subA.Next(canceled); got.Kind != events.DeliveryCanceled {
		t.Fatalf("canceled topic delivery = %+v", got)
	}
	var producers sync.WaitGroup
	start := make(chan struct{})
	for _, topic := range topics {
		producers.Go(func() {
			<-start
			appendAndReadDetachedTopicHistory(t, st, ctx, topic)
		})
	}
	close(start)
	producers.Wait()
	if t.Failed() {
		return
	}
	for position := 1; position <= 3; position++ {
		got := subB.Next(ctx)
		if got.Kind != events.DeliveryRecord || got.Record.ID.Topic != topics[1] || got.Record.ID.Position != events.AggregateSequence(position) || string(got.Record.Payload) != string(validAppendRequest().Payload) {
			t.Fatalf("peer delivery = %+v", got)
		}
	}
}

func appendAndReadDetachedTopicHistory(t *testing.T, st *Store, ctx context.Context, topic events.Topic) {
	t.Helper()
	for sequence := 1; sequence <= 3; sequence++ {
		req := validAppendRequest()
		req.Topic = topic
		req.SourceSequence = events.SourceSequence(sequence)
		appended, err := st.Append(ctx, req)
		if err != nil {
			t.Error(err)
			return
		}
		appended.Record.Payload[0] = 'X'
		read, err := st.Read(ctx, events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 3})
		if err != nil || len(read.Records) != sequence {
			t.Errorf("Read(%s) = %+v, %v", topic, read, err)
			return
		}
		for i, record := range read.Records {
			if record.ID.Topic != topic || record.ID.Position != events.AggregateSequence(i+1) || string(record.Payload) != string(req.Payload) {
				t.Errorf("topic %s record = %+v", topic, record)
			}
		}
		read.Records[0].Payload[0] = 'Y'
	}
}

func TestAppend_DuplicateIdentityReturnsOriginalRecordWithoutAdvancingHead(t *testing.T) {
	st := NewWithRetention(0, logging.NoopLogger{})
	ctx := context.Background()
	req := validAppendRequest()

	first, err := st.Append(ctx, req)
	if err != nil {
		t.Fatalf("Append() first error = %v", err)
	}

	// Repeat the identical identity with a different payload; the duplicate
	// outcome must still resolve to the originally accepted payload.
	repeat := req
	repeat.Payload = json.RawMessage(`{"tool":"grep","status":"different"}`)
	second, err := st.Append(ctx, repeat)
	if err != nil {
		t.Fatalf("Append() duplicate error = %v", err)
	}

	if second.Outcome != events.AppendOutcomeDuplicate {
		t.Fatalf("Outcome = %v, want AppendOutcomeDuplicate", second.Outcome)
	}
	if second.Record.ID != first.Record.ID {
		t.Fatalf("duplicate Record.ID = %+v, want %+v (same stable identity)", second.Record.ID, first.Record.ID)
	}
	if string(second.Record.Payload) != string(first.Record.Payload) {
		t.Fatalf("duplicate Record.Payload = %s, want original payload %s", second.Record.Payload, first.Record.Payload)
	}

	// A third distinct append must still land at position 2: the duplicate
	// never advanced the aggregate head.
	third := validAppendRequest()
	third.SourceEventID = "evt-2"
	result, err := st.Append(ctx, third)
	if err != nil {
		t.Fatalf("Append() third error = %v", err)
	}
	if result.Record.ID.Position != 2 {
		t.Fatalf("Position = %d, want 2 (duplicate must not advance the head)", result.Record.ID.Position)
	}
}

func TestAppend_RejectsMalformedRequestBeforeAnyStateChange(t *testing.T) {
	st := NewWithRetention(0, logging.NoopLogger{})
	ctx := context.Background()
	req := validAppendRequest()
	req.Payload = nil // malformed: empty payload

	_, err := st.Append(ctx, req)
	if !errors.Is(err, events.ErrEmptyPayload) {
		t.Fatalf("Append() error = %v, want ErrEmptyPayload", err)
	}

	// A subsequent valid append must still land at position 1: the rejected
	// request must not have advanced the topic head.
	valid, err := st.Append(ctx, validAppendRequest())
	if err != nil {
		t.Fatalf("Append() valid error = %v", err)
	}
	if valid.Record.ID.Position != 1 {
		t.Fatalf("Position = %d, want 1 (rejected append must not change aggregate state)", valid.Record.ID.Position)
	}
}

func TestAppend_RejectsCanceledContextBeforeAnyStateChange(t *testing.T) {
	st := NewWithRetention(0, logging.NoopLogger{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := st.Append(ctx, validAppendRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Append() error = %v, want context.Canceled", err)
	}

	valid, err := st.Append(context.Background(), validAppendRequest())
	if err != nil {
		t.Fatalf("Append() valid error = %v", err)
	}
	if valid.Record.ID.Position != 1 {
		t.Fatalf("Position = %d, want 1 (canceled append must not change aggregate state)", valid.Record.ID.Position)
	}
}

func TestAppend_RejectedAfterCloseBeforeAnyStateChange(t *testing.T) {
	st := NewWithRetention(0, logging.NoopLogger{})
	ctx := context.Background()

	if err := st.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err := st.Append(ctx, validAppendRequest())
	if !errors.Is(err, errClosed) {
		t.Fatalf("Append() after Close error = %v, want ErrClosed", err)
	}
	if !errors.Is(err, events.ErrOperationFailed) {
		t.Fatalf("Append() after Close error = %v, want it to also classify as ErrOperationFailed", err)
	}
}

func TestAppend_RejectedAfterCloseForATopicCreatedAfterwards(t *testing.T) {
	st := NewWithRetention(0, logging.NoopLogger{})
	ctx := context.Background()

	if err := st.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	req := validAppendRequest()
	req.Topic = "chat-session/never-seen-before/events"
	if _, err := st.Append(ctx, req); !errors.Is(err, errClosed) {
		t.Fatalf("Append() on a brand-new topic after Close error = %v, want ErrClosed", err)
	}
}

func TestAppend_CallerMutationOfRequestPayloadCannotAlterStoredRecord(t *testing.T) {
	st := NewWithRetention(0, logging.NoopLogger{})
	ctx := context.Background()

	payload := json.RawMessage(`{"tool":"grep"}`)
	req := validAppendRequest()
	req.Payload = payload

	result, err := st.Append(ctx, req)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	payload[2] = 'X' // mutate the caller's backing array after the call returns

	if string(result.Record.Payload) != `{"tool":"grep"}` {
		t.Fatalf("Record.Payload = %s, want unaffected by caller mutation of the original request", result.Record.Payload)
	}
}

func TestAppend_CallerMutationOfReturnedRecordCannotAlterLaterObservations(t *testing.T) {
	st := NewWithRetention(0, logging.NoopLogger{})
	ctx := context.Background()
	req := validAppendRequest()

	first, err := st.Append(ctx, req)
	if err != nil {
		t.Fatalf("Append() first error = %v", err)
	}
	first.Record.Payload[2] = 'X' // mutate the returned copy's backing array

	repeat := req
	second, err := st.Append(ctx, repeat)
	if err != nil {
		t.Fatalf("Append() duplicate error = %v", err)
	}

	if string(second.Record.Payload) != `{"tool":"grep","status":"ok"}` {
		t.Fatalf("duplicate Record.Payload = %s, want unaffected by mutation of a prior returned Record", second.Record.Payload)
	}
}
