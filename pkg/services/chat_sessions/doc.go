// Package chatsessions publishes the L1 V0 Chat Sessions domain boundary:
// detached public values for chat targets, sessions, target episodes, turns,
// attachments, and control intents, plus their enum vocabulary, deterministic
// value validation, and the singular Service root contract that operates on
// them (session create/read, target selection, turn admission/advancement,
// connection attachment, and control-intent request/advancement).
//
// Service and its supporting values (contracts.go, types.go, errors.go,
// transitions.go) are the entire public surface of this package: no
// implementation, persistence, dependency-injection wiring, transport,
// OpenAPI, CLI, ACP wire operation, Factory Sessions sealing, Worker Sessions
// behavior, or alternate peer-facing service contract, and they import only
// the standard library.
//
// internal/factorysessionsshim is a private, unexported implementation
// detail owned by this package but outside its public contract: a
// stopgap adapter over the published factory_sessions.Service root that some
// later L1 Chat Sessions flow will need ahead of L3 Factory Sessions
// sealing. It defines and implements its own internal FactoryTargetService
// contract, does not import or extend chatsessions.Service, and is not part
// of the L1 V0 contract slice above. It is registered under "Shims
// registered for deletion" in
// docs/internal/projects/root-consolidation/proposal.md, retired at L3
// Factory Sessions sealing.
package chatsessions
