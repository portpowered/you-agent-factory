package events

import (
	"errors"
	"testing"
)

func TestNewFactorySessionResponseEventsTopic(t *testing.T) {
	topic, err := NewFactorySessionResponseEventsTopic("s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topic != "factory-session/s1/response-events" {
		t.Fatalf("unexpected topic: %q", topic)
	}

	if _, err := NewFactorySessionResponseEventsTopic(""); !errors.Is(err, ErrInvalidSessionID) {
		t.Fatalf("expected errors.Is(err, ErrInvalidSessionID), got %v", err)
	}
	if _, err := NewFactorySessionResponseEventsTopic("s/1"); !errors.Is(err, ErrInvalidSessionID) {
		t.Fatalf("expected errors.Is(err, ErrInvalidSessionID) for a session id containing '/', got %v", err)
	}
}

func TestNewChatSessionEventsTopic(t *testing.T) {
	topic, err := NewChatSessionEventsTopic("c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topic != "chat-session/c1/events" {
		t.Fatalf("unexpected topic: %q", topic)
	}

	if _, err := NewChatSessionEventsTopic(""); !errors.Is(err, ErrInvalidSessionID) {
		t.Fatalf("expected errors.Is(err, ErrInvalidSessionID), got %v", err)
	}
}

func TestParseTopic(t *testing.T) {
	tests := []struct {
		name    string
		topic   TopicID
		want    ParsedTopic
		wantErr error
	}{
		{
			name:  "valid factory session topic",
			topic: "factory-session/s1/response-events",
			want:  ParsedTopic{Kind: SessionKindFactory, SessionID: "s1"},
		},
		{
			name:  "valid chat session topic",
			topic: "chat-session/c1/events",
			want:  ParsedTopic{Kind: SessionKindChat, SessionID: "c1"},
		},
		{
			name:    "empty topic",
			topic:   "",
			wantErr: ErrInvalidTopic,
		},
		{
			name:    "missing segments",
			topic:   "factory-session/s1",
			wantErr: ErrInvalidTopic,
		},
		{
			name:    "too many segments",
			topic:   "factory-session/s1/response-events/extra",
			wantErr: ErrInvalidTopic,
		},
		{
			name:    "empty session id",
			topic:   "factory-session//response-events",
			wantErr: ErrInvalidTopic,
		},
		{
			name:    "unknown session kind",
			topic:   "worker-session/w1/events",
			wantErr: ErrInvalidTopic,
		},
		{
			name:    "mismatched suffix for factory session",
			topic:   "factory-session/s1/events",
			wantErr: ErrInvalidTopic,
		},
		{
			name:    "mismatched suffix for chat session",
			topic:   "chat-session/c1/response-events",
			wantErr: ErrInvalidTopic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTopic(tt.topic)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected errors.Is(err, %v), got %v", tt.wantErr, err)
				}
				var verr *ValidationError
				if !errors.As(err, &verr) {
					t.Fatalf("expected a *ValidationError, got %T", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %+v, got %+v", tt.want, got)
			}
		})
	}
}

func TestParseTopicRoundTripsConstructedTopics(t *testing.T) {
	factoryTopic, err := NewFactorySessionResponseEventsTopic("s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed, err := ParseTopic(factoryTopic)
	if err != nil {
		t.Fatalf("unexpected error parsing constructed factory topic: %v", err)
	}
	if parsed != (ParsedTopic{Kind: SessionKindFactory, SessionID: "s1"}) {
		t.Fatalf("unexpected round-trip result: %+v", parsed)
	}

	chatTopic, err := NewChatSessionEventsTopic("c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed, err = ParseTopic(chatTopic)
	if err != nil {
		t.Fatalf("unexpected error parsing constructed chat topic: %v", err)
	}
	if parsed != (ParsedTopic{Kind: SessionKindChat, SessionID: "c1"}) {
		t.Fatalf("unexpected round-trip result: %+v", parsed)
	}
}
