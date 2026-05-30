# PRD: Inline Workflow Activity Bento Card Graph Panel Shell Class

## Introduction

Customer ask `1.5` continues shrinking the inline-component-class allowlist by inlining single-use class constants directly on nearby JSX. The workflow-activity bento card still declares `GRAPH_PANEL_SHELL_CLASS` as a module constant and references it once on the graph panel shell `<section>`. That pattern keeps an unnecessary allowlist exception alive without improving reuse or readability.

This change inlines the graph panel shell layout classes on the `<section>` element, removes the module constant, and deletes the matching allowlist entry. Graph editor wiring, selection behavior, and bento card composition stay unchanged.

## Context

### Customer ask

Inline `GRAPH_PANEL_SHELL_CLASS` directly on the JSX `className` and remove the matching allowlist entry while preserving workflow-activity bento card layout.

### Problem

- `GRAPH_PANEL_SHELL_CLASS` is declared separately from JSX even though it is used only once.
- The inline-class guard still carries a stale-exception entry for this constant.
- The extra indirection adds maintenance noise without enabling reuse or conditional styling.

### Solution

Move `"relative h-full min-h-0 min-w-0"` onto the graph panel shell `<section className="...">`, delete the constant, remove the allowlist entry, and prove layout behavior is unchanged through focused component tests and the inline-class guard.

## Goals

- Remove `GRAPH_PANEL_SHELL_CLASS` from `workflow-activity-bento-card.tsx`.
- Keep the graph panel shell layout classes identical in rendered output.
- Remove the corresponding allowlist exception and keep the inline-class guard green.
- Preserve existing workflow-activity bento card behavior and composition.

## Project-Level Acceptance Criteria

- [ ] `GRAPH_PANEL_SHELL_CLASS` no longer exists in `ui/src/features/workflow-activity/components/workflow-activity-bento-card.tsx`.
- [ ] The graph panel shell `<section>` renders with `relative h-full min-h-0 min-w-0` in the DOM.
- [ ] The allowlist entry `src/features/workflow-activity/components/workflow-activity-bento-card.tsx#GRAPH_PANEL_SHELL_CLASS` is removed from `ui/scripts/inline-component-class-usage-allowlist.mjs`.
- [ ] `bun run check:inline-component-class-usage --cwd ui` passes with no stale allowlist entries.
- [ ] Focused `WorkflowActivityBentoCard` tests pass with no changes to graph editor wiring, selection behavior, or card composition.
- [ ] Repository quality gate passes: UI typecheck, lint, and affected tests are green.

## User Stories

### inline-workflow-activity-bento-card-class-001: Inline graph panel shell classes on WorkflowActivityBentoCard

**Description:** As a dashboard user, I want the workflow-activity bento card graph panel shell to keep the same flex-safe layout classes so the embedded React Flow graph still fills the card without overflow regressions.

**Acceptance Criteria:**

- [ ] Remove the `GRAPH_PANEL_SHELL_CLASS` module constant from `workflow-activity-bento-card.tsx`.
- [ ] Set the graph panel shell `<section>` `className` directly to `"relative h-full min-h-0 min-w-0"`.
- [ ] Do not change graph editor wiring, selection handlers, header actions, or `ReactFlowCurrentActivityCardView` props.
- [ ] Add or extend a `WorkflowActivityBentoCard` rendering test that asserts the graph panel shell `<section>` wrapping the viewport region has a `className` containing `relative`, `h-full`, `min-h-0`, and `min-w-0`.
- [ ] Typecheck passes
- [ ] Tests pass

### inline-workflow-activity-bento-card-class-002: Remove the graph panel shell allowlist exception

**Description:** As a frontend maintainer, I want the inline-class guard to stop carrying an exception for a constant that no longer exists so policy checks stay honest and the allowlist keeps shrinking.

**Acceptance Criteria:**

- [ ] Remove `src/features/workflow-activity/components/workflow-activity-bento-card.tsx#GRAPH_PANEL_SHELL_CLASS` from `ui/scripts/inline-component-class-usage-allowlist.mjs`.
- [ ] Do not modify other allowlisted entries or unrelated workflow-activity files in this slice.
- [ ] `bun run check:inline-component-class-usage --cwd ui` exits successfully with no stale allowlist entries reported.
- [ ] Focused `workflow-activity-bento-card` tests still pass after the allowlist cleanup.
- [ ] Typecheck passes
- [ ] Tests pass

## Functional Requirements

- FR-1: The graph panel shell `<section>` in `WorkflowActivityBentoCard` must use the exact class string `relative h-full min-h-0 min-w-0` inline on JSX.
- FR-2: No module-level class constant may remain for the graph panel shell in `workflow-activity-bento-card.tsx`.
- FR-3: The inline-component-class allowlist must no longer reference `GRAPH_PANEL_SHELL_CLASS` for this file.
- FR-4: Existing workflow-activity bento card rendering, editor entry, duplicate-instance accessibility, and header-action ordering behavior must remain unchanged.

## Non-Goals

- Do not inline or modify other allowlisted class constants in this slice.
- Do not change notifications, work-outcome, or current-factory-definition public barrels.
- Do not refactor graph editor hooks, React Flow wiring, or bento card composition beyond the class inlining.
- Do not add structural, route-inventory, or source-registration meta tests.

## High-Level Technical Design

This is a localized JSX cleanup with no API, state, or routing changes.

1. **Component edit:** Replace `<section className={GRAPH_PANEL_SHELL_CLASS}>` with a literal `className` on the same element and delete the constant declaration.
2. **Behavioral proof:** Extend the existing Vitest suite in `workflow-activity-bento-card.test.tsx` to assert the shell section's rendered classes rather than scanning source structure.
3. **Policy cleanup:** Remove the single stale `file#CONSTANT` allowlist entry in the same follow-up story so `check-inline-component-class-usage.mjs` reports zero stale exceptions.

No new components, hooks, or shared utilities are required.

## Supporting Technical and UX Considerations

- The class string controls flex containment for the embedded graph viewport; changing token order or dropping a class can cause overflow or collapsed layout inside the bento card.
- The inline-class guard treats removed constants as stale allowlist entries; the allowlist change must land after the constant is deleted.
- Prefer reusing the existing `WorkflowActivityBentoCard` test harness and semantic dashboard snapshot fixtures rather than adding new Storybook or browser-only coverage for this refactor.
- Visible UI behavior should remain identical; no browser-only verification is required beyond the rendering test assertion on the shell section classes.

## Success Metrics

- One fewer entry in `inline-component-class-usage-allowlist.mjs`.
- Zero regressions in focused workflow-activity bento card tests.
- Inline-class guard passes on the first run after allowlist cleanup.
- No user-visible layout or interaction change on the dashboard workflow-activity card.

## Open Questions

None. Scope, target files, verification commands, and out-of-scope boundaries are fully specified by the customer ask.
