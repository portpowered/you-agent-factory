# meta view

## world state

- as of `2026-05-04T02:04:54-07:00`, local `HEAD` on `main` points to
  `797afee`
  (`normalize-dashboard-header-button-treatment (#86)`)
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
- after pruning stale local residue for merged PR `#69`, merged PR `#84`,
  merged PR `#85`, and merged PR `#86`, the next ignored operating submission
  should be a single standalone submit-work button follow-up under
  `factory/inputs/idea/default/`

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
- merged PR `#85` satisfied the remaining dashboard branding and header
  iconography lane on `main`:
  - `ui/index.html`, `ui/fallback_dist/index.html`, and
    `ui/fallback_dist/assets/index.js` no longer ship the old
    `Agent Factory` / triangle shell contract
  - `ui/src/features/header/dashboard-header.tsx`,
    `ui/src/features/header/dashboard-status-panel.tsx`,
    `ui/src/features/bento/agent-bento.tsx`,
    `ui/src/features/workflow-activity/react-flow-current-activity-card-import.tsx`,
    `ui/src/features/header/dashboard-export-dialog.tsx`, and
    `ui/src/features/export/build-factory-export-filename.ts` now align with
    the Infinite You rename
  - the header stream/export/current controls now use the requested
    pulsating-dot, share-style, and play-style semantics
- merged PR `#86` satisfied the remaining header button-convergence seam on
  `main`:
  - `ui/src/features/header/dashboard-header.tsx` and
    `ui/src/features/header/tick-slider-control.tsx` now route the export and
    return-to-current actions through
    `ui/src/features/header/dashboard-header-action-button.tsx`
  - `ui/src/features/header/dashboard-header.test.tsx` and
    `ui/src/App.stories.tsx` now protect the converged neutral header-action
    treatment through rendered behavior
- the tracked local ask diff now explicitly includes the array-valued
  non-success output-interface request, but that ask is already materially
  satisfied on `main` through merged PR `#69`
- the next selected unowned customer-visible follow-up is therefore the
  submit-work button-tone seam:
  - `ui/src/features/submit-work/submit-work-card.tsx` still renders the
    `Submit work` CTA through the shared `Button` primitive without an explicit
    tone, so it inherits the accent-filled `default` treatment from
    `ui/src/components/ui/button.tsx`
  - that remains visibly outside the open website button ask to converge
    dashboard buttons onto the neutral white-ish treatment instead of the
    primary accent color

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
  - `#86` `normalize-dashboard-header-button-treatment`, merged on
    `2026-05-04T08:36:11Z`
  - `#85` `align-dashboard-branding-and-header-iconography`, merged on
    `2026-05-04T08:00:00Z`
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
- once a narrow ask-owned sweep merges, the next best seam is often the next
  visible outlier within the same customer lane rather than a new abstraction
  push; after PR `#86`, that means the submit-work CTA tone is a better next
  dispatch than reopening header or branding code
- the cleaner prompt currently has a tracked local edit allowing multiple
  non-overlapping items in flight, but that does not remove the need to default
  to one standalone idea when a single seam is sufficient
