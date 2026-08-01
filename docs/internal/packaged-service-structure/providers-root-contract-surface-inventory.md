# Providers root `.go` contract-surface inventory (`pkg/services/providers`)

This owner-local inventory freezes the Providers root after the contract seal.
The root publishes the singular `providers.Service`, Providers-owned identity,
catalog, selection, lifecycle, ACP configuration, execution values, and typed
errors. Process-edge compatibility registrations live under the canonical
`providers/wire` construction boundary rather than in a public root sibling.

## Root-level `.go` file inventory

All current root-level files are classified as thin committed root contract
sources for this packet:

```text
acp_configuration_contract.go
acp_contract.go
catalog_characterization_test.go
catalog_contract.go
doc.go
execute_characterization_test.go
execute_contract.go
identity_characterization_test.go
identity_contract.go
lifecycle_contract.go
root_catalog_delegation_test.go
root_contract_characterization_test.go
selection_characterization_test.go
selection_contract.go
service_contract.go
```

**Total:** 15 root-level `.go` files — 15 thin committed root-contract files,
0 excess fold clusters in this packet.

The closed inventory is mirrored by:

- `internal/ownershipinventory/providers_root_contract.go`;
- `cmd/packagetargetmanifestcheck/providers_root_contract.go`; and
- `pkg/services/providers/root_contract_characterization_test.go` for the
  immediate child directory boundary and root inventory seal.

`pkg/services/providers/root_catalog_delegation_test.go` proves that
wire-constructed external registrations are visible and executable only
through the published `providers.Service`, and that invalid registrations
remain deterministic construction failures.

## Deferred Providers implementation work

The legacy execution implementation tree under
`providers/internal/services/execution/internal/provider/**` remains the
separately mapped Providers execution-flattening work. This packet does not
move those adapters or change CLI/OpenAPI transport ownership.
