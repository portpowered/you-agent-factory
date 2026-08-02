package events

import (
	"context"
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
	rec := Record{ID: RecordID{Topic: testTopic, Position: 1}}
	gap := &GapFacts{Topic: testTopic, Requested: 1, EarliestRetained: 5, Head: 9}

	tests := []struct {
		name    string
		result  ReadResult
		wantErr error
	}{
		{"progress with records", ReadResult{Outcome: ReadOutcomeProgress, Records: []Record{rec}}, nil},
		{"progress with no records is inconsistent", ReadResult{Outcome: ReadOutcomeProgress}, ErrInconsistentReadOutcome},
		{"at head with no records", ReadResult{Outcome: ReadOutcomeAtHead}, nil},
		{"at head with records is inconsistent", ReadResult{Outcome: ReadOutcomeAtHead, Records: []Record{rec}}, ErrInconsistentReadOutcome},
		{"invalid cursor with no records", ReadResult{Outcome: ReadOutcomeInvalidCursor}, nil},
		{"gap with gap facts", ReadResult{Outcome: ReadOutcomeGap, Gap: gap}, nil},
		{"gap without gap facts is inconsistent", ReadResult{Outcome: ReadOutcomeGap}, ErrInconsistentReadOutcome},
		{"gap with fabricated records is inconsistent", ReadResult{Outcome: ReadOutcomeGap, Gap: gap, Records: []Record{rec}}, ErrInconsistentReadOutcome},
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
