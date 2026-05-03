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
| Contract drift enforcement: generated and runtime API surfaces stay in sync. | `complete` | `pkg/api/openapi_contract_test.go` keeps `/factory` and `/factory/~current` on the canonical `Factory` schema, and `tests/functional/runtime_api/api_named_factory_test.go` now proves `POST /factory` and `GET /factory/~current` round-trip bundled portable files through that same public payload while the on-disk named-factory layout stays persistence-safe. | No remaining backend contract-drift gap in this lane after `US-002`; `US-004` can reuse these proofs in the final verification inventory. |
| Backend test-layer coverage: behavior is reviewable through runtime outcomes rather than source inspection. | `complete` | `tests/functional/bootstrap_portability/api_export_import_e2e_smoke_test.go` covers full export/import portability, `tests/functional/runtime_api/api_named_factory_test.go` now covers the live `/factory` contract roundtrip with bundled files and persisted thin `factory.json`, and `pkg/service/factory_test.go` keeps service-seam activation and disk rehydration behavior focused at the package layer. | `US-004` should reference this backend bundle directly when it finalizes the cross-surface verification inventory. |

## Frontend Checklist

| Standard area | Status | Current evidence | Follow-up or remaining risk |
| --- | --- | --- | --- |
| Shared UI direction: import/export flows compose repository shared dialog and button primitives instead of bespoke overlays. | `complete` | `ui/src/features/export/export-factory-dialog.tsx` and `ui/src/features/import/dashboard-import-preview-dialog.tsx` both compose `ui/src/components/ui` dialog and button primitives, and `ui/src/features/workflow-activity/mutation-dialog.test.tsx` proves the shared mutation-dialog wrapper keeps distinct accessible title and description wiring across the export and import dialog surfaces. | No remaining shared-control verification gap in this lane after `US-004`. |
| Typed network and mutation ownership: import/export flows route async work through typed API and focused hooks instead of inline network calls. | `complete` | `ui/src/features/import/use-factory-import-activation.tsx` is covered through `ui/src/features/import/use-factory-import-activation.test.tsx`, and the data-model doc records the canonical UI import activation seam. Export continues to treat the authored `GET /factory/~current` response as the source payload. | `US-003` should revisit this only if export orchestration still owns too much contract shaping in UI feature code. |
| Loading, error, success, and no-data states are explicit for import/export UI. | `complete` | `ui/src/features/export/export-factory-dialog.tsx` now keeps preparation, validation, error, and post-download success feedback visible inside the shared export dialog instead of treating successful export as implicit dialog closure only. Import still exposes drag-active, reading, preview-ready, activation-error, and dismissible failure states through `ui/src/features/import/dashboard-import-preview-dialog.tsx` and `ui/src/features/workflow-activity/current-activity-import-controller.ts`, while the graph card keeps its explicit no-topology empty state in `ui/src/features/workflow-activity/react-flow-current-activity-card.tsx`. | Browser verification for keyboard focus and narrow/wide layouts still remains in `US-003`, but there is no remaining product-state gap for loading, error, success, or no-data coverage in this lane. |
| Accessibility and keyboard behavior: touched controls use semantic dialog and button primitives and wire validation state to fields. | `complete` | Export wires validation text through `aria-invalid` and `aria-describedby` in `ui/src/features/export/export-factory-dialog.tsx`, and browser-backed Storybook plays in `ui/src/features/export/export-factory-dialog.stories.tsx` prove keyboard tab reachability across the dialog fields and actions. Import uses shared dialog primitives and close suppression during activation in `ui/src/features/import/dashboard-import-preview-dialog.tsx`, with browser-backed keyboard reachability proof in `ui/src/features/import/dashboard-import-preview-dialog.stories.tsx`. | No remaining keyboard or focus gap in this lane after `US-003`; `US-004` can reuse these browser-visible checks in the final verification inventory. |
| Frontend test-layer coverage: standards-sensitive states are protected by rendered behavior. | `complete` | `ui/src/features/export/export-factory-dialog.test.tsx` now proves visible validation, loading, failure, and success states inside the shared export dialog; `ui/src/features/import/dashboard-import-preview-dialog.test.tsx` proves visible import preview, cancel dismissal, activation failure, submit-time close blocking, and activation-success dismissal behavior; `ui/src/features/workflow-activity/mutation-dialog.test.tsx` proves the shared dialog wrapper semantics used by both surfaces; and the focused Storybook browser plays in `ui/src/features/export/export-factory-dialog.stories.tsx` plus `ui/src/features/import/dashboard-import-preview-dialog.stories.tsx` keep keyboard-visible dialog behavior reviewable outside the full app shell. | No remaining frontend verification gap in this lane after `US-004`. |
| Responsive behavior for the touched surface. | `complete` | The export dialog and import preview both keep viewport-bounded dialog widths through `w-[min(92vw,...)]` classes in `ui/src/features/export/export-factory-dialog.tsx` and `ui/src/features/import/dashboard-import-preview-dialog.tsx`, and `ui/scripts/verify-import-export-storybook-responsive.mjs` now opens the built Storybook dialog stories at `390x844`, `768x1024`, and `1440x900` to assert visible controls, in-viewport dialog bounds, and no horizontal overflow. | No remaining viewport-evidence gap in this lane after `US-003`; the automated Storybook breakpoint check now owns this proof. |

## Verification Inventory

| Surface | Evidence now available | Remaining manual check |
| --- | --- | --- |
| Backend contract and persistence | `tests/functional/bootstrap_portability/api_export_import_e2e_smoke_test.go`, `tests/functional/runtime_api/api_named_factory_test.go`, `pkg/api/openapi_contract_test.go`, `pkg/service/factory_test.go` | None for this lane beyond optional reviewer reruns |
| UI metadata roundtrip and export dialog states | `ui/src/features/export/factory-png-export.test.ts`, `ui/src/features/export/export-factory-dialog.test.tsx`, `ui/src/features/import/factory-png-import.test.ts`, `ui/src/features/import/use-factory-import-activation.test.tsx`, `ui/src/features/export/export-factory-dialog.stories.tsx`, `ui/scripts/verify-import-export-storybook-responsive.mjs` | None for this lane beyond optional reviewer reruns |
| UI drag/drop lifecycle and import preview states | `ui/src/features/import/use-factory-png-drop.test.tsx`, `ui/src/features/import/dashboard-import-preview-dialog.test.tsx`, `ui/src/features/import/dashboard-import-preview-dialog.stories.tsx`, `ui/scripts/verify-import-export-storybook-responsive.mjs` | None for this lane beyond optional reviewer reruns |
| Shared UI direction and local architecture intent | `ui/src/features/workflow-activity/mutation-dialog.test.tsx`, `ui/src/features/export/export-factory-dialog.test.tsx`, `ui/src/features/import/dashboard-import-preview-dialog.test.tsx` | None for this lane beyond optional reviewer reruns |

## Next Story Targets

- `US-002`: close any remaining backend standards gaps that still depend on
  manual reasoning and update the backend rows above with final evidence.
- `US-003`: complete.
- `US-004`: complete.
