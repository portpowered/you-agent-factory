# C11 no-browser test-processes implementation ledger

## Scope and source authority

- Behavior: `BEH-NB-01` — test-spawned built CLI processes suppress the real
  browser launcher before construction while production defaults and explicit
  injection remain unchanged.
- Implementation story: `functional-test-optimization-c11-no-browser-test-processes-001`.
- Source-plan reference: `Scope 9 — No real browser launch from any test run;
  Acceptance Criterion 8; Non-goals`.
- The PRD references `docs/temp/functional-test-optimization.md`, which is not
  present in this checkout. That absence is recorded as an authority gap; no
  source-plan content was invented or silently repaired.
- Authored production source: `pkg/wire/profiles.go`.
- Authored harness sources: `internal/builtcliacceptance/env.go` and
  `internal/builtcliacceptance/harness.go`.
- Generated Wire output was regenerated with `make wire-smoke`; no generated
  diff remained.

## Configuration and precedence

`YOU_NO_BROWSER_OPEN=1` is an exact-value, process-environment opt-out. Wire
selection order is:

1. an explicit `edges.BrowserOpener`;
2. a no-op opener when the value is exactly `1`;
3. the existing `platformbrowser.NewHost(runtime.GOOS).Open` fallback.

Missing, empty, whitespace, `0`, `true`, and other values retain the real
fallback. The host factory is not evaluated for the exact opt-out.

Every canonical built-child environment removes inherited or extra
`YOU_NO_BROWSER_OPEN` entries and appends exactly one
`YOU_NO_BROWSER_OPEN=1`, while retaining isolated home variables and unrelated
entries.

## Local implementation evidence

Environment: Windows `go1.25.0`, `windows/amd64`, local controlled launcher
sentinel, no remote or paid dependency.

| Evidence | Result | Property proved |
| --- | --- | --- |
| `go test ./pkg/wire -run 'Browser|Opener' -count=1` | PASS | Exact-value selection, injection precedence, lazy non-construction. |
| `go test ./internal/builtcliacceptance/... -count=1` | PASS | Canonical environment propagation and child-runner delivery. |
| `make wire-smoke` | PASS | Wire regeneration, drift check, and package-wide Wire tests. |
| `go test ./tests/integration/transport/cli/process -run 'TestBuiltCLINoBrowserOpen' -count=1 -timeout 5m` | PASS | A real built server reaches readiness, fails on an occupied `--listen` port, recovers, overlaps on isolated ports, and stops without the controlled `rundll32.cmd` marker. |
| `go test ./tests/integration/transport/cli/process -count=1 -timeout 10m` | PASS | Existing built-process signal, pipe, startup, recovery, and cleanup behavior remains green with the canonical environment. |

The integration test uses a temporary fail-closed `rundll32.cmd` at the front
of the child PATH. Its marker was absent after readiness, failure, recovery,
concurrent execution, and stop. The child stdout scanner was joined after
each started process. Temporary homes and the reusable package binary directory
remain owned by the existing Go test cleanup and `TestMain` paths.

## Case coverage at this stage

| Cases | Evidence | Status | Remaining edge |
| --- | --- | --- | --- |
| `CASE-NB-001`–`CASE-NB-006` | Wire selector tests | PASS locally | Linux same-head evidence belongs to `GATE-LINUX`. |
| `CASE-NB-007`–`CASE-NB-009`, `CASE-NB-020` | Harness environment and child-runner tests | PASS locally | Final clean-room audit belongs to `GATE-LOOPBACK`. |
| `CASE-NB-010`–`CASE-NB-017` | Built CLI integration matrix on Windows | PASS locally | Linux sentinel and current-head CI evidence belong to `GATE-LINUX`. |
| `CASE-NB-018` | No authorization contract exists | Not applicable | None. |
| `CASE-NB-019` | Exact opt-out disables launcher construction | Not applicable under opt-out | Server deadline behavior remains owned by existing process tests. |

## Security, privacy, and exclusions

No browser, network, remote provider, paid dependency, persisted secret, or
inherited-environment dump was introduced. No public CLI, OpenAPI, event,
persisted contract, production auto-open default, functional shared support,
C01 inventory, baseline, or stability-cleanup surface was changed. UI,
accessibility, keyboard, responsive, and localization checks are not
applicable.

## Handoff and remaining gates

Story 001 is implemented and locally releasable on the available supported
platform. Story 002 must independently perform the clean final-head Windows
rerun, current-head Linux CI review, one final `make test-functional` pass,
exclusion/source audit, and validation-loopback report. Terminal CI, conflict
resolution, and merge remain review-owned.
