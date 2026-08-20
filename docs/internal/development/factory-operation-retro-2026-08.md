# Factory Operation Retrospective — 2026-08-09 to 2026-08-19

Operator record for the run that closed the BTRC, WSE, WSR, DWRO, UX, and DCP
programs. Written at `b947a8525`.

This document is history plus a forward list. Sections 1-2 are the delivery
record. Section 3 is the architecture as it stands. Section 4 is what remains.
Sections 5-7 are the operator-side findings — the instrument failures that cost
the most time and the instruments that should exist before the next run.

Numbers here were measured on the dates given and go stale. Re-measure before
acting on any of them.

---

## 1. Scope

| | |
|---|---|
| Period | 2026-08-09 through 2026-08-19 |
| PRs merged | 232 (`#1817` .. `#2071`) |
| PRs merged since 2026-08-01 | 377 |
| Open PRs at close | 0 |
| Board state at close | drained; daemon on port 7437 down |
| Lane worktrees accumulated | 228 |

The original operating goal was: complete the BTRC, WSE, and UX programs, clear
backend blockers preventing PRs from completing, and make delivery faster. All
three programs closed, plus WSR, DWRO, and DCP which were pulled in behind them.

## 2. Delivery record

Merges by lane-branch prefix over the period:

| prefix | merges | what it was |
|---|---:|---|
| `thr` | 103 | throughput / developer experience — deflakes, lint and build speed, file splits, gate fixes |
| `wo`, `wsr`, `wse` | 34 | worker and session decomposition programs |
| `ux`, `gfxux`, `gfx` | 17 | dashboard and graph editor |
| `dcp` | 14 | decoupling program — transports move-downs, registry removal, cutovers |
| `btrc` | 13 | the original goal program |
| `dwro` | 6 | worker runtime |
| other | ~45 | ci, cov, pf, sac, cli, obs, prose, operator lanes |

**44% of merged work was tooling on tooling.** That was the correct allocation
under concurrency — a flake on `main` taxes every lane in flight, not only the
lane that hits it — but it is the honest characterisation of the period. The
run was mostly about making a 16-slot factory capable of sustained operation.

### What worked

**Whole-program DAGs submitted as one idea-typed batch.** Dependents sit at
`init/INITIAL` until the upstream PR merges; the gated lane's setup-workspace
then runs against a `main` that already contains the upstream work. Near-zero
operator latency between stages, and it is self-healing — restorations keep the
lane name, so the DAG re-arms.

**Flakes treated as first-class lanes, not as retries.** This is why 103 `thr`
lanes was defensible: each deflake compounds across every concurrent lane.

**Measurement before payload.** Lanes that landed cleanly had before-numbers,
`file:line`, and the exact hot spot already in the payload. Lanes that had to
rediscover facts burned 2h sessions doing it.

**Strangler-shaped migrations.** Go type aliases (`type X = pkg.X`) make a
vocabulary rehome genuinely zero-churn: the alias is the identical type,
consumers do not move, and the final lane deletes the shim. Use `var F = pkg.F`
for functions — a `func F(...) { return pkg.F(...) }` wrapper trips the deadcode
ratchet as an exported wrapper.

### What did not work

The dominant failure mode was **instruments that reported a result without
reporting their own scope**. That is documented in section 5; it is the main
transferable finding from this run.

Three other recurring costs:

- **Payload wording.** Acceptance criteria that made the PROCESS stage own
  "PR MERGED" cost roughly 31 agent-hours on a single lane. The processor cannot
  merge — review owns that — so it babysat CI in 2h loops, and each "CI evidence"
  commit created a new head that invalidated the run it had just recorded.
- **Worktree accumulation.** 144 to 228 over the period, with no reaper. Nothing
  broke, but Windows file locks make removal unreliable and it is pure growth.
- **`CLAUDE.md` drift.** It names 14 service families under `pkg/services/`.
  There are 18 (section 3). Every lane reads that file as its map.

---

## 3. Architecture as it stands

Six top-level package families: `initializer`, `platform`, `root`, `services`,
`transports`, `wire`.

### Service inventory and size

Non-test Go LOC, measured 2026-08-19 at `b947a8525`:

| service | LOC | | service | LOC |
|---|---:|---|---|---:|
| `factory_sessions` | 55,746 | | `providers` | 16,123 |
| `factory_runtime` | 44,436 | | `automations` | 9,456 |
| `factory_definitions` | 38,324 | | `factory_visualization` | 7,771 |
| `recordings` | 32,368 | | `operator_settings` | 7,534 |
| `workers` | 21,772 | | `provider_sessions` | 5,979 |
| `work` | 21,078 | | `chat_sessions` | 5,359 |
| `models` | 17,913 | | `events` | 2,126 |
| `worker_sessions` | 17,781 | | `system_initialization` | 953 |
| | | | `webhooks` | 813 |
| | | | `edges` | 711 |

**`CLAUDE.md` does not list `chat_sessions`, `events`, `webhooks`, or
`worker_sessions`.** `worker_sessions` alone is 17,781 lines — larger than
`providers`. Fixing that list is a ten-minute change with outsized value,
because it is the map every lane starts from.

### The decomposition is sound

Services own their internals, `edges` is a genuine 711-line port list rather than
a service locator, and the ownership registries gate for real. What remains is
not "decompose further" — it is three specific defects in the shape, plus the
transports move-downs.

### Defect 1 — host effects leak across service roots

`pkg/services/workers/command.go:12` declares `CommandRunner`. It is a second
declaration of a port that already exists at
`pkg/platform/process/command.go:17` with an identical method set.

Measured 2026-08-19, non-test references to `workers.CommandRunner` /
`CommandRequest` / `CommandResult`:

| package | files |
|---|---:|
| `pkg/services/workers` | 12 |
| `pkg/services/factory_sessions` | 8 |
| `pkg/services/factory_runtime` | 6 |
| `pkg/services/automations` | 6 |
| `pkg/services/recordings` | 4 |
| `pkg/wire` | 3 |

The rule that should hold, and does not:

> An exported interface on a service's public root whose method set is a *host
> effect* — process execution, filesystem, clock, network, PTY — may be
> referenced by that service, by `pkg/services/edges`, and by `pkg/wire`. It must
> never be referenced by a peer product service. A peer that needs the effect
> takes it from `pkg/platform/*`.

Under that test **only 3 of 20** port-shaped interfaces violate:
`workers.CommandRunner`, `workers.PTYAllocator`, and
`factory_runtime.InputFileSystem`. The other 17 are consumer-defined narrow
interfaces on their own owner and are fine. Of the four peer services holding
the leak, two are pure DI plumbing that disappears and two want a platform port
that already exists — **no consumer actually needs Workers.**

### Defect 2 — the request shape is still engine-shaped

`workers.CommandRequest` carries 5 subprocess fields (`Command`, `Args`,
`Stdin`, `Env`, `WorkDir`) against 9 dispatch and identity fields (`DispatchID`,
`TransitionID`, `WorkerType`, `WorkstationName`, `ProjectID`,
`CurrentChainingTraceID`, `PreviousChainingTraceIDs`, `Execution`,
`InputTokens`/`InputBindings`). It is a work dispatch wearing a subprocess
interface.

26 Petri-vocabulary occurrences (`PlaceID`, `TransitionID`, `Marking`,
`TokenID`) remain on the `work` and `workers` public roots, against a 101-entry
`docs/internal/baselines/petri-public-surface-baseline.json` ratchet.

**Sequencing matters:** fix Defect 2 before or with Defect 1. Privatising the
port without fixing the shape merely hides the wrong shape.

### Defect 3 — the boundary linters cannot see test edges

All three checkers filter `_test.go` before building the import graph:
`cmd/pkgboundarycheck/main.go:1082,1658,1899,1958`,
`constructed_service_edges.go:58`, `initializer_behavior.go:54`;
`cmd/ownershipboundarycheck/main.go:209`, `peer_import.go:126`;
`cmd/packagetargetmanifestcheck/inventory.go:44`.

They do not conflate the two classes — they are blind to test edges entirely.
Nothing reports that a test-only edge exists, and nothing labels an observed
edge as one class or the other.

This produced a false architectural blocker. A hand grep for
`providers -> workers` returns 10 files; **all 10 are `_test.go`**. Production
count is 0, and the real production direction is `workers -> providers` across
20 files — already the direction the architecture wants. Lane `dcp-7` recorded
the grep result as a structural blocker and deferred a story on it; the operator
then repeated the claim before re-checking.

Until the checkers classify edges, split the classes by hand:

    grep -rl 'pkg/services/<target>' pkg/services/<source> --include='*.go' | grep -v _test
    grep -rl 'pkg/services/<target>' pkg/services/<source> --include='*.go' | grep    _test

`go build` will not catch it either — it does not compile test files. `go vet`
reports the edges without saying they are test edges.

### Transports — smaller than the raw count suggests

| subtree | non-test LOC | files |
|---|---:|---|
| `pkg/transports/cli/` | 25,693 | 110 |
| `pkg/transports/mapping/` | 12,318 | 38 |
| `pkg/transports/acp/` | 6,872 | 36 |
| `pkg/transports/http/` | 29,442 | 9 |
| `pkg/transports/mcp/` | 220 | 3 |

The HTTP figure is misleading: 27,961 of those 29,442 lines are generated
(`generated/server.gen.go` 14,891 and `client/client.gen.go` 13,070).
**Hand-written HTTP transport code is 1,481 lines.** The business logic that
belongs further down lives in `cli/` and `mapping/`, not in `http/`.

When planning a move-down, a package is safe to relocate under service `S` only
if **both** hold:

1. it imports no service other than `S` (out-edges), and
2. it is imported only by `S` or by shared `pkg/transports/` code (in-edges).

A peer-service in-edge is fatal, but the same in-edge relayed through a shared
`pkg/transports/mapping/` facade is legal — so an in-edge measurement says the
move is unsafe *as written*, not that it is impossible. Proven on `dcp-t3` and
`dcp-t4`.

### Measured debt registries

24 baseline files under `docs/internal/baselines/`. The live ones:

| registry | rows | note |
|---|---:|---|
| `unfinished-package-moves.json` | 44 | all intra-service folds, mechanical; closed by cutover proofs |
| `petri-public-surface-baseline.json` | 101 | the Defect 2 ratchet |
| `ownership-inventory.json` | 9 + 12 + 6 | seed services, additional roots, named owner confirmations |
| plus | | deadcode, coverage (unit + functional), file-count, exemption budget, transport behavior, service cycle ceiling |

Six registries need surgical edits per package move. Both coverage manifests are
sorted **and** completeness-checked, so a desorted manifest aborts the gate
before any floor is evaluated. Moving well-covered code *lowers* the source
package's ratio — a deletion lane can fail the gate for the opposite reason a
growth lane does.

---

## 4. What remains, in order

1. **Split test vs production edges in the three boundary checkers.** Report
   both; gate production as blocking and test-only on its own counter. Small
   change to existing code. Everything below is currently measured with a
   half-blind instrument, and it has already produced one wrong deferral and one
   wrong operator statement. **Do this first.**
2. **Fix the service list in `CLAUDE.md`** — four services are missing.
3. **De-Petri the request shape** (Defect 2), then privatise `CommandRunner`,
   `PTYAllocator`, and `InputFileSystem` behind the host-effect rule (Defect 1).
   Inverting the order locks in the wrong shape.
4. **Encode the host-effect rule into `cmd/pkgboundarycheck`** so it cannot
   regress.
5. **Revisit whether `work` is the right home for the shared vocabulary.**
   `docs/internal/development/plans/service-shape-followups.md` §4.1 is explicit
   that "it went to `work`" is not settled architecture. The question is clearer
   after step 3, when the shape is no longer engine-flavoured.
6. **Transports move-downs**, targeting `cli/` and `mapping/`, under the
   two-direction rule above.
7. **Blocked on an ownership decision:** the `factory_definitions -> workers`
   vocabulary packet needs `workers/worker_vocabulary_contract.go` and
   `factory_definitions/owned_contract.go` amended. Recorded in
   `service-shape-followups.md` §5.

The API-surface noise item is recorded in that document as **unmeasured**, with
a list of what to count first. Do not plan against it until it is measured.

---

## 5. Operator instrumentation: the failure taxonomy

The single transferable finding from this run:

> **Almost every instrument reported a result without reporting its own scope.**
> The failures were not wrong answers. They were correct answers to a narrower
> question than the operator thought had been asked.

### Class 1 — selection-blind

The check ran on a subset and reported as though on the whole.

- The functional coverage gate is selection-based. A "green main" ran one test.
- The boundary checkers skip `_test.go` (Defect 3 above).
- `go build` does not compile test files, so symbol-removal proofs made with it
  are structurally blind. Use `go vet` and read its output.
- `go list -f '{{.Imports}}'` dedupes per package — counting it yields
  package-pairs, not import statements. On one graph this read 22 where the true
  count was 42, and it reordered which lane was heaviest.
- `verify-fast`'s smoke test re-runs the entire unit suite inside a functional
  test, so any unit flake reddens the functional tier and blanks the coverage
  gate.

### Class 2 — aggregation collapse

A rollup destroyed the distinction being looked for.

- An empty CI rollup reads as green. Re-query per check.
- Backend Lint is a counted drift ratchet, not pass/fail. `job.conclusion` is
  meaningless for it; read the allowance table.
- Draft PRs run no checks and read as green to a failure-count sweep. Count
  checks, not failures.
- A cancelled run manufactures failures via `if: always()` — setup steps skip,
  always-steps run and fail. Discard the whole run, not the job.
- Piping a probe discards the exit code being measured (`124` collapses to `0`).
- A whole-matrix red with short job durations is a codeload rate limit, not a
  code failure.

### Class 3 — attribution

The right failure attributed to the wrong cause.

- CI grades the *merge*. A cross-file semantic conflict compiles locally, shows
  MERGEABLE, and fails at a line not in the branch's tree.
- `main`'s tip is not the branch's base. Check merge-base before attributing.
- Verify each red check against its own log. A review once blamed one cause for
  two red checks; a control grep showed only one matched.
- A `.stderr`-prefixed error can belong to a passing test that injected it
  deliberately. The loudest stack trace is often not the failure.
- The same test name failing on two lanes can be two different defects; elapsed
  time discriminates (a 0.44s tool error versus an 8.3s hang).
- `unmeasured` is a reporting defect, not a failure cause.

### Class 4 — determinism inferred from n=1

- One red run cannot separate flake from regression. 9 failures in 5000 looks
  like a regression at n=1. Stress at head **and** parent.
- Two failures at one head do not prove determinism either — extend the span to
  the next head.
- Never revert an instrument after one green run; green is the majority outcome
  for a flake.
- The gold standard is a same-head job re-run: it *observes* nondeterminism
  rather than inferring it.

### Class 5 — operator-side liveness monitors

The worst class, because these were operator-authored.

- Dirty-file count is not a liveness signal — it froze and fired two false
  alarms. Use mtime.
- Commits-ahead is not a stranded-work signal — 20 alerts, zero true positives.
  Recency is the discriminator.
- A monitor whose command began with `rm -f <seen-state>` replayed six-day-old
  failures at every startup.
- A log analyzer with a hardcoded path reported identical counts for three
  different logs. **Validate every analyzer against a known-green control.**
- A stranded-work detector must not trust session state; key on PR head versus
  local HEAD plus mtime, with a dirty-but-idle clause.
- **A failed query must never be computed into state.** Empty output from a
  wedged API became "lanes released". Validate the query succeeded before
  deriving anything from its result.

---

## 6. Instruments that should exist before the next run

**1. A board-truth join.** One command, one table. The lane *name* is already
the join key across every system — work ID, branch, PR, worktree directory,
session, and OS process all derive from it. Nothing joined them, so the operator
hand-joined four partial views every time. Every Class 5 failure is a symptom.

    name | board state | PR# | PR head | local HEAD | worktree mtime | session | PID | verdict

The verdict must be computed from *agreement*, and disagreement must print as
disagreement rather than being resolved to one column.

**2. Every gate emits an inventory line.**

    instrument=<name> selected=N executed=N skipped=N floor=X measured=Y

Pieces of this were built during the run — the coverage gate naming its
instrument, the functional gate emitting output on timeout, the suite inventory
line, and `#2009` making CI checks name the attributable failure — but each was
built reactively after it had already cost time. They were all the same fix,
paid for separately.

**3. A flake discriminator.** Same-head re-run plus parent-head stress in one
invocation, returning `flake | regression | undetermined`. This was the
highest-frequency operator judgment call in the run and it was never scripted.

**4. A payload linter, in the repo rather than in operator instructions.** Every
one of these failures is deterministic and cheap to detect:

- acceptance criteria that make the PROCESS stage own "PR MERGED" (~31
  agent-hours on one lane; the planner emitted this trap into every lane until
  it was fixed);
- idea payloads over roughly 9.2k, which die pre-dispatch in ~70ms with no
  provider identity and nothing in the daemon log (`task`-typed payloads are
  exempt);
- negative claims about the tree that go stale between authoring and dispatch;
- unsatisfiable leading clauses in delivery criteria.

A rule that binds only the operator is the "be more careful" pattern. In-repo it
gates at submit time and binds anyone running the factory.

**5. Test/production edge classification in the linters** — Defect 3, and the
cheapest high-value fix in the tree.

---

## 7. Factory defects worth fixing, not working around

**Board state is in-memory.** A daemon restart wipes every work item, token, and
visit counter. This single fact shaped more operator behaviour than anything
else; "never restart the daemon" is a standing rule that exists only because
there is no persistence layer, and every restart costs a full resubmission.

It also creates a deadlock: **workstation prompts are not hot-reloaded, so
improving a prompt requires a restart, which wipes the board.** The factory's
own instructions cannot be improved while it is working. This is a hard
throughput ceiling and it is why prompt-level fixes had to wait for natural
board drains. `recordings` already owns a canonical event ledger with replay, so
the machinery arguably exists and is simply not wired to board state.

**Review verdicts travel as PR comments matching the string `BLOCKING`.**
`gh pr view --json reviews` returns `[]`. That is a convention, not a channel,
and it produced: reviews that re-hand a token with no BLOCKING comment at all;
the operator's own correction comment counting as a review strike, because
operator and reviewer share one GitHub login; and a breaker that counts
unchanged-head re-hands. A typed verdict would remove the whole class.

**Restoration overrides do not reach the worker.** The process prompt reads the
worktree's `prd.json` and `progress.txt`; payload text is invisible to it,
verified across three consecutive breaker-outs. So operator overrides must be
written into files inside the worktree. `prd.json` is worker-writable, so it must
not be md5-guarded — a guard on it destroyed the implementer's notes four times.

**No worktree reaper.** Safe-deletion criteria: merged into `origin/main`, zero
commits ahead, and clean `git status --porcelain`. Needs sign-off before any
bulk deletion.

---

## 8. Open items at close

Product defects filed and unfixed: Worker Sessions listing returns 200 with an
empty array while workers are running; `work move` only handles idea-typed work,
leaving task and review tokens unrecoverable when wedged; 11 `@xyflow/react`
test doubles omit `useStore`.

Found and deliberately not filed: `pkg/platform/process/command_process_test_unix.go`
is misnamed, so it compiles as production and its `Test` functions never run;
`ci.yml` should use `if: ${{ !cancelled() }}` rather than `if: always()`.

---

## 9. Related documents

- `docs/internal/development/plans/service-shape-followups.md` — the detailed
  plan for sections 3 and 4 above, including the full port classification, the
  alias strangler recipe, and the registry edit list.
- `docs/internal/development/plans/decoupling-program-remaining-work.md` — the
  predecessor. Its section 3 is superseded; its `providers -> workers` blocker
  is retracted.
- `docs/internal/baselines/README.md` — the ratchet registry index.
