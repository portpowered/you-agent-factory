# Validation report: C07 sessions, Work, and Chat cleanup

## Environment and artifact

- Validated source commit: `4c124e4d5566ad800380ebf20ae694165ba7f5e9`; all
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
| Package behavior and cleanup | PASS | Clean checkout `go test -v -count=1 -timeout=30m` passed for all three packages: lifecycle `5.856s`, routing `1.460s`, and chat `24.801s`. The verbose count-one census reported zero residue for every owned resource class. | Remote provider behavior and crash/restart cleanup. |
| Repeat-safe teardown | PASS | Clean checkout `go test -count=3 -timeout=30m` passed for lifecycle, routing, and chat at `33.944s/4.194s/168.651s`; all three package bodies completed on every repeat. | Host-wide resources outside the package-owned census. |
| Supported race paths | PASS | Clean checkout race commands passed for lifecycle `7.852s`, routing `3.001s`, and full supported chat `82.540s`. No source change was made during validation. | Unsupported race-prohibitive paths and remote dependencies. |
| Functional-test construction policy | PASS | Clean checkout `make test-lane-audit` passed and reported `contract=8 functional=140 integration=10 maintenance=106 release=1 stress=1 unit=446`. | CI-only policy enforcement on a future head. |
| Diff hygiene | PASS | Clean detached checkout `git diff --check` passed; the final tracked diff is checked again before commit. | Review-owned merge/conflict state. |
| Performance direction | PASS (directional) | Accepted topology remains measurable: lifecycle uses 2 root processes for 12 tests, routing uses 1 root process for 16 nested scenarios, and chat uses shared cohorts plus 24 owned processes for 25 top-level tests. Exact-head clean package results were lifecycle/routing/chat `5.856s/1.460s/24.801s` at count one and `33.944s/4.194s/168.651s` at count three. | Local Windows timing is noisy and has no portable threshold; REV-CI-001 must provide the same-head package timing verdict. |
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
   branch)`, and the worktree was removed after the complete validation pass.
2. The three public functional packages were discovered, then exercised with
   the exact count-one and count-three commands below. All exited `0`:

   ```text
   go test -v -count=1 -timeout=30m ./tests/functional/sessions/lifecycle
   go test -v -count=1 -timeout=30m ./tests/functional/work/routing
   go test -v -count=1 -timeout=30m ./tests/functional/sessions/chat_sessions/root_composition
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
5. The exact clean-room `make test` rerun exited `0` with the runner summary
   `1..153`, `# tests 153`, `# pass 152`, `# fail 0`, `# skipped 1`, and
   `# duration_ms 34431.658`. No source change was made during the run.
6. The remaining clean-room gates all exited `0`:

   ```text
   make test-lane-audit -> contract=8 functional=140 integration=10 maintenance=106 release=1 stress=1 unit=446
   make backend-size -> all owned Go backend files are within file limit 1000 and function limit 100
   make pkg-structure -> service and functional package structure holds (532 deletion-only baseline entries remain)
   git diff --check -> no output
   ```

   The supported race commands were:

   ```text
   go test -race -count=1 -timeout=30m -run '^(TestFactorySessionCleanupRunsAfterEarlySubtestExit|TestCLIRemoteRunStartsDurableSessionOnSelectedServer|TestCLILocalAndRemoteRunCancellationParityThroughRootProcess)$' ./tests/functional/sessions/lifecycle
   go test -race -count=1 -timeout=30m -run '^TestSharedProcessWorkRouting$' ./tests/functional/work/routing
   go test -race -count=1 -timeout=30m ./tests/functional/sessions/chat_sessions/root_composition
   ```

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

The local Go package times are directional only. The exact-head count-one
package results were lifecycle `5.856s`, routing `1.460s`, and chat `24.801s`;
count three reported `33.944s`, `4.194s`, and `168.651s`. These values do not
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
| C07-F002 | informational | Package runs exercise fixture discovery against transient child directories. | Existing diagnostics may be emitted without changing customer-visible behavior. | The exact-head routing run retained the known discovery boundary; all public assertions and cleanup censuses passed. No shared-support defect was localized and no protected support file was edited. | Existing characterization note and exact-head clean package output. |

## Verdict

PASS for VAL-001 local clean-room validation and the c07 implementation evidence.
This report records the complete read-only validation from source commit
`4c124e4d5566ad800380ebf20ae694165ba7f5e9`; the implementation handoff still
requires the report refresh to be pushed and required CI to be started on that
new head.

## Remaining unproven edges

- `REV-CI-001`: same-head Backend Functional Coverage terminal result and
  package performance comment, owned by review.
- `EXISTING-PROVIDER-INTEGRATION-RELEASE`: real remote provider availability
  and credentials, intentionally excluded by the zero-call budget.
- `EXCLUDED-STABILITY-RESTART`: crash/restart cleanup, outside this lane.
