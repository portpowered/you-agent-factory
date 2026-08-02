// Package events defines the dependency-free L1 V0 public contract for the
// session-scoped, single-process Events stream: Append, AttachSource, Read,
// and Subscribe over validated delivery envelopes.
//
// Events owns no persistence, filesystem, database, or storage-engine
// responsibility. Recordings remains the canonical JSONL Factory Event
// ledger; process exit ends all Events state. Events also owns no
// second event-kind taxonomy: source-native payloads pass through this
// contract opaque and uninterpreted.
//
// This package is contract-only for its V0 iteration. It publishes detached
// request, result, record, and typed-error values plus the singular Service
// interface later in-memory implementations and independent callers depend
// on; it does not construct, wire, or migrate any concrete implementation.
package events

import "context"

// Service is the singular Events root contract for the session-scoped,
// single-process, dependency-free L1 V0 event stream. Append, AttachSource,
// and Read are the operations this contract-only iteration publishes;
// Subscribe is published as an additive method on this same Service by a
// later contract-only story.
type Service interface {
	// Append commits one source-native delivery envelope to a destination
	// Topic in commit order and returns the assigned Record identity,
	// suppressing duplicate delivery using the exact (SourceType, SourceID,
	// SourceSequence, SourceEventID) idempotency tuple.
	//
	// Append returns a *ValidationError when req fails Validate. It never
	// inspects, transforms, or reinterprets req.Payload, and it defines no
	// persistence, filesystem, or storage-engine behavior: a committed Record
	// exists only for the lifetime of the owning process.
	Append(ctx context.Context, req AppendRequest) (AppendResult, error)

	// AttachSource attaches req.SourceTopic as an input to req.DestinationTopic
	// and returns a stable AttachmentID plus a typed attached/already-attached
	// or conflict outcome.
	//
	// AttachSource returns a *ValidationError when req fails Validate. It
	// defines no service, callback, transport, or storage-engine behavior of
	// its own: an attachment exists only for the lifetime of the owning
	// process.
	AttachSource(ctx context.Context, req AttachSourceRequest) (AttachSourceResult, error)

	// Read returns a bounded, ordered slice of committed Records for
	// req.Topic strictly after req.After, up to req.Limit Records.
	//
	// Read returns a *ValidationError when req fails Validate, including
	// when req.After names a different Topic than req.Topic. It returns
	// ErrCursorStaleGeneration when req.After names a StreamGeneration other
	// than the Topic's current one, and ErrTopicNotFound when req.Topic is
	// unknown to the Service; both are runtime classifications Validate
	// cannot make on req's shape alone. A requested range bounded retention
	// has evicted is reported as a ReadResult with ReadOutcomeGap, never as
	// an error and never as silent loss.
	Read(ctx context.Context, req ReadRequest) (ReadResult, error)

	// Subscribe opens a bounded, ordered live Subscription to req.Topic
	// starting at req.Start, buffering at most req.Capacity undelivered
	// SubscriptionDelivery observations.
	//
	// Subscribe returns a *ValidationError only when req fails Validate on
	// its own shape. Every runtime classification — an unknown or stale
	// req.Start, retention loss, topic completion, caller cancellation, and
	// slow-consumer backpressure — is reported through the returned
	// Subscription's SubscriptionTerminal, never as an error from Subscribe
	// itself and never as silent record loss. Subscribe never blocks on, or
	// is blocked by, a slow consumer: a subscriber that does not keep pace
	// within req.Capacity ends with SubscriptionTerminalBackpressure rather
	// than pausing committed Append progress or buffering unboundedly.
	Subscribe(ctx context.Context, req SubscribeRequest) (SubscribeResult, error)
}
