// Package factorysessionsshim is the Chat Sessions-owned adapter over the
// public factory_sessions.Service root. It defines and implements its own
// FactoryTargetService contract, exposing only the Factory-target start,
// invoke, cancel, and close capabilities Chat Sessions needs, and delegates
// each call exactly once, unmodified, to the injected root. This contract is
// private to this package: it is not exported from chat_sessions, does not
// extend chatsessions.Service, and is not part of the public L1 V0 chat
// sessions boundary. It is registered under "Shims registered for deletion"
// in docs/internal/projects/root-consolidation/proposal.md, retired at L3
// Factory Sessions sealing; it must not grow into a second session
// authority.
//
// This adapter's constructor (New) depends on FactoryTargetExecutionService,
// the narrow start/invoke/cancel/close subset of factorysessions.Service
// this shim actually forwards to -- not the full 30+ method root. Any
// concrete factorysessions.Service still satisfies it structurally, but a
// caller-owned execution authority with no reason to implement the rest of
// that large interface does not need to fake or stub it just to be handed to
// New: the injected value is never always the process-scoped CLI-daemon
// singleton (which stays permanently inert outside the CLI daemon's single
// fixed-project bootstrap). ACP L1 V1 ordinary prompt delegation
// (pkg/transports/acp, wired through pkg/wire's
// provideACPServerFactoryTargetService) instead supplies
// factory_sessions/wire.OnDemandFactoryTargetService, a distinct live-runtime
// activation per dynamically ACP-selected Factory target that implements
// exactly FactoryTargetExecutionService's four methods -- completely, with
// no panic-capable stand-in for any capability beyond them. This adapter
// itself needed no change to support that: it was always a stateless,
// exactly-once forwarder over whatever FactoryTargetExecutionService its
// constructor is given, never a fixed assumption about which one. The
// activation singleton owns the on-demand per-target runtime-opening logic
// this adapter's own package cannot (it would require importing
// factory_sessions' internal runtime-opening machinery, which this
// package's own internal-package boundary forbids); that division of
// ownership is why the activation lives in
// factory_sessions/internal/ondemandtarget rather than here, not evidence of
// a second, independent session authority -- pkg/wire composes exactly one
// FactoryTargetExecutionService-satisfying value into this one shim per
// process, the same as any other caller of New.
package factorysessionsshim
