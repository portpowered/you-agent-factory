// Package chatsessions is the public, transport-neutral Chat Sessions
// contract root described by
// docs/internal/projects/acp-client/final-proposal.md section 4.
//
// This packet (V0) defines pure public values, exhaustive enum and transition
// validation, and structured typed errors for request identity, unversioned
// Chat targets, session and target-episode lifecycle, turn admission and
// version-checked mutation, independent attachments, and race-safe control
// intents. It introduces no runtime implementation, no persistence, no
// Worker Sessions behavior, and no second normalized event vocabulary:
// pkg/services/workers/response_drafts.go remains the sole owner of the
// Worker event Kind/Phase taxonomy.
package chatsessions
