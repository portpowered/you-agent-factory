# Validation report: work move restore board occupancy

Status: VAL-001 PASS under the operator amendment. The supplied recording was
replaced by a reviewer-regenerable recording from the committed isolated rig.

Date: 2026-08-30

## Environment and artifact

- Runtime implementation SHA tested: c39662edbdb4dd592359fad497e08877228ea1d6.
- Platform: Windows NT 10.0.26200.0, windows/amd64, Go go1.25.0.
- The fixed binary was built with go build -o <scratch>\bin\you-verify.exe
  ./cmd/factory. It exited 0, was 76,047,872 bytes, and had SHA-256
  09E37B4F2CE4B91AEE3BB15A97A8BBDE598513EB71BD025A84D305C37BB7C189.
- Bun 1.4.0 and the frozen UI lockfile were used for the repository gates.
- The operator amendment authorizes a generated recording and makes the
  original 20,502-event/247-Work counts non-binding. No raw recording,
  prompt, model output, or secret was committed.
- Generated source prefix: agent-factory.replay.v1, 15 events, 185,985 bytes,
  SHA-256 D9A7853E0F9313615A618BB6302FE60614E57222F74069C0F1731A6762494CB4.
- Fixed-head successor recording: agent-factory.replay.v1, 15 events,
  185,985 bytes, SHA-256
  805D04533AF73776F5CD143979AE11028F32265C8ECF0294B55B6E0EEF90DEAC.
- All calls were local and used mock workers or controlled command-runner
  edges. Paid or remote provider calls: zero.

## Project criteria

| Criterion | Result | Evidence | Remaining edge |
| --- | --- | --- | --- |
| Compact functional, repeat/race, and sibling behavior | PASS | Runtime five-selector suite, Recordings three-selector suite, new functional package (3 tests), ten-repeat run, race run, and the existing sibling selector all exited 0. | Other unrelated recording shapes were not replayed. |
| Generated binary resume and exact public Work list | PASS | The fixed binary resumed the generated 15-event prefix and public work list returned one result: the named Work, type idea, state complete/TERMINAL, confirmation CONFIRMED. | The amendment intentionally waives the unavailable supplied artifact. |
| Current occupancy precedence and preserved failure classes | PASS | Current projected idea:complete occupancy won over two retained historical idea:to-complete moves; duplicate current occupancy, invalid topology/reference, and unsupported missing occupancy remain covered by focused tests. | No production recording outside the generated incident shape was needed for this fix. |
| Repository quality gates | PASS with attributed findings | git diff --check, backend-size, pkg-maint, mcp-contract-check, focused Go tests, UI typecheck, UI tests, UI lint, and UI dead-code checks passed. The second make verify-fast run exited 0 with 169 tests passed and 1 supported skip. make lint exited 1 only because shared deadcode findings were 3073 versus the recorded baseline 3074; no baseline was edited. | Review/CI owns the terminal repository result and any baseline disposition. |
| VAL-001 exact real journey | PASS under amendment | The generated recording was produced by the isolated rig, exercised two successful moves, was hashed, resumed by the fixed built binary, and was checked through public work list and successor recording. | The original supplied recording remains unavailable and is not required by the amendment. |
| Final implementation handoff | PENDING at report generation | The final rebase, push, PR creation, CI start, and feedback check are the remaining handoff actions for this iteration. | Terminal CI and merge belong to review. |

## Customer journey

The committed rig inputs are:

- docs/internal/development/fix-work-move-corrupting-board-occupancy-rig-factory.json
- docs/internal/development/fix-work-move-corrupting-board-occupancy-rig-work.json

The generated source run used an explicit copied factory, isolated HOME and
USERPROFILE directories, and loopback port 127.0.0.1:7439. Ports 7437 and
7438 were not accessed. After the Work appeared at idea:complete, both
operator moves returned exit 0:

    you-verify.exe --server http://127.0.0.1:7439 --json work move batch-board-occupancy-repro-request-current-terminal-after-move to-complete --request-id board-occupancy-move-1
    you-verify.exe --server http://127.0.0.1:7439 --json work move batch-board-occupancy-repro-request-current-terminal-after-move to-complete --request-id board-occupancy-move-2

Each accepted dispatch returned the Work to complete/TERMINAL. The source
prefix was closed only after the second accepted dispatch was durable.

The fixed binary command was:

    you-verify.exe run --dir <scratch>\factory --continuously --with-server --listen 127.0.0.1:7439 --resume <scratch>\source-prefix.replay.json --record <scratch>\fixed-successor.replay.json

The public list request exited 0 after deterministic polling. It returned a
single result for
batch-board-occupancy-repro-request-current-terminal-after-move with
workTypeName idea, state name complete, state type TERMINAL, and confirmation
state CONFIRMED. The target occurred once; the generated recording contained
one Work total. The list synchronization completed in 0.249 seconds.

## Generated event order and values

| Sequence | Event | Observed value |
| ---: | --- | --- |
| 4 | WORK_REQUEST | The incident Work was created at idea:complete. |
| 5 | WORK_STATE_CHANGE | Move 1: idea:complete to idea:to-complete. |
| 6 | DISPATCH_REQUEST | The incident Work was dispatched. |
| 9 | DISPATCH_RESPONSE | ACCEPTED; output Work was complete/TERMINAL. |
| 10 | WORK_STATE_CHANGE | Move 2: idea:complete to idea:to-complete. |
| 11 | DISPATCH_REQUEST | The incident Work was dispatched again. |
| 14 | DISPATCH_RESPONSE | ACCEPTED; output Work was complete/TERMINAL. |

The final projection had exactly one current idea:complete occupancy. Both
move records remained in ordered Work-state-change history, with destination
idea:to-complete. The detached parent binary produced the expected
value-level conflict for this same generated state:

    Work "batch-board-occupancy-repro-request-current-terminal-after-move" has conflicting current places "idea:complete" and "idea:to-complete"

The fixed resolver treats the current projection as authoritative and uses
historical move placement only when current and active-dispatch placement are
absent.

## Verification commands and observations

- Runtime owner selectors exited 0; all five selected tests passed.
- Recordings projection selectors exited 0; all three selected tests passed.
- go test ./tests/functional/sessions/board_occupancy_resume -count=1 -v
  exited 0; 3 tests passed in 2.178 seconds.
- The same functional package with -count=10 exited 0 in 45.929 seconds.
- The same functional package with -race -count=1 exited 0 in 10.221
  seconds.
- The existing sibling automation recovery selector exited 0 in 2.024
  seconds.
- No functional test spawned an OS process and no ratchet baseline changed.
- make verify-fast exited 0 on its bounded rerun: 169 passed, 1 supported
  skip. Its first post-install run exposed one shared full-suite fixture
  interaction; the exact named selector passed independently, and the
  bounded rerun passed without a code change.
- make lint exited 1 after all UI and static checks passed because the shared
  deadcode report observed 3073 findings against a 3074-finding baseline.
  This is recorded as existing baseline drift; the baseline was not changed.
- The scratch directory, isolated home, recordings, logs, and built binary
  were removed after the run. The source and successor processes joined and
  port 7439 was free. Four runtime log/metrics paths were all under scratch;
  the fixed resume emitted one safe historical-move-superseded warning.

## Findings and remaining edges

The generated artifact proves the incident-shaped local-real journey and
reproduces the parent failure by value. It does not claim the unavailable
supplied recording or broader unrelated recording populations. The only
repository findings are the attributed full-suite harness interaction from
the first fast-gate run and the shared deadcode baseline drift described
above; neither caused a product regression in this change.

CI-run evidence will be recorded in the pull-request conversation, never in
this repository. Review owns terminal CI, merge conflicts, and merge after
the implementation handoff.

## Verdict

PASS under the operator amendment, pending the final rebase/push/PR/CI-start
handoff.
