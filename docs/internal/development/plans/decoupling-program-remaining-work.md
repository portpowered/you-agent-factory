# Decoupling Program — Remaining Work

Status snapshot taken 2026-08-17 against `origin/main` at `516aa58b8`.

This document exists because the decoupling program (`dcp`) is close to done on
its import-graph half while a substantially larger structural half remains, and
because several measured findings from the operating session that produced it
would otherwise be lost. Everything below was measured, not estimated; where a
number is quoted, the command that produced it is given so the next reader can
re-derive it rather than trust it.

Numbers in this document go stale. Re-measure before acting on any of them.

## 1. What is already done

The `dcp` program ran in two batches.

**Core batch (dcp-1 … dcp-7).** Reduces the minimum feedback arc set of the
`pkg/services` cross-service import graph to zero.

| Lane | Subject | State |
| --- | --- | --- |
| dcp-1 | Coverage default floor and per-service completeness | merged |
| dcp-2 | Retire derivable package enumerations | merged |
| dcp-3 | Derived service-cycle ratchet | merged |
| dcp-4 | Cut transport-carried service back-edges | merged |
| dcp-5 | Cut wire-carried service back-edges | merged |
| dcp-6 | Cut work-service back-edges | in flight |
| dcp-7 | Cut remaining service back-edges to zero | gated on dcp-6 |

**Transports move-down batch (dcp-t1 … dcp-t5).** All five merged. Moves
single-service adapter packages out of the shared `pkg/transports/` tree and
under their owning service.

The cycle ratchet lives in `docs/internal/baselines/service-cycle-ceiling.json`
and is two-directional: it fails when the measured weight rises above the
ceiling, and also when the weight drops below the ceiling without the ceiling
being lowered in the same change. At `516aa58b8` the ceiling is **22**. dcp-6
takes it to 15, dcp-7 to 0.

Measure the live weight with:

    make service-cycle

## 2. Two techniques this program produced

These are the transferable results. They cost several lane-sessions to learn and
they generalise well beyond `dcp`.

### 2.1 Rank candidate cuts by carrier package, not by edge

dcp-5 was predicted to take the weight from 30 to 24 and actually took it to 22.
The extra gain was not luck and not a property of minimum-feedback-arc-set being
a global optimum. The mechanism: the two files it relocated each carried a
`factory_definitions` import, and `work -> factory_definitions` was
independently in the minimum cut, so that edge dropped from weight 4 to 2 as
collateral.

**A single carrier package can carry imports to several services at once, so
gutting one carrier buys weight on every edge it carries.** When choosing what
to cut next, rank by carrier package and look for a carrier that appears under
more than one edge — not by edge weight alone.

To make the checker dump its chosen cut set on a tree that is currently green,
point `-ceiling-file` at a throwaway JSON containing `{"ceiling": 999}`. The
check then reports the measured weight and the edges it selected instead of
passing silently.

### 2.2 Classifying a package for move-down needs BOTH import directions

The transports move-down lanes were originally selected by asking *which
`pkg/services/*` does this package import?* and keeping the packages that import
exactly one. That criterion is only half a test. It measures out-edges. A
package can import exactly one service and still be imported **by** several.

Get in-edges with a reverse query, not `.Imports` on the package itself:

    go list -f '{{$p := .ImportPath}}{{range .Imports}}{{$p}} -> {{.}}
    {{end}}' ./... | grep ' -> .*<candidate-package>$'

`dcp-t2` hit this on `pkg/transports/mapping/workcontent`, which imports only
`pkg/services/work` but is imported by `pkg/services/models`,
`pkg/services/recordings`, and `pkg/transports/cli/run`. It measured
`make pkg-boundary` going from exit 0 to exit 1 with
`found 5 package-boundary violation(s)` and correctly deferred that story rather
than forcing it.

**The constraint is asymmetric, and this is the part that matters.** A peer
*service* may not import another service's subpackage, but shared
`pkg/transports/` code may — see `cmd/pkgboundarycheck/main.go`, which prints
"service subpackages are owner-internal for ordinary consumers; pkg/wire is the
unrestricted composition-root exception."

So a cross-service in-edge does not make a package immovable. It has three
possible outcomes:

1. **Defer the move.** Correct only when nothing cheap decouples the consumers.
2. **Fold in a behavioural refactor.** Wrong inside a move-down lane; it belongs
   in its own lane.
3. **Route the cross-service consumers through a shared facade that stays put.**
   Usually correct.

`dcp-t4` demonstrated option 3 and merged green. It moved
`workerinference` under `factory_definitions` and added ten lines to
`pkg/transports/mapping/factorydefinitionentry/entry.go` re-exporting
`OperationBindingsFromGenerated`, so the two `pkg/services/models` transports
import the shared mapping package instead of reaching into another service's
subtree. `pkg-boundary` accepted it. `dcp-t3` had already established the same
shape with `validationentry`.

**Consequence for planning: do not budget a preparatory behavioural-refactor
lane for every package with cross-service in-edges.** Check first whether a
shared facade already sits on the consumer path. If one does, the relay is about
ten lines inside the move-down lane itself.

## 3. The remaining structural half: 44 unfinished package moves

`docs/internal/baselines/unfinished-package-moves.json` is the consolidated
ledger of packages under `pkg/` that still carry unfinished Packaged Service
Structure migration intent. It declares its own completion condition:

> This ledger only shrinks. Landing a move deletes its row; it is never a place
> to register a package that already lives where it belongs. When moves is
> empty, delete this file together with its loaders and checks.

At `516aa58b8` it holds **44 rows**. This is the single clearest measurable
answer to "is the structural work done" — it is not, and the file deletes itself
when it is.

Count and list them with:

    python3 -c "import json;d=json.load(open('docs/internal/baselines/unfinished-package-moves.json'));print(len(d['moves']));[print(x['packagePath'],'->',x['destination']) for x in d['moves']]"

The rows cluster by owner, which is the natural lane split:

| Owner | Rows | Notes |
| --- | ---: | --- |
| `recordings` | 12 | splits across `artifacts_export`, `canonical_ledger`, `projection_query`, `replay` — four destination subservices, so this is four lanes, not one |
| `factory_definitions` | 7 | all to `factory_definitions/internal` |
| `work` | 6 | all to `work/internal` |
| `operator_settings` | 6 | all to `operator_settings/internal` |
| `factory_runtime` | 5 | one goes to `internal/services/orchestration` |
| `workers` | 4 | all to `workers/internal` |
| `factory_sessions` | 3 | all to `internal/services/runtime_opening` |
| `providers` | 1 | `agy/agypty` to `internal/services/execution` |

Suggested approach, based on what worked for `dcp-t1..t5`:

- **Pre-split at planning time, one lane per owner-plus-destination.** Review has
  blocked before on oversized cutovers, and the size criterion is enforceable at
  planning time only — a 112-file lane was correctly asked to split when it was
  two statements from green. Splitting after the fact is expensive.
- **Run the in-edge reverse query for every package before writing the payload**,
  and record the answer in the payload so the lane does not rediscover it.
- **Name the shared facade in the payload** when option 3 from §2.2 applies.
- **Expect four registries to need surgical edits** per move: the coverage
  manifests are sorted and completeness-checked, so a move that desorts them
  aborts before any floor is evaluated. Never bulk-regenerate; remove and re-add
  the specific entries.

## 4. Other measured debt

All counts from `docs/internal/baselines/` at `516aa58b8`.

| Registry | Entries | Direction |
| --- | ---: | --- |
| `package-structure-baseline.json` | 558 | shrink-only |
| `backend-exemption-budget.json` | 474 | shrink-only |
| `go-unit-coverage-package-minimums.json` | 470 | floors, not debt |
| `go-functional-coverage-package-minimums.json` | 350 | floors, not debt |
| `functional-undocumented-tests.json` | 221 | shrink-only |
| `petri-public-surface-baseline.json` | 101 | shrink-only |
| `backend-package-file-count.json` | 62 | deletion-only; never raise |
| `frontend-deadcode-baseline.json` | 30 | shrink-only |
| `ownership-inventory.json` | 8 | shrink-only |

The two coverage-minimum manifests are floors rather than debt — they are
supposed to have an entry per package. The others are all violation baselines
that should trend to zero.

## 5. Open defects, not yet fixed

**Worker Sessions listing returns 200 with an empty array while workers are
running.** A live API correctness bug, not a test issue.

**`work move` only addresses idea-typed work.** Task and review tokens are
unrecoverable when wedged, which has required manual intervention repeatedly.

**Eleven `@xyflow/react` test doubles omit `useStore`.** The mock has drifted
from the real module across the UI integration suite; consolidating it is one
lane.

**`pkg/platform/process/command_process_test_unix.go` does not end in
`_test.go`.** It therefore compiles as production code and its `Test` functions
have never executed. Renaming it will surface whatever those assertions actually
claim; budget for them failing.

**`findAvailablePort()` is a check-then-act race.**
`ui/integration/browser-test-harness.mjs` binds a probe socket on port 0, reads
the assigned port, closes it, and returns the number; the real bind happens later
in a Go child spawned with `--api-port`. Anything on the runner can take the port
in that window. Observed once as
`listen tcp 127.0.0.1:39389: bind: address already in use` killing
`UI Backend Integration`. `browserPreviewPorts()` calls the helper twice and only
guards against the two results being equal to each other, not against either
being stolen. About twenty integration files consume this harness, so measure
frequency before changing it. Cheapest fix is a bounded retry on `EADDRINUSE`;
the correct fix is passing the listening socket to the child so it is never
released.

**`.github/workflows/ci.yml` uses `if: always()` on steps that should be
`if: ${{ !cancelled() }}`.** During job cancellation the setup steps skip while
the `always()` steps run and fail, manufacturing failures that look real. Ignore
the whole run in that case, not the individual job.

**Raw CI logs remain hard to grep even though the job summaries no longer are.**
The `actions/checkout` step prints every remote branch, and enough branch names
contain words like `failure` and `manifest` that a grep for a real error returns
forty lines of branch list first. Filter with `grep -v 'Run actions/checkout'`.

## 6. Operational notes for whoever picks this up

**Lane worktrees accumulate.** `git worktree list | grep -c '.claude/worktrees'`
returned 144. Each is a full checkout. `du -sh` on that directory did not
complete within two minutes. This is a plausible contributor to host slowness and
build contention.

Do **not** bulk-delete them. A worktree has twice held the only copy of real
work — one lane died with seven unpushed commits, another sat on four. Safe
removal requires all three of: the branch is merged into `origin/main`, it has
zero commits ahead of origin, and `git status --porcelain` is empty.

**The operator's root checkout lags `origin/main`.** Lanes merge on GitHub and
work in their own worktrees; nothing pulls the root. Measure repository state
with `git show <ref>:<path>` or a disposable worktree, never with a bare path in
the root checkout. The tell that this has bitten you is a grep hit at a line
number past the file's actual length.

**One failing test blanks the entire coverage gate.** The unit coverage job
prints `coverage not evaluated: N failed tests observed; package floors were NOT
checked` and exits. A single flaky test therefore costs far more than one test,
which is the argument for fixing flakes promptly rather than re-running.

**A same-head re-run is the only clean proof of a flake.** Two runs at
`57850f2d5` disagreed — one succeeded, one failed on a graph-editor dialog
timeout — which settles it as nondeterminism rather than inference. Where a same-head
re-run is unavailable because a new head has landed, say so rather than calling
it flaky on one observation.

## 7. Suggested order

1. Land dcp-6, then dcp-7. Cycle ratchet reaches 0 and
   `service-cycle-ceiling.json` can be retired.
2. Plan the 44 package moves as owner-scoped lanes per §3, applying §2.2 up
   front.
3. Fix the two correctness defects in §5 (Worker Sessions listing, `work move`
   token coverage) — these are user-visible, unlike the baselines.
4. Measure the `findAvailablePort` race frequency, then fix it. It taxes every
   pull request that touches the UI.
5. Work the violation baselines in §4 down, largest first.
