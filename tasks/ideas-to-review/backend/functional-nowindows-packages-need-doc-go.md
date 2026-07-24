# Functional `!windows` packages need a Windows-visible stub

## Problem

When every `.go` file under a `tests/functional/<domain>/<subsection>/` package
uses `//go:build !windows`, `go test ./tests/functional/...` on Windows fails
with `build constraints exclude all Go files` during package setup. That breaks
the required Windows Go Functional CI lane even though the Make-wrapper proofs
are intentionally POSIX-only.

## Why this recurs

Make-contract and fail-closed smokes correctly use `!windows` because they
shell out to GNU Make + `sh`. Relocating those proofs into approved functional
subsections (away from grandfathered `smoke/`) creates new leaf packages that
often have no untagged file.

## Proposed direction

- Keep the POSIX Make proofs behind `!windows`.
- Always include a tiny untagged `doc.go` (package comment only) in such
  packages so Windows discovery succeeds with `[no test files]`.
- Optionally teach `pkg-structure` or a short Windows smoke to reject
  all-`!windows` functional leaf packages missing a stub.

## Non-goals

- Do not run Make-wrapper smokes on Windows.
- Do not weaken the functional domain/subsection ownership rules.
