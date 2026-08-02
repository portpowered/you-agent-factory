// Package events is the public Events service boundary: a process-local,
// in-memory event stream root that gives ACP Core and later Worker Events one
// stable session-scoped boundary for append, source attachment, retained
// reads, and subscriptions. Persistence and event stream are separate
// concerns (D2 in docs/internal/projects/acp-program/README.md): Recordings
// (pkg/services/recordings) remains the canonical durable Factory Event
// ledger, and Events introduces no durable journal (D1), no internal/store
// package, and no second event taxonomy. Source payloads cross the contract
// as source-native JSON — Events does not convert them into an Events-owned
// kind union.
//
// This package currently publishes the L1 V0 slice described in
// docs/internal/projects/acp-client/final-proposal.md §5: detached identity,
// position, envelope, and outcome contracts only. It has no implementation
// and no wire construction; those belong to a later slice.
package events
