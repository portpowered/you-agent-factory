# Validation report: C07 isolation inventory reconciliation

## Environment and artifact

- Commit/build identifier: clean-room head
  `515f3bd3c125c37a18bb9e5fb79358c66fc58c9f`; executable source commit
  `ec194b5ab5d24803307b0cd8bb8895cb6d5ab9ee`. The paired artifact hashes in
  that committed checkout were JSON
  `ce864db14c3e8381e58cd724ea8df579c54a6ce2888bc317b38f0205d583465a` and
  Markdown
  `eec166d801c1414bdba9d90b5a146d1e7e2127bf7d61dd9edc2da3631e83c3a4`.
- Environment and configuration: Windows `10.0.26200`, Go
  `go1.25.0 windows/amd64`, `GOAMD64=v1`, PowerShell `7.6.5`, repository
  `go.mod`, empty `GOFLAGS`, default local Go cache, no remote credentials,
  and no paid calls.
- Customer entry point: the existing local-real `root.BuildProcess` and
  `Process.Execute` witnesses through public Work, Factory Event, Factory
  Session, CLI, runtime API, JavaScript, and ACP boundaries.
- Real and substituted dependencies: real source files, Go test executable,
  filesystem, loopback/listener, OS process, and stdio boundaries; controlled
  provider/worker/command effects remain at the existing `edges.Edges` and
  command-runner boundaries. No production or shared-support code was edited.
- Cost/call budget used: `$0`; zero remote or paid calls.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Clean package and identity reproduction | PASS | Recorded `go list` pattern sets resolved 77 packages with no missing or extra entries. One `go test $RESOLVED_PACKAGE -list '^Test' -count=0` run per package exited 0 and listed 595/595 top-level declarations. | Full runtime execution of every final row. |
| JSON/Markdown agreement | PASS | Portable jq structural filters and the PowerShell audit passed: format 2, 812 rows, 595 top-level declarations, 217 named scenarios, 206 process-start observations, class sum 812, zero unclassified, 354 unique C01 mappings, and exact Markdown totals. | None for the audited artifact fields. |
| Canonical format-2 schema | PASS | The read-only schema validator required `package`, `sourceHashSha256`, string `classification`, nullable `isolation`, array `priorC01RowIds`, string `mappingKind`/`evidence`, and runtime observation fields; it rejected legacy field names/types and returned zero legacy fields. | No runtime behavior is implied by the evidence schema. |
| Source and witness integrity | PASS | All 812 source files existed; 812 SHA-256 pairs, 812 source references, declaration/named-parent locations, and all process-observation references matched. | Unsupported operating systems and behavior beyond the cited source/ledger witnesses. |
| Representative process-sensitive behavior | PASS | All eight declared focused selectors exited 0. The set covers process start/shutdown, malformed configuration, port binding, executable selection, provider crash/exit, process environment, Work, Factory Event, CLI, runtime API, JavaScript, and ACP retained replay witnesses. | Full-suite runtime parity, remote providers, and live AGY behavior. |
| Repository lane ownership | PASS | `make test-lane-audit` exited 0 and reported `contract=8 functional=141 integration=10 maintenance=106 release=1 stress=1 unit=446`. | Required CI on the pushed PR head. |
| Inventory Markdown diagnostics | PASS | `go run ./cmd/markdown-linter docs/internal/development/functional-test-optimization/c01-eligibility-inventory.md` exited 0 with no diagnostics. | Review-host rendering. |
| Diff hygiene | PASS | `git diff --check` exited 0 in the clean checkout; the implementation worktree check is rerun before commit. | Review-owned merge/conflict state. |
| Directional timing/topology | PASS (directional) | Final-head clean-room focused-selector wall times were 5.973s, 6.653s, 6.230s, 12.038s, 15.463s, 5.823s, 5.368s, and 5.779s. The available identity comparison is C01 format 1: 39 packages/342 top-level/354 rows with no separate process unit, versus final: 77 resolved directories/595 top-level/812 rows/206 process-start observations. | These are noisy local observations, not a timing threshold or a like-for-like total process count; review CI owns same-head package timing. |
| VAL-001 clean-room loopback | PASS | This report records the read-only detached-checkout reproduction, exact commands, artifact hashes, focused runtime results, and remaining edges. No repair was made. | Current-head required CI, terminal CI result, and merge. |

## Clean-room procedure and observed results

1. A detached worktree was created from the delivered implementation tree.
   `git status --short --untracked-files=all` was empty before validation and
   remained empty after all commands.
2. The five recorded `go list` pattern sets reproduced the artifact's 77
   package import paths with zero missing or extra entries.
3. Each of the 77 package import paths ran independently with:

   ```text
   go test $RESOLVED_PACKAGE -list '^Test' -count=0 -timeout=5m
   ```

   Every invocation exited `0`; aggregate output was `packages=77
   expectedTop=595 listedTop=595 failures=0`.
4. The GATE-AUDIT-005 jq filters checked format, count sums, class totals,
   unique row and stable identities, 354 unique C01 row IDs, 200 mapped final
   identities, 612 added identities, and current classification counts. The
   read-only format-2 schema validator required the canonical row, mapping,
   and observation fields and failed on the legacy names/types (`importPath`,
   `sourceSha256`, `isolationReason`, object-valued `classification`, and
   legacy mapping keys); it passed with zero legacy fields. The PowerShell
   source/hash audit checked all source paths, SHA-256 values,
   source-reference and line-shape agreement, observation references, and
   exact Markdown count rows. All audits passed.
5. The eight focused selectors were run once each with
   `go test -count=1 -timeout=15m -run <selector> <package>`. Every command
   exited `0`:

   | Package | Selector | Wall time |
   | --- | --- | ---: |
   | `./tests/functional/work/routing` | `^TestSharedProcessWorkRouting$` | 5.973s |
   | `./tests/functional/workers/mock` | `^TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected$` | 6.653s |
   | `./tests/functional/workers/inference` | `^TestProviderNonZeroExitMapsToPublicFailure$` | 6.230s |
   | `./tests/functional/factory/packaged/cross` | `^TestPackagedFactoryContinuousServerWithoutInvocationRemainsReachable$` | 12.038s |
   | `./tests/functional/workers/mock` | `^TestBuiltCLIBatchExitCodesReportSingleWorkOutcome$` | 15.463s |
   | `./tests/functional/orchestration/javascript/workers` | `^TestJavaScriptSharedWorkerBehavior$` | 5.823s |
   | `./tests/functional/sessions/chat_sessions/root_composition` | `^TestACPWorkerChildStreamSurvivesRetainedReplay$` | 5.368s |
   | `./tests/functional/runtime_api/factory_transformation` | `^TestCurrentFactoryPUT_RequiresAdvancedSaveVersion$` | 5.779s |

6. The remaining clean-room commands all exited `0`:

   ```text
   make test-lane-audit
   go run ./cmd/markdown-linter docs/internal/development/functional-test-optimization/c01-eligibility-inventory.md
   git diff --check
   ```

The source-plan path named by the PRD,
`docs/temp/functional-test-optimization.md`, is not present in the clean
tracked checkout. The inventory retains that authority status explicitly and
the PRD was used as the task authority; no source plan was reconstructed and
no repair was made.

## Customer journey

1. The independent validator checked out the delivered tree without local
   changes and confirmed the tracked identity inventory was present.
2. It resolved the same package universe, listed every executable top-level
   declaration, and reconciled the JSON and Markdown count units.
3. It recomputed each final source hash and reference, verified all canonical
   C01 mappings and final lineages, and confirmed zero unclassified rows.
4. It exercised the selected local-real Work, Factory Event, Factory Session,
   CLI, runtime API, JavaScript, listener, provider-process, and ACP replay
   witnesses through their existing boundaries.
5. It ran the ownership and Markdown quality gates, observed no discrepancy,
   and removed the detached worktree without changing the repository.

## Cross-task integration and usability

- Documentation discoverability: the paired inventory links this report under
  its Story 004 clean-room section; no customer-facing documentation or UI
  surface changed.
- Permission and error behavior: no authorization policy changed; the focused
  witnesses retain their existing typed Work, Factory Event, provider, CLI,
  runtime API, and ACP error assertions.
- Persistence/reload behavior: the retained witnesses cover the recorded
  Factory Session, Work, Factory Event, replay, and response surfaces; this
  loopback adds no new persistence behavior.
- Accessibility/keyboard/responsive behavior: not applicable; no UI changed.
- Operational signals: package IDs, stable identities, source hashes, process
  observation IDs, declaration counts, class counts, and lane-ownership counts
  were checked at the owning artifact boundaries.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| C07-F001 | informational/pre-existing | `Test-Path docs/temp/functional-test-optimization.md` and clean-checkout file discovery | The governing source plan is available for an independent comparison. | The path is absent from the tracked checkout; the PRD, paired artifacts, and prior authority note remain available. No inventory discrepancy was found, and no plan was reconstructed. | `sourcePlanAuthority.status=operator-root-authority-not-present-in-clean-worktree`; paired JSON metadata. |

## Verdict

PASS for the read-only clean-room inventory reconciliation, canonical
format-2 schema, and representative local-real process witnesses. The
remaining unproven edges are explicitly owned by review or by
unsupported/remote environments; this report does not claim them.

## Delta-plan request [Required for FAIL/BLOCKED]

Not applicable: no artifact, identity, source-hash, mapping, classification,
focused-selector, ownership, Markdown, or diff discrepancy was observed.
