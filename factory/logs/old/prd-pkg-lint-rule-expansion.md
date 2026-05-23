# PRD: `pkg/` Lint Rule Expansion

## Introduction

Expand backend lint enforcement for the Go packages under `pkg/` by introducing a curated `golangci-lint` configuration focused on high-signal bug prevention first, then broader maintainability rules in later phases. Today the repository's root `make lint` lane runs UI linting, UI dead-code checks, `go vet ./...`, and the pinned Go deadcode analyzer, but it does not yet enforce a broader set of Go-specific correctness rules across the backend package tree.

This feature closes that gap by defining a practical first-wave lint policy for `pkg/`, integrating it into the repository-owned lint workflow, and documenting how the project should evolve the ruleset without creating review churn or forcing a one-time mass refactor. Phase 1 should be intentionally conservative: only rules with strong bug-finding value should block CI for `pkg/`.

## Goals

- Catch correctness bugs in `pkg/` earlier than code review or runtime tests.
- Establish a repository-owned `golangci-lint` policy that complements existing `go vet` and deadcode checks.
- Keep phase 1 limited to high-signal rules so CI failures are actionable and low-noise.
- Scope initial enforcement to `pkg/` while leaving room for a later backend-wide rollout.
- Document a phased adoption strategy that avoids a mandatory mass cleanup of all historical issues.

## User Stories

### US-001: Add a repository-owned Go lint configuration for `pkg/`
**Description:** As a maintainer, I want a checked-in lint configuration for `pkg/` so that backend code quality is enforced consistently in local development and CI.

**Acceptance Criteria:**
- [ ] A checked-in `golangci-lint` configuration exists in the repository root.
- [ ] The initial configuration scopes enforcement to `pkg/` in phase 1.
- [ ] The configuration documents which linters are enabled and why they were chosen.
- [ ] The configuration avoids enabling broad low-signal stylistic rules by default in phase 1.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-002: Enforce a high-signal correctness ruleset in phase 1
**Description:** As a backend contributor, I want phase-1 lint failures to point to likely bugs so that fixing them improves reliability rather than creating style-only churn.

**Acceptance Criteria:**
- [ ] Phase 1 enables `govet` through `golangci-lint` or an equivalent integrated path.
- [ ] Phase 1 enables `staticcheck`.
- [ ] Phase 1 enables `errcheck`.
- [ ] Phase 1 enables `ineffassign`.
- [ ] Phase 1 enables `nilerr`.
- [ ] Phase 1 enables `errorlint`.
- [ ] Phase 1 enables `contextcheck` or an equivalent rule that catches missing context propagation in supported call paths.
- [ ] Phase 1 enables `bodyclose` for HTTP response handling where applicable.
- [ ] The PRD or implementation notes classify any additional phase-1 linter as high-signal and bug-oriented.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-003: Keep maintainability and style-heavy rules out of the first blocking wave
**Description:** As a maintainer, I want the first rollout to avoid broad style churn so that contributors trust the lint lane and can adopt it incrementally.

**Acceptance Criteria:**
- [ ] Phase 1 does not block CI on file-length limits, function-length limits, cyclomatic complexity caps, or broad comment-style rules.
- [ ] The first blocking lane does not require repository-wide rewrites of pre-existing code shape issues.
- [ ] Maintainability-focused rules such as `gocyclo`, `funlen`, `gocognit`, selective `revive`, or selective `gocritic` checks are explicitly recorded as later-phase candidates instead of phase-1 blockers.
- [ ] The rollout plan explains why these rules are deferred even though repository standards prefer smaller functions and modules.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-004: Integrate the new lint lane into repository workflows
**Description:** As a contributor, I want the new Go lint checks to run in the normal project commands so that local verification matches CI expectations.

**Acceptance Criteria:**
- [ ] The repository's root lint workflow calls the new Go lint lane in a stable, documented way.
- [ ] The root workflow continues to preserve existing UI lint and deadcode checks.
- [ ] The new lint lane is runnable locally with a single documented command.
- [ ] CI documentation explains whether `go vet` remains separate, is replaced by `golangci-lint`'s `govet`, or both run intentionally.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-005: Support incremental rollout without mass refactoring
**Description:** As a maintainer, I want an adoption strategy that lets us improve quality without stopping unrelated backend work to fix every historical warning at once.

**Acceptance Criteria:**
- [ ] The rollout strategy does not require a one-time repository-wide cleanup of all existing backend lint findings before merge.
- [ ] The rollout strategy defines how existing violations are handled, such as targeted suppressions, narrow excludes, phased package adoption, or a tracked baseline process.
- [ ] New suppressions require justification and are documented as exceptions rather than defaults.
- [ ] The rollout guidance avoids broad blanket disables for entire package trees when a narrower exception would work.
- [ ] Typecheck, lint, and relevant backend tests pass.

### US-006: Document the recommended linter roadmap for later phases
**Description:** As a maintainer, I want a written roadmap for future backend lint tightening so that we can expand quality checks deliberately after phase 1 proves stable.

**Acceptance Criteria:**
- [ ] Documentation lists the phase-1 blocking linters and their purpose.
- [ ] Documentation lists later-phase candidates for maintainability and readability enforcement.
- [ ] Later-phase candidates include at least selective `gocritic`, selective `revive`, and one code-shape rule such as `funlen`, `gocyclo`, or `gocognit`.
- [ ] Documentation explains that future backend-wide rollout may extend beyond `pkg/` to `cmd/`, `internal/`, and relevant tests.
- [ ] Typecheck, lint, and relevant backend tests pass.

## Functional Requirements

1. FR-1: The system must add a repository-owned `golangci-lint` configuration for Go backend linting.
2. FR-2: Phase 1 must scope blocking enforcement to the `pkg/` directory.
3. FR-3: The phase-1 blocking ruleset must prioritize bug-finding linters over style or formatting-only linters.
4. FR-4: The phase-1 ruleset must include `staticcheck`, `errcheck`, `ineffassign`, `nilerr`, and `errorlint`.
5. FR-5: The phase-1 ruleset must include `govet` through the selected execution path and document whether standalone `go vet` still runs separately.
6. FR-6: The phase-1 ruleset must include `contextcheck` or an equivalent context-propagation analyzer if it operates correctly on the repository's supported Go version and call patterns.
7. FR-7: The phase-1 ruleset must include `bodyclose` where applicable to catch unclosed HTTP response bodies.
8. FR-8: The implementation must avoid enabling broad maintainability or stylistic rules as phase-1 CI blockers unless they are shown to be low-noise and high-signal in this codebase.
9. FR-9: The root lint workflow must invoke the new Go lint lane in addition to the existing repository-owned UI lint and deadcode checks.
10. FR-10: The feature must define a documented local command for contributors to run the Go lint lane before review.
11. FR-11: The rollout must define a strategy for handling pre-existing violations without requiring an immediate repository-wide cleanup.
12. FR-12: New suppressions or excludes must be narrow, justified, and documented in the implementation guidance.
13. FR-13: Documentation must explain the purpose of each enabled phase-1 linter in plain language suitable for junior contributors and AI agents.
14. FR-14: Documentation must record later-phase candidates such as selective `gocritic`, selective `revive`, `funlen`, `gocyclo`, `gocognit`, or file-size rules as non-blocking future work unless promoted by a later decision.
15. FR-15: The design must allow future extension from `pkg/` to broader backend surfaces such as `cmd/`, `internal/`, and relevant backend tests.

## Non-Goals

- Frontend, UI, or Biome linting changes.
- A mandatory repository-wide cleanup of all existing Go lint findings before phase 1 can merge.
- Broad style normalization work unrelated to correctness, such as comment rewording or large-scale function splitting across untouched packages.
- Custom in-house analyzer development in phase 1.
- Immediate blocking enforcement for function length, file length, cyclomatic complexity, or naming-style rules.
- Replacing the repository's existing deadcode gate.

## Design Considerations

- The first blocking ruleset should feel trustworthy to contributors: each failure should likely correspond to a real bug, leak, ignored error, or risky contract mistake.
- The lint output should be understandable by maintainers who are less familiar with specific analyzers, so rule documentation matters.
- The rollout should match the repository's preference for mechanical enforcement from standards rather than relying on reviewer memory alone.
- The phase boundary should be explicit: correctness rules now, maintainability tightening later after the codebase absorbs the first wave.

## Technical Considerations

- The repository currently runs `go vet ./...` and a separate backend deadcode analyzer through `make lint`; the new lane must fit that workflow cleanly.
- `golangci-lint` should be pinned or otherwise version-controlled enough that CI and local results remain stable across contributors.
- The implementation should decide whether to keep standalone `go vet` for clarity and redundancy or fold vet into the `golangci-lint` execution path.
- Some analyzers may need careful exclusion rules for generated code, test fixtures, or intentionally unusual runtime seams, but those exclusions should stay narrow.
- Phase-1 high-signal candidates for `pkg/` are:
  - `staticcheck` for general correctness and API misuse.
  - `errcheck` for ignored errors.
  - `ineffassign` for ineffective assignments.
  - `nilerr` for nil/non-nil mismatch bugs in error returns.
  - `errorlint` for incorrect error wrapping and comparison patterns.
  - `contextcheck` for missing or dropped context propagation where applicable.
  - `bodyclose` for unclosed HTTP response bodies.
  - `govet` for core compiler-adjacent checks and suspicious constructs.
- Later-phase candidates are:
  - selective `gocritic` checks for suspicious code patterns.
  - selective `revive` rules for naming, comments, and consistency only after noise is understood.
  - `funlen`, `gocyclo`, or `gocognit` for maintainability once the team chooses thresholds and exception policy.
- Verification should cover both clean-pass behavior and intentional-failure behavior so maintainers can trust the lane.

## Success Metrics

- New backend bugs caused by ignored errors, ineffective assignments, bad error handling, or missed context propagation are caught by CI before merge more often than today.
- Contributors can explain why a phase-1 lint failure matters without needing reviewer tribal knowledge.
- The initial rollout lands without forcing a broad repository pause for lint-only cleanup.
- The repository gains a documented path to extend lint enforcement from `pkg/` to broader backend surfaces later.

## Open Questions

- Should `go vet ./...` remain a separate root-lane command after `golangci-lint` adoption, or should the repository standardize on one entrypoint that includes `govet`?
-> stnadardize
- Should phase 1 use a package-by-package allowlist inside `pkg/` if a full-tree rollout still produces too much initial noise?
-> yes
- Should the repository pin `golangci-lint` through a checked-in installer path, a containerized version, or a documented developer dependency version?
-> checked-in
- Which generated, fixture, or replay-contract files under `pkg/` need narrow exclusions, if any?
-> any generated fixtures should be excluded
- What evidence threshold should be required before promoting maintainability rules like `funlen` or `gocyclo` to blocking CI checks?
-> we should have some idea afterwards. 