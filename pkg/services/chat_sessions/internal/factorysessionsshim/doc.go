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
package factorysessionsshim
