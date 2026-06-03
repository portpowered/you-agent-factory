# PRD: Workstation Guard Selector Autocomplete

## Introduction

The dashboard **current-selection** workstation configuration panel lets factory authors add workstation-level `MATCHES_FIELDS` guards and edit `matchConfig.inputKey` (for example `.Name` or `.Tags["_last_output"]`). Today that selector field is hard to use for two reasons:

1. **Focus loss while typing:** Guard list rows use a React `key` that includes `formatWorkstationGuardSummary(guard)`, which embeds the live `inputKey`. Each keystroke changes the summary, changes the key, remounts the row, and blurs the active input—so authors cannot type a full selector in one pass.
2. **No authoring help:** The selector is a plain text `Input`, while the workstation prompt editor already offers a Monaco-style autocomplete/help pattern for related template variables.

This project fixes the focus regression and adds a **compact Monaco-style selector editor** for `MATCHES_FIELDS` guards with curated suggestions (`.Name`, `.WorkID`, `.Tags["key"]`). Validation stays limited to the existing required-field rule for empty `inputKey`; there is no new selector-syntax validation contract.

**Intent:** Operators can author guard field selectors as comfortably as they edit prompts—without losing focus, without memorizing syntax, and without changing runtime `MATCHES_FIELDS` behavior or backend validation.

## Context

### Customer ask

Fix the guard selector focus-loss regression and add Monaco-style autocomplete/help for `MATCHES_FIELDS` `matchConfig.inputKey`, suggesting `.Name`, `.WorkID`, and a `.Tags["key"]` template. Keep validation behavior unchanged (existing required-field errors only).

### Problem

| Gap | Observable symptom |
|-----|-------------------|
| Unstable row identity | Typing into the selector blurs the field after each character; authors cannot enter `.Tags["_last_output"]` in one interaction. |
| Summary-driven list keys | `EditableConfigurationWorkstationGuardsField` keys each `<li>` with `formatWorkstationGuardSummary(guard)`, which changes as `inputKey` changes. |
| Plain text only | No inline suggestions for common selector patterns even though prompt editing already uses `MonacoPromptEditor` patterns. |
| Trust risk on save | If focus loss interrupts editing, operators may believe save dropped or mangled selectors even when draft mapping is otherwise correct. |

### High-level solution

1. Give each workstation guard row a **stable client-side identity** that does not change when mutable summary fields (such as `inputKey`) change; keep summary text in the row header without using it as the list key.
2. Replace the `MATCHES_FIELDS` plain text input with a **small Monaco-based selector editor** that reuses established prompt-editor setup patterns but scopes completion items to guard selectors only.
3. Offer curated completions for `.Name`, `.WorkID`, and `.Tags["key"]` with short descriptions; still allow free-form text.
4. Add **focused component tests** for focus stability, draft updates, save payload mapping, and unchanged required-field validation.

**Scope boundary:** Frontend-only UX in the workstation guards field; no OpenAPI, backend, or runtime guard matching changes.

## Project-level acceptance criteria

- [ ] **PAC-1:** A factory author can type a full `MATCHES_FIELDS` selector (for example `.Tags["_last_output"]`) in one continuous interaction without the selector editor losing focus or remounting.
- [ ] **PAC-2:** Each keystroke or accepted suggestion updates the editable workstation draft at `guards[index].matchConfig.inputKey`; the guard row summary may update without remounting the row.
- [ ] **PAC-3:** The selector editor offers Monaco-style suggestions for `.Name`, `.WorkID`, and `.Tags["key"]` (with brief descriptions) while still accepting arbitrary manual text.
- [ ] **PAC-4:** Empty or whitespace-only `matchConfig.inputKey` continues to show the existing required-field validation error; non-empty values that are not in the suggestion list do not block save.
- [ ] **PAC-5:** Saving workstation configuration sends the exact edited selector string in `matchConfig.inputKey` without new normalization beyond existing trim-on-validation behavior.
- [ ] **PAC-6 (quality gate):** UI typecheck, lint, and targeted tests for touched workstation guard editing paths pass.

## Goals

- Eliminate focus loss while editing `MATCHES_FIELDS` guard selectors.
- Preserve draft updates and existing save payload mapping for `matchConfig.inputKey`.
- Provide a compact Monaco-style autocomplete/help experience aligned with the workstation prompt editor.
- Suggest `.Name`, `.WorkID`, and `.Tags["key"]` with short, actionable descriptions.
- Keep validation limited to the current required-field rule for empty `inputKey`.
- Prove focus stability and save payload correctness with direct component tests.

## User Stories

### prd-workstation-guard-selector-autocomplete-001: Stable guard row identity and continuous selector editing

**Description:** As a factory operator, I want to type a guard selector continuously so that I can enter values like `.Tags["_last_output"]` without the field losing focus after each character.

**Acceptance Criteria:**

- [ ] Workstation guard list rows use a stable React identity that does not change when `matchConfig.inputKey` or other mutable summary fields change (for example a client-generated row id assigned when a guard is added, combined with list index only where necessary).
- [ ] `formatWorkstationGuardSummary(guard)` is not part of the guard row list `key`.
- [ ] Typing multiple characters into a `MATCHES_FIELDS` `matchConfig.inputKey` control does not blur or remount the active editor; `document.activeElement` remains the selector control through the interaction in component tests.
- [ ] Each typed change updates the editable draft via the existing `onGuardsChange` path with the latest `matchConfig.inputKey` value.
- [ ] The visible guard summary text updates while the row remains the same React instance (no loss of focus).
- [ ] Adding, removing, and editing other guard rows continues to work.
- [ ] Component tests simulate multi-character typing into the selector and assert the control stays focused and the draft receives the full string.
- [ ] Typecheck passes
- [ ] Tests pass

### prd-workstation-guard-selector-autocomplete-002: Preserve save payload mapping for edited selectors

**Description:** As a factory operator, I want the selector I typed to save exactly as `matchConfig.inputKey` so that guard behavior matches what I authored in the dashboard.

**Acceptance Criteria:**

- [ ] Editing a `MATCHES_FIELDS` selector updates `guards[index].matchConfig.inputKey` on the editable workstation draft returned through the guards field `onGuardsChange` callback.
- [ ] When the editable workstation configuration save path runs with a valid draft, the workstation save payload includes the edited selector at `guards[index].matchConfig.inputKey` with the same string the operator entered (no new rename/normalization beyond existing trim used only for required-field validation).
- [ ] Selectors containing quotes and brackets (for example `.Tags["_last_output"]`) round-trip unchanged through draft → save payload mapping in focused tests.
- [ ] Typecheck passes
- [ ] Tests pass

### prd-workstation-guard-selector-autocomplete-003: Monaco-style selector autocomplete for MATCHES_FIELDS

**Description:** As a factory operator, I want selector suggestions while editing `MATCHES_FIELDS` guards so that I can choose common patterns without memorizing exact syntax.

**Acceptance Criteria:**

- [ ] The `MATCHES_FIELDS` selector control uses a Monaco-style editor/autocomplete pattern consistent with `MonacoPromptEditor` (compact height, accessible label/`aria-describedby`, loading/error affordances as applicable).
- [ ] Triggering autocomplete shows `.Name` with a short description of what it matches.
- [ ] Autocomplete shows `.WorkID` with a short description.
- [ ] Autocomplete shows `.Tags["key"]` (or equivalent placeholder) with a short description that users should replace `key` with a real tag name.
- [ ] Accepting a suggestion writes the selector value into `matchConfig.inputKey` through the same draft update path as manual typing.
- [ ] Operators can still type arbitrary selector text manually without being forced to pick a suggestion.
- [ ] Selector completion items are scoped to guard selectors (not prompt template variables).
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill: open workstation Configuration, add or edit a `MATCHES_FIELDS` guard, open suggestions, accept `.Name` and type a custom tag selector.

### prd-workstation-guard-selector-autocomplete-004: Keep existing validation semantics

**Description:** As a maintainer, I want selector autocomplete to avoid changing save-time validation so this UX improvement does not introduce a new contract.

**Acceptance Criteria:**

- [ ] Empty or whitespace-only `matchConfig.inputKey` continues to surface `editableConfigurationMatchesFieldsInputKeyRequired` (or current equivalent) on `guards[index].matchConfig.inputKey` and blocks save with other invalid guard fields.
- [ ] A non-empty selector that is not one of the suggested values does not add editor diagnostics or save-blocking errors by itself.
- [ ] No new backend or OpenAPI validation rule is added for selector syntax in this work.
- [ ] Existing tests in `workstation-editable-validation.test.ts` and guard save/mapping tests continue to pass without changing expected messages for the required-field case.
- [ ] Typecheck passes
- [ ] Tests pass

## Functional Requirements

- **FR-1:** `MATCHES_FIELDS` guard selector editing must not remount the guard row or selector editor on each keystroke.
- **FR-2:** Guard row list keys must remain stable across edits to mutable summary fields such as `matchConfig.inputKey`.
- **FR-3:** The selector editor must update `matchConfig.inputKey` on the editable workstation draft on every change.
- **FR-4:** The selector editor must offer curated suggestions for `.Name`, `.WorkID`, and `.Tags["key"]` with brief descriptions.
- **FR-5:** The selector editor must allow free-form text entry and must not restrict values to the suggestion list.
- **FR-6:** Required-field validation for empty/whitespace `matchConfig.inputKey` must remain unchanged.
- **FR-7:** No new backend selector syntax validation may be introduced.
- **FR-8:** Save behavior must preserve the exact edited selector string in `matchConfig.inputKey` (subject only to existing trim semantics used for emptiness checks).
- **FR-9:** Component or hook tests must prove focus stability during multi-character typing and correct save payload mapping.

## Non-Goals

- Building a full selector language parser or linter.
- Autocompleting every possible runtime token field or dynamic tag keys from live work history.
- Changing `MATCHES_FIELDS` runtime matching behavior.
- Redesigning per-input guard editing or `VISIT_COUNT` guard fields.
- Adding new backend/OpenAPI validation for selector syntax.
- Broad unrelated dashboard or prompt-editor refactors.

## High-Level Technical Design

### Canonical state and mutations

- **Canonical model:** `EditableWorkstationDraft.guards[]` on the selected workstation, shaped by OpenAPI `Guard` / `matchConfig.inputKey`.
- **Mutations:** `EditableConfigurationWorkstationGuardsField` `onGuardsChange` replaces one guard via `guards.map` or filters on remove; `MatchesFieldsGuardFields` `onChange` updates `matchConfig.inputKey`.
- **Validation:** `validateEditableWorkstationDraft` in `workstation-editable-validation.ts` (required non-empty `inputKey` after trim); field errors keyed as `guards[index].matchConfig.inputKey`.
- **Save:** Existing `applyEditableWorkstationDraft` + session-scoped factory document save; no new API fields.

### Row identity (focus fix)

- Today: list `key` is `` `${guard.type}-${formatWorkstationGuardSummary(guard)}-${index}` `` in `workstation-guards-field.tsx`, which remounts when `inputKey` changes.
- Target: assign each guard row a **stable `rowId`** (for example `crypto.randomUUID()` or monotonic id) when the guard is created in `onGuardsChange`, stored on the draft or parallel editor-local map keyed by index; list `key` uses `rowId` only.
- Summary text in the row header may continue to call `formatWorkstationGuardSummary(guard)` without affecting identity.

### Selector editor (autocomplete)

- Introduce a focused **guard selector Monaco editor** (new component under `ui/src/components/prompt-editor/` or workstation-selection components) that:
  - Reuses Monaco registration/bootstrap patterns from `monaco-prompt-setup.ts` where practical, with a separate language/completion provider scoped to selector literals.
  - Uses a smaller default height than the workstation prompt editor.
  - Exposes `value`, `onChange`, `ariaLabel`, `ariaDescribedBy`, `ariaInvalid`, and optional `fieldErrors` wiring consistent with `MatchesFieldsGuardFields`.
- Completion items (static v1):
  - `.Name` — match grouped inputs by resolved work name.
  - `.WorkID` — match by work identifier.
  - `.Tags["key"]` — template for tag lookup; description states to replace `key`.

### Verification layers

| Behavior | Evidence |
|----------|----------|
| Focus stability | `workstation-guards-field.test.tsx` (or dedicated selector editor test) using `userEvent.type` and `document.activeElement` / role queries |
| Draft + save payload | Component test with `onGuardsChange` spy; hook/unit test through editable workstation save builder if already exposed |
| Autocomplete | Component test that accepts a suggestion and asserts input value; browser verification for Monaco widget |
| Validation unchanged | Existing `workstation-editable-validation.test.ts` + guards field error display test |

## Design Considerations

- The selector editor should feel related to the workstation prompt editor but remain visually lighter (shorter height, no diagnostics panel unless reusing shared error display below the field).
- Suggestion descriptions should be one line each; the `.Tags["key"]` item must make the placeholder obvious.
- Keep keyboard accessibility: label association, `aria-invalid` when `guards[index].matchConfig.inputKey` has an error, and operable suggestion UI via keyboard where Monaco provides it.

## Supporting Technical and UX Considerations

- Reuse `DashboardActionButton`, `GuardFieldError`, and dashboard typography classes for consistency with other guard fields.
- Do not export guard selector editor from `workstation-selection/public` unless other features need it.
- Tests should assert user-visible outcomes (focus, value, callback payload), not internal `key` string formats.
- `VISIT_COUNT` rows should pick up stable row identity from the same mechanism to avoid asymmetric remount behavior when visiting count fields change summaries.

## Success Metrics

- Operators can type `.Tags["_last_output"]` in one continuous interaction without focus loss.
- Operators can insert `.Name`, `.WorkID`, or the tag template from autocomplete in fewer steps than manual typing from memory.
- Saved workstations reload with the exact `matchConfig.inputKey` authored in the dashboard.
- Existing guard validation and factory guard mapping tests remain green.

## Open Questions

- Should a follow-up mine known tag keys from factory submissions or runtime tokens for dynamic `.Tags[...]` suggestions?
- Should non-blocking selector syntax hints be added later once the selector language is formally specified?
