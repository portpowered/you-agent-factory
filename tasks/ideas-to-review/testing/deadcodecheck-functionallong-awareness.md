# Deadcodecheck should understand `functionallong` test reachability

## Problem

The repository's deadcode gate runs `go run golang.org/x/tools/cmd/deadcode@v0.25.1 -test ./...`
through the default build graph only. As more decomposed functional coverage moves from
runtime `support.SkipLongFunctional(...)` skips into real `//go:build functionallong`
`*_long_test.go` files, helper functions that are still live in the opt-in long lane become
"unreachable" to the default deadcode pass.

That creates large baseline churn whenever a branch correctly moves entire long-only files out
of the short lane, even when no production dead code was added. The churn is especially noisy
for package-local test helpers and a few library helpers that are exercised only by the long
functional lane.

## Why This Matters

- It discourages the preferred migration pattern of moving all-long files into real
  `functionallong` units.
- It makes `docs/development/deadcode-baseline.txt` track build-tag reachability noise instead
  of true accepted dead-code debt.
- Future runtime-target work under `US-010` will likely keep hitting the same baseline churn as
  more short-lane compile weight gets pushed behind `functionallong`.

## Proposed Direction

- Teach `cmd/deadcodecheck` to evaluate both the default test graph and the
  `-tags=functionallong` test graph, then treat a symbol as reachable if either graph uses it.
- Alternatively, maintain separate normalized deadcode reports per lane and diff the union
  against committed baselines.
- Keep the current review rule of deleting truly stale helpers first; this idea is only about
  reducing false-positive drift from intentional long-lane ownership.
