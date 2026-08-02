package events

import (
	"errors"
	"testing"
)

func validAppendRequest() AppendRequest {
	return AppendRequest{
		Topic:          "factory-session/s1/response-events",
		SourceType:     "factory_session",
		SourceID:       "s1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		Schema:         "factory_session.response_event.v1",
		Payload:        "opaque source-native text",
	}
}

func TestAppendRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(AppendRequest) AppendRequest
		wantErr error
	}{
		{
			name:    "valid request",
			mutate:  func(r AppendRequest) AppendRequest { return r },
			wantErr: nil,
		},
		{
			name:    "empty topic",
			mutate:  func(r AppendRequest) AppendRequest { r.Topic = ""; return r },
			wantErr: ErrInvalidTopic,
		},
		{
			name:    "empty source type",
			mutate:  func(r AppendRequest) AppendRequest { r.SourceType = ""; return r },
			wantErr: ErrInvalidSourceType,
		},
		{
			name:    "empty source id",
			mutate:  func(r AppendRequest) AppendRequest { r.SourceID = ""; return r },
			wantErr: ErrInvalidSourceID,
		},
		{
			name:    "zero source sequence",
			mutate:  func(r AppendRequest) AppendRequest { r.SourceSequence = 0; return r },
			wantErr: ErrInvalidSourceSequence,
		},
		{
			name:    "negative source sequence",
			mutate:  func(r AppendRequest) AppendRequest { r.SourceSequence = -5; return r },
			wantErr: ErrInvalidSourceSequence,
		},
		{
			name:    "empty source event id",
			mutate:  func(r AppendRequest) AppendRequest { r.SourceEventID = ""; return r },
			wantErr: ErrInvalidSourceEventID,
		},
		{
			name:    "empty schema id",
			mutate:  func(r AppendRequest) AppendRequest { r.Schema = ""; return r },
			wantErr: ErrInvalidSchemaID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mutate(validAppendRequest()).Validate()
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

// TestAppendRequestValidateIgnoresPayloadVocabulary proves Validate accepts
// representative payloads from different source vocabularies without any
// taxonomy-specific interpretation: only envelope identity fields matter.
func TestAppendRequestValidateIgnoresPayloadVocabulary(t *testing.T) {
	type toolCallPayload struct {
		Name string
		Args map[string]any
	}

	payloads := []Payload{
		nil,
		"plain string payload",
		[]byte{0x01, 0x02, 0x03},
		map[string]any{"kind": "message_delta", "text": "hi"},
		toolCallPayload{Name: "search", Args: map[string]any{"query": "docs"}},
		42,
	}

	for i, payload := range payloads {
		req := validAppendRequest()
		req.SourceEventID = SourceEventID(indexedID(i))
		req.Payload = payload

		if err := req.Validate(); err != nil {
			t.Fatalf("payload %d (%#v): expected no validation error, got %v", i, payload, err)
		}
	}
}

func indexedID(i int) string {
	return "evt-payload-" + string(rune('a'+i))
}

func TestAppendRequestKeyMatchesRecordKeyShape(t *testing.T) {
	req := validAppendRequest()

	got := req.Key()
	want := IdempotencyKey{
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
	}
	if got != want {
		t.Fatalf("expected Key() = %+v, got %+v", want, got)
	}
}

func TestAppendOutcomeValuesAreDistinct(t *testing.T) {
	if AppendOutcomeCommitted == AppendOutcomeDuplicate {
		t.Fatalf("expected AppendOutcomeCommitted and AppendOutcomeDuplicate to be distinct values")
	}
}
