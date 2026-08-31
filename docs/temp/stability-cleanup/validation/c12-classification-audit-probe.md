# Validation report: stability-cleanup C12 classification audit probe

Status: **FAIL — complete report-only delivery**

This is a strict validation-loopback report for the classification-audit
probe. The FAIL is the intended product result: the independent audit found
two active comparison inputs missing from the committed classification
catalog. No catalog, checker, workflow, baseline, inventory, or other product
file was repaired.

## Environment and artifact

- Commit/build identifier: clean-room evidence is pinned to full
  `origin/main` SHA `8b7f496b601eac700468f5f6241c8e09141d4144`.
- Environment and configuration: Windows/amd64; Git 2.44.0, Go 1.25.0,
  Node 22.12.0, GNU Make 4.4.1, and `gh` 2.88.0. The detached checkout
  recorded `git rev-parse HEAD` equal to the pin and an empty
  `git status --porcelain`.
- Customer entry point: required workflow jobs were followed into Make
  targets and checker sources; the functional OS edge is
  `Makefile:570` -> `cmd/functionalosboundarycheck` with the two paths named
  by `cmd/functionalosboundarycheck/main.go:18-19`.
- Real and substituted dependencies: local Git, filesystem, source, and
  checker evidence were real at the pinned revision. Failure cases used
  disposable copies only. No hosted run, artifact, or GitHub mutation is
  claimed by this report.
- Cost/call budget used: zero paid calls; GitHub access was read-only; the
  disposable worktree and transient outputs were removed.

The governing source plan named by the PRD was not required to produce this
finding and was not reconstructed. Counts and hashes below are historical
observations bound to the pinned SHA, not claims about later revisions.

## Independent ledger result

The audit procedure was run twice before the operator amendment and produced
the same result. This report carries forward the second exact-pin result; it
does not rerun the audit.

| Check | Observed result | Property / boundary |
| --- | --- | --- |
| Source-first ledger | 30 unique active comparison paths | Required jobs/checkers to comparison-file inputs |
| File-first ledger | 30 unique active comparison paths | Independently reverse-traced tracked candidates to active consumers |
| Set comparison | Both bidirectional differences empty | The two ledgers agree before the catalog join |
| Duplicate check | Zero duplicates in either ledger | Normalized sorted sets are unique |
| Tracking/readability | 30/30 paths tracked and readable | `git ls-files --error-unmatch` plus read checks at the pin |
| Ledger digest | SHA-256 `e2ca1773479ab2aa12cd8ad4b8f147b3cf6b33b94e2a33126f60df527917d531` | Digest of the canonical normalized ledger evidence |
| Candidate sweep | 674 tracked keyword candidates; 26 literal and four source-aware active resolutions | Completeness witness only; it is not ledger input |
| Catalog join | 28 unique catalog rows; zero out-of-scope rows; two active paths absent | This is the decisive FAIL |

The four source-aware reverse resolutions recorded the expression, base, rule,
resolved path, and required consumer as follows:

| Resolver | Expression and base | Resolution rule and resolved path | Required consumer |
| --- | --- | --- | --- |
| R-04 | `internal/functionaltestmetadata/baseline_repo_test.go:20`; `filepath.Join(repoRoot, "docs", "internal", "baselines", "functional-undocumented-tests.json")` | Resolve from `repoRoot`, normalize to Git separators: `docs/internal/baselines/functional-undocumented-tests.json` | `make verify-tests` -> `test-maintenance` |
| R-07 | `ui/scripts/check-hardcoded-ui-copy.ts:10-18`; `path.join(UI_DIR, "..", "docs", "internal", "baselines", "hardcoded-ui-copy-baseline.txt")` | Resolve from `UI_DIR`, normalize to Git separators: `docs/internal/baselines/hardcoded-ui-copy-baseline.txt` | `make lint` -> `ui-lint` -> localized-copy checker |
| R-12 | `cmd/servicecyclecheck/main.go:56`; `filepath.Join(cfg.root, defaultCeilingRelativePath)` with `defaultCeilingRelativePath` from `report.go:20` | Resolve from `cfg.root`, normalize to Git separators: `docs/internal/baselines/service-cycle-ceiling.json` | `make lint` -> `service-cycle-check` |
| R-17 | `ui/src/styles/palette-contrast-ratchet.component.test.ts:9`; relative import `./palette-contrast-baseline` | Resolve relative to `ui/src/styles` and apply the `.ts` extension: `ui/src/styles/palette-contrast-baseline.ts` | `make ui-component-test` |

## Confirmed classification gap

The two paths are active, maintained inputs, not keyword-only candidates:

| Active path | Source/consumer evidence | Required repair-lane class | Owning writer / maintenance owner |
| --- | --- | --- | --- |
| `docs/internal/baselines/functional-os-spawn-baseline.json` | `Makefile:570` passes it as `-baseline`; `cmd/functionalosboundarycheck/main.go:18` declares the default. The checker evaluates the current static OS-spawn ceiling and never writes this file. | Proposed `R-18` manual ratchet: functional OS-spawn ceiling, deletion-tolerant site/package count and IDs | No canonical writer in this revision. Functional-test / functional-OS boundary maintainers own a reviewed manual update; do not repin unattended. |
| `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json` | `Makefile:570` passes it as `-inventory`; `cmd/functionalosboundarycheck/main.go:19` declares the default. The checker reconciles source sites and verdicts and never writes this file. | Proposed `R-19` manual ratchet: source-backed OS-spawn intentionality and admission records | No canonical writer in this revision. Functional-test / functional-OS boundary maintainers own a reviewed manual update; do not repin unattended. |

The class IDs above are the next available manual-ratchet IDs, not existing
claims: the separate catalog-repair lane should confirm or change the IDs while
preserving the stated manual one-way maintenance rule. The checker’s own
diagnostic requires the two files to be updated together with an
`INTENTIONAL-OS` row naming an allowed property when a new site is admitted.

The disposable fail-closed matrix recorded `FAIL` with `repairs=0` for empty,
duplicate, unreadable, untracked, missing, and unequal ledger conditions. The
detached checkout remained clean after every case. No repository repair was
used to turn a failure into a pass.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| GATE-IDENTITY and GATE-LEDGERS: non-empty equal tracked/readable duplicate-free ledgers with zero unclassified paths | FAIL | The pinned clean-room identity and both 30-path ledgers passed all integrity checks, but the post-hoc catalog has 28 rows and omits the two confirmed active functional-OS paths above. | Reviewed catalog classification and a fresh Story 001 audit after the separate repair |
| GATE-SEMANTICS: every class follows checker/writer semantics and ratchets retain reviewed one-way maintenance | BLOCKED | Story 002 was not started after the Story 001 catalog-completeness failure. The two omitted inputs have only the source/reader evidence recorded above. | Full row-by-row checker/writer, owner, procedure, and unattended-writer audit |
| GATE-SEMANTICS: deterministic snapshots and grouped failure preservation | BLOCKED | No Story 002 disposable writer or fault-injection evidence was run. | Deterministic generation, staging/replacement rollback, invalid-candidate, partial-completion, and concurrency witnesses |
| GATE-ALLOWLIST: exact protected snapshot set and out-of-scope rejection | BLOCKED | The independent active set exposed the catalog gap before allowlist comparison; no protected allowlist verdict was claimed. | Exact snapshot-to-allowlist comparison and rejection probes |
| GATE-HOSTED: successful post-delivery remote-real generation and quiescence/supersession | BLOCKED | Story 002 was gated; no hosted source/run/job/artifact/log chain was inspected for this lane. | Qualifying remote-real run bound to source and delivered SHAs |
| Coverage floors and drawdown baselines remain manual ratchets; no required comparison is weakened | BLOCKED | The report-only scope made no such changes, but the semantic audit that proves every classification was not reached. | Independent semantic confirmation of all ratchet rows |
| GATE-LOOPBACK binds mutable counts and hashes to immutable identity | PASS | The 30/30 counts and ledger SHA-256 are explicitly bound to clean-room SHA `8b7f496b601eac700468f5f6241c8e09141d4144`; no later revision is substituted. | Future revisions require a new clean-room pin |
| Story 002 focused atomicity, allowlist, invalid-input, concurrency, and quiescence suites pass or report failure | BLOCKED | Those suites were not reached because Story 001 exposed a real catalog gap; no result is invented. | Controlled Story 002 suite execution after catalog repair and Story 001 rerun |
| Package-level PR/CI evidence is the performance verdict; hosted latency remains remote-real | BLOCKED | No performance run was authorized or required for this report-only FAIL finding. | Package-level PR/CI and hosted unit-latency evidence |
| Security and operational constraints hold | PASS | The audits used read-only GitHub access, exposed no token, made zero paid calls, kept retries bounded, cleaned disposable outputs, and changed no product file. | Review of the final PR head and CI environment |
| GATE-LOOPBACK creates the owned report and returns strict PASS only when all assigned proof exists, otherwise FAIL/BLOCKED | PASS | This report is the sole owned artifact, follows the validation-loopback template, names the failed criterion and smallest delta, and returns exactly one overall `FAIL`. | Report-PR handoff checks remain review-owned |
| GATE-SCOPE: only the owned report is changed and whitespace is clean | PASS | The report-only change was checked with `git diff --check`; `prd.json` and `progress.txt` remain ignored local scaffolding and are not PR content. | Final pushed-head scope check |
| Implementation-stage delivery: final head pushed, PR open, CI started, feedback addressed | BLOCKED at report creation | No PR existed when this report was authored. The implementation handoff must record the final head and CI start in the PR conversation. | Open PR, named checks started on this head, and any blocking feedback |

## Task criteria

### Story 001 — complete active comparison-file set

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Clean checkout identity equals full current-main pin, status is empty, and prerequisites are source-visible | PASS | Detached checkout at `8b7f496b601eac700468f5f6241c8e09141d4144`; `HEAD` matched the pin, `git status --porcelain` was empty, tool identities were recorded, and all required source surfaces were tracked/readable. | Current main may advance; this evidence remains bound to the pin |
| Independent ledgers are non-empty, equal, tracked, readable, duplicate-free, and zero-unclassified | FAIL | Both ledgers are 30 unique tracked/readable paths with empty set differences and zero duplicates; the 28-row post-hoc catalog omits the two functional-OS paths. | Separate reviewed classification and fresh rerun |
| Source-built/relative paths record expression, base, resolver, resolved path, and consumer | PASS | R-04, R-07, R-12, and R-17 are recorded in the resolver table above. | No additional constructed path outside the audited four |
| Empty, duplicate, unreadable, untracked, missing, and unequal ledgers fail without repair | PASS | Disposable fail-closed matrix: all six conditions returned `FAIL`, `repairs=0`; the detached worktree remained clean. | No production repair behavior was exercised |
| GATE-IDENTITY/GATE-LEDGERS record commands, outputs, immutable identity, and remaining edges | PASS | Procedure, pin, environment, counts, digest, catalog difference, and remaining semantic/hosted edges are recorded above. | Terminal PR/CI and merge |

### Story 002 — classification semantics and protected snapshot maintenance

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Every active row has source-backed class, owner, comparison unit, and update mechanism | BLOCKED | Story 002 was gated by Story 001’s confirmed classification gap. | Full row-by-row semantic ledger |
| Every ratchet has reviewed manual procedure, one-way commitment, and no unattended repin | BLOCKED | No Story 002 ratchet audit was run; the two omitted paths are reported as manual-ratchet candidates only. | Full ratchet audit |
| Every snapshot is deterministic from identical pinned inputs | BLOCKED | No disposable snapshot generation was run. | Deterministic snapshot evidence |
| Grouped writers preserve prior state under staging/replacement/invalid/rollback/partial failures | BLOCKED | No grouped-writer fault-injection case was run. | Controlled atomicity cases |
| Distinct-root concurrent grouped publications finish without cross-root corruption | BLOCKED | No concurrency case was run. | Controlled concurrency case |
| Snapshot set equals protected allowlist and rejects out-of-scope candidates | BLOCKED | Allowlist comparison was not reached after the earlier catalog failure. | Exact set comparison and rejection case |
| Qualifying hosted run proves generation, bounded scope, source identity, and quiescence/supersession | BLOCKED | No qualifying hosted run was inspected. | Remote-real run, artifact, job, and log chain |
| Denied/cancelled/timeout/missing hosted evidence is BLOCKED and contradicted/invalid evidence is FAIL | BLOCKED | Story 002 hosted evidence was not reached; no outcome is inferred. | Hosted evidence closure matrix |

### Story 003 — strict validation-loopback verdict

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Every project and task criterion has an independent PASS/FAIL/BLOCKED row with evidence and an edge | PASS | The project and Story 001–003 tables map every declared criterion, including gated Story 002 criteria. | Review may identify a transcription defect |
| Contradiction or indispensable missing proof produces exactly FAIL or BLOCKED with the smallest delta | PASS | The confirmed two-row catalog omission is a contradiction to zero-unclassified completeness; the single verdict is `FAIL` with a catalog-repair delta. | Catalog repair belongs to a separate lane |
| Counts, hashes, and observations name immutable Git/run identity or are labeled historical | PASS | Ledger counts/digest are bound to `8b7f496b601eac700468f5f6241c8e09141d4144`; no unbound hosted observation is claimed. | Future revisions require new identity |
| Final worktree contains only the owned report and passes `git diff --check` | PASS | The report is the only intended PR path; ignored scaffolding is excluded from the diff. | Final pushed-head verification |
| GATE-LOOPBACK combines local-real and remote-real proof without claiming future revisions | BLOCKED | Local-real completeness evidence exists, but Story 002’s remote-real hosted proof was gated and therefore is not claimed. | Hosted proof after the separate prerequisite repair |
| GATE-PR-CI records this PR head’s named checks in a PR comment and stops at implementation handoff | BLOCKED at report creation | The PR had not yet been opened when this report was authored; CI-start evidence belongs in the PR conversation, never in this report commit. | Final PR head, started checks, and review-owned terminal CI |

## Customer journey

1. Fetch `origin/main`, create a detached disposable worktree at its full
   SHA, and record `git rev-parse HEAD` plus `git status --porcelain`. The
   recorded HEAD matched `8b7f496b601eac700468f5f6241c8e09141d4144` and status
   was empty.
2. Record Windows/amd64 and Git/Go/Node/Make/gh identities. All required
   source surfaces were present and readable.
3. Traverse required workflow jobs through Make/checker sources to construct
   the source-first ledger. Independently discover tracked candidates and
   reverse-trace active consumers, using the four source-aware resolvers where
   literal matching was insufficient.
4. Normalize, sort, deduplicate, and compare the ledgers. Each contained 30
   paths; both differences were empty; all 30 were tracked/readable; the
   normalized evidence digest was
   `e2ca1773479ab2aa12cd8ad4b8f147b3cf6b33b94e2a33126f60df527917d531`.
5. Join the independent active set to the 28-row classification catalog only
   after ledger construction. The join found the two active paths listed in
   the classification-gap table and therefore violated zero-unclassified.
6. Run the disposable fail-closed matrix. Empty, duplicate, unreadable,
   untracked, missing, and unequal conditions all returned `FAIL` with
   `repairs=0`; no canonical state changed.
7. Stop at the confirmed FAIL, preserve the smallest repair delta, and publish
   this report. Story 002 is gated and its semantic/hosted evidence is
   intentionally not represented as a pass.

## Cross-task integration and usability

- Documentation discoverability: the report is the sole owned validation
  artifact under `docs/temp/stability-cleanup/validation/` and names the
  source edges, omitted paths, proposed classes, owner, and retest procedure.
- Permission and error behavior: source and Git evidence was read-only. The
  fail-closed matrix records errors as FAIL without repair; missing hosted or
  semantic evidence remains BLOCKED.
- Persistence/reload behavior: no product persistence or runtime state was
  changed. The report binds all mutable observations to one immutable SHA.
- Accessibility/keyboard/responsive behavior: not applicable; no UI or
  browser surface changed.
- Operational signals: the report records exact identity, counts, digest,
  zero-repair behavior, bounded scope, cleanup, and the review-owned PR/CI
  boundary. No secrets or paid-provider payloads were recorded.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| `VAL-C12-001` | BLOCKING | From a clean detached checkout at `8b7f496b601eac700468f5f6241c8e09141d4144`, independently build both ledgers and compare the normalized active set with the maintenance catalog. | Every active comparison path has exactly one classification row. | Ledgers agree at 30 paths, but the catalog has 28 rows and omits `docs/internal/baselines/functional-os-spawn-baseline.json` and `docs/internal/development/functional-test-optimization/c01-eligibility-inventory.json`. | 30-path ledger digest `e2ca1773479ab2aa12cd8ad4b8f147b3cf6b33b94e2a33126f60df527917d531`; source edges `Makefile:570` and `cmd/functionalosboundarycheck/main.go:18-19`. |
| `VAL-C12-002` | BLOCKED / gated | Attempt to continue into Story 002 semantic, disposable-writer, allowlist, and hosted evidence after Story 001 reports the confirmed catalog gap. | Story 002 requires a complete classified active set as its dependency. | No Story 002 evidence was run or promoted to PASS; the missing catalog classification is left for the separate repair lane. | Story 001 acceptance result and PRD dependency graph. |

## Verdict

FAIL

The verdict is strict because the zero-unclassified requirement is violated by
two confirmed active inputs. This is a complete report-only delivery under the
operator amendment: detecting and precisely routing the catalog gap is the
lane’s product. The report does not claim Story 002’s unperformed semantic or
hosted evidence.

## Delta-plan request [Required for FAIL/BLOCKED]

- Affected behavior and criterion: BEH-CLASSIFY, GATE-IDENTITY/GATE-LEDGERS,
  and Story 001’s zero-unclassified criterion.
- Root-cause evidence or remaining uncertainty: the functional OS boundary
  target at `Makefile:570` passes two real maintained files to a read-only
  checker whose defaults are declared at `cmd/functionalosboundarycheck/main.go:18-19`.
  The independently derived active set has 30 paths while the committed
  classification catalog has 28 rows. The two omitted paths are named above;
  their active status is confirmed by the checker source and the paired
  functional inventory/baseline evidence at the immutable pin.
- Smallest recommended correction/prerequisite: in the separate catalog lane,
  add reviewed rows for both paths as manual ratchets (proposed `R-18` for the
  deletion-tolerant functional OS baseline and `R-19` for the source-backed
  intentionality inventory), document the functional-test/functional-OS
  maintainer, and state that no unattended writer exists. Do not broaden the
  checker or convert the comparison to advisory.
- Dependencies and retest scope: after that reviewed catalog correction,
  rerun only Story 001 from a new full current-main pin, then unlock Story 002
  for its row semantics, writer/atomicity, allowlist, and hosted checks. This
  report’s exact-pin evidence remains historical and is not silently reused
  for a later tree.

## Handoff

The implementation stage must force-add this ignored-but-owned report with
`git add -f docs/temp/stability-cleanup/validation/c12-classification-audit-probe.md`,
commit only the report, push the final head, open the named PR, and record the
current PR head plus required-check start in a PR comment. Review owns terminal
CI, conflicts, merge, and any future catalog-repair lane. No CI transcript or
audit note belongs in a follow-up commit.
