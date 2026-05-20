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

## Phase-1 Non-Goals

Phase 1 does not block on stylistic or code-shape rules such as naming preferences, comment-style requirements, function length, file length, or complexity thresholds.

Future stories can widen the correctness analyzer set and later document the maintainability roadmap after the repo-owned command and config path are in place.
