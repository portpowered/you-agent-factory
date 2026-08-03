// Package workersessions is the ACP Worker Events W1 foundation: one stable
// process-scoped Worker Session identity, the exact eight-state lifecycle
// vocabulary, and immutable deterministic Get/List inspection over a
// concurrency-safe in-memory registry.
//
// W1 exposes only Reserve, Get, and List. Worker launch, supervision,
// terminal-cause classification, Events publication, Runtime and Provider
// Session association, Pause/Resume/Cancel/Terminate controls, persistence,
// and transport behavior belong to later ACP Worker Events slices (W2-W7)
// and are intentionally absent from this package.
//
// Implementation state lives under internal/. Peers depend on Service and the
// request/result/value types published at this root.
package workersessions
