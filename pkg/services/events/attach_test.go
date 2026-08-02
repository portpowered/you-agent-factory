package events

import (
	"errors"
	"testing"
)

func validAttachSourceRequest() AttachSourceRequest {
	return AttachSourceRequest{
		DestinationTopic: "chat-session/c1/events",
		SourceTopic:      "factory-session/s1/response-events",
		SourceType:       "factory_session",
		SourceID:         "s1",
		Start:            AttachStartPosition{Mode: AttachStartBeginning},
	}
}

func TestAttachSourceRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(AttachSourceRequest) AttachSourceRequest
		wantErr error
	}{
		{
			name:    "valid request",
			mutate:  func(r AttachSourceRequest) AttachSourceRequest { return r },
			wantErr: nil,
		},
		{
			name:    "empty destination topic",
			mutate:  func(r AttachSourceRequest) AttachSourceRequest { r.DestinationTopic = ""; return r },
			wantErr: ErrInvalidTopic,
		},
		{
			name: "malformed destination topic",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.DestinationTopic = "not-a-topic"
				return r
			},
			wantErr: ErrInvalidTopic,
		},
		{
			name:    "empty source topic",
			mutate:  func(r AttachSourceRequest) AttachSourceRequest { r.SourceTopic = ""; return r },
			wantErr: ErrInvalidSourceTopic,
		},
		{
			name: "malformed source topic",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.SourceTopic = "not-a-topic"
				return r
			},
			wantErr: ErrInvalidSourceTopic,
		},
		{
			name: "self attachment",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.SourceTopic = r.DestinationTopic
				return r
			},
			wantErr: ErrSelfAttachment,
		},
		{
			name: "chat source into factory destination is incompatible",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.DestinationTopic = "factory-session/s2/response-events"
				r.SourceTopic = "chat-session/c1/events"
				return r
			},
			wantErr: ErrIncompatibleAttachment,
		},
		{
			name: "factory source into factory destination is incompatible",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.DestinationTopic = "factory-session/s2/response-events"
				return r
			},
			wantErr: ErrIncompatibleAttachment,
		},
		{
			name: "chat source into chat destination is incompatible",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.SourceTopic = "chat-session/c2/events"
				return r
			},
			wantErr: ErrIncompatibleAttachment,
		},
		{
			name:    "empty source type",
			mutate:  func(r AttachSourceRequest) AttachSourceRequest { r.SourceType = ""; return r },
			wantErr: ErrInvalidSourceType,
		},
		{
			name:    "empty source id",
			mutate:  func(r AttachSourceRequest) AttachSourceRequest { r.SourceID = ""; return r },
			wantErr: ErrInvalidSourceID,
		},
		{
			name: "at mode without a position is ambiguous",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.Start = AttachStartPosition{Mode: AttachStartAt}
				return r
			},
			wantErr: ErrAmbiguousStartPosition,
		},
		{
			name: "beginning mode with a position is ambiguous",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.Start = AttachStartPosition{Mode: AttachStartBeginning, At: 5}
				return r
			},
			wantErr: ErrAmbiguousStartPosition,
		},
		{
			name: "live head mode with a position is ambiguous",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.Start = AttachStartPosition{Mode: AttachStartLiveHead, At: 5}
				return r
			},
			wantErr: ErrAmbiguousStartPosition,
		},
		{
			name: "unknown start mode",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.Start = AttachStartPosition{Mode: "UNKNOWN"}
				return r
			},
			wantErr: ErrInvalidStartPositionMode,
		},
		{
			name: "at mode with a positive position is valid",
			mutate: func(r AttachSourceRequest) AttachSourceRequest {
				r.Start = AttachStartPosition{Mode: AttachStartAt, At: 7}
				return r
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mutate(validAttachSourceRequest()).Validate()
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

func TestAttachOutcomeValuesAreDistinct(t *testing.T) {
	outcomes := map[AttachOutcome]struct{}{
		AttachOutcomeAttached:        {},
		AttachOutcomeAlreadyAttached: {},
		AttachOutcomeConflict:        {},
	}
	if len(outcomes) != 3 {
		t.Fatalf("expected 3 distinct AttachOutcome values, got %d", len(outcomes))
	}
}
