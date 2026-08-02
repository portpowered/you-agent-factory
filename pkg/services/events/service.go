package events

import "context"

// Service is the singular Events root contract: append, attach a source,
// read, and subscribe over one process-local, in-memory event stream (D1,
// D2 in docs/internal/projects/acp-program/README.md). Peers depend on
// Service rather than a store, journal, or transport type.
type Service interface {
	Append(context.Context, AppendRequest) (AppendResult, error)
	AttachSource(context.Context, AttachSourceRequest) (AttachSourceResult, error)
	Read(context.Context, ReadRequest) (ReadResult, error)
	Subscribe(context.Context, SubscribeRequest) (Subscription, error)
}
