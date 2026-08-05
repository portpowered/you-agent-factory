package workersessions

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProviderSessionObservationPublisher serializes the required Worker
// Sessions association ahead of the existing downstream progress publisher.
// Factory Runtime constructs it before Workers execution is assembled and
// binds the session-owned Service once its pool is available. A reference that
// cannot be committed is deliberately not forwarded, so no response output
// can present an unassociated, malformed, or foreign Provider Session as a
// resumable Worker Session.
type ProviderSessionObservationPublisher struct {
	mu       sync.RWMutex
	observer Service
	next     workers.ProgressPublisher
}

// NewProviderSessionObservationPublisher creates an unbound progress bridge.
// Non-reference-bearing fragments continue to the supplied publisher, while
// reference-bearing fragments wait for Bind and a successful Worker Sessions
// association before they are forwarded.
func NewProviderSessionObservationPublisher(next workers.ProgressPublisher) *ProviderSessionObservationPublisher {
	return &ProviderSessionObservationPublisher{next: next}
}

// Bind attaches the one Worker Sessions service that owns the runtime's
// supervision registry. Binding is intentionally a construction-time action;
// replacing an existing observer would risk routing a live dispatch to a
// different Factory Runtime session, so only the first non-nil observer wins.
func (p *ProviderSessionObservationPublisher) Bind(observer Service) {
	if p == nil || observer == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.observer == nil {
		p.observer = observer
	}
}

// Publish commits a detached exact Provider Session observation before it
// forwards the original Workers progress fragment. This method intentionally
// has the same no-return signature as workers.ProgressPublisher: rejection is
// recorded by Worker Sessions' typed operation and safely suppresses only the
// reference-bearing output rather than fabricating fallback provider work.
func (p *ProviderSessionObservationPublisher) Publish(fragment workers.ProgressFragment) {
	if p == nil {
		return
	}
	p.mu.RLock()
	observer, next := p.observer, p.next
	p.mu.RUnlock()

	metadata := workers.CloneProviderSessionMetadata(fragment.ProviderSessionRef)
	reference := workers.CloneProviderSessionReference(fragment.ProviderSessionReference)
	if reference != nil && metadata != nil &&
		(reference.Provider.String() != metadata.Provider || reference.Kind != metadata.Kind || reference.ID != metadata.ID) {
		// The public response metadata must agree with the exact typed source
		// reference. Forwarding a mismatch could display another provider
		// session under this Worker Session even though association succeeded.
		return
	}
	if reference == nil && metadata != nil {
		// Metadata is a response projection, not a trusted provider-native
		// SessionRef. In particular, it can be compatibility data derived from
		// a request's configured SessionID. Do not reconstruct a resumable
		// reference from it or let dependent output claim an unassociated
		// Provider Session.
		return
	}
	if reference != nil {
		if observer == nil {
			return
		}
		_, err := observer.ObserveProviderSession(context.Background(), ProviderSessionObservationRequest{
			DispatchID: fragment.DispatchID,
			Reference:  reference.Clone(),
		})
		if err != nil {
			return
		}
	}
	if next != nil {
		next(fragment)
	}
}

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
