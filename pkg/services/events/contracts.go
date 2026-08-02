package events

import "context"

// Service is the singular Events root contract for the session-scoped,
// single-process, dependency-free L1 V0 event stream. Append is the
// operation this contract-only iteration publishes; AttachSource, Read, and
// Subscribe are published as additive methods on this same Service by later
// contract-only stories.
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
}
