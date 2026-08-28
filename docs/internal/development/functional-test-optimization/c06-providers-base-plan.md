# C06 base-provider functional-test optimization plan

## Authority and source-plan resolution

The task packet names `docs/temp/functional-test-optimization.md` as the
source plan. That path is absent from this checkout and from `origin/main`;
the absence was checked with the working-tree path check and Git tree/history
inspection on 2026-08-28. This document is the explicit, tracked replacement
authority for the C06 base-provider lane. It is a source-plan conflict
resolution, not a silent derivation from the PRD or the evidence ledger.

The PRD remains the operator-authorized scope and acceptance authority. The
companion evidence ledger records executable observations and does not replace
this plan.

## Scope and behavior spine

The lane covers the Go functional tests directly under
`tests/functional/providers/`. It characterizes each public Work, Factory
Event, Factory Session, replay, process, filesystem, logging, and provider
command-runner witness; migrates eligible controlled-edge behavior to one
package-scoped root/listener with explicit Factory Sessions and immutable
WorkDir routes; and retains process-sensitive, malformed-startup, child,
replay, environment, logging, telemetry, and executable cases in isolated
boundaries with an exact reason.

The migrated fixture and its topology/cleanup proofs are test-only declarations
in the existing aggregate test files, with the fixture core in
`tests/functional/providers/helpers_test.go`, shared-process assertions in
`cli_script_executor_test.go`, runtime-log scanning in
`runtime_logging_smoke_test.go`, and forced-cleanup proof in
`cli_timeout_cleanup_smoke_test.go`. Keeping the fixture in test files is
required because the repository deadcode checker analyzes normal package files
without test variants; the split also keeps each file within the backend-size
limit while preserving the aggregate package command without adding an
excluded deadcode-baseline entry.
Controlled model behavior uses the `ProviderCommandRunner` edge with
provider-shaped sanitized results. The process-sensitive mock-worker and
child-process assertions remain isolated where their CLI-global or executable
boundary is the behavior under test.

## Story and gate mapping

- `PROV-CHAR-001`: the evidence ledger is the one-to-one 40-row scenario and
  five-row cleanup inventory, including the original public witnesses.
- `PROV-ISO-003`: retained startup, shutdown, timeout, child, executable,
  replay, environment, logging, telemetry, and forced-assertion cases keep
  individual isolation reasons and cleanup census assertions. Unix-only cells
  require Linux verification; an unavailable platform cell is recorded as
  unproven rather than passed.
- `PROV-MIG-002`: the base fixture proves one default root/listener, explicit
  sessions, immutable route selection, route cleanup, and the post-migration
  construction limits.
- `PROV-REPEAT-004`: exact once and count-three package commands retain the
  mixed-worker refusal assertion. A clean-base reproduction is recorded as a
  pre-existing blocked criterion when present.
- `PROV-LONG-005`: the supported `functionallong` package command covers all
  four long rows; any clean-base provider/template mismatch remains visible
  and unchanged.
- `PR-CI-006`: review records final-head package timing and topology in the PR
  conversation; local timing is diagnostic and has no fixed threshold.
- `VAL-007`: the evidence ledger follows the validation-loopback template,
  labels PASS/FAIL/BLOCKED honestly, records unproven edges, and requests the
  smallest owner delta for pre-existing failures.

## Verification and exclusions

The implementation verification uses the root package command
`go test ./tests/functional/providers`, the wildcard package command when
useful, `go run ./cmd/functionalboundarycheck`, `make pkg-structure`, and the
repository test/lint gates named by the PRD. CI remains the authority for
Linux-only process cells, final-head package timing, terminal checks, conflict
resolution, and merge.

This lane does not change provider subpackages, shared functional support,
production/provider-runtime behavior, API contracts, generated clients,
workflows, Makefiles, c01 inventory/baselines, or unrelated deflake work.
