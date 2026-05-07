# meta view

## world state

- as of `2026-05-07T02:06:20.9186342-07:00`, local `HEAD` on `main` points to
  `bf2ea7e` (`Merge pull request #138 from portpowered/branchling`) and is
  current with `origin/main`
- the canonical maintainer ask surface remains `factory/logs/meta/asks.md`
- before this refresh, the only visible tracked local edit was the
  user-maintained canonical ask file `factory/logs/meta/asks.md`
- canonical `factory/inputs/**` remains tracked-sentinel-only in git, while
  live maintainer submissions remain ignored operating state

## workflow truth

- `factory/factory.json` still defines five work types: `thoughts`, `idea`,
  `plan`, `task`, and `cron-triggers`
- the checked-in maintainer loop remains:
  `thoughts:init -> ideafy -> thoughts:complete`
  `idea:init -> plan -> idea:complete + plan:init`
  `plan:init -> setup-workspace -> plan:complete + task:init`
  `task:init -> process -> task:in-review -> review -> task:complete`
- topology details that still matter:
  - `process` and `review` run in `.claude/worktrees/{{name}}`
  - shared `executor-slot` capacity is `10`
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
  `factory/inputs/idea/default/cover-gocoveragecheck-helper-runtime-and-parser-branches.md`
  is stale on live `main` because merged PR `#137` landed that exact lane on
  `2026-05-07T03:23:26Z`
- after pruning that stale residue, the maintainer-owned ignored queue should
  carry three fresh non-overlapping idea files so the autonomous lane matches
  the standing ask to keep at least three tasks running:
  - `audit-repository-against-2026-website-and-backend-checklists.md`
  - `localize-and-accessibility-harden-import-preview-dialog.md`
  - `retire-deadcodecheck-gotypesalias-compat-shim.md`

## customer-ask truth

- the canonical ask file still carries one broad active quality lane plus the
  autonomy notice through `2026-05-25`
- the active quality asks remain:
  - follow the external website and backend checklists and create alignment
    tasks
  - keep backend and website testing moving toward declared high-coverage goals
  - keep simplifying backend and website ownership where duplicate or stale
    logic remains
  - keep at least three non-overlapping tasks running at a time
- the external checklist links in `factory/logs/meta/asks.md` still point at
  the live `portpowered/checklists` repository, and the explicit linked docs
  remain the current `2026` checklist revisions:
  - `website-development-checklist.md`
  - `backend-development-checklist.md`
  - `asks.md`
- there is still no checked-in repo-wide review record mapping this repository
  against those external checklist documents; the only checked-in alignment
  checklist found this turn is the narrower import/export lane record at
  `docs/internal/development/import-export-standards-alignment-checklist.md`
- the UI still lacks shared localization infrastructure:
  - `ui/src/i18n` does not exist on live `main`
  - `ui/package.json` does not currently carry an automated accessibility test
    dependency such as `axe-core` or a wrapper matcher

## replay truth

- `factory/logs/agent-fails.json` and
  `factory/logs/agent-fails.replay.json` remain the checked-in replay sample
  pair described in `factory/README.md`
- the replay pair is still historical fixture coverage rather than an exact
  copy of the current workflow contract
- one replay rejection payload is still quoted oddly as `"\"<REJECTED>\"\n"`;
  treat that as fixture history rather than live workflow behavior

## recent repo movement

- recent merged PRs on `main` now include:
  - `#138` `Branchling`, merged on `2026-05-07T08:23:18Z`
  - `#137` `cover-gocoveragecheck-helper-runtime-and-parser-branches`, merged
    on `2026-05-07T03:23:26Z`
  - `#136` `Windows release`, merged on `2026-05-07T00:19:46Z`
  - `#135` `cover-functionallane-command-owner-error-and-entrypoint-branches`,
    merged on `2026-05-06T23:18:17Z`
  - `#134` `cover-gocoveragecheck-command-owner-threshold-and-entrypoint-branches`,
    merged on `2026-05-06T22:18:52Z`
- `gh pr list --state open` still reports only the two older meta-refresh PRs:
  - `#123` `docs: refresh meta world state`
  - `#120` `docs: refresh meta world state`
- those open PRs do not own the next code cleanup or checklist-alignment lane

## next cleanup candidates

- repo-wide standards evidence remains the highest-priority open ask because
  there is still no checked-in audit mapping this repository to the current
  external backend and website checklists
- the narrowest current UI standards lane is the import preview dialog:
  - `ui/src/features/import/dashboard-import-preview-dialog.tsx` still owns a
    concentrated set of hardcoded user-visible strings
  - `ui/src/features/import/dashboard-import-preview-dialog.test.tsx` and
    `.stories.tsx` already provide focused verification seams
  - the repo still lacks `ui/src/i18n` and automated accessibility assertions
    for that feature surface
- the narrowest current backend simplification lane is in `cmd/deadcodecheck`:
  - `runDeadcode()` still injects a `GODEBUG=gotypesalias=1` compatibility shim
    through `deadcodeEnv()` and `ensureGoTypesAliasEnabled()`
  - live manual command checks on the supported Go `1.24.x` toolchain succeed
    both with and without that override, so the shim now appears to be
    redundant legacy handling rather than a live policy requirement

## theory of mind

- the authoritative world model comes from live `main`, the checked-in workflow
  contract, the canonical ask file, current PR state, and current external
  checklist docs together
- `factory/inputs/**` must still be reasoned about in two layers:
  checked-in contract versus ignored operating residue
- when a previously queued idea lands on `main`, prune the ignored residue and
  refresh the next seam from live code and PR state before queuing anything
  else; merged PR `#137` invalidated the previously recorded next seam within
  hours
- when the customer ask requires at least three tasks in flight, satisfy that
  ask with three narrow non-overlapping idea files rather than one broad batch
  unless dependency ordering is actually required
- when checklist conformance is still undocumented, queue one audit lane plus
  smaller implementation-ready follow-ups instead of claiming alignment from
  standards intent alone
