# meta view

## world state

- as of `2026-05-04T00:01:47.3762163-07:00`, local `HEAD` on `main` points to
  `2781170`
  (`Merge pull request #69 from portpowered/ralph/workstation-non-success-route-arrays`)
  and matches `origin/main`
- the local worktree is not clean:
  - tracked local edits exist in `factory/logs/meta/asks.md` and
    `factory/workstations/cleaner/AGENTS.md`
  - ignored local workflow residue exists under `factory/inputs/**`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`

## workflow truth

- `factory/factory.json` still defines five work types: `thoughts`, `idea`,
  `plan`, `task`, and `cron-triggers`
- the checked-in maintainer loop remains:
  `thoughts:init -> ideafy -> thoughts:complete`
  `idea:init -> plan -> idea:to-complete + plan:init`
  `plan:init -> setup-workspace -> plan:complete + task:init`
  `task:init -> process -> task:in-review -> review -> task:to-complete`
  `consume` completes same-name `idea` + `task` pairs once both reach
  `to-complete`
- topology details that still matter:
  - `process` and `review` run in `.claude/worktrees/{{name}}`
  - shared `executor-slot` capacity is `10`; each staffed workstation requests
    `1`
  - hourly `cleaner` emits `cron-triggers:complete`
  - `executor-loop-breaker` fails `task:init` after `process` visit `50`
  - `review-loop-breaker` fails `task:in-review` after `review` visit `10`

## input surface truth

- tracked `factory/inputs/**` content is still sentinel-only:
  - `factory/inputs/BATCH/default/.gitkeep`
  - `factory/inputs/idea/default/.gitkeep`
  - `factory/inputs/plan/default/.gitkeep`
  - `factory/inputs/task/default/.gitkeep`
  - `factory/inputs/thoughts/default/.gitkeep`
- `.gitignore` still ignores live workflow submissions under `factory/inputs/**`
  except those sentinel paths
- the file watcher still enforces the documented three-segment watched-input
  contract and no longer accepts direct
  `factory/inputs/<work-type>/<file>` submissions as an implicit `default`
  channel fallback
- after pruning stale local residue for merged PR `#69` and merged PR `#84`,
  the next ignored operating submission should be a single standalone branding
  follow-up under `factory/inputs/idea/default/`

## customer-ask truth

- the import/export P0 lane is now materially satisfied on `main` through
  merged PRs `#67`, `#68`, `#69`, `#70`, `#71`, and `#72`
- the selected-work current-selection ask is materially satisfied on `main`
  through merged PRs `#74` and `#77`
- the submit-work copy ask is satisfied on `main` through merged PR `#75`
- merged PR `#83` satisfied the header-verbosity copy reduction ask on `main`:
  visible `Factory state`, `Stream`, `Export PNG`, and `Current` toolbar text
  labels are gone
- merged PR `#84` satisfied the work-outcome chart ask on `main`:
  the chart no longer teaches detached axis labels and now carries increased
  axis spacing through rendered chart behavior
- the remaining customer-visible dashboard branding lane is still open:
  - `ui/index.html` still brands the page as `Agent Factory Dashboard`,
    describes it as an Agent Factory shell, and serves the old triangle favicon
  - `ui/src/features/header/dashboard-header.tsx` still renders the main `Agent Factory`
    heading
  - `ui/src/features/header/dashboard-status-panel.tsx` still uses `Agent Factory`
    as eyebrow copy
  - `ui/src/features/bento/agent-bento.tsx` still labels the main board as
    `Agent Factory bento board`
  - `ui/src/features/workflow-activity/react-flow-current-activity-card-import.tsx`
    still teaches `Port OS Agent Factory` PNG import wording
  - `ui/src/features/header/dashboard-export-dialog.tsx` and
    `ui/src/features/export/build-factory-export-filename.ts` still fall back
    to `agent-factory` export names
  - the header controls still have residual iconography drift from the ask:
    the stream status uses check/offline glyphs instead of a pulsating-dot
    contract, the export button still uses a download arrow instead of a share
    icon, and the `Current` button still uses a refresh-style icon instead of
    a play icon
- if the checked-in fallback shell is still served anywhere, its bundled
  branding remains stale too:
  - `ui/fallback_dist/index.html`
  - `ui/fallback_dist/assets/index.js`
- the next selected unowned customer-visible follow-up is therefore a single
  branding-and-header-iconography sweep

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is still historical fixture coverage rather than an exact
  copy of the current workflow contract; it predates `to-complete`, `consume`,
  and the current `executor-slot` capacity of `10`
- replay outcome counts remain unchanged in the sample:
  - `process`: `9 ACCEPTED <COMPLETE>`, `27 CONTINUE <CONTINUE>`
  - `review`: `5 ACCEPTED <COMPLETE>`, `4 REJECTED <REJECTED>`

## recent repo movement

- recent merged PRs on `main` now include:
  - `#69` `workstation-non-success-route-arrays`, merged into `main` before the
    current refresh and now represented by `HEAD`
  - `#84` `align-work-outcome-chart-axis-labels-and-margins`, merged on
    `2026-05-04T06:31:54Z`
  - `#83` `simplify-dashboard-header-toolbar-verbosity`, merged on
    `2026-05-04T05:53:10Z`
  - `#82` `trim-ralph-starter-input-readme-contract`, merged on
    `2026-05-04T04:20:52Z`
  - `#81` `trim-starter-input-readme-contract`, merged on
    `2026-05-04T03:18:53Z`
  - `#80` `align-default-starter-task-input-contract`, merged on
    `2026-05-04T02:36:26Z`
  - `#79` `retire-filewatcher-default-channel-fallback`, merged on
    `2026-05-04T01:27:11Z`
  - `#78` `remove-list-work-legacy-pagination-shim`, merged on
    `2026-05-04T00:28:40Z`
- `gh pr list --state open` currently reports no open PRs

## theory of mind

- the authoritative world model still comes from live git state plus the
  checked-in workflow contract, not from replay fixtures alone
- `factory/inputs/**` must always be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- ignored queue residue can become stale within a single merge cycle, so the
  meta loop has to reconcile local inbox files against the newest merged PRs
  before dispatching anything new
- merged PR state can flip between the start of a refresh and the end of it;
  open-PR assumptions need to be revalidated after `git pull` and before queue
  writes
- once a customer-visible ask lands, the right follow-up is usually not another
  broad lane but the smallest remaining seam that preserves the ask's public
  intent without reopening merged work
- after PR `#69`, PR `#83`, and PR `#84`, the cleanest next non-overlapping
  dispatch seam is the dashboard branding/iconography sweep because it stays
  isolated to the UI shell, metadata, and import/export naming copy
