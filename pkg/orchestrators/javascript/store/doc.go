// Package store owns JavaScript orchestrator durable session state projections
// such as source snapshots, phase summaries, checkpoints, artifacts, and replay
// indexes. Runtime VM state stays outside general session snapshots.
//
// Store read/write implementations land in later contract-repair and runtime
// batches; this package establishes orchestrator ownership for Batch 002+ work.
package store
