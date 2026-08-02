// Package chatsessions is the public Chat Sessions service boundary.
//
// Peer-facing root contract (source of truth for published slices):
//   - Service — singular cross-service seam
//
// Chat Sessions currently owns only the narrow Factory-target execution
// dependency described on Service: start, invoke, cancel, and close one
// Factory Session, in the published factory_sessions root vocabulary.
// internal/factorysessionsshim is the sole implementation, adapting the
// public factory_sessions.Service root without importing its internals or
// becoming a second session authority. It is registered under "Shims
// registered for deletion" in
// docs/internal/projects/root-consolidation/proposal.md, retired at L3
// Factory Sessions sealing.
package chatsessions
