# Models root `.go` contract-surface inventory (`pkg/services/models`)

Owner-local live inventory for the Models root-contract seal. This note freezes
the current root file boundary; it does not perform the separately mapped
legacy implementation moves under `models/internal`.

The Models root is the one cross-service authority for runtime scopes, catalog
discovery, asset preparation, host readiness and leases, and local inference.
Root characterization tests use detached request/result vocabulary and the
singular `models.Service` seam. Implementations remain private below
`models/internal` and construction remains behind `models/wire`.

## Root-level `.go` file inventory

All current root-level files are classified as **thin committed root contract
(keep)** for this packet:

```text
asset_scope_characterization_test.go
assets_contract.go
catalog_contract.go
catalog_scope_characterization_test.go
host_contract.go
host_diagnostics_contract.go
host_scope_characterization_test.go
invocation_artifacts.go
local_execution_contract.go
managed_runtime_contract.go
models_root_contract_seal_test.go
packaged_root_shape_test.go
root_authority_seal_characterization_test.go
root_slice_characterization_test.go
root_wire_behavioral_boundary_test.go
runtime_config_contract.go
runtime_construction_contract.go
service_contract.go
```

**Totals:** 18 root-level `.go` files — 18 thin committed root contract
files, 0 excess fold clusters in this packet.

The inventory is mirrored by:

- `internal/ownershipinventory/models_root_contract.go`;
- `cmd/packagetargetmanifestcheck/models_root_contract.go`; and
- `pkg/services/models/packaged_root_shape_test.go` for the immediate child
  directory boundary.

The root wire boundary is exercised by
`pkg/services/models/root_wire_behavioral_boundary_test.go`, while the broader
asset, catalog, host, lease, and inference parity suite remains in the existing
Models root and Wire tests.

## Deferred implementation folds

The following paths remain the planned private successors recorded by the
packaged-service audit and are intentionally not changed by this root-surface
freeze:

- `models/internal/catalog` → `models/internal/services/catalog`;
- `models/internal/host` → `models/internal/services/runtime_host`; and
- `models/internal/inference` → `models/internal/services/inference`.

Those moves need their own import-retargeting and parity proof because the
transitional aggregate still supplies compatibility operations.
