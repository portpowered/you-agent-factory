# PRD: Worker Runtime Configuration

## Introduction

The dashboard worker configuration editor currently exposes only core worker identity and provider fields. Existing worker runtime fields — `skipPermissions`, `timeout`, and `stopToken` — are preserved during saves but cannot be edited from the UI. Customers must leave the dashboard and hand-edit configuration files for common worker execution settings.

This project adds editable worker-owned runtime configuration controls to the existing worker configuration section. The feature uses the existing public worker contract fields only: `skipPermissions`, `timeout`, and `stopToken`. It does not edit workstation `stopWords`.

## Context

### Customer ask

Add editable worker-owned runtime configuration controls (`skipPermissions`, `timeout`, `stopToken`) to the existing dashboard worker configuration editor, persist values on the selected worker in the canonical factory definition, show field-specific validation feedback, and preserve existing dirty-state, discard, conflict, shared-worker impact, and save notification behavior — without changing API contracts or workstation `stopWords`.

### Problem

- Worker runtime settings exist in the public contract and are round-tripped through save, but the editable draft and UI do not expose them.
- Customers cannot configure permission bypass, execution timeout, or stop token without editing raw factory config.
- Invalid timeout values surface as generic contract errors rather than field-specific feedback.
- Without draft-level runtime fields, type changes and dirty detection cannot accurately reflect runtime edits.

### Solution

Extend the editable worker draft, apply path, dirty/equality checks, overwrite detection, validation mapping, and configuration UI to treat `skipPermissions`, `timeout`, and `stopToken` as first-class editable fields. Add a duration picker for timeout, model-worker-only permission bypass toggle, and worker-owned stop token input with clear copy distinguishing it from workstation `stopWords`. Save continues through the session-scoped canonical factory document flow; no backend execution or OpenAPI changes.

## Project-level acceptance criteria

- [ ] **PAC-1:** A factory operator can edit `skipPermissions` (model workers), `timeout` (all worker types), and `stopToken` from the worker editable configuration section and persist via session-scoped factory document save.
- [ ] **PAC-2:** Saved runtime values reload into the editor matching the canonical worker definition; empty timeout and whitespace-only stop token clear the corresponding optional worker fields.
- [ ] **PAC-3:** Saving worker runtime settings writes only to the selected worker in `factory.workers` and does not mutate workstation `stopWords`.
- [ ] **PAC-4:** Changing worker type between `MODEL_WORKER`, `SCRIPT_WORKER`, and `HOSTED_WORKER` preserves runtime settings in the draft and on save unless the operator edits them.
- [ ] **PAC-5:** Contract validation failures for `factory.workers[index].timeout`, `stopToken`, and `skipPermissions` map to the corresponding controls; save stays disabled while local validation errors are present.
- [ ] **PAC-6:** Existing dirty-state, discard, conflict overwrite detection, reset-to-latest, shared-worker warnings, and save notifications continue to work for runtime fields.
- [ ] **PAC-7 (quality gate):** UI typecheck, lint, and targeted tests for touched worker editable paths pass.

## Goals

- Allow customers to edit `skipPermissions`, `timeout`, and `stopToken` from the existing worker configuration editor.
- Persist edited values on the selected worker in the canonical factory definition.
- Keep workstation configuration unchanged when saving worker runtime settings.
- Show field-specific validation feedback when saved configuration is invalid.
- Preserve existing dirty-state, discard, conflict, shared-worker impact, and save notification behavior.

## User Stories

### US-001: Edit worker permission bypass
**Description:** As a factory operator, I want to toggle `skipPermissions` for a worker so that supported model providers can bypass provider permission prompts during execution.

**Acceptance Criteria:**
- [ ] Worker configuration initializes the `skipPermissions` control from the selected worker's current `skipPermissions` value.
- [ ] The control is visible for model workers only; it is not shown for script or hosted workers.
- [ ] The control remains available regardless of selected model provider.
- [ ] Saving writes `skipPermissions: true` when enabled on the selected worker.
- [ ] Saving omits or clears `skipPermissions` when disabled, matching existing optional boolean serialization behavior.
- [ ] Save, discard, dirty-state, overwrite detection, and reset-to-latest behavior include `skipPermissions`.
- [ ] Focused unit tests cover draft initialization, apply, and dirty detection for `skipPermissions`.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill: select a model worker, toggle permission bypass, observe dirty/save state, save, and confirm persisted value reloads.

### US-002: Edit worker timeout
**Description:** As a factory operator, I want to set a per-worker execution timeout so that any worker type can be bounded during a run.

**Acceptance Criteria:**
- [ ] Worker configuration initializes timeout from the selected worker's current `timeout` value.
- [ ] Timeout is editable for `MODEL_WORKER`, `SCRIPT_WORKER`, and `HOSTED_WORKER`.
- [ ] The UI uses a duration picker with a numeric value and unit dropdown (seconds, minutes, hours).
- [ ] The duration picker represents an empty timeout as "not configured" and clears the worker `timeout` field on save.
- [ ] Saving writes a Go duration string such as `30s`, `5m`, or `1h` to the selected worker's `timeout`.
- [ ] Save, discard, dirty-state, conflict overwrite detection, and reset-to-latest behavior include timeout.
- [ ] Focused unit tests cover duration parsing/formatting, draft apply, and dirty detection for timeout.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill: set timeout on each worker type, save, reload, and confirm displayed value matches saved duration.

### US-003: Edit worker stop token
**Description:** As a factory operator, I want to set a worker stop token so that model-oriented worker output can be treated as complete when the configured marker appears.

**Acceptance Criteria:**
- [ ] Worker configuration initializes stop token from the selected worker's current `stopToken` value.
- [ ] Stop token is edited as a worker-owned field and saved only to the selected worker.
- [ ] Helper copy clarifies stop token is worker-owned and unrelated to workstation `stopWords`.
- [ ] Saving worker stop token does not add, remove, or change any workstation `stopWords`.
- [ ] Empty or whitespace-only stop token input clears the worker `stopToken` field on save.
- [ ] Save, discard, dirty-state, conflict overwrite detection, and reset-to-latest behavior include stop token.
- [ ] Focused unit tests cover draft apply and dirty detection for stop token; save path does not mutate workstation `stopWords`.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill: edit stop token, save, confirm worker value persists and workstation stop words are unchanged.

### US-004: Preserve worker runtime fields across type changes
**Description:** As a factory operator, I want worker runtime settings to remain attached to the worker when changing worker type so that I do not accidentally lose timeout, stop token, or permission settings.

**Acceptance Criteria:**
- [ ] Changing between `MODEL_WORKER`, `SCRIPT_WORKER`, and `HOSTED_WORKER` preserves `timeout`, `stopToken`, and `skipPermissions` in the draft unless the operator edits those fields.
- [ ] Saving a type change writes the selected type plus the current runtime configuration values to the same worker.
- [ ] Existing preserved worker fields not in this PRD continue to be preserved.
- [ ] Unit tests cover at least one type-change save path with runtime fields present.
- [ ] Typecheck passes
- [ ] Tests pass

### US-005: Validate and save runtime configuration
**Description:** As a factory operator, I want validation errors for runtime settings to appear beside the relevant controls so that I can correct mistakes quickly.

**Acceptance Criteria:**
- [ ] Worker draft validation and contract-validation mapping support `timeout`, `stopToken`, and `skipPermissions` field names.
- [ ] Contract validation failures for `factory.workers[index].timeout` show on the timeout control.
- [ ] Contract validation failures for `factory.workers[index].stopToken` show on the stop token control.
- [ ] Contract validation failures for `factory.workers[index].skipPermissions` show on the permission control.
- [ ] Invalid timeout save errors map to the timeout field instead of only a generic contract error.
- [ ] Save remains disabled while local validation errors are present.
- [ ] Tests cover field-specific validation mapping for the new fields.
- [ ] Typecheck passes
- [ ] Tests pass

## Functional Requirements

- **FR-1:** The existing worker configuration editor must include controls for `skipPermissions`, `timeout`, and `stopToken`.
- **FR-2:** The editable worker draft must include `skipPermissions`, `timeout`, and `stopToken`.
- **FR-3:** The selected worker's existing `skipPermissions`, `timeout`, and `stopToken` values must populate the draft when the worker is selected.
- **FR-4:** Saving must write runtime settings only to the selected worker in `factory.workers`.
- **FR-5:** Saving must not mutate workstation `stopWords`.
- **FR-6:** `timeout` must be editable for all worker types.
- **FR-7:** `skipPermissions` must be shown for model workers only and must not be hidden based on model provider.
- **FR-8:** `stopToken` must be treated as a single worker-owned string value.
- **FR-9:** Empty timeout and empty stop token inputs must clear their corresponding optional fields from the worker payload.
- **FR-10:** Dirty-state, discard, conflict overwrite detection, reset-to-latest, and saved-state tracking must include the new fields.
- **FR-11:** Field-specific save validation errors must map to the new controls when backend or contract validation references the corresponding worker field.
- **FR-12:** UI copy must make clear that stop token is worker-owned and unrelated to workstation `stopWords`.

## Non-Goals

- Do not add or change API/OpenAPI contract fields.
- Do not add worker-owned `stopWords`.
- Do not edit workstation `stopWords` from the worker configuration editor.
- Do not deprecate or rename `stopToken`.
- Do not add `resources`, `openCodeAgent`, `operations`, `auth`, or `linear` editing in this PRD.
- Do not change backend worker execution behavior.
- Do not change provider-specific handling of `skipPermissions`.
- Do not show `skipPermissions` for non-model worker types (disabled/read-only or hidden).

## High-Level Technical Design

### Canonical state and draft model

- **Source of truth:** Session-scoped `CanonicalFactoryDefinition` via the current factory document hook, same as existing worker editable configuration.
- **Editable projection:** Extend `EditableWorkerValues` and `EditableWorkerDraft` with `skipPermissions` (boolean | null), `timeout` (string | null for Go duration), and `stopToken` (string).
- **Apply path:** Update `buildWorkerFromDraft` to write runtime fields from the draft instead of relying solely on `pickPreservedWorkerFields` passthrough. When disabled/empty, omit optional fields per existing serialization conventions.
- **Session state:** Extend `editableWorkerDraftsEqual`, dirty detection, and `resolveEditableWorkerOverwriteFields` to include the three runtime fields. Add mutators (`onSkipPermissionsChange`, `onTimeoutChange`, `onStopTokenChange`) to the editable configuration state hook.

### Duration picker

- Introduce a reusable duration picker (numeric input + unit select) if no suitable shared component exists.
- Parse existing Go duration strings (`30s`, `5m`, `1h`) into picker state on load; format picker state back to Go duration strings on apply.
- Represent "not configured" as empty picker state that clears `timeout` on save.
- Design the picker for later reuse with cron jitter and expiry window (follow-up cleanup, not in scope here).

### UI projection

- Add runtime controls inside the existing worker editable configuration expandable section in the worker detail card.
- Reuse existing form field shells, validation message treatment, shared inputs, and select styling.
- Show `skipPermissions` toggle only when `draft.type === "MODEL_WORKER"`.
- Show timeout and stop token for all editable worker types.

### Validation

- Extend `EditableWorkerValidationField`, contract field resolver, and save validation error types with `timeout`, `stopToken`, and `skipPermissions`.
- Map `factory.workers[N].timeout|stopToken|skipPermissions` contract errors to field-level messages on the corresponding controls.

### Verification surfaces

- Unit: `worker-editable-values` (draft init, apply, type-change preservation), duration parse/format helpers, validation field mapping.
- Component: `worker-editable-configuration-section` tests for control rendering, validation display, and model-worker-only permission bypass visibility.
- Save hook: confirm runtime fields participate in dirty/save and do not mutate workstations.

## Design Considerations

- Add the new controls inside the existing worker configuration expandable section.
- Reuse existing form field shells, validation message treatment, shared inputs, and select styling.
- Keep visible copy concise; helper text should clarify ownership and examples, not repeat the contract.
- The timeout picker should produce Go duration strings compatible with existing examples such as `30s`, `5m`, and `1h`.
- Loading, empty, error, and success states follow existing worker editable configuration patterns.

## Technical Considerations

- The canonical API contract already exposes `Worker.skipPermissions`, `Worker.timeout`, and `Worker.stopToken`.
- The current worker draft/apply path preserves these fields via `pickPreservedWorkerFields` but does not expose them as editable draft fields.
- Update worker editable values, draft creation, apply logic, equality checks, overwrite field detection, validation field mapping, and tests together so saves are not lossy.
- Existing docs and alias handling may reference `stopWords`; this PRD intentionally does not change that contract surface.
- Frontend changes must follow the current-selection canonical state pattern: the canonical factory definition remains the source of truth, and UI draft state is only an editable projection.

## Success Metrics

- A customer can configure `skipPermissions`, `timeout`, and `stopToken` for a worker without editing files by hand.
- A saved worker configuration reloads with the same runtime settings shown in the editor.
- Invalid timeout values produce field-specific feedback instead of a generic save failure.
- Worker runtime saves do not produce unintended workstation `stopWords` changes.

## Open Questions

- **Resolved:** `skipPermissions` remains absent (not shown) for non-model worker types.
- **Resolved:** The duration picker should become a shared primitive for cron jitter and expiry window in a later cleanup.
