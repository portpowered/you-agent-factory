// Package responseeventstore owns the immutable session-scoped buffer of
// FactoryResponseEvent records for one live Factory Session runtime.
//
// Unlike canonical factory event history, stored response events are ephemeral
// observation records. They are not projected into replay and must not derive
// canonical work state after replay.
//
// Each store instance belongs to one session runtime. The Factory Session
// coordinator manages lifecycle, but the ordered event buffer itself remains
// session-runtime-local state rather than coordinator-wide configuration.
//
// Retention applies hard session-wide event-count and serialized-envelope byte
// limits. It removes the oldest event in the lowest semantic tier first while
// preserving every retained envelope and its published identity verbatim.
// Removed sequence spans are tracked separately. A stale subscription read
// receives one cursor-relative STREAM_GAP before retained catch-up. Gap markers
// are out-of-band (sequence zero), are never retained, and do not consume or
// reuse the monotonically assigned identities of published events.
package responseeventstore
