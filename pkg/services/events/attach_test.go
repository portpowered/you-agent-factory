package events

import (
	"errors"
	"testing"
)

func validAttachSourceRequest() AttachSourceRequest {
	source := Topic("factory-session/abc/response-events")
	return AttachSourceRequest{
		Destination: "chat-session/abc/events",
		Source:      source,
		StartAt:     Cursor{Topic: source, Position: 3},
		Mode:        AttachModeRetainedThenLive,
	}
}

func TestAttachSourceRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(AttachSourceRequest) AttachSourceRequest
		wantErr error
	}{
		{"valid retained-then-live", func(r AttachSourceRequest) AttachSourceRequest { return r }, nil},
		{"valid live-only", func(r AttachSourceRequest) AttachSourceRequest { r.Mode = AttachModeLiveOnly; return r }, nil},
		{"missing destination", func(r AttachSourceRequest) AttachSourceRequest { r.Destination = ""; return r }, ErrEmptyTopic},
		{"missing source", func(r AttachSourceRequest) AttachSourceRequest { r.Source = ""; return r }, ErrEmptyTopic},
		{
			name: "self attachment",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.Source = r.Destination
				r.StartAt = Cursor{Topic: r.Destination, Position: 3}
				return r
			},
			wantErr: ErrSelfAttachment,
		},
		{"unsupported mode", func(r AttachSourceRequest) AttachSourceRequest { r.Mode = AttachMode(99); return r }, ErrUnsupportedAttachMode},
		{"invalid starting position", func(r AttachSourceRequest) AttachSourceRequest { r.StartAt = Cursor{}; return r }, ErrEmptyTopic},
		{
			name: "starting cursor belongs to a different topic",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.StartAt = Cursor{Topic: r.Destination, Position: 3}
				return r
			},
			wantErr: ErrIncompatibleAttachmentCursor,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.mutate(validAttachSourceRequest())
			if err := req.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAttachmentIDValidate(t *testing.T) {
	tests := []struct {
		name    string
		id      AttachmentID
		wantErr error
	}{
		{
			name:    "valid",
			id:      AttachmentID{Destination: "chat-session/abc/events", Source: "factory-session/abc/response-events"},
			wantErr: nil,
		},
		{
			name:    "self attachment",
			id:      AttachmentID{Destination: "chat-session/abc/events", Source: "chat-session/abc/events"},
			wantErr: ErrSelfAttachment,
		},
		{
			name:    "malformed destination",
			id:      AttachmentID{Destination: " chat-session/abc/events", Source: "factory-session/abc/response-events"},
			wantErr: ErrMalformedTopic,
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

func TestAttachModeValidate(t *testing.T) {
	tests := []struct {
		name    string
		mode    AttachMode
		wantErr error
	}{
		{"retained then live", AttachModeRetainedThenLive, nil},
		{"live only", AttachModeLiveOnly, nil},
		{"unspecified", AttachModeUnspecified, ErrUnsupportedAttachMode},
		{"unknown", AttachMode(42), ErrUnsupportedAttachMode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.mode.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAttachOutcomeValues(t *testing.T) {
	id := AttachmentID{Destination: "chat-session/abc/events", Source: "factory-session/abc/response-events"}

	accepted := AttachSourceResult{ID: id, Outcome: AttachOutcomeAccepted}
	alreadyAttached := AttachSourceResult{ID: id, Outcome: AttachOutcomeAlreadyAttached}

	if accepted.Outcome == alreadyAttached.Outcome {
		t.Fatalf("accepted and already-attached outcomes must be distinct")
	}
	if accepted.ID != alreadyAttached.ID {
		t.Fatalf("accepted and already-attached outcomes must report the same stable AttachmentID")
	}
}
