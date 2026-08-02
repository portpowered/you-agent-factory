// Package chatsessions publishes the L1 V0 Chat Sessions domain boundary:
// detached public values for chat targets, sessions, target episodes, turns,
// attachments, and control intents, plus their enum vocabulary, deterministic
// value validation, and the singular Service root contract that operates on
// them (session create/read, target selection, turn admission/advancement,
// connection attachment, and control-intent request/advancement).
//
// Service and its supporting values (contracts.go, types.go, errors.go,
// transitions.go) are an enabling contract slice: no implementation,
// persistence, dependency-injection wiring, transport, OpenAPI, CLI, ACP wire
// operation, Factory Sessions sealing, or Worker Sessions behavior, and they
// import only the standard library.
//
// FactoryTargetService (factory_target_contract.go) is a separate, narrower
// root: the Factory-target execution dependency some L1 Chat Sessions flows
// need ahead of L3 Factory Sessions sealing (start, invoke, cancel, and close
// one Factory Session, in the published factory_sessions root vocabulary).
// internal/factorysessionsshim is its sole implementation, adapting the
// public factory_sessions.Service root without importing its internals or
// becoming a second session authority. It is registered under "Shims
// registered for deletion" in
// docs/internal/projects/root-consolidation/proposal.md, retired at L3
// Factory Sessions sealing. It is not part of the L1 V0 contract slice above.
package chatsessions
