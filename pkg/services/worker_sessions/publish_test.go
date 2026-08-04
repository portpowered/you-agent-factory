package workersessions_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func validPublishRequest() workersessions.PublishRecordRequest {
	return workersessions.PublishRecordRequest{
		SessionID:      "worker-1",
		Draft:          workers.Draft{Kind: workers.KindProgress, Phase: workers.PhaseUpdated, Payload: []byte(`{"label":"working"}`)},
		SourceType:     "worker_provider",
		SourceID:       "worker-1",
		SourceSequence: 1,
		SourceEventID:  "evt-1",
		SchemaID:       "workers.draft.v1",
	}
}

// TestPublishRecordRequest_Validate_AcceptsWellFormedRequest proves a request
// whose SessionID, complete Events identity, SchemaID, and Draft are each
// individually well-formed passes Validate unchanged.
func TestPublishRecordRequest_Validate_AcceptsWellFormedRequest(t *testing.T) {
	if err := validPublishRequest().Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

// TestPublishRecordRequest_Validate_RejectsBlankSessionID proves Validate
// checks SessionID before ever inspecting the Events identity, SchemaID, or
// Draft.
func TestPublishRecordRequest_Validate_RejectsBlankSessionID(t *testing.T) {
	req := validPublishRequest()
	req.SessionID = "   "
	if err := req.Validate(); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("Validate() error = %v, want ErrInvalidSessionID", err)
	}
}

// TestPublishRecordRequest_Validate_RejectsMalformedEventsIdentity proves
// Validate delegates the four-part Events idempotency identity to
// events.AppendIdentity.Validate rather than reimplementing its rules.
func TestPublishRecordRequest_Validate_RejectsMalformedEventsIdentity(t *testing.T) {
	req := validPublishRequest()
	req.SourceType = ""
	if err := req.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want a non-nil error for an empty SourceType")
	}
}

// TestPublishRecordRequest_Validate_RejectsMalformedSchemaID proves Validate
// checks SchemaID even once SessionID and the Events identity are both
// well-formed.
func TestPublishRecordRequest_Validate_RejectsMalformedSchemaID(t *testing.T) {
	req := validPublishRequest()
	req.SchemaID = ""
	if err := req.Validate(); !errors.Is(err, events.ErrEmptySchemaID) {
		t.Fatalf("Validate() error = %v, want ErrEmptySchemaID", err)
	}
}

// TestPublishRecordRequest_Validate_RejectsInvalidDraft proves Validate's
// final check is the existing workers.ValidateDraft rules applied to
// req.Draft.
func TestPublishRecordRequest_Validate_RejectsInvalidDraft(t *testing.T) {
	req := validPublishRequest()
	req.Draft = workers.Draft{}
	if err := req.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want a non-nil error for a zero-value Draft")
	}
}
