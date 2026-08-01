# Work root `.go` contract-surface inventory (`pkg/services/work`)

Owner-local live inventory for **INV-WORK-TOPLEVEL** (`pss-inv-work-toplevel`).
This note records the current root file boundary; it does not move or delete
root-level `.go` files.

**Inventory captured:** 2026-07-31 from the live tree at
`pkg/services/work/*.go` (non-recursive root files only).

The root retains published Work vocabulary, the singular `Service` contract,
thin peer bindings, and characterization tests. Policy implementations and
private materialization/admission/state helpers live below `work/internal` or
the corresponding private subservice. Root adapters must use the published
contract rather than importing those implementation packages.

## Root-level `.go` file inventory

All files in this live inventory are classified as **thin committed root
contract (keep)**:

```text
admission_contract.go
content_contract.go
content_materialization_public_seam_test.go
content_materialize_contract.go
content_staging_contract.go
content_staging_public_seam_test.go
contracts.go
input.go
input_test.go
invocation_policy_service_test.go
invocation_return_policy_contract.go
invocation_return_policy_convert.go
lineage_contract.go
primary_result_regression_test.go
primary_result_test.go
proposal_materialization_contract.go
proposal_materialization_contract_test.go
read_contract.go
recordings_request_boundary_test.go
service_contract.go
service_peer_bindings.go
service_peer_bindings_test.go
service_root_contract_seal_test.go
service_root_contract_test.go
wire_behavioral_proof_test.go
```

**Totals:** 25 root-level `.go` files — 25 thin committed root contract
files, 0 excess fold clusters.

## Private implementation destinations

The current private implementation packages are under `work/internal`,
including `contenturl`, `invocationreturnpolicy`, `lineagegraph`,
`proposalmaterialization`, `requestadmission`, and `stateaccessquery`. The
state-access service implementation remains under
`work/internal/services/state_access`; its wire package is the only peer-facing
construction seam for that subservice.

## Generator mirror

Committed generator tables mirror this inventory:

- `internal/ownershipinventory/work_root_contract.go` —
  `WorkThinRootContractFiles`, `WorkExcessRootContractFolds`, and the root
  classification helpers.
- `cmd/packagetargetmanifestcheck/work_root_contract.go` — package-target
  manifest mirror for the same root partition.

## Out of scope for this note

- Nested packages under immediate child directories (classified in
  [`work-top-level-inventory.md`](work-top-level-inventory.md)).
- JSON baseline regeneration beyond the Work rows changed by the convergence
  cutover.
