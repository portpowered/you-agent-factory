# Validation report: U01 unit-coverage build diagnostics

## Environment and artifact

- Commit/build identifier: implementation/test head
  `e4b62f745f010dbaef9da182ecfbae27214b6a3f`; this report is a
  documentation-only descendant and does not change the implementation or the
  hosted evidence sources.
- Environment and configuration: clean tracked worktree on Windows
  `10.0.26200` (`windows/amd64`). The installed Go is `1.24.2`; the required
  `go1.25.0` toolchain was selected by `GOTOOLCHAIN=auto` for the prescribed
  local suite. The hosted artifacts were produced on `ubuntu-latest` with
  `go1.25.0`.
- Customer entry point: the canonical `make test-unit-coverage` invocation,
  through `cmd/unitcoverage` and the existing `cmd/gocoveragecheck` diagnostic
  writer.
- Real and substituted dependencies: local Go tests used the real repository
  packages and Go toolchain. Hosted artifacts used the real GitHub Actions
  runner, repository checkout, Go toolchain, and artifact service. No product
  API/UI runtime, provider, paid dependency, or browser was applicable.
- Cost/call budget used: `$0`; no hosted rerun and no paid or provider call was
  made. The twelve story-002 artifacts were downloaded read-only.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| `GATE-U01-SPINE` — default-compatible canonical path | PASS | The default Make dry run retained the existing coverage arguments and omitted both optional flags. The configured dry run emitted exactly one `-jobs 4` and one `-coverage-build-diagnostics` path. The focused wrapper/diagnostic suite passed on the delivered implementation head. | A future toolchain or runner outside the tested environments. |
| `GATE-U01-FOCUSED` — wrapper and diagnostic behavior | PASS | `go test ./cmd/unitcoverage/... ./cmd/gocoveragecheck/... -count=1` passed with exit `0`; `node scripts/ci/workflow-lint.mjs` passed with `WORKFLOW_LINT_OK files=7`. Existing story-001 tests prove forwarding, cleanup, default omission, single invocation, and failure preservation. | No new product runtime behavior is in this lane. |
| `GATE-U01-HOSTED` — diagnostic publication | PASS | All six diagnostic observations contain a complete schema-v2 diagnostic with `expectedPackages=445`, `commandResult=passed`, the jobs-specific invocation hash, and the exact recorded wall. All six controls contain no diagnostic file. | Valid timing corpus and attribution admission remain blocked below. |
| `GATE-U01-INVARIANTS` — inherited corpus neutrality | BLOCKED | Every downloaded timing artifact reports `complete=false`, `testCount=11560` instead of `18273`, `testSkipCount=57` at the exclusive ceiling, and 445 timing rows; every coverage summary reports 80.7% but 480 measured packages rather than the required 445-package evidence universe. Package-state rows expose no cached/unknown state. | Corrective timing/caching instrumentation and its own hosted loopback. |
| `GATE-U01-ATTRIBUTION` — medians, buckets, and packages | BLOCKED | The canonical JSON, Markdown projection, and ledger agree that no valid cohort, median, bucket record, or 445-package classification is admitted. No arithmetic or inferred-over-30-second value was substituted. | A future valid same-head cohort and direct stage/cache measurements. |
| `LOOP-U01-THOUGHTS` — clean-room reconciliation | BLOCKED with complete-delivery disposition | The read-only audit independently downloaded all twelve named artifacts and reconciled raw values to the canonical inventory, Markdown, ledger, charter constants, privacy rules, rollback boundary, and empty-state contracts. Findings and corrective Work requests are recorded below without mutation. | Corrective Work must run a new loopback after instrumentation. |
| `GATE-U01-PR-CI` — implementation handoff | REVIEW HANDOFF | The existing PR is open as #2503 with no reviewer or inline blocking feedback. The final report head must be pushed and required CI must start there; terminal CI and merge remain review-owned. | Terminal current-head CI, conflict resolution, and merge. |
| Privacy and scope | PASS | The report and canonical evidence contain aggregates and stable identities only. No secrets, credentials, absolute runner paths, raw traces, test payloads, API/UI changes, or excluded-tree changes were introduced. | Host-wide state outside the owned worktree/artifact downloads. |

## Hosted artifact reconciliation

The ledger's source head is `4d13d577ce699ea80ff9643b2221bbd2f178bd09`;
the implementation/test head above differs only by the already-delivered
documentation/evidence descendant for this loopback. Each row below was
downloaded with `gh run download <run> --name unit-coverage-diagnostics` and
checked against the corresponding inventory observation and artifact ID.

| Mode | Jobs | Run / job / artifact | Diagnostic wall seconds | Admission |
| --- | ---: | --- | ---: | --- |
| diagnostic | 1 | `33356894321 / 99380925320 / 9745671137` | 390.983 | rejected |
| diagnostic | 1 | `33356916051 / 99381238422 / 9745966479` | 415.174 | rejected |
| diagnostic | 1 | `33356916069 / 99381250371 / 9746063004` | 582.640 | rejected |
| control | 1 | `33356915970 / 99381063719 / 9745715186` | 440.585 | rejected |
| control | 1 | `33356915983 / 99381031153 / 9745682014` | 481.061 | rejected |
| control | 1 | `33356916185 / 99381199219 / 9745920874` | 439.731 | rejected |
| diagnostic | 4 | `33356916103 / 99381199510 / 9745903639` | 309.098 | rejected |
| diagnostic | 4 | `33356916178 / 99381180962 / 9745836442` | 246.872 | rejected |
| diagnostic | 4 | `33356916239 / 99381102075 / 9745724679` | 305.377 | rejected |
| control | 4 | `33356916302 / 99381113336 / 9745738959` | 311.144 | rejected |
| control | 4 | `33356916310 / 99381242763 / 9745965836` | 308.866 | rejected |
| control | 4 | `33356916131 / 99381278545 / 9746040698` | 312.564 | rejected |

The raw diagnostic files agree with the inventory's jobs-specific hashes:
`sha256:e39f89958fb59fc2e03287be130b31df125103f09e43e5c1cd1c006c4af2b009`
for jobs 1 and
`sha256:0f4d3d21a7577dbe387bbec48624e064275ad6204f9c547ea5ca7b2c8b54ead0`
for jobs 4. They all report schema version 2, Go `go1.25.0`, 445 expected
packages, and `commandResult=passed`. The six control downloads contain no
diagnostic file.

All twelve raw timing files agree with the ledger on `packageCount=445`,
`expectedPackageCount=445`, `testPassCount=11503`, `testFailCount=0`, and
`testSkipCount=57`; each is `complete=false` for the same capture reason.
All twelve coverage summaries are complete at 80.7% and contain 480 measured
packages. Since the timing and coverage invariants fail before admission, both
jobs cohorts have `medianStatus=not-published`; there is no median to
recalculate or claim.

The canonical empty-state values also reconcile exactly: `records=[]`,
`buckets=[]`, `packageClassifications=[]`, and `cohorts=[]`; the parallelism
verdict is `inconclusive`; the factory-sessions pilot remains `KEEP-AS-IS` at
10.63 seconds; targets remain at most 240 seconds for Backend Unit Coverage
and at most 200 seconds for Backend Unit Latency; and the 147.3-second floor
and rejected sub-two-minute disposition remain unchanged.

## Customer journey

1. The tracked worktree was clean at implementation head
   `b060bb6a947cafa9497de4237f365a788f0385b0`. The default and configured
   `make -n --no-print-directory test-unit-coverage` procedures preserved the
   existing arguments and showed the optional flags only when selected.
2. The prescribed focused command
   `go test ./cmd/unitcoverage/... ./cmd/gocoveragecheck/... -count=1`
   passed with exit `0` after Go auto-selected `go1.25.0`. No product or
   browser runtime was applicable to this documentation/diagnostic lane.
3. The workflow source passed `node scripts/ci/workflow-lint.mjs` with exit
   `0`. A three-dot path audit found only the eight authorized implementation
   and U01 evidence paths; `api`, `pkg`, `ui`, `tests`, and `factory` were
   unchanged by the delivered implementation/evidence commits.
4. The twelve named artifacts from the operator-authorized hosted attempt were
   downloaded read-only under ignored `.artifacts/u01-loopback-artifacts/`.
   The independent reconciliation passed with 12 observations, 6 diagnostic
   files, 6 controls, 0 valid cohorts, 0 admitted buckets, and 0 package
   classifications.
5. The rejected invariant result was retained as a finding. No source plan,
   implementation, workflow, canonical inventory, Markdown projection, or
   ledger value was silently repaired.

## Cross-task integration and usability

- Documentation discoverability: the charter, canonical JSON, Markdown
  projection, evidence ledger, and this report are co-located under
  `docs/internal/development/unit-test-optimization/`; the report links the
  canonical ledger and records the exact hosted identities.
- Permission and error behavior: no customer-facing API, CLI, event, or
  product runtime contract changed. The existing wrapper tests and hosted
  diagnostic publication preserve child failure authority; rejected evidence
  remains rejected rather than being converted into a pass.
- Persistence/reload behavior: not applicable; this lane changes no product
  persistence or Factory Session state.
- Accessibility/keyboard/responsive behavior: not applicable; this lane has
  no UI surface.
- Operational signals: run IDs, job IDs, artifact IDs, source head, Go/runner
  identity, invocation hashes, raw wall values, invariant reasons, and the
  effectively empty setup-go cache observation are available without exposing
  secrets or raw runner paths.
- Rollback: the loopback is read-only against implementation and evidence
  sources. Removing this report rolls back only the report; the optional
  wrapper and workflow changes remain independently revertible through their
  already isolated commits. No rollback action was needed or performed.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| `U01-LOOP-001` | blocking, complete-delivery blocked | Independently download and parse all twelve named artifacts from source head `4d13d577ce699ea80ff9643b2221bbd2f178bd09`. | Every jobs 1/4 cohort preserves `complete`, 18,273 tests, skips below 57, the 445-package evidence universe, and cache-state identity before median/attribution admission. | Every observation is `complete=false`, has 11,560 tests and 57 skips, and its coverage summary has 480 measured packages. No valid cohort is admissible. | Raw artifact reconciliation above; canonical JSON and `c01-u01-evidence-ledger.md`. |
| `U01-LOOP-002` | blocking, complete-delivery blocked | Inspect the first diagnostic setup-go log and diagnostic JSON cache fields. | A cache observation must identify populated reusable content before any cache-cost bucket is measured. | The setup-go key is an exact hit with a 7,439-byte (`~0 MB`) archive; diagnostic action-cache keys are empty. This is an effectively empty hit, not populated-cache evidence. | Canonical inventory `authorizedHostedAttempt.cacheObservation`; downloaded `command.log` and diagnostic JSON. |
| `U01-LOOP-003` | review-stage | Inspect final PR state after this report is pushed. | Final head is pushed, required CI starts on that head, and review owns terminal CI/merge. | The current report is ready for handoff; no terminal current-head claim is made in committed prose. | PR #2503 conversation and review-owned `GATE-U01-PR-CI`. |

## Corrective Work requests

### `UTO-C01-CORRECTIVE-001` — Restore complete timing and cache-state instrumentation

- Affected behavior and criteria: `BEH-U01-ATTRIBUTION`,
  `GATE-U01-INVARIANTS`, and `GATE-U01-ATTRIBUTION`.
- Root-cause evidence: the same direct failures occur in all twelve completed
  observations; timing capture cannot read every Go test event, the test
  denominator is 11,560 rather than 18,273, skips are at the exclusive
  ceiling, coverage measures 480 packages, and package states do not expose
  cached/unknown identity. The exact setup-go hit is only 7,439 bytes and the
  diagnostic action-cache fields are empty.
- Smallest recommended correction/prerequisite: a separately owned
  instrumentation Work should make the timing collector complete and expose
  package/cache state while preserving the canonical package selection,
  `-count=1`, coverage floor, and required checks. It must independently prove
  its parser/identity behavior before any new hosted measurement.
- Dependencies and retest scope: after the corrective implementation passes
  its focused tests, its owner may run a new bounded same-head jobs 1/4
  diagnostic/control cohort and reconcile the five infrastructure buckets and
  package universe. This U01 lane must not be silently reopened or repaired.

### `UTO-C01-CORRECTIVE-002` — Run the corrective clean-room loopback

- Affected behavior and criteria: `UTO-LOOPBACK`, `GATE-U01-INVARIANTS`, and
  `GATE-U01-ATTRIBUTION` after `UTO-C01-CORRECTIVE-001`.
- Root-cause evidence or remaining uncertainty: the current loopback can
  prove the diagnostic spine and the conservative rejection, but it cannot
  prove a valid median, direct bucket timing, or 445 package classification
  from invalid artifacts.
- Smallest recommended correction/prerequisite: the corrective owner must
  produce a new clean-room report using the validation-loopback template,
  reconcile newly valid raw artifacts to a new canonical evidence record, and
  report FAIL/BLOCKED again rather than inferring missing values.
- Dependencies and retest scope: depends on
  `UTO-C01-CORRECTIVE-001`; rerun only after that Work's focused and hosted
  evidence is complete. The existing U01 artifacts remain historical inputs.

## Verdict

**BLOCKED with complete-delivery disposition under the operator amendment.**

The clean-room loopback passed the executable-spine, publication, scope,
privacy, and conservative-empty-state checks, but the authorized hosted
artifacts fail the inherited invariants required for valid cohorts and
attribution. The amendment explicitly permits this partial/blocked result as
complete delivery after a genuine attempt. No median, bucket measurement,
445-package classification, optimization claim, terminal CI result, or merge
is claimed.

## Delta-plan request [Required for FAIL/BLOCKED]

- Affected behavior and criterion: `GATE-U01-INVARIANTS` and
  `GATE-U01-ATTRIBUTION` under `BEH-U01-ATTRIBUTION`.
- Root-cause evidence or remaining uncertainty: all twelve raw timing
  artifacts fail `complete`, test-count, skip-ceiling, coverage-package, and
  cache-state requirements; therefore the source inventory correctly admits
  no medians, buckets, or package rows.
- Smallest recommended correction/prerequisite: accept and route
  `UTO-C01-CORRECTIVE-001` for timing/cache instrumentation, then
  `UTO-C01-CORRECTIVE-002` for a fresh clean-room loopback. Do not infer
  attribution from the rejected artifacts or rerun this lane unchanged.
- Dependencies and retest scope: the corrective owner supplies focused
  parser/instrumentation evidence, a new bounded same-head cohort if the
  invariants become valid, and its own validation-loopback. Review owns the
  final current-head CI, conflict resolution, and merge for this PR.
