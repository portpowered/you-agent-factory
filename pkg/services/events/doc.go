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
