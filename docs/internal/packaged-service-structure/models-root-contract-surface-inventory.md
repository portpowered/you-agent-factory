# Models root `.go` contract-surface inventory (`pkg/services/models`)

Owner-local live inventory for the Models root-contract seal and production
convergence. The Models root exposes one peer-facing `Service` contract;
construction ports and implementation details live below `models/internal` or
at the `models/wire` composition boundary.

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
host_scope_characterization_test.go
local_execution_contract.go
managed_runtime_contract.go
root_authority_seal_characterization_test.go
root_slice_characterization_test.go
runtime_config_contract.go
runtime_construction_contract.go
service_contract.go
```

**Totals:** 13 root-level `.go` files — 13 thin committed root contract
files, 0 excess fold clusters in this packet.

The inventory is mirrored by:

- `internal/ownershipinventory/models_root_contract.go`;
- `cmd/packagetargetmanifestcheck/models_root_contract.go`; and
- `internal/ownershipinventory/models_root_contract_test.go` for the immediate
  child directory boundary.

The root wire boundary is exercised by
`pkg/services/models/wire/root_wire_behavioral_boundary_test.go`, while the broader
asset, catalog, host, lease, and inference parity suite remains in the existing
Models root and Wire tests.

## Production implementation boundary

The implementation convergence covered by this inventory is complete for the
Models root surface:

- catalog and inference contracts are owned by
  `models/internal/services/catalog` and `models/internal/services/inference`;
- runtime host and its lease service are owned by
  `models/internal/services/runtime_host`;
- streamed invocation artifacts are owned by
  `models/internal/services/inference/internal/artifacts`;
- construction and external-effect ports are private in
  `models/internal/effects`, with narrow aliases only in `models/wire`; and
- remaining compatibility helpers are explicitly private under
  `models/internal/legacyhost` and `models/internal/service`.

No peer imports those compatibility packages. Models CLI, HTTP, MCP, Workers,
Factory Sessions, Edges, and process wiring consume the root contract and its
explicit runtime-scope operations.
