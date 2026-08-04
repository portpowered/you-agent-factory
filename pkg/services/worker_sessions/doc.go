// Package workersessions is the ACP Worker Events W1+W2+W3 foundation: one
// stable process-scoped Worker Session identity, the exact eight-state
// lifecycle vocabulary, immutable deterministic Get/List inspection, and
// supervised Start with exactly-once terminal classification, over a
// concurrency-safe in-memory registry.
//
// W1 exposes Reserve, Get, and List. W2 adds Start: validated
// identity-before-handoff supervision of one already-resolved Workers
// attempt through one directly injected workers.WorkstationExecutionService,
// and result-first terminal classification into COMPLETED or FAILED with a
// non-nil typed FailureCause. W3 attaches Start to one directly injected
// Events service: before Workers is ever invoked, Start commits a
// KindSession/PhaseStarted workers.Draft to Topic(id)
// (worker-session/<id>/events), and a failure establishing that opening
// record terminalizes FAILED with FailureCauseEventPublicationFailure
// without calling Workers at all. Start is synchronous — it returns only
// once the started session has committed its exactly-once terminal outcome,
// and that committed outcome is observable through Get/List immediately
// after. Publishing Worker source observations and the terminal SESSION
// projection onto the same topic, asynchronous supervision, Runtime and
// Provider Session association, Pause/Resume/Cancel/Terminate controls,
// persistence, and transport behavior remain later ACP Worker Events slices
// (W3 remainder, W4-W7) and are intentionally absent from this package.
//
// Implementation state lives under internal/. Peers depend on Service and the
// request/result/value types published at this root.
package workersessions
