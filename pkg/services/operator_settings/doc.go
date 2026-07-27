// Package operatorsettings is the public Operator Settings service boundary.
//
// Peer-facing root contract (source of truth for published slices):
//   - Service — singular cross-service seam (document operations and
//     effective-resolution slices publish additively on this interface)
//   - Document, DocumentDefaults, and related values — Operator-Settings-owned
//     document vocabulary
//   - detached request, result, value, and typed-error contracts
//
// Construction/process-edge ports (FileSystem, ConfigDecoder, ConfigEncoder, and
// similar) exist so Wire and owner constructors can assemble production behavior.
// They are not the peer-facing source of truth for document operations or
// effective resolution: cross-service callers invoke Service methods without
// supplying filesystem, temporary-file, encoder/decoder, CLI, UI, Wire, or
// Initializer types.
//
// Existing implementation helpers remain in place for later IMP-SET-* absorption.
// Nested document/resolution package motion, Wire/root/initializer cutover, CLI
// manifest regeneration, and OpenAPI package-motion edits remain out of scope for
// the root-contract packet.
package operatorsettings
