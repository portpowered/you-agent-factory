// Package factorydefinitions publishes Factory Definitions peer contracts at
// this service root. CLN-DEF-CONTRACTS story 001 keeps Definition-owned
// vocabulary here so peers import pkg/services/factory_definitions instead of
// the contracts mega-barrel.
//
// Owned peer surfaces:
//   - service_contract.go: singular Service plus catalog, authoring, compile,
//     validate, snapshot, and distribute request/result/value types.
//   - validation_contract.go, persistence_contract.go, packages_contract.go,
//     scaffold_contract.go, snapshot_capture.go, loader.go, and the owned
//     aliases at the top of contracts_root.go.
//
// Foreign event envelope vocabulary is owned by pkg/services/recordings
// (event_contract.go). Deletion-only aliases remain in
// event_recording_deletion_aliases.go until peers finish cutover.
//
// Foreign world-state, dispatch, replay, and worker vocabulary below
// the foreign-vocabulary marker in contracts_root.go are temporary
// deletion-only aliases until CLN-DEF stories 004-005 rehome them.
package factorydefinitions
