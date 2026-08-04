package workersessions

import (
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// PublishOutcome distinguishes a newly committed Worker record from a
// duplicate resolved to its originally accepted Events identity. Both
// outcomes report the same AggregateSequence: a caller cannot distinguish
// them by position, only by Outcome.
type PublishOutcome int

const (
	PublishOutcomeUnspecified PublishOutcome = iota
	// PublishOutcomeAccepted reports that the record was newly committed and
	// assigned a new Events aggregate position.
	PublishOutcomeAccepted
	// PublishOutcomeDuplicate reports that an equivalent
	// (SourceType, SourceID, SourceSequence, SourceEventID) tuple was already
	// committed to this Worker Session's topic; the returned
	// AggregateSequence is the original position, not a new one.
	PublishOutcomeDuplicate
)

// PublishRecordRequest asks Service to append one validated, detached
// source-native workers.Draft record onto Topic(SessionID). SourceType,
// SourceID, SourceSequence, and SourceEventID form the complete explicit
// Events idempotency identity (events.AppendIdentity): repeating that exact
// tuple on the same Worker Session resolves to the original committed
// record instead of a second one. PublishRecord does not translate Draft
// into an Events-owned or ACP-owned kind union: Draft's existing Kind,
// Phase, Payload, and Provenance are preserved verbatim on the committed
// record.
type PublishRecordRequest struct {
	SessionID      string
	Draft          workers.Draft
	SourceType     events.SourceType
	SourceID       events.SourceID
	SourceSequence events.SourceSequence
	SourceEventID  events.SourceEventID
	SchemaID       events.SchemaID
}

// Validate reports whether req is well-formed enough to attempt publication:
// SessionID is a well-formed stable identity, every Events identity field
// (including SchemaID) is well-formed, and Draft satisfies the existing
// Workers draft validation rules (workers.ValidateDraft). Validate is pure
// and does not mutate req or call Events.
func (req PublishRecordRequest) Validate() error {
	if !validSessionID(req.SessionID) {
		return ErrInvalidSessionID
	}
	identity := events.AppendIdentity{
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
	}
	if err := identity.Validate(); err != nil {
		return err
	}
	if err := req.SchemaID.Validate(); err != nil {
		return err
	}
	return workers.ValidateDraft(req.Draft)
}

// PublishRecordResult is the detached outcome of one PublishRecord call.
type PublishRecordResult struct {
	SessionID string
	// AggregateSequence is the committed record's position within
	// Topic(SessionID), in commit order.
	AggregateSequence events.AggregateSequence
	Outcome           PublishOutcome
}
