# meta view

## 2026-05-17 mainline refresh and guards-batch dispatch

### world state

- as of `2026-05-17T20:03:25+07:00`, `git pull --ff-only` leaves the
  workspace current at `origin/main` `0300994`
- the canonical ask surface is still `factory/logs/meta/asks.md`, with the
  same active priorities through `2026-05-25`: checklist conformance, backend
  `90%` functional coverage, website `90%` coverage, and simplification
- the checked-in workflow contract from `factory/README.md` is unchanged:
  default to one standalone idea under `factory/inputs/idea/default/`, and
  widen the queue only for a concrete non-overlapping blocker or cleanup seam
- the checked-in inbox contract is still clean:
  `git ls-files factory/inputs` resolves only to the tracked `.gitkeep`
  sentinels, but the live local operating queue now includes one new local idea
  file:
  `factory/inputs/idea/default/split-functionallong-guards-batch-helpers-from-default-support.md`
- visible remote ownership for ask-critical work still exists outside the local
  inbox:
  `origin/ralph/fix-gocoveragecheck-zero-coverage-report-gap` and
  `origin/ralph/audit-repository-against-2026-website-and-backend-checklists`
- recent `main` history moved since the prior worldview snapshot:
  `0300994` merged PR `#181` from `refactor-selection-pages`, and the commits
  between `41bfb0a` and `0300994` include UI replay-coverage and
  current-selection fixes

### delegated verification truth

- one delegated explorer recommended refreshing the worldview first because the
  checked-in meta logs were stale on repo tip and the ask-critical lanes still
  have remote ownership
- a second delegated explorer plus direct reads reconfirmed the best new narrow
  cleanup seam is still the long-only helper ownership mismatch in
  `tests/functional/guards_batch/helpers_test.go`
- direct reads show `providerErrorCorpusEntryForTest` and `panickingExecutor`
  remain defined in the default helper owner, while live callers remain only in
  `tests/functional/guards_batch/partial_batch_long_test.go` and
  `tests/functional/guards_batch/concurrency_limit_long_test.go`
- direct reads also reconfirm the repository-owned `workspace-setup`
  portability issue is still live:
  `factory/workers/workspace-setup/AGENTS.md`,
  `tests/adhoc/factory/workers/workspace-setup/AGENTS.md`, and
  `tests/functional_test/testdata/idea_plan_execute_review_with_limits/workers/workspace-setup/AGENTS.md`
  still hard-code `command: python`, and replay assets still preserve
  `SCRIPT_REQUEST` events with `command":"python"`
- recent remote cleanup branches for already-landed work should now be treated
  as stale residue rather than active demand:
  `origin/ralph/simplify-cron-watcher-runtime-lookup-width`,
  `origin/ralph/dedupe-replay-contract-tagged-helpers`, and
  `origin/ralph/split-bootstrap-portability-functionallong-helpers` all have
  merged commits on the path to `main`

### queue decision

- dispatch one new standalone cleanup idea:
  `factory/inputs/idea/default/split-functionallong-guards-batch-helpers-from-default-support.md`
- reason:
  the checked-in local inbox was effectively empty in this checkout, the seam
  is narrow and already validated in code, it does not overlap the remotely
  owned ask lanes, and it preserves the factory contract of widening the queue
  by exactly one concrete idea

### theory of mind

- the previous worldview overstated local queue width because it depended on
  local operating files that are not present in this checkout; the live truth
  is the checked-in inbox contract plus whatever repository-local request files
  exist right now
- ask handling remains more about ownership truth than spawning duplicates:
  the coverage-gap and checklist-audit lanes are still real customer pressure,
  but they already have remote owners, so the local meta layer should avoid
  cloning that work into another request file
- the best simplification work right now is still lane-ownership cleanup in the
  functional suite:
  keep long-only helpers in explicit long-lane owners, keep default helpers
  narrow, and protect behavior through runtime assertions rather than file-shape
  tests

## 2026-05-10 queue-truth refresh and hold

### world state

- as of `2026-05-10T03:01:41+09:00`, `git pull --ff-only` still leaves the
  workspace at `origin/main` `41bfb0a`
- the canonical ask surface is still `factory/logs/meta/asks.md`, with the
  same active priorities through `2026-05-25`: checklist conformance, backend
  `90%` functional coverage, website `90%` coverage, and simplification
- the checked-in workflow contract from `factory/README.md` is still unchanged:
  default to one standalone idea under `factory/inputs/idea/default/`, and
  widen the queue only for repository-owned blockers
- the checked-in `factory/inputs/**` surface is still only the tracked
  `.gitkeep` sentinels; the real request files remain local operating state,
  not checked-in workflow input
- the live local operating queue is now best described as:
  - active idea
    `factory/inputs/idea/default/split-functionallong-provider-template-helpers-from-default-support.md`
  - active workflow-unblocker idea
    `factory/inputs/idea/default/canonicalize-workspace-setup-on-python3.md`
  - active ask-aligned task with remote ownership
    `factory/inputs/task/default/fix-gocoveragecheck-zero-coverage-report-gap.md`
    via `origin/ralph/fix-gocoveragecheck-zero-coverage-report-gap`
  - stale local residue
    `factory/inputs/task/default/simplify-cron-watcher-runtime-lookup-width.md`
- visible local dirtiness outside the maintainer logs still concentrates in
  `docs/reference/batch-work.md`, `factory/workstations/cleaner/AGENTS.md`,
  `ui/src/features/current-selection/*`, `ui/src/features/terminal-work/*`,
  `.tmp/`, `factory/logs/old/`, `test.jsonl`, and replay/test artifacts under
  the repo root plus `ui/integration/`

### delegated verification truth

- a delegated queue explorer reconfirmed that the checked-in inbox contract is
  clean:
  `git ls-files factory/inputs` still resolves only to the tracked `.gitkeep`
  sentinels, so the four live markdown request files are repository-local
  operating state rather than committed factory inputs
- direct reads reconfirm the solved cron seam is already on `main`:
  `pkg/service/cron_watcher.go` now narrows
  `startCronWatchersForRuntime` to
  `interfaces.RuntimeWorkstationLookup`, and
  `git log --all -- pkg/service/cron_watcher.go` shows
  `38addb3` (`simplify-cron-watcher-runtime-lookup-width`) on the path
- direct reads reconfirm the `workspace-setup` portability blocker is still
  live:
  `factory/workers/workspace-setup/AGENTS.md`,
  `tests/adhoc/factory/workers/workspace-setup/AGENTS.md`, and
  `tests/functional_test/testdata/idea_plan_execute_review_with_limits/workers/workspace-setup/AGENTS.md`
  still hard-code `command: python`
- the same repository-owned interpreter contract is still preserved in replay
  verification surfaces:
  `ui/integration/fixtures/event-stream-replay-2.jsonl`,
  `ui/integration/fixtures/terminal-summary-regression-replay.jsonl`, and
  `factory/logs/agent-fails.json` still emit `SCRIPT_REQUEST` events with
  `command":"python"`
- direct reads reconfirm the active provider-helper cleanup is still the right
  narrow simplification lane:
  every helper named in
  `split-functionallong-provider-template-helpers-from-default-support.md`
  remains defined in `tests/functional/providers/helpers_test.go`, and the
  live callers remain only in
  `tests/functional/providers/cli_template_resolution_long_test.go`
- the next non-overlapping future seams remain unchanged:
  `tests/functional/guards_batch/helpers_test.go` still keeps
  `providerErrorCorpusEntryForTest` and `panickingExecutor` in default-build
  ownership even though callers remain only in
  `partial_batch_long_test.go` and `concurrency_limit_long_test.go`, and
  `tests/functional/workflow/helpers_test.go` still keeps the
  process-review helper cluster in default-build ownership even though callers
  remain only in `process_review_contract_long_test.go`

### queue decision

- do not dispatch a new cleanup or ask-driven request in this refresh
- reason:
  the queue is already wider than the default for justified reasons, the
  highest-value ask lane still has active remote ownership, the workspace-setup
  portability blocker is already queued, and the remaining verified cleanup
  seams are unchanged future work rather than new blockers

### theory of mind

- the checked-in factory inbox contract and the local operating queue are
  different truths and need to stay separate in the worldview:
  `factory/inputs/**` being tracked only by `.gitkeep` is expected, while the
  local markdown request files are transient operating state
- active ownership matters more than branch count:
  `origin/ralph/fix-gocoveragecheck-zero-coverage-report-gap` is real queue
  pressure, while `origin/ralph/simplify-cron-watcher-runtime-lookup-width`
  is leftover history because the code already landed on `main`
- the safest next cleanup remains helper-ownership alignment in functional
  tests, but not before either the active provider lane or the workspace-setup
  unblocker clears
- the repository-owned `python` contract is broader than one worker file:
  it also lives in replay artifacts and fixture mirrors, so the queued
  portability fix should be treated as a workflow-contract alignment lane, not
  just a one-line worker edit

## 2026-05-10 stale-residue correction and queue hold

### world state

- as of `2026-05-10T02:04:20+09:00`, `git pull --ff-only` still leaves the
  workspace at `origin/main` `41bfb0a`
- the canonical ask surface is still `factory/logs/meta/asks.md`, with the
  same active priorities through `2026-05-25`: checklist conformance, backend
  `90%` functional coverage, website `90%` coverage, and simplification
- the checked-in workflow contract from `factory/README.md` is still unchanged:
  default to one standalone idea under `factory/inputs/idea/default/`, and
  widen the queue only for repository-owned blockers
- the live local operating queue is now best described as:
  - active idea
    `factory/inputs/idea/default/split-functionallong-provider-template-helpers-from-default-support.md`
  - active workflow-unblocker idea
    `factory/inputs/idea/default/canonicalize-workspace-setup-on-python3.md`
  - active ask-aligned task
    `factory/inputs/task/default/fix-gocoveragecheck-zero-coverage-report-gap.md`
  - stale local residue
    `factory/inputs/task/default/simplify-cron-watcher-runtime-lookup-width.md`
- visible local dirtiness outside the maintainer logs still concentrates in
  `docs/reference/batch-work.md`, `factory/workstations/cleaner/AGENTS.md`,
  `ui/src/features/current-selection/*`, `ui/src/features/terminal-work/*`,
  `.tmp/`, `factory/logs/old/`, `test.jsonl`, and replay/test artifacts under
  the repo root plus `ui/integration/`

### delegated verification truth

- a delegated queue explorer plus direct reads reconfirmed that the
  cron-watcher task file is solved local residue:
  `origin/main` already merged PR `#180`, and
  `pkg/service/cron_watcher.go` now narrows
  `startCronWatchersForRuntime` to
  `interfaces.RuntimeWorkstationLookup`
- the same delegated read reconfirmed the other active lanes are still live:
  `factory/workers/workspace-setup/AGENTS.md` and both repository-owned mirror
  fixtures still use `command: python`, the provider long-only helper cluster
  still lives in `tests/functional/providers/helpers_test.go`, and the
  `gocoveragecheck` zero-coverage false-pass task is still not merged
- a second delegated cleanup explorer found only a provider-test helper dedupe
  inside the same already-queued provider seam:
  `tests/functional/providers/helpers_test.go` still duplicates
  `support.UpdateFactoryConfig` via `updateScriptFixtureFactory`, and wraps
  `support.WriteAgentConfig` via `writeNamedWorkerAgents`
- because that dedupe sits inside the same provider helper owner file as the
  active idea, it is not a new non-overlapping lane; it is implementation
  detail that can be absorbed when the provider-helper split is worked
- direct reads still reconfirm the previously validated future seams remain
  behind the active queue:
  `tests/functional/guards_batch/helpers_test.go` keeps
  `providerErrorCorpusEntryForTest` and `panickingExecutor` in the default
  helper file even though callers are long-only, and
  `tests/functional/workflow/helpers_test.go` still keeps the
  process-review helper cluster in default-build ownership even though the
  callers live only in `process_review_contract_long_test.go`

### queue decision

- do not dispatch a new cleanup or ask-driven request in this refresh
- reason:
  the queue is already wider than the default for justified reasons, the only
  newly surfaced simplification seam overlaps the active provider lane, and the
  main correction this cycle is world-model accuracy about stale solved residue

### theory of mind

- queue hygiene matters as much as finding new seams:
  solved local request files should be treated as residue, not as live demand,
  or the meta layer will overestimate queue width and ownership
- not every simplification candidate deserves its own inbox file:
  when a dedupe sits inside a file already claimed by an active cleanup idea,
  it should be folded into that lane instead of spawning parallel overlap
- the ask-critical path is still measurement truth first:
  the coverage-gate fix remains the highest-value ask-aligned task before
  widening backend coverage work, while the workspace-setup interpreter fix
  remains the highest-value workflow unblocker for the repository-owned loop

## 2026-05-10 workflow-seam queue-hold refresh

### world state

- as of `2026-05-10T01:03:15+09:00`, `origin/main` still resolves to
  `41bfb0a`; `git pull --ff-only` again reported the workspace is already up
  to date
- the canonical ask surface remains `factory/logs/meta/asks.md`, with the same
  active priorities through `2026-05-25`: checklist conformance, backend
  `90%` functional coverage, website `90%` coverage, and general
  simplification
- the checked-in inbox contract from `factory/README.md` is still unchanged:
  default to one standalone idea under `factory/inputs/idea/default/`, with
  wider queue width justified only by a concrete repository-owned blocker
- the current local operating queue is still:
  - active idea
    `factory/inputs/idea/default/split-functionallong-provider-template-helpers-from-default-support.md`
  - justified workflow-unblocker idea
    `factory/inputs/idea/default/canonicalize-workspace-setup-on-python3.md`
  - active task
    `factory/inputs/task/default/fix-gocoveragecheck-zero-coverage-report-gap.md`
  - stale merged residue
    `factory/inputs/task/default/simplify-cron-watcher-runtime-lookup-width.md`
- visible local dirtiness outside the maintainer logs still concentrates in
  `docs/reference/batch-work.md`, `factory/workstations/cleaner/AGENTS.md`,
  `ui/src/features/current-selection/*`, `ui/src/features/terminal-work/*`,
  `.tmp/`, `factory/logs/old/`, `test.jsonl`, and replay/test artifacts under
  the repo root plus `ui/integration/`

### delegated verification truth

- a delegated ask explorer plus direct reads reconfirmed that this refresh
  should hold the queue rather than widen it:
  the existing coverage-fix task is still actively owned by
  `origin/ralph/fix-gocoveragecheck-zero-coverage-report-gap`, the checklist
  audit remains actively owned by
  `origin/ralph/audit-repository-against-2026-website-and-backend-checklists`,
  and `ui/vite.config.ts` still enforces UI coverage thresholds above the
  customer's `90%` target
- direct reads still confirm the provider-helper idea is live and unchanged:
  the long-only helper cluster remains in
  `tests/functional/providers/helpers_test.go`, with callers still in
  `tests/functional/providers/cli_template_resolution_long_test.go`
- direct reads still confirm the previously validated guards-batch future seam:
  `tests/functional/guards_batch/helpers_test.go` keeps
  `providerErrorCorpusEntryForTest` and `panickingExecutor` in the default
  helper owner even though their callers remain only in
  `partial_batch_long_test.go` and `concurrency_limit_long_test.go`
- a delegated cleanup explorer surfaced one more non-overlapping future seam,
  and direct reads confirmed it:
  `tests/functional/workflow/helpers_test.go` still keeps
  `newAdhocProcessReviewHarness`, `assertProviderCallWorkstations`,
  `assertDispatchHasOutputToPlace`, and `assertDispatchOutputTagAbsent` in the
  default helper file, while their live callers sit only in
  `//go:build functionallong`
  `tests/functional/workflow/process_review_contract_long_test.go`
- `go run ./cmd/deadcodecheck` still exits cleanly with
  `[agent-factory:deadcode] baseline matches`, so the workflow helper split is
  a validated future deadcode-baseline cleanup seam, not a new ask blocker

### queue decision

- do not dispatch a new cleanup or ask-driven request in this refresh
- reason:
  the queue is already wider than the default for justified reasons, the
  active ask-owned lanes still have remote ownership, and the newly validated
  workflow helper seam is non-urgent future work rather than a blocker
- keep the guards-batch helper split as the next standalone cleanup idea after
  the current provider-helper or workspace-setup lane clears, and keep the
  workflow process-review helper split immediately behind it

### theory of mind

- queue width should track blockers and ownership, not merely the existence of
  another clean seam:
  once the repo already carries one active cleanup idea, one workflow
  unblocker, and one ask-critical task, further widening should wait for a new
  blocker or for one lane to clear
- deadcode-baseline hygiene remains a productive simplification pattern in this
  repo, but validated future seams do not all need immediate inbox entries
- local UI dirtiness is a real coordination signal:
  even when a UI simplification seam exists, backend or test-only helper splits
  with clean ownership are safer candidates for future queueing

## 2026-05-10 ask-gate refresh

### world state

- as of `2026-05-10T00:00:00+09:00`, `origin/main` still resolves to
  `41bfb0a`; `git pull --rebase --autostash` reported the workspace is already
  up to date
- the canonical ask surface remains `factory/logs/meta/asks.md`, with the same
  active priorities: checklist conformance, backend `90%` functional coverage,
  website `90%` coverage, and general simplification through `2026-05-25`
- the checked-in inbox contract from `factory/README.md` is unchanged:
  default to one standalone idea under `factory/inputs/idea/default/`, with
  extra queue width justified only when a repository-owned blocker makes it
  necessary
- the current local operating queue remains:
  - active idea
    `factory/inputs/idea/default/split-functionallong-provider-template-helpers-from-default-support.md`
  - justified workflow-unblocker idea
    `factory/inputs/idea/default/canonicalize-workspace-setup-on-python3.md`
  - active task
    `factory/inputs/task/default/fix-gocoveragecheck-zero-coverage-report-gap.md`
  - stale merged residue
    `factory/inputs/task/default/simplify-cron-watcher-runtime-lookup-width.md`
- visible local dirtiness outside the maintainer logs is still concentrated in
  `docs/reference/batch-work.md`, `factory/workstations/cleaner/AGENTS.md`,
  `ui/src/features/current-selection/*`, `ui/src/features/terminal-work/*`,
  `.tmp/`, `factory/logs/old/`, and replay/test artifacts under the repo root
  plus `ui/integration/`

### delegated verification truth

- direct reads and a delegated explorer both reconfirm the best next narrow
  cleanup seam after the active ideas is still
  `tests/functional/guards_batch/helpers_test.go`:
  `providerErrorCorpusEntryForTest` is only called by
  `partial_batch_long_test.go`, and `panickingExecutor` is only called by
  `concurrency_limit_long_test.go`
- the ask-driven quality lane is still blocked by a repository-owned false
  signal:
  a live `go run ./cmd/gocoveragecheck -min 80 -timeout 300s` run still prints
  `pkg/apisurface`, `pkg/buffers`, and `pkg/cli/default` at `0.0%` coverage
  while finishing with `Go coverage 86.6% meets minimum 80.0%`
- that means the active task
  `factory/inputs/task/default/fix-gocoveragecheck-zero-coverage-report-gap.md`
  remains the highest-leverage ask-aligned lane before any broader backend
  coverage campaign
- the website coverage ask does not currently need another threshold-setting
  task:
  `ui/vite.config.ts` still enforces repo-owned UI coverage thresholds at
  `93.1` statements, `80.4` branches, `94.9` functions, and `93.1` lines
- remote ownership also argues against queueing another checklist or coverage
  ask right now:
  remote refs still include
  `origin/ralph/audit-repository-against-2026-website-and-backend-checklists`
  and `origin/ralph/fix-gocoveragecheck-zero-coverage-report-gap`
- direct GitHub reads confirm the linked `portpowered/checklists` repository
  still exposes the 2026 backend and website checklists used by the ask; this
  refresh did not find a narrower new checklist-driven repo task than the
  existing audit lane

### queue decision

- do not dispatch a new cleanup or ask-driven request in this refresh
- reason:
  the active queue already contains the justified workflow unblocker and the
  active coverage-gate task, while the next validated cleanup seam is known and
  non-urgent
- keep the guards-batch helper split as the next standalone cleanup idea after
  either the provider-helper lane or the workspace-setup unblocker clears

### theory of mind

- ask work should not outrun measurement truth:
  when the repo-owned coverage gate still passes `0.0%` packages, additional
  package coverage requests risk optimizing against a broken signal
- broad checklist follow-up should wait for one of two concrete triggers:
  either the existing checklist audit branch lands a repo-owned gap, or the
  live ask surface names a narrower enforcement seam than the current audit
- validated future seams still matter even when they are not queued yet:
  keeping the next helper split in the worldview preserves momentum without
  creating overlapping inbox residue
- the repo's default of one standalone idea still holds, with the current
  second idea remaining justified only because it unblocks the checked-in
  workflow itself

## 2026-05-09 workspace-setup unblocker refresh

### world state

- as of `2026-05-09T21:31:00+09:00`, `origin/main` still resolves to
  `41bfb0a`; `git pull --ff-only` remains clean
- the canonical ask surface is still active at `factory/logs/meta/asks.md`
  with checklist conformance, `90%` coverage goals, and simplification work
  live through `2026-05-25`
- the checked-in inbox contract from `factory/README.md` still defaults to one
  standalone idea under `factory/inputs/idea/default/`, but the current local
  operating queue now has one justified additional idea because the existing
  idea lane is blocked by the checked-in workflow itself:
  - active idea
    `factory/inputs/idea/default/split-functionallong-provider-template-helpers-from-default-support.md`
  - new workflow-unblocker idea
    `factory/inputs/idea/default/canonicalize-workspace-setup-on-python3.md`
  - active task
    `factory/inputs/task/default/fix-gocoveragecheck-zero-coverage-report-gap.md`
  - stale merged residue
    `factory/inputs/task/default/simplify-cron-watcher-runtime-lookup-width.md`
- visible local dirtiness outside the maintainer logs still includes:
  - tracked `docs/reference/batch-work.md`
  - tracked `factory/workstations/cleaner/AGENTS.md`
  - tracked `ui/src/features/current-selection/*`
  - tracked `ui/src/features/terminal-work/*`
  - untracked `.tmp/`
  - untracked `factory/logs/old/`
  - untracked replay/test artifacts under the repo root and `ui/integration/`

### delegated verification truth

- direct reads and delegated explorer output both confirm the provider-helper
  idea is still the current code-cleanup lane:
  `tests/functional/providers/helpers_test.go` still keeps a partial cluster of
  helpers that are only called from
  `//go:build functionallong`
  `tests/functional/providers/cli_template_resolution_long_test.go`
- the exact long-only provider helper cluster still includes:
  - `buildModelWorkerConfig`
  - `writeNamedWorkerAgents`
  - `writeExecutionTemplateWorkstationAgents`
  - `configureResourceGatedTemplateWorkstation`
  - `configureExecutionTemplateWorkstation`
  - `configureTwoInputResourceGatedTemplateWorkstation`
  - `writeTwoInputResourceSeeds`
  - `writeExecutionTemplateSeed`
  - `twoInputTemplateArgs`
  - `executionTemplatePrompt`
  - `executionTemplateWantPrompt`
  - `assertProviderArgsPrompt`
  - `assertProviderStdin`
  - `assertProviderExecutionFields`
- direct reads plus delegated verification also confirm the best next cleanup
  seam after the provider split still sits in
  `tests/functional/guards_batch/helpers_test.go`:
  `providerErrorCorpusEntryForTest` is only called by
  `partial_batch_long_test.go`, and `panickingExecutor` is only called by
  `concurrency_limit_long_test.go`
- the more important new truth is workflow-level, not code-seam-level:
  the checked-in `workspace-setup` script worker still hard-codes
  `command: python` in `factory/workers/workspace-setup/AGENTS.md`, while the
  current environment has `python3` but no `python`
- the active provider-helper idea has already hit that failure path in live
  operating data:
  the fresh local run in `test.jsonl` records the accepted idea dispatching
  `setup-workspace` with `command:"python"` and then failing with
  `exec: "python": executable file not found in $PATH`; the checked-in replay
  fixture at
  `ui/integration/fixtures/terminal-summary-regression-replay.jsonl` records
  the same failure shape
- this is not isolated to one replay sample:
  repository-owned docs, adhoc fixtures, testdata, and the script usage text
  in `factory/scripts/setup-workspace.py` still encode the same `python`
  contract for `setup-workspace`

### queue decision

- dispatch one new standalone idea:
  `canonicalize-workspace-setup-on-python3`
- reason:
  the current provider-helper cleanup request is valid but blocked from
  advancing through the checked-in workflow on this machine, so a narrow
  workflow-unblocker is higher leverage than holding the queue to a single
  blocked idea
- keep the provider-helper idea and its verified guards-batch follow-up in the
  worldview; the new idea is non-overlapping because it targets the factory
  workflow boundary rather than the functional-test helper files

### theory of mind

- one active standalone idea is the default, not an absolute rule; a second
  idea is warranted when the first idea is blocked by a repository-owned
  workflow defect
- queue truth has to include operating viability, not just code-seam validity:
  an accepted cleanup lane is not truly live if the next checked-in workstation
  cannot start in the current environment
- when the repository already ships `python3` shebang scripts but the checked-in
  worker command still says `python`, the cleaner seam is to canonicalize one
  interpreter contract rather than add more fallback logic
- keep future helper-cleanup ideas separate from workflow unblocking work so
  ownership stays narrow and non-overlapping

## 2026-05-09 queue-hold refresh

### world state

- as of `2026-05-09T21:03:32+09:00`, `origin/main` still resolves to
  `41bfb0a`; `git pull` reports the workspace is already current
- the canonical ask surface remains `factory/logs/meta/asks.md`, and the
  active ask set is unchanged: checklist conformance, `90%` coverage goals,
  and general simplification remain live through `2026-05-25`
- the canonical checked-in inbox contract from `factory/README.md` is still:
  one standalone cleanup idea under `factory/inputs/idea/default/` by default,
  with batch JSON reserved for dependency ordering or mixed work types
- tracked `factory/inputs/**` content is still sentinel-only in git, while the
  visible local operating queue remains:
  - active idea
    `factory/inputs/idea/default/split-functionallong-provider-template-helpers-from-default-support.md`
  - active task
    `factory/inputs/task/default/fix-gocoveragecheck-zero-coverage-report-gap.md`
  - stale merged residue
    `factory/inputs/task/default/simplify-cron-watcher-runtime-lookup-width.md`
- additional live local dirtiness now exists outside the maintainer logs in:
  - tracked `ui/src/features/current-selection/*`
  - tracked `ui/src/features/terminal-work/*`
  - tracked `docs/reference/batch-work.md`
  - tracked `factory/workstations/cleaner/AGENTS.md`
  - untracked `.tmp/`
  - untracked `factory/logs/old/`
  - untracked replay/test artifacts under the repo root and `ui/integration/`

### delegated verification truth

- direct reads still confirm the queued provider-helper idea is the current
  cleanup lane:
  every helper named in
  `split-functionallong-provider-template-helpers-from-default-support.md`
  remains defined in `tests/functional/providers/helpers_test.go` and every
  live caller still sits in
  `//go:build functionallong`
  `tests/functional/providers/cli_template_resolution_long_test.go`
- the current provider-helper seam matches the repository's recent merged
  cleanup pattern rather than drifting away from it:
  PR `#176` split another `functionallong` helper lane in
  `tests/functional`, and PR `#177` deduped adjacent replay helper ownership
- delegated explorer plus direct verification reconfirm two valid follow-up
  seams after the active provider-helper idea clears:
  - `tests/functional/guards_batch/helpers_test.go` still keeps
    `providerErrorCorpusEntryForTest` and `panickingExecutor` in the untagged
    helper file even though their exact callers remain only the
    `functionallong` suites
    `partial_batch_long_test.go` and `concurrency_limit_long_test.go`
  - `ui/src/components/ui/classnames.ts` is still a byte-for-byte duplicate of
    `ui/src/lib/cx.ts`; the safe future cleanup is to delete the duplicate
    `classnames.ts` surface and repoint its clean consumers
    `ui/src/components/ui/widget-frame.tsx` and
    `ui/src/features/bento/agent-bento.tsx` to `ui/src/lib/cx.ts`, rather than
    deleting `ui/src/lib/cx.ts` while `terminal-work` is locally dirty

### queue decision

- do not dispatch a new cleanup request in this refresh
- reason:
  the canonical idea inbox already contains one valid active standalone idea,
  and the checked-in maintainer workflow still defaults to one standalone idea
  file by default rather than accumulating parallel idea residue
- the highest-value update this cycle is queue truth plus overlap truth:
  the active idea is still correct, the next seams are verified, and the UI
  duplicate seam now has an explicit non-overlapping deletion direction

### theory of mind

- when the active standalone idea still matches live callers exactly, prefer
  a worldview refresh over queue churn
- verify the next seam not only for existence but for deletion direction:
  which duplicate should survive can depend on local dirtiness and import
  ownership, not just on byte equality
- helper-cleanup ideas in this repo often land in short adjacent waves across
  the same functional package; recent merged patterns are useful signals for
  what the next narrow seam should look like
- separate visible local operating work from checked-in queue truth before
  dispatching follow-up ideas; user-authored dirty files can change which
  cleanup direction is safely non-overlapping

## 2026-05-09 delegated seam refresh

### world state

- as of `2026-05-09T21:03:32+09:00`, live upstream `origin/main` still points
  to `41bfb0a` after merged PR `#180`
  (`simplify-cron-watcher-runtime-lookup-width`)
- the canonical ask surface remains active at `factory/logs/meta/asks.md`
- the canonical checked-in inbox contract still comes from `factory/README.md`:
  one standalone cleanup idea under `factory/inputs/idea/default/` by default,
  with batch JSON reserved for dependency-ordered or mixed work
- the current local operating queue is unchanged from the prior refresh:
  - active idea
    `factory/inputs/idea/default/split-functionallong-provider-template-helpers-from-default-support.md`
  - active task
    `factory/inputs/task/default/fix-gocoveragecheck-zero-coverage-report-gap.md`
  - stale merged task residue
    `factory/inputs/task/default/simplify-cron-watcher-runtime-lookup-width.md`

### delegated verification truth

- direct code reads still confirm the queued provider-helper idea is the
  current active cleanup lane:
  `tests/functional/providers/helpers_test.go` owns helpers that are only
  called from
  `//go:build functionallong`
  `tests/functional/providers/cli_template_resolution_long_test.go`
- delegated explorer findings plus direct verification identified two valid
  follow-up seams after the current idea queue clears:
  - `tests/functional/guards_batch/helpers_test.go` keeps
    `providerErrorCorpusEntryForTest` and `panickingExecutor` in the default
    helper file even though live callers are only the `functionallong` tests
    `partial_batch_long_test.go` and `concurrency_limit_long_test.go`; the
    same stale ownership is recorded at the top of
    `docs/internal/development/deadcode-baseline.txt`
  - `ui/src/components/ui/classnames.ts` duplicates
    `ui/src/lib/cx.ts`, and the duplicate helper currently has only one live
    import path in `ui/src/components/ui/widget-frame.tsx` plus the shared
    re-export from `ui/src/components/ui/index.ts`

### queue decision

- do not open another cleanup idea yet
- reason:
  `factory/inputs/idea/default/` already contains one valid unowned active
  request, and the public maintainer workflow defaults to one standalone idea
  file rather than parallel idea accumulation
- the highest-value action in this refresh is to keep the world model aligned:
  current queue truth, current PR ownership, and the verified next seams once
  the existing idea is consumed

### theory of mind

- when the canonical idea inbox already contains one valid active request,
  record the next seams in the worldview instead of staging a second idea by
  default
- verify delegated cleanup suggestions with direct `rg` and file reads before
  promoting them into the worldview
- after a helper-lane split idea is queued, the next best follow-up often sits
  in another build-tagged functional test helper file rather than in production
  runtime code
- if a duplicate shared UI helper has collapsed to one live import site plus a
  barrel re-export, prefer deleting the duplicate helper over preserving both
  names

## 2026-05-09 live refresh

### world state

- as of `2026-05-09T20:03:02+09:00`, live upstream `origin/main` points to
  `41bfb0a` after merged PR `#180`
  (`simplify-cron-watcher-runtime-lookup-width`)
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the canonical ask file is active again and restores the broad checklist,
  coverage, simplification, and autonomous-maintainer backlog through
  `2026-05-25`
- tracked `factory/inputs/**` content remains sentinel-only in git, while
  ignored local operating state currently contains:
  - active `factory/inputs/idea/default/split-functionallong-provider-template-helpers-from-default-support.md`
  - active `factory/inputs/task/default/fix-gocoveragecheck-zero-coverage-report-gap.md`
  - stale merged residue
    `factory/inputs/task/default/simplify-cron-watcher-runtime-lookup-width.md`
- the pre-existing local worktree dirtiness for this refresh is:
  - tracked `docs/reference/batch-work.md`
  - tracked `factory/workstations/cleaner/AGENTS.md`
  - untracked `.tmp/`
  - untracked `factory/logs/old/`

### workflow and queue truth

- `factory/README.md` still defines the canonical checked-in inboxes as
  `inputs/<work-type>/default/`
- `git ls-files factory/inputs` still shows only the tracked `.gitkeep`
  sentinels, so visible task files under `factory/inputs/task/default/` are
  ignored operating state rather than checked-in queue truth
- the visible local operating queue has two real active lanes and one stale
  leftover:
  - `split-functionallong-provider-template-helpers-from-default-support`
    remains queued as an ignored idea file and is not yet owned by any open or
    merged PR
  - `fix-gocoveragecheck-zero-coverage-report-gap` maps to open PR `#179`
  - `simplify-cron-watcher-runtime-lookup-width` maps to merged PR `#180`
- `factory/inputs/idea/default/` is not empty: the provider-helper ownership
  cleanup is already staged there, so the next action is to preserve that queue
  truth instead of opening a duplicate idea

### open-lane truth

- `PR #179` owns the `cmd/gocoveragecheck` zero-coverage gate lane
- `PR #141` owns the repository-wide external checklist audit lane
- `PR #167` owns the current `ui/src/features/work-outcome/*` localization lane
- `PR #171` owns the dashboard-shell and workflow-graph padding lane
- `PR #172` owns the same-trace guard lane across config, petri, API, and
  functional coverage
- the many open `docs: refresh meta world state` PRs are duplicate meta-refresh
  residue rather than code-cleanup ownership signals

### current non-overlapping cleanup candidate

- the next narrow maintainer-owned seam is provider-template helper ownership
  cleanup under `tests/functional/providers/*`
- `tests/functional/providers/helpers_test.go` currently mixes default-build
  helpers with helpers only consumed by
  `//go:build functionallong`
  `tests/functional/providers/cli_template_resolution_long_test.go`
- the live deadcode baseline still reports that long-lane helper cluster as
  unreachable even though the functionallong suite calls it directly
- the concrete cluster is:
  - `buildModelWorkerConfig`
  - `writeNamedWorkerAgents`
  - `writeExecutionTemplateWorkstationAgents`
  - `configureResourceGatedTemplateWorkstation`
  - `configureExecutionTemplateWorkstation`
  - `configureTwoInputResourceGatedTemplateWorkstation`
  - `writeTwoInputResourceSeeds`
  - `writeExecutionTemplateSeed`
  - `twoInputTemplateArgs`
  - `executionTemplatePrompt`
  - `executionTemplateWantPrompt`
  - `assertProviderArgsPrompt`
  - `assertProviderStdin`
  - `assertProviderExecutionFields`
- the narrow fix is to align helper ownership with the `functionallong` lane,
  or add only the minimum local contract coverage needed for helpers that
  truly need default-build ownership
- this seam does not overlap open PRs `#141`, `#167`, `#171`, `#172`, or `#179`
- the cleanup request for this seam is already queued at
  `factory/inputs/idea/default/split-functionallong-provider-template-helpers-from-default-support.md`

### theory of mind

- treat `factory/logs/meta/asks.md` as the immediate routing truth even when it
  reopens backlog that older worldview notes had marked withdrawn
- reason about `factory/inputs/**` in two layers:
  checked-in contract versus ignored operating state
- reconcile ignored local task files against open and merged PRs before
  counting them as active backlog
- treat ignored idea files the same way; once an idea already exists in the
  canonical inbox and remains unowned, update the worldview instead of queuing a
  duplicate request
- when deadcode reports a test helper as unreachable but direct code reads show
  only `functionallong` callers, prefer aligning helper ownership with build
  tags over preserving stale baseline entries

## world state

- as of `2026-05-09T12:03:55+09:00`, live upstream `origin/main` points to
  `bc1e149` after merged PR `#170` (`weird-work-names`) and merged PR `#169`
  (`collapse-replay-safe-diagnostics-rehydration`)
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the current canonical local ask file says there are no active customer asks:
  `for now no asks exists.`
- the tracked maintainer workflow inputs remain sentinel-only under
  `factory/inputs/**`; live work items there are ignored operating state
- the local worktree is dirty before this refresh from tracked local edits in
  `factory/logs/meta/asks.md` and `factory/workers/workspace-setup/AGENTS.md`;
  treat those as existing local state, not as noise to revert

## workflow truth

- `factory/factory.json` defines five work types: `thoughts`, `idea`, `plan`,
  `task`, and `cron-triggers`
- the checked-in maintainer loop on live `main` is:
  `thoughts:init -> ideafy -> thoughts:complete`
  `idea:init -> plan -> idea:to-complete + plan:init`
  `plan:init -> setup-workspace -> plan:complete + task:init`
  `task:init -> process -> task:in-review -> review -> task:to-complete`
  `idea/task:to-complete -> consume -> idea/task:complete`
- topology details that still matter:
  - `process` and `review` execute in `.claude/worktrees/{{name}}`
  - shared `executor-slot` capacity remains `10`
  - loop breakers still guard repeated `process` and `review` retries

## input surface truth

- tracked `factory/inputs/**` content is still sentinel-only:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- `.gitignore` still keeps live workflow submissions under `factory/inputs/**`
  out of normal commits except for those sentinel paths
- the previously queued ignored idea
  `factory/inputs/idea/default/collapse-replay-safe-diagnostics-rehydration.md`
  is stale because its owning work merged through PR `#169`
- after pruning that stale residue, the next maintainer-owned ignored backlog
  slot should be a fresh standalone idea rather than a duplicate replay request

## customer-ask truth

- the local canonical ask file now withdraws the earlier checklist, coverage,
  simplification, and minimum-concurrency backlog for this cycle
- there is therefore no active customer-directed requirement right now to keep
  a minimum number of simultaneous lanes in flight
- open PRs can still inform overlap checks, but they no longer count as
  customer-required work unless the canonical ask file reintroduces them

## recent repo movement

- recent merged PRs on `main` now include:
  - `#170` `weird-work-names`
  - `#169` `collapse-replay-safe-diagnostics-rehydration`
  - `#166` `simplify-loaded-runtime-definition-lookups`
  - `#165` `localize-workflow-activity-graph-import-copy`
  - `#164` `localize-terminal-work-card-copy`
  - `#162` `localize-dashboard-flow-axis-legend-copy`
- `gh pr list --state open` on `2026-05-09` reports:
  - `#172` `same-trace`
  - `#171` `workflow-graph-padding`
  - `#168` `docs: refresh meta world state`
  - `#167` `localize-work-outcome-trend-cards-copy`
  - `#163` `docs: refresh meta world state`
  - `#152` `docs: refresh meta world state`
  - `#145` `docs: refresh meta world state`
  - `#143` `docs: refresh meta world state`
  - `#141` `audit-repository-against-2026-website-and-backend-checklists`
  - `#139` `docs: refresh meta world state`
  - `#123` `docs: refresh meta world state`
  - `#120` `docs: refresh meta world state`

## open-lane truth

- `PR #141` still owns the repository-wide external checklist audit lane
- `PR #167` owns the current `ui/src/features/work-outcome/*` localization lane
- `PR #171` owns the dashboard-shell and workflow-graph padding lane
- `PR #172` owns the same-trace guard lane across config, petri, API, and
  functional coverage
- `PR #168` still owns the previously opened meta-refresh branch for the older
  worldview
- the replay diagnostics dedupe lane is closed on live `main` through merged
  `PR #169`, so it should not remain in the ignored inbox

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is still historical fixture coverage rather than an exact
  copy of the current workflow contract
- one replay rejection payload is still quoted oddly as `"\"<REJECTED>\"\n"`;
  treat that as fixture history rather than live workflow behavior

## next cleanup candidate

- the next maintainer-owned non-overlapping cleanup seam is service-smoke test
  helper dedupe:
  - `tests/functional/smoke/short_helpers_test.go` defines
    `simpleServicePipelineConfig` and `twoStageServicePipelineConfig`
  - `tests/functional/smoke/service_lifecycle_smoke_long_test.go` defines the
    same two helpers again for the `functionallong` lane
  - `tests/functional/smoke/service_config_override_alignment_test.go` already
    consumes the short-lane shared helper, so the duplicate long-lane owner is
    now the drift risk
  - this is a narrow simplification lane that removes duplicated fixture
    builders while preserving the observable service smoke behavior in both the
    default and `functionallong` test lanes
- that seam is not covered by open PRs `#141`, `#167`, `#168`, `#171`, or
  `#172`

## theory of mind

- the authoritative world model comes from live upstream git state, the
  checked-in workflow contract, the canonical ask file, current PR ownership,
  and direct code reads together
- when `factory/logs/meta/asks.md` changes locally, treat that edit as the
  immediate routing truth even if it withdraws a previously active backlog
- reason about `factory/inputs/**` in two layers:
  checked-in contract versus ignored operating state
- prune ignored local idea files once their owning PR merges; otherwise the
  local queue can preserve stale work that the live repo already finished
- treat delegated explorer suggestions as provisional after a fast-forward or
  merge; confirm the seam still exists on live `main` before re-queuing it
- when one functional test lane already exposes shared fixture builders, prefer
  reusing that owner across build-tag variants instead of keeping duplicated
  config definitions in long and short suites
