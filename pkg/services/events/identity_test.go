package events

import (
	"errors"
	"strings"
	"testing"
)

func TestTopicValidate(t *testing.T) {
	tests := []struct {
		name    string
		topic   Topic
		wantErr error
	}{
		{"valid factory session topic", Topic("factory-session/abc-123/response-events"), nil},
		{"valid chat session topic", Topic("chat-session/xyz/events"), nil},
		{"valid arbitrary future topic family", Topic("worker-session/w-1/events"), nil},
		{"empty", Topic(""), ErrEmptyTopic},
		{"internal whitespace", Topic("factory-session/ abc /events"), ErrMalformedTopic},
		{"leading whitespace", Topic(" factory-session/abc/events"), ErrMalformedTopic},
		{"trailing whitespace", Topic("factory-session/abc/events "), ErrMalformedTopic},
		{"control character", Topic("factory-session/abc\n/events"), ErrMalformedTopic},
		{"disallowed character", Topic("factory-session/abc#1/events"), ErrMalformedTopic},
		{"too long", Topic(strings.Repeat("a", maxTopicLength+1)), ErrMalformedTopic},
		{"exactly max length", Topic(strings.Repeat("a", maxTopicLength)), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.topic.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSourceTypeValidate(t *testing.T) {
	tests := []struct {
		name       string
		sourceType SourceType
		wantErr    error
	}{
		{"valid", SourceType("worker.tool"), nil},
		{"empty", SourceType(""), ErrEmptySourceType},
		{"whitespace only", SourceType("   "), ErrMalformedSourceType},
		{"disallowed character", SourceType("worker tool"), ErrMalformedSourceType},
		{"too long", SourceType(strings.Repeat("a", maxSourceTypeLength+1)), ErrMalformedSourceType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.sourceType.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSourceIDValidate(t *testing.T) {
	tests := []struct {
		name     string
		sourceID SourceID
		wantErr  error
	}{
		{"valid", SourceID("worker-42"), nil},
		{"empty", SourceID(""), ErrEmptySourceID},
		{"disallowed character", SourceID("worker@42"), ErrMalformedSourceID},
		{"too long", SourceID(strings.Repeat("a", maxSourceIDLength+1)), ErrMalformedSourceID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.sourceID.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSourceSequenceValidate(t *testing.T) {
	tests := []struct {
		name    string
		seq     SourceSequence
		wantErr error
	}{
		{"zero is unset and invalid", SourceSequence(0), ErrInvalidSourceSequence},
		{"one is the first valid sequence", SourceSequence(1), nil},
		{"large sequence is valid", SourceSequence(1 << 40), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.seq.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSourceEventIDValidate(t *testing.T) {
	tests := []struct {
		name    string
		id      SourceEventID
		wantErr error
	}{
		{"valid", SourceEventID("evt-1"), nil},
		{"empty", SourceEventID(""), ErrEmptySourceEventID},
		{"disallowed character", SourceEventID("evt 1"), ErrMalformedSourceEventID},
		{"too long", SourceEventID(strings.Repeat("a", maxSourceEventIDLength+1)), ErrMalformedSourceEventID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.id.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSchemaIDValidate(t *testing.T) {
	tests := []struct {
		name    string
		id      SchemaID
		wantErr error
	}{
		{"valid", SchemaID("worker.output.v1"), nil},
		{"empty", SchemaID(""), ErrEmptySchemaID},
		{"disallowed character", SchemaID("worker output v1"), ErrMalformedSchemaID},
		{"too long", SchemaID(strings.Repeat("a", maxSchemaIDLength+1)), ErrMalformedSchemaID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.id.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAggregateSequenceValidateAssigned(t *testing.T) {
	tests := []struct {
		name    string
		seq     AggregateSequence
		wantErr error
	}{
		{"zero means before the first record and is not an assigned position", AggregateSequence(0), ErrInvalidAggregateSequence},
		{"one is the first assigned position", AggregateSequence(1), nil},
		{"large position is valid", AggregateSequence(1 << 40), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.seq.ValidateAssigned(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateAssigned() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRecordIDValidate(t *testing.T) {
	tests := []struct {
		name    string
		id      RecordID
		wantErr error
	}{
		{
			name:    "valid",
			id:      RecordID{Topic: "chat-session/abc/events", Position: 1},
			wantErr: nil,
		},
		{
			name:    "zero value is invalid",
			id:      RecordID{},
			wantErr: ErrEmptyTopic,
		},
		{
			name:    "malformed topic",
			id:      RecordID{Topic: " chat-session/abc/events", Position: 1},
			wantErr: ErrMalformedTopic,
		},
		{
			name:    "unassigned position with a valid topic",
			id:      RecordID{Topic: "chat-session/abc/events", Position: 0},
			wantErr: ErrInvalidAggregateSequence,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.id.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
