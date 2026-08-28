# Validation report: C07 sessions, Work, and Chat cleanup

## Environment and artifact

- Validated source commit: `790f897e8dbfb6bec813bb4fcf79fdd9af19fb0e`; all
  runtime checks below ran from a detached clean worktree at this exact code
  head. This report refresh is documentation-only and does not change that
  validated source.
- Environment and configuration: Windows `10.0.26100`, `go1.25.0
  windows/amd64`, PowerShell `7.6.5`, repository `go.mod`, no remote or paid
  provider credentials used.
- Customer entry point: local-real `root.BuildProcess` and `Process.Execute`
  through the public CLI, HTTP, Factory Session, Work, Factory Event, and ACP
  boundaries exercised by the three owned functional packages.
- Real and substituted dependencies: real application wiring, filesystem,
  loopback listener, OS processes, and stdio pipes; controlled worker/provider
  effects remain injected only through the existing `edges.Edges` command-runner
  boundaries.
- Cost/call budget used: zero remote or paid calls; maximum cost `$0`.

## Project criteria

| Criterion | PASS/FAIL/BLOCKED | Evidence | Unproven edge |
| --- | --- | --- | --- |
| Final executable denominator | PASS | Clean checkout `go test -list '^Test'` for lifecycle, routing, and chat reported 12 + 1 + 25 = 38 top-level tests. The exact names remain in the c07 characterization artifact. | Whole-project denominator and source-plan full-suite results remain governing-project work. |
| Package behavior and cleanup | PASS | Clean checkout `go test -count=1 -timeout=30m` passed for all three packages: lifecycle `4.717s`, routing `1.625s`, and chat `23.981s`. The verbose count-one census reported zero residue for every owned resource class. | Remote provider behavior and crash/restart cleanup. |
| Repeat-safe teardown | PASS | Clean checkout `go test -count=3 -timeout=30m` passed for lifecycle, routing, and chat at `26.647s/3.814s/70.280s`; all three package bodies completed on every repeat. | Host-wide resources outside the package-owned census. |
| Supported race paths | PASS | Review-follow-up clean-room race commands passed for lifecycle `9.396s`, routing `3.068s`, and full supported chat `59.558s`. No source change was made for any bounded rerun. | Unsupported race-prohibitive paths and remote dependencies. |
| Functional-test construction policy | PASS | Clean checkout `make test-lane-audit` passed and reported `contract=8 functional=140 integration=10 maintenance=106 release=1 stress=1 unit=446`. | CI-only policy enforcement on a future head. |
| Diff hygiene | PASS | Clean detached checkout `git diff --check` passed; the final tracked diff is checked again before commit. | Review-owned merge/conflict state. |
| Performance direction | PASS (directional) | Accepted topology remains measurable: lifecycle uses 2 root processes for 12 tests, routing uses 1 root process for 16 nested scenarios, and chat uses shared cohorts plus 24 owned processes for 25 top-level tests. Review-follow-up clean package results were lifecycle/routing/chat `4.717s/1.625s/23.981s` at count one and `26.647s/3.814s/70.280s` at count three. | Local Windows timing is noisy and has no portable threshold; REV-CI-001 must provide the same-head package timing verdict. |
| VAL-001 clean-room journey | PASS | The detached clean worktree had empty status before removal; discovery, repeats, supported races, policy audit, diff check, public witnesses, final censuses, and the exact full-suite rerun passed. | Remote availability, paid behavior, and restart semantics. |
| Implementation-stage delivery | PENDING HANDOFF | This report is the pre-push validation artifact. Final head push, open PR, required CI start, and review feedback state are recorded at handoff. | Review owns terminal CI, conflicts, and merge. |

## Denominator

The clean-room discovery commands were:

```text
go test -list '^Test' ./tests/functional/sessions/lifecycle
go test -list '^Test' ./tests/functional/work/routing
go test -list '^Test' ./tests/functional/sessions/chat_sessions/root_composition
```

The observed top-level denominator was exactly 38:

| Package | Top-level tests |
| --- | ---: |
| `./tests/functional/sessions/lifecycle` | 12 |
| `./tests/functional/work/routing` | 1 |
| `./tests/functional/sessions/chat_sessions/root_composition` | 25 |
| **Total** | **38** |

The routing top-level test retains 16 nested scenarios, and the chat package
retains the 25 names listed in the characterization artifact. The review
correction relocated the existing Chat root-composition package under the
approved `sessions` functional domain; no test was added, removed, or renamed,
and test behavior remained unchanged.

## Customer journey

1. A detached worktree at the validation head was created with `git worktree
   add --detach`; `git status --short --branch` reported only `## HEAD (no
   branch)`, and the worktree was removed after each bounded validation pass.
2. The three public functional packages were discovered, then exercised with
   the exact count-one and count-three commands below. All exited `0`:

   ```text
   go test -count=1 -timeout=30m ./tests/functional/sessions/lifecycle
   go test -count=1 -timeout=30m ./tests/functional/work/routing
   go test -count=1 -timeout=30m ./tests/functional/sessions/chat_sessions/root_composition
   go test -count=3 -timeout=30m ./tests/functional/sessions/lifecycle
   go test -count=3 -timeout=30m ./tests/functional/work/routing
   go test -count=3 -timeout=30m ./tests/functional/sessions/chat_sessions/root_composition
   ```

3. The final verbose count-one run exposed the package-local cleanup evidence:

   ```text
   lifecycle: process-builds=2 process-closes=2 listener-starts=1 listener-closes=1 invocation-streams-opened=30 invocation-streams-closed=30 sessions-opened=12 sessions-closed=12 live-sessions-absent=9 durable-sessions-terminal=3 temporary-factory-paths-removed=12
   routing: process-builds=1 process-starts=1 process-closes=1 listener-starts=1 listener-closes=1 scenarios=16 scenario-roots-closed=16 factory-paths-removed=16 sessions-opened=16 sessions-closed=16 readers-opened=139 readers-closed=139 routes-registered=18 routes-unregistered=18 active-route-selectors=0 active-controlled-calls=0
   chat: processes=24/24 connections=58/58 response-streams=58/58 pipes=56/56 sessions=29/29 sessions-explicit=0 sessions-process-fallback=29 turns=30/30 active-calls=34/34 peer-processes=4/4 paths-removed=97/97 listeners=0/0 violations=0
   ```

4. The public witnesses retained their established behavior: Factory Session
   list/show/get/close/delete and local/remote parity; Work routing payload,
   lineage, dispatch, rejection, failure, capacity, ordering, and replay;
   and ACP Chat Session turns, target selection, errors, streams, usage,
   child attribution, duplicate suppression, and retained replay.
5. The exact clean-room `make test` rerun exited `0` with 146 passing tests,
   zero failures, and one supported environment skip. An earlier full-suite
   attempt had one unrelated local-real Git fixture assertion; its focused
   replay passed all 30 tests in that file, and the immediate exact rerun
   passed. No source change was made for either rerun.

## Cross-task integration and usability

- Documentation discoverability: the lane characterization and ownership
  inventory remain in `c07-cleanup-sessions-work-chat.md`; this report is the
  VAL-001 loopback artifact.
- Permission and error behavior: the existing public typed not-found,
  transport, cancellation, timeout, routing failure, ACP error, busy, and
  resolver-error assertions passed without contract changes.
- Persistence/reload behavior: lifecycle durable terminal history, Work and
  Factory Event replay, and ACP retained child-stream observations passed.
- Accessibility/keyboard/responsive behavior: not applicable; this lane has no
  UI surface.
- Operational signals: process/listener/stream/session/turn/call/route/reader,
  peer-process, path, public-absence, and violation counters were observed at
  their owning package boundaries.

## Performance and optimization follow-up

The local Go package times are directional only. The review-follow-up count-one
package results were lifecycle `4.717s`, routing `1.625s`, and chat `23.981s`;
count three reported `26.647s`, `3.814s`, and `70.280s`. These values do not
establish a portable timing threshold and are not used to overrule the accepted
topology.

The bounded follow-up for the non-improving local timing direction was a
read-only topology/census check: no new root, listener, process, fixture, or
worker construction was introduced by loopback, and the accepted shared-process
and explicit-isolation classifications remain intact. No implementation repair
was made during validation. REV-CI-001 remains the primary same-head package
performance verdict.

## Findings

| ID | Severity | Reproduction | Expected | Actual | Evidence |
| --- | --- | --- | --- | --- | --- |
| C07-F001 | non-blocking, bounded rerun passed | `go test -race -count=1 -timeout=30m -run=^TestSharedProcessWorkRouting$ ./tests/functional/work/routing` from a clean detached worktree | The supported race path exits `0` and reports a clean census. | The first bounded attempt exited `1` with `race detected during execution of test`; its final census still closed 16/16 sessions, roots, paths, readers, and 18/18 routes with zero active calls. One immediate rerun of the same command exited `0` with no race diagnostic and the same clean behavior. No source was changed. | First attempt and rerun output; prior story-003 focused race evidence also passed. If CI reproduces the failure, review should treat it as blocking rather than infer a pass from this rerun. |
| C07-F002 | informational | Package runs exercise fixture discovery against transient child directories. | Existing diagnostics may be emitted without changing customer-visible behavior. | Routing emitted the known `logicaltarget/discovery.go` missing-root diagnostics; all public assertions and cleanup censuses passed. No shared-support defect was localized and no protected support file was edited. | Existing characterization note and clean package output. |
| C07-F003 | non-blocking, reproduced cleanly on rerun | Exact clean-room `make test` at the validated source commit | The repository suite exits `0`. | The first full attempt had one unrelated local-real Git fixture assertion; the focused replay passed all 30 tests in that file, and the immediate exact full-suite rerun exited `0` with 146 passes, 0 failures, and 1 supported skip. No source changed between runs. | Clean-room command output from the validated source commit; review should use the final rerun result. |

## Verdict

PASS for VAL-001 local clean-room validation and the c07 implementation evidence.
The implementation handoff remains review-stage work: push the final head, open
the PR, start required CI, and stop without waiting for terminal CI.

## Remaining unproven edges

- `REV-CI-001`: same-head Backend Functional Coverage terminal result and
  package performance comment, owned by review.
- `EXISTING-PROVIDER-INTEGRATION-RELEASE`: real remote provider availability
  and credentials, intentionally excluded by the zero-call budget.
- `EXCLUDED-STABILITY-RESTART`: crash/restart cleanup, outside this lane.
