package events

import (
	"fmt"
	"strings"
)

// SessionKind names the owning-session form a Topic scopes to: a Factory
// Session's response-events stream or a Chat Session's events stream. Events
// owns no other session kind in V0.
type SessionKind string

const (
	// SessionKindFactory identifies a Factory Session's response-events Topic.
	SessionKindFactory SessionKind = "factory-session"
	// SessionKindChat identifies a Chat Session's events Topic.
	SessionKindChat SessionKind = "chat-session"
)

// SessionID identifies the owning Factory Session or Chat Session a Topic is
// scoped to. It MUST be non-empty and MUST NOT contain a "/" separator.
type SessionID string

const (
	factorySessionTopicSegment = "response-events"
	chatSessionTopicSegment    = "events"
)

// ParsedTopic is the detached decomposition of a valid TopicID into its
// owning SessionKind and SessionID.
type ParsedTopic struct {
	Kind      SessionKind
	SessionID SessionID
}

var (
	// ErrInvalidSessionID reports an empty or malformed SessionID.
	ErrInvalidSessionID = fmt.Errorf("events: invalid session id")
)

// NewFactorySessionResponseEventsTopic constructs the
// factory-session/<id>/response-events TopicID for sessionID, or returns a
// deterministic *ValidationError when sessionID is empty or malformed.
func NewFactorySessionResponseEventsTopic(sessionID SessionID) (TopicID, error) {
	if err := validateSessionID(sessionID); err != nil {
		return "", err
	}
	return TopicID(fmt.Sprintf("%s/%s/%s", SessionKindFactory, sessionID, factorySessionTopicSegment)), nil
}

// NewChatSessionEventsTopic constructs the chat-session/<id>/events TopicID
// for sessionID, or returns a deterministic *ValidationError when sessionID
// is empty or malformed.
func NewChatSessionEventsTopic(sessionID SessionID) (TopicID, error) {
	if err := validateSessionID(sessionID); err != nil {
		return "", err
	}
	return TopicID(fmt.Sprintf("%s/%s/%s", SessionKindChat, sessionID, chatSessionTopicSegment)), nil
}

// ParseTopic decomposes topic into its owning SessionKind and SessionID, or
// returns a deterministic *ValidationError when topic is empty or does not
// match one of the supported L1 forms
// (factory-session/<id>/response-events, chat-session/<id>/events).
func ParseTopic(topic TopicID) (ParsedTopic, error) {
	if topic == "" {
		return ParsedTopic{}, &ValidationError{Field: "topic", Err: ErrInvalidTopic}
	}

	parts := strings.Split(string(topic), "/")
	if len(parts) != 3 || parts[1] == "" {
		return ParsedTopic{}, &ValidationError{Field: "topic", Err: ErrInvalidTopic}
	}

	kind, sessionID, segment := parts[0], parts[1], parts[2]
	switch {
	case kind == string(SessionKindFactory) && segment == factorySessionTopicSegment:
		return ParsedTopic{Kind: SessionKindFactory, SessionID: SessionID(sessionID)}, nil
	case kind == string(SessionKindChat) && segment == chatSessionTopicSegment:
		return ParsedTopic{Kind: SessionKindChat, SessionID: SessionID(sessionID)}, nil
	default:
		return ParsedTopic{}, &ValidationError{Field: "topic", Err: ErrInvalidTopic}
	}
}

func validateSessionID(sessionID SessionID) error {
	if sessionID == "" || strings.Contains(string(sessionID), "/") {
		return &ValidationError{Field: "session_id", Err: ErrInvalidSessionID}
	}
	return nil
}
