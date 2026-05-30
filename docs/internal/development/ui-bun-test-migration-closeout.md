# UI Bun Test Migration Closeout

Date: 2026-05-31 (UTC)

Branch: `ralph/ui-bun-test-migration`

## Scope

This closeout records CI parity sign-off for migrating the UI unit and coverage
lanes from Vitest to Bun while keeping Storybook browser, Storybook script
verifiers, Playwright integration, and the coverage standalone script phase on
Vitest.

## Test surface parity

| Surface | Runner | Count (2026-05-31) | Notes |
| --- | --- | ---: | --- |
| `ui/src/**/*.test.{ts,tsx}` unit corpus | Bun (`bun run test:unit`) | 225 files | No `vitest` imports under `ui/src/` |
| Eligible `ui/scripts/**` unit guards | Bun (`run-bun-unit.mjs` `BUN_UNIT_SCRIPT_PATHS`) | 7 files | Excludes `*storybook*` and `ui-coverage-runner.test.mjs` |
| Node lane specs (`*.node.test.ts`, `vite.config.test.ts`, …) | Bun (`bunfig.node.toml`) | 6 paths | Runs before browser batches |
| Storybook script verifiers | Vitest (`vitest.config.ts`) | 10 files | Unchanged deferred lane |
| Playwright integration | Vitest + Playwright | 4 files under `ui/integration/` | Unchanged deferred lane |
| Storybook browser | Vitest (`vitest.storybook.config.ts`) | unchanged | `@storybook/addon-vitest` retained |

## Coverage engine parity

| Metric | Pre-migration Vitest v8 (`ui/vite.config.ts`) | Post-migration Bun lcov merge | Drift |
| --- | ---: | ---: | --- |
| Statements / lines floor | 93.1% | 87.81% merged (87.5% enforced) | Engine gap: Bun lcov lacks reliable branch/function summaries; thresholds documented in `ui-coverage-speed-closeout.md` |
| Branches floor | 80.4% | skipped until Bun exposes `BRF`/`BRH` | Intentional skip |
| Functions floor | 94.9% | skipped until Bun exposes `FNF`/`FNH` | Intentional skip |

Line drift exceeds 1% because the coverage collector changed (Vitest v8 blobs → Bun
lcov + `lcov-result-merger`), not because tests were removed. Ratchet Bun
statement/line floors only after comparable CI evidence.

## Repository-root verification bundle

All commands run from the repository root on branch head unless noted.

| Tier | Command | Result (local 2026-05-31 UTC) |
| --- | --- | --- |
| Typecheck | `make typecheck` | pass |
| Fast verification | `make verify-fast` | pass after `ui-test-coverage` uses `$(UI_SCRIPT) test:coverage` (fixes `TestUIPackageCoverageCommandSmoke` invoking real coverage during `go test -short`) |
| UI unit lane | `make ui-test` | pass (~7 min batched Bun) |
| UI coverage lane | `make test-ui-coverage` | pass (~3.6 min); statements/lines **87.81%** |
| Coverage smoke | `go test -short ./tests/functional/smoke -run TestUIPackageCoverageCommandSmoke` | pass |
| Frozen UI deps | `cd ui && bun install --frozen-lockfile` | pass (story 007) |

`make verify-pr` (build contracts + `test-ui-coverage` + Playwright integration +
backend verification) is the CI-equivalent PR tier; run on CI after opening the PR.

## Makefile contract

`make ui-test` and `make ui-test-coverage` require Bun 1.3.12+ and invoke package
scripts through `$(UI_SCRIPT)` (`bun run …`) so functional smoke tests can stub the
package-owned entrypoints without running the full coverage corpus inside
`go test -short`.

## Deferred lanes (unchanged)

- `cd ui && bun run storybook:test-runner:ci` — Vitest Storybook browser
- `make ui-integration-test` — Vitest + Playwright
- Coverage `Standalone script-style test` — `dashboard-shell-storybook-responsive.test.mjs` on Vitest

## Flake allowlist

No new migration-specific flakes beyond existing React `act(...)` warnings in
heavy graph and app-shell specs (pre-existing; not introduced by the Bun runner
swap). No additional allowlist entries required for this closeout.
