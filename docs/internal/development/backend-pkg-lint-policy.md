# Backend `pkg/` Lint Policy

This document defines the repository-owned phase-1 Go lint policy for backend packages under `pkg/`.

## Scope

- The owned command path is `go run ./cmd/pkglintcheck`.
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

`contextcheck` is enabled as a blocking analyzer, but phase 1 excludes two narrow files:

- `pkg/factory/runtime/factory.go`
- `pkg/service/factory.go`

Those files contain runtime bootstrap and hot-swap orchestration helpers that derive or store long-lived contexts for worker pools and sidecars. On the repository's current call shapes, `contextcheck` reports low-signal findings against those wrappers even when they are already deriving from the inherited runtime context. Keep the exclusions narrow, and prefer real fixes over expanding them when future changes touch ordinary request or execution flow.

## Verification

- Clean pass path: run `go run ./cmd/pkglintcheck`.
- Intentional failure path: temporarily add an ignored error inside a `pkg/` test or helper, rerun `go run ./cmd/pkglintcheck`, confirm `errcheck` fails, then remove the seed before normal verification.

## Phase-1 Non-Goals

Phase 1 does not block on stylistic or code-shape rules such as naming preferences, comment-style requirements, function length, file length, or complexity thresholds.

Future stories can widen the correctness analyzer set and later document the maintainability roadmap after the repo-owned command and config path are in place.
