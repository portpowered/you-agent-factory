# C09: ACP stdio contention characterization

## Scope and result

- Story: `functional-test-optimization-c09-transport-acp-stdio-contention-001`.
- Parent behavior: preserve the eight P039 ACP stdio process, pipe, and protocol
  witnesses while making their test-local ownership contention-hermetic.
- This document records the immutable hosted failure, the unchanged P039
  inventory, and the smallest local-real characterization completed before any
  test edit.
- Repository head used for local characterization:
  `bdb65eac0115e3d81a3dd55aba5365600e190e20`.
- Result: the story's evidence and characterization branch is complete. The
  hosted diagnostic identifies an observed projection-warning phase, but not
  the later assertion that failed. That uncertainty is recorded as a
  structured blocker below; no test or shared support file was changed.

The planned source note `docs/temp/functional-test-optimization.md` and the
worktree `progress.txt` were not present in this checkout when this story
started. The PRD and the committed c01/c07 evidence are therefore the
authority used here; the missing planning note is a provenance gap, not a
reason to invent a correction.

## EVIDENCE-RAW-01: retained hosted failure

The failure was downloaded from the immutable GitHub Actions run and job
specified by the PRD:

| Field | Value |
| --- | --- |
| Run | [33220840795](https://github.com/portpowered/you-agent-factory/actions/runs/33220840795) |
| Workflow/event | `CI`, `pull_request` |
| Source branch | `functional-test-optimization-c08-ci-concurrency-and-gate-removal` |
| Head SHA | `63937e9878f2d1a1b1d86964c201e24a62d2d1d0` |
| Job | [99014418744](https://github.com/portpowered/you-agent-factory/actions/runs/33220840795/job/99014418744), `Backend Functional Coverage` |
| Job result | `completed`, `failure` |
| Runner | Ubuntu 24.04.4 / `ubuntu-24.04`, functional runner reported `logical_cpus=4`, `jobs=8` |
| Artifact | `functional-test-diagnostics`, artifact ID `9705215279`, 996,972 bytes, not expired |
| Artifact download | `gh run download 33220840795 --name functional-test-diagnostics --dir C:\Users\andre\AppData\Local\Temp\c09-functional-diagnostics-908eb692863c47bba0b98d0ee10444e8` |
| Downloaded-file manifest SHA-256 | `C19A92C009FCBCA9891444DBD195ED698F4AFE819781A32F28D9F057297EB853` |

The manifest hash is over sorted `sha256 length filename` entries for the
downloaded files. It is a reproducible local artifact-content hash, not a
claim that GitHub exposed a separate archive digest.

The artifact's `command.log` contains this exact retained failure line; the
trailing `...` is part of the artifact's truncation:

```text
functional test failure: package=github.com/portpowered/infinite-you/tests/functional/transport/acp/stdio test=TestServeACP_RootBuildProcessCloseThenLoadReplaysRetainedItemIdentities reason={"level":"warn","ts":1787960332.3812165,"caller":"logging/runtime_logger.go:196","msg":"acp skipped malformed worker child projection","op":"ProjectWorkerChild","dispatch_id":"c051d59c-7c73-4832-b435-41bfa9015c8e","worker_session_id":"c051d...
```

The targeted entry in `functional-timing-summary.json` repeats the same
selector, with `seconds=1.53`, `outcome=fail`, and this exact reason prefix:

```text
{"level":"warn","ts":1787960332.3812165,"caller":"logging/runtime_logger.go:196","msg":"acp skipped malformed worker child projection","op":"ProjectWorkerChild","dispatch_id":"c051d59c-7c73-4832-b435-41bfa9015c8e","worker_session_id":"c051d...
```

The package summary in the same diagnostic bundle reports 147 packages, 1,051
tests, 1,048 passes, 2 failures, and 1 skip. The P039 row is the retained ACP
failure at 11.376 seconds. A separate `tests/functional/work/watch` failure is
also present in the bundle and is not attributed to this lane. Coverage was
not evaluated because the run had two functional failures.

The raw files used for this record are independently identifiable by these
SHA-256 values:

| File | Bytes | SHA-256 |
| --- | ---: | --- |
| `command.log` | 28,150 | `7C20519E8C889BE49B9B353FC15B58C11B44A20BAD43F44631ECDEF4F59F8900` |
| `functional-timing-summary.json` | 407,380 | `937B9A011D237DF66E8B6F53876475A32AD864D85A4D2B2638CF8003AE6D4125` |
| `functional-tests.md` | 752,529 | `040E1535F242D684CC6AE610654EDF5A8194385527457AA965F81BEF70DDFD36` |
| `functional-coverage-verdict.txt` | 17,278 | `B5733A329CE3407E970A76CFB1CC89DC979708B5BFF575E4E83D045A742EC574` |

## P039 preservation inventory

The committed c01 inventory contains exactly eight P039 rows. Every row is
classified `isolated-with-reason`; no row was added, removed, or reclassified
for this characterization.

| Selector | Source | Isolation reason |
| --- | --- | --- |
| `TestServeACP_RootBuildProcessProviderFailureTerminalizesPrompt` | `tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go:32` | `provider-protocol` |
| `TestServeACP_RootBuildProcessCancelTerminalizesOnlyCapturedPrompt` | `tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go:69` | `connection` |
| `TestServeACP_RootBuildProcessCloseStopsCapturedFactorySession` | `tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go:104` | `teardown` |
| `TestServeACP_RootBuildProcessCloseThenLoadReplaysRetainedItemIdentities` | `tests/functional/transport/acp/stdio/cli_serve_acp_controls_test.go:138` | `persistence-restart` |
| `TestServeACP_RootBuildProcessCompletesOneFactoryPrompt` | `tests/functional/transport/acp/stdio/cli_serve_acp_prompt_test.go:66` | `provider-protocol` |
| `TestServeACPWritesAWireTranscriptByDefault` | `tests/functional/transport/acp/stdio/cli_serve_acp_wire_log_test.go:42` | `connection` |
| `TestServeACPDoesNotRecordFailedOutboundFrame` | `tests/functional/transport/acp/stdio/cli_serve_acp_wire_log_test.go:466` | `connection` |
| `TestServeACPWireTranscriptIsOwnerReadableOnly` | `tests/functional/transport/acp/stdio/cli_serve_acp_wire_log_test.go:519` | `environment` |

The c01 JSON inventory hash before and after this story is
`AA56F631810E812F34CE167247C11E7741890862ECCF2A7EF1475F530C9C1F80`; its
Markdown companion hash is
`1C4A73A1FEB6AAE330998121BAE565BF2B38CED8933E95BD364177659A4025CF`.
The separate P038 real-ACPX package and its merged PR #2376 are outside this
P039 lane.

All eight tests retain the same executable-spine properties: each constructs a
fresh `root.BuildProcess`, starts the `you server acp` command, uses actual
`os.Pipe` stdin/stdout boundaries, and owns its temporary home/profile,
working directory, process, streams, and cleanup. The tests do not use
`t.Parallel` or a shared process. This story made no test, provider, ACP
production, or shared-support edit.

## CHAR-01: local-real observation

The required exact selector was run once on the local real package:

```text
go test -count=1 -timeout=15m -run '^TestServeACP_RootBuildProcessCloseThenLoadReplaysRetainedItemIdentities$' ./tests/functional/transport/acp/stdio
```

Observed result: exit 0, `ok`, test/package wall time `7.330s`. A diagnostic
JSON run of that same selector was then used to observe lifecycle ordering:

```text
go test -json -count=1 -timeout=15m -run '^TestServeACP_RootBuildProcessCloseThenLoadReplaysRetainedItemIdentities$' ./tests/functional/transport/acp/stdio
```

Observed result: exit 0, test elapsed `14.35s`, package elapsed `14.461s`.
The trace contained the following sequence:

1. `ProjectWorkerChild` logged `acp skipped malformed worker child projection`
   for `SESSION/UPDATED` child records. The two observed item IDs were
   `b247356c-f854-4896-94d5-eb4ec45a74de` and
   `48a7c81f-166a-4139-af73-af0b784dfd8a`; both had parent
   `bb97d5b9-4c56-48f5-85b3-dd23d05e7eee` and dispatch/worker-session
   `f2a5bfbe-da99-4919-a81a-218c0de1df48`.
2. The second active prompt was closed. The runtime logged a canceled
   invocation with status `CANCELED`, code `INVOCATION_CANCELED`, failure class
   `cancellation`, and message `invocation was canceled while waiting for
   primary result`.
3. The same two `SESSION/UPDATED` projection warnings appeared during the
   retained/replay path.
4. The test ended with
   `--- PASS: TestServeACP_RootBuildProcessCloseThenLoadReplaysRetainedItemIdentities (14.35s)`.

This is a concrete observed phase, not a claim that the warning is the failed
assertion. The local run proves that the warning can occur immediately before
successful completion. In addition, the functional diagnostic helper's
`firstFunctionalFailureReason` selects the first nonempty output line for a
failed test and truncates long lines. A warning emitted before a later
`t.Fatalf` can therefore become the retained reason without preserving the
assertion.

The local environment was Windows (`go1.25.0 windows/amd64`, `GOMAXPROCS=auto`,
Windows NT 10.0.26200.0), while the retained failure was Ubuntu with raised
package parallelism. The local pass is useful phase evidence but does not
prove the hosted contention edge or a corrected test.

## Compatibility and ownership matrix

The matrix below carries the c07 transport behavior lanes into this story.
It records ownership and applicability; it is not a claim that story 001
passes the later correction/repeat/package gates.

| PRD criterion | P039 observation or owner | Status at characterization |
| --- | --- | --- |
| `MATRIX-01` handshake and request/response | ACP prompt and control witnesses; broader protocol matrix owns malformed handshakes | Preserved, later repeat gate |
| `MATRIX-02` malformed frame/input | Failed-outbound-frame witness covers output recording; input-malformation cases remain broader ACP transport ownership | No new P039 case |
| `MATRIX-03` provider/executable failure | Provider-failure witness is P039; executable-selection coverage belongs to the separate P038/transport lane | Provider case preserved; executable case out of scope |
| `MATRIX-04` partial completion | Failed outbound frame and prompt terminalization are existing transport evidence | Broader matrix owns full partition |
| `MATRIX-05` early exit | Close-stops-session witness | Preserved |
| `MATRIX-06` cancellation | Cancel and close control witnesses | Preserved |
| `MATRIX-07` timeout guard | Test-owned 5-second hang guards only; no product timeout is introduced | Later correction/repeat gate |
| `MATRIX-08` shutdown | ACP close and process finish cleanup | Preserved |
| `MATRIX-09` stream closure | Actual ACP stdio pipes and wire transcript ownership | Preserved |
| `MATRIX-10` environment | Owner-readable transcript and test-owned temp environment | Preserved |
| `MATRIX-11` platform | Hosted Ubuntu versus local Windows is explicitly recorded | Hosted edge unproven |
| `MATRIX-12` recovery | Close/load retained item identities witness | Preserved; retained path is the reported selector |
| `MATRIX-13` ordering | Wire transcript and ordered close/load assertions | Preserved |
| `MATRIX-14` duplicate/idempotency | Post-completion cancel and repeated close behavior in controls | Preserved |
| `MATRIX-15` authorization/capacity | Owner-readable transcript covers authorization; capacity limits belong to the broader transport matrix | Capacity not a P039 case |

## Structured blocker and smallest next step

**Blocker:** `CHAR-01` cannot identify the hosted test's final assertion or a
precise contention race from the retained artifact. The exact reason stops at
the first `ProjectWorkerChild` warning, and the same warning occurs in a
successful local run. The hosted run's Ubuntu/jobs=8 topology is not
reproduced by the local Windows run.

**Impact:** There is no demonstrated readiness, synchronization, or cleanup
correction target. Editing the eight tests now would be speculative and could
alter their process/pipe/protocol witnesses, violating the story boundary.

**Safe work completed:** the raw artifact and hashes are preserved, the c01
inventory is audited unchanged, and the warning's live/replay lifecycle phase
is characterized without changing tests or shared support.

**Smallest follow-up:** obtain untruncated per-test output/JSON for the same
Ubuntu raised-parallelism selector, or reproduce that selector in the same
contention topology, and identify the actual assertion before applying any
test-local correction. Stories 002 and 003 retain the repeat, race, clean
package, and CI handoff gates after that diagnosis.

