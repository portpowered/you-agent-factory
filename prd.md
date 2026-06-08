# PRD: Website Structure Separation Lints

## Context

### Project Overview

`infinite-you` is an AI agent factory for scheduling and orchestrating many AI workers across backend, CLI, API, and website surfaces. This work item targets the React website under `ui/src`, where the customer wants the UI refactor direction in [docs/temp/refactoring-ui.md](/Users/abdifamily/infinite-you/docs/temp/refactoring-ui.md) turned into enforceable lint and guard behavior.

### Customer Ask

Enforce website lint rules that push the UI toward the intended refactor shape:

1. `ui/src/components/ui` should contain styling-oriented shared UI primitives and approved shared presentation wrappers only.
2. Feature directories should be constrained to at most `10` files per folder.
3. Feature structure should be linted so feature-owned code conforms to the shape described in the refactor notes, with clear separation between shared UI primitives and feature-owned behavior.

### Problem

The intended website structure already exists in standards and in parts of the codebase, but contributors can still add code that blurs the shared-UI and feature boundary. Shared primitives can drift toward feature-specific behavior, feature directories can accumulate too many files in one folder, and deep feature shapes can diverge from the refactor direction without a repository-owned enforcement path.

That drift has concrete product impact even when it is not a new customer-visible feature. It makes UI behavior harder to reason about, increases review difficulty, encourages duplicated styling and interaction shells, and raises regression risk when loading, empty, error, success, keyboard, and responsive behavior need to stay consistent across many feature surfaces.

Several feature directories already exceed the requested `10 files per folder` limit by a large margin, so this work needs a staged enforcement path that blocks new drift immediately and ratchets existing debt down in reviewable slices instead of pretending the repository can flip to a hard global cap in one unsafe change.

### High-Level Solution

Use the repository's existing website guard pattern to add and extend static checks in `ui/scripts` and wire them into the normal `lint` and `check` flows. The enforcement lane should cover three structural contracts:

1. Shared UI ownership: `ui/src/components/ui` stays limited to shared styling primitives, shared presentational wrappers, tokens, and narrow semantic-button style wrappers rather than feature-owned state, network, or workflow logic.
2. Feature topology: `ui/src/features/<feature>/` and its subdirectories follow an approved shape with intentional `public/` boundaries and no leakage of feature internals into the shared UI layer.
3. Folder size: feature-owned folders are capped at `10` files, introduced first as a non-regression and debt-reporting guard, then tightened as the largest hotspots are split into smaller reviewable slices.

## Project-Level Acceptance Criteria

- The website has one repo-owned structural enforcement path that maps the refactor intent in `docs/temp/refactoring-ui.md` onto the actual `ui/src/components/ui` and `ui/src/features` layout.
- `ui/src/components/ui` rejects feature-owned imports, feature-specific state or network logic, and other violations that would make the shared UI layer more than a styling or shared presentation lane.
- Feature structure enforcement rejects disallowed root-level files, disallowed directory shapes, and feature-boundary violations that bypass intentional `public/` ownership where the rule applies.
- The `10 files per folder` policy is enforced through a staged guard that blocks new debt immediately, reports existing oversized folders clearly, and supports ratcheting the current debt down in bounded follow-up stories.
- The largest in-scope feature hotspots moved under the new size and shape rules preserve current browser-visible loading, empty, error, success, keyboard, and responsive behavior.
- Guard failures report the violating path and the reason clearly enough for contributors to correct the structure without reading implementation internals.
- Quality gate: frontend typecheck, lint, and relevant automated tests pass with the final enabled enforcement state.

## Goals

- Turn the UI refactor direction into enforceable website guard behavior.
- Keep `ui/src/components/ui` focused on shared primitives, tokens, and shared presentation shells.
- Keep feature-owned behavior in `ui/src/features` with intentional boundaries and predictable folder shapes.
- Stop new folder-size debt from entering the website codebase and create a safe path to remove current debt.
- Preserve current customer-visible website behavior while structural cleanup lands.
- Provide reviewer-verifiable enforcement and failure output through the normal frontend quality workflow.

## User Stories

### website-structure-separation-lints-001: Enforce shared UI as a styling and shared presentation lane

**Description:** As a website contributor, I want `ui/src/components/ui` to reject feature-owned behavior so that shared primitives stay reusable, style-consistent, and independent from feature workflows.

**Acceptance Criteria:**
- [x] The repository adds or extends a guard that fails when production files under `ui/src/components/ui` import from `ui/src/features`, feature-owned state containers, or feature-owned network modules.
- [x] The guard fails when shared UI files own feature-specific workflow logic, feature-specific copy, or customer-flow orchestration that belongs in a feature layer instead of a shared primitive or shared wrapper.
- [x] Approved shared primitive owners such as tokens, style variants, presentational wrappers, semantic-button style wrappers, tests, and stories remain allowed.
- [x] Guard output identifies the violating file and explains that `ui/src/components/ui` is reserved for shared styling primitives and shared presentation owners.
- [x] The guard runs through the normal website `lint` and `check` workflow rather than a one-off command only.
- [x] Typecheck passes.
- [x] Tests pass.

### website-structure-separation-lints-002: Enforce feature shape and intentional boundaries

**Description:** As a maintainer, I want feature-owned code to follow one approved shape so that contributors can add UI behavior without bypassing shared primitives or leaking feature internals across the website.

**Acceptance Criteria:**
- [ ] The repository enforces that each `ui/src/features/<feature>/` root contains approved subdirectories only and does not accumulate new root-level implementation files.
- [ ] The repository enforces approved feature-owned lanes such as `components/`, `hooks/`, `lib/`, `messages/`, `state/`, `public/`, or a narrower domain folder when the feature has a justified internal subdivision.
- [ ] Cross-feature imports that are covered by the rule fail when they reach into another feature's internals instead of using that feature's intentional `public/` boundary.
- [ ] Guard output identifies the violating path and states which approved feature boundary or subdirectory shape was expected.
- [ ] The enforcement integrates with the existing feature-root and website structure guard workflow instead of creating a disconnected second system.
- [ ] Typecheck passes.
- [ ] Tests pass.

### website-structure-separation-lints-003: Add a staged file-count guard for feature folders

**Description:** As a reviewer, I want oversized feature folders reported and blocked from growing so that the website can ratchet toward smaller, easier-to-review feature slices without unsafe repo-wide churn.

**Acceptance Criteria:**
- [ ] The repository adds a guard that measures file counts for feature-owned folders under `ui/src/features` and fails when a non-allowlisted folder exceeds `10` files.
- [ ] Existing oversized folders are reported as named legacy debt when they are temporarily allowlisted, and the report includes the observed file count for each folder.
- [ ] The guard fails if an allowlisted folder grows, if a new oversized folder appears, or if an allowlist entry becomes stale after the folder is reduced or reshaped.
- [ ] Guard output makes the ratchet explicit by telling contributors to split the folder into smaller approved subdirectories and remove allowlist debt in the same change when possible.
- [ ] The file-count guard runs from the normal website `lint` and `check` commands.
- [ ] Typecheck passes.
- [ ] Tests pass.

### website-structure-separation-lints-004: Make the graph and dashboard structure pass the new guards

**Description:** As a customer using the dashboard and graph editing surfaces, I want the initial high-debt website hotspots cleaned up to fit the enforced structure without changing how the product behaves.

**Acceptance Criteria:**
- [ ] The initial oversized graph and dashboard hotspots selected for this lane no longer rely on file-count debt growth or shared-UI boundary violations to pass the enabled guards.
- [ ] The affected dashboard and graph-editor surfaces continue to render explicit loading, empty, error, and success states where those states already exist.
- [ ] The affected surfaces preserve keyboard reachability, visible focus, and responsive usability on supported mobile, tablet, and desktop widths.
- [ ] Existing customer-visible actions, dialogs, toggles, and disclosures in the affected surfaces preserve their current observable behavior after the structural split.
- [ ] The implementation stays within the selected graph and dashboard hotspots and does not widen into unrelated website cleanup.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

### website-structure-separation-lints-005: Make current-selection structure pass the new guards

**Description:** As a customer using current-selection detail flows, I want the largest current-selection hotspots reshaped into smaller feature slices so that future changes are safer without regressing the existing interaction model.

**Acceptance Criteria:**
- [ ] The selected oversized current-selection folders no longer exceed the staged file-count policy without an explicit shrinking debt path.
- [ ] The affected current-selection flows preserve their current loading, empty, error, and success behavior for detail panels and editable sections.
- [ ] Save, retry, expand, and detail-view interactions in the affected current-selection flows preserve current keyboard behavior, focus movement, and disabled or pending treatment.
- [ ] Any feature-boundary cleanup required for the selected current-selection hotspots continues to route shared primitives through `ui/src/components/ui` and feature-owned behavior through approved `ui/src/features` subdirectories.
- [ ] The implementation stays bounded to the selected current-selection hotspots rather than broad cleanup across unrelated features.
- [ ] Typecheck passes.
- [ ] Tests pass.
- [ ] Verify in browser using dev-browser skill.

## High-Level Technical Design

- Extend the existing website structural guard approach already used in `ui/scripts`, rather than introducing a second linting mechanism with different conventions.
- Treat `ui/src/components/ui` as the canonical shared primitive and shared presentation layer. Shared files in that lane may own tokens, styling variants, presentational wrappers, and narrow reusable semantic-button shells, but not feature workflows, feature stores, feature-local API hooks, or customer-flow orchestration.
- Treat `ui/src/features` as the canonical feature-owned behavior layer. Feature roots should stay directory-only, feature internals should live under approved subdirectories, and intentional cross-feature reuse should travel through `public/` when the boundary is enforced.
- Implement the `10 files per folder` rule as a ratcheting guard with explicit legacy-debt reporting because the current repository already has multiple folders far beyond the target. The first enforcement state should prevent new growth and stale waivers; follow-up stories should remove waivers by splitting the largest hotspots.
- Keep verification behavior-first even for structural work. When a cleanup story touches browser-visible surfaces, the evidence should prove loading, empty, error, success, keyboard, focus, and responsive behavior still work rather than only proving that files moved.

## Functional Requirements

1. FR-1: The website must provide one repo-owned enforcement path for the UI structure direction described in `docs/temp/refactoring-ui.md`.
2. FR-2: `ui/src/components/ui` must reject feature-owned imports and feature workflow ownership that do not belong in a shared primitive or shared presentation wrapper.
3. FR-3: The shared UI lane must still allow shared tokens, shared style variants, shared presentational wrappers, narrow reusable semantic-button wrappers, tests, and stories.
4. FR-4: `ui/src/features/<feature>/` roots must stay directory-only and use approved subdirectories for implementation files.
5. FR-5: Feature-owned code must follow intentional boundaries, including `public/` import boundaries where the repository chooses to enforce them.
6. FR-6: Feature-owned folders must be constrained to at most `10` files per folder through a staged guard that blocks new debt and reports legacy debt clearly.
7. FR-7: The file-count guard must fail on new oversized folders, growth in allowlisted oversized folders, and stale allowlist entries after a folder is reduced.
8. FR-8: Guard output must identify the violating path and explain the expected structural rule in reviewer-verifiable language.
9. FR-9: Cleanup required to satisfy the new guards must preserve existing loading, empty, error, success, keyboard, focus, and responsive behavior on touched website surfaces.
10. FR-10: All enforcement must run through the normal frontend `lint` and `check` workflow used in CI and local review.

## Non-Goals

- A broad website redesign or visual refresh.
- Rewriting the entire website to the target structure in one change.
- Moving all shared UI code out of `ui/src/components/ui` when the file already fits the shared primitive or shared presentation lane.
- Changing backend, CLI, API contracts, or persisted behavior.
- Enforcing arbitrary folder counts outside the scoped website feature directories.
- Bundling unrelated cleanup that is not required to make the new structure guards trustworthy.

## Supporting Technical and UX Considerations

- The customer request refers to `components/ui` and `components/features`; in this repository the actual enforcement targets are `ui/src/components/ui` and `ui/src/features`.
- The existing website standards already require explicit loading, empty, error, and success states and clear feature boundaries. The new guards should codify those boundaries without weakening the current standards.
- The current website already has structure guards such as feature-root, button-usage, dashboard-expand, spacing-token, and inline-class checks. This work should follow that model for consistency of output and maintenance.
- The file-count cap must be introduced carefully because several current hotspots exceed it substantially. The enforcement should prevent new debt first, then drive bounded shrinkage through specific cleanup stories.
- Shared UI enforcement should stay narrow enough to avoid false positives on legitimate reusable shells while still rejecting feature-specific ownership drift.

## Success Metrics

- New website code cannot silently reintroduce shared-UI and feature-boundary drift in guarded paths.
- Oversized feature folders stop growing, and the named debt list trends downward over follow-up cleanup stories.
- Reviewers can tell from lint output why a structure violation failed and what boundary needs to be restored.
- Touched dashboard, graph-editor, and current-selection flows preserve their existing browser-visible behavior while the structure becomes easier to reason about.

## Open Questions

None for planning. This PRD assumes the implementation will use the existing website guard pattern and stage the folder-count cap because the repository already contains oversized folders.
