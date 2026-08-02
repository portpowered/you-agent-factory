package events

import (
	"errors"
	"testing"
)

func TestCursorValidate(t *testing.T) {
	tests := []struct {
		name    string
		cursor  Cursor
		wantErr error
	}{
		{"start of topic is valid", Cursor{Topic: "chat-session/abc/events"}, nil},
		{"advanced position is valid", Cursor{Topic: "chat-session/abc/events", Position: 7}, nil},
		{"zero value is invalid", Cursor{}, ErrEmptyTopic},
		{"malformed topic", Cursor{Topic: " chat-session/abc/events"}, ErrMalformedTopic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cursor.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCursorBelongsTo(t *testing.T) {
	cursor := Cursor{Topic: "chat-session/abc/events", Position: 3}

	if !cursor.BelongsTo("chat-session/abc/events") {
		t.Fatalf("BelongsTo() = false, want true for the issuing topic")
	}
	if cursor.BelongsTo("chat-session/other/events") {
		t.Fatalf("BelongsTo() = true, want false for a different topic")
	}
	if cursor.BelongsTo("factory-session/abc/response-events") {
		t.Fatalf("BelongsTo() = true, want false for a different topic family")
	}
}

func TestRetainedRangeValidate(t *testing.T) {
	tests := []struct {
		name    string
		rng     RetainedRange
		wantErr error
	}{
		{
			name:    "empty topic with no records",
			rng:     RetainedRange{Topic: "chat-session/abc/events"},
			wantErr: nil,
		},
		{
			name:    "single retained record",
			rng:     RetainedRange{Topic: "chat-session/abc/events", Earliest: 1, Head: 1},
			wantErr: nil,
		},
		{
			name:    "multiple retained records",
			rng:     RetainedRange{Topic: "chat-session/abc/events", Earliest: 5, Head: 20},
			wantErr: nil,
		},
		{
			name:    "zero value is invalid",
			rng:     RetainedRange{},
			wantErr: ErrEmptyTopic,
		},
		{
			name:    "malformed topic",
			rng:     RetainedRange{Topic: " chat-session/abc/events"},
			wantErr: ErrMalformedTopic,
		},
		{
			name:    "earliest reserved position with a nonzero head",
			rng:     RetainedRange{Topic: "chat-session/abc/events", Earliest: 0, Head: 5},
			wantErr: ErrInvalidRetainedRange,
		},
		{
			name:    "earliest after head",
			rng:     RetainedRange{Topic: "chat-session/abc/events", Earliest: 10, Head: 5},
			wantErr: ErrInvalidRetainedRange,
		},
		{
			name:    "nonzero earliest with a zero head",
			rng:     RetainedRange{Topic: "chat-session/abc/events", Earliest: 1, Head: 0},
			wantErr: ErrInvalidRetainedRange,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.rng.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
