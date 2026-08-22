// Package workersessions is the ACP Worker Events W1+W2+W3 foundation: one
// stable process-scoped Worker Session identity, the exact eight-state
// lifecycle vocabulary, immutable deterministic Get/List inspection, and
// supervised Start with exactly-once terminal classification, over a
// concurrency-safe in-memory registry.
//
// W1 exposes Reserve, Get, and List. W2 adds the shared supervision
// foundation: validated identity-before-handoff supervision of one
// already-resolved Workers attempt through one directly injected
// workers.Service, and result-first terminal
// classification into COMPLETED or FAILED with a non-nil typed FailureCause.
// W3 attaches Start and PublishRecord to one directly injected Events service:
// before Workers is ever invoked, Start commits a KindSession/PhaseStarted
// workers.Draft to Topic(id) (worker-session/<id>/events), and a failure
// establishing that opening record terminalizes FAILED with
// FailureCauseEventPublicationFailure without calling Workers at all.
// Start returns only after retained replay, live subscription, and the Workers
// admission callback are all observable; its server-owned attempt then
// completes asynchronously through the same exactly-once terminal path as
// InvokeSession. Its optional Stop(context.Context) capability is the
// canonical Factory Runtime shutdown hook: it rejects new asynchronous
// starts, routes admitted cancellation through Workers, and joins terminal
// publication.
// PublishRecord lets a caller append validated source-native
// Worker observations onto that same topic using the complete Events
// idempotency identity, but only while the session's publication window is
// open (after its opening record, before its terminal record starts
// committing) and only in non-decreasing SourceSequence order per
// (SourceType, SourceID); every record one session ever commits -- opening,
// published, and terminal alike -- is serialized against the same per-session
// lock, so they can never interleave or commit out of order. InvokeSession
// remains the synchronous compatibility operation and waits for the terminal
// outcome. Runtime and Provider Session association, Pause/Resume,
// Cancel/Terminate controls, persistence, and transport behavior continue to
// build on this shared supervision boundary.
//
// Implementation state lives under internal/. Peers depend on Service and the
// request/result/value types published at this root.
package workersessions
