// Package chatsessions is the public, transport-neutral Chat Sessions
// service boundary described by
// docs/internal/projects/acp-client/final-proposal.md section 4 and by
// docs/internal/projects/root-consolidation/proposal.md's Service peer
// contract.
//
// This packet defines pure public values, exhaustive enum and transition
// validation, and structured typed errors for request identity, unversioned
// Chat targets, session and target-episode lifecycle, turn admission and
// version-checked mutation, independent attachments, and race-safe control
// intents. It introduces no persistence, no Worker Sessions behavior, and no
// second normalized event vocabulary: pkg/services/workers/response_drafts.go
// remains the sole owner of the Worker event Kind/Phase taxonomy.
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
