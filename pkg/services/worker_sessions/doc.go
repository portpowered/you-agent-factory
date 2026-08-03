// Package workersessions is the ACP Worker Events W1+W2 foundation: one
// stable process-scoped Worker Session identity, the exact eight-state
// lifecycle vocabulary, immutable deterministic Get/List inspection, and
// supervised Start with exactly-once terminal classification, over a
// concurrency-safe in-memory registry.
//
// W1 exposes Reserve, Get, and List. W2 adds Start: validated
// identity-before-handoff supervision of one already-resolved Workers
// attempt through one directly injected workers.WorkstationExecutionService,
// and result-first terminal classification into COMPLETED or FAILED with a
// non-nil typed FailureCause. Start is synchronous — it returns only once
// the started session has committed its exactly-once terminal outcome, and
// that committed outcome is observable through Get/List immediately after.
// Asynchronous supervision, Events publication, Runtime and Provider Session
// association, Pause/Resume/Cancel/Terminate controls, persistence, and
// transport behavior remain later ACP Worker Events slices (W3-W7) and are
// intentionally absent from this package.
//
// Implementation state lives under internal/. Peers depend on Service and the
// request/result/value types published at this root.
package workersessions
