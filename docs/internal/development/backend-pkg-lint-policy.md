# Backend `pkg/` Lint Policy

This document defines the repository-owned phase-1 Go lint policy for backend packages under `pkg/`.

## Scope

- The owned command path is `go run ./cmd/pkglintcheck`.
- The root lint workflow path is `make lint`, which delegates backend `pkg/` linting to `make pkg-lint`.
- `make pkg-lint` runs the same owned command, `go run ./cmd/pkglintcheck`.
- The command always runs `golangci-lint` against `./pkg/...`.
- The checked-in config lives at `.golangci.pkg.yml`.

This keeps the first blocking rollout scoped to maintained backend packages in `pkg/` instead of relying on reviewer convention.

## Phase-1 Blocking Linters

Phase 1 enables only high-signal correctness analyzers. The initial owned rule set is:

- `govet`: catches likely logic mistakes, suspicious API misuse, and other compiler-adjacent correctness issues that should block backend changes.
- `staticcheck`: catches high-confidence API misuse, nil-context mistakes, empty branches, and other correctness bugs that compile cleanly.
- `errcheck`: catches ignored returned errors so tests and runtime cleanup paths do not silently discard failures.
- `ineffassign`: catches assignments whose values are never observed, which often means a logic branch is not doing what it claims.
- `nilerr`: catches branches that build a real error and then accidentally return `nil`, which would hide failure from callers.
- `errorlint`: catches wrapped-error comparisons, assertions, and formatting mistakes so sentinel and typed errors stay observable after context is added.
- `bodyclose`: catches HTTP response paths that forget to close response bodies.
- `contextcheck`: catches backend calls that drop inherited request/runtime context instead of threading it through to downstream work.

## Phase-1 Exceptions

Phase 1 supports incremental adoption through explicit, narrow exceptions recorded in `.golangci.pkg.yml`. Do not widen those exceptions casually:

- Prefer fixing the code before adding any exclusion.
- When a real exclusion is still necessary, keep it scoped to one generated package, one file, or one rule on one file.
- Every new exclusion must carry written justification in this policy or the adjacent config comments so reviewers can see why the exception exists and why a narrower fix would not work yet.
- Do not disable a linter for the entire `pkg/` tree when a package-, file-, or rule-scoped exception would address the actual rollout blocker.

Phase 1 currently uses these explicit exceptions:

- `pkg/api/generated/`: exclude the generated OpenAPI backend package from phase-1 findings. This keeps reviewer attention on maintained handwritten `pkg/` code while avoiding churn on generated artifacts that are refreshed from source contracts.
- `contextcheck` is enabled as a blocking analyzer, but phase 1 excludes two narrow files:

  - `pkg/factory/runtime/factory.go`
  - `pkg/service/factory.go`

Those files contain runtime bootstrap and hot-swap orchestration helpers that derive or store long-lived contexts for worker pools and sidecars. On the repository's current call shapes, `contextcheck` reports low-signal findings against those wrappers even when they are already deriving from the inherited runtime context. Keep the exclusions narrow, and prefer real fixes over expanding them when future changes touch ordinary request or execution flow.

This rollout strategy is intentionally conservative. It does not require a one-time cleanup of every historical phase-1 finding before ordinary backend work can merge. Instead, maintainers should use explicit, reviewable exceptions only where historical code or generated artifacts would otherwise block adoption, then shrink those exceptions as real fixes land.

## Verification

- Clean pass path: run `go run ./cmd/pkglintcheck`.
- Root workflow path: run `make lint` to execute UI lint, UI deadcode, the owned backend `pkg` lint lane, and the backend deadcode baseline gate in the same order CI uses for repository linting.
- Intentional failure path: temporarily add an ignored error inside a `pkg/` test or helper, rerun `go run ./cmd/pkglintcheck`, confirm `errcheck` fails, then remove the seed before normal verification.

## Workflow Integration

The repository standardizes `govet` through `golangci-lint` for the first backend lint wave instead of running a separate `go vet ./...` step in the root lint workflow. That keeps the `govet` configuration, package scope, and future backend analyzer additions behind one repo-owned command surface while phase 1 remains intentionally limited to `./pkg/...`.

## Phase-1 Non-Goals

Phase 1 does not block on stylistic or code-shape rules such as naming preferences, comment-style requirements, function length, file length, or complexity thresholds.

This boundary is intentional. The first blocking rollout should catch likely bugs without forcing repository-wide cleanup of legacy code shape before ordinary backend work can merge. Backend standards still prefer smaller modules and simpler code, but phase 1 defers those maintainability gates until the repo has evidence about analyzer noise, false positives, and reviewer value.

## Deferred Maintainability Candidates

The following analyzers are intentionally deferred to later phases instead of blocking phase-1 CI:

- Selective `gocritic`: useful for maintainability and readability cleanups, but broad defaults can produce high review churn until the team narrows which checks are valuable in this codebase.
- Selective `revive`: useful for documentation and readability policy, but many rules are style-oriented or require repository-specific tuning before they should block backend delivery.
- `funlen`: useful for highlighting oversized functions, but promoting it too early would force broad shape cleanup across historical code rather than catching correctness defects first.
- `gocyclo` or `gocognit`: useful for flagging complex control flow, but they need evidence-backed thresholds and a clear exception policy before they become blocking signals.

Later phases can widen beyond `pkg/` to `cmd/`, `internal/`, and relevant backend tests after the phase-1 correctness lane proves stable and the exception policy is understood.

Promotion of later-phase maintainability analyzers to blocking CI should be based on post-rollout evidence about signal quality, false positives, and reviewer value in this repository. Backend standards can justify evaluating those analyzers, but standards text alone is not enough reason to make them blocking without repository-specific rollout evidence.
