# Workers root `.go` contract-surface inventory (`pkg/services/workers`)

Owner-local live inventory for **INV-WRK-TOPLEVEL** (`pss-inv-wrk-toplevel`). This
packet records evidence-backed classification only; it does **not** move, fold,
or delete any root-level `.go` files.

**Inventory captured:** 2026-07-28 UTC from the live tree at
`pkg/services/workers/*.go` (non-recursive root files only).

Companion directory inventory:
[`workers-top-level-inventory.md`](workers-top-level-inventory.md).

## Classification legend

| Classification | Meaning |
| --- | --- |
| **Thin committed root contract (keep)** | Intentional `package workers` surface that cross-service peers may import directly (interfaces, request/result vocabulary, safe diagnostics types, publication-boundary shapes). Expected to remain at the service root after debt cleanup, possibly slimmed. |
| **Private-implementation leakage** | Root-level implementation or integration logic that belongs under `workers/internal` or `workers/internal/services/{runners,runtime_assembly,workstations}` once later CLN/DEL packets fold the matching top-level debt directories. |
| **Delete-after-fold dead surface** | Root files that become redundant once transitional top-level debt (`interface/`, `executor/`, mock-worker schema paths, and related folds) is absorbed or replaced by generated/OpenAPI-owned contracts. |

## Root-level `.go` file inventory

| File | Classification | Later target / rationale |
| --- | --- | --- |
| `command.go` | Thin committed root contract (keep) | Subprocess execution port (`CommandRunner`, `CommandRequest`, `CommandResult`). |
| `env_diagnostics.go` | Private-implementation leakage | `workstations` — command-environment classification and safe diagnostic projection helpers. |
| `env_diagnostics_test.go` | Private-implementation leakage | Co-located tests for `env_diagnostics.go`; move with diagnostics projection logic. |
| `execution_context.go` | Thin committed root contract (keep) | Worker execution environment (`Context`, project/session defaults). |
| `execution_contracts.go` | Thin committed root contract (keep) | Canonical inference/script/model/agent event and request vocabulary at the Workers root boundary. |
| `execution_requests.go` | Thin committed root contract (keep) | Runner capability and workstation execution request vocabulary published for selection and dispatch planning. |
| `execution_tokens.go` | Thin committed root contract (keep) | Worker-facing dispatch token/color view shared across execution paths. |
| `executor_test_helpers_test.go` | Delete-after-fold dead surface | Root-level executor integration helpers; delete once `executor/` folds under `workstations`. |
| `failure.go` | Thin committed root contract (keep) | Normalized provider failure type (`ProviderError`) at the public boundary. |
| `inference_failure.go` | Private-implementation leakage | `workstations` — inference failure classification and customer-safe messaging logic. |
| `inference_failure_test.go` | Private-implementation leakage | Co-located tests for `inference_failure.go`. |
| `interfaces.go` | Thin committed root contract (keep) | Primary Workers root contracts (`Service`, hosted poller ports, provider identity, docs loader, and related peer-facing interfaces). |
| `invocation_executor_test.go` | Private-implementation leakage | `workstations` — integration tests that reach `invocation/` and `executor/` through the root package; relocate with workstation fold. |
| `mock_workers.go` | Delete-after-fold dead surface | Legacy JSON mock-worker config loader; file documents TODO to replace with OpenAPI-generated schema owned by `interface/` fold. |
| `mock_workers_config_test.go` | Delete-after-fold dead surface | Co-located tests for `mock_workers.go`; remove with mock-worker schema fold. |
| `model_invocation.go` | Private-implementation leakage | `workstations` — workstation pool lifecycle errors, requests/results, and `ModelInvoker` wiring belong with workstation execution, not a thin root contract. |
| `opencode_agent_contract_test.go` | Private-implementation leakage | `runners` — runner-selection contract tests for OpenCode agent policy; move with `runner_policy.go`. |
| `progress_observations.go` | Thin committed root contract (keep) | Provider-neutral progress fragments accepted by Factory Session response streams. |
| `prompt_templates.go` | Private-implementation leakage | `workstations` — prompting template contract/diagnostic types; fold with `prompting/` debt. |
| `provider_port.go` | Thin committed root contract (keep) | `Provider` inference port explicitly documented for cross-service consumers (for example Recordings replay). |
| `provider_port_test.go` | Thin committed root contract (keep) | Root-contract characterization test for `provider_port.go`. |
| `response_drafts.go` | Thin committed root contract (keep) | Provider response draft shapes and publication-boundary helpers. |
| `response_draft_validation.go` | Private-implementation leakage | `workstations` — draft validation logic ahead of publication; fold with inference/publication path. |
| `runner_policy.go` | Private-implementation leakage | `runners` — built-in runner metadata and `ResolveRunnerSelection` policy implementation. |
| `runner_registry.go` | Private-implementation leakage | `runners` — built-in runner prerequisite validation and availability reporting. |
| `runner_registry_test.go` | Private-implementation leakage | Co-located tests for `runner_registry.go`. |
| `runtime_service.go` | Thin committed root contract (keep) | `RuntimeService` opening contract for Factory Runtime assembly. |
| `safe_diagnostics.go` | Thin committed root contract (keep) | Canonical safe diagnostics types for history, replay, and projections. |
| `safe_diagnostics_codec.go` | Private-implementation leakage | `workers/internal` (diagnostics helper) — projection/codec logic between worker and safe diagnostics shapes. |
| `template_fields.go` | Thin committed root contract (keep) | `TemplateFieldResolver` contract without exposing prompting implementation packages. |
| `template_fields_test.go` | Private-implementation leakage | `workstations` — integration tests that assemble prompting/executor paths; relocate with workstation fold. |
| `token_lineage.go` | Private-implementation leakage | `workstations` — chaining-trace derivation helpers over execution tokens. |
| `workstation_pool_boundary.go` | Private-implementation leakage | `workstations` — pool boundary implementation; retain only slim runtime-facing interfaces at root during later fold. |
| `workstation_pool_boundary_test.go` | Private-implementation leakage | Co-located tests for `workstation_pool_boundary.go`. |

**Totals:** 34 root-level `.go` files — 14 thin committed root contract (keep), 17
private-implementation leakage, 3 delete-after-fold dead surface.

## Out of scope for this note

- Nested packages under immediate child directories (classified in
  [`workers-top-level-inventory.md`](workers-top-level-inventory.md)).
- `packagetargetmanifestcheck` / `ownershipinventory` remap rows and JSON baseline
  regeneration (stories 003–005).
- Production package moves, folds, deletes, or `pkg/wire` edits.
