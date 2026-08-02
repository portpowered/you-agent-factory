package events

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

const testTopic Topic = "chat-session/abc/events"

func TestReadRequestValidate(t *testing.T) {
	valid := ReadRequest{Topic: testTopic, From: Cursor{Topic: testTopic, Position: 2}, Limit: 10}

	tests := []struct {
		name    string
		mutate  func(ReadRequest) ReadRequest
		wantErr error
	}{
		{"valid", func(r ReadRequest) ReadRequest { return r }, nil},
		{"missing topic", func(r ReadRequest) ReadRequest { r.Topic = ""; return r }, ErrEmptyTopic},
		{"invalid cursor", func(r ReadRequest) ReadRequest { r.From = Cursor{}; return r }, ErrEmptyTopic},
		{
			name: "cursor from a different topic",
			mutate: func(r ReadRequest) ReadRequest {
				r.From = Cursor{Topic: "factory-session/abc/response-events"}
				return r
			},
			wantErr: ErrCursorTopicMismatch,
		},
		{"zero limit", func(r ReadRequest) ReadRequest { r.Limit = 0; return r }, ErrInvalidReadLimit},
		{"negative limit", func(r ReadRequest) ReadRequest { r.Limit = -1; return r }, ErrInvalidReadLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.mutate(valid)
			if err := req.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSubscribeRequestValidate(t *testing.T) {
	valid := SubscribeRequest{Topic: testTopic, From: Cursor{Topic: testTopic}, Limit: 1}

	tests := []struct {
		name    string
		mutate  func(SubscribeRequest) SubscribeRequest
		wantErr error
	}{
		{"valid", func(r SubscribeRequest) SubscribeRequest { return r }, nil},
		{"missing topic", func(r SubscribeRequest) SubscribeRequest { r.Topic = ""; return r }, ErrEmptyTopic},
		{
			name: "cursor from a different topic",
			mutate: func(r SubscribeRequest) SubscribeRequest {
				r.From = Cursor{Topic: "factory-session/abc/response-events"}
				return r
			},
			wantErr: ErrCursorTopicMismatch,
		},
		{"zero limit", func(r SubscribeRequest) SubscribeRequest { r.Limit = 0; return r }, ErrInvalidReadLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.mutate(valid)
			if err := req.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestReadResultValidate(t *testing.T) {
	rec1 := Record{
		ID:             RecordID{Topic: testTopic, Position: 1},
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{"tool":"grep"}`),
	}
	rec2 := Record{
		ID:             RecordID{Topic: testTopic, Position: 2},
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 2,
		SourceEventID:  "evt-2",
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{"tool":"grep"}`),
	}
	otherTopicRec := Record{
		ID:             RecordID{Topic: "factory-session/abc/response-events", Position: 1},
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{}`),
	}
	next := Cursor{Topic: testTopic, Position: 2}
	retained := RetainedRange{Topic: testTopic, Earliest: 1, Head: 2}
	atHeadNext := Cursor{Topic: testTopic, Position: 9}
	atHeadRetained := RetainedRange{Topic: testTopic, Earliest: 1, Head: 9}
	gap := &GapFacts{Topic: testTopic, Requested: 1, EarliestRetained: 5, Head: 9}

	tests := []struct {
		name    string
		result  ReadResult
		wantErr error
	}{
		{
			"progress with records",
			ReadResult{Outcome: ReadOutcomeProgress, Records: []Record{rec1, rec2}, Next: next, Retained: retained},
			nil,
		},
		{"progress with no records is inconsistent", ReadResult{Outcome: ReadOutcomeProgress}, ErrInconsistentReadOutcome},
		{
			"progress with a malformed record",
			ReadResult{
				Outcome:  ReadOutcomeProgress,
				Records:  []Record{{ID: RecordID{Topic: testTopic, Position: 1}}},
				Next:     next,
				Retained: retained,
			},
			ErrEmptySourceType,
		},
		{
			"progress with a mixed-topic record",
			ReadResult{Outcome: ReadOutcomeProgress, Records: []Record{otherTopicRec}, Next: next, Retained: retained},
			ErrCursorTopicMismatch,
		},
		{
			"progress with a zero Next cursor",
			ReadResult{Outcome: ReadOutcomeProgress, Records: []Record{rec1}, Retained: retained},
			ErrEmptyTopic,
		},
		{
			"progress with a Next cursor mismatched to the last record",
			ReadResult{Outcome: ReadOutcomeProgress, Records: []Record{rec1, rec2}, Next: Cursor{Topic: testTopic, Position: 1}, Retained: retained},
			ErrInconsistentReadOutcome,
		},
		{
			"progress with an inconsistent retained range",
			ReadResult{Outcome: ReadOutcomeProgress, Records: []Record{rec1, rec2}, Next: next, Retained: RetainedRange{Topic: testTopic, Earliest: 5, Head: 2}},
			ErrInvalidRetainedRange,
		},
		{
			"progress with a record outside the retained range",
			ReadResult{Outcome: ReadOutcomeProgress, Records: []Record{rec1, rec2}, Next: next, Retained: RetainedRange{Topic: testTopic, Earliest: 2, Head: 2}},
			ErrInconsistentReadOutcome,
		},
		{
			"progress with records out of order",
			ReadResult{Outcome: ReadOutcomeProgress, Records: []Record{rec2, rec1}, Next: next, Retained: retained},
			ErrInconsistentReadOutcome,
		},
		{
			"at head with no records",
			ReadResult{Outcome: ReadOutcomeAtHead, Next: atHeadNext, Retained: atHeadRetained},
			nil,
		},
		{
			"at head with records is inconsistent",
			ReadResult{Outcome: ReadOutcomeAtHead, Records: []Record{rec1}, Next: atHeadNext, Retained: atHeadRetained},
			ErrInconsistentReadOutcome,
		},
		{
			"at head with a Next cursor not at the retained head",
			ReadResult{Outcome: ReadOutcomeAtHead, Next: Cursor{Topic: testTopic, Position: 5}, Retained: atHeadRetained},
			ErrInconsistentReadOutcome,
		},
		{
			"at head with an invalid Next cursor",
			ReadResult{Outcome: ReadOutcomeAtHead, Next: Cursor{}, Retained: atHeadRetained},
			ErrEmptyTopic,
		},
		{
			"progress with a Next/Retained topic mismatch",
			ReadResult{
				Outcome: ReadOutcomeProgress,
				Records: []Record{rec1},
				Next:    next,
				Retained: RetainedRange{
					Topic:    "factory-session/abc/response-events",
					Earliest: 1,
					Head:     2,
				},
			},
			ErrCursorTopicMismatch,
		},
		{"invalid cursor with no records", ReadResult{Outcome: ReadOutcomeInvalidCursor}, nil},
		{
			"invalid cursor with fabricated records is inconsistent",
			ReadResult{Outcome: ReadOutcomeInvalidCursor, Records: []Record{rec1}},
			ErrInconsistentReadOutcome,
		},
		{"gap with gap facts", ReadResult{Outcome: ReadOutcomeGap, Gap: gap}, nil},
		{"gap without gap facts is inconsistent", ReadResult{Outcome: ReadOutcomeGap}, ErrInconsistentReadOutcome},
		{"gap with fabricated records is inconsistent", ReadResult{Outcome: ReadOutcomeGap, Gap: gap, Records: []Record{rec1}}, ErrInconsistentReadOutcome},
		{
			"gap with malformed gap facts is inconsistent",
			ReadResult{Outcome: ReadOutcomeGap, Gap: &GapFacts{Topic: testTopic}},
			ErrInvalidGapFacts,
		},
		{"unspecified outcome is inconsistent", ReadResult{}, ErrInconsistentReadOutcome},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.result.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGapFactsIdentifyResumablePosition(t *testing.T) {
	gap := GapFacts{Topic: testTopic, Requested: 3, EarliestRetained: 10, Head: 40}

	if gap.Topic != testTopic {
		t.Fatalf("Topic = %v, want %v", gap.Topic, testTopic)
	}
	if gap.EarliestRetained <= gap.Requested {
		t.Fatalf("EarliestRetained = %d, want greater than Requested %d to describe real loss", gap.EarliestRetained, gap.Requested)
	}
	if gap.Head < gap.EarliestRetained {
		t.Fatalf("Head = %d, want >= EarliestRetained %d", gap.Head, gap.EarliestRetained)
	}
}

func TestGapFactsValidate(t *testing.T) {
	valid := GapFacts{Topic: testTopic, Requested: 3, EarliestRetained: 10, Head: 40}

	tests := []struct {
		name    string
		mutate  func(GapFacts) GapFacts
		wantErr error
	}{
		{"valid", func(g GapFacts) GapFacts { return g }, nil},
		{"missing topic", func(g GapFacts) GapFacts { g.Topic = ""; return g }, ErrEmptyTopic},
		{"zero head describes no history", func(g GapFacts) GapFacts { g.Head = 0; return g }, ErrInvalidGapFacts},
		{"zero earliest retained with nonzero head", func(g GapFacts) GapFacts { g.EarliestRetained = 0; return g }, ErrInvalidGapFacts},
		{"earliest retained beyond head", func(g GapFacts) GapFacts { g.EarliestRetained = 41; return g }, ErrInvalidGapFacts},
		{"requested at or past earliest retained", func(g GapFacts) GapFacts { g.Requested = 10; return g }, ErrInvalidGapFacts},
		{"requested past earliest retained", func(g GapFacts) GapFacts { g.Requested = 12; return g }, ErrInvalidGapFacts},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gap := tt.mutate(valid)
			if err := gap.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeliveryValidate(t *testing.T) {
	rec := Record{
		ID:             RecordID{Topic: testTopic, Position: 1},
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "worker.output.v1",
		Payload:        []byte(`{"tool":"grep"}`),
	}
	cur := Cursor{Topic: testTopic, Position: 1}
	gap := &GapFacts{Topic: testTopic, Requested: 1, EarliestRetained: 5, Head: 9}

	tests := []struct {
		name     string
		delivery Delivery
		wantErr  error
	}{
		{"record delivery", Delivery{Kind: DeliveryRecord, Record: rec, Cursor: cur}, nil},
		{"record delivery missing record", Delivery{Kind: DeliveryRecord, Cursor: cur}, ErrInconsistentDelivery},
		{"record delivery missing cursor", Delivery{Kind: DeliveryRecord, Record: rec}, ErrInconsistentDelivery},
		{"record delivery with a leftover gap", Delivery{Kind: DeliveryRecord, Record: rec, Cursor: cur, Gap: gap}, ErrInconsistentDelivery},
		{
			name:     "record delivery with a cursor from another topic",
			delivery: Delivery{Kind: DeliveryRecord, Record: rec, Cursor: Cursor{Topic: "factory-session/abc/response-events", Position: 1}},
			wantErr:  ErrCursorTopicMismatch,
		},
		{
			name:     "record delivery with a cursor position mismatched to the record",
			delivery: Delivery{Kind: DeliveryRecord, Record: rec, Cursor: Cursor{Topic: testTopic, Position: 2}},
			wantErr:  ErrInconsistentDelivery,
		},
		{
			name:     "record delivery with a malformed record",
			delivery: Delivery{Kind: DeliveryRecord, Record: Record{ID: RecordID{Topic: testTopic, Position: 1}}, Cursor: cur},
			wantErr:  ErrEmptySourceType,
		},
		{
			name:     "record delivery with a malformed cursor",
			delivery: Delivery{Kind: DeliveryRecord, Record: rec, Cursor: Cursor{Topic: " chat-session/abc/events", Position: 1}},
			wantErr:  ErrMalformedTopic,
		},
		{"gap delivery", Delivery{Kind: DeliveryGap, Gap: gap}, nil},
		{"gap delivery with nil gap facts", Delivery{Kind: DeliveryGap}, ErrInconsistentDelivery},
		{"gap delivery with malformed gap facts", Delivery{Kind: DeliveryGap, Gap: &GapFacts{Topic: testTopic}}, ErrInvalidGapFacts},
		{"gap delivery with a leftover record", Delivery{Kind: DeliveryGap, Gap: gap, Record: rec}, ErrInconsistentDelivery},
		{"gap delivery with a leftover cursor", Delivery{Kind: DeliveryGap, Gap: gap, Cursor: cur}, ErrInconsistentDelivery},
		{
			name:     "gap delivery with a partial record carrying only a payload",
			delivery: Delivery{Kind: DeliveryGap, Gap: gap, Record: Record{Payload: []byte(`"secret"`)}},
			wantErr:  ErrInconsistentDelivery,
		},
		{
			name:     "gap delivery with a partial record carrying only a non-nil empty payload",
			delivery: Delivery{Kind: DeliveryGap, Gap: gap, Record: Record{Payload: json.RawMessage{}}},
			wantErr:  ErrInconsistentDelivery,
		},
		{
			name:     "gap delivery with a partial record carrying only source metadata",
			delivery: Delivery{Kind: DeliveryGap, Gap: gap, Record: Record{SourceType: "worker.tool"}},
			wantErr:  ErrInconsistentDelivery,
		},
		{"closed delivery", Delivery{Kind: DeliveryClosed}, nil},
		{"closed delivery with a leftover record", Delivery{Kind: DeliveryClosed, Record: rec}, ErrInconsistentDelivery},
		{
			name:     "closed delivery with a partial record carrying only a payload",
			delivery: Delivery{Kind: DeliveryClosed, Record: Record{Payload: []byte(`"secret"`)}},
			wantErr:  ErrInconsistentDelivery,
		},
		{
			name:     "closed delivery with a partial record carrying only a non-nil empty payload",
			delivery: Delivery{Kind: DeliveryClosed, Record: Record{Payload: json.RawMessage{}}},
			wantErr:  ErrInconsistentDelivery,
		},
		{"canceled delivery", Delivery{Kind: DeliveryCanceled}, nil},
		{"canceled delivery with a leftover cursor", Delivery{Kind: DeliveryCanceled, Cursor: cur}, ErrInconsistentDelivery},
		{
			name:     "canceled delivery with a partial record carrying only a schema id",
			delivery: Delivery{Kind: DeliveryCanceled, Record: Record{SchemaID: "worker.output.v1"}},
			wantErr:  ErrInconsistentDelivery,
		},
		{
			name:     "canceled delivery with a partial record carrying only a non-nil empty payload",
			delivery: Delivery{Kind: DeliveryCanceled, Record: Record{Payload: json.RawMessage{}}},
			wantErr:  ErrInconsistentDelivery,
		},
		{"backpressure delivery", Delivery{Kind: DeliveryBackpressure}, nil},
		{"backpressure delivery with a leftover gap", Delivery{Kind: DeliveryBackpressure, Gap: gap}, ErrInconsistentDelivery},
		{
			name:     "backpressure delivery with a partial record carrying only a source sequence",
			delivery: Delivery{Kind: DeliveryBackpressure, Record: Record{SourceSequence: 1}},
			wantErr:  ErrInconsistentDelivery,
		},
		{
			name:     "backpressure delivery with a partial record carrying only a non-nil empty payload",
			delivery: Delivery{Kind: DeliveryBackpressure, Record: Record{Payload: json.RawMessage{}}},
			wantErr:  ErrInconsistentDelivery,
		},
		{"unspecified delivery", Delivery{}, ErrInconsistentDelivery},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.delivery.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestOperationFailureErrorsClassify(t *testing.T) {
	if !errors.Is(ErrUnknownTopic, ErrOperationFailed) {
		t.Fatalf("ErrUnknownTopic must classify as ErrOperationFailed")
	}
	if !errors.Is(ErrUnresolvableCursor, ErrOperationFailed) {
		t.Fatalf("ErrUnresolvableCursor must classify as ErrOperationFailed")
	}
	if errors.Is(ErrUnknownTopic, ErrUnresolvableCursor) {
		t.Fatalf("ErrUnknownTopic must not classify as the unrelated ErrUnresolvableCursor")
	}
	if errors.Is(ErrOperationFailed, ErrUnknownTopic) {
		t.Fatalf("the general ErrOperationFailed must not classify as the narrower ErrUnknownTopic")
	}
	if errors.Is(ErrInconsistentReadOutcome, ErrOperationFailed) {
		t.Fatalf("a validation/consistency error must not classify as an operation failure")
	}
}

func TestSubscriptionNext(t *testing.T) {
	rec := Record{ID: RecordID{Topic: testTopic, Position: 1}}
	delivered := Delivery{Kind: DeliveryRecord, Record: rec, Cursor: Cursor{Topic: testTopic, Position: 1}}

	var sub Subscription = func(ctx context.Context) Delivery {
		return delivered
	}

	got := sub.Next(context.Background())
	if got.Kind != delivered.Kind || got.Record.ID != delivered.Record.ID || got.Cursor != delivered.Cursor {
		t.Fatalf("Next() = %+v, want %+v", got, delivered)
	}
}

func TestNilSubscriptionReportsClosed(t *testing.T) {
	var sub Subscription

	got := sub.Next(context.Background())
	if got.Kind != DeliveryClosed {
		t.Fatalf("Next().Kind = %v, want DeliveryClosed for a nil Subscription", got.Kind)
	}
}

func TestSubscriptionReportsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var sub Subscription = func(ctx context.Context) Delivery {
		if ctx.Err() != nil {
			return Delivery{Kind: DeliveryCanceled}
		}
		return Delivery{Kind: DeliveryRecord}
	}

	got := sub.Next(ctx)
	if got.Kind != DeliveryCanceled {
		t.Fatalf("Next().Kind = %v, want DeliveryCanceled once the context is canceled", got.Kind)
	}
}

func TestSubscriptionReportsBackpressureTermination(t *testing.T) {
	var sub Subscription = func(ctx context.Context) Delivery {
		return Delivery{Kind: DeliveryBackpressure}
	}

	got := sub.Next(context.Background())
	if got.Kind != DeliveryBackpressure {
		t.Fatalf("Next().Kind = %v, want DeliveryBackpressure as an explicit terminal outcome, never silent loss", got.Kind)
	}
}
