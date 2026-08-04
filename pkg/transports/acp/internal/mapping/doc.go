// Package mapping is the ACP transport's outbound projection: pure
// functions from one sequenced source-native record (workers.Draft) to
// zero or one acpsdk.SessionUpdate. It owns no service dependency, no I/O,
// and no persistence coupling -- per
// docs/internal/projects/acp-client/final-proposal.md §6.1, this is where
// the record→acpsdk.SessionUpdate inversion transformation lives, kept
// separate from the inbound (third-party agent stream→workers.Draft)
// mapping the providers package owns. workers.Draft, the vocabulary owner
// (pkg/services/workers/response_drafts.go), stays the one input shape;
// this package introduces no parallel event taxonomy.
package mapping
