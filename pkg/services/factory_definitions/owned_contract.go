// Package factorydefinitions publishes Factory Definitions peer contracts at
// this service root. CLN-DEF-CONTRACTS story 001 keeps Definition-owned
// vocabulary here so peers import pkg/services/factory_definitions instead of
// the contracts mega-barrel.
//
// Owned peer surfaces:
//   - service_contract.go: singular Service plus catalog, authoring, compile,
//     validate, snapshot, and distribute request/result/value types.
//   - validation_contract.go, persistence_contract.go, packages_contract.go,
//     scaffold_contract.go, snapshot_capture.go, loader.go, current_factory_pointer.go,
//     and the owned aliases at the top of contracts_root.go.
//
// Foreign event envelope vocabulary is owned by pkg/services/recordings
// (canonical_ledger/event_contract.go with peer aliases in contracts.go). World-state,
// dispatch, and workstation-request vocabulary is owned by projection_query with peer
// aliases in recordings/contracts.go. Replay vocabulary is owned by
// recordings/internal/services/replay (replay_contract.go with peer aliases in
// contracts.go). Worker execution vocabulary is
// owned by pkg/services/workers (worker_vocabulary_contract.go,
// execution_contracts.go); provider-session identity is published at
// pkg/services/providers (SessionRef) and pkg/services/workers
// (ProviderSessionMetadata). Deletion-only aliases remain in event_recording_deletion_aliases.go,
// world_state_recording_deletion_aliases.go, dispatch_runtime_deletion_aliases.go,
// replay_recording_deletion_aliases.go, worker_provider_deletion_aliases.go, and
// parallel_operation_deletion_aliases.go until peers finish cutover.
//
// The public contracts mega-barrel was deleted in CLN-DEF story 007; implementation
// types live in pkg/services/factory_definitions/internal/contracts and shared
// namevalue helpers in pkg/services/factory_definitions/namevalue.
package factorydefinitions
