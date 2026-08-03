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
// FactoryTargetCatalogService (factory_target_catalog_contract.go,
// factory_target_catalog_errors.go) is a separate L1 V1 detached root
// contract published by this same package: it resolves the allowed,
// installed FACTORY target catalog for an ACP client picker by combining the
// effective Operator Settings ACP Agent profile with Factory Definitions'
// public installed-catalog behavior. Its public files publish only the
// interface and its detached request/result/error values and import only the
// standard library, matching Service's contract-only convention. Its
// implementation lives in internal/service, composed through wire/, and is
// the first implementation this package publishes; the L1 V0 Service root
// above remains contract-only until its own L1 V1 implementation lands.
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
