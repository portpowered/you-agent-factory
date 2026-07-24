# Invert Factory Sessions execution type ownership without import cycles

## Status

Resolved by `pss-ctr-ses-root-contract-008` (2026-07-24): durable execution and
effect-port type bodies now live on the `factory_sessions` root;
`internal/execution` and `internal/contracts` alias from the root.

## Problem (historical)

Closing `type X = internal/execution.X` and `type Y = internal/contracts.Y`
re-exports from the Factory Sessions root failed when contracts aliased from the
root while the root still imported execution:

`factory_sessions` → `internal/execution` → `internal/contracts` → `factory_sessions`

## Resolution

1. Move durable execution request/result/error type bodies onto the
   `factory_sessions` root (`execution_owned_contract.go`).
2. Change `internal/execution` to alias root types (`root_contract_aliases.go`)
   and keep thin wrappers for helpers that remain useful inside execution.
3. Invert `internal/contracts` to alias effect-port types from the root.
4. Keep listing helpers that need execution-local filters (`ApplySessionListScope`)
   inside execution; publish pure lifecycle/listing helpers used by peers on the root.
