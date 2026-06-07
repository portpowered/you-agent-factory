# PRD: Dashboard Select Dropdown Convergence

## Introduction

The dashboard still renders dropdown menus through a native `<select>` wrapper instead of the shared shadcn-style select experience, and feature component code can still bypass `ui/src/components/ui/` for standard form controls. This project converges dashboard dropdowns onto one shared select primitive, preserves existing form behavior in current selection, factory graph editing, submit work, and other dashboard selectors, and adds lint enforcement so future feature component work keeps using shared UI primitives.

## Context

### Customer ask

Migrate dropdown select menus such as the current-selection worker configuration, factory graph popup dialogs, and submit work to the shadcn select component. Add lint enforcement so feature component code does not directly use raw form controls when shared styled controls under `components/ui` should be used instead.

### Problem

- **Inconsistent dropdown behavior:** Dashboard dropdowns currently rely on a native select wrapper, which does not match the shared shadcn-style interaction model used for richer Radix-based primitives elsewhere.
- **Repeated feature-level coupling:** Current-selection editors, factory graph add flows, submit work, and other dashboard selectors each depend on the legacy select contract instead of a richer shared select surface.
- **Weak guardrails:** Feature component code can still render raw form controls or depend on low-level markup patterns rather than shared UI primitives, which makes styling drift and accessibility regressions easy to reintroduce.

### High-level solution

1. Introduce a shared shadcn-style select primitive under `ui/src/components/ui/` with accessible trigger, content, item, placeholder, disabled, and error-description support suitable for dashboard forms.
2. Migrate dashboard dropdown callers onto that shared primitive, including current-selection editable configuration flows, factory graph add dialogs, submit work, and remaining in-product selectors that still rely on the legacy select wrapper.
3. Add lint and repo-owned guard checks so feature component files use shared UI form primitives instead of reintroducing raw dropdown/form-control implementations.

## Goals

- Standardize dashboard dropdowns on one shared shadcn-style select behavior and visual treatment.
- Preserve existing form semantics, validation, dirty-state behavior, and saved values across current-selection, graph-editor, and submit-work flows.
- Keep dropdowns keyboard-operable, screen-reader understandable, and responsive on mobile and desktop.
- Add enforceable guardrails so future feature component work reuses `components/ui` form controls instead of bypassing them.

## Project-level acceptance criteria

- [ ] Dashboard dropdowns use the shared shadcn-style select primitive for current-selection editable configuration, factory graph add dialogs, submit work, and other existing dashboard select menus that currently depend on the legacy shared select wrapper.
- [ ] Affected dropdowns preserve existing selected values, validation messaging, disabled/loading behavior, and submission outcomes across success, empty, and error states relevant to each surface.
- [ ] Keyboard behavior is consistent across migrated dropdowns: trigger focus is visible, Enter/Space opens, arrow keys move through options, Escape closes, and selection updates the owning form state.
- [ ] Screen-reader semantics remain explicit through labels, descriptions, errors, and selected-value announcements for migrated form fields.
- [ ] Feature component guardrails fail lint when new feature component code bypasses shared UI form-control primitives in places where the repository requires `components/ui` ownership.
- [ ] Typecheck, lint, and affected automated tests pass.

## User Stories

### you-ui-select-dropdown-001: Shared shadcn select foundation

**Description:** As a maintainer, I want one shared dashboard select primitive with shadcn-style behavior so every dropdown can reuse consistent trigger, listbox, placeholder, and keyboard interaction instead of relying on a native wrapper.

**Acceptance Criteria:**

- [ ] `components/ui` exposes a shared select primitive suitable for dashboard forms, including trigger, value, content, item, and disabled-state behavior.
- [ ] The primitive supports labels, placeholder text, descriptions, inline validation wiring, controlled values, and keyboard interaction expected from shadcn/Radix select patterns.
- [ ] Shared styling uses existing dashboard tokens and remains usable on narrow mobile widths and standard desktop form layouts.
- [ ] UI foundation or shared-component tests cover open/close behavior, keyboard selection, disabled state, placeholder rendering, and accessible labeling.
- [ ] Typecheck passes
- [ ] Tests pass

### you-ui-select-dropdown-002: Current-selection editable configuration uses the shared select

**Description:** As an operator editing workers, workstations, and resources from current selection, I want every dropdown field to use the shared select experience so the form feels consistent while preserving save, validation, and dirty-state behavior.

**Acceptance Criteria:**

- [ ] Current-selection editable configuration dropdowns use the shared shadcn-style select for worker, workstation, and resource flows that currently render select menus.
- [ ] Worker, workstation, and resource dropdowns keep their current selected values, conditional field behavior, and save/dirty-state wiring.
- [ ] Validation and helper text remain associated with the correct field when a dropdown is invalid or disabled.
- [ ] Existing loading, empty, error, and ready states for current-selection detail cards are unchanged outside the select interaction upgrade.
- [ ] Affected current-selection component and hook tests prove value changes still update draft state and validation correctly.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill: current-selection worker/workstation/resource editors open dropdowns, change values, and preserve visible validation and save behavior

### you-ui-select-dropdown-003: Factory graph add dialogs use the shared select

**Description:** As an operator adding or editing graph-backed factory entities, I want the graph dialog dropdowns to use the same shared select behavior so adding workers, workstations, work types, and states feels consistent and keyboard-safe.

**Acceptance Criteria:**

- [ ] Factory graph add-dialog dropdowns use the shared shadcn-style select for kind, worker type, model provider, assigned worker, workstation type, work type, state type, and any other existing select-backed fields in that dialog flow.
- [ ] Conditional field visibility continues to respond correctly when the selected kind or type changes.
- [ ] Dialog submission, validation errors, disabled states, and focus management continue to work when the select menu is opened and closed from the modal.
- [ ] Component and integration-style tests prove representative add flows still succeed and validation still blocks missing required selections.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill: factory graph add dialog opens, dropdowns are keyboard-usable, conditional fields switch correctly, and save/validation still work

### you-ui-select-dropdown-004: Submit work and remaining dashboard selectors use the shared select

**Description:** As a dashboard user, I want submit-work and the remaining dashboard dropdowns to match the shared select interaction so every selector behaves consistently across the product.

**Acceptance Criteria:**

- [ ] Submit-work work-type selection uses the shared shadcn-style select without changing submit gating, validation, or status-panel behavior.
- [ ] Remaining dashboard select menus that still rely on the legacy shared select wrapper, such as dashboard add-widget selection and work-outcome range selection, use the new shared select contract.
- [ ] Empty-option, placeholder, and disabled behavior remain explicit on each affected surface.
- [ ] Affected tests and stories are updated so browser-visible dropdown behavior is asserted through the new trigger/content interaction instead of native select change assumptions.
- [ ] Typecheck passes
- [ ] Tests pass
- [ ] Verify in browser using dev-browser skill: submit-work, add-widget selector, and trend-range selector open and change values correctly on desktop and mobile-width layouts

### you-ui-select-dropdown-005: Lint and guardrails prevent feature component bypasses

**Description:** As a maintainer, I want lint and repo-owned checks to block feature component files from bypassing shared UI form primitives so raw dropdown implementations and other standard form-control drift do not come back.

**Acceptance Criteria:**

- [ ] Lint fails when feature component files introduce raw dropdown/select implementations instead of the shared `components/ui` select primitive.
- [ ] Guardrails also block the standard form-control bypasses this project intends to prevent in feature component directories when shared `components/ui` primitives already own that control category.
- [ ] Allowed exceptions, if any, are narrow, documented, and covered by tests for the guard itself.
- [ ] The main `ui` lint/check commands execute the new guard automatically.
- [ ] Guard tests prove both failure output and approved usage paths.
- [ ] Typecheck passes
- [ ] Tests pass

## High-level technical design

### Canonical UI boundary

| Layer | Responsibility |
| --- | --- |
| `ui/src/components/ui/` | Owns the canonical shadcn-style select primitive and any select-specific styling/semantics shared across the dashboard |
| `ui/src/features/**/components/` | Composes shared select primitives into feature forms without redefining dropdown markup contracts |
| `ui/scripts/` + `ui/biome.jsonc` | Enforce repository rules that feature component code must reuse shared form-control primitives |

### Migration approach

- Replace the legacy native-select wrapper contract with a shared select API that works in controlled forms and modal/dialog contexts.
- Migrate callers in dependency order: shared primitive first, then current-selection editors, then graph-editor dialogs, then submit-work and remaining selectors.
- Keep feature-owned state canonical in existing hooks/stores; the select primitive only projects and dispatches value changes.

### Verification surfaces

- Shared primitive tests for open/close, keyboard, placeholder, disabled, and accessible labeling behavior.
- Feature component tests for value changes, validation, conditional rendering, and modal/form integration.
- Browser verification for current-selection, factory graph add dialog, submit-work, and remaining visible dashboard selectors.
- Guard-script and lint tests for failure cases and approved exceptions.

## Functional Requirements

- **FR-1:** The dashboard MUST provide a shared shadcn-style select primitive under `ui/src/components/ui/` for standard dropdown interactions.
- **FR-2:** Current-selection editable configuration dropdowns MUST use the shared select primitive without changing canonical draft/save ownership.
- **FR-3:** Factory graph add-dialog dropdowns MUST use the shared select primitive and preserve conditional field logic and modal focus behavior.
- **FR-4:** Submit-work and any remaining dashboard selectors that currently rely on the legacy shared select wrapper MUST use the shared select primitive.
- **FR-5:** Migrated dropdowns MUST preserve labels, descriptions, validation errors, keyboard support, disabled states, and responsive layouts.
- **FR-6:** Repository lint/check tooling MUST fail when feature component files bypass shared UI dropdown/form-control primitives in the guarded categories.

## Non-Goals

- Rewriting non-dropdown menus or popovers that are already using the correct shared primitives.
- Broad layout refactors of current-selection, submit-work, or graph-editor cards beyond what the select migration requires.
- Introducing new domain-level validation rules or backend contract changes unrelated to dropdown interaction.

## Supporting technical and UX considerations

- Reuse existing dashboard form labels, descriptions, errors, and tokenized surfaces rather than inventing feature-local select chrome.
- Modal and popover contexts need explicit layering and focus behavior so select content does not break dialog interaction.
- Tests should verify behavior, not implementation details such as internal portal structure or CSS class inventories.
- Any allowed guard exceptions should be narrow and justified by a missing shared primitive, not convenience.

## Success metrics

- Operators can use the same dropdown interaction model across current-selection, graph-editor, submit-work, and other dashboard selectors.
- Dropdown-related UI regressions caused by native-select limitations or feature-local bypasses stop recurring.
- New feature component code that tries to reintroduce guarded raw form controls fails CI immediately.

## Open Questions

None.
