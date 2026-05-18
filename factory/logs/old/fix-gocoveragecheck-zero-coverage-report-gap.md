# fix-gocoveragecheck-zero-coverage-report-gap

## Why

The repo-owned backend coverage gate is still overstating quality on live
`main`.

Current evidence from `2026-05-09`:

- running `go run ./cmd/gocoveragecheck -min 80 -timeout 300s` exits
  successfully with:
  `Go coverage 86.6% meets minimum 80.0%`
- the same command output also prints backend-owned packages at `0.0%`:
  - `github.com/portpowered/infinite-you/pkg/apisurface`
  - `github.com/portpowered/infinite-you/pkg/buffers`
  - `github.com/portpowered/infinite-you/pkg/cli/default`
- this means the zero-coverage gate in `cmd/gocoveragecheck` is still missing
  at least one live report shape or package-detection path and can let
  backend-owned packages pass the customer-facing quality lane with no
  statement coverage

This is a higher-value cleanup than a small local dedupe because it makes the
existing coverage signal truthful before we queue more broad coverage work.

## Do

- reproduce the current false-pass in package-local tests for
  `cmd/gocoveragecheck`
- fix the zero-coverage detection so backend-owned packages reported at
  `0.0% of statements` fail the command even when they are absent from or
  represented unexpectedly in the generated coverage profile
- keep the fix local to `cmd/gocoveragecheck` and its tests
- preserve the existing aggregate-threshold behavior and current public command
  contract
- make the failure message continue to name the offending backend-owned
  packages explicitly

## Constraints

- do not broaden this lane into adding application-package tests for
  `pkg/apisurface`, `pkg/buffers`, or `pkg/cli/default`
- do not raise the aggregate coverage minimum in this change
- prefer fixing the parser or package-reconciliation logic over adding special
  cases for individual packages
- keep verification behavioral at the repo-owned command boundary rather than
  source-layout or inventory-style assertions

## Verification

- run `go test ./cmd/gocoveragecheck`
- run `go run ./cmd/gocoveragecheck -min 80 -timeout 300s` and confirm the
  command now fails when backend-owned packages still print `0.0%`
- if the fix changes the exact set of failing packages, record that through the
  command output rather than by hard-coding assumptions into docs
