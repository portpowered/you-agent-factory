// Package responseeventstore owns the immutable session-scoped buffer of
// FactoryResponseEvent records for one live Factory Session runtime.
//
// Unlike canonical factory event history, stored response events are ephemeral
// observation records. They are not projected into replay and must not derive
// canonical work state after replay.
//
// Each store instance belongs to one session runtime. FactoryService may
// coordinate session lifecycle, but the ordered event buffer itself is
// session-runtime-local state rather than service-wide mutable configuration.
//
// Retention applies hard session-wide event-count and serialized-envelope byte
// limits. It removes the oldest event in the lowest semantic tier first while
// preserving every retained envelope and its published identity verbatim.
package responseeventstore
