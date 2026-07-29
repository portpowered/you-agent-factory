// Package providers is the public Providers service boundary.
//
// Peer-facing root contract (source of truth for published slices):
//   - Service — singular cross-service seam (catalog, availability/capabilities,
//     and one-attempt Execute slices publish additively on this interface)
//   - ID, Descriptor, SessionRef — Providers-owned identity vocabulary
//   - detached request, result, value, and typed-error contracts
//
// Construction/process-edge ports exist so Wire and owner constructors can
// assemble a production Service. They are not the peer-facing source of truth
// for catalog enumeration, availability/capability facts, or one normalized
// execution attempt: cross-service callers invoke Service methods without
// supplying Workers provider registry/conductor types, concrete adapters, or
// transport/UI concerns.
//
// Transitional pkg/services/providers/internal/services/execution/internal/provider/** implementations remain in place
// for later IMP-PROV-* absorption. Nested catalog/execution implementation
// moves, Wire/root/initializer wiring, CLI-manifest, and OpenAPI package-motion
// edits remain out of scope for the root-contract packet.
package providers
