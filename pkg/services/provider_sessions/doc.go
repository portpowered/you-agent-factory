// Package providersessions is the public Provider Sessions service boundary.
//
// Peer-facing root contract (source of truth for published slices):
//   - Service — singular cross-service seam
//   - InspectRequest/InspectResult and ProjectRequest/ProjectResult using the
//     canonical providers.SessionRef identity
//   - Detail and related normalized transcript/parse/usage value types
//   - typed errors (ErrUnsupportedProvider, ErrUnsupportedKind,
//     ErrInvalidIdentifier, ErrSessionNotFound, ErrAmbiguousSessionFile, LookupError)
//
// Construction/process-edge ports (FileSystem, home/OS resolution, Codex/Cursor
// walk/symlink/SQL helpers) exist so Wire and owner constructors can assemble a
// production Service. They are not the peer-facing source of truth for
// detached-ref validation/inspection or normalized transcript/detail
// projection: cross-service callers invoke Service methods without supplying
// those ports or private Codex/Cursor reader types.
//
// Nested IMP-PSES reader cuts, CTR-PROV/IMP-PROV, Standardized Providers
// conductor/migration, CLI-manifest, workers construction, and OpenAPI
// package-motion edits remain out of scope for the root-contract packet.
package providersessions
