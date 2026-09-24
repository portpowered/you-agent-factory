# Validation report: ACP-GATE / Story 003

## Environment and artifact

- Commit/build identifier: PR #2645 head `ff1ffd2d92a53915af6ada5c4320b8e011da8797`; base `41f5c1926eafa94f467e63b34c21ce30b0b0e242`. Required run `35873309829`, attempt 2.
- Environment and configuration: Read-only GitHub status/conversation review plus an existing clean detached Windows checkout at the exact PR head. The checkout had no modified files. The referenced source plan at `docs/temp/projects/factory-reliability/source-plan.md` is absent from this worktree; this report uses the PRD and its scoped task criteria as authority.
- Customer entry point: Hosted `root.BuildProcess` / `Process.Execute` through a Factory Session, ACP child peer, and public terminal result.
- Real and substituted dependencies: Hosted controlled peer from the failed functional run. The raw artifact omits child-peer response, stderr, exit status, and correlated process/Factory Event identity; those fields are not inferred from the Factory terminal result.
- Cost/call budget used: No new hosted run, rerun, paid call, or product execution in this loopback.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| FR-A1 | BLOCKED | This slice did not restart or replay the preserved recording. | FR1 preserved-recording independent replay. |
| FR-A2 | BLOCKED | This slice did not exercise concurrent allocation or exact-ID fan-in. | FR1/FR2 concurrent allocation and fan-in independent validation. |
| FR-A3 | BLOCKED | Slice contribution PASS: exact failed selectors and missing process/event fields are recorded; the red gate and Reliability owner/release event are linked below. | FR3/FR8 independent failure-to-repair-to-review-to-merge-to-consumption validation. |
| FR-A4 | BLOCKED | Loaded/authored instruction identity is outside this slice. | FR3 loaded-definition and review-handoff independent validation. |
| FR-A5 | BLOCKED | Slice contribution PASS: the shared ACP gate blocker and its owner are recorded; worker reconciliation and stream behavior were not exercised. | Task 77/#2549 and FR4 history/stream/UI independent validation. |
| FR-A6 | BLOCKED | Cancellation and interruption behavior is outside this slice. | FR5 cancel/interrupt independent validation. |
| FR-A7 | BLOCKED | Restart recovery and retry behavior are outside this slice. | FR6 host restart independent validation. |
| FR-A8 | BLOCKED | Cleanup and retention behavior is outside this slice. | FR7 retention independent validation. |
| FR-A9 | BLOCKED | Slice contribution PASS: raw hosted evidence, exact red head, and external owner/release event are recorded; no causal correction or soak is claimed. | FR8 compiled-bytes engineering proof and 24-hour injected-failure soak. |
| FR-A10 | BLOCKED | Source-blind operator probes are outside this slice. | FR8 source-blind Luna/max clean-install operator probe. |

## Story 003 criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Exact failed selectors and identities, or exact missing identity | PASS | Run `35873309829` / job `107241161711`, attempt 2, artifact `10758933879` names both selectors and their Factory outcomes; absent child response/stderr/exit and process/event identity are explicitly recorded. | Child-peer cause and process/event correlation. |
| Changed-condition run only when #2644 changes ACP behavior | PASS | #2644 merged diff is only `contracts/testdata/baseline/cli-commands.json`; no ACP-relevant condition changed, so no hosted run was justified. | A future ACP-relevant change needs the single authorized changed-condition run. |
| Persistent red or unavailable evidence names owner and release event | PASS | Required checks are terminal red; comment [#5800402494](https://github.com/portpowered/you-agent-factory/pull/2645#issuecomment-5800402494) names Factory Reliability (ACP/CI), the missing child protocol/Work/Event evidence, and the required causal correction or exact-head green result. | Reliability's evidence/correction or green release event has not occurred. |
| Clean-room report follows template and requests a delta plan | PASS | This report records project and story criteria, the exact-head journey, findings, blocked verdict, and smallest prerequisite; no implementation repair was performed. | Full project acceptance and green hosted ACP gate remain open. |

## Customer journey

1. Read PR #2645 and confirmed it is OPEN at head `ff1ffd2d92a53915af6ada5c4320b8e011da8797`, base `41f5c1926eafa94f467e63b34c21ce30b0b0e242`.
2. Confirmed required run `35873309829` is terminal: Backend Functional Coverage job `107241161711` and dependent Verification Policy job `107243332217` both failed. The other relevant checks are not a substitute for these failures.
3. Inspected attempt 2 artifact `10758933879` (raw-failures index complete; JSONL SHA-256 `6CEF9C94B7BE6D04F4B4AFC414347DAEC5230060EE49D72893D6E562BB9A9D73`). It lists `TestProvidersACPSerializesConcurrentPromptsOnOneStdioConnection` and `TestYouRunMapsSkipPermissionsToSDKGoldenPermissionSelection`. The serialization case ends with Factory Session `FAILED` / result `UNAVAILABLE`; the permission case records completed Work 0 of 1 and a failed/unknown dispatch. Peer response/exit and child process/event identity are absent.
4. Compared the #2644 merge `cef9c40ed4150063806e50eaaa069c5dba1e30ee`: its diff changes only `contracts/testdata/baseline/cli-commands.json`, with no ACP-relevant changed condition. No hosted retry or shared code edit is justified.
5. Confirmed the existing exact-head handoff comment [#5800402494](https://github.com/portpowered/you-agent-factory/pull/2645#issuecomment-5800402494) assigns Factory Reliability (ACP/CI) to capture child protocol response/stderr/exit and correlate Work/Factory Event timing, then provide a causal correction or exact-head green required checks. The PR remains open and unmerged.

## Cross-task integration and usability

- Documentation discoverability: This report is retained with internal development validation records; the exact-head handoff is also on PR #2645.
- Permission and error behavior: The hosted Factory boundary reports failure/unavailable or zero completed Work. The missing child-peer evidence prevents assigning a lower-level cause.
- Persistence/reload behavior: Not exercised by this read-only gate audit.
- Accessibility/keyboard/responsive behavior: Not applicable to this backend gate audit.
- Operational signals: Hosted diagnostics identify selectors and Factory outcomes but omit child-peer response/stderr/exit and correlated process/Factory Event timing.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| CI-ACP-01 | BLOCKING | Inspect run `35873309829`, attempt 2, job `107241161711`, artifact `10758933879`, at exact PR head `ff1ffd2`. | Required Functional Coverage and Verification Policy are green, or a precise external owner/release event is recorded. | Both required checks are terminal red. The raw artifact lacks child-peer response/stderr/exit and process/event correlation. | [Raw evidence and owner handoff](https://github.com/portpowered/you-agent-factory/pull/2645#issuecomment-5800402494). |
| CTX-01 | NON-BLOCKING | Check `docs/temp/projects/factory-reliability/source-plan.md` in this worktree. | Referenced source plan is available to loopback. | It is absent; the PRD contains the task scope, evidence procedure, ownership, and acceptance criteria used here. | Current worktree file check; source-plan absence is recorded in the Story 001 notes. |

## Verdict

BLOCKED for the hosted ACP gate and full project acceptance: PR #2645 remains red pending the external Reliability evidence/release event. Story 003's scoped handoff outcome is satisfied by the exact-head red status, the named external owner/release event, and this clean-room report. FR-A3/A5/A9 remain open for independent project validation.

## Delta-plan request

- Affected behavior and criterion: ACP-GATE; FR-A3/FR-A5/FR-A9 local contribution and Story 003 exact-head handoff.
- Root-cause evidence or remaining uncertainty: The Factory Session/Work failures are visible, but the hosted artifact does not identify child-peer response, stderr, exit status, or correlated process/Factory Event timing.
- Smallest recommended correction/prerequisite: Factory Reliability (ACP/CI) captures those fields on the hosted failing path and correlates them to Work/Event timing, then provides a causally supported correction or an exact-head green required-check result. Do not rerun unchanged conditions.
- Dependencies and retest scope: After an ACP-relevant condition changes, use the single authorized changed-condition hosted run and record its exact head/base, raw artifact, and Verification Policy result. Independent review owns terminal CI and merge; LocalAI owns the TTS PR. FR-wide acceptance remains with the listed independent gates.
