// Package providers is the public Providers service boundary.
//
// Peer-facing root contract (source of truth for published slices):
//   - Service — singular cross-service seam (catalog, availability/capabilities,
//     one-attempt Execute, and attempt-control slices publish additively on
//     this interface)
//   - ID, Descriptor, SessionRef — Providers-owned identity vocabulary
//   - Service.ResolveIdentity, Service.ResolveSelection,
//     Service.ValidatePrerequisites — Providers-owned alias, selection, and
//     prerequisite authority
//   - Service.ControlAttempt — Providers-owned pause/cancel/terminate action
//     vocabulary with a closed completed/unsupported outcome. Cancel and
//     Terminate reach the exact live native (non-ACP) attempt named by
//     canonical provider identity plus attempt ID, when one is in flight,
//     by cancelling the context every native adapter (codex, claude, agy)
//     already force-terminates its subprocess/PTY session on. Cancel also
//     reaches the exact live ACP attempt it names once that attempt has an
//     established session/prompt turn in flight, by delivering the ACP
//     protocol's session/cancel notification through its owning session;
//     Pause, ACP Terminate, ACP attempts before or after that turn is in
//     flight, and unknown/already-terminal/mismatched attempts answer with
//     the canonical unsupported outcome
//   - Service.Continue — Providers-owned provider-session continuation
//     contract. ContinueRequest.Reference names the exact provider,
//     provider-specific session kind, and opaque Provider Session identity
//     to resume; a malformed or foreign reference (Attempt names a
//     different provider than Reference) fails with a typed
//     ContinuationFailure before any provider adapter runs, a reference the
//     resolved provider cannot continue returns the closed unsupported
//     outcome as a successful result, and a valid reference reaches the
//     matching adapter with provider, kind, and session identity unchanged.
//     Ordinary Execute rejects any request that already carries a resume
//     reference - continuation is requested exclusively through Continue
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
