package events

import (
	"errors"
	"testing"
)

func validReadRequest() ReadRequest {
	topic := TopicID("factory-session/s1/response-events")
	return ReadRequest{
		Topic: topic,
		After: Cursor{Topic: topic, Generation: 1, Mode: CursorBeginning},
		Limit: 10,
	}
}

func TestReadRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(ReadRequest) ReadRequest
		wantErr error
	}{
		{
			name:    "valid request",
			mutate:  func(r ReadRequest) ReadRequest { return r },
			wantErr: nil,
		},
		{
			name:    "empty topic",
			mutate:  func(r ReadRequest) ReadRequest { r.Topic = ""; return r },
			wantErr: ErrInvalidTopic,
		},
		{
			name: "malformed cursor shape",
			mutate: func(r ReadRequest) ReadRequest {
				r.After.Mode = "UNKNOWN"
				return r
			},
			wantErr: ErrInvalidCursorMode,
		},
		{
			name: "ambiguous cursor position",
			mutate: func(r ReadRequest) ReadRequest {
				r.After.Mode = CursorAt
				r.After.At = 0
				return r
			},
			wantErr: ErrAmbiguousCursorPosition,
		},
		{
			name: "cursor topic does not match request topic",
			mutate: func(r ReadRequest) ReadRequest {
				r.After.Topic = "factory-session/other/response-events"
				return r
			},
			wantErr: ErrCursorForeignTopic,
		},
		{
			name:    "zero limit",
			mutate:  func(r ReadRequest) ReadRequest { r.Limit = 0; return r },
			wantErr: ErrInvalidReadLimit,
		},
		{
			name:    "negative limit",
			mutate:  func(r ReadRequest) ReadRequest { r.Limit = -1; return r },
			wantErr: ErrInvalidReadLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mutate(validReadRequest()).Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no validation error, got %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected errors.Is(err, %v), got %v", tt.wantErr, err)
			}
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("expected a *ValidationError, got %T", err)
			}
		})
	}
}

func TestReadOutcomeValuesAreDistinct(t *testing.T) {
	outcomes := map[ReadOutcome]struct{}{
		ReadOutcomeComplete:  {},
		ReadOutcomeTruncated: {},
		ReadOutcomeGap:       {},
	}
	if len(outcomes) != 3 {
		t.Fatalf("expected 3 distinct ReadOutcome values, got %d", len(outcomes))
	}
}

func validCursor() Cursor {
	return Cursor{Topic: "factory-session/s1/response-events", Generation: 1, Mode: CursorBeginning}
}

func TestCursorValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(Cursor) Cursor
		wantErr error
	}{
		{
			name:    "valid beginning cursor",
			mutate:  func(c Cursor) Cursor { return c },
			wantErr: nil,
		},
		{
			name:    "empty topic",
			mutate:  func(c Cursor) Cursor { c.Topic = ""; return c },
			wantErr: ErrInvalidTopic,
		},
		{
			name:    "zero generation",
			mutate:  func(c Cursor) Cursor { c.Generation = 0; return c },
			wantErr: ErrInvalidCursorGeneration,
		},
		{
			name:    "negative generation",
			mutate:  func(c Cursor) Cursor { c.Generation = -1; return c },
			wantErr: ErrInvalidCursorGeneration,
		},
		{
			name: "beginning mode with a position is ambiguous",
			mutate: func(c Cursor) Cursor {
				c.Mode = CursorBeginning
				c.At = 5
				return c
			},
			wantErr: ErrAmbiguousCursorPosition,
		},
		{
			name: "live head mode with a position is ambiguous",
			mutate: func(c Cursor) Cursor {
				c.Mode = CursorLiveHead
				c.At = 5
				return c
			},
			wantErr: ErrAmbiguousCursorPosition,
		},
		{
			name: "at mode without a position is ambiguous",
			mutate: func(c Cursor) Cursor {
				c.Mode = CursorAt
				c.At = 0
				return c
			},
			wantErr: ErrAmbiguousCursorPosition,
		},
		{
			name: "at mode with a non-positive position is ambiguous",
			mutate: func(c Cursor) Cursor {
				c.Mode = CursorAt
				c.At = -3
				return c
			},
			wantErr: ErrAmbiguousCursorPosition,
		},
		{
			name: "at mode with a positive position is valid",
			mutate: func(c Cursor) Cursor {
				c.Mode = CursorAt
				c.At = 7
				return c
			},
			wantErr: nil,
		},
		{
			name: "live head mode without a position is valid",
			mutate: func(c Cursor) Cursor {
				c.Mode = CursorLiveHead
				return c
			},
			wantErr: nil,
		},
		{
			name:    "unknown mode",
			mutate:  func(c Cursor) Cursor { c.Mode = "UNKNOWN"; return c },
			wantErr: ErrInvalidCursorMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mutate(validCursor()).Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected no validation error, got %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected errors.Is(err, %v), got %v", tt.wantErr, err)
			}
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("expected a *ValidationError, got %T", err)
			}
		})
	}
}

func TestCursorClassifyAgainst(t *testing.T) {
	c := Cursor{Topic: "factory-session/s1/response-events", Generation: 3, Mode: CursorBeginning}

	tests := []struct {
		name       string
		topic      TopicID
		generation StreamGeneration
		want       CursorStatus
	}{
		{
			name:       "matching topic and generation is valid",
			topic:      c.Topic,
			generation: c.Generation,
			want:       CursorStatusValid,
		},
		{
			name:       "different topic is foreign",
			topic:      "factory-session/other/response-events",
			generation: c.Generation,
			want:       CursorStatusForeignTopic,
		},
		{
			name:       "different generation is stale",
			topic:      c.Topic,
			generation: c.Generation + 1,
			want:       CursorStatusStaleGeneration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.ClassifyAgainst(tt.topic, tt.generation)
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestCursorStatusValuesAreDistinct(t *testing.T) {
	statuses := map[CursorStatus]struct{}{
		CursorStatusValid:           {},
		CursorStatusForeignTopic:    {},
		CursorStatusStaleGeneration: {},
	}
	if len(statuses) != 3 {
		t.Fatalf("expected 3 distinct CursorStatus values, got %d", len(statuses))
	}
}
