package events

import (
	"encoding/json"
	"errors"
	"testing"
)

func validAppendRequest() AppendRequest {
	return AppendRequest{
		Topic:          "chat-session/abc/events",
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{"tool":"grep","status":"ok"}`),
	}
}

func TestAppendRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(AppendRequest) AppendRequest
		wantErr error
	}{
		{"valid", func(r AppendRequest) AppendRequest { return r }, nil},
		{"missing topic", func(r AppendRequest) AppendRequest { r.Topic = ""; return r }, ErrEmptyTopic},
		{"missing source type", func(r AppendRequest) AppendRequest { r.SourceType = ""; return r }, ErrEmptySourceType},
		{"missing source id", func(r AppendRequest) AppendRequest { r.SourceID = ""; return r }, ErrEmptySourceID},
		{"missing source sequence", func(r AppendRequest) AppendRequest { r.SourceSequence = 0; return r }, ErrInvalidSourceSequence},
		{"missing source event id", func(r AppendRequest) AppendRequest { r.SourceEventID = ""; return r }, ErrEmptySourceEventID},
		{"missing schema id", func(r AppendRequest) AppendRequest { r.SchemaID = ""; return r }, ErrEmptySchemaID},
		{"empty payload", func(r AppendRequest) AppendRequest { r.Payload = nil; return r }, ErrEmptyPayload},
		{"invalid json payload", func(r AppendRequest) AppendRequest { r.Payload = json.RawMessage(`{not json`); return r }, ErrMalformedPayloadJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.mutate(validAppendRequest())
			if err := req.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAppendRequestIdentity(t *testing.T) {
	req := validAppendRequest()
	want := AppendIdentity{
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
	}
	if got := req.Identity(); got != want {
		t.Fatalf("Identity() = %+v, want %+v", got, want)
	}
}

func TestAppendRequestDetachedPayload(t *testing.T) {
	original := json.RawMessage(`{"tool":"grep"}`)
	req := validAppendRequest()
	req.Payload = original

	detached := req.Detached()
	original[2] = 'X' // mutate the byte backing the caller's original slice

	if string(detached.Payload) != `{"tool":"grep"}` {
		t.Fatalf("Detached().Payload = %s, want unaffected by caller mutation", detached.Payload)
	}
}

func TestRecordValidate(t *testing.T) {
	valid := Record{
		ID:             RecordID{Topic: "chat-session/abc/events", Position: 1},
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "worker.output.v1",
		Payload:        json.RawMessage(`{"tool":"grep"}`),
	}

	tests := []struct {
		name    string
		mutate  func(Record) Record
		wantErr error
	}{
		{"valid", func(r Record) Record { return r }, nil},
		{"unassigned position", func(r Record) Record { r.ID.Position = 0; return r }, ErrInvalidAggregateSequence},
		{"empty payload", func(r Record) Record { r.Payload = nil; return r }, ErrEmptyPayload},
		{"invalid json payload", func(r Record) Record { r.Payload = json.RawMessage(`{`); return r }, ErrMalformedPayloadJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := tt.mutate(valid)
			if err := rec.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRecordDetachedPayload(t *testing.T) {
	original := json.RawMessage(`{"tool":"grep"}`)
	rec := Record{
		ID:      RecordID{Topic: "chat-session/abc/events", Position: 1},
		Payload: original,
	}

	detached := rec.Detached()
	original[2] = 'X'

	if string(detached.Payload) != `{"tool":"grep"}` {
		t.Fatalf("Detached().Payload = %s, want unaffected by caller mutation", detached.Payload)
	}
}

func TestAppendIdentityValidate(t *testing.T) {
	valid := AppendIdentity{
		SourceType:     "worker.tool",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
	}

	tests := []struct {
		name    string
		mutate  func(AppendIdentity) AppendIdentity
		wantErr error
	}{
		{"valid", func(id AppendIdentity) AppendIdentity { return id }, nil},
		{"missing source type", func(id AppendIdentity) AppendIdentity { id.SourceType = ""; return id }, ErrEmptySourceType},
		{"missing source id", func(id AppendIdentity) AppendIdentity { id.SourceID = ""; return id }, ErrEmptySourceID},
		{"missing source sequence", func(id AppendIdentity) AppendIdentity { id.SourceSequence = 0; return id }, ErrInvalidSourceSequence},
		{"missing source event id", func(id AppendIdentity) AppendIdentity { id.SourceEventID = ""; return id }, ErrEmptySourceEventID},
		{"malformed source id", func(id AppendIdentity) AppendIdentity { id.SourceID = " worker-1"; return id }, ErrMalformedSourceID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := tt.mutate(valid)
			if err := id.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAppendRequestValidateDelegatesToIdentity(t *testing.T) {
	req := validAppendRequest()
	req.SourceSequence = 0

	if err := req.Validate(); !errors.Is(err, ErrInvalidSourceSequence) {
		t.Fatalf("Validate() = %v, want %v (delegated from Identity().Validate())", err, ErrInvalidSourceSequence)
	}
}

func TestAppendOutcomeValues(t *testing.T) {
	rec := Record{ID: RecordID{Topic: "chat-session/abc/events", Position: 1}}

	accepted := AppendResult{Record: rec, Outcome: AppendOutcomeAccepted}
	duplicate := AppendResult{Record: rec, Outcome: AppendOutcomeDuplicate}

	if accepted.Outcome == duplicate.Outcome {
		t.Fatalf("accepted and duplicate outcomes must be distinct")
	}
	if accepted.Record.ID != duplicate.Record.ID {
		t.Fatalf("accepted and duplicate outcomes must report the same stable Record.ID")
	}
}
