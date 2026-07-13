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
// Retention eviction is intentionally out of scope for the initial store lane;
// published events remain in the retained buffer until a later story introduces
// compaction policy.
package responseeventstore
