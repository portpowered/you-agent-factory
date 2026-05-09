# meta view

## world state

- as of `2026-05-09T13:05:17+09:00`, local `main` has been rebased onto live
  `origin/main` at `74b874a` and then carries one local meta-refresh commit
  `57076b3`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- the current canonical local ask file still says there are no active customer
  asks: `for now no asks exists.`
- the tracked maintainer workflow inputs remain sentinel-only under
  `factory/inputs/**`; live work items there are ignored operating state
- the local worktree is still dirty from tracked local edits in
  `factory/logs/meta/asks.md` and `factory/workers/workspace-setup/AGENTS.md`;
  treat those as existing local state, not as noise to revert

## workflow truth

- `factory/factory.json` still defines five work types: `thoughts`, `idea`,
  `plan`, `task`, and `cron-triggers`
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
  `factory/inputs/idea/default/dedupe-service-smoke-pipeline-config-builders.md`
  was stale because merged PR `#174` landed that exact lane on `main`
- that stale ignored idea residue has now been pruned locally so the operating
  queue no longer points at already-merged work

## customer-ask truth

- the local canonical ask file continues to withdraw the earlier checklist,
  coverage, simplification, and minimum-concurrency backlog for this cycle
- there is therefore no active customer-directed requirement right now to keep
  a minimum number of simultaneous lanes in flight
- open PRs can still inform overlap checks, but they do not become asks unless
  `factory/logs/meta/asks.md` reintroduces them

## recent repo movement

- recent merged PRs on `main` now include:
  - `#174` `dedupe-service-smoke-pipeline-config-builders`
  - `#170` `weird-work-names`
  - `#169` `collapse-replay-safe-diagnostics-rehydration`
  - `#166` `simplify-loaded-runtime-definition-lookups`
  - `#165` `localize-workflow-activity-graph-import-copy`
- `gh pr list --state open` on `2026-05-09` reports:
  - `#173` `docs: refresh meta world state`
  - `#172` `same-trace`
  - `#171` `workflow-graph-padding`
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
- `PR #173` is the current open meta-refresh branch and is now older than the
  live rebased local worldview again
- the smoke helper dedupe lane is closed on live `main` through merged
  `PR #174`, so it must not be re-queued

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is still historical fixture coverage rather than an exact
  copy of the current workflow contract
- one replay rejection payload is still quoted oddly as `"\"<REJECTED>\"\n"`;
  treat that as fixture history rather than live workflow behavior

## current maintainer decision

- this cycle does not queue a new cleanup request
- reason:
  - the previously selected next cleanup seam already merged through `PR #174`
  - the delegated explorer's replacement suggestions did not survive direct
    code validation on live `main`
  - `pkg/listeners/filewatcher.go` already rejects direct two-segment
    `inputs/<work-type>/<file>` drops and tests cover the canonical
    `default/`-channel-only contract
  - `pkg/api/server.go` no longer carries a handwritten `/work` pagination shim
    and the generated binding path already owns invalid `maxResults` rejection
  - `pkg/replay/event_stream_artifact.go` still has in-repo test and
    functional callers for the file-wrapper helpers, so that seam is not dead

## theory of mind

- the authoritative world model comes from live upstream git state, the
  checked-in workflow contract, the canonical ask file, current PR ownership,
  ignored queue residue, and direct code reads together
- when `factory/logs/meta/asks.md` changes locally, treat that edit as the
  immediate routing truth even if it withdraws a previously active backlog
- reason about `factory/inputs/**` in two layers:
  checked-in contract versus ignored operating state
- prune ignored local idea files once their owning PR merges; otherwise the
  canonical inbox can preserve stale work that the live repo already finished
- treat delegated explorer suggestions as hypotheses; re-verify them against
  live `main` before dispatching new cleanup work because recent merges can
  invalidate an otherwise plausible seam within the same cycle
