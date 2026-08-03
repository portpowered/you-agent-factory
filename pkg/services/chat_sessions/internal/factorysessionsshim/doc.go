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
// This adapter wraps the process-scoped factorysessions.Service singleton,
// which stays permanently inert outside the CLI daemon's single
// fixed-project bootstrap, and its StartFactoryTarget exposes only the
// asynchronous AsyncStartResult status vocabulary. ACP L1 V1 ordinary prompt
// delegation (pkg/transports/acp, wired through pkg/wire's
// provideACPServerFactoryTarget) activates a distinct live runtime per
// dynamically ACP-selected Factory target and needs the synchronous,
// truthful InvocationResult its first turn actually observes; neither fits
// this package's fixed-runtime, status-only shape, and extending this
// package to cover both would grow it into exactly the second session
// authority its own package comment above forbids. That flow therefore
// depends on the dedicated factory_sessions/wire.OnDemandFactoryTargetService
// instead of this adapter.
package factorysessionsshim
