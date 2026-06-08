# PRD: Work Content OpenAPI Type Consolidation

---
author: Codex
last modified: 2026, june, 8
status: draft
---

## Context

### Project Overview

`infinite-you` is an AI agent factory that helps customers schedule and orchestrate concurrent AI work across a Go backend, an OpenAPI-defined contract surface, and a React website built from feature-owned modules. This work item is a narrow website maintainability cleanup inside the `work-content` feature so the feature has one canonical place for its OpenAPI-derived work content aliases without changing any customer-visible behavior.

### Customer Ask

Collapse the duplicate `WorkContent` and `WorkContentPart` OpenAPI schema aliases currently re-declared in `ui/src/features/work-content/components/work-content-part-list.tsx`, `ui/src/features/work-content/components/work-content-read-only-list.tsx`, and `ui/src/features/work-content/lib/describe-work-content-part.ts` into one canonical module inside `ui/src/features/work-content/`, update internal imports to use that module, remove redundant component-level type exports unless a public re-export is still intentionally required, keep runtime behavior, rendering, and message copy unchanged, and prove the seam with `make ui-deadcode` plus the relevant existing work-content tests.

### Problem

The `work-content` feature currently repeats identical `components["schemas"]["WorkContent"]` and `components["schemas"]["WorkContentPart"]` aliases in three separate modules that have no distinct ownership or behavior. That duplication widens the apparent number of type-definition seams inside one feature, makes internal imports less explicit about the canonical source of truth, and creates unnecessary maintenance churn when reviewers need to verify which alias is intended to be authoritative.

### Solution

Add one feature-owned canonical module for the `WorkContent` and `WorkContentPart` aliases, retarget the existing list and helper modules to import those aliases from that single source, and remove the duplicate alias declarations from the component and helper modules unless an intentional feature-public re-export is required. Keep the change strictly behavior-preserving and verify it through the existing work-content rendering and description tests plus `make ui-deadcode`.

## Project-Level Acceptance Criteria

- [ ] Exactly one canonical feature-owned module under `ui/src/features/work-content/` defines the `WorkContent` and `WorkContentPart` aliases for `components["schemas"]["WorkContent"]` and `components["schemas"]["WorkContentPart"]`.
- [ ] `work-content-part-list`, `work-content-read-only-list`, and `describe-work-content-part` import the canonical aliases instead of re-declaring identical OpenAPI schema aliases locally.
- [ ] The `WorkContentPartList` and `WorkContentReadOnlyList` surfaces keep the same loading, unavailable, error, empty, and populated rendering behavior, accessible region labeling, and message copy after the alias consolidation.
- [ ] `describeWorkContentPart` keeps the same file, URL, label, content-type, and fallback description behavior after the alias consolidation.
- [ ] Redundant component-level and helper-level type exports are removed unless an intentional feature-public re-export is still required, and no unrelated `work-content` type moves or alias renames are introduced.
- [ ] The change stays scoped to `ui/src/features/work-content/` and does not rename `ImportFactoryValue`, `CurrentFactoryDocument`, timeline-local `WorkContent` aliases, or `CURRENT_ACTIVITY_NODE_TYPES`.
- [ ] Quality gate: `make ui-deadcode`, frontend typecheck, and the relevant existing tests for `work-content-read-only-list` and `describe-work-content-part` pass.

## Goals

- [ ] Establish one canonical `WorkContent` and `WorkContentPart` alias module inside the `work-content` feature.
- [ ] Make internal `work-content` consumers depend on that canonical module instead of repeating identical OpenAPI aliases.
- [ ] Preserve all current work-content UI rendering, helper output, and message behavior.
- [ ] Keep the cleanup narrow, reviewable, and backed by focused regression evidence.

## User Stories

### work-content-openapi-type-consolidation-001: Establish the canonical work-content type seam

**Description:** As a frontend maintainer, I want the `work-content` feature to own one canonical module for `WorkContent` and `WorkContentPart` so that internal feature code depends on a single explicit type source instead of repeating identical OpenAPI aliases.

**Acceptance Criteria:**
- [ ] A canonical module such as `ui/src/features/work-content/lib/work-content-types.ts` exports `WorkContent` and `WorkContentPart` as the feature-owned aliases for the generated OpenAPI schemas.
- [ ] `ui/src/features/work-content/components/work-content-part-list.tsx`, `ui/src/features/work-content/components/work-content-read-only-list.tsx`, and `ui/src/features/work-content/lib/describe-work-content-part.ts` import the aliases from the canonical module instead of declaring their own identical aliases.
- [ ] Internal feature typing remains explicit and local to `work-content`; the change does not move the aliases outside the feature or introduce new cross-feature type dependencies.
- [ ] The canonical module becomes the only definition site for these two aliases inside `ui/src/features/work-content/`.
- [ ] Typecheck passes

### work-content-openapi-type-consolidation-002: Remove duplicate alias surfaces without changing work-content behavior

**Description:** As a reviewer, I want duplicate `work-content` alias declarations removed from component and helper modules so that the feature boundary is easier to maintain while the existing list rendering and part-description behavior remain unchanged.

**Acceptance Criteria:**
- [ ] Redundant `export type WorkContent` and `export type WorkContentPart` declarations are removed from the component and helper modules unless an intentional public re-export is still required through `ui/src/features/work-content/public/index.ts`.
- [ ] `WorkContentPartList` continues to render authored text, formatted JSON, and fallback non-text descriptions exactly as before for unchanged work-content fixtures.
- [ ] `WorkContentReadOnlyList` continues to render the same loading, unavailable, error, empty, and populated states with the same accessible region semantics and message copy.
- [ ] `describeWorkContentPart` continues to return the same descriptions for file-backed content, file URLs, remote URLs, labels, content types, and part-type-only fallbacks.
- [ ] Verification uses the existing behavioral coverage in `ui/src/features/work-content/components/work-content-read-only-list.test.tsx` and `ui/src/features/work-content/lib/describe-work-content-part.test.ts` rather than new file-topology or export-inventory tests.
- [ ] The cleanup does not broaden into cross-feature type moves, runtime rendering changes, or unrelated work-content refactors.
- [ ] Typecheck passes
- [ ] Tests pass

## High-Level Technical Design

This work is a feature-local ownership cleanup, not a contract or rendering redesign. The canonical data model remains the generated OpenAPI `components["schemas"]["WorkContent"]` and `components["schemas"]["WorkContentPart"]` types. The new `work-content` canonical module simply gives the feature one place to alias those generated types for internal use.

After consolidation:

- `work-content-types.ts` is the only definition site for the feature-local `WorkContent` and `WorkContentPart` aliases.
- `WorkContentPartList` consumes the canonical aliases while preserving its existing rendering split between text, JSON, and fallback descriptive parts.
- `WorkContentReadOnlyList` continues to treat `content` as the canonical source for populated-state rendering while preserving explicit loading, unavailable, error, and empty states.
- `describeWorkContentPart` continues to be a pure helper over `WorkContentPart`, with unchanged file, URL, label, content-type, and fallback resolution behavior.

If any external consumer genuinely needs these aliases from the feature boundary, the public seam should re-export from the canonical module rather than allowing component-level alias redeclarations to remain the source of truth. Otherwise, the aliases should remain feature-internal.

## Functional Requirements

1. FR-1: The `work-content` feature must define `WorkContent` and `WorkContentPart` in one canonical module inside `ui/src/features/work-content/`.
2. FR-2: `work-content-part-list`, `work-content-read-only-list`, and `describe-work-content-part` must import those aliases from the canonical module.
3. FR-3: Duplicate local alias declarations for the same OpenAPI schema types must be removed from the affected component and helper modules unless an intentional public re-export is still required.
4. FR-4: The consolidation must not change `WorkContentPartList` rendering behavior for text, JSON, or fallback descriptive items.
5. FR-5: The consolidation must not change `WorkContentReadOnlyList` loading, unavailable, error, empty, populated, or accessible-region behavior.
6. FR-6: The consolidation must not change `describeWorkContentPart` output for existing file, URL, label, content-type, and fallback cases.
7. FR-7: Verification must rely on focused behavioral evidence and `make ui-deadcode`, not on meta tests about file layout or export inventories.

## Non-Goals

- Renaming `ImportFactoryValue`, `CurrentFactoryDocument`, or timeline-local `WorkContent` aliases.
- Retiring the `CURRENT_ACTIVITY_NODE_TYPES` alias or doing overlapping editor cleanup.
- Moving `WorkContent` or `WorkContentPart` aliases outside the `work-content` feature.
- Changing runtime rendering, message copy, helper output, or OpenAPI schema shapes.
- Adding topology-only tests, export inventory assertions, or unrelated `work-content` cleanup.

## Supporting Technical Or UX Considerations

- This is a frontend-only cleanup, so the strongest evidence is unchanged behavior in the existing list and helper tests plus dead-code verification that no duplicate seam remains in active feature code.
- Because `WorkContentReadOnlyList` is a user-visible surface, the plan keeps loading, unavailable, error, empty, and populated states explicit even though the intended implementation change is type-only.
- The feature should continue to treat the generated OpenAPI types as the canonical contract model; the new module is a feature-owned alias seam, not a replacement schema.

## Success Metrics

- Reviewers can identify one canonical `WorkContent` and `WorkContentPart` alias module inside `work-content` with no duplicate definition sites in the affected feature modules.
- Existing work-content tests continue to prove unchanged rendering and description behavior after the consolidation.
- `make ui-deadcode` and focused frontend verification remain green without follow-up cleanup work.

## Open Questions

None.
