// Package responseevents defines the provider-neutral FactoryResponseEvent
// vocabulary for transient agent activity observed during a Factory Session.
//
// # Ephemeral observation records
//
// FactoryResponseEvent records are ephemeral observation data. They capture
// provider-neutral agent activity while a session is live or recently active.
// They must not be projected into canonical factory replay and must not be used
// to derive canonical work state after replay. Canonical lifecycle, dispatch,
// and work outcomes remain owned by factoryapi.FactoryEvent and its projections.
//
// # Resolved public contract decisions (v1, documentation only)
//
// Later transport, storage, SSE, CLI, and adapter lanes implement these
// decisions; this package owns the vocabulary types and validation only.
//
// Public HTTP endpoint:
//
//	GET /factory-sessions/{session_id}/response-events
//
// SSE reconnect cursor:
//
//	Clients resume with the after_sequence query parameter using
//	FactoryResponseEvent.sequence as the monotonic session-scoped cursor.
//
// v1 retention:
//
//	Session-wide retention with global limits and snapshot priority when
//	compaction is required. MESSAGE and TOOL snapshots are preferred over
//	high-volume deltas when retention pressure forces drops.
//
// CLI response-stream JSON:
//
//	The you run --output response-stream JSON schema evolves in place on
//	agent-factory.response-event.v1 without a temporary --response-stream-version
//	flag.
//
// Tool argument and result publication:
//
//	Defaults to bounded redacted structured summaries (ToolPayload.argumentsSummary
//	and ToolPayload.resultSummary) rather than raw provider protocol payloads.
//
// Service restart:
//
//	v1 does not imply durable replay of response events across service restart.
//	Consumers treat the stream as live-session observation, not canonical history.
//
// # Vocabulary surface
//
// The envelope carries schemaVersion, eventId, sequence, recordedAt,
// factorySessionId, runId, kind, phase, provenance, payload, and optional
// correlation fields. ValidateEvent rejects invalid kind/phase/payload
// combinations before publication. Raw provider payloads and internal compaction
// types are not part of this public surface.
//
// # Isolation
//
// This package does not import CLI, HTTP, subprocess, or provider packages.
package responseevents
