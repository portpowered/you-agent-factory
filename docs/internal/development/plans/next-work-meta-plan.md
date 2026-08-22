# Next Work — Meta Plan, WIP State, and Program Verification Probes

---
author: operator
last modified: 2026, august, 22
doc-id: PLAN-META-001
status: proposed
---

# problem statement

Four programs reported complete while an adversarial evaluation found the capabilities they were meant to deliver absent or unverified, and we have no mechanism that re-opens a program when its delivered capability fails in customer posture.

## customer ask

Say what is in progress, what is next, and add probes that induce a loopback when newly generated work — cost modelling, observability, resume, local models — does not actually succeed.

## solution

Sequence the four implementation plans behind one blocking prerequisite, and give every completed program an adversarial probe whose negative verdict automatically produces remediation work rather than a report nobody acts on.

# original document

Companion plans in this directory:

- `ci-integration-test-matrix.md`
- `javascript-workflow-parity-and-permissions.md`
- `docs-generation-from-testable-sources.md`
- `mock-worker-validation-and-failed-states.md`

## Part 1 — The systematic failure, named

Four programs merged cleanly and were reported complete. An independent evaluation then
found:

- **no token or cost reporting anywhere in the product**, after the cost lanes had merged
- **checkpoint/resume untested**, with no CLI entry point that could be exercised at all
- the entire **MCP surface unreachable** from an installed binary
- **`session list --scope persisted`** dead for the same reason
- two shipped **catalog factories that do not load**

These are not four unrelated misses. They share one cause:

> **Every acceptance criterion was satisfiable from inside the repository.**

Lanes proved their code correct with unit and functional tests run from a repo checkout,
where `pkg/transports/http/testdata/` exists, where the working directory has a parent
containing a `go.mod`, and where fixtures resolve. Nobody ran the shipped binary from a
directory that has none of those things. The tests were not wrong; they were measuring a
posture no customer occupies.

That is the same root cause as the integration-test gap, at a different altitude. The
integration tier fixes it going forward. The probes below fix it retroactively, for work
already merged.

**Corollary for future planning:** an acceptance criterion that can be satisfied without
leaving the repository is not evidence of delivery for any capability a customer reaches
through the installed binary.

## Part 2 — Work in progress

State as of 2026-08-22, reconstructed from GitHub because the daemon is down.

### Infrastructure

The factory has been down since roughly 08:30Z. `bin/you.exe` was built at 04:39Z and
`main` moved at 08:09Z, so **the binary must be rebuilt before any restart** — a merged
schema change against a stale binary fails at startup in a way that is indistinguishable
from a broken config. 125 worktrees hold no unpushed commits, so nothing is stranded.

### Open PRs — 24

**Clean and mergeable now (8):** #2132 (lmx-p3b), #2133 (perf-c1), #2161 (tts contract),
#2162 (obs-13), #2163 (operator coverage floors), #2170 (lint deadcode gate), #2172
(deflake worker cancel), #2173 (ci current-merge lint gate). These are finished work
blocked only because review is a factory workstation and the factory is dead.

**Conflicted (2):** #2158, #2171.

**Blocked (14) — three causes, not fourteen:**

| Cause | PRs | Owner |
|---|---|---|
| Functional flake cluster on main | #2150, #2151, #2169, and contributing to #2152, #2155, #2174, #2175, #2176 | Baseline, not the lane |
| Backend Lint — head predates #2165 (`cmd/gocoveragecheck/main.go` 1030 > 1000) | #2128, #2146 | Legitimate rebase; the comment must name #2165 |
| Backend Lint — lane's own oversized file | #2177 (1242 lines), #2178 (137 > 100), #2154, #2175 | Lane; mechanical split |

`Verification Policy` red is **derived** — it is green on healthy PRs and must never be
diagnosed as an independent cause.

### Main is ~40% green

Of the last ten uncancelled CI runs on `main`, four passed and six failed, each failing a
*different* test: `TestPackagedFactoryInvokedByCLICanBeInspectedByAPI`,
`TestCancel_BeforeBoundaryAdmissionEitherWaitsOrTerminatesTheExactSupervision`,
`TestRunReportsPackagedFactorySchemaDriftWithRegenerationRemedy` (twice),
`TestACPExecuteObservesProviderSessionWhileAttemptIsLive`; one red run failed outside the
test set. On lanes additionally `TestLargeRolloutNeverFailsWithoutCause` — on two
unrelated diffs, which makes it baseline-owned by definition — and
`TestBTRCP0OneShotCancellationCharacterization`.

A rotating cast of failures is scattered async flake, not one bad test. Every lane
inherits a coin-flip chance of a red required check it did not cause. **This is the
throughput ceiling and it sits upstream of every program in this document.**

## Part 3 — Standalone lanes that block the plans

Three defects are not part of any plan above but gate them.

**SEC-1 — READ_ONLY does not sandbox anything.** Provider adapters append
`--dangerously-bypass-approvals-and-sandbox` (codex) and `--dangerously-skip-permissions`
(claude, agy) gated on `SkipPermissions`, while the capability validators are referenced
only inside their own package. Detail and remedy in
`javascript-workflow-parity-and-permissions.md`. **This is security-shaped and must not
ride behind a test-infrastructure program** — it lands on its own schedule.

**FIX-1 — Bundle or remove the fixture-catalog dependency.**
`pkg/services/factory_sessions/execution_contract.go:28` and
`internal/executionopening/factory.go:305` make `mcp serve`, `session list --scope`, and
`session dispatches` unreachable from an installed binary. Blocks integration tests I4 and
I5, and is the clearest instance of the class the integration tier's no-internal-files rule
exists to catch: an entrypoint that works only inside our repository.

**FLAKE-1 — Deflake the main functional tier.** Six named tests above. Higher priority
than any feature program, because it taxes all of them. The prepared batch
`docs/temp/batches/2026-08-22/flakepolicy.json` also fixes the reviewer's flake escape
hatch, which currently requires reproducing a CI-only flake on the base SHA — an
unsatisfiable condition that forces correct reviewers to block healthy lanes.

## Part 4 — Sequencing

```
FLAKE-1 ──────────────────────────────► (unblocks everything, do first)
SEC-1 ────────────────────────────────► (independent, security)

INT Story 0 (truthful batch exit codes)
   ├─► MOCK-1..2 ──► MOCK-3..5
   ├─► INT harness + I8 ──► INT job wired ──► I1,I2,I3 ──► I4
   └─► DOC-6
FIX-1 ────────────────────────────────► INT I4, I5
REC-6 ────────────────────────────────► INT I6   (resume reads the recording)
EXT-5 ────────────────────────────────► INT I7   (replay drift is an error)

DOC-1 ──► DOC-2 ──► DOC-3 ──► DOC-4 ──► DOC-5
                              (DOC-4 == JS-P7)

JS-P1 ──► JS-P2 ──► JS-P3 ──► JS-P4     (permissions strangler)
JS-P5, JS-P6                             (parity, independent)
```

**Integration Story 0 is the single highest-leverage item after FLAKE-1.** Until batch
mode exits non-zero on a failed work state, every end-to-end assertion anyone writes is
green by construction — including the probes in Part 5.

## Part 5 — Program verification probes with induced loopback

### Shape

Each probe is one lane that:

1. Runs from a **blanked-out working directory** with the **installed binary** — no
   repository on any parent path, `HOME` pointed at a fresh directory.
2. Attempts the capability the way a customer would.
3. Emits a **structured verdict**: `PASS` / `FAIL` / `INCONCLUSIVE`, with the exact
   command, output, and the falsifier it applied.
4. On `FAIL`, writes a **remediation batch file** naming the specific lanes needed.

A single `thoughts` work, `program-verification-loopback`, `DEPENDS_ON` all four probes.
Its mission is to read every verdict and submit the remediation batches. That is the
induced loopback: a failing probe produces work automatically rather than a report that
waits for someone to notice.

**`INCONCLUSIVE` is a first-class verdict and must be treated as `FAIL` by the loopback.**
The resume capability was recorded as unverified rather than working, and an unverifiable
capability is not a delivered one. A probe that cannot reach the capability has found
something — usually a missing entry point — and that is remediation work.

### Design rules

- **Adversarial default.** Each probe assumes the capability does not work and must
  produce positive evidence. "No error" is not evidence.
- **Named falsifiers.** Every check states what result would prove failure, in advance.
  The dominant failure mode in this codebase is a field that is present but always empty —
  `schemaValidated: false`, `subject: ""`, a cost of zero after a billed call. A probe
  that only checks for presence will pass on exactly the defect we are hunting. **Assert
  values, never presence.**
- **No live-system mutation.** Probes run against their own instance in their own
  directory. The live daemon on port 7437 is off limits beyond read-only queries.
- **Bounded.** Each probe is timeboxed and reports partial results with explicit gaps
  rather than hanging.

### PROBE-COST — cost modelling

Covers obs-2, obs-9, obs-12, obs-13.

| Check | Falsifier |
|---|---|
| A dispatch that consumed tokens reports a non-zero cost | Cost absent, or zero where a call occurred |
| Session rollup equals the sum of its dispatch costs | Rollup present but zero, or disagreeing with its parts |
| The price table resolves the provider and model actually used | Falls back to a default while reporting success |
| A partially-known cost is marked partial | Reported as a complete zero |
| `metrics` CLI surfaces the same numbers as the API | Two surfaces disagree |

### PROBE-OBS — observability

Covers obs-1, obs-3 through obs-8, obs-10, obs-11.

| Check | Falsifier |
|---|---|
| Metrics reader returns non-empty data after a run | Empty, or a 500 |
| Error codes are coded, not free text | A generic string where a code is contracted |
| The model captured on the dispatch route equals the model actually invoked | Captured field empty, or the requested rather than resolved model |
| Worker sessions carry provider attribution and session scope | Field present, always empty |
| Turn count and context growth are non-zero after a multi-turn run | Zero after a run that clearly had turns |
| Every metrics field declared in the contract is populated by at least one real run | A declared-but-dead field — the `schemaValidated` shape |

### PROBE-RESUME — checkpoint and resume

Covers rsm-1 through rsm-9. **Highest operational value of the four**: this capability was
recorded as untested, and we lost a ~185-work board to a machine restart on 2026-08-22.

| Check | Falsifier |
|---|---|
| A CLI entry point exists that exercises genuine crash-resume | No entry point — records `INCONCLUSIVE`, which the loopback treats as `FAIL` |
| Hard-kill mid-run, restart with resume: the board is restored | Board empty, or short |
| In-flight work resumes rather than restarting from zero | Work re-dispatched from the beginning |
| The resumed run reaches the same terminal states as an uninterrupted control | Divergent outcomes |
| Durable session state survives when the process is killed without cleanup | Corrupt or unreadable state |

This probe's result also determines the priority of #2128 (`fr-board-persistence`), which
is currently blocked.

### PROBE-LOCALMODELS — local models

Covers lmx-p0 through p3b, lai-p0, lai-p05, lai-t1, tts.

| Check | Falsifier |
|---|---|
| Model pull and readiness from an empty model cache | Reports ready without the asset present |
| TTS → STT roundtrip preserves the input text within tolerance | Empty output, or silent fallback to a remote provider |
| Embeddings return vectors of the declared dimension | Wrong dimension, or zeros |
| Backend artifacts resolve to pinned versions | Floating resolution while reporting pinned |
| A missing backend fails loudly | Silent fallback, or a success verdict with no model |

### Submission shape

One batch: four `idea`-typed probe lanes plus one `thoughts`-typed
`program-verification-loopback` with a `DEPENDS_ON` edge to each probe at
`requiredState: complete`. Cross-batch dependency, if ever needed, uses `targetWorkId`
(`batch-<requestId>-<name>`) rather than `targetWorkName`, which raises an ambiguity
error when the name already exists on the board.

Probes must not be scheduled until integration Story 0 has merged. Before that, a probe
running a factory in batch mode receives exit 0 regardless of outcome and will report
`PASS` on a broken capability — reproducing the exact failure this whole document exists
to stop.

## Part 6 — Immediate operator actions

1. Rebuild `bin/you.exe`, restart the daemon. Nothing moves otherwise, and eight finished
   PRs are waiting on review.
2. Submit `flakepolicy.json` (FLAKE-1), extended with the four newly identified tests.
3. Unblock the cheap ones: name #2165 to #2128 and #2146; tell #2154, #2175, #2177, #2178
   to split their oversized files.
4. Submit SEC-1 as its own lane.
5. Submit integration Story 0.
6. Merge #2162 to close observability; land perf-c1/f5/u6/u7 to close the three-minute
   test target.
7. Schedule the probe batch only after Story 0 merges.

## Part 7 — Second-evaluation extensions

Two further plans were added on 2026-08-22 after the 41-agent evaluation
("The Gigabyte Session File"), and they change the sequencing above.

- `recordings-as-single-session-artifact.md` (`PLAN-REC-001`) — retire durable sessions,
  move recordings to the `YYYY/MM/DD/<session-id>.jsonl` layout that logs and metrics
  already use, and bound them with the rotation machinery already in the binary. **REC-0
  is the highest-priority lane in this document**: it caps the unbounded failure log and
  makes the durable write atomic, and it is the fix for a measured, reproducible outage in
  which a session file reached exactly 1 GiB and left the server unable to start.
- `adversarial-findings-extensions.md` (`PLAN-EXT-001`) — routes every material finding to
  one owning plan and designs the six that had no owner: the default-session delete that
  kills the server, the single error code absorbing twenty root causes, unknown fields
  warning instead of failing, `you metrics` ignoring `--server`, replay serving stale
  output over detected drift, and secrets stored verbatim in recordings.

Both reinforce Part 1 rather than contradicting it. Every one of those defects is
reachable only by running the shipped binary as a customer would; none is visible from
inside the repository. The corpus of failures keeps arriving from the same place because
that is the only place we are still not looking.

### Amendments to Part 6

Insert as item 0: **submit REC-0**. It is independent of the daemon restart, of the flake
policy, and of Story 0, and it is the only item here with a reproduced outage behind it.

Add as an operator action, not a lane: **correct the claim that codex requires a git
repository**, which is written into an operator skill document and was refuted by this
round. A false finding in a briefing document costs a lane the same as a real defect.

## Delivery loop

Every lane derived from this document: implementation finishes when its final head is
pushed, the PR is open, CI has started, and blocking review feedback is addressed. Review
owns terminal-and-passing CI, conflict resolution, and merge. CI-run evidence goes in a
PR comment, never a commit.
