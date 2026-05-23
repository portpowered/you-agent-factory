# PRD: Feature Directory Governance

## Context

### Project Overview

`infinite-you` is an AI agent factory that helps customers schedule and orchestrate large numbers of AI workers across backend, CLI, API, and website surfaces. This work item focuses on the website code under `ui/src/features`, where feature directories currently mix public entrypoints, React components, hooks, state stores, messages, stories, tests, and pure helper modules directly at the feature root.

### Customer Ask

Define a durable feature-directory model for the website so feature ownership is easier to understand, oversized features can be split into smaller sibling features when appropriate, and automated validation prevents any files from existing directly at `ui/src/features/<feature>/`.

### Problem

The current website feature tree has two related problems. First, some feature directories have grown broad enough that they contain multiple distinct product concerns inside one feature boundary. Second, even when a feature remains conceptually valid, its root directory often becomes a catch-all for files that do not have a clear home. That increases the cost of navigation, slows reviews, makes public vs internal ownership less obvious, and leaves structure decisions open to repeated debate.

The lack of an automated guardrail is part of the problem. Even if maintainers agree on a preferred layout, the repository currently allows feature-root files to drift back in over time. The requested model needs both a directory contract and an enforceable validation rule so the structure can improve without regressing.

### Solution

Adopt a feature-governance model for `ui/src/features` with three core rules:

1. A feature root directory may contain directories only; no files may exist directly at `ui/src/features/<feature>/`.
2. Feature internals should be organized by clear ownership using standard subdirectories such as `components/`, `hooks/`, `messages/`, `state/`, `selectors/`, `lib/`, or another well-named domain folder when the feature has a stronger internal concept.
3. When a feature grows beyond one coherent responsibility, the default should be to split it into multiple sibling features under `ui/src/features/` rather than hiding multiple concerns inside one oversized feature.

Because existing features already violate the rule, enforcement should ship with a temporary allowlist that records current exceptions and must shrink over time. New violations outside the allowlist should fail validation immediately.

## Project-Level Acceptance Criteria

- The repository defines one explicit governance model for directories under `ui/src/features`.
- The governance model states that `ui/src/features/<feature>/` may contain subdirectories only and may not contain any files, including `index.ts`.
- The governance model defines approved internal ownership buckets such as `components/`, `hooks/`, `messages/`, `state/`, `selectors/`, `lib/`, and other well-named domain folders when needed.
- The governance model states that oversized features should default toward splitting into multiple sibling features when that produces clearer ownership than adding more internal buckets.
- The repository gains automated validation that fails when a feature root contains a non-allowlisted file.
- The initial rollout supports a temporary allowlist for current violations, and the validation output makes it clear which files remain allowlisted debt.
- The migration plan preserves current website behavior, stable import intent, accessibility, and explicit loading, empty, error, and success states for affected surfaces.
- Quality gate: frontend lint, typecheck, and relevant automated tests pass for each migration slice and for the final governance enforcement changes.

## Goals

- Make feature ownership predictable across all directories under `ui/src/features`.
- Prevent feature-root files from accumulating over time.
- Encourage smaller behavior-owned features instead of oversized catch-all features.
- Preserve current website behavior while the directory contract is rolled out.
- Provide a migration path that is strict for new code and manageable for existing debt.

## User Stories

### US-001: Define the feature directory contract
**Description:** As a maintainer, I want one clear directory contract for `ui/src/features` so that I can place new code without guessing where it belongs.

**Acceptance Criteria:**
- [ ] The repository documents that each `ui/src/features/<feature>/` directory may contain subdirectories only and may not contain files directly at the feature root.
- [ ] The documented contract explicitly states that `index.ts` is not a special exception and must also live under a subdirectory if a public entrypoint is needed.
- [ ] The documented contract names the standard internal ownership buckets for feature code, including `components/`, `hooks/`, `messages/`, `state/`, `selectors/`, and `lib/`.
- [ ] The documented contract allows a feature to introduce a well-named domain folder such as `editing/`, `graph/`, `chart/`, or `replay/` when that name describes the ownership more clearly than a generic helper bucket.
- [ ] The documented contract explains that code shared across multiple features should be promoted out of `ui/src/features/<feature>/` into an appropriate shared UI module rather than copied between features.
- [ ] Typecheck passes.
- [ ] Tests pass.

### US-002: Enforce the no-root-files rule with temporary debt tracking
**Description:** As a maintainer, I want automated validation for feature-root files so the new structure cannot silently regress while existing violations are being migrated.

**Acceptance Criteria:**
- [ ] The repository adds a lint or validation step that inspects each direct child of `ui/src/features` and fails when any file exists directly under a feature root that is not explicitly allowlisted.
- [ ] The validation treats all file types as violations, including source files, stories, tests, markdown files, and `index.ts`.
- [ ] The initial rollout supports a temporary allowlist for current violations so existing debt can be migrated incrementally without blocking unrelated work.
- [ ] Validation output identifies the exact violating path and distinguishes between new hard-fail violations and allowlisted legacy exceptions.
- [ ] Repository contributors can run the validation locally through the existing frontend lint or validation workflow without needing ad hoc commands.
- [ ] Typecheck passes.
- [ ] Tests pass.

### US-003: Define the public entrypoint pattern without feature-root files
**Description:** As a maintainer, I want an approved way to expose feature public APIs without placing entrypoint files at the feature root so that import intent stays clear while the root remains file-free.

**Acceptance Criteria:**
- [ ] The governance model defines one approved pattern for feature public exports that does not require files at `ui/src/features/<feature>/`.
- [ ] The approved pattern supports intentional public imports without requiring consuming code to guess arbitrary deep paths.
- [ ] The approved pattern distinguishes public surface modules from internal-only modules clearly enough that reviewers can tell whether a new file is part of the supported feature API.
- [ ] Migration guidance explains how to move existing root `index.ts` behavior into the approved public-entrypoint pattern without changing customer-visible behavior.
- [ ] Typecheck passes.
- [ ] Tests pass.

### US-004: Split oversized features by behavior-owned responsibility
**Description:** As a maintainer, I want oversized features to split into smaller sibling features when they represent multiple responsibilities so that each feature remains understandable and reviewable.

**Acceptance Criteria:**
- [ ] The governance model states that when one feature contains multiple distinct user-facing responsibilities, the default choice is to split it into multiple sibling features under `ui/src/features/`.
- [ ] Migration guidance includes criteria for when to split into sibling features versus when to keep one feature and add named internal subdirectories.
- [ ] The criteria favor behavior-owned feature boundaries over purely technical splits such as “all helpers in one place.”
- [ ] The guidance includes at least one representative path for a complex existing feature such as `current-selection` showing how shared selection state, workstation details, provider-session details, dispatch history, and terminal-work-related behavior could be separated or retained intentionally.
- [ ] Typecheck passes.
- [ ] Tests pass.

### US-005: Migrate existing feature directories without changing website behavior
**Description:** As a customer, I want the dashboard and related website surfaces to keep working the same way while maintainers reorganize feature directories so that cleanup does not introduce regressions.

**Acceptance Criteria:**
- [ ] Each migrated feature moves root files into approved subdirectories or into new sibling features without changing the visible behavior of the affected UI.
- [ ] Migrated features preserve explicit loading, empty, error, and success states on the affected website surfaces.
- [ ] Migrated features preserve accessibility semantics, keyboard interaction, and supported mobile and desktop behavior after import paths and module locations change.
- [ ] Existing stories and automated tests move with the modules they verify or are updated to resolve the new structure without losing coverage of the intended behavior.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

### US-006: Shrink and eventually remove the feature-root allowlist
**Description:** As a reviewer, I want visible proof that the temporary allowlist is shrinking over time so that the no-root-files rule becomes real repository behavior instead of permanent exception debt.

**Acceptance Criteria:**
- [ ] The repository records current allowlisted feature-root files in one deliberate place used by the validation step.
- [ ] Migration slices remove allowlist entries as corresponding root files are relocated.
- [ ] Validation fails if an allowlist entry points to a file that no longer exists, preventing stale exceptions from accumulating.
- [ ] The final completion state for this initiative is a passing validation run with an empty allowlist and no files directly under any `ui/src/features/<feature>/` directory.
- [ ] Typecheck passes.
- [ ] Tests pass.

## High-Level Technical Design

- Define a repository-owned feature contract for `ui/src/features/<feature>/`:
  - feature root contains directories only
  - common ownership buckets include `components/`, `hooks/`, `messages/`, `state/`, `selectors/`, and `lib/`
  - features may add a well-named domain folder when it communicates ownership more clearly than a generic bucket
- Define one approved public-surface pattern that keeps feature entrypoints out of the feature root. The exact directory name may be chosen during implementation, but it must be consistent and intentional across features.
- Add a lint or validation rule to scan direct children of `ui/src/features/*` and fail on files, with a temporary allowlist for currently known exceptions.
- Make the validation part of the normal frontend quality path so new violations are blocked by default.
- Migrate existing features in bounded slices:
  - simpler features can move root files into subdirectories within the same feature
  - broader features should be evaluated for splitting into multiple sibling features where that produces clearer ownership
- Keep all migrations behavior-preserving. The primary proof should be stable UI behavior, accessibility, responsive behavior, and automated coverage, not only directory diffs.

## Functional Requirements

1. FR-1: The repository must define a canonical directory-governance contract for all feature directories under `ui/src/features`.
2. FR-2: A feature root directory under `ui/src/features/<feature>/` must not contain any files directly.
3. FR-3: The no-root-files rule must apply to all file types, including `index.ts`, tests, stories, and documentation files.
4. FR-4: The repository must define approved internal ownership buckets for feature code, including `components/`, `hooks/`, `messages/`, `state/`, `selectors/`, and `lib/`.
5. FR-5: The repository must allow well-named domain-specific folders inside a feature when those names describe ownership more accurately than a generic helper bucket.
6. FR-6: The repository must define one approved public-entrypoint pattern for features that does not place files directly at the feature root.
7. FR-7: The repository must add automated validation that fails on feature-root files unless the file is temporarily allowlisted.
8. FR-8: The validation must report violating paths clearly and must distinguish allowlisted debt from newly introduced failures.
9. FR-9: The allowlist must be stored intentionally, must shrink as migrations land, and must fail when it contains stale paths.
10. FR-10: Migration guidance must default toward splitting oversized features into multiple sibling features when that produces clearer behavior-owned boundaries.
11. FR-11: The migration must preserve current customer-visible website behavior, including loading, empty, error, and success states for affected features.
12. FR-12: The migration must preserve accessibility and responsive behavior for affected website surfaces.

## Non-Goals

- No redesign of dashboard visuals, copy, or interaction flows as part of this initiative.
- No backend, CLI, API schema, or generated client changes unless a frontend import adjustment requires a small compatibility touch.
- No requirement that every feature use every standard subdirectory; features should only add subdirectories they actually need.
- No generic `helpers/` convention as the primary solution for unclear ownership.
- No broad cross-feature consolidation of unrelated domain logic unless that code is truly shared and promoted into an appropriate shared module.

## Supporting Technical and UX Considerations

- This work is architecture-focused, but frontend standards still apply because module moves can easily break loading, empty, error, success, accessibility, or responsive behavior if they are not tested carefully.
- The no-root-files rule is intentionally stricter than many common frontend conventions because the goal is to eliminate the feature-root catch-all pattern completely.
- Because `index.ts` is disallowed at the feature root, the chosen public-entrypoint pattern needs to be simple enough that contributors will actually follow it.
- Allowlist-based rollout should create pressure toward cleanup without forcing one giant migration that is risky to review.
- Reviewers should judge migration slices by preserved behavior and clearer ownership, not by raw file movement volume.

## Success Metrics

- Maintainers can inspect any directory under `ui/src/features` and immediately understand that the feature root contains directories only.
- New feature-root files are blocked automatically by validation rather than being caught manually in review.
- Oversized features are split or internally clarified using behavior-owned boundaries instead of accumulating miscellaneous root modules.
- The allowlist shrinks steadily and reaches zero without customer-visible regressions in the affected website surfaces.

## Open Questions

- Which directory name should be the canonical location for public feature entrypoints, for example `public/`, `exports/`, or another approved alternative?
- Should the no-root-files validation live inside the existing frontend lint command, a dedicated repository validation script, or both?
