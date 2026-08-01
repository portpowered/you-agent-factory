// Package providers is the public Providers service boundary.
//
// Peer-facing root contract (source of truth for published slices):
//   - Service — singular cross-service seam (catalog, availability/capabilities,
//     and one-attempt Execute slices publish additively on this interface)
//   - ID, Descriptor, SessionRef — Providers-owned identity vocabulary
//   - ResolveIdentity, ResolveSelection, ValidatePrerequisites — Providers-owned
//     alias, selection, and prerequisite authority over Service
//   - detached request, result, value, and typed-error contracts
//
// Construction/process-edge ports exist so Wire and owner constructors can
// assemble a production Service. The compatibility registration vocabulary is
// owned by providers/wire and is not a second peer-facing Providers contract.
// The root is the source of truth for catalog enumeration,
// availability/capability facts, and one normalized execution attempt:
// cross-service callers invoke Service methods without supplying Workers
// provider registry/conductor types, concrete adapters, or transport/UI
// concerns.
//
// Transitional pkg/services/providers/internal/services/execution/internal/provider/** implementations remain in place
// for later IMP-PROV-* absorption. Nested catalog/execution implementation
// moves, Wire/root/initializer wiring, CLI-manifest, and OpenAPI package-motion
// edits remain out of scope for the root-contract packet.
package providers
