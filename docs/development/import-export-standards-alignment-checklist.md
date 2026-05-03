---
author: Codex
last modified: 2026, may, 3
doc-id: AGF-DEV-007
status: active
---

# Import Export Standards Alignment Checklist

This checklist records the current standards-alignment state for the active
import/export lane. It maps the shipped import/export surface to
`docs/standards/code/general-backend-standards.md` and
`docs/standards/code/general-website-standards.md` so reviewers can see what is
already implemented, what is protected by tests, and what remains for follow-on
 stories.

## Scope

- Backend named-factory export and import seams owned by `GET /factory/~current`
  and `POST /factory`
- Portable imported-factory persistence and runtime reload behavior
- Dashboard PNG export dialog, graph-based PNG import flow, and their typed API
  hooks

Status meanings:

- `complete`: implemented and backed by direct evidence below
- `partial`: some standards work landed, but follow-on closure is still needed
- `follow-up`: intentionally deferred with a scoped next step and named risk

## Backend Checklist

| Standard area | Status | Current evidence | Follow-up or remaining risk |
| --- | --- | --- | --- |
| Architecture boundaries: keep public contract translation at the boundary and service ownership in one seam. | `complete` | `pkg/apisurface/contract.go` keeps the API-facing named-factory seam narrow, `pkg/service/factory.go` owns named-factory creation, activation, and current-factory reads, and `docs/development/named-factory-api-contract-data-model.md` records that ownership explicitly. | No immediate gap for `US-001`. `US-002` should only revisit this if new import/export behavior starts bypassing the named-factory seam. |
| Public contract ownership: import/export reuses one canonical public payload instead of export-only DTOs. | `complete` | `docs/development/development.md` names the canonical sharing payload as the generated `NamedFactory` contract, and `docs/development/named-factory-api-contract-data-model.md` documents `GET /factory/~current`, `POST /factory`, and the PNG metadata wrapper as one shared contract family. | `US-002` should keep authored OpenAPI, generated artifacts, and runtime handlers aligned if the payload changes again. |
| Runtime and persistence behavior: imported factories stay thin on disk while runtime bodies remain reloadable from split `AGENTS.md` files. | `complete` | `tests/functional/bootstrap_portability/api_export_import_e2e_smoke_test.go` proves reimported factories accept work, keep `factory.json` free of inline worker and workstation bodies, and reload the runtime bodies from split files. | No immediate gap for `US-001`. |
| Contract drift enforcement: generated and runtime API surfaces stay in sync. | `partial` | `pkg/api/openapi_contract_test.go` is the contract-guard surface called out by the development guide, and the named-factory data-model doc points reviewers back to generated contract evidence instead of helper topology. | `US-002` should add or tighten import/export-specific backend regression coverage if a remaining standards gap still depends on doc reasoning instead of observable backend behavior. |
| Backend test-layer coverage: behavior is reviewable through runtime outcomes rather than source inspection. | `partial` | Functional coverage already exists in `tests/functional/bootstrap_portability/api_export_import_e2e_smoke_test.go`, with supporting contract references in `docs/development/development.md` and `docs/development/named-factory-api-contract-data-model.md`. | `US-004` should consolidate the final backend verification set referenced by this checklist once the remaining gap-closure work is complete. |

## Frontend Checklist

| Standard area | Status | Current evidence | Follow-up or remaining risk |
| --- | --- | --- | --- |
| Shared UI direction: import/export flows compose repository shared dialog and button primitives instead of bespoke overlays. | `complete` | `ui/src/features/export/export-factory-dialog.tsx` and `ui/src/features/workflow-activity/react-flow-current-activity-card-import.tsx` both compose `ui/src/components/ui` dialog and button primitives and reuse dashboard typography tokens. `docs/development/export-import-dashboard-reuse-audit.md` records the reuse decision that drove this direction. | No immediate gap for `US-001`. |
| Typed network and mutation ownership: import/export flows route async work through typed API and focused hooks instead of inline network calls. | `complete` | `ui/src/features/import/use-factory-import-activation.tsx` is covered through `ui/src/features/import/use-factory-import-activation.test.tsx`, and the data-model doc records the canonical UI import activation seam. Export continues to treat the authored `GET /factory/~current` response as the source payload. | `US-003` should revisit this only if export orchestration still owns too much contract shaping in UI feature code. |
| Loading, error, success, and no-data states are explicit for import/export UI. | `partial` | Export shows preparation and validation feedback in `ui/src/features/export/export-factory-dialog.tsx`. Import exposes drag-active, reading, preview-ready, activation-error, and dismissible failure states through `ui/src/features/workflow-activity/react-flow-current-activity-card-import.tsx` and `ui/src/features/workflow-activity/current-activity-import-controller.ts`. The graph card also has an explicit no-topology empty state in `ui/src/features/workflow-activity/react-flow-current-activity-card.tsx`. | `US-003` should confirm whether export still needs an explicit success acknowledgment and whether any import/export state remains visible only through implicit dialog closure. |
| Accessibility and keyboard behavior: touched controls use semantic dialog and button primitives and wire validation state to fields. | `partial` | Export wires validation text through `aria-invalid` and `aria-describedby` in `ui/src/features/export/export-factory-dialog.tsx`. Import uses shared dialog primitives and close suppression during activation in `ui/src/features/workflow-activity/react-flow-current-activity-card-import.tsx`. | `US-003` should add final browser verification for keyboard and focus behavior across the touched flows. |
| Frontend test-layer coverage: standards-sensitive states are protected by rendered behavior. | `partial` | `ui/src/features/import/use-factory-png-drop.test.tsx`, `ui/src/features/import/factory-png-import.test.ts`, `ui/src/features/export/factory-png-export.test.ts`, and `ui/src/features/import/use-factory-import-activation.test.tsx` cover import/export metadata, drop supersession, and activation behavior through observable outcomes. | `US-004` should add the final rendered or browser-visible coverage bundle for loading, failure, success, and shared-control behavior. |
| Responsive behavior for the touched surface. | `follow-up` | The import preview and export dialog use viewport-bounded layout classes, but this checklist does not yet have browser evidence for supported narrow and wide dashboard widths. | `US-003` should verify the import/export dashboard surface in browser and record the exact evidence path. Risk: layout regressions on narrow dashboards may still depend on manual reviewer inspection. |

## Verification Inventory

| Surface | Evidence now available | Remaining manual check |
| --- | --- | --- |
| Backend contract and persistence | `tests/functional/bootstrap_portability/api_export_import_e2e_smoke_test.go`, `pkg/api/openapi_contract_test.go` | Final `US-002` and `US-004` regression bundle selection |
| UI metadata roundtrip | `ui/src/features/export/factory-png-export.test.ts`, `ui/src/features/import/factory-png-import.test.ts`, `ui/src/features/import/use-factory-import-activation.test.tsx` | Browser confirmation after frontend gap closure |
| UI drag/drop lifecycle | `ui/src/features/import/use-factory-png-drop.test.tsx` | Browser confirmation of focus, dialog, and responsive behavior |
| Shared UI direction and local architecture intent | `docs/development/export-import-dashboard-reuse-audit.md`, `docs/development/named-factory-api-contract-data-model.md` | Replace any remaining doc-only reasoning with direct behavioral checks in `US-004` where needed |

## Next Story Targets

- `US-002`: close any remaining backend standards gaps that still depend on
  manual reasoning and update the backend rows above with final evidence.
- `US-003`: close the remaining frontend gaps around explicit visible success,
  keyboard and focus confirmation, and responsive verification.
- `US-004`: finish the behavioral verification bundle and update this checklist
  so completed rows point at the final runtime or rendered tests.
